package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// LimitBody 限制从请求体读取的字节数。它使用 http.MaxBytesReader，使限制作用于
// 实际读取的字节数，而非声明的 Content-Length。读取超过限制会使下游的 JSON
// 解码以 *http.MaxBytesError 失败，处理器会将其映射为 HTTP 413。
func LimitBody(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		c.Next()
	}
}
