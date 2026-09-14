package middleware

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/gin-gonic/gin"
)

// RequestIDKey 是保存请求 ID 的上下文键。它与 response 包共享字符串值
// "request_id"。
const RequestIDKey = "request_id"

// RequestID 将请求 ID（来自 X-Request-Id 或新生成的）注入上下文，并在响应中
// 原样返回。
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-Id")
		if id == "" {
			id = newRequestID()
		}
		c.Set(RequestIDKey, id)
		c.Header("X-Request-Id", id)
		c.Next()
	}
}

func newRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
