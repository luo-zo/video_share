package session

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"video_share/internal/middleware"
	"video_share/internal/response"
	"video_share/internal/user"
)

const (
	RefreshCookieName = "video_share_refresh"
	CSRFCookieName    = "video_share_csrf"
)

type Handler struct {
	auth         *user.Service
	sessions     *Service
	log          *slog.Logger
	refreshTTL   time.Duration
	csrfTTL      time.Duration
	cookieSecure bool
}

func NewHandler(auth *user.Service, sessions *Service, refreshTTL time.Duration, cookieSecure bool, log *slog.Logger) *Handler {
	if refreshTTL <= 0 {
		refreshTTL = 30 * 24 * time.Hour
	}
	return &Handler{auth: auth, sessions: sessions, refreshTTL: refreshTTL, csrfTTL: refreshTTL, cookieSecure: cookieSecure, log: log}
}

func (h *Handler) CSRF(c *gin.Context) {
	token, err := newCSRFToken()
	if err != nil {
		if h.log != nil {
			h.log.Error("generate csrf token", "error", err)
		}
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "internal server error")
		return
	}
	h.setCSRFCookie(c, token)
	response.OK(c, http.StatusOK, gin.H{"csrf_token": token})
}

func (h *Handler) Login(c *gin.Context) {
	var req LoginRequest
	if !bindJSON(c, &req) {
		return
	}
	u, err := h.auth.Authenticate(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		if errors.Is(err, user.ErrInvalidCredentials) {
			response.Error(c, http.StatusUnauthorized, response.CodeInvalidCredentials, "用户名或密码错误")
			return
		}
		if h.log != nil {
			h.log.Error("session login failed", "error", err)
		}
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "internal server error")
		return
	}
	issued, err := h.sessions.Issue(c.Request.Context(), u.ID)
	if err != nil {
		if h.log != nil {
			h.log.Error("issue session failed", "error", err, "user_id", u.ID)
		}
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "internal server error")
		return
	}
	h.setSessionCookies(c, issued.RefreshToken)
	response.OK(c, http.StatusOK, user.NewLoginResponse(u, issued.AccessToken, issued.ExpiresIn))
}

func (h *Handler) Refresh(c *gin.Context) {
	refresh, err := c.Cookie(RefreshCookieName)
	if err != nil || refresh == "" {
		response.Error(c, http.StatusUnauthorized, response.CodeSessionExpired, "登录状态已失效，请重新登录")
		return
	}
	issued, err := h.sessions.Refresh(c.Request.Context(), refresh)
	if err != nil {
		switch {
		case errors.Is(err, ErrRefreshConflict):
			response.Error(c, http.StatusConflict, response.CodeRefreshConflict, "刷新请求冲突，请稍后重试")
		case errors.Is(err, ErrSessionReused):
			h.clearSessionCookies(c)
			response.Error(c, http.StatusUnauthorized, response.CodeSessionReused, "登录状态已失效，请重新登录")
		case errors.Is(err, ErrSessionExpired), errors.Is(err, ErrSessionRevoked), errors.Is(err, ErrRefreshInvalid):
			h.clearSessionCookies(c)
			response.Error(c, http.StatusUnauthorized, response.CodeSessionExpired, "登录状态已失效，请重新登录")
		default:
			if h.log != nil {
				h.log.Error("refresh session failed", "error", err)
			}
			response.Error(c, http.StatusInternalServerError, response.CodeInternal, "internal server error")
		}
		return
	}
	h.setSessionCookies(c, issued.RefreshToken)
	response.OK(c, http.StatusOK, gin.H{"access_token": issued.AccessToken, "token_type": "Bearer", "expires_in": int64(issued.ExpiresIn.Seconds())})
}

func (h *Handler) Logout(c *gin.Context) {
	familyID := c.GetString(middleware.SessionIDKey)
	if familyID != "" {
		if err := h.sessions.Logout(c.Request.Context(), familyID); err != nil {
			if h.log != nil {
				h.log.Error("logout session failed", "error", err)
			}
			response.Error(c, http.StatusInternalServerError, response.CodeInternal, "internal server error")
			return
		}
	} else if refresh, err := c.Cookie(RefreshCookieName); err == nil && refresh != "" {
		// An expired access JWT must not prevent logout. The HttpOnly refresh
		// cookie is sufficient to identify and revoke the complete family.
		if err := h.sessions.LogoutByRefreshToken(c.Request.Context(), refresh); err != nil {
			if h.log != nil {
				h.log.Error("logout by refresh token failed", "error", err)
			}
			response.Error(c, http.StatusInternalServerError, response.CodeInternal, "internal server error")
			return
		}
	}
	h.clearSessionCookies(c)
	c.Status(http.StatusNoContent)
}

func (h *Handler) UpdateProfile(c *gin.Context) {
	var req ProfilePatchRequest
	if !bindJSON(c, &req) {
		return
	}
	u, err := h.auth.UpdateProfile(c.Request.Context(), c.GetUint64(middleware.UserIDKey), req.Nickname, req.Bio)
	if err != nil {
		switch {
		case errors.Is(err, user.ErrNicknameInvalid), errors.Is(err, user.ErrBioInvalid):
			response.Error(c, http.StatusBadRequest, response.CodeInvalidParameter, "资料格式不符合要求")
		case errors.Is(err, user.ErrNotFound), errors.Is(err, user.ErrAccountDisabled):
			response.Error(c, http.StatusUnauthorized, response.CodeUnauthorized, "账号已失效")
		default:
			if h.log != nil {
				h.log.Error("update profile failed", "error", err)
			}
			response.Error(c, http.StatusInternalServerError, response.CodeInternal, "internal server error")
		}
		return
	}
	response.OK(c, http.StatusOK, user.Response(u))
}

func (h *Handler) ChangePassword(c *gin.Context) {
	var req ChangePasswordRequest
	if !bindJSON(c, &req) {
		return
	}
	err := h.auth.ChangePassword(c.Request.Context(), c.GetUint64(middleware.UserIDKey), req.OldPassword, req.NewPassword)
	if err != nil {
		switch {
		case errors.Is(err, user.ErrInvalidCredentials):
			response.Error(c, http.StatusUnauthorized, response.CodeInvalidCredentials, "旧密码不正确")
		case errors.Is(err, user.ErrPasswordInvalid):
			response.Error(c, http.StatusBadRequest, response.CodeInvalidParameter, "密码须为 8 个字符以上且不超过 72 字节")
		default:
			if h.log != nil {
				h.log.Error("change password failed", "error", err)
			}
			response.Error(c, http.StatusInternalServerError, response.CodeInternal, "internal server error")
		}
		return
	}
	h.clearSessionCookies(c)
	response.OK(c, http.StatusOK, gin.H{})
}

func (h *Handler) setSessionCookies(c *gin.Context, refresh string) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(RefreshCookieName, refresh, int(h.refreshTTL.Seconds()), "/api/v1/auth", "", h.cookieSecure, true)
	token, err := newCSRFToken()
	if err == nil {
		h.setCSRFCookie(c, token)
	}
}

func (h *Handler) setCSRFCookie(c *gin.Context, token string) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(CSRFCookieName, token, int(h.csrfTTL.Seconds()), "/", "", h.cookieSecure, false)
}

func (h *Handler) clearSessionCookies(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(RefreshCookieName, "", -1, "/api/v1/auth", "", h.cookieSecure, true)
	c.SetCookie(CSRFCookieName, "", -1, "/", "", h.cookieSecure, false)
}

func newCSRFToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func bindJSON(c *gin.Context, dst any) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeInvalidParameter, "invalid request body")
		return false
	}
	return true
}
