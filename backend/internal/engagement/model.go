package engagement

import (
	"time"

	"video_share/internal/video"
)

type VideoStats struct {
	VideoID       uint64    `gorm:"primaryKey"`
	ViewCount     uint64    `gorm:"not null;default:0"`
	LikeCount     uint64    `gorm:"not null;default:0"`
	FavoriteCount uint64    `gorm:"not null;default:0"`
	CommentCount  uint64    `gorm:"not null;default:0"`
	UpdatedAt     time.Time `gorm:"not null"`
}

func (VideoStats) TableName() string { return "video_stats" }

type Like struct {
	UserID    uint64    `gorm:"primaryKey"`
	VideoID   uint64    `gorm:"primaryKey"`
	CreatedAt time.Time `gorm:"not null"`
}

func (Like) TableName() string { return "video_likes" }

type Favorite struct {
	UserID    uint64    `gorm:"primaryKey"`
	VideoID   uint64    `gorm:"primaryKey"`
	CreatedAt time.Time `gorm:"not null"`
}

func (Favorite) TableName() string { return "video_favorites" }

type Comment struct {
	ID        uint64     `gorm:"primaryKey;autoIncrement"`
	VideoID   uint64     `gorm:"not null;index"`
	UserID    uint64     `gorm:"not null;index"`
	Content   string     `gorm:"type:varchar(500);not null"`
	CreatedAt time.Time  `gorm:"not null"`
	UpdatedAt time.Time  `gorm:"not null"`
	DeletedAt *time.Time `gorm:"type:datetime(3);index"`

	Author video.Author `gorm:"-"`
}

func (Comment) TableName() string { return "comments" }

type WatchHistory struct {
	UserID         uint64    `gorm:"primaryKey"`
	VideoID        uint64    `gorm:"primaryKey"`
	ProgressMS     uint64    `gorm:"not null;default:0"`
	DurationMS     uint64    `gorm:"not null;default:0"`
	FirstWatchedAt time.Time `gorm:"not null"`
	LastWatchedAt  time.Time `gorm:"not null;index"`
}

func (WatchHistory) TableName() string { return "watch_histories" }
