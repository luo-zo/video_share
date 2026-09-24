package creator

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"video_share/internal/user"
	"video_share/internal/video"
)

var ErrNotFound = errors.New("public creator not found")

type Repository interface {
	FindPublicProfile(ctx context.Context, creatorID, viewerID uint64) (*ProfileResponse, error)
}

type gormRepository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) Repository { return &gormRepository{db: db} }

func (r *gormRepository) FindPublicProfile(ctx context.Context, creatorID, viewerID uint64) (*ProfileResponse, error) {
	if creatorID == 0 {
		return nil, ErrNotFound
	}
	var row struct {
		ID             uint64
		Username       string
		Nickname       string
		Bio            string
		CreatedAt      time.Time
		VideoCount     int64
		FollowerCount  int64
		FollowingCount int64
		Following      bool
	}
	query := r.db.WithContext(ctx).Table("users AS u").Select(`
		u.id, u.username, u.nickname, u.bio, u.created_at,
		(SELECT COUNT(*) FROM videos AS v WHERE v.user_id = u.id AND v.status = ? AND v.visibility = ? AND v.moderation_status = 'visible') AS video_count,
		(SELECT COUNT(*) FROM user_follows AS f JOIN users AS follower ON follower.id = f.follower_id
		 WHERE f.followee_id = u.id AND follower.status = ?) AS follower_count,
		(SELECT COUNT(*) FROM user_follows AS f JOIN users AS followee ON followee.id = f.followee_id
		 WHERE f.follower_id = u.id AND followee.status = ?) AS following_count,
		CASE WHEN ? > 0 AND EXISTS (SELECT 1 FROM user_follows AS mine WHERE mine.follower_id = ? AND mine.followee_id = u.id) THEN 1 ELSE 0 END AS following`,
		video.StatusReady, video.VisibilityPublic, user.StatusNormal, user.StatusNormal, viewerID, viewerID).
		Where("u.id = ? AND u.status = ?", creatorID, user.StatusNormal)
	result := query.Scan(&row)
	if result.Error != nil {
		return nil, fmt.Errorf("find public creator: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, ErrNotFound
	}
	return &ProfileResponse{
		ID: row.ID, Username: row.Username, Nickname: row.Nickname, Bio: row.Bio,
		CreatedAt: row.CreatedAt, VideoCount: row.VideoCount, FollowerCount: row.FollowerCount,
		FollowingCount: row.FollowingCount, Following: row.Following,
	}, nil
}
