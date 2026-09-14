package video

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

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

func (h *Handler) Create(c *gin.Context) {
	userID := c.GetUint64(middleware.UserIDKey)
	if userID == 0 {
		response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "请先登录")
		return
	}
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeInvalidParameter, "invalid request body")
		return
	}
	result, err := h.svc.Create(c.Request.Context(), userID, req)
	if err != nil {
		h.handleError(c, "create video", err)
		return
	}
	response.OK(c, http.StatusCreated, result)
}

func (h *Handler) Complete(c *gin.Context) {
	userID := c.GetUint64(middleware.UserIDKey)
	if userID == 0 {
		response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "请先登录")
		return
	}
	id, ok := videoID(c)
	if !ok {
		return
	}
	result, err := h.svc.Complete(c.Request.Context(), userID, id)
	if err != nil {
		h.handleError(c, "complete video", err)
		return
	}
	response.OK(c, http.StatusAccepted, result)
}

func (h *Handler) List(c *gin.Context) {
	page, pageSize, ok := pagination(c)
	if !ok {
		return
	}
	result, err := h.svc.List(c.Request.Context(), page, pageSize)
	if err != nil {
		h.handleError(c, "list videos", err)
		return
	}
	response.OK(c, http.StatusOK, result)
}

func (h *Handler) Detail(c *gin.Context) {
	id, ok := videoID(c)
	if !ok {
		return
	}
	result, err := h.svc.Detail(c.Request.Context(), id)
	if err != nil {
		h.handleError(c, "get video detail", err)
		return
	}
	response.OK(c, http.StatusOK, result)
}

func (h *Handler) Mine(c *gin.Context) {
	userID := c.GetUint64(middleware.UserIDKey)
	if userID == 0 {
		response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "请先登录")
		return
	}
	page, pageSize, ok := pagination(c)
	if !ok {
		return
	}
	result, err := h.svc.Mine(c.Request.Context(), userID, page, pageSize)
	if err != nil {
		h.handleError(c, "list current user videos", err)
		return
	}
	response.OK(c, http.StatusOK, result)
}

func (h *Handler) MineDetail(c *gin.Context) {
	userID := c.GetUint64(middleware.UserIDKey)
	if userID == 0 {
		response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "请先登录")
		return
	}
	id, ok := videoID(c)
	if !ok {
		return
	}
	result, err := h.svc.MineDetail(c.Request.Context(), userID, id)
	if err != nil {
		h.handleError(c, "get current user video", err)
		return
	}
	response.OK(c, http.StatusOK, result)
}

func (h *Handler) HLS(c *gin.Context) {
	id, ok := videoID(c)
	if !ok {
		return
	}
	mediaPath := strings.TrimPrefix(c.Param("path"), "/")
	if strings.HasSuffix(strings.ToLower(mediaPath), ".m3u8") {
		manifest, err := h.svc.HLSManifest(c.Request.Context(), id, mediaPath)
		if err != nil {
			h.handleError(c, "serve HLS manifest", err)
			return
		}
		c.Data(http.StatusOK, "application/vnd.apple.mpegurl", manifest)
		return
	}
	url, err := h.svc.HLSObjectURL(c.Request.Context(), id, mediaPath)
	if err != nil {
		h.handleError(c, "serve HLS object", err)
		return
	}
	c.Redirect(http.StatusTemporaryRedirect, url)
}

func (h *Handler) Cover(c *gin.Context) {
	id, ok := videoID(c)
	if !ok {
		return
	}
	url, err := h.svc.CoverURL(c.Request.Context(), id)
	if err != nil {
		h.handleError(c, "serve video cover", err)
		return
	}
	c.Redirect(http.StatusTemporaryRedirect, url)
}

func videoID(c *gin.Context) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(c, http.StatusBadRequest, response.CodeInvalidParameter, "invalid video id")
		return 0, false
	}
	return id, true
}

func pagination(c *gin.Context) (int, int, bool) {
	page, ok := positiveQuery(c, "page", 1)
	if !ok {
		return 0, 0, false
	}
	pageSize, ok := positiveQuery(c, "page_size", 12)
	if !ok || pageSize > 50 {
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

func (h *Handler) handleError(c *gin.Context, operation string, err error) {
	switch {
	case errors.Is(err, ErrUnauthorized):
		response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "请先登录")
	case errors.Is(err, ErrForbidden):
		response.Error(c, http.StatusForbidden, response.CodeForbidden, "无权操作这个视频")
	case errors.Is(err, ErrNotFound):
		response.Error(c, http.StatusNotFound, response.CodeNotFound, "视频不存在")
	case errors.Is(err, ErrTitleInvalid):
		response.Error(c, http.StatusBadRequest, response.CodeInvalidParameter, "标题不能为空且不超过 100 个字符")
	case errors.Is(err, ErrDescriptionInvalid):
		response.Error(c, http.StatusBadRequest, response.CodeInvalidParameter, "简介不能超过 2000 个字符")
	case errors.Is(err, ErrFileNameInvalid), errors.Is(err, ErrContentTypeInvalid):
		response.Error(c, http.StatusBadRequest, response.CodeInvalidParameter, "只支持 MP4 视频")
	case errors.Is(err, ErrFileSizeInvalid):
		response.Error(c, http.StatusBadRequest, response.CodeInvalidParameter, "视频文件大小不符合要求")
	case errors.Is(err, ErrPaginationInvalid):
		response.Error(c, http.StatusBadRequest, response.CodeInvalidParameter, "分页参数无效")
	case errors.Is(err, ErrUploadIncomplete):
		response.Error(c, http.StatusConflict, response.CodeUploadIncomplete, "视频文件尚未上传完成")
	case errors.Is(err, ErrUploadMismatch):
		response.Error(c, http.StatusConflict, response.CodeUploadMismatch, "上传文件与投稿信息不一致")
	case errors.Is(err, ErrStateConflict):
		response.Error(c, http.StatusConflict, response.CodeVideoStateConflict, "当前视频状态不能执行此操作")
	case errors.Is(err, ErrInvalidMediaPath):
		response.Error(c, http.StatusBadRequest, response.CodeInvalidParameter, "无效的媒体路径")
	default:
		h.log.Error(operation+" failed", "error", err)
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "internal server error")
	}
}
