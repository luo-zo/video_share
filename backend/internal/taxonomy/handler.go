package taxonomy

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"video_share/internal/response"
)

type Handler struct {
	svc *Service
	log *slog.Logger
}

func NewHandler(svc *Service, log *slog.Logger) *Handler { return &Handler{svc: svc, log: log} }

func (h *Handler) ListCategories(c *gin.Context) {
	result, err := h.svc.ListCategories(c.Request.Context())
	if err != nil {
		if h.log != nil {
			h.log.Error("list categories failed", "error", err)
		}
		response.Error(c, http.StatusServiceUnavailable, response.CodeServiceUnavailable, "分类暂时不可用")
		return
	}
	response.OK(c, http.StatusOK, result)
}

func ErrorStatus(err error) (int, string) {
	switch {
	case errors.Is(err, ErrCategoryNotFound):
		return http.StatusBadRequest, "分类不存在或已停用"
	case errors.Is(err, ErrTooManyTags):
		return http.StatusBadRequest, "标签最多选择 5 个"
	case errors.Is(err, ErrTagInvalid):
		return http.StatusBadRequest, "标签需为 1–20 个字符"
	default:
		return http.StatusInternalServerError, "internal server error"
	}
}
