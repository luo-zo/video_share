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
	"github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"

	"video_share/internal/middleware"
	"video_share/internal/token"
)

func newTestRouter(repo Repository, tm *token.Manager) *gin.Engine {
	gin.SetMode(gin.TestMode)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewHandler(NewService(repo, tm), log)
	r := gin.New()
	r.POST("/register", h.Register)
	r.POST("/login", h.Login)
	validate := func(ctx context.Context, id uint64) (bool, error) {
		u, err := repo.FindByID(ctx, id)
		if err != nil {
			return false, err
		}
		return u.Status == StatusNormal, nil
	}
	r.GET("/me", middleware.Auth(tm, validate), h.Me)
	return r
}

func doJSON(r *gin.Engine, method, path, body string, mutate func(*http.Request)) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if mutate != nil {
		mutate(req)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestRegisterHandler(t *testing.T) {
	repo := &mockRepo{}
	repo.createFn = func(ctx context.Context, u *User) error { u.ID = 1; return nil }
	tm := token.NewManager("test-secret", "video-share", time.Hour)
	r := newTestRouter(repo, tm)

	w := doJSON(r, http.MethodPost, "/register", `{"username":"Alice","password":"password123","nickname":"爱丽丝"}`, nil)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "password_hash") {
		t.Fatal("response leaked password_hash")
	}
	var resp struct {
		Data struct {
			ID       uint64 `json:"id"`
			Username string `json:"username"`
			Nickname string `json:"nickname"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data.Username != "alice" || resp.Data.Nickname != "爱丽丝" || resp.Data.ID != 1 {
		t.Fatalf("unexpected body: %+v", resp.Data)
	}
}

func TestRegisterHandlerInvalidBody(t *testing.T) {
	repo := &mockRepo{}
	tm := token.NewManager("test-secret", "video-share", time.Hour)
	r := newTestRouter(repo, tm)

	w := doJSON(r, http.MethodPost, "/register", `{"username":"alice"}`, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestRegisterHandlerDuplicate(t *testing.T) {
	repo := &mockRepo{}
	repo.createFn = func(ctx context.Context, u *User) error {
		return &mysql.MySQLError{Number: 1062, Message: "Duplicate entry"}
	}
	tm := token.NewManager("test-secret", "video-share", time.Hour)
	r := newTestRouter(repo, tm)

	w := doJSON(r, http.MethodPost, "/register", `{"username":"alice","password":"password123","nickname":"nick"}`, nil)
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", w.Code)
	}
	if !strings.Contains(w.Body.String(), "USER_ALREADY_EXISTS") {
		t.Fatalf("expected USER_ALREADY_EXISTS, got %s", w.Body.String())
	}
}

func TestLoginHandlerSuccess(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	repo := &mockRepo{}
	repo.findByUsernameFn = func(ctx context.Context, username string) (*User, error) {
		return &User{ID: 5, Username: "alice", PasswordHash: string(hash), Status: StatusNormal}, nil
	}
	tm := token.NewManager("test-secret", "video-share", time.Hour)
	r := newTestRouter(repo, tm)

	w := doJSON(r, http.MethodPost, "/login", `{"username":"alice","password":"password123"}`, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data LoginResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data.AccessToken == "" || resp.Data.TokenType != "Bearer" || resp.Data.ExpiresIn <= 0 {
		t.Fatalf("bad login response: %+v", resp.Data)
	}
}

func TestLoginHandlerWrongPassword(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	repo := &mockRepo{}
	repo.findByUsernameFn = func(ctx context.Context, username string) (*User, error) {
		return &User{ID: 5, Username: "alice", PasswordHash: string(hash), Status: StatusNormal}, nil
	}
	tm := token.NewManager("test-secret", "video-share", time.Hour)
	r := newTestRouter(repo, tm)

	w := doJSON(r, http.MethodPost, "/login", `{"username":"alice","password":"wrong"}`, nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
	if !strings.Contains(w.Body.String(), "用户名或密码错误") {
		t.Fatalf("expected generic message, got %s", w.Body.String())
	}
}

func TestMeHandler(t *testing.T) {
	repo := &mockRepo{}
	repo.findByIDFn = func(ctx context.Context, id uint64) (*User, error) {
		return &User{ID: id, Username: "alice", Nickname: "爱丽丝", Status: StatusNormal}, nil
	}
	tm := token.NewManager("test-secret", "video-share", time.Hour)
	tok, _, _ := tm.Generate(5)
	r := newTestRouter(repo, tm)

	w := doJSON(r, http.MethodGet, "/me", "", func(req *http.Request) {
		req.Header.Set("Authorization", "Bearer "+tok)
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "password_hash") {
		t.Fatal("response leaked password_hash")
	}
}

func TestMeHandlerDisabled(t *testing.T) {
	repo := &mockRepo{}
	repo.findByIDFn = func(ctx context.Context, id uint64) (*User, error) {
		return &User{ID: id, Status: StatusDisabled}, nil
	}
	tm := token.NewManager("test-secret", "video-share", time.Hour)
	tok, _, _ := tm.Generate(5)
	r := newTestRouter(repo, tm)

	w := doJSON(r, http.MethodGet, "/me", "", func(req *http.Request) {
		req.Header.Set("Authorization", "Bearer "+tok)
	})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestMeHandlerNoToken(t *testing.T) {
	repo := &mockRepo{}
	tm := token.NewManager("test-secret", "video-share", time.Hour)
	r := newTestRouter(repo, tm)

	w := doJSON(r, http.MethodGet, "/me", "", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}
