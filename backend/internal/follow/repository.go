package follow

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"video_share/internal/user"
)

var (
	ErrSelfFollow        = errors.New("cannot follow yourself")
	ErrUserNotFound      = errors.New("user not found")
	ErrUnauthorized      = errors.New("unauthorized")
	ErrPaginationInvalid = errors.New("invalid pagination")
)

// FollowedUser 是关注列表所需的用户公开字段投影，只暴露 ID、用户名和昵称。
type FollowedUser struct {
	ID       uint64
	Username string
	Nickname string
}

// Repository 以幂等方式改变关注关系并返回最终状态；重复关注或取关都成功。
type Repository interface {
	SetFollow(ctx context.Context, followerID, followeeID uint64, active bool) (*FollowState, error)
	ListFollows(ctx context.Context, followerID uint64, page, pageSize int) ([]FollowedUser, int64, error)
}

type gormRepository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

// SetFollow 在同一个事务里确认目标用户可被关注、幂等地写入关系，最后读回最终状态。
func (r *gormRepository) SetFollow(ctx context.Context, followerID, followeeID uint64, active bool) (*FollowState, error) {
	var state FollowState
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ensureFollowableUser(tx, followeeID); err != nil {
			return err
		}
		if err := applyFollow(tx, followerID, followeeID, active); err != nil {
			return err
		}
		var rows []Follow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("follower_id = ? AND followee_id = ?", followerID, followeeID).
			Limit(1).Find(&rows).Error; err != nil {
			return fmt.Errorf("check follow: %w", err)
		}
		state = FollowState{Following: len(rows) > 0}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &state, nil
}

// ensureFollowableUser 只允许关注存在且状态正常的用户。
func ensureFollowableUser(tx *gorm.DB, userID uint64) error {
	var count int64
	if err := tx.Model(&user.User{}).Where("id = ? AND status = ?", userID, user.StatusNormal).
		Count(&count).Error; err != nil {
		return fmt.Errorf("check followable user: %w", err)
	}
	if count == 0 {
		return ErrUserNotFound
	}
	return nil
}

// applyFollow 幂等地写入或删除关注关系：重复插入影响 0 行，按键删除在不存在时同样影响 0 行。
func applyFollow(tx *gorm.DB, followerID, followeeID uint64, active bool) error {
	if active {
		record := Follow{FollowerID: followerID, FolloweeID: followeeID, CreatedAt: time.Now().UTC()}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&record).Error; err != nil {
			return fmt.Errorf("create follow: %w", err)
		}
		return nil
	}
	if err := tx.Where("follower_id = ? AND followee_id = ?", followerID, followeeID).Delete(&Follow{}).Error; err != nil {
		return fmt.Errorf("delete follow: %w", err)
	}
	return nil
}

// ListFollows 按关注时间倒序返回被关注用户；已禁用的用户不出现在列表里。
func (r *gormRepository) ListFollows(ctx context.Context, followerID uint64, page, pageSize int) ([]FollowedUser, int64, error) {
	scope := func() *gorm.DB {
		return r.db.WithContext(ctx).Table("user_follows AS f").
			Joins("JOIN users AS u ON u.id = f.followee_id").
			Where("f.follower_id = ? AND u.status = ?", followerID, user.StatusNormal)
	}

	var total int64
	if err := scope().Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count follows: %w", err)
	}

	rows := make([]FollowedUser, 0, pageSize)
	if err := scope().
		Select("u.id, u.username, u.nickname").
		Order("f.created_at DESC, f.followee_id DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Scan(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("list follows: %w", err)
	}
	return rows, total, nil
}
