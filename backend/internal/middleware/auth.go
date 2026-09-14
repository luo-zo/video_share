package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"video_share/internal/response"
	"video_share/internal/token"
)

// UserIDKey 是保存已认证用户 ID 的上下文键。
const UserIDKey = "user_id"

// Auth 校验 Bearer 令牌，并将已认证的用户 ID 存入上下文。
// 它不检查用户是否存在或状态——那是处理器的职责。
func Auth(tm *token.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		authz := c.GetHeader("Authorization")
		if authz == "" {
			response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "missing authorization header")
			return
		}
		scheme, raw, ok := strings.Cut(authz, " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") || raw == "" {
			response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "invalid authorization header")
			return
		}
		claims, err := tm.Parse(raw)
		if err != nil {
			response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "invalid or expired token")
			return
		}
		c.Set(UserIDKey, claims.UserID)
		c.Next()
	}
}
