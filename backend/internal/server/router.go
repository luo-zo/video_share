package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"video_share/internal/config"
	"video_share/internal/database"
	"video_share/internal/middleware"
	"video_share/internal/response"
	"video_share/internal/token"
	"video_share/internal/user"
	"video_share/internal/video"
)

// maxJSONBodyBytes 限制认证与创建投稿的 JSON 元数据大小。视频二进制直接
// 上传到对象存储，不经过这些端点。
const maxJSONBodyBytes = 32 * 1024 // 32 KiB

// NewRouter 将中间件、依赖和路由装配进一个 *gin.Engine。
func NewRouter(cfg *config.Config, db *gorm.DB, log *slog.Logger, tm *token.Manager, objectStore video.ObjectStore) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()

	// 默认不信任任何代理；可通过 TRUSTED_PROXIES 配置。
	if err := r.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		log.Warn("set trusted proxies failed", "error", err)
	}

	r.Use(middleware.RequestID())
	r.Use(middleware.AccessLog(log))
	r.Use(middleware.Recovery(log))

	userRepo := user.NewRepository(db)
	userSvc := user.NewService(userRepo, tm)
	userHandler := user.NewHandler(userSvc, log)
	videoRepo := video.NewRepository(db, video.WithProcessingQueue(cfg.KafkaTranscodeTopic, uint(cfg.TranscodeMaxAttempts)))
	videoSvc := video.NewService(videoRepo, objectStore, cfg.MaxVideoBytes, cfg.UploadExpiry, cfg.PlayExpiry)
	videoHandler := video.NewHandler(videoSvc, log)

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/readyz", readinessHandler(db, log))

	rateLimiter := middleware.NewRateLimiter(cfg.RateLimitPerSecond, cfg.RateLimitBurst, 10000)

	api := r.Group("/api/v1")
	auth := api.Group("/auth")
	bodyLimit := middleware.LimitBody(maxJSONBodyBytes)
	auth.POST("/register", rateLimiter.Middleware(), bodyLimit, userHandler.Register)
	auth.POST("/login", rateLimiter.Middleware(), bodyLimit, userHandler.Login)

	api.GET("/users/me", middleware.Auth(tm), userHandler.Me)
	api.GET("/users/me/videos", middleware.Auth(tm), videoHandler.Mine)
	api.GET("/users/me/videos/:id", middleware.Auth(tm), videoHandler.MineDetail)

	api.GET("/videos", videoHandler.List)
	api.GET("/videos/:id/cover", videoHandler.Cover)
	api.GET("/videos/:id/hls/*path", videoHandler.HLS)
	api.GET("/videos/:id", videoHandler.Detail)
	api.POST("/videos", middleware.Auth(tm), bodyLimit, videoHandler.Create)
	api.POST("/videos/:id/complete", middleware.Auth(tm), videoHandler.Complete)

	return r
}

func readinessHandler(db *gorm.DB, log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := database.Ping(ctx, db); err != nil {
			log.Warn("readiness check failed", "error", err)
			response.Error(c, http.StatusServiceUnavailable, response.CodeServiceUnavailable, "database unavailable")
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	}
}
