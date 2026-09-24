package engagement

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"video_share/internal/analytics"
	"video_share/internal/notification"
	"video_share/internal/user"
	"video_share/internal/video"
)

var (
	ErrVideoNotFound        = errors.New("video not found")
	ErrUnauthorized         = errors.New("unauthorized")
	ErrContentInvalid       = errors.New("invalid comment content")
	ErrCommentNotFound      = errors.New("comment not found")
	ErrCommentForbidden     = errors.New("comment belongs to another user")
	ErrParentCommentInvalid = errors.New("invalid parent comment")
	ErrIdempotencyConflict  = notification.ErrReceiptConflict
	ErrProgressInvalid      = errors.New("invalid watch progress")
	ErrPaginationInvalid    = errors.New("invalid pagination")
)

const (
	counterViews     = "view_count"
	counterLikes     = "like_count"
	counterFavorites = "favorite_count"
	counterComments  = "comment_count"
)

// Repository 以幂等方式改变点赞 / 收藏关系，并返回变更后的最终状态。重复的启用
// 或停用都成功，且不会重复计数。
type Repository interface {
	SetLike(ctx context.Context, userID, videoID uint64, active bool) (*RelationState, error)
	SetFavorite(ctx context.Context, userID, videoID uint64, active bool) (*RelationState, error)

	CreateComment(ctx context.Context, userID, videoID uint64, content string) (*Comment, error)
	ListComments(ctx context.Context, videoID uint64, page, pageSize int) ([]Comment, int64, error)
	ListReplies(ctx context.Context, commentID uint64, page, pageSize int) ([]Comment, int64, error)
	DeleteComment(ctx context.Context, userID, commentID uint64) error

	RecordWatch(ctx context.Context, userID, videoID, progressMS, durationMS uint64) error
	ListFavorites(ctx context.Context, userID uint64, page, pageSize int) ([]video.Video, int64, error)
	ListHistory(ctx context.Context, userID uint64, page, pageSize int) ([]video.Video, int64, error)
}

type gormRepository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

func (r *gormRepository) SetLike(ctx context.Context, userID, videoID uint64, active bool) (*RelationState, error) {
	return r.mutate(ctx, userID, videoID, active, &Like{UserID: userID, VideoID: videoID}, counterLikes)
}

func (r *gormRepository) SetFavorite(ctx context.Context, userID, videoID uint64, active bool) (*RelationState, error) {
	return r.mutate(ctx, userID, videoID, active, &Favorite{UserID: userID, VideoID: videoID}, counterFavorites)
}

// mutate 在同一个事务里确认视频可见、幂等地写入关系，并且只在关系真正发生变化
// 时才移动统计计数，最后读回最终状态。record 是按键填好的关系模型。
func (r *gormRepository) mutate(ctx context.Context, userID, videoID uint64, active bool, record any, counter string) (*RelationState, error) {
	var state RelationState
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ensureEngageableVideo(tx, videoID); err != nil {
			return err
		}
		changed, err := applyRelation(tx, record, userID, videoID, active)
		if err != nil {
			return err
		}
		if changed {
			if err := applyCounter(tx, videoID, counter, active); err != nil {
				return err
			}
			metric := "net_likes"
			if counter == counterFavorites {
				metric = "net_favorites"
			}
			delta := int64(1)
			if !active {
				delta = -1
			}
			if err := analytics.RecordVideoDeltaTx(tx, videoID, metric, delta, time.Now().UTC()); err != nil {
				return err
			}
		}
		state, err = readRelationState(tx, record, userID, videoID, counter)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &state, nil
}

// ensureEngageableVideo 只允许对公开且已就绪的视频建立互动关系。
func ensureEngageableVideo(tx *gorm.DB, videoID uint64) error {
	var count int64
	if err := tx.Table("videos AS v").
		Joins("JOIN users AS u ON u.id = v.user_id").
		Where("v.id = ? AND v.status = ? AND v.visibility = ? AND v.moderation_status = ? AND u.status = ?",
			videoID, video.StatusReady, video.VisibilityPublic, "visible", user.StatusNormal).
		Count(&count).Error; err != nil {
		return fmt.Errorf("check engageable video: %w", err)
	}
	if count == 0 {
		return ErrVideoNotFound
	}
	return nil
}

