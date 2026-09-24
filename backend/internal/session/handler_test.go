package session

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"video_share/internal/middleware"
	"video_share/internal/token"
	"video_share/internal/user"
)

type handlerUserRepository struct {
	byUsername  *user.User
	byID        *user.User
	updated     *user.User
	updateErr   error
	passwordErr error
}

func (r *handlerUserRepository) Create(context.Context, *user.User) error { return nil }
func (r *handlerUserRepository) FindByUsername(context.Context, string) (*user.User, error) {
	if r.byUsername == nil {
		return nil, user.ErrNotFound
	}
	return r.byUsername, nil
}
func (r *handlerUserRepository) FindByID(context.Context, uint64) (*user.User, error) {
	if r.byID == nil {
		return nil, user.ErrNotFound
	}
	return r.byID, nil
}
func (r *handlerUserRepository) UpdateProfile(context.Context, uint64, string, string) (*user.User, error) {
	if r.updateErr != nil {
		return nil, r.updateErr
	}
	return r.updated, nil
}
func (r *handlerUserRepository) ChangePasswordAndRevokeSessions(context.Context, uint64, string, time.Time) error {
	return r.passwordErr
}

func TestLoginSetsScopedRefreshCookieAndDoesNotReturnRefreshPlaintext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	hash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	account := &user.User{ID: 42, Username: "milo", Nickname: "小猫", PasswordHash: string(hash), Status: user.StatusNormal, CreatedAt: time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)}
	users := &handlerUserRepository{byUsername: account}
	users.byID = account
	sessions := NewService(&fakeRepository{createFamilyFn: func(context.Context, *Family, *RefreshToken) error { return nil }}, token.NewManager("test-secret", "video-share", 15*time.Minute), Options{
		Now: func() time.Time { return time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC) },
	})
	h := NewHandler(user.NewService(users, token.NewManager("test-secret", "video-share", 15*time.Minute)), sessions, 24*time.Hour, false, nil)
	r := gin.New()
	r.POST("/", h.Login)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"username":"milo","password":"correct-password"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("login status = %d, body=%s", w.Code, w.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(w.Body.String(), "refresh_token") {
		t.Fatalf("login response leaked refresh token: %s", w.Body.String())
	}
	var refreshCookie, csrfCookie *http.Cookie
	for _, cookie := range w.Result().Cookies() {
		copy := *cookie
		switch cookie.Name {
		case RefreshCookieName:
			refreshCookie = &copy
		case CSRFCookieName:
			csrfCookie = &copy
		}
	}
	if refreshCookie == nil || refreshCookie.Path != "/api/v1/auth" || !refreshCookie.HttpOnly || refreshCookie.Secure {
		t.Fatalf("refresh cookie attributes = %+v", refreshCookie)
	}
	if csrfCookie == nil || csrfCookie.Path != "/" || csrfCookie.HttpOnly {
		t.Fatalf("csrf cookie attributes = %+v", csrfCookie)
	}
}

func TestUpdateProfileReturnsUserShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	updated := &user.User{ID: 7, Username: "milo", Nickname: "新昵称", Bio: "简介", Status: user.StatusNormal, CreatedAt: time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)}
	users := &handlerUserRepository{updated: updated}
	h := NewHandler(user.NewService(users, token.NewManager("test-secret", "video-share", time.Minute)), nil, time.Hour, false, nil)
	r := gin.New()
	r.PATCH("/", func(c *gin.Context) {
		c.Set(middleware.UserIDKey, uint64(7))
		h.UpdateProfile(c)
	})
	req := httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(`{"nickname":"新昵称","bio":"简介"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), "access_token") {
		t.Fatalf("profile response = %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"nickname":"新昵称"`) || !strings.Contains(w.Body.String(), `"bio":"简介"`) {
		t.Fatalf("profile response missing user fields: %s", w.Body.String())
	}
}

func TestLogoutRevokesByRefreshCookieWhenAccessTokenIsAbsent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	called := false
	sessions := NewService(&fakeRepository{
		revokeByRefreshFn: func(_ context.Context, hash []byte, _ time.Time) error {
			called = len(hash) == 32
			return nil
		},
	}, token.NewManager("test-secret", "video-share", time.Minute), Options{})
	h := NewHandler(nil, sessions, time.Hour, false, nil)
	r := gin.New()
	r.POST("/", h.Logout)
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.AddCookie(&http.Cookie{Name: RefreshCookieName, Value: "refresh-secret"})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent || !called {
		t.Fatalf("logout status=%d called=%v body=%s", w.Code, called, w.Body.String())
	}
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == RefreshCookieName && cookie.MaxAge != -1 {
			t.Fatalf("refresh cookie was not cleared: %+v", cookie)
		}
	}
}
