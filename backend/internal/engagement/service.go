package engagement

import (
	"context"
	"strings"
	"unicode/utf8"

	"video_share/internal/video"
)

const (
	defaultPageSize = 12
	maxPageSize     = 50
	maxCommentRunes = 500
)

type Service struct {
	repo Repository
}

type threadedCommentRepository interface {
	CreateCommentWithRequest(ctx context.Context, userID, videoID uint64, content string, parentID *uint64, requestID string) (*Comment, error)
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// SetLike 启用或停用当前用户对某个视频的点赞，返回最终状态。缺少用户或视频时
// 在触达存储前就拒绝。
func (s *Service) SetLike(ctx context.Context, userID, videoID uint64, active bool) (*RelationState, error) {
	return s.setRelation(ctx, userID, videoID, active, s.repo.SetLike)
}

// SetFavorite 启用或停用当前用户对某个视频的收藏，返回最终状态。
func (s *Service) SetFavorite(ctx context.Context, userID, videoID uint64, active bool) (*RelationState, error) {
	return s.setRelation(ctx, userID, videoID, active, s.repo.SetFavorite)
}

func (s *Service) setRelation(ctx context.Context, userID, videoID uint64, active bool, write func(context.Context, uint64, uint64, bool) (*RelationState, error)) (*RelationState, error) {
	if userID == 0 {
		return nil, ErrUnauthorized
	}
	if videoID == 0 {
		return nil, ErrVideoNotFound
	}
	return write(ctx, userID, videoID, active)
}

// CreateComment 校验并规整评论内容，再去重首尾空白后写入。内容长度按字符计数，
// 允许 1 到 500 个字符。
func (s *Service) CreateComment(ctx context.Context, userID, videoID uint64, content string) (*CommentResponse, error) {
	return s.CreateCommentWithRequest(ctx, userID, videoID, content, nil, "")
}

func (s *Service) CreateCommentWithRequest(ctx context.Context, userID, videoID uint64, content string, parentID *uint64, requestID string) (*CommentResponse, error) {
	if userID == 0 {
		return nil, ErrUnauthorized
	}
	if videoID == 0 {
		return nil, ErrVideoNotFound
	}
	trimmed := strings.TrimSpace(content)
	if n := utf8.RuneCountInString(trimmed); n == 0 || n > maxCommentRunes {
		return nil, ErrContentInvalid
	}
	if parentID != nil && *parentID == 0 {
		return nil, ErrParentCommentInvalid
	}
	var comment *Comment
	var err error
	if threaded, ok := s.repo.(threadedCommentRepository); ok {
		comment, err = threaded.CreateCommentWithRequest(ctx, userID, videoID, trimmed, parentID, requestID)
	} else if parentID != nil || requestID != "" {
		return nil, ErrParentCommentInvalid
	} else {
		comment, err = s.repo.CreateComment(ctx, userID, videoID, trimmed)
	}
	if err != nil {
		return nil, err
	}
	result := toCommentResponse(comment)
	return &result, nil
}

func (s *Service) ListReplies(ctx context.Context, commentID uint64, page, pageSize int) (*CommentListResponse, error) {
	if commentID == 0 {
		return nil, ErrCommentNotFound
	}
	if err := validatePagination(page, pageSize); err != nil {
		return nil, err
	}
	comments, total, err := s.repo.ListReplies(ctx, commentID, page, pageSize)
	if err != nil {
		return nil, err
	}
	items := make([]CommentResponse, 0, len(comments))
	for i := range comments {
		items = append(items, toCommentResponse(&comments[i]))
	}
	return &CommentListResponse{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

func (s *Service) ListComments(ctx context.Context, videoID uint64, page, pageSize int) (*CommentListResponse, error) {
	if videoID == 0 {
		return nil, ErrVideoNotFound
	}
	if err := validatePagination(page, pageSize); err != nil {
		return nil, err
	}
	comments, total, err := s.repo.ListComments(ctx, videoID, page, pageSize)
	if err != nil {
		return nil, err
	}
	items := make([]CommentResponse, 0, len(comments))
	for i := range comments {
		items = append(items, toCommentResponse(&comments[i]))
	}
	return &CommentListResponse{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

func (s *Service) DeleteComment(ctx context.Context, userID, commentID uint64) error {
	if userID == 0 {
		return ErrUnauthorized
	}
	if commentID == 0 {
		return ErrCommentNotFound
	}
	return s.repo.DeleteComment(ctx, userID, commentID)
}

// RecordWatch 记录观看进度。只有第一次观看才增加播放量；时长未知时允许任意进度。
func (s *Service) RecordWatch(ctx context.Context, userID, videoID, progressMS, durationMS uint64) error {
	if userID == 0 {
		return ErrUnauthorized
	}
	if videoID == 0 {
		return ErrVideoNotFound
	}
	if durationMS != 0 && progressMS > durationMS {
		return ErrProgressInvalid
	}
	return s.repo.RecordWatch(ctx, userID, videoID, progressMS, durationMS)
}

func (s *Service) ListFavorites(ctx context.Context, userID uint64, page, pageSize int) (*video.ListResponse, error) {
	if userID == 0 {
		return nil, ErrUnauthorized
	}
	if err := validatePagination(page, pageSize); err != nil {
		return nil, err
	}
	videos, total, err := s.repo.ListFavorites(ctx, userID, page, pageSize)
	if err != nil {
		return nil, err
	}
	return &video.ListResponse{
		Items:    video.ToVideoResponses(videos),
		Page:     page,
		PageSize: pageSize,
		Total:    total,
	}, nil
}

func (s *Service) ListHistory(ctx context.Context, userID uint64, page, pageSize int) (*video.ListResponse, error) {
	if userID == 0 {
		return nil, ErrUnauthorized
	}
	if err := validatePagination(page, pageSize); err != nil {
		return nil, err
	}
	videos, total, err := s.repo.ListHistory(ctx, userID, page, pageSize)
	if err != nil {
		return nil, err
	}
	return &video.ListResponse{
		Items:    video.ToVideoResponses(videos),
		Page:     page,
		PageSize: pageSize,
		Total:    total,
	}, nil
}

func validatePagination(page, pageSize int) error {
	if page < 1 || pageSize < 1 || pageSize > maxPageSize {
		return ErrPaginationInvalid
	}
	return nil
}