// applyRelation 写入关系并报告本次调用是否真的改变了行数：INSERT IGNORE 在关系
// 已存在时影响 0 行，按键删除在关系不存在时同样影响 0 行。
func applyRelation(tx *gorm.DB, record any, userID, videoID uint64, active bool) (bool, error) {
	if active {
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(record)
		if result.Error != nil {
			return false, fmt.Errorf("create relation: %w", result.Error)
		}
		return result.RowsAffected == 1, nil
	}
	result := tx.Where("user_id = ? AND video_id = ?", userID, videoID).Delete(record)
	if result.Error != nil {
		return false, fmt.Errorf("delete relation: %w", result.Error)
	}
	return result.RowsAffected == 1, nil
}

// applyCounter 只在关系变化时被调用。
func applyCounter(tx *gorm.DB, videoID uint64, counter string, active bool) error {
	if active {
		return bumpCounter(tx, videoID, counter)
	}
	return dropCounter(tx, videoID, counter)
}

func bumpCounter(tx *gorm.DB, videoID uint64, counter string) error {
	if !knownCounter(counter) {
		return fmt.Errorf("unsupported counter %q", counter)
	}
	if err := tx.Model(&VideoStats{}).Where("video_id = ?", videoID).
		Update(counter, gorm.Expr(counter+" + 1")).Error; err != nil {
		return fmt.Errorf("increment %s: %w", counter, err)
	}
	return nil
}

// dropCounter 用 LEAST 夹住下界，因为 BIGINT UNSIGNED 列在 0 上再减一会直接报错。
func dropCounter(tx *gorm.DB, videoID uint64, counter string) error {
	if !knownCounter(counter) {
		return fmt.Errorf("unsupported counter %q", counter)
	}
	if err := tx.Model(&VideoStats{}).Where("video_id = ?", videoID).
		Update(counter, gorm.Expr(counter+" - LEAST("+counter+", 1)")).Error; err != nil {
		return fmt.Errorf("decrement %s: %w", counter, err)
	}
	return nil
}

// knownCounter 是动态列名的白名单，保证列名永远不会来自外部输入。
func knownCounter(counter string) bool {
	switch counter {
	case counterViews, counterLikes, counterFavorites, counterComments:
		return true
	default:
		return false
	}
}

func readRelationState(tx *gorm.DB, record any, userID, videoID uint64, counter string) (RelationState, error) {
	var relationRows []struct{ Marker int }
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Model(record).
		Select("1 AS marker").
		Where("user_id = ? AND video_id = ?", userID, videoID).
		Limit(1).Find(&relationRows).Error; err != nil {
		return RelationState{}, fmt.Errorf("check relation: %w", err)
	}
	var stats struct{ Value uint64 }
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Model(&VideoStats{}).
		Select(counter+" AS value").Where("video_id = ?", videoID).
		Take(&stats).Error; err != nil {
		return RelationState{}, fmt.Errorf("read relation counter: %w", err)
	}
	return RelationState{Active: len(relationRows) > 0, Count: stats.Value}, nil
}

func (r *gormRepository) CreateComment(ctx context.Context, userID, videoID uint64, content string) (*Comment, error) {
	return r.createComment(ctx, userID, videoID, content, nil, "")
}

// CreateCommentWithRequest extends the original write with one-level reply and
// idempotency semantics. It is intentionally an additional method so old
// Repository test doubles and callers remain source-compatible.
func (r *gormRepository) CreateCommentWithRequest(ctx context.Context, userID, videoID uint64, content string, parentID *uint64, requestID string) (*Comment, error) {
	return r.createComment(ctx, userID, videoID, content, parentID, requestID)
}

