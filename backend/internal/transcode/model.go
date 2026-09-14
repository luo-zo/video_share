package transcode

import "time"

type JobStatus uint8

const (
	JobPending   JobStatus = 1
	JobQueued    JobStatus = 2
	JobRunning   JobStatus = 3
	JobSucceeded JobStatus = 4
	JobFailed    JobStatus = 5
)

type Job struct {
	ID             uint64    `gorm:"primaryKey"`
	JobID          string    `gorm:"type:char(36);not null;uniqueIndex"`
	VideoID        uint64    `gorm:"not null;index"`
	IdempotencyKey string    `gorm:"type:varchar(128);not null;uniqueIndex"`
	Status         JobStatus `gorm:"type:tinyint;not null"`
	Attempts       uint      `gorm:"not null"`
	MaxAttempts    uint      `gorm:"not null"`
	NextRetryAt    time.Time `gorm:"not null"`
	WorkerID       *string
	LastError      *string
	StartedAt      *time.Time
	FinishedAt     *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (Job) TableName() string { return "video_transcode_jobs" }

type Output struct {
	HLSMasterKey   string
	CoverObjectKey string
	DurationMS     uint64
	Width          uint
	Height         uint
}
