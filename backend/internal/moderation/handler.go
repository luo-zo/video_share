package moderation

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"log/slog"

	"video_share/internal/middleware"
	"video_share/internal/notification"
	"video_share/internal/response"
)

type Handler struct {
	service *Service
	log     *slog.Logger
}

func NewHandler(service *Service, log *slog.Logger) *Handler {
	return &Handler{service: service, log: log}
}

func (h *Handler) CreateReport(c *gin.Context) {
	userID, ok := currentUser(c)
	if !ok {
		return
	}
	var input CreateReportRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeInvalidParameter, "invalid report body")
		return
	}
	input.RequestID = requestID(c, input.RequestID)
	result, err := h.service.CreateReport(c.Request.Context(), userID, input)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.OK(c, http.StatusCreated, result)
}

func (h *Handler) ListMyReports(c *gin.Context) {
	userID, ok := currentUser(c)
	if !ok {
		return
	}
	page, pageSize, valid := pagination(c)
	if !valid {
		return
	}
	result, err := h.service.ListMyReports(c.Request.Context(), userID, page, pageSize)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.OK(c, http.StatusOK, result)
}

func (h *Handler) ListAdminReports(c *gin.Context) {
	page, pageSize, valid := pagination(c)
	if !valid {
		return
	}
	result, err := h.service.ListAdminReports(c.Request.Context(), page, pageSize)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.OK(c, http.StatusOK, result)
}

func (h *Handler) AssignReport(c *gin.Context) {
	adminID, ok := currentUser(c)
	if !ok {
		return
	}
	reportID, ok := pathID(c, "id")
	if !ok {
		return
	}
	id := requestID(c, "")
	result, err := h.service.AssignReport(c.Request.Context(), adminID, reportID, id)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.OK(c, http.StatusOK, result)
}

func (h *Handler) DecideReport(c *gin.Context) {
	adminID, ok := currentUser(c)
	if !ok {
		return
	}
	reportID, ok := pathID(c, "id")
	if !ok {
		return
	}
	var input ReportDecisionRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeInvalidParameter, "invalid decision body")
		return
	}
	input.RequestID = requestID(c, input.RequestID)
	result, err := h.service.ResolveReport(c.Request.Context(), adminID, reportID, input)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.OK(c, http.StatusOK, result)
}

func (h *Handler) ModerateVideo(c *gin.Context) {
	h.moderate(c, TargetVideo, c.Param("id"), c.Param("action"))
}
func (h *Handler) ModerateComment(c *gin.Context) {
	h.moderate(c, TargetComment, c.Param("id"), c.Param("action"))
}
func (h *Handler) ModerateUser(c *gin.Context) {
	h.moderate(c, TargetUser, c.Param("id"), c.Param("action"))
}

func (h *Handler) moderate(c *gin.Context, targetType, rawID, action string) {
	adminID, ok := currentUser(c)
	if !ok {
		return
	}
	targetID, err := strconv.ParseUint(rawID, 10, 64)
	if err != nil || targetID == 0 {
		response.Error(c, http.StatusBadRequest, response.CodeInvalidParameter, "invalid target id")
		return
	}
	var input ModerationActionRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeInvalidParameter, "invalid moderation body")
		return
	}
	input.RequestID = requestID(c, input.RequestID)
	result, err := h.service.ModerateTarget(c.Request.Context(), adminID, targetType, targetID, action, input)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.OK(c, http.StatusOK, result)
}

func (h *Handler) ListActions(c *gin.Context) {
	page, pageSize, valid := pagination(c)
	if !valid {
		return
	}
	result, err := h.service.ListActions(c.Request.Context(), page, pageSize)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.OK(c, http.StatusOK, result)
}

func currentUser(c *gin.Context) (uint64, bool) {
	value, exists := c.Get(middleware.UserIDKey)
	if !exists {
		response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "请先登录")
		return 0, false
	}
	userID, ok := value.(uint64)
	if !ok || userID == 0 {
		response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "请先登录")
		return 0, false
	}
	return userID, true
}

func pathID(c *gin.Context, name string) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil || id == 0 {
		response.Error(c, http.StatusBadRequest, response.CodeInvalidParameter, "invalid id")
		return 0, false
	}
	return id, true
}

func pagination(c *gin.Context) (int, int, bool) {
	page, err1 := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, err2 := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if err1 != nil || err2 != nil || page < 1 || pageSize < 1 || pageSize > 50 {
		response.Error(c, http.StatusBadRequest, response.CodeInvalidParameter, "分页参数无效")
		return 0, 0, false
	}
	return page, pageSize, true
}

func requestID(c *gin.Context, body string) string {
	if strings.TrimSpace(body) != "" {
		return strings.TrimSpace(body)
	}
	if value := strings.TrimSpace(c.GetHeader("Idempotency-Key")); value != "" {
		return value
	}
	return strings.TrimSpace(c.GetHeader("X-Request-ID"))
}

func (h *Handler) writeError(c *gin.Context, err error) {
	status, code, message := http.StatusInternalServerError, response.CodeInternal, "internal server error"
	switch {
	case errors.Is(err, notification.ErrInvalidRequestID), errors.Is(err, ErrInvalidReport), errors.Is(err, ErrReasonRequired), errors.Is(err, ErrPaginationInvalid):
		status, code, message = http.StatusBadRequest, response.CodeInvalidParameter, "请求参数无效"
	case errors.Is(err, ErrReportNotFound), errors.Is(err, ErrTargetNotFound), errors.Is(err, notification.ErrNotFound):
		status, code, message = http.StatusNotFound, response.CodeNotFound, "目标不存在"
	case errors.Is(err, ErrActiveReport), errors.Is(err, ErrReportStateConflict), errors.Is(err, ErrActionConflict), errors.Is(err, notification.ErrReceiptConflict):
		status, code, message = http.StatusConflict, response.CodeIdempotencyConflict, "请求与当前治理状态冲突"
	case errors.Is(err, ErrTargetDeleted):
		status, code, message = http.StatusConflict, response.CodeVideoStateConflict, "已删除内容不能恢复或处置"
	case errors.Is(err, ErrSelfModeration), errors.Is(err, ErrLastAdmin), errors.Is(err, ErrAdminRequired):
		status, code, message = http.StatusForbidden, response.CodeForbidden, "没有权限执行这个治理操作"
	}
	if h.log != nil && status >= 500 {
		h.log.Error("moderation request failed", "error", err)
	}
	response.Error(c, status, code, message)
}
