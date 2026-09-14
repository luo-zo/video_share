package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"video_share/internal/messaging"
)

type Repository interface {
	ClaimDue(ctx context.Context, limit int, lease time.Duration) ([]Event, error)
	MarkPublished(ctx context.Context, id uint64) error
	MarkFailed(ctx context.Context, id uint64, message string, retryAt time.Time) error
}

type GormRepository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }

func (r *GormRepository) ClaimDue(ctx context.Context, limit int, lease time.Duration) ([]Event, error) {
	if limit < 1 {
		return []Event{}, nil
	}
	var events []Event
	now := time.Now().UTC()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = ? AND next_retry_at <= ?", StatusPending, now).
			Order("id ASC").Limit(limit).Find(&events).Error; err != nil {
			return fmt.Errorf("claim outbox events: %w", err)
		}
		if len(events) == 0 {
			return nil
		}
		ids := make([]uint64, 0, len(events))
		for i := range events {
			ids = append(ids, events[i].ID)
			events[i].Attempts++
		}
		if err := tx.Model(&Event{}).Where("id IN ? AND status = ?", ids, StatusPending).
			Updates(map[string]any{
				"attempts":      gorm.Expr("attempts + 1"),
				"next_retry_at": now.Add(lease),
			}).Error; err != nil {
			return fmt.Errorf("lease outbox events: %w", err)
		}
		return nil
	})
	return events, err
}

func (r *GormRepository) MarkPublished(ctx context.Context, id uint64) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var event Event
		if err := tx.Where("id = ?", id).Take(&event).Error; err != nil {
			return fmt.Errorf("load published outbox event: %w", err)
		}
		result := tx.Model(&Event{}).
			Where("id = ? AND status = ?", id, StatusPending).
			Updates(map[string]any{"status": StatusPublished, "published_at": now, "last_error": nil})
		if result.Error != nil {
			return fmt.Errorf("mark outbox event published: %w", result.Error)
		}
		if event.EventType == messaging.TranscodeRequestedType {
			var message messaging.TranscodeRequested
			if err := json.Unmarshal(event.Payload, &message); err != nil {
				return fmt.Errorf("decode transcode outbox payload: %w", err)
			}
			if err := tx.Table("video_transcode_jobs").Where("job_id = ? AND status = ?", message.JobID, 1).
				Update("status", 2).Error; err != nil {
				return fmt.Errorf("mark transcode job queued: %w", err)
			}
		}
		return nil
	})
}

func (r *GormRepository) MarkFailed(ctx context.Context, id uint64, message string, retryAt time.Time) error {
	message = truncate(message, 1000)
	if err := r.db.WithContext(ctx).Model(&Event{}).
		Where("id = ? AND status = ?", id, StatusPending).
		Updates(map[string]any{"last_error": message, "next_retry_at": retryAt}).Error; err != nil {
		return fmt.Errorf("record outbox failure: %w", err)
	}
	return nil
}

func truncate(value string, limit int) string {
	text := []rune(value)
	if len(text) <= limit {
		return value
	}
	return string(text[:limit])
}