func (r *gormRepository) createComment(ctx context.Context, userID, videoID uint64, content string, parentID *uint64, requestID string) (*Comment, error) {
	// 落库前再次裁剪：仓储是写入边界，任何调用方传进来的首尾空白都不应被持久化。
	trimmed := strings.TrimSpace(content)
	hashPayload, _ := json.Marshal(struct {
		Content  string  `json:"content"`
		ParentID *uint64 `json:"parent_id"`
	}{trimmed, parentID})
	requestHash := notification.RequestHash(string(hashPayload))
	created := Comment{VideoID: videoID, UserID: userID, ParentID: parentID, Content: trimmed}
	duplicate := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ensureEngageableVideo(tx, videoID); err != nil {
			return err
		}
		if receipt, err := notification.ExistingReceipt(tx, userID, "comment.create", requestID, requestHash); err != nil {
			return err
		} else if receipt != nil && receipt.ResourceID != nil {
			duplicate = true
			return tx.Where("id = ?", *receipt.ResourceID).Take(&created).Error
		}
		if parentID != nil {
			var parent Comment
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND video_id = ?", *parentID, videoID).Take(&parent).Error; errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrParentCommentInvalid
			} else if err != nil {
				return fmt.Errorf("load parent comment: %w", err)
			} else if parent.DeletedAt != nil || (parent.ModerationStatus != "" && parent.ModerationStatus != "visible") {
				return ErrParentCommentInvalid
			} else if parent.RootID != nil {
				created.RootID = parent.RootID
			} else {
				created.RootID = &parent.ID
			}
		} else {
			// The root ID is the auto-increment ID, so it is filled immediately
			// after insertion below.
		}
		if err := tx.Create(&created).Error; err != nil {
			return fmt.Errorf("create comment: %w", err)
		}
		if created.RootID == nil {
			created.RootID = &created.ID
			if err := tx.Model(&Comment{}).Where("id = ?", created.ID).Update("root_id", created.ID).Error; err != nil {
				return fmt.Errorf("set comment root: %w", err)
			}
		}
		if err := bumpCounter(tx, videoID, counterComments); err != nil {
			return err
		}
		if err := analytics.RecordVideoDeltaTx(tx, videoID, "net_comments", 1, time.Now().UTC()); err != nil {
			return err
		}
		var ownerID uint64
		if err := tx.Table("videos").Where("id = ?", videoID).Pluck("user_id", &ownerID).Error; err != nil {
			return fmt.Errorf("load video owner: %w", err)
		}
		actorID := userID
		if parentID == nil {
			if err := notification.Emit(tx, notification.Event{RecipientID: ownerID, ActorID: &actorID, EventKey: fmt.Sprintf("comment:%d:author", created.ID), Type: "comment", VideoID: &videoID, CommentID: &created.ID}); err != nil {
				return err
			}
		} else {
			var parent Comment
			if err := tx.Select("user_id").Where("id = ?", *parentID).Take(&parent).Error; err != nil {
				return fmt.Errorf("reload parent author: %w", err)
			}
			if err := notification.Emit(tx, notification.Event{RecipientID: parent.UserID, ActorID: &actorID, EventKey: fmt.Sprintf("comment:%d:reply:%d", created.ID, parent.UserID), Type: "reply", VideoID: &videoID, CommentID: &created.ID}); err != nil {
				return err
			}
			if ownerID != parent.UserID {
				if err := notification.Emit(tx, notification.Event{RecipientID: ownerID, ActorID: &actorID, EventKey: fmt.Sprintf("comment:%d:author", created.ID), Type: "reply", VideoID: &videoID, CommentID: &created.ID}); err != nil {
					return err
				}
			}
		}
		if err := notification.SaveReceipt(tx, userID, "comment.create", requestID, requestHash, &created.ID); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if duplicate || err == nil {
		// Duplicate requests return the original row and must not increment
		// counters or emit a second notification.
	}
	if err := r.loadCommentAuthor(ctx, &created); err != nil {
		return nil, err
	}
	return &created, nil
}

func (r *gormRepository) ListComments(ctx context.Context, videoID uint64, page, pageSize int) ([]Comment, int64, error) {
	if err := ensureEngageableVideo(r.db.WithContext(ctx), videoID); err != nil {
		return nil, 0, err
	}
	var total int64
	if err := r.db.WithContext(ctx).Model(&Comment{}).
		Where("video_id = ? AND parent_id IS NULL AND moderation_status = ?", videoID, "visible").Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count comments: %w", err)
	}

	rows := make([]commentWithAuthorRow, 0, pageSize)
	err := r.db.WithContext(ctx).Table("comments AS c").
		Select(commentAuthorColumns).
		Joins("JOIN users AS u ON u.id = c.user_id").
		Where("c.video_id = ? AND c.parent_id IS NULL AND c.moderation_status = ?", videoID, "visible").
		Order("c.created_at DESC, c.id DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, fmt.Errorf("list comments: %w", err)
	}

	comments := make([]Comment, 0, len(rows))
	for i := range rows {
		comments = append(comments, rows[i].comment())
	}
	return comments, total, nil
}

