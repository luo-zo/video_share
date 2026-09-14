package video

import "time"

// Status is the persisted lifecycle state of a video submission.
type Status uint8

const (
	StatusUploading  Status = 1
	StatusReady      Status = 2
	StatusFailed     Status = 3
	StatusDeleted    Status = 4
	StatusProcessing Status = 5
)

func (s Status) String() string {
	switch s {
	case StatusUploading:
		return "uploading"
	case StatusReady:
		return "ready"
	case StatusFailed:
		return "failed"
	case StatusDeleted:
		return "deleted"
	case StatusProcessing:
		return "processing"
	default:
		return "unknown"
	}
}

// Visibility controls whether a processed video can be discovered and played
// through public endpoints.
type Visibility uint8

const (
	VisibilityPublic  Visibility = 1
	VisibilityPrivate Visibility = 2
)

func (v Visibility) String() string {
	switch v {
	case VisibilityPublic:
		return "public"
	case VisibilityPrivate:
		return "private"
	default:
		return "unknown"
	}
}

// Author contains the public owner fields returned with video discovery data.
type Author struct {
	ID       uint64
	Username string
	Nickname string
}

// Stats is the public aggregate read model for one video.
type Stats struct {
	ViewCount     uint64 `json:"view_count"`
	LikeCount     uint64 `json:"like_count"`
	FavoriteCount uint64 `json:"favorite_count"`
	CommentCount  uint64 `json:"comment_count"`
}

// ViewerState describes relationships belonging to the current viewer. It is
// omitted from anonymous responses.
type ViewerState struct {
	Liked           bool `json:"liked"`
	Favorited       bool `json:"favorited"`
	FollowingAuthor bool `json:"following_author"`
}

// Video is the persisted metadata for one uploaded object. ObjectKey is internal
// storage data and must never be copied directly into an HTTP response.
type Video struct {
	ID                 uint64     `gorm:"primaryKey;autoIncrement"`
	UserID             uint64     `gorm:"not null;index"`
	Title              string     `gorm:"type:varchar(100);not null"`
	Description        string     `gorm:"type:text;not null"`
	ObjectKey          string     `gorm:"type:varchar(512);not null"`
	Status             Status     `gorm:"type:tinyint;not null;default:1;index"`
	Visibility         Visibility `gorm:"type:tinyint;not null;default:1;index"`
	FileSize           int64      `gorm:"not null"`
	ContentType        string     `gorm:"type:varchar(100);not null"`
	HLSMasterKey       *string    `gorm:"type:varchar(512)"`
	CoverObjectKey     *string    `gorm:"type:varchar(512)"`
	DurationMS         *uint64    `gorm:"type:bigint unsigned"`
	Width              *uint      `gorm:"type:int unsigned"`
	Height             *uint      `gorm:"type:int unsigned"`
	ProcessingProgress uint8      `gorm:"type:tinyint unsigned;not null;default:0"`
	ProcessingError    *string    `gorm:"type:varchar(1000)"`
	ProcessedAt        *time.Time `gorm:"type:datetime(3)"`
	PublishedAt        *time.Time `gorm:"type:datetime(3)"`
	CreatedAt          time.Time  `gorm:"not null"`
	UpdatedAt          time.Time  `gorm:"not null"`

	Author      Author       `gorm:"-"`
	Stats       Stats        `gorm:"-"`
	ViewerState *ViewerState `gorm:"-"`
}

func (Video) TableName() string { return "videos" }
