package user

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"video_share/internal/middleware"
	"video_share/internal/token"
)

func newBodyLimitRouter(repo Repository, tm *token.Manager) *gin.Engine {
	gin.SetMode(gin.TestMode)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewHandler(NewService(repo, tm), log)
	r := gin.New()
	r.Use(middleware.RequestID())
	limit := middleware.LimitBody(32 * 1024)
	r.POST("/register", limit, h.Register)
	r.POST("/login", limit, h.Login)
	return r
}

func TestBodyLimitNormalRegister(t *testing.T) {
	repo := &mockRepo{}
	repo.createFn = func(ctx context.Context, u *User) error { u.ID = 1; return nil }
	tm := token.NewManager("test-secret", "video-share", time.Hour)
	r := newBodyLimitRouter(repo, tm)

	w := doJSON(r, http.MethodPost, "/register", `{"username":"Alice","password":"password123","nickname":"爱丽丝"}`, nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
}

func TestBodyLimitOversizedRegister(t *testing.T) {
	repo := &mockRepo{}
	tm := token.NewManager("test-secret", "video-share", time.Hour)
	r := newBodyLimitRouter(repo, tm)

	big := `{"username":"alice","password":"password123","nickname":"` + strings.Repeat("x", 40*1024) + `"}`
	w := doJSON(r, http.MethodPost, "/register", big, nil)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413; body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		Error struct {
			Code      string `json:"code"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error.Code != "REQUEST_TOO_LARGE" {
		t.Fatalf("code = %q, want REQUEST_TOO_LARGE", resp.Error.Code)
	}
	if resp.Error.RequestID == "" {
		t.Fatal("request_id missing from 413 response")
	}
}

func TestBodyLimitOversizedNoContentLength(t *testing.T) {
	repo := &mockRepo{}
	tm := token.NewManager("test-secret", "video-share", time.Hour)
	r := newBodyLimitRouter(repo, tm)

	big := `{"username":"alice","password":"password123","nickname":"` + strings.Repeat("x", 40*1024) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/register", unknownLengthReader{strings.NewReader(big)})
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413; body=%s", w.Code, w.Body.String())
	}
}

type unknownLengthReader struct{ r io.Reader }

func (u unknownLengthReader) Read(p []byte) (int, error) { return u.r.Read(p) }
