package analytics

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"video_share/internal/user"
	"video_share/internal/video"
)

var (
	ErrUnauthorized      = errors.New("unauthorized")
	ErrVideoNotFound     = errors.New("video not found")
	ErrSessionNotFound   = errors.New("watch session not found")
	ErrSessionExpired    = errors.New("watch session expired")
	ErrHeartbeatInvalid  = errors.New("invalid watch heartbeat")
	ErrPaginationInvalid = errors.New("invalid analytics range")
)

type Repository interface {
	StartSession(ctx context.Context, userID, videoID uint64, now time.Time) (*startResult, error)
	Heartbeat(ctx context.Context, userID uint64, sessionID string, input HeartbeatInput, now time.Time) (*heartbeatResult, error)
	CreatorSummary(ctx context.Context, creatorID uint64, days int, now time.Time) (CreatorSummaryResponse, error)
}

type gormRepository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) Repository { return &gormRepository{db: db} }

func (r *gormRepository) StartSession(ctx context.Context, userID, videoID uint64, now time.Time) (*startResult, error) {
	if userID == 0 {
		return nil, ErrUnauthorized
	}
	var result startResult
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row struct {
			ID         uint64
			DurationMS *uint64
		}
		if err := tx.Table("videos AS v").Select("v.id, v.duration_ms").Joins("JOIN users AS u ON u.id = v.user_id").
			Where("v.id = ? AND v.status = ? AND v.visibility = ? AND v.moderation_status = ? AND u.status = ?", videoID, video.StatusReady, video.VisibilityPublic, "visible", user.StatusNormal).
			Take(&row).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrVideoNotFound
		} else if err != nil {
			return fmt.Errorf("load watch video: %w", err)
		}
		duration := uint64(0)
		if row.DurationMS != nil {
			duration = *row.DurationMS
		}
		var history struct {
			ProgressMS uint64
			DurationMS uint64
		}
		if err := tx.Table("watch_histories").Where("user_id = ? AND video_id = ?", userID, videoID).Take(&history).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("load watch history: %w", err)
		}
		resume := history.ProgressMS
		if duration > 0 && resume+2000 >= duration {
			resume = 0
		}
		if duration > 0 && resume > duration {
			resume = 0
		}
		session := WatchSession{ID: uuid.NewString(), UserID: userID, VideoID: videoID, CreatedAt: now, ExpiresAt: now.Add(24 * time.Hour), DurationMS: duration, LastReceivedAt: now}
		if err := tx.Create(&session).Error; err != nil {
			return fmt.Errorf("create watch session: %w", err)
		}
		result = startResult{Session: session, DurationMS: duration, ResumeMS: resume}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *gormRepository) Heartbeat(ctx context.Context, userID uint64, sessionID string, input HeartbeatInput, now time.Time) (*heartbeatResult, error) {
	if userID == 0 {
		return nil, ErrUnauthorized
	}
	if sessionID == "" || input.Seq == 0 || input.WatchedDeltaMS > 15*1000 {
		return nil, ErrHeartbeatInvalid
	}
	var result heartbeatResult
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var session WatchSession
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", sessionID, userID).Take(&session).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrSessionNotFound
		} else if err != nil {
			return fmt.Errorf("lock watch session: %w", err)
		}
		if !now.Before(session.ExpiresAt) {
			return ErrSessionExpired
		}
		if input.PositionMS > session.DurationMS && session.DurationMS != 0 {
			return ErrHeartbeatInvalid
		}
		// A duplicate or out-of-order heartbeat is acknowledged as a no-op.
		if input.Seq <= session.LastSeq {
			var history struct{ EffectiveWatchMS uint64 }
			_ = tx.Table("watch_histories").Select("effective_watch_ms").Where("user_id = ? AND video_id = ?", userID, session.VideoID).Take(&history).Error
			result = heartbeatResult{Session: session, EffectiveWatchMS: history.EffectiveWatchMS}
			return nil
		}
		var current struct {
			ID         uint64
			UserID     uint64
			DurationMS *uint64
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Table("videos AS v").Select("v.id, v.user_id, v.duration_ms").Joins("JOIN users AS u ON u.id = v.user_id").Where("v.id = ? AND v.status = ? AND v.visibility = ? AND v.moderation_status = ? AND u.status = ?", session.VideoID, video.StatusReady, video.VisibilityPublic, "visible", user.StatusNormal).Take(&current).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrVideoNotFound
		} else if err != nil {
			return fmt.Errorf("lock heartbeat video: %w", err)
		}
		duration := session.DurationMS
		if current.DurationMS != nil {
			duration = *current.DurationMS
		}
		if duration != 0 && input.PositionMS > duration {
			return ErrHeartbeatInvalid
		}
		var history struct {
			UserID            uint64
			VideoID           uint64
			ProgressMS        uint64
			DurationMS        uint64
			FirstWatchedAt    time.Time
			LastWatchedAt     time.Time
			EffectiveWatchMS  uint64
			LastWatchCreditAt *time.Time
		}
		historyErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Table("watch_histories").Where("user_id = ? AND video_id = ?", userID, session.VideoID).Take(&history).Error
		if errors.Is(historyErr, gorm.ErrRecordNotFound) {
			history = struct {
				UserID            uint64
				VideoID           uint64
				ProgressMS        uint64
				DurationMS        uint64
				FirstWatchedAt    time.Time
				LastWatchedAt     time.Time
				EffectiveWatchMS  uint64
				LastWatchCreditAt *time.Time
			}{UserID: userID, VideoID: session.VideoID, FirstWatchedAt: now, LastWatchedAt: now}
		} else if historyErr != nil {
			return fmt.Errorf("lock watch history: %w", historyErr)
		}
		elapsed := now.Sub(session.LastReceivedAt)
		if elapsed < 0 {
			elapsed = 0
		}
		accepted := minUint64(input.WatchedDeltaMS, uint64(elapsed/time.Millisecond), 15*1000)
		if history.LastWatchCreditAt != nil {
			globalElapsed := now.Sub(*history.LastWatchCreditAt)
			if globalElapsed < 0 {
				globalElapsed = 0
			}
			accepted = minUint64(accepted, uint64(globalElapsed/time.Millisecond))
		}
		if historyErr != nil {
			var creditAt any
			if accepted > 0 {
				creditAt = now
			}
			if err := tx.Exec(`INSERT INTO watch_histories (user_id, video_id, progress_ms, duration_ms, first_watched_at, last_watched_at, effective_watch_ms, last_watch_credit_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, userID, session.VideoID, input.PositionMS, duration, now, now, accepted, creditAt).Error; err != nil {
				return fmt.Errorf("create watch history: %w", err)
			}
			// Preserve the legacy logged-in user/video counter: the first
			// history row is the one-time old view, independent of the new
			// effective-watch qualification threshold.
			if err := tx.Exec(`UPDATE video_stats SET view_count = view_count + 1, updated_at = UTC_TIMESTAMP(3) WHERE video_id = ?`, session.VideoID).Error; err != nil {
				return fmt.Errorf("increment legacy views: %w", err)
			}
		} else {
			updates := map[string]any{
				"progress_ms": input.PositionMS, "duration_ms": duration, "last_watched_at": now,
			}
			if accepted > 0 {
				updates["effective_watch_ms"] = gorm.Expr("effective_watch_ms + ?", accepted)
				updates["last_watch_credit_at"] = now
			}
			if err := tx.Table("watch_histories").Where("user_id = ? AND video_id = ?", userID, session.VideoID).Updates(updates).Error; err != nil {
				return fmt.Errorf("update watch history: %w", err)
			}
		}
		effective := history.EffectiveWatchMS + accepted
		sessionCredited := session.CreditedMS + accepted
		// Qualification and completion belong to this playback session. Using
		// the shared history total here would let a second tab qualify without
		// receiving any credit after the cross-tab cap suppressed its delta.
		qualified := session.QualifiedAt != nil || (duration > 0 && sessionCredited >= minUint64(3000, duration))
		completed := session.CompletedAt != nil || (input.Ended && duration > 0 && input.PositionMS+2000 >= duration && sessionCredited*100 >= duration*80)
		if historyErr != nil {
			history.EffectiveWatchMS = effective
		}
		if qualified && session.QualifiedAt == nil {
			qualifiedAt := now
			session.QualifiedAt = &qualifiedAt
			cohort := dateInShanghai(now)
			session.CohortDate = &cohort
			if err := upsertPlaybackMetric(tx, session.VideoID, current.UserID, cohort, 1, 0, 0); err != nil {
				return err
			}
		}
		if completed && session.CompletedAt == nil {
			completedAt := now
			session.CompletedAt = &completedAt
			completionDate := dateInShanghai(now)
			if session.CohortDate != nil {
				completionDate = *session.CohortDate
			}
			if err := upsertPlaybackMetric(tx, session.VideoID, current.UserID, completionDate, 0, 0, 1); err != nil {
				return err
			}
		}
		if accepted > 0 {
			if err := upsertPlaybackMetric(tx, session.VideoID, current.UserID, dateInShanghai(now), 0, accepted, 0); err != nil {
				return err
			}
		}
		session.LastSeq = input.Seq
		session.LastPositionMS = input.PositionMS
		session.DurationMS = duration
		session.LastReceivedAt = now
		session.CreditedMS += accepted
		if err := tx.Model(&WatchSession{}).Where("id = ?", session.ID).Updates(map[string]any{
			"last_seq": session.LastSeq, "last_position_ms": session.LastPositionMS, "duration_ms": session.DurationMS, "last_received_at": session.LastReceivedAt, "credited_ms": session.CreditedMS, "qualified_at": session.QualifiedAt, "completed_at": session.CompletedAt, "cohort_date": session.CohortDate,
		}).Error; err != nil {
			return fmt.Errorf("update watch session: %w", err)
		}
		result = heartbeatResult{Session: session, AcceptedDeltaMS: accepted, EffectiveWatchMS: effective}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *gormRepository) CreatorSummary(ctx context.Context, creatorID uint64, days int, now time.Time) (CreatorSummaryResponse, error) {
	if creatorID == 0 {
		return CreatorSummaryResponse{}, ErrUnauthorized
	}
	if days != 7 && days != 30 {
		return CreatorSummaryResponse{}, ErrPaginationInvalid
	}
	start := dateInShanghai(now).AddDate(0, 0, -(days - 1))
	var current CreatorCurrent
	creatorQuery := r.db.WithContext(ctx).Table("users AS u").Select(`
		(SELECT COUNT(*) FROM videos WHERE user_id = u.id AND status = ? AND visibility = ? AND moderation_status = 'visible') AS video_count,
		(SELECT COALESCE(SUM(s.view_count),0) FROM videos v JOIN video_stats s ON s.video_id = v.id WHERE v.user_id = u.id AND v.status = ? AND v.visibility = ? AND v.moderation_status = 'visible') AS view_count,
		(SELECT COALESCE(SUM(s.like_count),0) FROM videos v JOIN video_stats s ON s.video_id = v.id WHERE v.user_id = u.id AND v.status = ? AND v.visibility = ? AND v.moderation_status = 'visible') AS like_count,
		(SELECT COALESCE(SUM(s.favorite_count),0) FROM videos v JOIN video_stats s ON s.video_id = v.id WHERE v.user_id = u.id AND v.status = ? AND v.visibility = ? AND v.moderation_status = 'visible') AS favorite_count,
		(SELECT COALESCE(SUM(s.comment_count),0) FROM videos v JOIN video_stats s ON s.video_id = v.id WHERE v.user_id = u.id AND v.status = ? AND v.visibility = ? AND v.moderation_status = 'visible') AS comment_count,
		(SELECT COUNT(*) FROM user_follows f JOIN users follower ON follower.id = f.follower_id WHERE f.followee_id = u.id AND follower.status = ?) AS follower_count`,
		video.StatusReady, video.VisibilityPublic, video.StatusReady, video.VisibilityPublic, video.StatusReady, video.VisibilityPublic, video.StatusReady, video.VisibilityPublic, video.StatusReady, video.VisibilityPublic, user.StatusNormal).
		Where("u.id = ? AND u.status = ?", creatorID, user.StatusNormal).Scan(&current)
	if creatorQuery.Error != nil {
		return CreatorSummaryResponse{}, fmt.Errorf("load creator totals: %w", creatorQuery.Error)
	}
	if creatorQuery.RowsAffected == 0 {
		return CreatorSummaryResponse{}, ErrUnauthorized
	}
	var trendRows []struct {
		StatDate       time.Time
		EffectiveViews uint64
		WatchTimeMS    uint64
		Completions    uint64
		NetLikes       int64
		NetFavorites   int64
		NetComments    int64
		NetFollowers   int64
	}
	if err := r.db.WithContext(ctx).Table("creator_daily_metrics").Select("stat_date, effective_views, watch_time_ms, completions, net_likes, net_favorites, net_comments, net_followers").Where("creator_id = ? AND stat_date >= ?", creatorID, start).Order("stat_date ASC").Scan(&trendRows).Error; err != nil {
		return CreatorSummaryResponse{}, fmt.Errorf("load creator trend: %w", err)
	}
	trend := make([]TrendPoint, 0, len(trendRows))
	for _, row := range trendRows {
		trend = append(trend, TrendPoint{Date: row.StatDate.Format("2006-01-02"), EffectiveViews: row.EffectiveViews, WatchTimeMS: row.WatchTimeMS, Completions: row.Completions, NetLikes: row.NetLikes, NetFavorites: row.NetFavorites, NetComments: row.NetComments, NetFollowers: row.NetFollowers})
	}
	var topRows []TopVideo
	if err := r.db.WithContext(ctx).Table("videos AS v").Select("v.id AS video_id, v.title, COALESCE(SUM(m.effective_views),0) AS effective_views, COALESCE(SUM(m.watch_time_ms),0) AS watch_time_ms, COALESCE(SUM(m.completions),0) AS completions").Joins("LEFT JOIN video_daily_metrics m ON m.video_id = v.id AND m.stat_date >= ?", start).Where("v.user_id = ? AND v.status = ? AND v.visibility = ? AND v.moderation_status = ?", creatorID, video.StatusReady, video.VisibilityPublic, "visible").Group("v.id, v.title").Order("effective_views DESC, v.id DESC").Limit(10).Scan(&topRows).Error; err != nil {
		return CreatorSummaryResponse{}, fmt.Errorf("load creator top videos: %w", err)
	}
	return CreatorSummaryResponse{Days: days, MetricVersion: "effective-watch-v1", AsOf: now, Current: current, Trend: trend, TopVideos: topRows}, nil
}

func minUint64(values ...uint64) uint64 {
	if len(values) == 0 {
		return 0
	}
	value := values[0]
	for _, candidate := range values[1:] {
		if candidate < value {
			value = candidate
		}
	}
	return value
}

func dateInShanghai(now time.Time) time.Time {
	zone := time.FixedZone("Asia/Shanghai", 8*60*60)
	local := now.In(zone)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, zone)
}
