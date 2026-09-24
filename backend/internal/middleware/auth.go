package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"video_share/internal/response"
	"video_share/internal/token"
)

// UserIDKey 是保存已认证用户 ID 的上下文键。
const UserIDKey = "user_id"

// SessionIDKey is the durable session-family ID from a sid-bearing access JWT.
const SessionIDKey = "session_id"

// UserValidator checks that a token subject still maps to an enabled account.
// The boolean distinguishes an inactive/missing account from a database failure.
type UserValidator func(ctx context.Context, userID uint64) (bool, error)

// SessionValidator checks that an access token's sid is still active for its
// user. It is deliberately separate from the user status check so legacy
// callers can keep using Auth while upgraded routes require sid.
type SessionValidator func(ctx context.Context, userID uint64, sessionID string) (bool, error)

// Auth 校验 Bearer 令牌和令牌主体对应的账号状态，再将用户 ID 存入上下文。
func Auth(tm *token.Manager, validate UserValidator) gin.HandlerFunc {
	return auth(tm, validate, nil, false)
}

// AuthWithSession validates both account status and the durable session family.
// Tokens without sid are rejected, forcing migration-era clients to log in.
func AuthWithSession(tm *token.Manager, validate UserValidator, validateSession SessionValidator) gin.HandlerFunc {
	return auth(tm, validate, validateSession, true)
}

func auth(tm *token.Manager, validate UserValidator, validateSession SessionValidator, requireSession bool) gin.HandlerFunc {
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
		active, err := validate(c.Request.Context(), claims.UserID)
		if err != nil {
			response.Error(c, http.StatusInternalServerError, response.CodeInternal, "internal server error")
			return
		}
		if !active {
			response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "invalid or expired token")
			return
		}
		if requireSession {
			if claims.SessionID == "" || validateSession == nil {
				response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "invalid or expired token")
				return
			}
			activeSession, sessionErr := validateSession(c.Request.Context(), claims.UserID, claims.SessionID)
			if sessionErr != nil {
				response.Error(c, http.StatusInternalServerError, response.CodeInternal, "internal server error")
				return
			}
			if !activeSession {
				response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "invalid or expired token")
				return
			}
			c.Set(SessionIDKey, claims.SessionID)
		}
		c.Set(UserIDKey, claims.UserID)
		c.Next()
	}
}

// OptionalAuth 在携带有效 Bearer 令牌且账号仍可用时设置 user_id。无效、过期、
// 不存在或已禁用的账号按匿名处理；账号校验发生数据库错误时返回 500。
func OptionalAuth(tm *token.Manager, validate UserValidator) gin.HandlerFunc {
	return optionalAuth(tm, validate, nil, false)
}

// OptionalAuthWithSession keeps invalid session credentials anonymous while
// still validating sid for a valid optional credential.
func OptionalAuthWithSession(tm *token.Manager, validate UserValidator, validateSession SessionValidator) gin.HandlerFunc {
	return optionalAuth(tm, validate, validateSession, true)
}

func optionalAuth(tm *token.Manager, validate UserValidator, validateSession SessionValidator, requireSession bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, ok := bearerClaims(c, tm)
		if !ok {
			c.Next()
			return
		}
		id := claims.UserID
		active, err := validate(c.Request.Context(), id)
		if err != nil {
			response.Error(c, http.StatusInternalServerError, response.CodeInternal, "internal server error")
			return
		}
		if active {
			if requireSession {
				if claims.SessionID == "" || validateSession == nil {
					c.Next()
					return
				}
				activeSession, sessionErr := validateSession(c.Request.Context(), id, claims.SessionID)
				if sessionErr != nil {
					response.Error(c, http.StatusInternalServerError, response.CodeInternal, "internal server error")
					return
				}
				if !activeSession {
					c.Next()
					return
				}
				c.Set(SessionIDKey, claims.SessionID)
			}
			c.Set(UserIDKey, id)
		}
		c.Next()
	}
}

// bearerUserID 从可选的 Authorization 头解析用户 ID；头缺失、格式错误或令牌无效
// 时都返回 0，表示匿名访问。
func bearerUserID(c *gin.Context, tm *token.Manager) uint64 {
	claims, ok := bearerClaims(c, tm)
	if !ok {
		return 0
	}
	return claims.UserID
}

func bearerClaims(c *gin.Context, tm *token.Manager) (*token.Claims, bool) {
	scheme, raw, ok := strings.Cut(c.GetHeader("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || raw == "" {
		return nil, false
	}
	claims, err := tm.Parse(raw)
	if err != nil {
		return nil, false
	}
	return claims, true
}
