package follow

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"video_share/internal/middleware"
)

func newHandlerForTest(repo Repository) *Handler {
	return NewHandler(NewService(repo), slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func withUserID(id uint64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(middleware.UserIDKey, id)
		c.Next()
	}
}

func perform(r http.Handler, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestFollowHandlerReturnsFinalStateAndRequiresUser(t *testing.T) {
	repo := &mockRepository{}
	repo.setFollowFn = func(_ context.Context, followerID, followeeID uint64, active bool) (*FollowState, error) {
		if followerID != 7 || followeeID != 9 {
			t.Fatalf("SetFollow args = %d/%d", followerID, followeeID)
		}
		return &FollowState{Following: active}, nil
	}
	h := newHandlerForTest(repo)
	r := gin.New()
	r.PUT("/users/:id/follow", withUserID(7), h.Follow)
	r.DELETE("/users/:id/follow", withUserID(7), h.Unfollow)
	r.PUT("/open/:id/follow", h.Follow)

	w := perform(r, http.MethodPut, "/users/9/follow")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"following":true`) {
		t.Fatalf("follow = %d %s", w.Code, w.Body.String())
	}

	w = perform(r, http.MethodDelete, "/users/9/follow")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"following":false`) {
		t.Fatalf("unfollow = %d %s", w.Code, w.Body.String())
	}

	w = perform(r, http.MethodPut, "/open/9/follow")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated follow = %d, want 401", w.Code)
	}

	w = perform(r, http.MethodPut, "/users/nope/follow")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid id = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

func TestFollowHandlerMapsValidationAndStorageErrors(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"self follow", ErrSelfFollow, http.StatusBadRequest, "INVALID_PARAMETER"},
		{"missing target", ErrUserNotFound, http.StatusNotFound, "NOT_FOUND"},
		{"storage failure", errors.New("boom"), http.StatusInternalServerError, "INTERNAL_ERROR"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mockRepository{setFollowFn: func(context.Context, uint64, uint64, bool) (*FollowState, error) {
				return nil, tc.err
			}}
			h := newHandlerForTest(repo)
			r := gin.New()
			r.PUT("/users/:id/follow", withUserID(7), h.Follow)

			w := perform(r, http.MethodPut, "/users/9/follow")
			if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.code) {
				t.Fatalf("status=%d body=%s, want %d %s", w.Code, w.Body.String(), tc.status, tc.code)
			}
		})
	}
}

func TestListFollowsHandlerRequiresUserAndMapsUsers(t *testing.T) {
	repo := &mockRepository{listFollowsFn: func(_ context.Context, followerID uint64, page, pageSize int) ([]FollowedUser, int64, error) {
		if followerID != 7 || page != 1 || pageSize != 12 {
			t.Fatalf("repository args = %d/%d/%d", followerID, page, pageSize)
		}
		return []FollowedUser{{ID: 11, Username: "bob", Nickname: "Bob"}}, 1, nil
	}}
	h := newHandlerForTest(repo)
	r := gin.New()
	r.GET("/users/me/follows", withUserID(7), h.ListFollows)
	r.GET("/open/follows", h.ListFollows)

	w := perform(r, http.MethodGet, "/users/me/follows")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"username":"bob"`) || !strings.Contains(w.Body.String(), `"total":1`) {
		t.Fatalf("list follows = %d %s", w.Code, w.Body.String())
	}

	w = perform(r, http.MethodGet, "/open/follows")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated list = %d, want 401", w.Code)
	}

	w = perform(r, http.MethodGet, "/users/me/follows?page_size=99")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid page_size = %d, want 400", w.Code)
	}
}
