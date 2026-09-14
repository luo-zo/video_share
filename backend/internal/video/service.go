package video

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrUnauthorized       = errors.New("authentication required")
	ErrForbidden          = errors.New("video does not belong to current user")
	ErrNotFound           = errors.New("video not found")
	ErrTitleInvalid       = errors.New("invalid video title")
	ErrDescriptionInvalid = errors.New("invalid video description")
	ErrFileNameInvalid    = errors.New("invalid video file name")
	ErrContentTypeInvalid = errors.New("invalid video content type")
	ErrFileSizeInvalid    = errors.New("invalid video file size")
	ErrPaginationInvalid  = errors.New("invalid pagination")
	ErrUploadIncomplete   = errors.New("video upload is incomplete")
	ErrUploadMismatch     = errors.New("uploaded object does not match submission")
	ErrStateConflict      = errors.New("video state conflict")
)

type ObjectStore interface {
	PresignPut(ctx context.Context, key, contentType string, expiry time.Duration) (string, error)
	PresignGet(ctx context.Context, key string, expiry time.Duration) (string, error)
	PresignGetAs(ctx context.Context, key, contentType string, expiry time.Duration) (string, error)
	Stat(ctx context.Context, key string) (size int64, contentType string, err error)
	Read(ctx context.Context, key string, maxBytes int64) ([]byte, error)
}

type Service struct {
	repo         Repository
	store        ObjectStore
	maxFileSize  int64
	uploadExpiry time.Duration
	playExpiry   time.Duration
}

func NewService(repo Repository, store ObjectStore, maxFileSize int64, uploadExpiry, playExpiry time.Duration) *Service {
	return &Service{
		repo:         repo,
		store:        store,
		maxFileSize:  maxFileSize,
		uploadExpiry: uploadExpiry,
		playExpiry:   playExpiry,
	}
}

func (s *Service) Create(ctx context.Context, userID uint64, req CreateRequest) (*CreateResponse, error) {
	if userID == 0 {
		return nil, ErrUnauthorized
	}
	title := strings.TrimSpace(req.Title)
	if n := utf8.RuneCountInString(title); n == 0 || n > 100 {
		return nil, ErrTitleInvalid
	}
	description := strings.TrimSpace(req.Description)
	if utf8.RuneCountInString(description) > 2000 {
		return nil, ErrDescriptionInvalid
	}
	fileName := strings.TrimSpace(req.FileName)
	baseName := strings.TrimSuffix(filepath.Base(fileName), filepath.Ext(fileName))
	if fileName == "" || strings.TrimSpace(baseName) == "" || !strings.EqualFold(filepath.Ext(fileName), ".mp4") {
		return nil, ErrFileNameInvalid
	}
	contentType := strings.ToLower(strings.TrimSpace(req.ContentType))
	if contentType != "video/mp4" {
		return nil, ErrContentTypeInvalid
	}
	if req.FileSize <= 0 || req.FileSize > s.maxFileSize {
		return nil, ErrFileSizeInvalid
	}

	pendingKey, err := newPendingObjectKey(userID)
	if err != nil {
		return nil, fmt.Errorf("generate pending video key: %w", err)
	}
	v := &Video{
		UserID:      userID,
		Title:       title,
		Description: description,
		ObjectKey:   pendingKey,
		Status:      StatusUploading,
		FileSize:    req.FileSize,
		ContentType: contentType,
	}
	if err := s.repo.Create(ctx, v); err != nil {
		return nil, err
	}

	objectKey := fmt.Sprintf("videos/%d/%d/source.mp4", userID, v.ID)
	if err := s.repo.UpdateObjectKey(ctx, v.ID, objectKey); err != nil {
		return nil, err
	}
	v.ObjectKey = objectKey
	uploadURL, err := s.store.PresignPut(ctx, objectKey, contentType, s.uploadExpiry)
	if err != nil {
		return nil, fmt.Errorf("create video upload URL: %w", err)
	}
	return &CreateResponse{
		VideoResponse:   toVideoResponse(v),
		UploadURL:       uploadURL,
		UploadExpiresIn: int64(s.uploadExpiry.Seconds()),
	}, nil
}

func (s *Service) Complete(ctx context.Context, userID, videoID uint64) (*VideoResponse, error) {
	if userID == 0 {
		return nil, ErrUnauthorized
	}
	if videoID == 0 {
		return nil, ErrNotFound
	}
	v, err := s.repo.FindByID(ctx, videoID)
	if err != nil {
		return nil, err
	}
	if v.Status == StatusDeleted {
		return nil, ErrNotFound
	}
	if v.UserID != userID {
		return nil, ErrForbidden
	}
	if v.Status == StatusReady {
		result := toVideoResponse(v)
		return &result, nil
	}
	if v.Status == StatusProcessing {
		result := toVideoResponse(v)
		return &result, nil
	}
	if v.Status != StatusUploading {
		return nil, ErrStateConflict
	}

	size, contentType, err := s.store.Stat(ctx, v.ObjectKey)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUploadIncomplete, err)
	}
	if size != v.FileSize || !strings.EqualFold(strings.TrimSpace(contentType), v.ContentType) {
		return nil, ErrUploadMismatch
	}
	if err := s.repo.BeginProcessing(ctx, v); err != nil {
		if errors.Is(err, ErrStateConflict) {
			current, findErr := s.repo.FindByID(ctx, v.ID)
			if findErr == nil && (current.Status == StatusProcessing || current.Status == StatusReady) {
				result := toVideoResponse(current)
				return &result, nil
			}
		}
		return nil, err
	}
	v.Status = StatusProcessing
	v.ProcessingProgress = 0
	result := toVideoResponse(v)
	return &result, nil
}

