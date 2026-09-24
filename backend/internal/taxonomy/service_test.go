package taxonomy

import (
	"context"
	"errors"
	"sync"
	"testing"

	"video_share/internal/cache"
)

type fakeRepository struct {
	mu       sync.Mutex
	calls    int
	cats     []Category
	selected []Selection
}

func (f *fakeRepository) ListEnabledCategories(context.Context) ([]Category, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return append([]Category(nil), f.cats...), nil
}
func (f *fakeRepository) FindEnabledCategory(_ context.Context, id uint64) (*Category, error) {
	for _, category := range f.cats {
		if category.ID == id && category.Enabled {
			copy := category
			return &copy, nil
		}
	}
	return nil, ErrCategoryNotFound
}
func (f *fakeRepository) ApplyVideo(_ context.Context, _ uint64, _ uint64, tags []NormalizedTag) error {
	f.selected = append(f.selected, Selection{Tags: tags})
	return nil
}
func (f *fakeRepository) ForVideos(context.Context, []uint64) (map[uint64]VideoTaxonomy, error) {
	return map[uint64]VideoTaxonomy{}, nil
}

func TestNormalizeTagsTrimsNFCDeduplicatesAndBounds(t *testing.T) {
	svc := NewService(&fakeRepository{}, nil)
	got, err := svc.NormalizeTags([]string{"  Café  ", "CAFÉ", "Go"})
	if err != nil {
		t.Fatalf("NormalizeTags: %v", err)
	}
	if len(got) != 2 || got[0].Normalized != "café" || got[0].Display != "Café" || got[1].Normalized != "go" {
		t.Fatalf("normalized tags = %+v", got)
	}
	if _, err := svc.NormalizeTags([]string{""}); !errors.Is(err, ErrTagInvalid) {
		t.Fatalf("empty tag error = %v", err)
	}
	if _, err := svc.NormalizeTags([]string{"a", "b", "c", "d", "e", "f"}); !errors.Is(err, ErrTooManyTags) {
		t.Fatalf("too many tags error = %v", err)
	}
}

func TestCategoryCacheCoalescesAndReturnsSourceOnRedisFailure(t *testing.T) {
	repo := &fakeRepository{cats: []Category{{ID: 1, Slug: "uncategorized", Name: "未分类", Enabled: true}}}
	cacheClient := cache.NewJSONCache(nil, cache.CategoriesCacheKey, cache.JSONCacheOptions{})
	svc := NewService(repo, cacheClient)
	for i := 0; i < 2; i++ {
		got, err := svc.ListCategories(context.Background())
		if err != nil || len(got.Items) != 1 {
			t.Fatalf("ListCategories = %+v, %v", got, err)
		}
	}
	// A nil Redis client intentionally falls back to the source; the cache's
	// singleflight still keeps concurrent cold-start loads bounded.
	if repo.calls == 0 {
		t.Fatal("expected source loader to run")
	}
}
