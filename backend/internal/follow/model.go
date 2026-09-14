package follow

import "time"

type Follow struct {
	FollowerID uint64    `gorm:"primaryKey"`
	FolloweeID uint64    `gorm:"primaryKey"`
	CreatedAt  time.Time `gorm:"not null"`
}

func (Follow) TableName() string { return "user_follows" }
