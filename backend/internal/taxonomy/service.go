package taxonomy

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"video_share/internal/cache"
)

const (
	UncategorizedID = uint64(1)
	maxTagRunes     = 20
	maxTags         = 5
)

type Manager interface {
	ValidateSelection(ctx context.Context, categoryID uint64, tags []string) (Selection, error)
	ApplyVideo(ctx context.Context, videoID uint64, selection Selection) error
	ForVideos(ctx context.Context, videoIDs []uint64) (map[uint64]VideoTaxonomy, error)
	NormalizeTags(tags []string) ([]NormalizedTag, error)
}

type Service struct {
	repo  Repository
	cache *cache.JSONCache
}

func NewService(repo Repository, categoryCache *cache.JSONCache) *Service {
	return &Service{repo: repo, cache: categoryCache}
}

func (s *Service) ListCategories(ctx context.Context) (*CategoryListResponse, error) {
	var items []Category
	loader := func(loadCtx context.Context) (any, error) { return s.repo.ListEnabledCategories(loadCtx) }
	if s.cache != nil {
		if err := s.cache.GetOrLoad(ctx, &items, loader); err != nil {
			return nil, err
		}
	} else {
		var err error
		items, err = s.repo.ListEnabledCategories(ctx)
		if err != nil {
			return nil, err
		}
	}
	return &CategoryListResponse{Items: items}, nil
}

func (s *Service) InvalidateCategories(ctx context.Context) error {
	if s == nil || s.cache == nil {
		return nil
	}
	return s.cache.Delete(ctx)
}

func (s *Service) ValidateSelection(ctx context.Context, categoryID uint64, tags []string) (Selection, error) {
	if s == nil || s.repo == nil {
		return Selection{}, errors.New("taxonomy service unavailable")
	}
	if categoryID == 0 {
		categoryID = UncategorizedID
	}
	if _, err := s.repo.FindEnabledCategory(ctx, categoryID); err != nil {
		return Selection{}, err
	}
	normalized, err := s.NormalizeTags(tags)
	if err != nil {
		return Selection{}, err
	}
	return Selection{CategoryID: categoryID, Tags: normalized}, nil
}

func (s *Service) ApplyVideo(ctx context.Context, videoID uint64, selection Selection) error {
	if s == nil || s.repo == nil {
		return errors.New("taxonomy service unavailable")
	}
	return s.repo.ApplyVideo(ctx, videoID, selection.CategoryID, selection.Tags)
}

func (s *Service) ForVideos(ctx context.Context, videoIDs []uint64) (map[uint64]VideoTaxonomy, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("taxonomy service unavailable")
	}
	return s.repo.ForVideos(ctx, videoIDs)
}

func (s *Service) NormalizeTags(tags []string) ([]NormalizedTag, error) {
	if len(tags) > maxTags {
		return nil, ErrTooManyTags
	}
	result := make([]NormalizedTag, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, raw := range tags {
		display := norm.NFC.String(strings.TrimSpace(raw))
		if display == "" {
			return nil, ErrTagInvalid
		}
		normalized := strings.ToLower(display)
		if utf8.RuneCountInString(normalized) < 1 || utf8.RuneCountInString(normalized) > maxTagRunes {
			return nil, ErrTagInvalid
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, NormalizedTag{Normalized: normalized, Display: display})
	}
	return result, nil
}
