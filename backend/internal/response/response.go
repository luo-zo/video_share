package response

import (
	"github.com/gin-gonic/gin"
)

// 统一的错误信封中返回的稳定业务错误码。
const (
	CodeInvalidParameter     = "INVALID_PARAMETER"
	CodeRequestTooLarge      = "REQUEST_TOO_LARGE"
	CodeUnauthorized         = "UNAUTHORIZED"
	CodeForbidden            = "FORBIDDEN"
	CodeCommentForbidden     = "COMMENT_FORBIDDEN"
	CodeSelfFollow           = "SELF_FOLLOW"
	CodeAccountDisabled      = "ACCOUNT_DISABLED"
	CodeUserAlreadyExists    = "USER_ALREADY_EXISTS"
	CodeInvalidCredentials   = "INVALID_CREDENTIALS"
	CodeRateLimited          = "RATE_LIMITED"
	CodeRateLimitUnavailable = "RATE_LIMIT_UNAVAILABLE"
	CodeCSRFFailed           = "CSRF_FAILED"
	CodeRefreshConflict      = "REFRESH_CONFLICT"
	CodeSessionReused        = "SESSION_REUSED"
	CodeSessionExpired       = "SESSION_EXPIRED"
	CodeNotFound             = "NOT_FOUND"
	CodeUploadIncomplete     = "UPLOAD_INCOMPLETE"
	CodeUploadMismatch       = "UPLOAD_MISMATCH"
	CodeVideoStateConflict   = "VIDEO_STATE_CONFLICT"
	CodeInternal             = "INTERNAL_ERROR"
	CodeServiceUnavailable   = "SERVICE_UNAVAILABLE"
	CodeIdempotencyConflict  = "IDEMPOTENCY_CONFLICT"
)

const requestIDKey = "request_id"

// ErrorBody 是统一的错误信封：稳定的错误码、可读的消息，以及用于关联服务端
// 日志的请求 ID。
type ErrorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

// OK 写出带有顶层 "data" 字段的成功响应。
func OK(c *gin.Context, status int, data any) {
	c.JSON(status, gin.H{"data": data})
}

// Error 写出统一的错误信封并中止请求链。
func Error(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{
		"error": ErrorBody{
			Code:      code,
			Message:   message,
			RequestID: c.GetString(requestIDKey),
		},
	})
}
