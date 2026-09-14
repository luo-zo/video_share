package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pressly/goose/v3"

	"video_share/internal/config"
	"video_share/internal/database"
	"video_share/internal/messaging"
	"video_share/internal/outbox"
	"video_share/internal/server"
	"video_share/internal/storage"
	"video_share/internal/token"
)

func main() {
	log := newLogger()

	cfg, err := config.Load()
	if err != nil {
		log.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	// 迁移子命令：`api migrate up|down|status`。
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		runMigrate(cfg, log, os.Args[2:])
		return
	}

	db, err := database.Open(cfg, database.NewGormLogger(log))
	if err != nil {
		log.Error("connect mysql", "error", err)
		os.Exit(1)
	}

	// 启动检查：如果数据库不可达则快速失败。
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := database.Ping(ctx, db); err != nil {
		cancel()
		log.Error("mysql ping", "error", err)
		os.Exit(1)
	}
	cancel()

	publisher, err := messaging.NewKafkaPublisher(cfg.KafkaBrokers, cfg.KafkaClientID)
	if err != nil {
		log.Error("create Kafka publisher", "error", err)
		os.Exit(1)
	}
	defer publisher.Close()
	appCtx, stopBackground := context.WithCancel(context.Background())
	defer stopBackground()
	dispatcher := outbox.NewDispatcher(outbox.NewRepository(db), publisher, cfg.OutboxPollInterval, 50, log)
	go dispatcher.Run(appCtx)

	objectStore, err := storage.NewClient(cfg)
	if err != nil {
		log.Error("create object storage client", "error", err)
		os.Exit(1)
	}
	storageCtx, storageCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := objectStore.EnsureBucket(storageCtx); err != nil {
		storageCancel()
		log.Error("prepare object storage", "error", err)
		os.Exit(1)
	}
	storageCancel()

	tm := token.NewManager(cfg.JWTSecret, cfg.JWTIssuer, cfg.JWTTTL)
	router := server.NewRouter(cfg, db, log, tm, objectStore)

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Info("server listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info("shutting down")
	stopBackground()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("graceful shutdown", "error", err)
	}

	if sqlDB, err := db.DB(); err == nil {
		_ = sqlDB.Close()
	}
	log.Info("server stopped")
}

func newLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, nil))
}

func runMigrate(cfg *config.Config, log *slog.Logger, args []string) {
	db, err := database.Open(cfg, database.NewGormLogger(log))
	if err != nil {
		log.Error("connect mysql", "error", err)
		os.Exit(1)
	}
	sqlDB, err := db.DB()
	if err != nil {
		log.Error("get sql.DB", "error", err)
		os.Exit(1)
	}
	defer sqlDB.Close()

	action := "up"
	if len(args) > 0 {
		action = args[0]
	}

	migrator, err := database.NewMigrator(sqlDB)
	if err != nil {
		log.Error("create migrator", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()

	switch action {
	case "up":
		var results []*goose.MigrationResult
		results, err = migrator.Up(ctx)
		if err != nil {
			break
		}
		if len(results) == 0 {
			log.Info("migrate ok", "action", action, "applied", 0)
			return
		}
		for _, r := range results {
			log.Info("migrated", "result", r.String())
		}
	case "down":
		var result *goose.MigrationResult
		result, err = migrator.Down(ctx)
		if err == nil {
			log.Info("migrated", "result", result.String())
		}
	case "status":
		var statuses []*goose.MigrationStatus
		statuses, err = migrator.Status(ctx)
		if err != nil {
			break
		}
		for _, s := range statuses {
			log.Info("migration status",
				"version", s.Source.Version,
				"path", s.Source.Path,
				"state", string(s.State),
				"applied_at", s.AppliedAt,
			)
		}
	default:
		log.Error("unknown migrate action", "action", action)
		os.Exit(1)
	}
	if err != nil {
		log.Error("migrate failed", "action", action, "error", err)
		os.Exit(1)
	}
	log.Info("migrate ok", "action", action)
}
