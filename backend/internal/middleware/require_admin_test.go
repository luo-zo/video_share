package middleware

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequireAdminChecksCurrentRoleAndStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name    string
		role    string
		active  bool
		loadErr error
		want    int
	}{
		{name: "admin", role: "admin", active: true, want: 200},
		{name: "ordinary user", role: "user", active: true, want: 403},
		{name: "disabled admin", role: "admin", active: false, want: 403},
		{name: "database failure", role: "", active: false, loadErr: errors.New("down"), want: 500},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			r.Use(func(c *gin.Context) { c.Set(UserIDKey, uint64(7)); c.Next() })
			r.Use(RequireAdmin(func(context.Context, uint64) (string, bool, error) { return tt.role, tt.active, tt.loadErr }))
			r.GET("/", func(c *gin.Context) { c.Status(200) })
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
			if w.Code != tt.want {
				t.Fatalf("status=%d, want %d", w.Code, tt.want)
			}
		})
	}
}
