package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"

	"video_share/internal/cache"
)

func newRateLimitTestRedis(t *testing.T) (*cache.Client, *miniredis.Miniredis) {
	t.Helper()
	srv := miniredis.RunT(t)
	client := cache.NewClient(cache.Options{Addr: srv.Addr(), CommandTimeout: 100 * time.Millisecond})
	t.Cleanup(func() { _ = client.Close() })
	return client, srv
}

func TestRedisRateLimiterUsesSharedAtomicWindow(t *testing.T) {
	client, _ := newRateLimitTestRedis(t)
	limiter := NewRedisRateLimiter(client, RateRule{Name: "login", Limit: 2, Window: time.Minute}, nil, nil)

	for i := 0; i < 2; i++ {
		decision, err := limiter.Allow(context.Background(), "ip:127.0.0.1")
		if err != nil || !decision.Allowed {
			t.Fatalf("request %d decision = %+v, err=%v", i+1, decision, err)
		}
	}
	decision, err := limiter.Allow(context.Background(), "ip:127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allowed || decision.RetryAfter <= 0 {
		t.Fatalf("third request decision = %+v, want denied with retry-after", decision)
	}
	other, err := limiter.Allow(context.Background(), "ip:127.0.0.2")
	if err != nil || !other.Allowed {
		t.Fatalf("different subject decision = %+v, err=%v", other, err)
	}
}

func TestRedisRateLimiterRejectsWhenRedisUnavailable(t *testing.T) {
	client, srv := newRateLimitTestRedis(t)
	srv.Close()
	limiter := NewRedisRateLimiter(client, RateRule{Name: "login", Limit: 1, Window: time.Minute, Unavailable: RejectUnavailable}, nil, nil)
	decision, err := limiter.Allow(context.Background(), "ip:127.0.0.1")
	if !errors.Is(err, ErrRateLimitUnavailable) {
		t.Fatalf("error = %v, want ErrRateLimitUnavailable", err)
	}
	if decision.Allowed {
		t.Fatal("unavailable Redis must not allow protected auth traffic")
	}
}

func TestRedisRateLimiterFallsBackToLocalLimiter(t *testing.T) {
	client, srv := newRateLimitTestRedis(t)
	srv.Close()
	local := NewRateLimiter(1, 1, 100)
	limiter := NewRedisRateLimiter(client, RateRule{Name: "comment", Limit: 1, Window: time.Minute, Unavailable: FallbackToLocal}, local, nil)
	first, err := limiter.Allow(context.Background(), "user:1")
	if err != nil || !first.Allowed || !first.Degraded {
		t.Fatalf("first fallback decision = %+v, err=%v", first, err)
	}
	second, err := limiter.Allow(context.Background(), "user:1")
	if err != nil || second.Allowed || !second.Degraded {
		t.Fatalf("second fallback decision = %+v, err=%v", second, err)
	}
	stats := limiter.Stats()
	if stats.RedisErrors != 2 || stats.Degraded != 2 || stats.Denied != 1 {
		t.Fatalf("unexpected fallback stats: %+v", stats)
	}
}

func TestRedisRateLimiterMiddlewareSetsRetryAfterAndUnavailableStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client, _ := newRateLimitTestRedis(t)
	limiter := NewRedisRateLimiter(client, RateRule{Name: "login", Limit: 1, Window: time.Minute}, nil, nil)
	r := gin.New()
	r.Use(limiter.Middleware(func(*gin.Context) string { return "ip:test" }))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		if i == 0 && w.Code != http.StatusNoContent {
			t.Fatalf("first status = %d", w.Code)
		}
		if i == 1 {
			if w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") == "" {
				t.Fatalf("second status=%d retry-after=%q", w.Code, w.Header().Get("Retry-After"))
			}
		}
	}

	client2, srv2 := newRateLimitTestRedis(t)
	srv2.Close()
	unavailable := NewRedisRateLimiter(client2, RateRule{Name: "login", Limit: 1, Window: time.Minute, Unavailable: RejectUnavailable}, nil, nil)
	r2 := gin.New()
	r2.Use(unavailable.Middleware(func(*gin.Context) string { return "ip:test" }))
	r2.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	w := httptest.NewRecorder()
	r2.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("unavailable status = %d, want 503", w.Code)
	}
}

func TestRedisRateLimiterDoesNotTrustForwardedIPWithoutTrustedProxy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client, _ := newRateLimitTestRedis(t)
	limiter := NewRedisRateLimiter(client, RateRule{Name: "ip", Limit: 1, Window: time.Minute}, nil, nil)
	r := gin.New()
	if err := r.SetTrustedProxies(nil); err != nil {
		t.Fatal(err)
	}
	r.Use(limiter.Middleware(nil))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	first := httptest.NewRequest(http.MethodGet, "/", nil)
	first.RemoteAddr = "192.0.2.10:1234"
	first.Header.Set("X-Forwarded-For", "198.51.100.1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, first)
	if w.Code != http.StatusNoContent {
		t.Fatalf("first status = %d", w.Code)
	}

	second := httptest.NewRequest(http.MethodGet, "/", nil)
	second.RemoteAddr = "192.0.2.10:1234"
	second.Header.Set("X-Forwarded-For", "198.51.100.2")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, second)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want 429 despite forged forwarded IP", w.Code)
	}
}
