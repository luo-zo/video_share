package video

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"video_share/internal/messaging"
)

type Repository interface {
	Create(ctx context.Context, v *Video) error
	UpdateObjectKey(ctx context.Context, id uint64, objectKey string) error
	FindByID(ctx context.Context, id uint64) (*Video, error)
	BeginProcessing(ctx context.Context, video *Video) error
	MarkReady(ctx context.Context, id uint64) error
	ListReady(ctx context.Context, page, pageSize int) ([]Video, int64, error)
	FindReadyByID(ctx context.Context, id uint64) (*Video, error)
	ListByUser(ctx context.Context, userID uint64, page, pageSize int) ([]Video, int64, error)
}

type gormRepository struct {
	db             *gorm.DB
	transcodeTopic string
	maxAttempts    uint
}

type RepositoryOption func(*gormRepository)

func WithProcessingQueue(topic string, maxAttempts uint) RepositoryOption {
	return func(repository *gormRepository) {
		if topic != "" {
			repository.transcodeTopic = topic
		}
		if maxAttempts > 0 {
			repository.maxAttempts = maxAttempts
		}
	}
}

func NewRepository(db *gorm.DB, options ...RepositoryOption) Repository {
	repository := &gormRepository{db: db, transcodeTopic: "video.transcode.requested", maxAttempts: 3}
	for _, option := range options {
		option(repository)
	}
	return repository
}

func (r *gormRepository) Create(ctx context.Context, v *Video) error {
	if err := r.db.WithContext(ctx).Create(v).Error; err != nil {
		return fmt.Errorf("create video: %w", err)
	}
	return nil
}

