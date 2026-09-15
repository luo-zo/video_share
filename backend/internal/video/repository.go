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
	"video_share/internal/user"
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
	ListPublic(ctx context.Context, query ListQuery) ([]Video, int64, error)
	FindPublicByID(ctx context.Context, id uint64) (*Video, error)
	ViewerState(ctx context.Context, viewerID, videoID, authorID uint64) (*ViewerState, error)
	UpdateOwned(ctx context.Context, userID, videoID uint64, patch VideoPatch) error
	DeleteOwned(ctx context.Context, userID, videoID uint64) error
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
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(v).Error; err != nil {
			return fmt.Errorf("create video: %w", err)
		}
		if err := tx.Create(&videoStatsInsert{VideoID: v.ID}).Error; err != nil {
			return fmt.Errorf("create video stats: %w", err)
		}
		return nil
	})
}

// videoStatsInsert creates the zeroed counter row that every video owns. It is
// created together with the video so the invariant holds for rows added after
// the backfill migration.
type videoStatsInsert struct {
	VideoID uint64 `gorm:"primaryKey"`
}

func (videoStatsInsert) TableName() string { return "video_stats" }

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
	var row videoWithAuthorRow
	err := r.withAuthorAndStats(ctx).Where("v.id = ?", id).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find video by id: %w", err)
	}
	v := row.video()
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
	return r.ListPublic(ctx, ListQuery{Page: page, PageSize: pageSize, Sort: SortLatest})
}

func (r *gormRepository) FindReadyByID(ctx context.Context, id uint64) (*Video, error) {
	return r.FindPublicByID(ctx, id)
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
	err := r.withAuthorAndStats(ctx).
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

const videoAuthorColumns = `v.id, v.user_id, v.title, v.description, v.object_key, v.status,
	v.visibility, v.published_at,
	v.file_size, v.content_type, v.hls_master_key, v.cover_object_key,
	v.duration_ms, v.width, v.height, v.processing_progress, v.processing_error,
	v.processed_at, v.created_at, v.updated_at,
	u.id AS author_id, u.username AS author_username, u.nickname AS author_nickname`

const videoStatsColumns = `,
	COALESCE(s.view_count, 0) AS view_count,
	COALESCE(s.like_count, 0) AS like_count,
	COALESCE(s.favorite_count, 0) AS favorite_count,
	COALESCE(s.comment_count, 0) AS comment_count`

func (r *gormRepository) withAuthor(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).
		Table("videos AS v").
		Select(videoAuthorColumns).
		Joins("JOIN users AS u ON u.id = v.user_id")
}

func (r *gormRepository) withAuthorAndStats(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).
		Table("videos AS v").
		Select(videoAuthorColumns + videoStatsColumns).
		Joins("JOIN users AS u ON u.id = v.user_id").
		Joins("LEFT JOIN video_stats AS s ON s.video_id = v.id")
}

// applyPublicFilters narrows a discovery query to videos that are ready and
// public. Search patterns are bound as parameters and escaped with `!`, so
// user input cannot change the shape of the statement.
func applyPublicFilters(db *gorm.DB, query ListQuery) *gorm.DB {
	db = db.Where("v.status = ? AND v.visibility = ? AND u.status = ?",
		StatusReady, VisibilityPublic, user.StatusNormal)
	if query.Query == "" {
		return db
	}
	pattern := "%" + escapeLike(query.Query, '!') + "%"
	return db.Where(`v.title LIKE ? ESCAPE '!' OR v.description LIKE ? ESCAPE '!'
		OR u.username LIKE ? ESCAPE '!' OR u.nickname LIKE ? ESCAPE '!'`,
		pattern, pattern, pattern, pattern)
}

func publicOrderClause(sort Sort) string {
	if sort == SortPopular {
		return `COALESCE(s.view_count, 0) DESC, COALESCE(s.like_count, 0) DESC,
			COALESCE(v.published_at, v.created_at) DESC, v.id DESC`
	}
	return "COALESCE(v.published_at, v.created_at) DESC, v.id DESC"
}

func (r *gormRepository) ListPublic(ctx context.Context, query ListQuery) ([]Video, int64, error) {
	countQuery := applyPublicFilters(r.db.WithContext(ctx).
		Table("videos AS v").
		Joins("JOIN users AS u ON u.id = v.user_id"), query)
	var total int64
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count public videos: %w", err)
	}

	rows := make([]videoWithAuthorRow, 0, query.PageSize)
	err := applyPublicFilters(r.withAuthorAndStats(ctx), query).
		Order(publicOrderClause(query.Sort)).
		Offset((query.Page - 1) * query.PageSize).
		Limit(query.PageSize).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, fmt.Errorf("list public videos: %w", err)
	}
	return rowsToVideos(rows), total, nil
}

func (r *gormRepository) FindPublicByID(ctx context.Context, id uint64) (*Video, error) {
	var row videoWithAuthorRow
	err := r.withAuthorAndStats(ctx).
		Where("v.id = ? AND v.status = ? AND v.visibility = ? AND u.status = ?",
			id, StatusReady, VisibilityPublic, user.StatusNormal).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find public video by id: %w", err)
	}
	v := row.video()
	return &v, nil
}

