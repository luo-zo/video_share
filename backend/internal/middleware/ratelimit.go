package middleware

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"

	"video_share/internal/response"
)

// RateLimiter 是一个有界、进程内、按 key 的令牌桶限流器。map 的条目数被限制在
// max 以内，以保证内存有界。
type RateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	max      int
	r        rate.Limit
	b        int
}

func NewRateLimiter(perSecond float64, burst, max int) *RateLimiter {
	return &RateLimiter{
		limiters: make(map[string]*rate.Limiter),
		max:      max,
		r:        rate.Limit(perSecond),
		b:        burst,
	}
}

func (rl *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !rl.allow(c.ClientIP()) {
			response.Error(c, http.StatusTooManyRequests, response.CodeRateLimited, "too many requests")
			return
		}
		c.Next()
	}
}

func (rl *RateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	l, ok := rl.limiters[key]
	if !ok {
		if len(rl.limiters) >= rl.max {
			for k := range rl.limiters {
				delete(rl.limiters, k)
				break
			}
		}
		l = rate.NewLimiter(rl.r, rl.b)
		rl.limiters[key] = l
	}
	return l.Allow()
}