func (r *gormRepository) ListReplies(ctx context.Context, commentID uint64, page, pageSize int) ([]Comment, int64, error) {
	var parent Comment
	if err := r.db.WithContext(ctx).Where("id = ?", commentID).Take(&parent).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, 0, ErrCommentNotFound
	} else if err != nil {
		return nil, 0, fmt.Errorf("load reply root: %w", err)
	}
	if parent.ModerationStatus == "hidden" {
		return nil, 0, ErrCommentNotFound
	}
	if err := ensureEngageableVideo(r.db.WithContext(ctx), parent.VideoID); err != nil {
		return nil, 0, err
	}
	var total int64
	if err := r.db.WithContext(ctx).Model(&Comment{}).Where("root_id = ? AND parent_id IS NOT NULL AND moderation_status = ?", commentID, "visible").Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count replies: %w", err)
	}
	rows := make([]commentWithAuthorRow, 0, pageSize)
	if err := r.db.WithContext(ctx).Table("comments AS c").Select(commentAuthorColumns).
		Joins("JOIN users AS u ON u.id = c.user_id").
		Where("c.root_id = ? AND c.parent_id IS NOT NULL AND c.moderation_status = ?", commentID, "visible").
		Order("c.created_at ASC, c.id ASC").Offset((page - 1) * pageSize).Limit(pageSize).Scan(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("list replies: %w", err)
	}
	items := make([]Comment, 0, len(rows))
	for i := range rows {
		items = append(items, rows[i].comment())
	}
	return items, total, nil
}

func (r *gormRepository) DeleteComment(ctx context.Context, userID, commentID uint64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var comment Comment
		err := tx.Where("id = ? AND deleted_at IS NULL", commentID).Take(&comment).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrCommentNotFound
		}
		if err != nil {
			return fmt.Errorf("load comment: %w", err)
		}
		if comment.UserID != userID {
			return ErrCommentForbidden
		}
		result := tx.Model(&Comment{}).
			Where("id = ? AND user_id = ? AND deleted_at IS NULL", commentID, userID).
			Update("deleted_at", gorm.Expr("UTC_TIMESTAMP(3)"))
		if result.Error != nil {
			return fmt.Errorf("soft delete comment: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return ErrCommentNotFound
		}
		if err := dropCounter(tx, comment.VideoID, counterComments); err != nil {
			return err
		}
		return analytics.RecordVideoDeltaTx(tx, comment.VideoID, "net_comments", -1, time.Now().UTC())
	})
}

func (r *gormRepository) RecordWatch(ctx context.Context, userID, videoID, progressMS, durationMS uint64) error {
	// 先于写入拒绝越界进度：否则会撞上 chk_watch_histories_progress 约束，把原始
	// SQL 错误抛给调用方，而不是可映射的 ErrProgressInvalid。
	if durationMS != 0 && progressMS > durationMS {
		return ErrProgressInvalid
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ensureEngageableVideo(tx, videoID); err != nil {
			return err
		}
		// DATETIME 不保存时区。首次写入和后续更新必须使用同一数据库时钟，
		// 否则应用进程与 MySQL 的时区不同会使 last_watched_at 早于 first_watched_at。
		insert := tx.Exec(`INSERT INTO watch_histories
			(user_id, video_id, progress_ms, duration_ms, first_watched_at, last_watched_at)
			VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP(3), CURRENT_TIMESTAMP(3))
			ON DUPLICATE KEY UPDATE user_id = watch_histories.user_id`,
			userID, videoID, progressMS, durationMS)
		if insert.Error != nil {
			return fmt.Errorf("record watch history: %w", insert.Error)
		}
		if insert.RowsAffected == 1 {
			return bumpCounter(tx, videoID, counterViews)
		}
		result := tx.Model(&WatchHistory{}).
			Where("user_id = ? AND video_id = ?", userID, videoID).
			Updates(map[string]any{
				"progress_ms":     progressMS,
				"duration_ms":     durationMS,
				"last_watched_at": gorm.Expr("GREATEST(last_watched_at, first_watched_at, CURRENT_TIMESTAMP(3))"),
			})
		if result.Error != nil {
			return fmt.Errorf("update watch history: %w", result.Error)
		}
		return nil
	})
}

func (r *gormRepository) ListFavorites(ctx context.Context, userID uint64, page, pageSize int) ([]video.Video, int64, error) {
	return r.listPersonal(ctx, userID, page, pageSize,
		"JOIN video_favorites AS f ON f.video_id = v.id AND f.user_id = ?", "f.created_at")
}

