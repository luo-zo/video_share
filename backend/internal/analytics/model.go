package analytics

import "time"

// WatchSession is the server-owned state machine for one playback tab. The
// session is deliberately separate from watch_histories so progress can be
// replay-safe without turning a position update into watch time.
type WatchSession struct {
	ID             string     `gorm:"type:char(36);primaryKey"`
	UserID         uint64     `gorm:"not null;index"`
	VideoID        uint64     `gorm:"not null;index"`
	CreatedAt      time.Time  `gorm:"not null"`
	ExpiresAt      time.Time  `gorm:"not null"`
	LastSeq        uint64     `gorm:"not null;default:0"`
	LastPositionMS uint64     `gorm:"not null;default:0"`
	DurationMS     uint64     `gorm:"not null;default:0"`
	LastReceivedAt time.Time  `gorm:"not null"`
	CreditedMS     uint64     `gorm:"not null;default:0"`
	QualifiedAt    *time.Time `gorm:"type:datetime(3)"`
	CompletedAt    *time.Time `gorm:"type:datetime(3)"`
	CohortDate     *time.Time `gorm:"type:date"`
}

func (WatchSession) TableName() string { return "watch_sessions" }

type VideoDailyMetric struct {
	VideoID        uint64    `gorm:"primaryKey"`
	StatDate       time.Time `gorm:"type:date;primaryKey"`
	EffectiveViews uint64    `gorm:"not null;default:0"`
	WatchTimeMS    uint64    `gorm:"not null;default:0"`
	Completions    uint64    `gorm:"not null;default:0"`
	NetLikes       int64     `gorm:"not null;default:0"`
	NetFavorites   int64     `gorm:"not null;default:0"`
	NetComments    int64     `gorm:"not null;default:0"`
}

func (VideoDailyMetric) TableName() string { return "video_daily_metrics" }

type CreatorDailyMetric struct {
	CreatorID      uint64    `gorm:"primaryKey"`
	StatDate       time.Time `gorm:"type:date;primaryKey"`
	EffectiveViews uint64    `gorm:"not null;default:0"`
	WatchTimeMS    uint64    `gorm:"not null;default:0"`
	Completions    uint64    `gorm:"not null;default:0"`
	NetLikes       int64     `gorm:"not null;default:0"`
	NetFavorites   int64     `gorm:"not null;default:0"`
	NetComments    int64     `gorm:"not null;default:0"`
	NetFollowers   int64     `gorm:"not null;default:0"`
}

func (CreatorDailyMetric) TableName() string { return "creator_daily_metrics" }
