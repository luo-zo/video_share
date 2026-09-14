package engagement

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

func (h *Handler) Like(c *gin.Context)     { h.apply(c, "like video", true, h.svc.SetLike) }
func (h *Handler) Unlike(c *gin.Context)   { h.apply(c, "unlike video", false, h.svc.SetLike) }
func (h *Handler) Favorite(c *gin.Context) { h.apply(c, "favorite video", true, h.svc.SetFavorite) }
func (h *Handler) Unfavorite(c *gin.Context) {
	h.apply(c, "unfavorite video", false, h.svc.SetFavorite)
}

// apply 是所有点赞 / 收藏端点的共同骨架：先要求已登录，再解析视频 ID，然后写入
// 并返回最终状态。首次调用和重复调用都会返回相同的 200 结果。
func (h *Handler) apply(c *gin.Context, operation string, active bool, write func(context.Context, uint64, uint64, bool) (*RelationState, error)) {
	userID := c.GetUint64(middleware.UserIDKey)
	if userID == 0 {
		response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "请先登录")
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(c, http.StatusBadRequest, response.CodeInvalidParameter, "invalid video id")
		return
	}
	state, err := write(c.Request.Context(), userID, id, active)
	if err != nil {
		h.handleError(c, operation, err)
		return
	}
	response.OK(c, http.StatusOK, state)
}

func (h *Handler) handleError(c *gin.Context, operation string, err error) {
	switch {
	case errors.Is(err, ErrUnauthorized):
		response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "请先登录")
	case errors.Is(err, ErrVideoNotFound):
		response.Error(c, http.StatusNotFound, response.CodeNotFound, "视频不存在")
	default:
		h.log.Error(operation+" failed", "error", err)
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "internal server error")
	}
}
