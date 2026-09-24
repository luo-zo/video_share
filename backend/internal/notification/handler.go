package notification

import (
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

func NewHandler(svc *Service, log *slog.Logger) *Handler { return &Handler{svc: svc, log: log} }

func (h *Handler) List(c *gin.Context) {
	userID := c.GetUint64(middleware.UserIDKey)
	if userID == 0 {
		response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "请先登录")
		return
	}
	page, pageSize, ok := pageParams(c)
	if !ok {
		return
	}
	result, err := h.svc.List(c.Request.Context(), userID, page, pageSize)
	if err != nil {
		h.handleError(c, "list notifications", err)
		return
	}
	response.OK(c, http.StatusOK, result)
}

func (h *Handler) MarkRead(c *gin.Context) {
	userID := c.GetUint64(middleware.UserIDKey)
	if userID == 0 {
		response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "请先登录")
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(c, http.StatusBadRequest, response.CodeInvalidParameter, "invalid notification id")
		return
	}
	if err := h.svc.MarkRead(c.Request.Context(), userID, id); err != nil {
		h.handleError(c, "mark notification read", err)
		return
	}
	response.OK(c, http.StatusOK, gin.H{"id": id, "read": true})
}

func (h *Handler) MarkAllRead(c *gin.Context) {
	userID := c.GetUint64(middleware.UserIDKey)
	if userID == 0 {
		response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "请先登录")
		return
	}
	count, err := h.svc.MarkAllRead(c.Request.Context(), userID)
	if err != nil {
		h.handleError(c, "mark all notifications read", err)
		return
	}
	response.OK(c, http.StatusOK, gin.H{"marked": count})
}

func (h *Handler) handleError(c *gin.Context, operation string, err error) {
	switch {
	case errors.Is(err, ErrPaginationInvalid):
		response.Error(c, http.StatusBadRequest, response.CodeInvalidParameter, "分页参数无效")
	case errors.Is(err, ErrNotFound):
		response.Error(c, http.StatusNotFound, response.CodeNotFound, "通知不存在")
	default:
		if h.log != nil {
			h.log.Error(operation+" failed", "error", err)
		}
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "internal server error")
	}
}

func pageParams(c *gin.Context) (int, int, bool) {
	page := 1
	pageSize := defaultPageSize
	if raw := c.Query("page"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 {
			response.Error(c, http.StatusBadRequest, response.CodeInvalidParameter, "page must be a positive integer")
			return 0, 0, false
		}
		page = value
	}
	if raw := c.Query("page_size"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > maxPageSize {
			response.Error(c, http.StatusBadRequest, response.CodeInvalidParameter, "page_size must be between 1 and 50")
			return 0, 0, false
		}
		pageSize = value
	}
	return page, pageSize, true
}
