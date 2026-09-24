package moderation

import "time"

const (
	TargetVideo   = "video"
	TargetComment = "comment"
	TargetUser    = "user"

	ReasonSpam      = "spam"
	ReasonAbuse     = "abuse"
	ReasonCopyright = "copyright"
	ReasonIllegal   = "illegal"
	ReasonOther     = "other"

	ReportOpen          = "open"
	ReportInvestigating = "investigating"
	ReportResolved      = "resolved"
	ReportRejected      = "rejected"

	StateVisible  = "visible"
	StateHidden   = "hidden"
	StateNormal   = "normal"
	StateDisabled = "disabled"

	ActionHideVideo      = "hide_video"
	ActionRestoreVideo   = "restore_video"
	ActionHideComment    = "hide_comment"
	ActionRestoreComment = "restore_comment"
	ActionDisableUser    = "disable_user"
	ActionEnableUser     = "enable_user"
	ActionAssignReport   = "assign_report"
	ActionResolveReport  = "resolve_report"
	ActionRejectReport   = "reject_report"
)

type Report struct {
	ID               uint64  `gorm:"primaryKey;autoIncrement"`
	ReporterID       uint64  `gorm:"not null;index"`
	TargetType       string  `gorm:"type:varchar(16);not null"`
	TargetID         uint64  `gorm:"not null"`
	ReasonCode       string  `gorm:"type:varchar(32);not null"`
	Detail           string  `gorm:"type:varchar(500);not null"`
	Status           string  `gorm:"type:varchar(20);not null"`
	AssignedTo       *uint64 `gorm:"index"`
	ResolutionReason *string `gorm:"type:varchar(500)"`
	ActionID         *uint64
	ActiveKey        *string   `gorm:"type:varchar(128);uniqueIndex"`
	CreatedAt        time.Time `gorm:"not null"`
	ClosedAt         *time.Time
}

func (Report) TableName() string { return "reports" }

type ModerationAction struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement"`
	ActorID     uint64    `gorm:"not null;index"`
	ReportID    *uint64   `gorm:"index"`
	TargetType  string    `gorm:"type:varchar(16);not null"`
	TargetID    uint64    `gorm:"not null"`
	Action      string    `gorm:"type:varchar(32);not null"`
	Reason      string    `gorm:"type:varchar(500);not null"`
	BeforeState string    `gorm:"type:varchar(32);not null"`
	AfterState  string    `gorm:"type:varchar(32);not null"`
	RequestID   string    `gorm:"type:varchar(128);not null"`
	CreatedAt   time.Time `gorm:"not null"`
}

func (ModerationAction) TableName() string { return "moderation_actions" }
