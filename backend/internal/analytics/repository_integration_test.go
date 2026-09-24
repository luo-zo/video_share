//go:build integration

package analytics

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"video_share/internal/database"
	"video_share/internal/testutil"
)

func openAnalyticsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(mysql.Open(testutil.MySQLDSN(t)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	migrator, err := database.NewMigrator(sqlDB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := migrator.Up(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func TestWatchSessionCreditsOnceAndSharesCapAcrossTabs(t *testing.T) {
	db := openAnalyticsTestDB(t)
	ctx := context.Background()
	username := fmt.Sprintf("analytics_%d", time.Now().UnixNano())
	if err := db.Exec("INSERT INTO users (username,password_hash,nickname,status) VALUES (?, 'hash', '数据作者', 1)", username).Error; err != nil {
		t.Fatal(err)
	}
	var userID uint64
	if err := db.Raw("SELECT id FROM users WHERE username = ?", username).Scan(&userID).Error; err != nil {
		t.Fatal(err)
	}
	objectKey := fmt.Sprintf("analytics/%d.mp4", time.Now().UnixNano())
	if err := db.Exec(`INSERT INTO videos (user_id,title,description,object_key,status,visibility,moderation_status,file_size,content_type,duration_ms,published_at)
		VALUES (?, '统计视频', '', ?, 2, 1, 'visible', 1, 'video/mp4', 10000, CURRENT_TIMESTAMP(3))`, userID, objectKey).Error; err != nil {
		t.Fatal(err)
	}
	var videoID uint64
	if err := db.Raw("SELECT id FROM videos WHERE object_key = ?", objectKey).Scan(&videoID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO video_stats (video_id) VALUES (?)", videoID).Error; err != nil {
		t.Fatal(err)
	}
	// Start just before midnight in Asia/Shanghai so qualification and
	// completion exercise the session cohort-date rule across two dates.
	base := time.Date(2026, 9, 22, 15, 59, 53, 0, time.UTC)
	service := NewService(NewRepository(db), WithClock(func() time.Time { return base }))
	started, err := service.StartSession(ctx, userID, videoID)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if started.DurationMS != 10000 || started.ResumePositionMS != 0 {
		t.Fatalf("start response = %+v", started)
	}
	base = base.Add(5 * time.Second)
	first, err := service.Heartbeat(ctx, userID, started.SessionID, HeartbeatInput{Seq: 1, PositionMS: 5000, WatchedDeltaMS: 5000})
	if err != nil {
		t.Fatalf("first heartbeat: %v", err)
	}
	if first.AcceptedDeltaMS != 5000 || !first.Qualified || first.Completed {
		t.Fatalf("first heartbeat = %+v", first)
	}
	base = base.Add(5 * time.Second)
	duplicate, err := service.Heartbeat(ctx, userID, started.SessionID, HeartbeatInput{Seq: 1, PositionMS: 10000, WatchedDeltaMS: 5000})
	if err != nil {
		t.Fatalf("duplicate heartbeat: %v", err)
	}
	if duplicate.AcceptedDeltaMS != 0 || duplicate.EffectiveWatchMS != 5000 {
		t.Fatalf("duplicate heartbeat = %+v", duplicate)
	}
	completed, err := service.Heartbeat(ctx, userID, started.SessionID, HeartbeatInput{Seq: 2, PositionMS: 10000, WatchedDeltaMS: 5000, Ended: true})
	if err != nil {
		t.Fatalf("completion heartbeat: %v", err)
	}
	if completed.AcceptedDeltaMS != 5000 || !completed.Completed {
		t.Fatalf("completion heartbeat = %+v", completed)
	}

	second, err := service.StartSession(ctx, userID, videoID)
	if err != nil {
		t.Fatalf("start second tab: %v", err)
	}
	if second.ResumePositionMS != 0 {
		t.Fatalf("near-end resume=%d, want reset", second.ResumePositionMS)
	}
	base = base.Add(5 * time.Second)
	paused, err := service.Heartbeat(ctx, userID, second.SessionID, HeartbeatInput{Seq: 1, PositionMS: 0, WatchedDeltaMS: 0})
	if err != nil {
		t.Fatalf("paused second tab heartbeat: %v", err)
	}
	if paused.AcceptedDeltaMS != 0 {
		t.Fatalf("paused second tab credited=%d, want zero", paused.AcceptedDeltaMS)
	}
	active, err := service.Heartbeat(ctx, userID, started.SessionID, HeartbeatInput{Seq: 3, PositionMS: 10000, WatchedDeltaMS: 5000})
	if err != nil {
		t.Fatalf("late heartbeat: %v", err)
	}
	if active.AcceptedDeltaMS != 5000 {
		t.Fatalf("active tab credited=%d, want 5000 after paused tab heartbeat", active.AcceptedDeltaMS)
	}
	shared, err := service.Heartbeat(ctx, userID, second.SessionID, HeartbeatInput{Seq: 2, PositionMS: 3000, WatchedDeltaMS: 5000})
	if err != nil {
		t.Fatalf("second tab heartbeat: %v", err)
	}
	if shared.AcceptedDeltaMS != 0 {
		t.Fatalf("second tab credited=%d, want shared cap to suppress duplicate wall time", shared.AcceptedDeltaMS)
	}

	var views uint64
	if err := db.Raw("SELECT view_count FROM video_stats WHERE video_id = ?", videoID).Scan(&views).Error; err != nil {
		t.Fatal(err)
	}
	if views != 1 {
		t.Fatalf("view_count=%d, want one qualified session", views)
	}
	summary, err := service.CreatorSummary(ctx, userID, 7)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if summary.Current.ViewCount != 1 || len(summary.TopVideos) != 1 || len(summary.Trend) == 0 || summary.Trend[0].EffectiveViews != 1 || summary.Trend[0].Completions != 1 {
		t.Fatalf("summary = %+v", summary)
	}

	if _, err := service.Heartbeat(ctx, userID, second.SessionID, HeartbeatInput{Seq: 0, PositionMS: 0}); !errors.Is(err, ErrHeartbeatInvalid) {
		t.Fatalf("seq zero = %v", err)
	}
	if _, err := service.Heartbeat(ctx, userID, second.SessionID, HeartbeatInput{Seq: 2, PositionMS: 0, WatchedDeltaMS: 15001}); !errors.Is(err, ErrHeartbeatInvalid) {
		t.Fatalf("delta over cap = %v", err)
	}

	// A new session cannot revive an account/video that is no longer public.
	if err := db.Exec("UPDATE videos SET moderation_status = 'hidden' WHERE id = ?", videoID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartSession(ctx, userID, videoID); !errors.Is(err, ErrVideoNotFound) {
		t.Fatalf("hidden video session = %v", err)
	}
}
