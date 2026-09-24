package ranking

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestHandlerMapsFallbackSaturationToServiceUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, err := range []error{ErrFallbackBusy, ErrTooManyRows} {
		t.Run(err.Error(), func(t *testing.T) {
			repo := fakeRepository{listErr: err}
			h := NewHandler(NewService(repo), slog.New(slog.NewTextHandler(io.Discard, nil)))
			router := gin.New()
			router.GET("/videos/ranking", h.List)
			req := httptest.NewRequest(http.MethodGet, "/videos/ranking?window=day", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), `"code":"SERVICE_UNAVAILABLE"`) {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
		})
	}
}

var _ Repository = fakeRepository{}
