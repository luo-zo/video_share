package analytics

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

func (h *Handler) StartSession(c *gin.Context) {
	userID := c.GetUint64(middleware.UserIDKey)
	if userID == 0 {
		response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "请先登录")
		return
	}
	videoID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || videoID == 0 {
		response.Error(c, http.StatusBadRequest, response.CodeInvalidParameter, "invalid video id")
		return
	}
	result, err := h.svc.StartSession(c.Request.Context(), userID, videoID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.OK(c, http.StatusCreated, result)
}

func (h *Handler) Heartbeat(c *gin.Context) {
	userID := c.GetUint64(middleware.UserIDKey)
	if userID == 0 {
		response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "请先登录")
		return
	}
	var input HeartbeatInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeInvalidParameter, "invalid heartbeat body")
		return
	}
	result, err := h.svc.Heartbeat(c.Request.Context(), userID, c.Param("id"), input)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.OK(c, http.StatusOK, result)
}

func (h *Handler) CreatorSummary(c *gin.Context) {
	userID := c.GetUint64(middleware.UserIDKey)
	if userID == 0 {
		response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "请先登录")
		return
	}
	days, err := strconv.Atoi(c.DefaultQuery("days", "7"))
	if err != nil || (days != 7 && days != 30) {
		response.Error(c, http.StatusBadRequest, response.CodeInvalidParameter, "days must be 7 or 30")
		return
	}
	result, err := h.svc.CreatorSummary(c.Request.Context(), userID, days)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.OK(c, http.StatusOK, result)
}

func (h *Handler) writeError(c *gin.Context, err error) {
	status, code, message := http.StatusInternalServerError, response.CodeInternal, "internal server error"
	switch {
	case errors.Is(err, ErrUnauthorized):
		status, code, message = http.StatusUnauthorized, response.CodeUnauthorized, "请先登录"
	case errors.Is(err, ErrVideoNotFound), errors.Is(err, ErrSessionNotFound):
		status, code, message = http.StatusNotFound, response.CodeNotFound, "视频或观看会话不存在"
	case errors.Is(err, ErrSessionExpired):
		status, code, message = http.StatusConflict, response.CodeSessionExpired, "观看会话已过期"
	case errors.Is(err, ErrHeartbeatInvalid), errors.Is(err, ErrPaginationInvalid):
		status, code, message = http.StatusBadRequest, response.CodeInvalidParameter, "观看参数无效"
	}
	if status >= 500 && h.log != nil {
		h.log.Error("analytics request failed", "error", err)
	}
	response.Error(c, status, code, message)
}
