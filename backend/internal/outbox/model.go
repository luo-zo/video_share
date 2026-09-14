package outbox

import (
	"encoding/json"
	"time"
)

const (
	StatusPending   uint8 = 1
	StatusPublished uint8 = 2
)

type Event struct {
	ID          uint64          `gorm:"primaryKey"`
	EventID     string          `gorm:"type:char(36);not null;uniqueIndex"`
	Topic       string          `gorm:"type:varchar(255);not null"`
	EventKey    string          `gorm:"type:varchar(255);not null"`
	EventType   string          `gorm:"type:varchar(100);not null"`
	Payload     json.RawMessage `gorm:"type:json;not null"`
	Status      uint8           `gorm:"type:tinyint;not null"`
	Attempts    uint            `gorm:"not null"`
	NextRetryAt time.Time       `gorm:"not null"`
	PublishedAt *time.Time
	LastError   *string `gorm:"type:varchar(1000)"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (Event) TableName() string { return "outbox_events" }
