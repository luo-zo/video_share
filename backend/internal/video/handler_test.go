package video

import (
	"context"
	"encoding/json"
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

func newHandlerForTest(repo Repository, store ObjectStore) *Handler {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewHandler(newTestService(repo, store), log)
}

func withUserID(id uint64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(middleware.UserIDKey, id)
		c.Next()
	}
}

func performRequest(r http.Handler, method, path, body string) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestCreateHandlerSuccessAndInvalidJSON(t *testing.T) {
	repo := &mockRepository{createFn: func(_ context.Context, v *Video) error {
		v.ID = 9
		return nil
	}}
	h := newHandlerForTest(repo, &mockObjectStore{})
	r := gin.New()
	r.POST("/videos", withUserID(7), h.Create)

	w := performRequest(r, http.MethodPost, "/videos", `{"title":"demo","description":"","file_name":"demo.mp4","content_type":"video/mp4","file_size":10}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Data CreateResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Data.ID != 9 || body.Data.UploadURL == "" {
		t.Fatalf("response = %+v", body.Data)
	}

	w = performRequest(r, http.MethodPost, "/videos", `{"title":`)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "INVALID_PARAMETER") {
		t.Fatalf("invalid JSON response = %d %s", w.Code, w.Body.String())
	}
}

func TestCreateHandlerRequiresAuthenticatedUser(t *testing.T) {
	h := newHandlerForTest(&mockRepository{}, &mockObjectStore{})
	r := gin.New()
	r.POST("/videos", h.Create)
	w := performRequest(r, http.MethodPost, "/videos", `{"title":"demo","file_name":"demo.mp4","content_type":"video/mp4","file_size":10}`)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestCompleteHandlerAcceptsEmptyBody(t *testing.T) {
	repo := &mockRepository{findByIDFn: func(context.Context, uint64) (*Video, error) {
		return &Video{ID: 9, UserID: 7, ObjectKey: "videos/7/9/source.mp4", Status: StatusUploading, FileSize: 10, ContentType: "video/mp4"}, nil
	}}
	store := &mockObjectStore{statFn: func(context.Context, string) (int64, string, error) {
		return 10, "video/mp4", nil
	}}
	h := newHandlerForTest(repo, store)
	r := gin.New()
	r.POST("/videos/:id/complete", withUserID(7), h.Complete)

	w := performRequest(r, http.MethodPost, "/videos/9/complete", "")
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", w.Code, w.Body.String())
	}
}

func TestCompleteHandlerMapsBadIDForbiddenAndConflict(t *testing.T) {
	cases := []struct {
		name   string
		path   string
		video  *Video
		stat   func(context.Context, string) (int64, string, error)
		status int
	}{
		{name: "bad id", path: "/videos/nope/complete", status: http.StatusBadRequest},
		{name: "forbidden", path: "/videos/9/complete", video: &Video{ID: 9, UserID: 8, Status: StatusUploading}, status: http.StatusForbidden},
		{name: "mismatch", path: "/videos/9/complete", video: &Video{ID: 9, UserID: 7, Status: StatusUploading, FileSize: 10, ContentType: "video/mp4"}, stat: func(context.Context, string) (int64, string, error) { return 11, "video/mp4", nil }, status: http.StatusConflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mockRepository{}
			if tc.video != nil {
				repo.findByIDFn = func(context.Context, uint64) (*Video, error) { return tc.video, nil }
			}
			h := newHandlerForTest(repo, &mockObjectStore{statFn: tc.stat})
			r := gin.New()
			r.POST("/videos/:id/complete", withUserID(7), h.Complete)
			w := performRequest(r, http.MethodPost, tc.path, "")
			if w.Code != tc.status {
				t.Fatalf("status = %d, want %d; body=%s", w.Code, tc.status, w.Body.String())
			}
		})
	}
}

func TestListHandlerUsesDefaultsAndValidatesPagination(t *testing.T) {
	repo := &mockRepository{listReadyFn: func(_ context.Context, page, pageSize int) ([]Video, int64, error) {
		if page != 1 || pageSize != 12 {
			t.Fatalf("pagination = %d/%d, want 1/12", page, pageSize)
		}
		return []Video{}, 0, nil
	}}
	h := newHandlerForTest(repo, &mockObjectStore{})
	r := gin.New()
	r.GET("/videos", h.List)

	w := performRequest(r, http.MethodGet, "/videos", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"items":[]`) {
		t.Fatalf("response = %d %s", w.Code, w.Body.String())
	}

	for _, query := range []string{"?page=0", "?page=x", "?page_size=0", "?page_size=51"} {
		w = performRequest(r, http.MethodGet, "/videos"+query, "")
		if w.Code != http.StatusBadRequest {
			t.Fatalf("query %q status = %d, want 400", query, w.Code)
		}
	}
}

func TestDetailHandlerReturnsAuthorAndPlayURL(t *testing.T) {
	repo := &mockRepository{findReadyByIDFn: func(context.Context, uint64) (*Video, error) {
		return &Video{
			ID: 9, UserID: 7, ObjectKey: "videos/7/9/source.mp4", Status: StatusReady,
			Author: Author{ID: 7, Username: "alice", Nickname: "爱丽丝"},
		}, nil
	}}
	h := newHandlerForTest(repo, &mockObjectStore{})
	r := gin.New()
	r.GET("/videos/:id", h.Detail)

	w := performRequest(r, http.MethodGet, "/videos/9", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"play_url"`) || !strings.Contains(w.Body.String(), `"username":"alice"`) {
		t.Fatalf("response = %d %s", w.Code, w.Body.String())
	}
}

func TestDetailHandlerHidesNonReadyOrMissingVideo(t *testing.T) {
	repo := &mockRepository{findReadyByIDFn: func(context.Context, uint64) (*Video, error) {
		return nil, ErrNotFound
	}}
	h := newHandlerForTest(repo, &mockObjectStore{})
	r := gin.New()
	r.GET("/videos/:id", h.Detail)

	w := performRequest(r, http.MethodGet, "/videos/9", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestMineHandlerRequiresUserAndListsAllNonDeletedStates(t *testing.T) {
	repo := &mockRepository{listByUserFn: func(_ context.Context, userID uint64, page, pageSize int) ([]Video, int64, error) {
		return []Video{{ID: 1, UserID: userID, Status: StatusUploading}, {ID: 2, UserID: userID, Status: StatusFailed}}, 2, nil
	}}
	h := newHandlerForTest(repo, &mockObjectStore{})
	r := gin.New()
	r.GET("/mine", withUserID(7), h.Mine)
	r.GET("/mine-no-user", h.Mine)

	w := performRequest(r, http.MethodGet, "/mine", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"total":2`) {
		t.Fatalf("response = %d %s", w.Code, w.Body.String())
	}
	w = performRequest(r, http.MethodGet, "/mine-no-user", "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("missing user status = %d, want 401", w.Code)
	}
}

func TestHandlerMapsUnexpectedRepositoryErrorTo500(t *testing.T) {
	repo := &mockRepository{listReadyFn: func(context.Context, int, int) ([]Video, int64, error) {
		return nil, 0, errors.New("database unavailable")
	}}
	h := newHandlerForTest(repo, &mockObjectStore{})
	r := gin.New()
	r.GET("/videos", h.List)

	w := performRequest(r, http.MethodGet, "/videos", "")
	if w.Code != http.StatusInternalServerError || !strings.Contains(w.Body.String(), "INTERNAL_ERROR") {
		t.Fatalf("response = %d %s", w.Code, w.Body.String())
	}
}

func TestListHandlerAcceptsSearchAndRejectsInvalidSort(t *testing.T) {
	repo := &mockRepository{listPublicFn: func(_ context.Context, query ListQuery) ([]Video, int64, error) {
		if query.Query != "猫" || query.Sort != SortPopular || query.Page != 2 || query.PageSize != 6 {
			t.Fatalf("query = %+v", query)
		}
		return []Video{}, 0, nil
	}}
	h := newHandlerForTest(repo, &mockObjectStore{})
	r := gin.New()
	r.GET("/videos", h.List)
	w := performRequest(r, http.MethodGet, "/videos?q=%E7%8C%AB&sort=popular&page=2&page_size=6", "")
	if w.Code != http.StatusOK {
		t.Fatalf("valid search status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	for _, query := range []string{"?q=" + strings.Repeat("a", 51), "?sort=unknown"} {
		w = performRequest(r, http.MethodGet, "/videos"+query, "")
		if w.Code != http.StatusBadRequest {
			t.Fatalf("query %q status = %d, want 400", query, w.Code)
		}
	}
}
