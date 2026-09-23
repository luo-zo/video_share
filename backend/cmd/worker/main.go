package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"video_share/internal/config"
	"video_share/internal/database"
	"video_share/internal/messaging"
	"video_share/internal/storage"
	"video_share/internal/transcode"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("worker stopped", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	db, err := database.Open(cfg, database.NewGormLogger(log))
	if err != nil {
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := database.Ping(pingCtx, db); err != nil {
		return err
	}

	objectStore, err := storage.NewClient(cfg)
	if err != nil {
		return err
	}
	if err := objectStore.EnsureBucket(pingCtx); err != nil {
		return err
	}
	consumer, err := messaging.NewKafkaConsumer(cfg.KafkaBrokers, cfg.KafkaClientID, cfg.KafkaConsumerGroup, cfg.KafkaTranscodeTopic)
	if err != nil {
		return err
	}
	defer consumer.Close()
	if err := consumer.Ping(pingCtx); err != nil {
		return err
	}

	jobs := transcode.NewJobRepository(db, cfg.KafkaTranscodeTopic)
	service := transcode.NewService(
		jobs,
		objectStore,
		transcode.FFprobe{Path: cfg.FFprobePath},
		transcode.FFmpegRunner{Path: cfg.FFmpegPath},
		cfg.WorkerID,
		cfg.TranscodeTempDir,
		cfg.TranscodeTimeout,
		cfg.TranscodeRetryBase,
		log,
	)
	log.Info("transcode worker started", "worker_id", cfg.WorkerID, "topic", cfg.KafkaTranscodeTopic)
	return consume(ctx, log, consumer, service)
}
