package engagement

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"video_share/internal/video"
)

var (
	ErrVideoNotFound = errors.New("video not found")
	ErrUnauthorized  = errors.New("unauthorized")
)

const (
	counterLikes     = "like_count"
	counterFavorites = "favorite_count"
)

// Repository 以幂等方式改变点赞 / 收藏关系，并返回变更后的最终状态。重复的启用
// 或停用都成功，且不会重复计数。
type Repository interface {
	SetLike(ctx context.Context, userID, videoID uint64, active bool) (*RelationState, error)
	SetFavorite(ctx context.Context, userID, videoID uint64, active bool) (*RelationState, error)
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
	if err := tx.Model(&video.Video{}).
		Where("id = ? AND status = ? AND visibility = ?", videoID, video.StatusReady, video.VisibilityPublic).
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

// applyCounter 只在关系变化时被调用。递减使用 LEAST 夹住下界，因为 BIGINT
// UNSIGNED 列在 0 上再减一会直接报错。
func applyCounter(tx *gorm.DB, videoID uint64, counter string, active bool) error {
	if counter != counterLikes && counter != counterFavorites {
		return fmt.Errorf("unsupported counter %q", counter)
	}
	expression := gorm.Expr(counter + " + 1")
	if !active {
		expression = gorm.Expr(counter + " - LEAST(" + counter + ", 1)")
	}
	if err := tx.Model(&VideoStats{}).Where("video_id = ?", videoID).
		Update(counter, expression).Error; err != nil {
		return fmt.Errorf("update %s: %w", counter, err)
	}
	return nil
}

func readRelationState(tx *gorm.DB, record any, userID, videoID uint64, counter string) (RelationState, error) {
	var mine int64
	if err := tx.Model(record).Where("user_id = ? AND video_id = ?", userID, videoID).
		Count(&mine).Error; err != nil {
		return RelationState{}, fmt.Errorf("check relation: %w", err)
	}
	var stats struct{ Value uint64 }
	if err := tx.Model(&VideoStats{}).Select(counter+" AS value").Where("video_id = ?", videoID).
		Scan(&stats).Error; err != nil {
		return RelationState{}, fmt.Errorf("read relation counter: %w", err)
	}
	return RelationState{Active: mine > 0, Count: stats.Value}, nil
}
