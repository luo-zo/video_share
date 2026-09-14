package middleware

import (
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
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/protected", Auth(tm), func(c *gin.Context) {
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
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/public", OptionalAuth(tm), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"uid": c.GetUint64(UserIDKey)})
	})
	return r
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
