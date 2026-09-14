package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// AccessLog 为每个请求输出一行结构化日志。它只记录方法、路径、状态码、延迟、
// 客户端 IP 和请求 ID——绝不记录请求体、请求头或凭据。
func AccessLog(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		log.Info("http request",
			"request_id", c.GetString(RequestIDKey),
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"client_ip", c.ClientIP(),
			"latency_ms", time.Since(start).Milliseconds(),
		)
	}
}
