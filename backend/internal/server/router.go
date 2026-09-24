package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"video_share/internal/analytics"
	"video_share/internal/cache"
	"video_share/internal/config"
	"video_share/internal/creator"
	"video_share/internal/database"
	"video_share/internal/engagement"
	"video_share/internal/follow"
	"video_share/internal/middleware"
	"video_share/internal/moderation"
	"video_share/internal/notification"
	"video_share/internal/ranking"
	"video_share/internal/response"
	"video_share/internal/session"
	"video_share/internal/taxonomy"
	"video_share/internal/token"
	"video_share/internal/user"
	"video_share/internal/video"
)

// maxJSONBodyBytes 限制认证与创建投稿的 JSON 元数据大小。视频二进制直接
// 上传到对象存储，不经过这些端点。
const maxJSONBodyBytes = 32 * 1024 // 32 KiB

// NewRouter 将中间件、依赖和路由装配进一个 *gin.Engine。
func NewRouter(cfg *config.Config, db *gorm.DB, log *slog.Logger, tm *token.Manager, objectStore video.ObjectStore, redisClient *cache.Client) *gin.Engine {
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
	sessionRepo := session.NewRepository(db)
	sessionSvc := session.NewService(sessionRepo, tm, session.Options{
		FamilyTTL:      cfg.SessionFamilyTTL,
		RefreshTTL:     cfg.RefreshTokenTTL,
		ConflictWindow: cfg.RefreshConflictWindow,
	})
	sessionHandler := session.NewHandler(userSvc, sessionSvc, cfg.RefreshTokenTTL, cfg.CookieSecure, log)
	taxonomyService := taxonomy.NewService(taxonomy.NewRepository(db), cache.NewCategoriesCache(redisClient))
	videoRepo := video.NewRepository(db, video.WithProcessingQueue(cfg.KafkaTranscodeTopic, uint(cfg.TranscodeMaxAttempts)))
	videoSvc := video.NewService(videoRepo, objectStore, cfg.MaxVideoBytes, cfg.UploadExpiry, cfg.PlayExpiry, taxonomyService)
	videoHandler := video.NewHandler(videoSvc, log)
	taxonomyHandler := taxonomy.NewHandler(taxonomyService, log)
	engagementHandler := engagement.NewHandler(engagement.NewService(engagement.NewRepository(db)), log)
	followService := follow.NewService(follow.NewRepository(db))
	followHandler := follow.NewHandler(followService, log)
	notificationHandler := notification.NewHandler(notification.NewService(notification.NewRepository(db)), log)
	creatorHandler := creator.NewHandler(creator.NewService(creator.NewRepository(db), videoRepo, followService), log)
	moderationService := moderation.NewService(moderation.NewRepository(db))
	moderationHandler := moderation.NewHandler(moderationService, log)
	analyticsHandler := analytics.NewHandler(analytics.NewService(analytics.NewRepository(db)), log)
	rankingHandler := ranking.NewHandler(ranking.NewService(ranking.NewRepository(db, redisClient)), log)

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/readyz", readinessHandler(db, log))

	loginLimiter := middleware.NewRedisRateLimiter(redisClient, middleware.RateRule{
		Name:        "login",
		Limit:       cfg.LoginRateLimit,
		Window:      cfg.LoginRateWindow,
		Unavailable: middleware.RejectUnavailable,
	}, nil, log)
	registerLimiter := middleware.NewRedisRateLimiter(redisClient, middleware.RateRule{
		Name:        "register",
		Limit:       cfg.RegisterRateLimit,
		Window:      cfg.RegisterRateWindow,
		Unavailable: middleware.RejectUnavailable,
	}, nil, log)
	commentFallback := middleware.NewRateLimiter(localRatePerSecond(cfg.CommentRateLimit, cfg.CommentRateWindow), positiveBurst(cfg.CommentRateLimit), 10000)
	commentLimiter := middleware.NewRedisRateLimiter(redisClient, middleware.RateRule{
		Name:        "comment",
		Limit:       cfg.CommentRateLimit,
		Window:      cfg.CommentRateWindow,
		Unavailable: middleware.FallbackToLocal,
	}, commentFallback, log)
	reportFallback := middleware.NewRateLimiter(localRatePerSecond(cfg.ReportRateLimit, cfg.ReportRateWindow), positiveBurst(cfg.ReportRateLimit), 10000)
	reportLimiter := middleware.NewRedisRateLimiter(redisClient, middleware.RateRule{
		Name:        "report",
		Limit:       cfg.ReportRateLimit,
		Window:      cfg.ReportRateWindow,
		Unavailable: middleware.FallbackToLocal,
	}, reportFallback, log)
	validateUser := func(ctx context.Context, userID uint64) (bool, error) {
		u, err := userRepo.FindByID(ctx, userID)
		if errors.Is(err, user.ErrNotFound) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return u.Status == user.StatusNormal, nil
	}
	requireAuth := middleware.AuthWithSession(tm, validateUser, sessionSvc.ValidateAccess)
	requireAdmin := middleware.RequireAdmin(func(ctx context.Context, id uint64) (string, bool, error) {
		account, err := userRepo.FindByID(ctx, id)
		if errors.Is(err, user.ErrNotFound) {
			return "", false, nil
		}
		if err != nil {
			return "", false, err
		}
		return account.Role, account.Status == user.StatusNormal, nil
	})
	appOrigin := cfg.AppOrigin
	if appOrigin == "" {
		// Tests that assemble a router directly may use a zero-value config;
		// production configuration is validated before this point.
		appOrigin = "http://127.0.0.1:5173"
	}

	api := r.Group("/api/v1")
	auth := api.Group("/auth")
	bodyLimit := middleware.LimitBody(maxJSONBodyBytes)
	auth.GET("/csrf", sessionHandler.CSRF)
	auth.POST("/register", registerLimiter.Middleware(nil), bodyLimit, userHandler.Register)
	auth.POST("/login", loginLimiter.Middleware(nil), middleware.RequireSameOriginCSRF(appOrigin), bodyLimit, sessionHandler.Login)
	auth.POST("/refresh", middleware.RequireSameOriginCSRF(appOrigin), bodyLimit, sessionHandler.Refresh)
	// Logout is intentionally not behind access-token auth. If the short-lived
	// access JWT expired, the HttpOnly refresh cookie still identifies the
	// family that must be revoked.
	auth.POST("/logout", middleware.OptionalAuthWithSession(tm, validateUser, sessionSvc.ValidateAccess), middleware.RequireSameOriginCSRF(appOrigin), bodyLimit, sessionHandler.Logout)

	api.GET("/users/me", requireAuth, userHandler.Me)
	api.PATCH("/users/me", requireAuth, middleware.RequireSameOriginCSRF(appOrigin), bodyLimit, sessionHandler.UpdateProfile)
	api.POST("/users/me/change-password", requireAuth, middleware.RequireSameOriginCSRF(appOrigin), bodyLimit, sessionHandler.ChangePassword)
	reportSubject := func(c *gin.Context) string {
		userID, ok := c.Get(middleware.UserIDKey)
		if !ok {
			return ""
		}
		id, ok := userID.(uint64)
		if !ok {
			return ""
		}
		return "user:" + strconv.FormatUint(id, 10)
	}
	api.POST("/reports", requireAuth, reportLimiter.Middleware(reportSubject), middleware.RequireSameOriginCSRF(appOrigin), bodyLimit, moderationHandler.CreateReport)
	api.GET("/users/me/reports", requireAuth, moderationHandler.ListMyReports)

	// Public creator pages expose only normal-user projections and ready/public
	// videos. The profile uses optional auth solely to compute the viewer's
	// following state; anonymous requests remain first-class.
	api.GET("/users/:id", middleware.OptionalAuthWithSession(tm, validateUser, sessionSvc.ValidateAccess), creatorHandler.Profile)
	api.GET("/users/:id/videos", creatorHandler.Videos)
	api.GET("/users/:id/followers", creatorHandler.Followers)
	api.GET("/users/:id/follows", creatorHandler.Following)
	api.GET("/categories", taxonomyHandler.ListCategories)

	// 公开读取端点：详情页使用 OptionalAuth，匿名访问时互动状态返回 false。
	api.GET("/videos", videoHandler.List)
	// Keep the static ranking path before /videos/:id so "ranking" is never
	// parsed as a numeric video id by a future route change.
	api.GET("/videos/ranking", rankingHandler.List)
	api.GET("/videos/:id/cover", videoHandler.Cover)
	api.GET("/videos/:id/hls/*path", videoHandler.HLS)
	api.GET("/videos/:id/related", videoHandler.Related)
	api.GET("/videos/:id", middleware.OptionalAuthWithSession(tm, validateUser, sessionSvc.ValidateAccess), videoHandler.Detail)
	api.GET("/videos/:id/comments", engagementHandler.ListComments)
	api.GET("/comments/:id/replies", engagementHandler.ListReplies)

	// 需要登录的写端点。
	api.POST("/videos", requireAuth, bodyLimit, videoHandler.Create)
	api.POST("/videos/:id/complete", requireAuth, videoHandler.Complete)
	api.POST("/videos/:id/comments", requireAuth, commentLimiter.Middleware(func(c *gin.Context) string {
		userID, ok := c.Get(middleware.UserIDKey)
		if !ok {
			return ""
		}
		id, ok := userID.(uint64)
		if !ok {
			return ""
		}
		return "user:" + strconv.FormatUint(id, 10)
	}), middleware.RequireSameOriginCSRF(appOrigin), bodyLimit, engagementHandler.CreateComment)
	api.DELETE("/comments/:id", requireAuth, engagementHandler.DeleteComment)
	api.GET("/notifications", requireAuth, notificationHandler.List)
	api.PATCH("/notifications/:id/read", requireAuth, middleware.RequireSameOriginCSRF(appOrigin), bodyLimit, notificationHandler.MarkRead)
	api.POST("/notifications/read-all", requireAuth, middleware.RequireSameOriginCSRF(appOrigin), bodyLimit, notificationHandler.MarkAllRead)
	api.PUT("/videos/:id/like", requireAuth, engagementHandler.Like)
	api.DELETE("/videos/:id/like", requireAuth, engagementHandler.Unlike)
	api.PUT("/videos/:id/favorite", requireAuth, engagementHandler.Favorite)
	api.DELETE("/videos/:id/favorite", requireAuth, engagementHandler.Unfavorite)
	api.POST("/videos/:id/watch", requireAuth, bodyLimit, engagementHandler.RecordWatch)
	api.POST("/videos/:id/watch-sessions", requireAuth, bodyLimit, analyticsHandler.StartSession)
	api.POST("/watch-sessions/:id/heartbeat", requireAuth, bodyLimit, analyticsHandler.Heartbeat)

	api.GET("/users/me/videos", requireAuth, videoHandler.Mine)
	api.GET("/users/me/videos/:id", requireAuth, videoHandler.MineDetail)
	api.PATCH("/users/me/videos/:id", requireAuth, bodyLimit, videoHandler.Update)
	api.DELETE("/users/me/videos/:id", requireAuth, videoHandler.Delete)
	api.GET("/users/me/favorites", requireAuth, engagementHandler.Favorites)
	api.GET("/users/me/history", requireAuth, engagementHandler.History)
	api.GET("/users/me/follows", requireAuth, followHandler.ListFollows)
	api.GET("/feed/following", requireAuth, videoHandler.Following)
	api.GET("/users/me/creator/summary", requireAuth, analyticsHandler.CreatorSummary)

	api.PUT("/users/:id/follow", requireAuth, middleware.RequireSameOriginCSRF(appOrigin), followHandler.Follow)
	api.DELETE("/users/:id/follow", requireAuth, middleware.RequireSameOriginCSRF(appOrigin), followHandler.Unfollow)

	admin := api.Group("/admin", requireAuth, requireAdmin)
	admin.GET("/reports", moderationHandler.ListAdminReports)
	admin.PATCH("/reports/:id/assign", middleware.RequireSameOriginCSRF(appOrigin), moderationHandler.AssignReport)
	admin.PATCH("/reports/:id", middleware.RequireSameOriginCSRF(appOrigin), bodyLimit, moderationHandler.DecideReport)
	admin.GET("/moderation-actions", moderationHandler.ListActions)
	admin.POST("/videos/:id/:action", middleware.RequireSameOriginCSRF(appOrigin), bodyLimit, moderationHandler.ModerateVideo)
	admin.POST("/comments/:id/:action", middleware.RequireSameOriginCSRF(appOrigin), bodyLimit, moderationHandler.ModerateComment)
	admin.POST("/users/:id/:action", middleware.RequireSameOriginCSRF(appOrigin), bodyLimit, moderationHandler.ModerateUser)

	return r
}

func localRatePerSecond(limit int, window time.Duration) float64 {
	if limit <= 0 || window <= 0 {
		return 1
	}
	return float64(limit) / window.Seconds()
}

func positiveBurst(limit int) int {
	if limit <= 0 {
		return 1
	}
	return limit
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