func (r *gormRepository) ListHistory(ctx context.Context, userID uint64, page, pageSize int) ([]video.Video, int64, error) {
	return r.listPersonal(ctx, userID, page, pageSize,
		"JOIN watch_histories AS h ON h.video_id = v.id AND h.user_id = ?", "h.last_watched_at")
}

// listPersonal 只返回当前仍然公开且已就绪的视频，并按关系表自己的时间戳倒序，
// 因此隐藏或删除过的视频不会留在个人列表里。
func (r *gormRepository) listPersonal(ctx context.Context, userID uint64, page, pageSize int, join, orderColumn string) ([]video.Video, int64, error) {
	scope := func() *gorm.DB {
		return r.db.WithContext(ctx).Table("videos AS v").
			Joins(join, userID).
			Joins("JOIN users AS u ON u.id = v.user_id").
			Joins("LEFT JOIN video_stats AS s ON s.video_id = v.id").
			Where("v.status = ? AND v.visibility = ? AND v.moderation_status = ? AND u.status = ?",
				video.StatusReady, video.VisibilityPublic, "visible", user.StatusNormal)
	}

	var total int64
	if err := scope().Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count personal videos: %w", err)
	}

	rows := make([]personalVideoRow, 0, pageSize)
	err := scope().
		Select(personalVideoColumns).
		Order(orderColumn + " DESC, v.id DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, fmt.Errorf("list personal videos: %w", err)
	}

	videos := make([]video.Video, 0, len(rows))
	for i := range rows {
		videos = append(videos, rows[i].video())
	}
	return videos, total, nil
}

func (r *gormRepository) loadCommentAuthor(ctx context.Context, c *Comment) error {
	var row struct {
		ID       uint64
		Username string
		Nickname string
	}
	if err := r.db.WithContext(ctx).Raw("SELECT id, username, nickname FROM users WHERE id = ?", c.UserID).
		Scan(&row).Error; err != nil {
		return fmt.Errorf("load comment author: %w", err)
	}
	c.Author = video.Author{ID: row.ID, Username: row.Username, Nickname: row.Nickname}
	return nil
}

const commentAuthorColumns = `c.id, c.video_id, c.user_id, c.parent_id, c.root_id, c.content, c.deleted_at, c.moderation_status, c.created_at, c.updated_at,
	u.id AS author_id, u.username AS author_username, u.nickname AS author_nickname`

type commentWithAuthorRow struct {
	Comment
	AuthorID       uint64 `gorm:"column:author_id"`
	AuthorUsername string `gorm:"column:author_username"`
	AuthorNickname string `gorm:"column:author_nickname"`
}

func (row commentWithAuthorRow) comment() Comment {
	c := row.Comment
	if row.AuthorID != 0 {
		c.Author = video.Author{ID: row.AuthorID, Username: row.AuthorUsername, Nickname: row.AuthorNickname}
	}
	return c
}

// personalVideoColumns 复用公开卡片需要的列：视频本体、作者公开字段和统计计数。
const personalVideoColumns = `v.*, u.id AS author_id, u.username AS author_username, u.nickname AS author_nickname,
	COALESCE(s.view_count, 0) AS view_count, COALESCE(s.like_count, 0) AS like_count,
	COALESCE(s.favorite_count, 0) AS favorite_count, COALESCE(s.comment_count, 0) AS comment_count`

type personalVideoRow struct {
	video.Video
	AuthorID       uint64 `gorm:"column:author_id"`
	AuthorUsername string `gorm:"column:author_username"`
	AuthorNickname string `gorm:"column:author_nickname"`
	ViewCount      uint64 `gorm:"column:view_count"`
	LikeCount      uint64 `gorm:"column:like_count"`
	FavoriteCount  uint64 `gorm:"column:favorite_count"`
	CommentCount   uint64 `gorm:"column:comment_count"`
}

func (row personalVideoRow) video() video.Video {
	v := row.Video
	if row.AuthorID != 0 {
		v.Author = video.Author{ID: row.AuthorID, Username: row.AuthorUsername, Nickname: row.AuthorNickname}
	}
	v.Stats = video.Stats{
		ViewCount:     row.ViewCount,
		LikeCount:     row.LikeCount,
		FavoriteCount: row.FavoriteCount,
		CommentCount:  row.CommentCount,
	}
	return v
}
