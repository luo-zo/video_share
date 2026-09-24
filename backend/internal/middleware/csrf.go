package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"video_share/internal/response"
)

const CSRFTokenHeader = "X-CSRF-Token"

// RequireSameOriginCSRF protects cookie-driven writes. The configured origin
// is compared byte-for-byte; no wildcard CORS or client-supplied trust is used.
func RequireSameOriginCSRF(expectedOrigin string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if expectedOrigin == "" || c.GetHeader("Origin") != expectedOrigin {
			response.Error(c, http.StatusForbidden, response.CodeCSRFFailed, "请求来源未通过校验")
			return
		}
		cookie, err := c.Cookie("video_share_csrf")
		header := c.GetHeader(CSRFTokenHeader)
		if err != nil || cookie == "" || header == "" || len(cookie) != len(header) || subtle.ConstantTimeCompare([]byte(cookie), []byte(header)) != 1 {
			response.Error(c, http.StatusForbidden, response.CodeCSRFFailed, "CSRF 校验失败")
			return
		}
		if c.Request.Method == http.MethodPost || c.Request.Method == http.MethodPut || c.Request.Method == http.MethodPatch {
			contentType := c.GetHeader("Content-Type")
			if !strings.HasPrefix(strings.ToLower(contentType), "application/json") {
				response.Error(c, http.StatusUnsupportedMediaType, response.CodeCSRFFailed, "请求必须使用 JSON")
				return
			}
		}
		c.Next()
	}
}
