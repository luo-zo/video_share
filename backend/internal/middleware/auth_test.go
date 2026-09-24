package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"video_share/internal/token"
)

func setupAuthRouter(tm *token.Manager) *gin.Engine {
	return setupAuthRouterWithValidator(tm, func(context.Context, uint64) (bool, error) { return true, nil })
}

func setupAuthRouterWithValidator(tm *token.Manager, validate UserValidator) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/protected", Auth(tm, validate), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"uid": c.GetUint64(UserIDKey)})
	})
	return r
}

func setupAuthRouterWithSessionValidator(tm *token.Manager, validate SessionValidator) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/protected", AuthWithSession(tm, func(context.Context, uint64) (bool, error) { return true, nil }, validate), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"uid": c.GetUint64(UserIDKey)})
	})
	return r
}

func TestAuthMissingHeader(t *testing.T) {
	tm := token.NewManager("secret", "video-share", time.Hour)
	w := httptest.NewRecorder()
	setupAuthRouter(tm).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/protected", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestAuthMalformedHeader(t *testing.T) {
	tm := token.NewManager("secret", "video-share", time.Hour)
	r := setupAuthRouter(tm)
	for _, h := range []string{"Bearer", "Basic abc", "Bearer "} {
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", h)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("header %q: status = %d, want 401", h, w.Code)
		}
	}
}

func TestAuthValidToken(t *testing.T) {
	tm := token.NewManager("secret", "video-share", time.Hour)
	tok, _, _ := tm.Generate(99)
	r := setupAuthRouter(tm)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"uid":99`) {
		t.Fatalf("body = %s", w.Body.String())
	}
}

func TestAuthWithSessionRejectsLegacyAndRevokedSID(t *testing.T) {
	tm := token.NewManager("secret", "video-share", time.Hour)
	valid, _, _ := tm.GenerateWithSession(99, "family-1")
	legacy, _, _ := tm.Generate(99)
	validateSession := func(_ context.Context, userID uint64, sid string) (bool, error) {
		return userID == 99 && sid == "family-1", nil
	}
	r := setupAuthRouterWithSessionValidator(tm, validateSession)
	for name, value := range map[string]string{"valid": valid, "legacy": legacy} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			req.Header.Set("Authorization", "Bearer "+value)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			want := http.StatusOK
			if name == "legacy" {
				want = http.StatusUnauthorized
			}
			if w.Code != want {
				t.Fatalf("status = %d, want %d; body=%s", w.Code, want, w.Body.String())
			}
		})
	}
}

func TestAuthRejectsExpiredToken(t *testing.T) {
	tm := token.NewManager("secret", "video-share", time.Hour)
	now := time.Now()
	claims := token.Claims{
		UserID: 1,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "video-share",
			IssuedAt:  jwt.NewNumericDate(now.Add(-2 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(now.Add(-time.Hour)),
		},
	}
	signed, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("secret"))

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+signed)
	w := httptest.NewRecorder()
	setupAuthRouter(tm).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestAuthRejectsTamperedToken(t *testing.T) {
	tm := token.NewManager("secret", "video-share", time.Hour)
	tok, _, _ := tm.Generate(1)
	parts := strings.Split(tok, ".")
	parts[2] = strings.Repeat("A", len(parts[2]))
	tampered := strings.Join(parts, ".")

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tampered)
	w := httptest.NewRecorder()
	setupAuthRouter(tm).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestAuthRejectsWrongAlgorithm(t *testing.T) {
	tm := token.NewManager("secret", "video-share", time.Hour)
	now := time.Now()
	claims := token.Claims{
		UserID: 1,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "video-share",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		},
	}
	signed, _ := jwt.NewWithClaims(jwt.SigningMethodHS384, claims).SignedString([]byte("secret"))

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+signed)
	w := httptest.NewRecorder()
	setupAuthRouter(tm).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func setupOptionalAuthRouter(tm *token.Manager) *gin.Engine {
	return setupOptionalAuthRouterWithValidator(tm, func(context.Context, uint64) (bool, error) { return true, nil })
}

func setupOptionalAuthRouterWithValidator(tm *token.Manager, validate UserValidator) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/public", OptionalAuth(tm, validate), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"uid": c.GetUint64(UserIDKey)})
	})
	return r
}

func TestAuthRejectsMissingOrDisabledTokenSubject(t *testing.T) {
	tm := token.NewManager("secret", "video-share", time.Hour)
	tok, _, _ := tm.Generate(99)
	r := setupAuthRouterWithValidator(tm, func(_ context.Context, id uint64) (bool, error) {
		if id != 99 {
			t.Fatalf("validator id = %d", id)
		}
		return false, nil
	})
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized || !strings.Contains(w.Body.String(), `"code":"UNAUTHORIZED"`) {
		t.Fatalf("response = %d %s, want 401 UNAUTHORIZED", w.Code, w.Body.String())
	}
}

func TestOptionalAuthTreatsMissingOrDisabledTokenSubjectAsAnonymous(t *testing.T) {
	tm := token.NewManager("secret", "video-share", time.Hour)
	tok, _, _ := tm.Generate(99)
	r := setupOptionalAuthRouterWithValidator(tm, func(context.Context, uint64) (bool, error) { return false, nil })
	req := httptest.NewRequest(http.MethodGet, "/public", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"uid":0`) {
		t.Fatalf("response = %d %s, want anonymous", w.Code, w.Body.String())
	}
}

