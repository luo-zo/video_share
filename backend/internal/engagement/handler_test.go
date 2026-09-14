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
	"video_share/internal/video"
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

func performJSON(r http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
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

func TestCommentHandlersCreateListAndDelete(t *testing.T) {
	repo := &mockRepository{}
	repo.createCommentFn = func(_ context.Context, userID, videoID uint64, content string) (*Comment, error) {
		if userID != 7 || videoID != 9 || content != "你好" {
			t.Fatalf("CreateComment args = %d/%d/%q", userID, videoID, content)
		}
		return &Comment{
			ID: 3, VideoID: 9, UserID: 7, Content: content,
			Author: video.Author{ID: 7, Username: "bob", Nickname: "鲍勃"},
		}, nil
	}
	repo.listCommentsFn = func(_ context.Context, videoID uint64, page, pageSize int) ([]Comment, int64, error) {
		if videoID != 9 || page != 1 || pageSize != 12 {
			t.Fatalf("ListComments args = %d/%d/%d", videoID, page, pageSize)
		}
		return []Comment{{
			ID: 3, VideoID: 9, UserID: 7, Content: "你好",
			Author: video.Author{ID: 7, Username: "bob", Nickname: "鲍勃"},
		}}, 1, nil
	}
	repo.deleteCommentFn = func(_ context.Context, userID, commentID uint64) error {
		if userID != 7 || commentID != 3 {
			t.Fatalf("DeleteComment args = %d/%d", userID, commentID)
		}
		return nil
	}
	h := newHandlerForTest(repo)
	r := gin.New()
	r.POST("/videos/:id/comments", withUserID(7), h.CreateComment)
	r.GET("/videos/:id/comments", h.ListComments)
	r.DELETE("/comments/:id", withUserID(7), h.DeleteComment)

	w := performJSON(r, http.MethodPost, "/videos/9/comments", `{"content":"  你好  "}`)
	if w.Code != http.StatusCreated || !strings.Contains(w.Body.String(), `"id":3`) ||
		!strings.Contains(w.Body.String(), `"nickname":"鲍勃"`) || !strings.Contains(w.Body.String(), `"content":"你好"`) {
		t.Fatalf("create comment = %d %s", w.Code, w.Body.String())
	}

	w = perform(r, http.MethodGet, "/videos/9/comments")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"total":1`) || !strings.Contains(w.Body.String(), `"username":"bob"`) {
		t.Fatalf("list comments = %d %s", w.Code, w.Body.String())
	}

	w = perform(r, http.MethodDelete, "/comments/3")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"id":3`) {
		t.Fatalf("delete comment = %d %s", w.Code, w.Body.String())
	}
}

func TestCommentHandlersRejectBadInputAndMapErrors(t *testing.T) {
	repo := &mockRepository{}
	repo.deleteCommentFn = func(context.Context, uint64, uint64) error { return ErrCommentForbidden }
	h := newHandlerForTest(repo)
	r := gin.New()
	r.POST("/videos/:id/comments", withUserID(7), h.CreateComment)
	r.POST("/open/:id/comments", h.CreateComment)
	r.GET("/videos/:id/comments", h.ListComments)
	r.DELETE("/comments/:id", withUserID(7), h.DeleteComment)

	w := performJSON(r, http.MethodPost, "/videos/9/comments", `{"content":`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("malformed body = %d, want 400", w.Code)
	}
	w = performJSON(r, http.MethodPost, "/videos/9/comments", `{"content":"   "}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("blank content = %d, want 400", w.Code)
	}
	w = performJSON(r, http.MethodPost, "/open/9/comments", `{"content":"你好"}`)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated comment = %d, want 401", w.Code)
	}
	w = performJSON(r, http.MethodPost, "/videos/nope/comments", `{"content":"你好"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid video id = %d, want 400", w.Code)
	}
	w = perform(r, http.MethodGet, "/videos/9/comments?page=0")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid page = %d, want 400", w.Code)
	}
	w = perform(r, http.MethodDelete, "/comments/3")
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), `"code":"COMMENT_FORBIDDEN"`) {
		t.Fatalf("foreign comment delete = %d %s", w.Code, w.Body.String())
	}
}

func TestWatchHandlerReportsProgress(t *testing.T) {
	repo := &mockRepository{}
	repo.recordWatchFn = func(_ context.Context, userID, videoID, progressMS, durationMS uint64) error {
		if userID != 7 || videoID != 9 || progressMS != 3 || durationMS != 10 {
			t.Fatalf("RecordWatch args = %d/%d/%d/%d", userID, videoID, progressMS, durationMS)
		}
		return nil
	}
	h := newHandlerForTest(repo)
	r := gin.New()
	r.POST("/videos/:id/watch", withUserID(7), h.RecordWatch)
	r.POST("/open/:id/watch", h.RecordWatch)

	w := performJSON(r, http.MethodPost, "/videos/9/watch", `{"progress_ms":3,"duration_ms":10}`)
	if w.Code != http.StatusOK {
		t.Fatalf("watch = %d %s", w.Code, w.Body.String())
	}

	repo.recordWatchFn = func(context.Context, uint64, uint64, uint64, uint64) error { return ErrProgressInvalid }
	w = performJSON(r, http.MethodPost, "/videos/9/watch", `{"progress_ms":11,"duration_ms":10}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid progress = %d, want 400", w.Code)
	}

	w = performJSON(r, http.MethodPost, "/open/9/watch", `{"progress_ms":3,"duration_ms":10}`)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated watch = %d, want 401", w.Code)
	}
}

func TestPersonalListHandlersRequireUserAndMapVideos(t *testing.T) {
	repo := &mockRepository{}
	repo.listFavoritesFn = func(_ context.Context, userID uint64, page, pageSize int) ([]video.Video, int64, error) {
		return []video.Video{{
			ID: 4, UserID: 8, Title: "收藏的视频",
			Author: video.Author{ID: 8, Username: "bob", Nickname: "鲍勃"},
			Stats:  video.Stats{LikeCount: 2},
		}}, 1, nil
	}
	repo.listHistoryFn = func(_ context.Context, userID uint64, page, pageSize int) ([]video.Video, int64, error) {
		return []video.Video{{
			ID: 5, UserID: 8, Title: "看过的视频",
			Author: video.Author{ID: 8, Username: "bob", Nickname: "鲍勃"},
			Stats:  video.Stats{ViewCount: 9},
		}}, 1, nil
	}
	h := newHandlerForTest(repo)
	r := gin.New()
	r.GET("/favorites", withUserID(7), h.Favorites)
	r.GET("/history", withUserID(7), h.History)
	r.GET("/open-favorites", h.Favorites)

	w := perform(r, http.MethodGet, "/favorites")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "收藏的视频") || !strings.Contains(w.Body.String(), `"like_count":2`) {
		t.Fatalf("favorites = %d %s", w.Code, w.Body.String())
	}

	w = perform(r, http.MethodGet, "/history")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "看过的视频") || !strings.Contains(w.Body.String(), `"view_count":9`) {
		t.Fatalf("history = %d %s", w.Code, w.Body.String())
	}

	w = perform(r, http.MethodGet, "/open-favorites")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated favorites = %d, want 401", w.Code)
	}

	w = perform(r, http.MethodGet, "/favorites?page_size=51")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("oversized page_size = %d, want 400", w.Code)
	}
}
