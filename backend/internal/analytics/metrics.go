package analytics

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

// RecordVideoDeltaTx records a real relationship change in both the video and
// creator daily ledgers. Callers must invoke it only after the relation write
// succeeds, inside the same transaction.
func RecordVideoDeltaTx(tx *gorm.DB, videoID uint64, metric string, delta int64, now time.Time) error {
	if videoID == 0 || delta == 0 {
		return nil
	}
	var creatorID uint64
	if err := tx.Table("videos").Where("id = ?", videoID).Pluck("user_id", &creatorID).Error; err != nil {
		return fmt.Errorf("load metric creator: %w", err)
	}
	if creatorID == 0 {
		return fmt.Errorf("metric creator missing for video %d", videoID)
	}
	column, ok := metricColumn(metric)
	if !ok {
		return fmt.Errorf("unsupported daily metric %q", metric)
	}
	date := dateInShanghai(now)
	query := fmt.Sprintf(`INSERT INTO video_daily_metrics (video_id, stat_date, %s) VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE %s = %s + VALUES(%s)`, column, column, column, column)
	if err := tx.Exec(query, videoID, date, delta).Error; err != nil {
		return fmt.Errorf("record video daily metric: %w", err)
	}
	return recordCreatorDeltaTx(tx, creatorID, column, delta, date)
}

func RecordFollowDeltaTx(tx *gorm.DB, creatorID uint64, delta int64, now time.Time) error {
	if creatorID == 0 || delta == 0 {
		return nil
	}
	return recordCreatorDeltaTx(tx, creatorID, "net_followers", delta, dateInShanghai(now))
}

func recordCreatorDeltaTx(tx *gorm.DB, creatorID uint64, column string, delta int64, date time.Time) error {
	query := fmt.Sprintf(`INSERT INTO creator_daily_metrics (creator_id, stat_date, %s) VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE %s = %s + VALUES(%s)`, column, column, column, column)
	if err := tx.Exec(query, creatorID, date, delta).Error; err != nil {
		return fmt.Errorf("record creator daily metric: %w", err)
	}
	return nil
}

func metricColumn(metric string) (string, bool) {
	switch metric {
	case "net_likes", "net_favorites", "net_comments":
		return metric, true
	default:
		return "", false
	}
}

func upsertPlaybackMetric(tx *gorm.DB, videoID, creatorID uint64, date time.Time, views, watchTime, completions uint64) error {
	if err := tx.Exec(`INSERT INTO video_daily_metrics (video_id, stat_date, effective_views, watch_time_ms, completions)
		VALUES (?, ?, ?, ?, ?) ON DUPLICATE KEY UPDATE effective_views = effective_views + VALUES(effective_views), watch_time_ms = watch_time_ms + VALUES(watch_time_ms), completions = completions + VALUES(completions)`, videoID, date, views, watchTime, completions).Error; err != nil {
		return fmt.Errorf("record video playback metric: %w", err)
	}
	if err := tx.Exec(`INSERT INTO creator_daily_metrics (creator_id, stat_date, effective_views, watch_time_ms, completions)
		VALUES (?, ?, ?, ?, ?) ON DUPLICATE KEY UPDATE effective_views = effective_views + VALUES(effective_views), watch_time_ms = watch_time_ms + VALUES(watch_time_ms), completions = completions + VALUES(completions)`, creatorID, date, views, watchTime, completions).Error; err != nil {
		return fmt.Errorf("record creator playback metric: %w", err)
	}
	return nil
}
