package creator

import (
	"time"

	"video_share/internal/video"
)

// ProfileResponse contains only public account data and aggregate relations.
// Password hashes, status, roles and session identifiers are intentionally not
// represented by this type.
type ProfileResponse struct {
	ID             uint64    `json:"id"`
	Username       string    `json:"username"`
	Nickname       string    `json:"nickname"`
	Bio            string    `json:"bio"`
	CreatedAt      time.Time `json:"created_at"`
	VideoCount     int64     `json:"video_count"`
	FollowerCount  int64     `json:"follower_count"`
	FollowingCount int64     `json:"following_count"`
	Following      bool      `json:"following"`
}

type VideoListResponse struct {
	Items    []video.VideoResponse `json:"items"`
	Page     int                   `json:"page"`
	PageSize int                   `json:"page_size"`
	Total    int64                 `json:"total"`
}
