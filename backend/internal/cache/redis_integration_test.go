//go:build integration

package cache

import (
	"context"
	"fmt"
	"testing"
	"time"

	"video_share/internal/testutil"
)

func TestRedisIntegrationPingAndCategoryCache(t *testing.T) {
	config := testutil.RedisConfig(t)
	client := NewClient(Options{
		Addr:           config.Addr,
		Password:       config.Password,
		DB:             config.DB,
		CommandTimeout: 2 * time.Second,
		DialTimeout:    2 * time.Second,
		ReadTimeout:    2 * time.Second,
		WriteTimeout:   2 * time.Second,
	})
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx); err != nil {
		t.Fatal(err)
	}

	// Unit tests lock the production key contract. Integration tests use a
	// unique namespace so they cannot delete another run's data in the shared
	// dedicated Redis database.
	cache := NewJSONCache(client, fmt.Sprintf("%s:integration:%d", CategoriesCacheKey, time.Now().UnixNano()), JSONCacheOptions{
		BaseTTL:   time.Minute,
		MaxJitter: time.Second,
	})
	var got []testCategory
	if err := cache.GetOrLoad(ctx, &got, func(context.Context) (any, error) {
		return []testCategory{{Slug: "integration", Name: "Integration"}}, nil
	}); err != nil {
		t.Fatalf("GetOrLoad: %v", err)
	}
	if len(got) != 1 || got[0].Slug != "integration" {
		t.Fatalf("unexpected value: %+v", got)
	}
	if err := cache.Delete(ctx); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}
