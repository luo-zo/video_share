package middleware

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"video_share/internal/response"
)

// Recovery 将 panic 转换为 500 JSON 响应，并仅在服务端记录 panic 详情。
func Recovery(log *slog.Logger) gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered any) {
		log.Error("panic recovered", "request_id", c.GetString(RequestIDKey), "panic", recovered)
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "internal server error")
	})
}
