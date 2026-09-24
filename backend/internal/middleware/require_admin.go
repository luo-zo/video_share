package middleware

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"video_share/internal/response"
)

const AdminKey = "admin_user"

// RequireAdmin re-checks the current database role for every request. JWTs do
// not carry an authorization snapshot, so disabling or demoting an admin takes
// effect without waiting for token expiry.
type AdminValidator func(context.Context, uint64) (role string, active bool, err error)

func RequireAdmin(load AdminValidator) gin.HandlerFunc {
	return func(c *gin.Context) {
		value, ok := c.Get(UserIDKey)
		userID, valid := value.(uint64)
		if !ok || !valid || userID == 0 {
			response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "请先登录")
			return
		}
		role, active, err := load(c.Request.Context(), userID)
		if err != nil {
			response.Error(c, http.StatusInternalServerError, response.CodeInternal, "internal server error")
			return
		}
		if !active || role != "admin" {
			response.Error(c, http.StatusForbidden, response.CodeForbidden, "管理员权限不足")
			return
		}
		c.Set(AdminKey, userID)
		c.Next()
	}
}
