package user

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"video_share/internal/middleware"
	"video_share/internal/response"
)

type Handler struct {
	svc *Service
	log *slog.Logger
}

func NewHandler(svc *Service, log *slog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

func (h *Handler) Register(c *gin.Context) {
	var req RegisterRequest
	if !h.bindJSON(c, &req) {
		return
	}

	u, err := h.svc.Register(c.Request.Context(), req.Username, req.Password, req.Nickname)
	if err != nil {
		h.handleRegisterError(c, err)
		return
	}
	response.OK(c, http.StatusCreated, toUserResponse(u))
}

func (h *Handler) Login(c *gin.Context) {
	var req LoginRequest
	if !h.bindJSON(c, &req) {
		return
	}

	_, tokenStr, ttl, err := h.svc.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			response.Error(c, http.StatusUnauthorized, response.CodeInvalidCredentials, "用户名或密码错误")
			return
		}
		h.log.Error("login failed", "error", err)
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "internal server error")
		return
	}
	response.OK(c, http.StatusOK, LoginResponse{
		AccessToken: tokenStr,
		TokenType:   "Bearer",
		ExpiresIn:   int64(ttl.Seconds()),
	})
}

func (h *Handler) Me(c *gin.Context) {
	userID := c.GetUint64(middleware.UserIDKey)
	if userID == 0 {
		response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "invalid token")
		return
	}

	u, err := h.svc.GetByID(c.Request.Context(), userID)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "invalid token")
		case errors.Is(err, ErrAccountDisabled):
			response.Error(c, http.StatusForbidden, response.CodeAccountDisabled, "账号已禁用")
		default:
			h.log.Error("get current user failed", "error", err, "user_id", userID)
			response.Error(c, http.StatusInternalServerError, response.CodeInternal, "internal server error")
		}
		return
	}
	response.OK(c, http.StatusOK, toUserResponse(u))
}

// bindJSON 解码 JSON 请求体，将请求体过大错误（来自 LimitBody 中间件）映射为
// 413，并将其他绑定失败映射为 400。
func (h *Handler) bindJSON(c *gin.Context, dst any) bool {
	err := c.ShouldBindJSON(dst)
	if err == nil {
		return true
	}
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		response.Error(c, http.StatusRequestEntityTooLarge, response.CodeRequestTooLarge, "request body too large")
		return false
	}
	response.Error(c, http.StatusBadRequest, response.CodeInvalidParameter, "invalid request body")
	return false
}

func (h *Handler) handleRegisterError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrUsernameInvalid):
		response.Error(c, http.StatusBadRequest, response.CodeInvalidParameter, "用户名须为 3~32 位字母、数字或下划线")
	case errors.Is(err, ErrPasswordInvalid):
		response.Error(c, http.StatusBadRequest, response.CodeInvalidParameter, "密码须为 8 个字符以上且不超过 72 字节")
	case errors.Is(err, ErrNicknameInvalid):
		response.Error(c, http.StatusBadRequest, response.CodeInvalidParameter, "昵称不能为空且不超过 64 个字符")
	case errors.Is(err, ErrUsernameTaken):
		response.Error(c, http.StatusConflict, response.CodeUserAlreadyExists, "用户名已存在")
	default:
		h.log.Error("register failed", "error", err)
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "internal server error")
	}
}
