package ranking

import (
	"context"
	"errors"
	"time"

	"video_share/internal/video"
)

var ErrPagination = errors.New("invalid ranking pagination")

type Service struct {
	repo Repository
	now  func() time.Time
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo, now: time.Now}
}

func (s *Service) WithClock(now func() time.Time) *Service {
	if now != nil {
		s.now = now
	}
	return s
}

func (s *Service) Rebuild(ctx context.Context, window Window, now time.Time) (time.Time, error) {
	if s == nil || s.repo == nil {
		return time.Time{}, errors.New("ranking repository is nil")
	}
	return s.repo.Rebuild(ctx, window, now)
}

func (s *Service) List(ctx context.Context, window Window, page, pageSize int) (Response, error) {
	if page < 1 || page > 1_000_000 || pageSize < 1 || pageSize > 50 {
		return Response{}, ErrPagination
	}
	if _, err := ParseWindow(string(window)); err != nil {
		return Response{}, err
	}
	candidates, total, generatedAt, err := s.repo.List(ctx, window, page, pageSize, s.now())
	if err != nil {
		return Response{}, err
	}
	items := make([]RankingItem, 0, len(candidates))
	responses := video.ToVideoResponses(videoSlice(candidates))
	for i := range candidates {
		items = append(items, RankingItem{VideoResponse: responses[i], VideoID: candidates[i].VideoID, Score: candidates[i].Score, Rank: (page-1)*pageSize + i + 1})
	}
	return Response{Window: window, GeneratedAt: generatedAt, Page: page, PageSize: pageSize, Total: total, Items: items}, nil
}

func videoSlice(candidates []Candidate) []video.Video {
	result := make([]video.Video, 0, len(candidates))
	for _, candidate := range candidates {
		result = append(result, candidate.Video)
	}
	return result
}
