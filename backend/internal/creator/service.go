package creator

import (
	"context"
	"errors"

	"video_share/internal/follow"
	"video_share/internal/video"
)

var ErrPaginationInvalid = errors.New("invalid pagination")

const (
	defaultPageSize = 20
	maxPageSize     = 50
)

type Service struct {
	repo    Repository
	videos  video.Repository
	follows *follow.Service
}

func NewService(repo Repository, videos video.Repository, follows *follow.Service) *Service {
	return &Service{repo: repo, videos: videos, follows: follows}
}

func (s *Service) Profile(ctx context.Context, creatorID, viewerID uint64) (*ProfileResponse, error) {
	if s == nil || s.repo == nil || creatorID == 0 {
		return nil, ErrNotFound
	}
	return s.repo.FindPublicProfile(ctx, creatorID, viewerID)
}

func (s *Service) Videos(ctx context.Context, creatorID uint64, page, pageSize int) (*VideoListResponse, error) {
	if s == nil || s.videos == nil || creatorID == 0 {
		return nil, ErrNotFound
	}
	if err := validatePagination(page, pageSize); err != nil {
		return nil, err
	}
	if _, err := s.repo.FindPublicProfile(ctx, creatorID, 0); err != nil {
		return nil, err
	}
	items, total, err := s.videos.ListPublicByUser(ctx, creatorID, page, pageSize)
	if err != nil {
		return nil, err
	}
	return &VideoListResponse{Items: video.ToVideoResponses(items), Page: page, PageSize: pageSize, Total: total}, nil
}

func (s *Service) Followers(ctx context.Context, creatorID uint64, page, pageSize int) (*follow.FollowListResponse, error) {
	if s == nil || s.follows == nil || creatorID == 0 {
		return nil, ErrNotFound
	}
	if err := validatePagination(page, pageSize); err != nil {
		return nil, err
	}
	if _, err := s.repo.FindPublicProfile(ctx, creatorID, 0); err != nil {
		return nil, err
	}
	return s.follows.ListFollowers(ctx, creatorID, page, pageSize)
}

func (s *Service) Following(ctx context.Context, creatorID uint64, page, pageSize int) (*follow.FollowListResponse, error) {
	if s == nil || s.follows == nil || creatorID == 0 {
		return nil, ErrNotFound
	}
	if err := validatePagination(page, pageSize); err != nil {
		return nil, err
	}
	if _, err := s.repo.FindPublicProfile(ctx, creatorID, 0); err != nil {
		return nil, err
	}
	return s.follows.ListFollowing(ctx, creatorID, page, pageSize)
}

func validatePagination(page, pageSize int) error {
	if page < 1 || pageSize < 1 || pageSize > maxPageSize {
		return ErrPaginationInvalid
	}
	return nil
}

func DefaultPageSize() int { return defaultPageSize }
