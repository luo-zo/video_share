package cache

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

type testCategory struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

func newTestRedis(t *testing.T) (*Client, *miniredis.Miniredis) {
	t.Helper()
	srv := miniredis.RunT(t)
	client := NewClient(Options{
		Addr:           srv.Addr(),
		CommandTimeout: 100 * time.Millisecond,
		DialTimeout:    time.Second,
		ReadTimeout:    time.Second,
		WriteTimeout:   time.Second,
	})
	t.Cleanup(func() { _ = client.Close() })
	return client, srv
}

func TestJSONCacheLoadsOnceAndReturnsCachedValue(t *testing.T) {
	client, srv := newTestRedis(t)
	cache := NewJSONCache(client, CategoriesCacheKey, JSONCacheOptions{
		BaseTTL: 5 * time.Minute,
		Jitter:  func() time.Duration { return 17 * time.Second },
	})

	var loads atomic.Int32
	loader := func(context.Context) (any, error) {
		loads.Add(1)
		return []testCategory{{Slug: "tech", Name: "Technology"}}, nil
	}
	for i := 0; i < 2; i++ {
		var got []testCategory
		if err := cache.GetOrLoad(context.Background(), &got, loader); err != nil {
			t.Fatalf("GetOrLoad: %v", err)
		}
		if len(got) != 1 || got[0].Slug != "tech" {
			t.Fatalf("unexpected value: %+v", got)
		}
	}
	if got := loads.Load(); got != 1 {
		t.Fatalf("loader calls = %d, want 1", got)
	}
	if !srv.Exists(CategoriesCacheKey) {
		t.Fatal("expected category cache key to be written")
	}
	if ttl := srv.TTL(CategoriesCacheKey); ttl < 5*time.Minute || ttl > 5*time.Minute+17*time.Second {
		t.Fatalf("cache TTL = %s, want within configured range", ttl)
	}
	stats := cache.Stats()
	if stats.Misses != 1 || stats.Hits != 1 || stats.LoaderCalls != 1 {
		t.Fatalf("unexpected cache stats: %+v", stats)
	}
}

func TestNewCategoriesCacheUsesTheT02Contract(t *testing.T) {
	client, srv := newTestRedis(t)
	cache := NewCategoriesCache(client)
	if cache.key != CategoriesCacheKey {
		t.Fatalf("cache key = %q, want %q", cache.key, CategoriesCacheKey)
	}
	if err := cache.Set(context.Background(), []testCategory{{Slug: "contract", Name: "Contract"}}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	ttl := srv.TTL(CategoriesCacheKey)
	if ttl < 5*time.Minute || ttl > 6*time.Minute {
		t.Fatalf("cache TTL = %s, want 5-6 minutes", ttl)
	}
}

func TestJSONCacheCoalescesConcurrentLoads(t *testing.T) {
	client, _ := newTestRedis(t)
	cache := NewJSONCache(client, CategoriesCacheKey, JSONCacheOptions{BaseTTL: time.Minute})

	var loads atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	loader := func(context.Context) (any, error) {
		if loads.Add(1) == 1 {
			close(started)
			<-release
		}
		return []testCategory{{Slug: "life", Name: "Life"}}, nil
	}

	const callers = 8
	ready := make(chan struct{}, callers)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ready <- struct{}{}
		var got []testCategory
		if err := cache.GetOrLoad(context.Background(), &got, loader); err != nil {
			t.Errorf("GetOrLoad: %v", err)
		}
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("loader did not start")
	}
	for i := 1; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ready <- struct{}{}
			var got []testCategory
			if err := cache.GetOrLoad(context.Background(), &got, loader); err != nil {
				t.Errorf("GetOrLoad: %v", err)
			}
		}()
	}
	for i := 0; i < callers; i++ {
		<-ready
	}
	// The ready channel is sent immediately before GetOrLoad. Give every
	// goroutine a scheduling turn to enter singleflight while the first loader
	// is still blocked; otherwise a fast release can let a late caller start a
	// second flight after the first result has been published.
	time.Sleep(25 * time.Millisecond)
	close(release)
	wg.Wait()
	if got := loads.Load(); got != 1 {
		t.Fatalf("loader calls = %d, want 1", got)
	}
}

func TestJSONCacheSharedLoaderSurvivesFirstCallerCancellation(t *testing.T) {
	client, _ := newTestRedis(t)
	cache := NewJSONCache(client, CategoriesCacheKey, JSONCacheOptions{
		BaseTTL:       time.Minute,
		LoaderTimeout: time.Second,
	})

	started := make(chan struct{})
	release := make(chan struct{})
	var loads atomic.Int32
	loader := func(ctx context.Context) (any, error) {
		loads.Add(1)
		close(started)
		select {
		case <-release:
			return []testCategory{{Slug: "survived", Name: "Survived"}}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	firstCtx, cancelFirst := context.WithCancel(context.Background())
	firstDone := make(chan error, 1)
	go func() {
		var got []testCategory
		firstDone <- cache.GetOrLoad(firstCtx, &got, loader)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("loader did not start")
	}
	cancelFirst()
	if err := <-firstDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("first caller error = %v, want context canceled", err)
	}

	secondDone := make(chan error, 1)
	var got []testCategory
	go func() { secondDone <- cache.GetOrLoad(context.Background(), &got, loader) }()
	close(release)
	select {
	case err := <-secondDone:
		if err != nil {
			t.Fatalf("second caller error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("second caller did not receive shared result")
	}
	if len(got) != 1 || got[0].Slug != "survived" {
		t.Fatalf("unexpected value: %+v", got)
	}
	if loads.Load() != 1 {
		t.Fatalf("loader calls = %d, want 1", loads.Load())
	}
}

func TestJSONCacheFallsBackToLoaderWhenRedisUnavailable(t *testing.T) {
	client, srv := newTestRedis(t)
	cache := NewJSONCache(client, CategoriesCacheKey, JSONCacheOptions{BaseTTL: time.Minute})
	srv.Close()

	var got []testCategory
	err := cache.GetOrLoad(context.Background(), &got, func(context.Context) (any, error) {
		return []testCategory{{Slug: "fallback", Name: "Fallback"}}, nil
	})
	if err != nil {
		t.Fatalf("GetOrLoad fallback: %v", err)
	}
	if len(got) != 1 || got[0].Slug != "fallback" {
		t.Fatalf("unexpected fallback value: %+v", got)
	}
	if cache.Stats().RedisErrors == 0 {
		t.Fatal("expected Redis error to be counted")
	}
}

func TestJSONCacheLoaderErrorIsReturned(t *testing.T) {
	client, _ := newTestRedis(t)
	cache := NewJSONCache(client, CategoriesCacheKey, JSONCacheOptions{BaseTTL: time.Minute})
	want := errors.New("database unavailable")
	var got []testCategory
	if err := cache.GetOrLoad(context.Background(), &got, func(context.Context) (any, error) {
		return nil, want
	}); !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}

func TestJSONCacheDeleteInvalidatesKey(t *testing.T) {
	client, srv := newTestRedis(t)
	cache := NewJSONCache(client, CategoriesCacheKey, JSONCacheOptions{BaseTTL: time.Minute})
	if err := cache.Set(context.Background(), []testCategory{{Slug: "news", Name: "News"}}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if !srv.Exists(CategoriesCacheKey) {
		t.Fatal("expected key after Set")
	}
	if err := cache.Delete(context.Background()); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if srv.Exists(CategoriesCacheKey) {
		t.Fatal("expected key to be deleted")
	}
}
