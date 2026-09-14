package messaging

import "time"

const (
	TranscodeRequestedType = "video.transcode.requested"
	EventSchemaVersion     = 1
)

// TranscodeRequested is the stable, versioned payload sent through Kafka.
type TranscodeRequested struct {
	EventID         string    `json:"event_id"`
	JobID           string    `json:"job_id"`
	VideoID         uint64    `json:"video_id"`
	UserID          uint64    `json:"user_id"`
	SourceObjectKey string    `json:"source_object_key"`
	RequestedAt     time.Time `json:"requested_at"`
	SchemaVersion   int       `json:"schema_version"`
}
