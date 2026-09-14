package transcode

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"video_share/internal/messaging"
)

type MediaStore interface {
	Download(ctx context.Context, key, destination string) error
	UploadFile(ctx context.Context, key, source, contentType string) error
}

type Service struct {
	jobs      JobRepository
	store     MediaStore
	prober    Prober
	runner    Runner
	workerID  string
	tempRoot  string
	timeout   time.Duration
	retryBase time.Duration
	log       *slog.Logger
}

func NewService(jobs JobRepository, store MediaStore, prober Prober, runner Runner, workerID, tempRoot string, timeout, retryBase time.Duration, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{jobs: jobs, store: store, prober: prober, runner: runner, workerID: workerID, tempRoot: tempRoot, timeout: timeout, retryBase: retryBase, log: log}
}

func (s *Service) Process(parent context.Context, event messaging.TranscodeRequested) error {
	if event.SchemaVersion != messaging.EventSchemaVersion || event.JobID == "" || event.VideoID == 0 || event.SourceObjectKey == "" {
		return fmt.Errorf("invalid transcode event")
	}
	job, err := s.jobs.Claim(parent, event.JobID, event.VideoID, s.workerID, s.timeout+time.Minute)
	if errors.Is(err, ErrJobAlreadyProcessed) {
		return nil
	}
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(parent, s.timeout)
	defer cancel()

	if err := os.MkdirAll(s.tempRoot, 0o755); err != nil {
		return s.recordFailure(ctx, job, event, fmt.Errorf("create transcode temp root: %w", err))
	}
	workDir, err := os.MkdirTemp(s.tempRoot, "video-"+event.JobID+"-")
	if err != nil {
		return s.recordFailure(ctx, job, event, fmt.Errorf("create transcode temp directory: %w", err))
	}
	defer os.RemoveAll(workDir)

	sourcePath := filepath.Join(workDir, "source.mp4")
	if err := s.store.Download(ctx, event.SourceObjectKey, sourcePath); err != nil {
		return s.recordFailure(ctx, job, event, fmt.Errorf("download source: %w", err))
	}
	if err := s.jobs.UpdateProgress(ctx, job, 10); err != nil {
		return s.recordFailure(ctx, job, event, err)
	}

	info, err := s.prober.Probe(ctx, sourcePath)
	if err != nil {
		return s.recordFailure(ctx, job, event, err)
	}
	if err := s.jobs.UpdateProgress(ctx, job, 20); err != nil {
		return s.recordFailure(ctx, job, event, err)
	}

	outputDir := filepath.Join(workDir, "output")
	if err := s.runner.Process(ctx, sourcePath, outputDir, info, SelectProfiles(info)); err != nil {
		return s.recordFailure(ctx, job, event, err)
	}
	if err := s.jobs.UpdateProgress(ctx, job, 75); err != nil {
		return s.recordFailure(ctx, job, event, err)
	}

	base := fmt.Sprintf("videos/%d/%d", event.UserID, event.VideoID)
	masterKey := base + "/processed/v1/master.m3u8"
	coverKey := base + "/cover/v1/cover.jpg"
	if err := filepath.WalkDir(outputDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(outputDir, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		key := base + "/processed/v1/" + relative
		if relative == "cover.jpg" {
			key = coverKey
		}
		return s.store.UploadFile(ctx, key, path, mediaContentType(path))
	}); err != nil {
		return s.recordFailure(ctx, job, event, fmt.Errorf("upload processed outputs: %w", err))
	}
	if err := s.jobs.UpdateProgress(ctx, job, 95); err != nil {
		return s.recordFailure(ctx, job, event, err)
	}

	if err := s.jobs.Succeed(ctx, job, Output{
		HLSMasterKey:   masterKey,
		CoverObjectKey: coverKey,
		DurationMS:     info.DurationMS,
		Width:          uint(info.Width),
		Height:         uint(info.Height),
	}); err != nil {
		return err
	}
	s.log.Info("video transcode succeeded", "video_id", event.VideoID, "job_id", event.JobID, "worker_id", s.workerID)
	return nil
}

func (s *Service) recordFailure(ctx context.Context, job *Job, event messaging.TranscodeRequested, cause error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	terminal, err := s.jobs.Fail(ctx, job, event, cause, s.retryBase)
	if err != nil {
		return errors.Join(cause, err)
	}
	s.log.Warn("video transcode failed", "video_id", event.VideoID, "job_id", event.JobID, "worker_id", s.workerID, "attempt", job.Attempts, "terminal", terminal, "error", safeError(cause))
	return nil
}

func mediaContentType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".m3u8":
		return "application/vnd.apple.mpegurl"
	case ".ts":
		return "video/mp2t"
	case ".m4s":
		return "video/iso.segment"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	default:
		if value := mime.TypeByExtension(filepath.Ext(path)); value != "" {
			return value
		}
		return "application/octet-stream"
	}
}
