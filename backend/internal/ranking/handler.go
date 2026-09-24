package ranking

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"video_share/internal/response"
)

type Handler struct {
	svc *Service
	log *slog.Logger
}

func NewHandler(svc *Service, log *slog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

func (h *Handler) List(c *gin.Context) {
	window, err := ParseWindow(c.DefaultQuery("window", string(WindowDay)))
	if err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeInvalidParameter, "榜单周期只能是 day 或 week")
		return
	}
	page, pageSize, ok := rankingPagination(c)
	if !ok {
		return
	}
	result, err := h.svc.List(c.Request.Context(), window, page, pageSize)
	if err != nil {
		if errors.Is(err, ErrPagination) || errors.Is(err, ErrInvalidWindow) {
			response.Error(c, http.StatusBadRequest, response.CodeInvalidParameter, "榜单分页参数无效")
			return
		}
		if errors.Is(err, ErrFallbackBusy) || errors.Is(err, ErrTooManyRows) {
			response.Error(c, http.StatusServiceUnavailable, response.CodeServiceUnavailable, "榜单暂时繁忙，请稍后重试")
			return
		}
		if h.log != nil {
			h.log.Error("list ranking", "error", err, "window", window)
		}
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "榜单暂时不可用")
		return
	}
	response.OK(c, http.StatusOK, result)
}

func rankingPagination(c *gin.Context) (int, int, bool) {
	page := parsePositive(c.Query("page"), 1)
	pageSize := parsePositive(c.Query("page_size"), 20)
	if page < 1 || page > 1_000_000 || pageSize < 1 || pageSize > 50 {
		response.Error(c, http.StatusBadRequest, response.CodeInvalidParameter, "榜单分页参数无效")
		return 0, 0, false
	}
	return page, pageSize, true
}

func parsePositive(value string, fallback int) int {
	if value == "" {
		return fallback
	}
	number, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return number
}
