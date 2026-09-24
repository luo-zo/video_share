package cache

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"
)

// CategoriesCacheKey is reserved for the taxonomy cache wired by T05. T02
// provides the cache seam without creating the categories table.
const CategoriesCacheKey = "video_share:cache:categories:v1"

// NewCategoriesCache fixes the T02 taxonomy-cache contract. The actual data
// source remains injectable until T05 creates migration 000007_taxonomy.
func NewCategoriesCache(client *Client) *JSONCache {
	return NewJSONCache(client, CategoriesCacheKey, JSONCacheOptions{
		BaseTTL:   5 * time.Minute,
		MaxJitter: time.Minute,
	})
}

// JSONCacheOptions controls a JSON cache's expiration and bounded fallback
// loading. A loader is invoked only after a miss or Redis failure.
type JSONCacheOptions struct {
	BaseTTL            time.Duration
	Jitter             func() time.Duration
	MaxJitter          time.Duration
	MaxLoadConcurrency int
	LoaderTimeout      time.Duration
}

// CacheStats are process-local observability counters. They are diagnostic
// signals, not durable business metrics.
type CacheStats struct {
	Hits         uint64
	Misses       uint64
	LoaderCalls  uint64
	RedisErrors  uint64
	SetErrors    uint64
	DeleteErrors uint64
}

type cacheCounters struct {
	hits         atomic.Uint64
	misses       atomic.Uint64
	loaderCalls  atomic.Uint64
	redisErrors  atomic.Uint64
	setErrors    atomic.Uint64
	deleteErrors atomic.Uint64
}

// JSONCache stores arbitrary JSON-serializable values under one key. It is
// intentionally source-agnostic: T05 can inject the taxonomy repository when
// the 000007 migration exists.
type JSONCache struct {
	client        *Client
	key           string
	baseTTL       time.Duration
	jitter        func() time.Duration
	loadSlots     chan struct{}
	loaderTimeout time.Duration
	flight        singleflight.Group
	counters      cacheCounters
}

// NewJSONCache creates a cache with safe defaults for the T02 category-list
// boundary. A non-positive TTL or concurrency value uses a bounded default.
func NewJSONCache(client *Client, key string, options JSONCacheOptions) *JSONCache {
	if options.BaseTTL <= 0 {
		options.BaseTTL = 5 * time.Minute
	}
	if options.MaxLoadConcurrency <= 0 {
		options.MaxLoadConcurrency = 8
	}
	if options.LoaderTimeout <= 0 {
		options.LoaderTimeout = 5 * time.Second
	}
	if options.Jitter == nil {
		maxJitter := options.MaxJitter
		options.Jitter = func() time.Duration {
			if maxJitter <= 0 {
				return 0
			}
			return time.Duration(rand.Int63n(int64(maxJitter) + 1))
		}
	}
	return &JSONCache{
		client:        client,
		key:           key,
		baseTTL:       options.BaseTTL,
		jitter:        options.Jitter,
		loadSlots:     make(chan struct{}, options.MaxLoadConcurrency),
		loaderTimeout: options.LoaderTimeout,
	}
}

// GetOrLoad decodes a cached value into dst, or loads it from the injected
// source. Cache failures degrade to the source; source failures are returned.
// Concurrent misses for the same key share one bounded loader invocation.
func (c *JSONCache) GetOrLoad(ctx context.Context, dst any, loader func(context.Context) (any, error)) error {
	if c == nil {
		return errors.New("json cache is nil")
	}
	if loader == nil {
		return errors.New("json cache loader is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	if c.client != nil {
		raw, err := c.client.Get(ctx, c.key)
		switch {
		case err == nil:
			if decodeErr := json.Unmarshal([]byte(raw), dst); decodeErr == nil {
				c.counters.hits.Add(1)
				return nil
			}
			c.counters.redisErrors.Add(1)
		case IsMiss(err):
			c.counters.misses.Add(1)
		default:
			c.counters.redisErrors.Add(1)
			c.counters.misses.Add(1)
		}
	} else {
		c.counters.misses.Add(1)
	}

	resultCh := c.flight.DoChan(c.key, func() (any, error) {
		c.counters.loaderCalls.Add(1)
		// The shared loader must outlive the first caller. Otherwise one
		// canceled request can cancel every waiter coalesced on this key.
		loaderCtx, cancel := context.WithTimeout(context.Background(), c.loaderTimeout)
		defer cancel()
		select {
		case c.loadSlots <- struct{}{}:
			defer func() { <-c.loadSlots }()
		case <-loaderCtx.Done():
			return nil, loaderCtx.Err()
		}
		value, loadErr := loader(loaderCtx)
		if loadErr != nil {
			return nil, loadErr
		}
		raw, marshalErr := json.Marshal(value)
		if marshalErr != nil {
			return nil, marshalErr
		}
		if c.client != nil {
			ttl := c.baseTTL + c.jitter()
			if setErr := c.client.Set(loaderCtx, c.key, raw, ttl); setErr != nil {
				c.counters.setErrors.Add(1)
			}
		}
		return raw, nil
	})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case result := <-resultCh:
		if result.Err != nil {
			return result.Err
		}
		loaded := result.Val
		raw, ok := loaded.([]byte)
		if !ok {
			return errors.New("json cache loader returned invalid data")
		}
		return json.Unmarshal(raw, dst)
	}
}

// Set writes a value with the configured finite TTL. It is useful to write a
// freshly committed source value after a transaction without changing the
// cache's source-loading policy.
func (c *JSONCache) Set(ctx context.Context, value any) error {
	if c == nil || c.client == nil {
		return errors.New("json cache client is unavailable")
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	ttl := c.baseTTL + c.jitter()
	if err := c.client.Set(ctx, c.key, raw, ttl); err != nil {
		c.counters.setErrors.Add(1)
		return err
	}
	return nil
}

// Delete invalidates the key. MySQL callers should invoke it after committing
// a source change; a deletion failure must be logged and allowed to converge
// through the TTL rather than rolling back the source transaction.
func (c *JSONCache) Delete(ctx context.Context) error {
	if c == nil || c.client == nil {
		return nil
	}
	if err := c.client.Delete(ctx, c.key); err != nil {
		c.counters.deleteErrors.Add(1)
		return err
	}
	return nil
}

// Stats returns a point-in-time snapshot of cache diagnostics.
func (c *JSONCache) Stats() CacheStats {
	if c == nil {
		return CacheStats{}
	}
	return CacheStats{
		Hits:         c.counters.hits.Load(),
		Misses:       c.counters.misses.Load(),
		LoaderCalls:  c.counters.loaderCalls.Load(),
		RedisErrors:  c.counters.redisErrors.Load(),
		SetErrors:    c.counters.setErrors.Load(),
		DeleteErrors: c.counters.deleteErrors.Load(),
	}
}
