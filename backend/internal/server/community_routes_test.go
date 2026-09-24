package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"video_share/internal/config"
	"video_share/internal/token"
)

// newRouteTestRouter 装配真实的 NewRouter；不建立数据库连接，因此只能用于检查
// 路由表和在任何存储访问之前就会中止的未认证请求。
func newRouteTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	tm := token.NewManager("secret", "video-share", time.Hour)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewRouter(&config.Config{}, nil, log, tm, nil, nil)
}

func TestCommunityRoutesAreRegistered(t *testing.T) {
	r := newRouteTestRouter(t)

	registered := make(map[string]bool, len(r.Routes()))
	for _, route := range r.Routes() {
		registered[route.Method+" "+route.Path] = true
	}

	want := []string{
		"GET /api/v1/auth/csrf",
		"POST /api/v1/auth/login",
		"POST /api/v1/auth/refresh",
		"POST /api/v1/auth/logout",
		"GET /api/v1/users/:id",
		"GET /api/v1/users/:id/videos",
		"GET /api/v1/users/:id/followers",
		"GET /api/v1/users/:id/follows",
		"GET /api/v1/videos",
		"GET /api/v1/videos/ranking",
		"GET /api/v1/videos/:id",
		"GET /api/v1/videos/:id/comments",
		"POST /api/v1/videos/:id/comments",
		"DELETE /api/v1/comments/:id",
		"PUT /api/v1/videos/:id/like",
		"DELETE /api/v1/videos/:id/like",
		"PUT /api/v1/videos/:id/favorite",
		"DELETE /api/v1/videos/:id/favorite",
		"POST /api/v1/videos/:id/watch",
		"PATCH /api/v1/users/me/videos/:id",
		"DELETE /api/v1/users/me/videos/:id",
		"GET /api/v1/users/me/favorites",
		"GET /api/v1/users/me/history",
		"GET /api/v1/users/me/follows",
		"PUT /api/v1/users/:id/follow",
		"DELETE /api/v1/users/:id/follow",
	}
	for _, route := range want {
		if !registered[route] {
			t.Errorf("route %q is not registered", route)
		}
	}
	// Gin resolves in registration order; the fixed path must precede the
	// dynamic :id route so "ranking" can never be treated as a video id.
	paths := r.Routes()
	index := func(path string) int {
		for i, route := range paths {
			if route.Method == http.MethodGet && route.Path == path {
				return i
			}
		}
		return -1
	}
	if index("/api/v1/videos/ranking") >= index("/api/v1/videos/:id") {
		t.Fatalf("ranking route must precede dynamic video route: ranking=%d dynamic=%d", index("/api/v1/videos/ranking"), index("/api/v1/videos/:id"))
	}
}

// TestProtectedRoutesRequireAuthentication 对每个需要登录的端点发一次未携带令牌的
// 请求。强制鉴权必须在任何存储访问之前中止，因此缺少 Auth 的写路由会返回 500
// 或 200 而不是 401，从而暴露漏洞。
func TestProtectedRoutesRequireAuthentication(t *testing.T) {
	r := newRouteTestRouter(t)

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/users/me"},
		{http.MethodPatch, "/api/v1/users/me"},
		{http.MethodPost, "/api/v1/users/me/change-password"},
		{http.MethodGet, "/api/v1/users/me/videos"},
		{http.MethodGet, "/api/v1/users/me/videos/1"},
		{http.MethodGet, "/api/v1/users/me/favorites"},
		{http.MethodGet, "/api/v1/users/me/history"},
		{http.MethodGet, "/api/v1/users/me/follows"},
		{http.MethodPost, "/api/v1/videos"},
		{http.MethodPost, "/api/v1/videos/1/complete"},
		{http.MethodPatch, "/api/v1/users/me/videos/1"},
		{http.MethodDelete, "/api/v1/users/me/videos/1"},
		{http.MethodPut, "/api/v1/videos/1/like"},
		{http.MethodDelete, "/api/v1/videos/1/like"},
		{http.MethodPut, "/api/v1/videos/1/favorite"},
		{http.MethodDelete, "/api/v1/videos/1/favorite"},
		{http.MethodPost, "/api/v1/videos/1/watch"},
		{http.MethodPost, "/api/v1/videos/1/comments"},
		{http.MethodDelete, "/api/v1/comments/1"},
		{http.MethodPut, "/api/v1/users/1/follow"},
		{http.MethodDelete, "/api/v1/users/1/follow"},
	}
	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			req := httptest.NewRequest(route.method, route.path, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401 without a token", w.Code)
			}
		})
	}
}

func TestLoginRateLimitFailsClosedWhenRedisUnavailable(t *testing.T) {
	r := newRouteTestRouter(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 when Redis rate limiter is unavailable", w.Code)
	}
	if !strings.Contains(w.Body.String(), "RATE_LIMIT_UNAVAILABLE") {
		t.Fatalf("body = %s, want RATE_LIMIT_UNAVAILABLE", w.Body.String())
	}
}
