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
	"video_share/internal/video"
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
	case errors.Is(err, ErrCommentNotFound):
		response.Error(c, http.StatusNotFound, response.CodeNotFound, "评论不存在")
	case errors.Is(err, ErrCommentForbidden):
		response.Error(c, http.StatusForbidden, response.CodeForbidden, "只能删除自己的评论")
	case errors.Is(err, ErrContentInvalid):
		response.Error(c, http.StatusBadRequest, response.CodeInvalidParameter, "评论内容需为 1-500 个字符")
	case errors.Is(err, ErrProgressInvalid):
		response.Error(c, http.StatusBadRequest, response.CodeInvalidParameter, "观看进度不能超过视频时长")
	case errors.Is(err, ErrPaginationInvalid):
		response.Error(c, http.StatusBadRequest, response.CodeInvalidParameter, "分页参数无效")
	default:
		h.log.Error(operation+" failed", "error", err)
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "internal server error")
	}
}

func (h *Handler) CreateComment(c *gin.Context) {
	userID := c.GetUint64(middleware.UserIDKey)
	if userID == 0 {
		response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "请先登录")
		return
	}
	id, ok := videoID(c)
	if !ok {
		return
	}
	var req CommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeInvalidParameter, "invalid request body")
		return
	}
	result, err := h.svc.CreateComment(c.Request.Context(), userID, id, req.Content)
	if err != nil {
		h.handleError(c, "create comment", err)
		return
	}
	response.OK(c, http.StatusCreated, result)
}

func (h *Handler) ListComments(c *gin.Context) {
	id, ok := videoID(c)
	if !ok {
		return
	}
	page, pageSize, ok := pagination(c)
	if !ok {
		return
	}
	result, err := h.svc.ListComments(c.Request.Context(), id, page, pageSize)
	if err != nil {
		h.handleError(c, "list comments", err)
		return
	}
	response.OK(c, http.StatusOK, result)
}

func (h *Handler) DeleteComment(c *gin.Context) {
	userID := c.GetUint64(middleware.UserIDKey)
	if userID == 0 {
		response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "请先登录")
		return
	}
	commentID, ok := commentIDParam(c)
	if !ok {
		return
	}
	if err := h.svc.DeleteComment(c.Request.Context(), userID, commentID); err != nil {
		h.handleError(c, "delete comment", err)
		return
	}
	response.OK(c, http.StatusOK, gin.H{"id": commentID})
}

func (h *Handler) RecordWatch(c *gin.Context) {
	userID := c.GetUint64(middleware.UserIDKey)
	if userID == 0 {
		response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "请先登录")
		return
	}
	id, ok := videoID(c)
	if !ok {
		return
	}
	var req WatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeInvalidParameter, "invalid request body")
		return
	}
	if err := h.svc.RecordWatch(c.Request.Context(), userID, id, req.ProgressMS, req.DurationMS); err != nil {
		h.handleError(c, "record watch", err)
		return
	}
	response.OK(c, http.StatusOK, gin.H{
		"video_id":    id,
		"progress_ms": req.ProgressMS,
		"duration_ms": req.DurationMS,
	})
}

func (h *Handler) Favorites(c *gin.Context) { h.listPersonal(c, "list favorites", h.svc.ListFavorites) }

func (h *Handler) History(c *gin.Context) { h.listPersonal(c, "list history", h.svc.ListHistory) }

func (h *Handler) listPersonal(c *gin.Context, operation string, load func(context.Context, uint64, int, int) (*video.ListResponse, error)) {
	userID := c.GetUint64(middleware.UserIDKey)
	if userID == 0 {
		response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "请先登录")
		return
	}
	page, pageSize, ok := pagination(c)
	if !ok {
		return
	}
	result, err := load(c.Request.Context(), userID, page, pageSize)
	if err != nil {
		h.handleError(c, operation, err)
		return
	}
	response.OK(c, http.StatusOK, result)
}

func videoID(c *gin.Context) (uint64, bool) {
	return pathID(c, "id", "invalid video id")
}

func commentIDParam(c *gin.Context) (uint64, bool) {
	return pathID(c, "id", "invalid comment id")
}

func pathID(c *gin.Context, name, message string) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil || id == 0 {
		response.Error(c, http.StatusBadRequest, response.CodeInvalidParameter, message)
		return 0, false
	}
	return id, true
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