func (s *Service) List(ctx context.Context, page, pageSize int) (*ListResponse, error) {
	if err := validatePagination(page, pageSize); err != nil {
		return nil, err
	}
	videos, total, err := s.repo.ListReady(ctx, page, pageSize)
	if err != nil {
		return nil, err
	}
	return &ListResponse{Items: toVideoResponses(videos), Page: page, PageSize: pageSize, Total: total}, nil
}

func (s *Service) Detail(ctx context.Context, videoID uint64) (*DetailResponse, error) {
	if videoID == 0 {
		return nil, ErrNotFound
	}
	v, err := s.repo.FindReadyByID(ctx, videoID)
	if err != nil {
		return nil, err
	}
	playURL := fmt.Sprintf("/api/v1/videos/%d/hls/master.m3u8", v.ID)
	playType := "hls"
	playExpiresIn := int64(0)
	if v.HLSMasterKey == nil || *v.HLSMasterKey == "" {
		playURL, err = s.store.PresignGet(ctx, v.ObjectKey, s.playExpiry)
		if err != nil {
			return nil, fmt.Errorf("create video playback URL: %w", err)
		}
		playType = "mp4"
		playExpiresIn = int64(s.playExpiry.Seconds())
	}
	return &DetailResponse{
		VideoResponse: toVideoResponse(v),
		PlayURL:       playURL,
		PlayType:      playType,
		PlayExpiresIn: playExpiresIn,
	}, nil
}

func (s *Service) Mine(ctx context.Context, userID uint64, page, pageSize int) (*OwnerListResponse, error) {
	if userID == 0 {
		return nil, ErrUnauthorized
	}
	if err := validatePagination(page, pageSize); err != nil {
		return nil, err
	}
	videos, total, err := s.repo.ListByUser(ctx, userID, page, pageSize)
	if err != nil {
		return nil, err
	}
	return &OwnerListResponse{Items: toOwnerVideoResponses(videos), Page: page, PageSize: pageSize, Total: total}, nil
}

func (s *Service) MineDetail(ctx context.Context, userID, videoID uint64) (*OwnerVideoResponse, error) {
	if userID == 0 {
		return nil, ErrUnauthorized
	}
	v, err := s.repo.FindByID(ctx, videoID)
	if err != nil {
		return nil, err
	}
	if v.Status == StatusDeleted {
		return nil, ErrNotFound
	}
	if v.UserID != userID {
		return nil, ErrForbidden
	}
	result := toOwnerVideoResponse(v)
	return &result, nil
}

func (s *Service) HLSManifest(ctx context.Context, videoID uint64, manifestPath string) ([]byte, error) {
	v, relative, key, err := s.hlsObject(ctx, videoID, manifestPath)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(filepath.Ext(relative), ".m3u8") {
		return nil, ErrInvalidMediaPath
	}
	data, err := s.store.Read(ctx, key, 1024*1024)
	if err != nil {
		return nil, fmt.Errorf("read HLS manifest: %w", err)
	}
	return rewriteHLSManifest(v.ID, relative, data)
}

func (s *Service) HLSObjectURL(ctx context.Context, videoID uint64, objectPath string) (string, error) {
	_, relative, key, err := s.hlsObject(ctx, videoID, objectPath)
	if err != nil {
		return "", err
	}
	if strings.EqualFold(filepath.Ext(relative), ".m3u8") {
		return "", ErrInvalidMediaPath
	}
	contentType := hlsContentType(relative)
	url, err := s.store.PresignGetAs(ctx, key, contentType, s.playExpiry)
	if err != nil {
		return "", fmt.Errorf("presign HLS object: %w", err)
	}
	return url, nil
}

func (s *Service) CoverURL(ctx context.Context, videoID uint64) (string, error) {
	v, err := s.repo.FindReadyByID(ctx, videoID)
	if err != nil {
		return "", err
	}
	if v.CoverObjectKey == nil || *v.CoverObjectKey == "" {
		return "", ErrNotFound
	}
	url, err := s.store.PresignGetAs(ctx, *v.CoverObjectKey, "image/jpeg", s.playExpiry)
	if err != nil {
		return "", fmt.Errorf("presign video cover: %w", err)
	}
	return url, nil
}

func (s *Service) hlsObject(ctx context.Context, videoID uint64, requested string) (*Video, string, string, error) {
	v, err := s.repo.FindReadyByID(ctx, videoID)
	if err != nil {
		return nil, "", "", err
	}
	if v.HLSMasterKey == nil || *v.HLSMasterKey == "" {
		return nil, "", "", ErrNotFound
	}
	relative, err := safeHLSRelativePath(requested)
	if err != nil {
		return nil, "", "", err
	}
	prefix := strings.TrimSuffix(filepath.ToSlash(filepath.Dir(*v.HLSMasterKey)), "/")
	key := prefix + "/" + relative
	if !strings.HasPrefix(key, prefix+"/") {
		return nil, "", "", ErrInvalidMediaPath
	}
	return v, relative, key, nil
}

func hlsContentType(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".ts":
		return "video/mp2t"
	case ".m4s":
		return "video/iso.segment"
	case ".key":
		return "application/octet-stream"
	default:
		return "application/octet-stream"
	}
}

func validatePagination(page, pageSize int) error {
	if page < 1 || pageSize < 1 || pageSize > 50 {
		return ErrPaginationInvalid
	}
	return nil
}

func newPendingObjectKey(userID uint64) (string, error) {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return fmt.Sprintf("pending/%d/%s", userID, hex.EncodeToString(random)), nil
}
