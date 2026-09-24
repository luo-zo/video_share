package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequireSameOriginCSRF(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/write", RequireSameOriginCSRF("http://127.0.0.1:5173"), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	cases := []struct {
		name       string
		origin     string
		csrfCookie string
		csrfHeader string
		content    string
		want       int
	}{
		{"valid", "http://127.0.0.1:5173", "abc", "abc", "application/json", http.StatusNoContent},
		{"wrong origin", "https://evil.example", "abc", "abc", "application/json", http.StatusForbidden},
		{"missing token", "http://127.0.0.1:5173", "", "", "application/json", http.StatusForbidden},
		{"wrong content type", "http://127.0.0.1:5173", "abc", "abc", "text/plain", http.StatusUnsupportedMediaType},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/write", nil)
			req.Header.Set("Origin", tc.origin)
			req.Header.Set(CSRFTokenHeader, tc.csrfHeader)
			req.Header.Set("Content-Type", tc.content)
			if tc.csrfCookie != "" {
				req.AddCookie(&http.Cookie{Name: "video_share_csrf", Value: tc.csrfCookie})
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d; body=%s", w.Code, tc.want, w.Body.String())
			}
		})
	}
}
