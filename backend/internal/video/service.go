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

	"video_share/internal/taxonomy"
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
	ErrListQueryInvalid   = errors.New("invalid list query")
	ErrVisibilityInvalid  = errors.New("invalid video visibility")
	ErrPatchEmpty         = errors.New("empty video update")
	ErrRelatedInvalid     = errors.New("invalid related video request")
)

// maxSearchQueryRunes bounds the discovery keyword after trimming.
const maxSearchQueryRunes = 50

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
	taxonomy     taxonomy.Manager
}

func NewService(repo Repository, store ObjectStore, maxFileSize int64, uploadExpiry, playExpiry time.Duration, managers ...taxonomy.Manager) *Service {
	service := &Service{
		repo:         repo,
		store:        store,
		maxFileSize:  maxFileSize,
		uploadExpiry: uploadExpiry,
		playExpiry:   playExpiry,
	}
	if len(managers) > 0 {
		service.taxonomy = managers[0]
	}
	return service
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
	categoryID := taxonomy.UncategorizedID
	var selection taxonomy.Selection
	if s.taxonomy != nil {
		var selectionErr error
		selection, selectionErr = s.taxonomy.ValidateSelection(ctx, req.CategoryID, req.Tags)
		if selectionErr != nil {
			return nil, selectionErr
		}
		categoryID = selection.CategoryID
	}

	pendingKey, err := newPendingObjectKey(userID)
	if err != nil {
		return nil, fmt.Errorf("generate pending video key: %w", err)
	}
	v := &Video{
		UserID:      userID,
		CategoryID:  categoryID,
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
	if s.taxonomy != nil {
		if err := s.taxonomy.ApplyVideo(ctx, v.ID, selection); err != nil {
			return nil, err
		}
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

// ListPublic validates and normalizes a discovery query before handing it to the
// repository. An empty keyword is an ordinary public listing.
func (s *Service) ListPublic(ctx context.Context, query ListQuery) (*ListResponse, error) {
	if err := validatePagination(query.Page, query.PageSize); err != nil {
		return nil, err
	}
	query.Query = strings.TrimSpace(query.Query)
	if utf8.RuneCountInString(query.Query) > maxSearchQueryRunes {
		return nil, ErrListQueryInvalid
	}
	if s.taxonomy != nil {
		var err error
		normalizedTags, err := s.taxonomy.NormalizeTags(query.Tags)
		if err != nil {
			return nil, err
		}
		query.Tags = make([]string, 0, len(normalizedTags))
		for _, tag := range normalizedTags {
			query.Tags = append(query.Tags, tag.Normalized)
		}
	}
	switch query.Sort {
	case "":
		query.Sort = SortLatest
	case SortLatest, SortPopular:
	default:
		return nil, ErrListQueryInvalid
	}
	videos, total, err := s.repo.ListPublic(ctx, query)
	if err != nil {
		return nil, err
	}
	if err := s.enrich(ctx, videos); err != nil {
		return nil, err
	}
	return &ListResponse{Items: toVideoResponses(videos), Page: query.Page, PageSize: query.PageSize, Total: total}, nil
}

func (s *Service) Following(ctx context.Context, userID uint64, query ListQuery) (*ListResponse, error) {
	if userID == 0 {
		return nil, ErrUnauthorized
	}
	if err := validatePagination(query.Page, query.PageSize); err != nil {
		return nil, err
	}
	query.Query = strings.TrimSpace(query.Query)
	if utf8.RuneCountInString(query.Query) > maxSearchQueryRunes {
		return nil, ErrListQueryInvalid
	}
	if query.Sort == "" {
		query.Sort = SortLatest
	}
	if query.Sort != SortLatest && query.Sort != SortPopular {
		return nil, ErrListQueryInvalid
	}
	videos, total, err := s.repo.ListFollowing(ctx, userID, query.Page, query.PageSize, query)
	if err != nil {
		return nil, err
	}
	if err := s.enrich(ctx, videos); err != nil {
		return nil, err
	}
	return &ListResponse{Items: toVideoResponses(videos), Page: query.Page, PageSize: query.PageSize, Total: total}, nil
}

func (s *Service) Related(ctx context.Context, videoID uint64) (*ListResponse, error) {
	if videoID == 0 {
		return nil, ErrNotFound
	}
	if _, err := s.repo.FindPublicByID(ctx, videoID); err != nil {
		return nil, err
	}
	videos, err := s.repo.ListRelated(ctx, videoID, 6)
	if err != nil {
		return nil, err
	}
	if err := s.enrich(ctx, videos); err != nil {
		return nil, err
	}
	return &ListResponse{Items: toVideoResponses(videos), Page: 1, PageSize: len(videos), Total: int64(len(videos))}, nil
}

// Detail returns public playback metadata for an anonymous viewer.
func (s *Service) Detail(ctx context.Context, videoID uint64) (*DetailResponse, error) {
	return s.DetailForViewer(ctx, 0, videoID)
}

// DetailForViewer resolves public playback metadata and, for a logged-in
// viewer, the relationships that viewer holds for this video.
func (s *Service) DetailForViewer(ctx context.Context, viewerID, videoID uint64) (*DetailResponse, error) {
	if videoID == 0 {
		return nil, ErrNotFound
	}
	v, err := s.repo.FindPublicByID(ctx, videoID)
	if err != nil {
		return nil, err
	}
	result, err := s.playbackResponse(ctx, v)
	if err != nil {
		return nil, err
	}
	if viewerID != 0 {
		state, err := s.repo.ViewerState(ctx, viewerID, v.ID, v.UserID)
		if err != nil {
			return nil, err
		}
		result.ViewerState = state
	}
	single := []Video{*v}
	if err := s.enrich(ctx, single); err != nil {
		return nil, err
	}
	*v = single[0]
	result.VideoResponse = toVideoResponse(v)
	return result, nil
}

func (s *Service) playbackResponse(ctx context.Context, v *Video) (*DetailResponse, error) {
	playURL := fmt.Sprintf("/api/v1/videos/%d/hls/master.m3u8", v.ID)
	playType := "hls"
	playExpiresIn := int64(0)
	if v.HLSMasterKey == nil || *v.HLSMasterKey == "" {
		url, err := s.store.PresignGet(ctx, v.ObjectKey, s.playExpiry)
		if err != nil {
			return nil, fmt.Errorf("create video playback URL: %w", err)
		}
		playURL = url
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

// escapeLike escapes the escape character, `%`, and `_` so a search keyword
// matches literally instead of acting as a wildcard.
func escapeLike(value string, escape rune) string {
	var b strings.Builder
	b.Grow(len(value))
	for _, r := range value {
		if r == escape || r == '%' || r == '_' {
			b.WriteRune(escape)
		}
		b.WriteRune(r)
	}
	return b.String()
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
	if err := s.enrich(ctx, videos); err != nil {
		return nil, err
	}
	return &OwnerListResponse{Items: toOwnerVideoResponses(videos), Page: page, PageSize: pageSize, Total: total}, nil
}

func (s *Service) MineDetail(ctx context.Context, userID, videoID uint64) (*OwnerVideoResponse, error) {
	v, err := s.ownedVideo(ctx, userID, videoID)
	if err != nil {
		return nil, err
	}
	single := []Video{*v}
	if err := s.enrich(ctx, single); err != nil {
		return nil, err
	}
	*v = single[0]
	result := toOwnerVideoResponse(v)
	return &result, nil
}

// Update applies an author's partial update to their own video. Fields that were
// left out keep their stored value, and the first publish time is preserved.
func (s *Service) Update(ctx context.Context, userID, videoID uint64, req UpdateRequest) (*OwnerVideoResponse, error) {
	v, err := s.ownedVideo(ctx, userID, videoID)
	if err != nil {
		return nil, err
	}
	patch, err := buildVideoPatch(req)
	if err != nil {
		return nil, err
	}
	var selection taxonomy.Selection
	if s.taxonomy != nil && (patch.CategoryID != nil || patch.Tags != nil) {
		categoryID := v.CategoryID
		tags := []string(nil)
		if current, loadErr := s.taxonomy.ForVideos(ctx, []uint64{v.ID}); loadErr != nil {
			return nil, loadErr
		} else if existing, ok := current[v.ID]; ok {
			for _, tag := range existing.Tags {
				tags = append(tags, tag.DisplayName)
			}
		}
		if patch.CategoryID != nil {
			categoryID = *patch.CategoryID
		}
		if patch.Tags != nil {
			tags = *patch.Tags
		}
		selection, err = s.taxonomy.ValidateSelection(ctx, categoryID, tags)
		if err != nil {
			return nil, err
		}
	}
	if err := s.repo.UpdateOwned(ctx, userID, v.ID, patch); err != nil {
		return nil, err
	}
	if s.taxonomy != nil && (patch.CategoryID != nil || patch.Tags != nil) {
		if err := s.taxonomy.ApplyVideo(ctx, v.ID, selection); err != nil {
			return nil, err
		}
	}
	updated, err := s.repo.FindByID(ctx, v.ID)
	if err != nil {
		return nil, err
	}
	single := []Video{*updated}
	if err := s.enrich(ctx, single); err != nil {
		return nil, err
	}
	*updated = single[0]
	result := toOwnerVideoResponse(updated)
	return &result, nil
}

// Delete logically deletes an author's own video. Repeating the call reports the
// video as missing rather than reviving or corrupting it.
func (s *Service) Delete(ctx context.Context, userID, videoID uint64) error {
	v, err := s.ownedVideo(ctx, userID, videoID)
	if err != nil {
		return err
	}
	return s.repo.DeleteOwned(ctx, userID, v.ID)
}

// ownedVideo loads a video and rejects the request unless the caller owns it.
// Deleted videos are reported as missing so they never leak their former owner.
func (s *Service) ownedVideo(ctx context.Context, userID, videoID uint64) (*Video, error) {
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
	return v, nil
}

// buildVideoPatch validates and trims an update request into the partial patch
// the repository applies. An update that names no field is rejected outright.
func buildVideoPatch(req UpdateRequest) (VideoPatch, error) {
	var patch VideoPatch
	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		if n := utf8.RuneCountInString(title); n == 0 || n > 100 {
			return VideoPatch{}, ErrTitleInvalid
		}
		patch.Title = &title
	}
	if req.Description != nil {
		description := strings.TrimSpace(*req.Description)
		if utf8.RuneCountInString(description) > 2000 {
			return VideoPatch{}, ErrDescriptionInvalid
		}
		patch.Description = &description
	}
	if req.Visibility != nil {
		visibility, ok := parseVisibility(*req.Visibility)
		if !ok {
			return VideoPatch{}, ErrVisibilityInvalid
		}
		patch.Visibility = &visibility
	}
	if req.CategoryID != nil {
		if *req.CategoryID == 0 {
			return VideoPatch{}, taxonomy.ErrCategoryNotFound
		}
		patch.CategoryID = req.CategoryID
	}
	if req.Tags != nil {
		tags := append([]string(nil), (*req.Tags)...)
		patch.Tags = &tags
	}
	if patch.Title == nil && patch.Description == nil && patch.Visibility == nil && patch.CategoryID == nil && patch.Tags == nil {
		return VideoPatch{}, ErrPatchEmpty
	}
	return patch, nil
}

func (s *Service) enrich(ctx context.Context, videos []Video) error {
	if s.taxonomy == nil || len(videos) == 0 {
		return nil
	}
	ids := make([]uint64, 0, len(videos))
	for i := range videos {
		ids = append(ids, videos[i].ID)
	}
	values, err := s.taxonomy.ForVideos(ctx, ids)
	if err != nil {
		return err
	}
	for i := range videos {
		value, ok := values[videos[i].ID]
		if !ok {
			continue
		}
		if value.Category != nil {
			videos[i].Category = &CategoryInfo{ID: value.Category.ID, Slug: value.Category.Slug, Name: value.Category.Name}
		}
		videos[i].Tags = make([]TagInfo, 0, len(value.Tags))
		for _, tag := range value.Tags {
			videos[i].Tags = append(videos[i].Tags, TagInfo{ID: tag.ID, Name: tag.DisplayName})
		}
	}
	return nil
}

func parseVisibility(value string) (Visibility, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "public":
		return VisibilityPublic, true
	case "private":
		return VisibilityPrivate, true
	default:
		return 0, false
	}
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
	v, err := s.repo.FindPublicByID(ctx, videoID)
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
	v, err := s.repo.FindPublicByID(ctx, videoID)
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
