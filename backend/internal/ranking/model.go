package ranking

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"video_share/internal/video"
)

// Window is the calendar period used by a public ranking snapshot. Calendar
// boundaries are always evaluated in Asia/Shanghai, independent of the host
// process timezone.
type Window string

const (
	WindowDay  Window = "day"
	WindowWeek Window = "week"
)

var (
	ErrInvalidWindow = errors.New("invalid ranking window")
	ErrLockBusy      = errors.New("ranking rebuild already running")
	ErrLockLost      = errors.New("ranking rebuild lock expired")
	ErrFallbackBusy  = errors.New("ranking mysql fallback is busy")
	ErrTooManyRows   = errors.New("ranking metrics exceed safety limit")
)

func ParseWindow(value string) (Window, error) {
	switch Window(value) {
	case WindowDay, WindowWeek:
		return Window(value), nil
	default:
		return "", ErrInvalidWindow
	}
}

func (w Window) String() string { return string(w) }

func (w Window) start(now time.Time) time.Time {
	zone := time.FixedZone("Asia/Shanghai", 8*60*60)
	local := now.In(zone)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, zone)
	if w == WindowWeek {
		return start.AddDate(0, 0, -6)
	}
	return start
}

// Score is the versioned initial ranking formula from the task contract.
// Watch time and completions remain available in MySQL for analytics but do
// not silently change the public ranking score. Negative net changes stay in
// the trend tables and are clamped only for this score.
func Score(effectiveViews, _watchTimeMS, _completions uint64, netLikes, netFavorites, netComments int64) float64 {
	positive := func(value int64) float64 {
		if value < 0 {
			return 0
		}
		return float64(value)
	}
	return float64(effectiveViews) + 3*positive(netLikes) + 5*positive(netFavorites) + 2*positive(netComments)
}

// Redis member encoding makes the secondary order (video id descending)
// deterministic for equal scores because ZREVRANGE orders equal-score members
// in reverse lexicographical order.
func encodeMember(id uint64) string { return fmt.Sprintf("%020d", id) }

func decodeMember(member string) (uint64, error) {
	value, err := strconv.ParseUint(member, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("decode ranking member: %w", err)
	}
	return value, nil
}

type Candidate struct {
	VideoID uint64
	Score   float64
	Video   video.Video
}

type RankingItem struct {
	video.VideoResponse
	VideoID uint64  `json:"video_id"`
	Rank    int     `json:"rank"`
	Score   float64 `json:"score"`
}

type Response struct {
	Window      Window        `json:"window"`
	GeneratedAt time.Time     `json:"generated_at"`
	Page        int           `json:"page"`
	PageSize    int           `json:"page_size"`
	Total       int64         `json:"total"`
	Items       []RankingItem `json:"items"`
}
