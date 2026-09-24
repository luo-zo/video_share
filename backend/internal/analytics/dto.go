package analytics

import "time"

type HeartbeatInput struct {
	Seq            uint64 `json:"seq"`
	PositionMS     uint64 `json:"position_ms"`
	WatchedDeltaMS uint64 `json:"watched_delta_ms"`
	Ended          bool   `json:"ended"`
}

type StartResponse struct {
	SessionID        string    `json:"session_id"`
	VideoID          uint64    `json:"video_id"`
	DurationMS       uint64    `json:"duration_ms"`
	ResumePositionMS uint64    `json:"resume_position_ms"`
	ExpiresAt        time.Time `json:"expires_at"`
}

type HeartbeatResponse struct {
	SessionID         string `json:"session_id"`
	Seq               uint64 `json:"seq"`
	PositionMS        uint64 `json:"position_ms"`
	AcceptedDeltaMS   uint64 `json:"accepted_delta_ms"`
	SessionCreditedMS uint64 `json:"session_credited_ms"`
	EffectiveWatchMS  uint64 `json:"effective_watch_ms"`
	Qualified         bool   `json:"qualified"`
	Completed         bool   `json:"completed"`
}

type CreatorCurrent struct {
	VideoCount    int64 `json:"video_count"`
	ViewCount     int64 `json:"view_count"`
	LikeCount     int64 `json:"like_count"`
	FavoriteCount int64 `json:"favorite_count"`
	CommentCount  int64 `json:"comment_count"`
	FollowerCount int64 `json:"follower_count"`
}

type TrendPoint struct {
	Date           string `json:"date"`
	EffectiveViews uint64 `json:"effective_views"`
	WatchTimeMS    uint64 `json:"watch_time_ms"`
	Completions    uint64 `json:"completions"`
	NetLikes       int64  `json:"net_likes"`
	NetFavorites   int64  `json:"net_favorites"`
	NetComments    int64  `json:"net_comments"`
	NetFollowers   int64  `json:"net_followers"`
}

type TopVideo struct {
	VideoID        uint64 `json:"video_id"`
	Title          string `json:"title"`
	EffectiveViews uint64 `json:"effective_views"`
	WatchTimeMS    uint64 `json:"watch_time_ms"`
	Completions    uint64 `json:"completions"`
}

type CreatorSummaryResponse struct {
	Days          int            `json:"days"`
	MetricVersion string         `json:"metric_version"`
	AsOf          time.Time      `json:"as_of"`
	Current       CreatorCurrent `json:"current"`
	Trend         []TrendPoint   `json:"trend"`
	TopVideos     []TopVideo     `json:"top_videos"`
}

type startResult struct {
	Session    WatchSession
	DurationMS uint64
	ResumeMS   uint64
}

type heartbeatResult struct {
	Session          WatchSession
	AcceptedDeltaMS  uint64
	EffectiveWatchMS uint64
}