func (r *gormRepository) ViewerState(ctx context.Context, viewerID, videoID, authorID uint64) (*ViewerState, error) {
	if viewerID == 0 {
		return &ViewerState{}, nil
	}
	state := &ViewerState{}
	liked, err := r.relationExists(ctx, "video_likes", "user_id", "video_id", viewerID, videoID)
	if err != nil {
		return nil, fmt.Errorf("check viewer like: %w", err)
	}
	state.Liked = liked
	favorited, err := r.relationExists(ctx, "video_favorites", "user_id", "video_id", viewerID, videoID)
	if err != nil {
		return nil, fmt.Errorf("check viewer favorite: %w", err)
	}
	state.Favorited = favorited
	if authorID != 0 && authorID != viewerID {
		following, err := r.relationExists(ctx, "user_follows", "follower_id", "followee_id", viewerID, authorID)
		if err != nil {
			return nil, fmt.Errorf("check viewer follow: %w", err)
		}
		state.FollowingAuthor = following
	}
	return state, nil
}

// UpdateOwned applies a partial update that only the author may perform. A ready
// video that becomes public for the first time records its publish time; later
// visibility changes keep that first value.
func (r *gormRepository) UpdateOwned(ctx context.Context, userID, videoID uint64, patch VideoPatch) error {
	updates := map[string]any{"updated_at": gorm.Expr("UTC_TIMESTAMP(3)")}
	if patch.Title != nil {
		updates["title"] = *patch.Title
	}
	if patch.Description != nil {
		updates["description"] = *patch.Description
	}
	if patch.Visibility != nil {
		updates["visibility"] = *patch.Visibility
		if *patch.Visibility == VisibilityPublic {
			updates["published_at"] = gorm.Expr(
				"COALESCE(published_at, CASE WHEN status = ? THEN UTC_TIMESTAMP(3) ELSE NULL END)", StatusReady)
		}
	}
	result := r.db.WithContext(ctx).Model(&Video{}).
		Where("id = ? AND user_id = ? AND status <> ?", videoID, userID, StatusDeleted).
		Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("update owned video: %w", result.Error)
	}
	if result.RowsAffected > 0 {
		return nil
	}
	// MySQL reports changed rows, so an update whose values are already stored
	// also reports zero. Confirm the row still exists before calling it missing.
	var exists int64
	if err := r.db.WithContext(ctx).Model(&Video{}).
		Where("id = ? AND user_id = ? AND status <> ?", videoID, userID, StatusDeleted).
		Count(&exists).Error; err != nil {
		return fmt.Errorf("confirm owned video: %w", err)
	}
	if exists == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteOwned logically deletes an author's own video and makes it private so a
// stale link cannot keep serving it. Deleting twice reports the video as missing.
func (r *gormRepository) DeleteOwned(ctx context.Context, userID, videoID uint64) error {
	result := r.db.WithContext(ctx).Model(&Video{}).
		Where("id = ? AND user_id = ? AND status <> ?", videoID, userID, StatusDeleted).
		Updates(map[string]any{
			"status":     StatusDeleted,
			"visibility": VisibilityPrivate,
			"updated_at": gorm.Expr("UTC_TIMESTAMP(3)"),
		})
	if result.Error != nil {
		return fmt.Errorf("delete owned video: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormRepository) relationExists(ctx context.Context, table, ownerColumn, targetColumn string, ownerID, targetID uint64) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Table(table).
		Where(ownerColumn+" = ? AND "+targetColumn+" = ?", ownerID, targetID).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

type videoWithAuthorRow struct {
	ID                 uint64 `gorm:"column:id"`
	UserID             uint64 `gorm:"column:user_id"`
	Title              string
	Description        string
	ObjectKey          string
	Status             Status
	Visibility         Visibility
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
	PublishedAt        *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
	AuthorID           uint64 `gorm:"column:author_id"`
	AuthorUsername     string `gorm:"column:author_username"`
	AuthorNickname     string `gorm:"column:author_nickname"`
	ViewCount          uint64 `gorm:"column:view_count"`
	LikeCount          uint64 `gorm:"column:like_count"`
	FavoriteCount      uint64 `gorm:"column:favorite_count"`
	CommentCount       uint64 `gorm:"column:comment_count"`
}

func (r videoWithAuthorRow) video() Video {
	return Video{
		ID:                 r.ID,
		UserID:             r.UserID,
		Title:              r.Title,
		Description:        r.Description,
		ObjectKey:          r.ObjectKey,
		Status:             r.Status,
		Visibility:         r.Visibility,
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
		PublishedAt:        r.PublishedAt,
		CreatedAt:          r.CreatedAt,
		UpdatedAt:          r.UpdatedAt,
		Author: Author{
			ID:       r.AuthorID,
			Username: r.AuthorUsername,
			Nickname: r.AuthorNickname,
		},
		Stats: Stats{
			ViewCount:     r.ViewCount,
			LikeCount:     r.LikeCount,
			FavoriteCount: r.FavoriteCount,
			CommentCount:  r.CommentCount,
		},
	}
}

func rowsToVideos(rows []videoWithAuthorRow) []Video {
	videos := make([]Video, 0, len(rows))
	for i := range rows {
		videos = append(videos, rows[i].video())
	}
	return videos
}
