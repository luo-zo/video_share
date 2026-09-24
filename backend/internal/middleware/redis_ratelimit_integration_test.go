//go:build integration

package middleware

import (
	"context"
	"strconv"
	"testing"
	"time"

	"video_share/internal/cache"
	"video_share/internal/testutil"
)

func TestRedisRateLimiterSharesQuotaAcrossClients(t *testing.T) {
	config := testutil.RedisConfig(t)
	options := cache.Options{
		Addr:           config.Addr,
		Password:       config.Password,
		DB:             config.DB,
		CommandTimeout: 2 * time.Second,
		DialTimeout:    2 * time.Second,
		ReadTimeout:    2 * time.Second,
		WriteTimeout:   2 * time.Second,
	}
	first := cache.NewClient(options)
	second := cache.NewClient(options)
	defer first.Close()
	defer second.Close()

	rule := RateRule{Name: "integration-shared", Limit: 2, Window: time.Minute}
	firstLimiter := NewRedisRateLimiter(first, rule, nil, nil)
	secondLimiter := NewRedisRateLimiter(second, rule, nil, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	subject := "same-subject-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	for _, limiter := range []*RedisRateLimiter{firstLimiter, secondLimiter} {
		decision, err := limiter.Allow(ctx, subject)
		if err != nil || !decision.Allowed {
			t.Fatalf("shared request decision = %+v, err=%v", decision, err)
		}
	}
	decision, err := firstLimiter.Allow(ctx, subject)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allowed || decision.RetryAfter <= 0 {
		t.Fatalf("third shared request decision = %+v, want denied", decision)
	}
}
