// Command ranking-rebuild publishes complete day/week ranking snapshots.
// It is safe to run from cron or a single long-lived scheduler; the Redis
// owner/TTL lock prevents overlapping rebuilds from publishing out of order.
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"time"

	"video_share/internal/cache"
	"video_share/internal/config"
	"video_share/internal/database"
	"video_share/internal/ranking"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		log.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	db, err := database.Open(cfg, database.NewGormLogger(log))
	if err != nil {
		log.Error("connect mysql", "error", err)
		os.Exit(1)
	}
	sqlDB, err := db.DB()
	if err != nil {
		log.Error("get sql db", "error", err)
		os.Exit(1)
	}
	defer sqlDB.Close()
	redisClient := cache.NewClient(cache.Options{Addr: cfg.RedisAddr, Password: cfg.RedisPassword, DB: cfg.RedisDB, DialTimeout: cfg.RedisDialTimeout, ReadTimeout: cfg.RedisReadTimeout, WriteTimeout: cfg.RedisWriteTimeout, CommandTimeout: cfg.RedisCommandTimeout})
	defer redisClient.Close()
	service := ranking.NewService(ranking.NewRepository(db, redisClient))

	windows := []ranking.Window{ranking.WindowDay, ranking.WindowWeek}
	if len(os.Args) > 1 {
		window, parseErr := ranking.ParseWindow(os.Args[1])
		if parseErr != nil {
			log.Error("usage", "command", "ranking-rebuild [day|week|all]")
			os.Exit(2)
		}
		windows = []ranking.Window{window}
	}
	runOnce := func() {
		now := time.Now().UTC()
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		for _, window := range windows {
			generatedAt, rebuildErr := service.Rebuild(ctx, window, now)
			if rebuildErr != nil {
				if errors.Is(rebuildErr, ranking.ErrLockBusy) || errors.Is(rebuildErr, ranking.ErrLockLost) {
					log.Warn("ranking rebuild already owned by another process", "window", window)
					continue
				}
				log.Error("ranking rebuild failed", "window", window, "error", rebuildErr)
				os.Exit(1)
			}
			log.Info("ranking rebuilt", "window", window, "generated_at", generatedAt)
		}
	}
	runOnce()
	if os.Getenv("RANKING_REBUILD_ONCE") == "1" {
		return
	}
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		runOnce()
	}
}