func TestAuthValidatorDatabaseErrorsReturn500(t *testing.T) {
	tm := token.NewManager("secret", "video-share", time.Hour)
	tok, _, _ := tm.Generate(99)
	validate := func(context.Context, uint64) (bool, error) { return false, errors.New("database unavailable") }
	for _, r := range []*gin.Engine{
		setupAuthRouterWithValidator(tm, validate),
		setupOptionalAuthRouterWithValidator(tm, validate),
	} {
		path := "/protected"
		if len(r.Routes()) > 0 && r.Routes()[0].Path == "/public" {
			path = "/public"
		}
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusInternalServerError || !strings.Contains(w.Body.String(), `"code":"INTERNAL_ERROR"`) {
			t.Fatalf("%s response = %d %s, want 500", path, w.Code, w.Body.String())
		}
	}
}

func TestOptionalAuthTreatsMissingHeaderAsAnonymous(t *testing.T) {
	tm := token.NewManager("secret", "video-share", time.Hour)
	w := httptest.NewRecorder()
	setupOptionalAuthRouter(tm).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/public", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"uid":0`) {
		t.Fatalf("body = %s, want anonymous uid 0", w.Body.String())
	}
}

func TestOptionalAuthSetsUserIDFromValidToken(t *testing.T) {
	tm := token.NewManager("secret", "video-share", time.Hour)
	tok, _, _ := tm.Generate(42)

	req := httptest.NewRequest(http.MethodGet, "/public", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	setupOptionalAuthRouter(tm).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"uid":42`) {
		t.Fatalf("body = %s, want uid 42", w.Body.String())
	}
}

func TestOptionalAuthIgnoresInvalidTokensAnonymously(t *testing.T) {
	tm := token.NewManager("secret", "video-share", time.Hour)
	tok, _, _ := tm.Generate(7)
	parts := strings.Split(tok, ".")
	parts[2] = strings.Repeat("A", len(parts[2]))
	tampered := strings.Join(parts, ".")

	for _, h := range []string{"", "Bearer", "Basic abc", "Bearer ", "Bearer not-a-token", "Bearer " + tampered} {
		req := httptest.NewRequest(http.MethodGet, "/public", nil)
		if h != "" {
			req.Header.Set("Authorization", h)
		}
		w := httptest.NewRecorder()
		setupOptionalAuthRouter(tm).ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("header %q: status = %d, want 200", h, w.Code)
		}
		if !strings.Contains(w.Body.String(), `"uid":0`) {
			t.Fatalf("header %q: body = %s, want anonymous", h, w.Body.String())
		}
	}
}
