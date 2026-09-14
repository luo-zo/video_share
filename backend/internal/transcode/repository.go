package transcode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"video_share/internal/messaging"
	"video_share/internal/outbox"
	"video_share/internal/video"
)

var (
	ErrJobNotFound         = errors.New("transcode job not found")
	ErrJobAlreadyProcessed = errors.New("transcode job already processed")
	ErrJobBusy             = errors.New("transcode job is already running")
)

type JobRepository interface {
	Claim(ctx context.Context, jobID string, videoID uint64, workerID string, staleAfter time.Duration) (*Job, error)
	UpdateProgress(ctx context.Context, job *Job, progress uint8) error
	Succeed(ctx context.Context, job *Job, output Output) error
	Fail(ctx context.Context, job *Job, event messaging.TranscodeRequested, cause error, retryBase time.Duration) (terminal bool, err error)
}

type GormJobRepository struct {
	db    *gorm.DB
	topic string
}

func NewJobRepository(db *gorm.DB, topic string) *GormJobRepository {
	return &GormJobRepository{db: db, topic: topic}
}

func (r *GormJobRepository) Claim(ctx context.Context, jobID string, videoID uint64, workerID string, staleAfter time.Duration) (*Job, error) {
	now := time.Now().UTC()
	staleBefore := now.Add(-staleAfter)
	result := r.db.WithContext(ctx).Model(&Job{}).
		Where("job_id = ? AND video_id = ? AND attempts < max_attempts AND ((status IN ? AND next_retry_at <= ?) OR (status = ? AND (started_at < ? OR worker_id = ?)))",
			jobID, videoID, []JobStatus{JobPending, JobQueued}, now, JobRunning, staleBefore, workerID).
		Updates(map[string]any{
			"status":     JobRunning,
			"attempts":   gorm.Expr("attempts + 1"),
			"worker_id":  workerID,
			"started_at": now,
			"last_error": nil,
		})
	if result.Error != nil {
		return nil, fmt.Errorf("claim transcode job: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		var existing Job
		if err := r.db.WithContext(ctx).Where("job_id = ? AND video_id = ?", jobID, videoID).Take(&existing).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrJobNotFound
		} else if err != nil {
			return nil, fmt.Errorf("load unclaimed transcode job: %w", err)
		}
		if existing.Status == JobSucceeded || existing.Status == JobFailed {
			return nil, ErrJobAlreadyProcessed
		}
		return nil, ErrJobBusy
	}
	var job Job
	if err := r.db.WithContext(ctx).Where("job_id = ?", jobID).Take(&job).Error; err != nil {
		return nil, fmt.Errorf("load claimed transcode job: %w", err)
	}
	return &job, nil
}

func (r *GormJobRepository) UpdateProgress(ctx context.Context, job *Job, progress uint8) error {
	if progress > 99 {
		progress = 99
	}
	result := r.db.WithContext(ctx).Model(&video.Video{}).
		Where("id = ? AND status = ?", job.VideoID, video.StatusProcessing).
		Update("processing_progress", progress)
	if result.Error != nil {
		return fmt.Errorf("update transcode progress: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return video.ErrStateConflict
	}
	return nil
}

func (r *GormJobRepository) Succeed(ctx context.Context, job *Job, output Output) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		jobResult := tx.Model(&Job{}).
			Where("id = ? AND status = ?", job.ID, JobRunning).
			Updates(map[string]any{"status": JobSucceeded, "finished_at": now, "last_error": nil})
		if jobResult.Error != nil {
			return fmt.Errorf("mark transcode job succeeded: %w", jobResult.Error)
		}
		if jobResult.RowsAffected == 0 {
			return video.ErrStateConflict
		}
		videoResult := tx.Model(&video.Video{}).
			Where("id = ? AND status = ?", job.VideoID, video.StatusProcessing).
			Updates(map[string]any{
				"status":              video.StatusReady,
				"hls_master_key":      output.HLSMasterKey,
				"cover_object_key":    output.CoverObjectKey,
				"duration_ms":         output.DurationMS,
				"width":               output.Width,
				"height":              output.Height,
				"processing_progress": 100,
				"processing_error":    nil,
				"processed_at":        now,
			})
		if videoResult.Error != nil {
			return fmt.Errorf("publish processed video: %w", videoResult.Error)
		}
		if videoResult.RowsAffected == 0 {
			return video.ErrStateConflict
		}
		return nil
	})
}

func (r *GormJobRepository) Fail(ctx context.Context, job *Job, event messaging.TranscodeRequested, cause error, retryBase time.Duration) (bool, error) {
	message := safeError(cause)
	now := time.Now().UTC()
	terminal := job.Attempts >= job.MaxAttempts
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if terminal {
			result := tx.Model(&Job{}).Where("id = ? AND status = ?", job.ID, JobRunning).
				Updates(map[string]any{"status": JobFailed, "last_error": message, "finished_at": now})
			if result.Error != nil {
				return fmt.Errorf("mark transcode job failed: %w", result.Error)
			}
			if result.RowsAffected == 0 {
				return video.ErrStateConflict
			}
			if err := tx.Model(&video.Video{}).Where("id = ? AND status = ?", job.VideoID, video.StatusProcessing).
				Updates(map[string]any{"status": video.StatusFailed, "processing_error": message, "processing_progress": 0}).Error; err != nil {
				return fmt.Errorf("mark video processing failed: %w", err)
			}
			return nil
		}

		retryAt := now.Add(retryDelayForAttempt(retryBase, job.Attempts))
		result := tx.Model(&Job{}).Where("id = ? AND status = ?", job.ID, JobRunning).
			Updates(map[string]any{
				"status":        JobPending,
				"next_retry_at": retryAt,
				"worker_id":     nil,
				"last_error":    message,
			})
		if result.Error != nil {
			return fmt.Errorf("schedule transcode retry: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return video.ErrStateConflict
		}

		event.EventID = uuid.NewString()
		event.RequestedAt = now
		payload, err := json.Marshal(event)
		if err != nil {
			return fmt.Errorf("marshal transcode retry event: %w", err)
		}
		outboxEvent := outbox.Event{
			EventID:     event.EventID,
			Topic:       r.topic,
			EventKey:    fmt.Sprint(event.VideoID),
			EventType:   messaging.TranscodeRequestedType,
			Payload:     payload,
			Status:      outbox.StatusPending,
			NextRetryAt: retryAt,
		}
		if err := tx.Create(&outboxEvent).Error; err != nil {
			return fmt.Errorf("create transcode retry outbox event: %w", err)
		}
		return nil
	})
	return terminal, err
}

func retryDelayForAttempt(base time.Duration, attempt uint) time.Duration {
	if base <= 0 {
		base = time.Second
	}
	shift := uint(0)
	if attempt > 1 {
		shift = min(attempt-1, 6)
	}
	return base * time.Duration(1<<shift)
}

func safeError(err error) string {
	if err == nil {
		return "transcode failed"
	}
	text := []rune(err.Error())
	if len(text) > 1000 {
		text = text[:1000]
	}
	return string(text)
}
