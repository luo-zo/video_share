package notification

import "time"

// Notification is a structured event. It intentionally stores IDs and an event
// type instead of copying mutable comment text into a long-lived inbox row.
type Notification struct {
	ID          uint64     `gorm:"primaryKey;autoIncrement"`
	RecipientID uint64     `gorm:"not null;index:idx_notifications_recipient_read,priority:1"`
	ActorID     *uint64    `gorm:"index"`
	EventKey    string     `gorm:"type:varchar(191);not null;uniqueIndex:uq_notifications_recipient_event,priority:2"`
	Type        string     `gorm:"type:varchar(32);not null"`
	VideoID     *uint64    `gorm:"index"`
	CommentID   *uint64    `gorm:"index"`
	ReportID    *uint64    `gorm:"index"`
	ReadAt      *time.Time `gorm:"index:idx_notifications_recipient_read,priority:2"`
	CreatedAt   time.Time  `gorm:"not null;index:idx_notifications_recipient_read,priority:3"`
}

func (Notification) TableName() string { return "notifications" }

type OperationReceipt struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement"`
	ActorID     uint64    `gorm:"not null;uniqueIndex:uq_operation_receipts_actor_action_request,priority:1"`
	Action      string    `gorm:"type:varchar(64);not null;uniqueIndex:uq_operation_receipts_actor_action_request,priority:2"`
	RequestID   string    `gorm:"type:varchar(128);not null;uniqueIndex:uq_operation_receipts_actor_action_request,priority:3"`
	RequestHash string    `gorm:"type:char(64);not null"`
	ResourceID  *uint64   `gorm:"index"`
	CreatedAt   time.Time `gorm:"not null;index"`
}

func (OperationReceipt) TableName() string { return "operation_receipts" }
