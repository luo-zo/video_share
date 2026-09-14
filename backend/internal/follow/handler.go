package follow

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

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

func (h *Handler) Follow(c *gin.Context)   { h.apply(c, "follow user", h.svc.Follow) }
func (h *Handler) Unfollow(c *gin.Context) { h.apply(c, "unfollow user", h.svc.Unfollow) }

// apply 是关注 / 取关端点的共同骨架：先要求已登录，再解析目标用户 ID，然后写入并
// 返回最终状态。首次调用和重复调用都会返回相同的 200 结果。
func (h *Handler) apply(c *gin.Context, operation string, write func(context.Context, uint64, uint64) (*FollowState, error)) {
	userID := c.GetUint64(middleware.UserIDKey)
	if userID == 0 {
		response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "请先登录")
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(c, http.StatusBadRequest, response.CodeInvalidParameter, "invalid user id")
		return
	}
	state, err := write(c.Request.Context(), userID, id)
	if err != nil {
		h.handleError(c, operation, err)
		return
	}
	response.OK(c, http.StatusOK, state)
}

func (h *Handler) ListFollows(c *gin.Context) {
	userID := c.GetUint64(middleware.UserIDKey)
	if userID == 0 {
		response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "请先登录")
		return
	}
	page, pageSize, ok := pagination(c)
	if !ok {
		return
	}
	result, err := h.svc.ListFollows(c.Request.Context(), userID, page, pageSize)
	if err != nil {
		h.handleError(c, "list follows", err)
		return
	}
	response.OK(c, http.StatusOK, result)
}

func (h *Handler) handleError(c *gin.Context, operation string, err error) {
	switch {
	case errors.Is(err, ErrUnauthorized):
		response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "请先登录")
	case errors.Is(err, ErrSelfFollow):
		response.Error(c, http.StatusBadRequest, response.CodeInvalidParameter, "不能关注自己")
	case errors.Is(err, ErrUserNotFound):
		response.Error(c, http.StatusNotFound, response.CodeNotFound, "用户不存在")
	case errors.Is(err, ErrPaginationInvalid):
		response.Error(c, http.StatusBadRequest, response.CodeInvalidParameter, "分页参数无效")
	default:
		h.log.Error(operation+" failed", "error", err)
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "internal server error")
	}
}

func pagination(c *gin.Context) (int, int, bool) {
	page, ok := positiveQuery(c, "page", 1)
	if !ok {
		return 0, 0, false
	}
	pageSize, ok := positiveQuery(c, "page_size", defaultPageSize)
	if !ok || pageSize > maxPageSize {
		if ok {
			response.Error(c, http.StatusBadRequest, response.CodeInvalidParameter, "page_size must be between 1 and 50")
		}
		return 0, 0, false
	}
	return page, pageSize, true
}

func positiveQuery(c *gin.Context, name string, fallback int) (int, bool) {
	raw := c.Query(name)
	if raw == "" {
		return fallback, true
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		response.Error(c, http.StatusBadRequest, response.CodeInvalidParameter, name+" must be a positive integer")
		return 0, false
	}
	return value, true
}
