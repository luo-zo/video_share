package creator

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"video_share/internal/follow"
	"video_share/internal/middleware"
	"video_share/internal/response"
)

type Handler struct {
	svc *Service
	log *slog.Logger
}

func NewHandler(svc *Service, log *slog.Logger) *Handler { return &Handler{svc: svc, log: log} }

func (h *Handler) Profile(c *gin.Context) {
	id, ok := creatorID(c)
	if !ok {
		return
	}
	profile, err := h.svc.Profile(c.Request.Context(), id, c.GetUint64(middleware.UserIDKey))
	if err != nil {
		h.handleError(c, "get public creator", err)
		return
	}
	response.OK(c, http.StatusOK, profile)
}

func (h *Handler) Videos(c *gin.Context) {
	id, ok := creatorID(c)
	if !ok {
		return
	}
	page, pageSize, ok := pagination(c)
	if !ok {
		return
	}
	items, err := h.svc.Videos(c.Request.Context(), id, page, pageSize)
	if err != nil {
		h.handleError(c, "list public creator videos", err)
		return
	}
	response.OK(c, http.StatusOK, items)
}

func (h *Handler) Followers(c *gin.Context) {
	h.listRelations(c, h.svc.Followers)
}

func (h *Handler) Following(c *gin.Context) {
	h.listRelations(c, h.svc.Following)
}

func (h *Handler) listRelations(c *gin.Context, list func(context.Context, uint64, int, int) (*follow.FollowListResponse, error)) {
	id, ok := creatorID(c)
	if !ok {
		return
	}
	page, pageSize, ok := pagination(c)
	if !ok {
		return
	}
	items, err := list(c.Request.Context(), id, page, pageSize)
	if err != nil {
		h.handleError(c, "list public creator relations", err)
		return
	}
	response.OK(c, http.StatusOK, items)
}

func (h *Handler) handleError(c *gin.Context, operation string, err error) {
	switch {
	case errors.Is(err, ErrNotFound), errors.Is(err, follow.ErrUserNotFound):
		response.Error(c, http.StatusNotFound, response.CodeNotFound, "用户不存在")
	case errors.Is(err, ErrPaginationInvalid), errors.Is(err, follow.ErrPaginationInvalid):
		response.Error(c, http.StatusBadRequest, response.CodeInvalidParameter, "分页参数无效")
	default:
		if h.log != nil {
			h.log.Error(operation+" failed", "error", err)
		}
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "internal server error")
	}
}

func creatorID(c *gin.Context) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(c, http.StatusBadRequest, response.CodeInvalidParameter, "invalid user id")
		return 0, false
	}
	return id, true
}

func pagination(c *gin.Context) (int, int, bool) {
	page := positiveQuery(c, "page", 1)
	pageSize := positiveQuery(c, "page_size", defaultPageSize)
	if page < 1 || pageSize < 1 || pageSize > maxPageSize {
		response.Error(c, http.StatusBadRequest, response.CodeInvalidParameter, "分页参数无效")
		return 0, 0, false
	}
	return page, pageSize, true
}

func positiveQuery(c *gin.Context, name string, fallback int) int {
	raw := c.Query(name)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return value
}
