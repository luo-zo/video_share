package engagement

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

func TestLikeHandlerReturnsFinalStateAndRequiresUser(t *testing.T) {
	repo := &mockRepository{}
	repo.setLikeFn = func(_ context.Context, userID, videoID uint64, active bool) (*RelationState, error) {
		if userID != 7 || videoID != 9 {
			t.Fatalf("SetLike args = %d/%d", userID, videoID)
		}
		if active {
			return &RelationState{Active: true, Count: 3}, nil
		}
		return &RelationState{Active: false, Count: 2}, nil
	}
	h := newHandlerForTest(repo)
	r := gin.New()
	r.PUT("/videos/:id/like", withUserID(7), h.Like)
	r.DELETE("/videos/:id/like", withUserID(7), h.Unlike)
	r.PUT("/open/:id/like", h.Like)

	w := perform(r, http.MethodPut, "/videos/9/like")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"active":true`) || !strings.Contains(w.Body.String(), `"count":3`) {
		t.Fatalf("like = %d %s", w.Code, w.Body.String())
	}

	w = perform(r, http.MethodDelete, "/videos/9/like")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"active":false`) || !strings.Contains(w.Body.String(), `"count":2`) {
		t.Fatalf("unlike = %d %s", w.Code, w.Body.String())
	}

	w = perform(r, http.MethodPut, "/open/9/like")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated like = %d, want 401", w.Code)
	}

	w = perform(r, http.MethodPut, "/videos/nope/like")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid id = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

func TestFavoriteHandlerReturnsFinalStateAndRequiresUser(t *testing.T) {
	repo := &mockRepository{}
	repo.setFavoriteFn = func(_ context.Context, userID, videoID uint64, active bool) (*RelationState, error) {
		if userID != 7 || videoID != 9 {
			t.Fatalf("SetFavorite args = %d/%d", userID, videoID)
		}
		if active {
			return &RelationState{Active: true, Count: 1}, nil
		}
		return &RelationState{Active: false, Count: 0}, nil
	}
	h := newHandlerForTest(repo)
	r := gin.New()
	r.PUT("/videos/:id/favorite", withUserID(7), h.Favorite)
	r.DELETE("/videos/:id/favorite", withUserID(7), h.Unfavorite)
	r.DELETE("/open/:id/favorite", h.Unfavorite)

	w := perform(r, http.MethodPut, "/videos/9/favorite")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"active":true`) || !strings.Contains(w.Body.String(), `"count":1`) {
		t.Fatalf("favorite = %d %s", w.Code, w.Body.String())
	}

	w = perform(r, http.MethodDelete, "/videos/9/favorite")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"active":false`) || !strings.Contains(w.Body.String(), `"count":0`) {
		t.Fatalf("unfavorite = %d %s", w.Code, w.Body.String())
	}

	w = perform(r, http.MethodDelete, "/open/9/favorite")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated unfavorite = %d, want 401", w.Code)
	}
}

func TestRelationHandlersMapUnavailableVideoAndStorageFailure(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "unavailable video", err: ErrVideoNotFound, status: http.StatusNotFound, code: "NOT_FOUND"},
		{name: "storage failure", err: errors.New("connection reset by peer"), status: http.StatusInternalServerError, code: "INTERNAL_ERROR"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mockRepository{}
			repo.setLikeFn = func(context.Context, uint64, uint64, bool) (*RelationState, error) { return nil, tc.err }
			repo.setFavoriteFn = func(context.Context, uint64, uint64, bool) (*RelationState, error) { return nil, tc.err }
			h := newHandlerForTest(repo)
			r := gin.New()
			r.PUT("/videos/:id/like", withUserID(7), h.Like)
			r.PUT("/videos/:id/favorite", withUserID(7), h.Favorite)

			for _, path := range []string{"/videos/9/like", "/videos/9/favorite"} {
				w := perform(r, http.MethodPut, path)
				if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.code) {
					t.Fatalf("%s = %d %s, want %d %s", path, w.Code, w.Body.String(), tc.status, tc.code)
				}
			}
		})
	}
}
