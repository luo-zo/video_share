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

// OptionalAuth 在携带有效 Bearer 令牌时设置 user_id，否则以匿名身份放行。它从不
// 写出响应，因此同一个公开处理器既能服务匿名访问，也能返回当前用户的互动状态。
// 无效或过期的令牌按未认证处理，不会泄露错误细节。
func OptionalAuth(tm *token.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if id := bearerUserID(c, tm); id != 0 {
			c.Set(UserIDKey, id)
		}
		c.Next()
	}
}

// bearerUserID 从可选的 Authorization 头解析用户 ID；头缺失、格式错误或令牌无效
// 时都返回 0，表示匿名访问。
func bearerUserID(c *gin.Context, tm *token.Manager) uint64 {
	scheme, raw, ok := strings.Cut(c.GetHeader("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || raw == "" {
		return 0
	}
	claims, err := tm.Parse(raw)
	if err != nil {
		return 0
	}
	return claims.UserID
}
