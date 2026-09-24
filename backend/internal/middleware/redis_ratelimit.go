package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"

	"video_share/internal/cache"
	"video_share/internal/response"
)

// UnavailablePolicy controls the deliberately different behavior of auth and
// ordinary interaction limits when Redis cannot be reached.
type UnavailablePolicy uint8

const (
	RejectUnavailable UnavailablePolicy = iota
	FallbackToLocal
)

// RateRule is a fixed-window rule. The Lua script makes increment, expiry and
// decision one atomic Redis operation, so two API instances share one quota.
type RateRule struct {
	Name        string
	Limit       int
	Window      time.Duration
	Unavailable UnavailablePolicy
}

type RateDecision struct {
	Allowed    bool
	RetryAfter time.Duration
	Degraded   bool
}

// RateStats are process-local diagnostics for Redis errors and local
// degradation. They must not be presented as a distributed quota counter.
type RateStats struct {
	RedisErrors uint64
	Degraded    uint64
	Denied      uint64
}

type rateCounters struct {
	redisErrors atomic.Uint64
	degraded    atomic.Uint64
	denied      atomic.Uint64
}

var ErrRateLimitUnavailable = errors.New("redis rate limiter unavailable")

const rateLimitScript = `
local current = redis.call('INCR', KEYS[1])
local ttl = redis.call('PTTL', KEYS[1])
if current == 1 or ttl < 0 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
  ttl = tonumber(ARGV[1])
end
local allowed = 0
if current <= tonumber(ARGV[2]) then
  allowed = 1
end
return {allowed, ttl}
`

// RedisRateLimiter is safe for concurrent requests. It intentionally does not
// make Redis a source of identity or business truth; it only protects an API
// operation for a finite window.
type RedisRateLimiter struct {
	client   *cache.Client
	rule     RateRule
	fallback *RateLimiter
	log      *slog.Logger
	counters rateCounters
}

func NewRedisRateLimiter(client *cache.Client, rule RateRule, fallback *RateLimiter, log *slog.Logger) *RedisRateLimiter {
	return &RedisRateLimiter{client: client, rule: rule, fallback: fallback, log: log}
}

// Allow applies the Redis rule, or its explicitly configured fallback policy.
func (l *RedisRateLimiter) Allow(ctx context.Context, subject string) (RateDecision, error) {
	if l == nil || l.rule.Limit <= 0 || l.rule.Window <= 0 {
		return RateDecision{}, ErrRateLimitUnavailable
	}
	key := rateLimitKey(l.rule.Name, subject)
	if l.client == nil {
		return l.onUnavailable(ctx, subject, errors.New("redis client is nil"))
	}
	result, err := l.client.Eval(ctx, rateLimitScript, []string{key}, l.rule.Window.Milliseconds(), l.rule.Limit)
	if err != nil {
		return l.onUnavailable(ctx, subject, err)
	}
	values, ok := result.([]interface{})
	if !ok || len(values) != 2 {
		return l.onUnavailable(ctx, subject, fmt.Errorf("unexpected rate limit response %T", result))
	}
	allowed, ok := redisInt64(values[0])
	if !ok {
		return l.onUnavailable(ctx, subject, errors.New("invalid rate limit decision"))
	}
	ttlMillis, ok := redisInt64(values[1])
	if !ok {
		return l.onUnavailable(ctx, subject, errors.New("invalid rate limit TTL"))
	}
	decision := RateDecision{Allowed: allowed == 1}
	if !decision.Allowed {
		l.counters.denied.Add(1)
		decision.RetryAfter = retryAfter(ttlMillis)
	}
	return decision, nil
}

func (l *RedisRateLimiter) onUnavailable(_ context.Context, subject string, cause error) (RateDecision, error) {
	l.counters.redisErrors.Add(1)
	if l.log != nil {
		l.log.Warn("redis rate limiter unavailable", "rule", l.rule.Name, "error", cause)
	}
	if l.rule.Unavailable == FallbackToLocal && l.fallback != nil {
		l.counters.degraded.Add(1)
		allowed := l.fallback.Allow(subject)
		decision := RateDecision{Allowed: allowed, Degraded: true}
		if !allowed {
			l.counters.denied.Add(1)
			decision.RetryAfter = time.Second
		}
		return decision, nil
	}
	return RateDecision{}, fmt.Errorf("%w: %v", ErrRateLimitUnavailable, cause)
}

// Stats returns process-local rate-limit diagnostics.
func (l *RedisRateLimiter) Stats() RateStats {
	if l == nil {
		return RateStats{}
	}
	return RateStats{
		RedisErrors: l.counters.redisErrors.Load(),
		Degraded:    l.counters.degraded.Load(),
		Denied:      l.counters.denied.Load(),
	}
}

// Middleware applies a subject selector. A nil selector uses Gin's trusted
// ClientIP calculation, which does not accept arbitrary forwarded headers.
func (l *RedisRateLimiter) Middleware(subject func(*gin.Context) string) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := ""
		if subject != nil {
			key = subject(c)
		}
		if strings.TrimSpace(key) == "" {
			key = c.ClientIP()
		}
		decision, err := l.Allow(c.Request.Context(), key)
		if err != nil {
			response.Error(c, http.StatusServiceUnavailable, response.CodeRateLimitUnavailable, "rate limiter unavailable")
			return
		}
		if !decision.Allowed {
			if decision.RetryAfter > 0 {
				c.Header("Retry-After", strconv.FormatInt(int64(math.Ceil(decision.RetryAfter.Seconds())), 10))
			}
			response.Error(c, http.StatusTooManyRequests, response.CodeRateLimited, "too many requests")
			return
		}
		c.Next()
	}
}

func rateLimitKey(name, subject string) string {
	digest := sha256.Sum256([]byte(subject))
	return "video_share:rate:" + name + ":" + hex.EncodeToString(digest[:])
}

func redisInt64(value any) (int64, bool) {
	switch v := value.(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case string:
		n, err := strconv.ParseInt(v, 10, 64)
		return n, err == nil
	case []byte:
		n, err := strconv.ParseInt(string(v), 10, 64)
		return n, err == nil
	default:
		return 0, false
	}
}

func retryAfter(ttlMillis int64) time.Duration {
	if ttlMillis <= 0 {
		return time.Second
	}
	seconds := (ttlMillis + 999) / 1000
	return time.Duration(seconds) * time.Second
}