func (r *gormRepository) UpdateObjectKey(ctx context.Context, id uint64, objectKey string) error {
	result := r.db.WithContext(ctx).Model(&Video{}).
		Where("id = ?", id).
		Update("object_key", objectKey)
	if result.Error != nil {
		return fmt.Errorf("update video object key: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormRepository) FindByID(ctx context.Context, id uint64) (*Video, error) {
	var v Video
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find video by id: %w", err)
	}
	return &v, nil
}

func (r *gormRepository) BeginProcessing(ctx context.Context, video *Video) error {
	if video == nil || video.ID == 0 {
		return ErrNotFound
	}
	now := time.Now().UTC()
	jobID := uuid.NewString()
	eventID := uuid.NewString()
	event := messaging.TranscodeRequested{
		EventID:         eventID,
		JobID:           jobID,
		VideoID:         video.ID,
		UserID:          video.UserID,
		SourceObjectKey: video.ObjectKey,
		RequestedAt:     now,
		SchemaVersion:   messaging.EventSchemaVersion,
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal transcode event: %w", err)
	}

	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&Video{}).
			Where("id = ? AND status = ?", video.ID, StatusUploading).
			Updates(map[string]any{
				"status":              StatusProcessing,
				"processing_progress": 0,
				"processing_error":    nil,
			})
		if result.Error != nil {
			return fmt.Errorf("mark video processing: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return ErrStateConflict
		}

		job := transcodeJobInsert{
			JobID:          jobID,
			VideoID:        video.ID,
			IdempotencyKey: fmt.Sprintf("video:%d:transcode:v1", video.ID),
			Status:         1,
			MaxAttempts:    r.maxAttempts,
			NextRetryAt:    now,
		}
		if err := tx.Create(&job).Error; err != nil {
			return fmt.Errorf("create transcode job: %w", err)
		}
		outbox := outboxEventInsert{
			EventID:     eventID,
			Topic:       r.transcodeTopic,
			EventKey:    strconv.FormatUint(video.ID, 10),
			EventType:   messaging.TranscodeRequestedType,
			Payload:     payload,
			Status:      1,
			NextRetryAt: now,
		}
		if err := tx.Create(&outbox).Error; err != nil {
			return fmt.Errorf("create outbox event: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

type transcodeJobInsert struct {
	ID             uint64 `gorm:"primaryKey"`
	JobID          string
	VideoID        uint64
	IdempotencyKey string
	Status         uint8
	MaxAttempts    uint
	NextRetryAt    time.Time
}

func (transcodeJobInsert) TableName() string { return "video_transcode_jobs" }

type outboxEventInsert struct {
	ID          uint64 `gorm:"primaryKey"`
	EventID     string
	Topic       string
	EventKey    string
	EventType   string
	Payload     []byte `gorm:"type:json"`
	Status      uint8
	NextRetryAt time.Time
}

func (outboxEventInsert) TableName() string { return "outbox_events" }

func (r *gormRepository) MarkReady(ctx context.Context, id uint64) error {
	result := r.db.WithContext(ctx).Model(&Video{}).
		Where("id = ? AND status = ?", id, StatusUploading).
		Update("status", StatusReady)
	if result.Error != nil {
		return fmt.Errorf("mark video ready: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrStateConflict
	}
	return nil
}

func (r *gormRepository) ListReady(ctx context.Context, page, pageSize int) ([]Video, int64, error) {
	return r.listWithAuthor(ctx, page, pageSize, "v.status = ?", StatusReady)
}

func (r *gormRepository) FindReadyByID(ctx context.Context, id uint64) (*Video, error) {
	var row videoWithAuthorRow
	err := r.withAuthor(ctx).
		Where("v.id = ? AND v.status = ?", id, StatusReady).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find ready video by id: %w", err)
	}
	v := row.video()
	return &v, nil
}

func (r *gormRepository) ListByUser(ctx context.Context, userID uint64, page, pageSize int) ([]Video, int64, error) {
	return r.listWithAuthor(ctx, page, pageSize, "v.user_id = ? AND v.status <> ?", userID, StatusDeleted)
}

func (r *gormRepository) listWithAuthor(ctx context.Context, page, pageSize int, where string, args ...any) ([]Video, int64, error) {
	var total int64
	count := r.db.WithContext(ctx).Table("videos AS v").Where(where, args...).Count(&total)
	if count.Error != nil {
		return nil, 0, fmt.Errorf("count videos: %w", count.Error)
	}

	rows := make([]videoWithAuthorRow, 0, pageSize)
	err := r.withAuthor(ctx).
		Where(where, args...).
		Order("v.created_at DESC, v.id DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, fmt.Errorf("list videos: %w", err)
	}

	videos := make([]Video, 0, len(rows))
	for i := range rows {
		videos = append(videos, rows[i].video())
	}
	return videos, total, nil
}

func (r *gormRepository) withAuthor(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).
		Table("videos AS v").
		Select(`v.id, v.user_id, v.title, v.description, v.object_key, v.status,
			v.file_size, v.content_type, v.hls_master_key, v.cover_object_key,
			v.duration_ms, v.width, v.height, v.processing_progress, v.processing_error,
			v.processed_at, v.created_at, v.updated_at,
			u.id AS author_id, u.username AS author_username, u.nickname AS author_nickname`).
		Joins("JOIN users AS u ON u.id = v.user_id")
}

type videoWithAuthorRow struct {
	ID                 uint64 `gorm:"column:id"`
	UserID             uint64 `gorm:"column:user_id"`
	Title              string
	Description        string
	ObjectKey          string
	Status             Status
	FileSize           int64
	ContentType        string
	HLSMasterKey       *string
	CoverObjectKey     *string
	DurationMS         *uint64
	Width              *uint
	Height             *uint
	ProcessingProgress uint8
	ProcessingError    *string
	ProcessedAt        *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
	AuthorID           uint64 `gorm:"column:author_id"`
	AuthorUsername     string `gorm:"column:author_username"`
	AuthorNickname     string `gorm:"column:author_nickname"`
}

func (r videoWithAuthorRow) video() Video {
	return Video{
		ID:                 r.ID,
		UserID:             r.UserID,
		Title:              r.Title,
		Description:        r.Description,
		ObjectKey:          r.ObjectKey,
		Status:             r.Status,
		FileSize:           r.FileSize,
		ContentType:        r.ContentType,
		HLSMasterKey:       r.HLSMasterKey,
		CoverObjectKey:     r.CoverObjectKey,
		DurationMS:         r.DurationMS,
		Width:              r.Width,
		Height:             r.Height,
		ProcessingProgress: r.ProcessingProgress,
		ProcessingError:    r.ProcessingError,
		ProcessedAt:        r.ProcessedAt,
		CreatedAt:          r.CreatedAt,
		UpdatedAt:          r.UpdatedAt,
		Author: Author{
			ID:       r.AuthorID,
			Username: r.AuthorUsername,
			Nickname: r.AuthorNickname,
		},
	}
}
