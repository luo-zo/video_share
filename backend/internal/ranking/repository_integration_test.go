//go:build integration

package ranking

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"video_share/internal/cache"
	"video_share/internal/database"
	"video_share/internal/testutil"
)

func openRankingTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(mysql.Open(testutil.MySQLDSN(t)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open mysql: %v", err)
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

func TestRankingRebuildFiltersVisibilityAndFallsBackAfterRedisLoss(t *testing.T) {
	db := openRankingTestDB(t)
	redisConfig := testutil.RedisConfig(t)
	redisClient := cache.NewClient(cache.Options{Addr: redisConfig.Addr, Password: redisConfig.Password, DB: redisConfig.DB, CommandTimeout: 2 * time.Second})
	defer redisClient.Close()
	ctx := context.Background()
	if err := redisClient.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	dayKey := snapshotKey(WindowDay, time.Date(2026, 9, 23, 4, 0, 0, 0, time.UTC))
	_ = redisClient.Delete(ctx, dayKey, metadataKey(WindowDay, time.Date(2026, 9, 23, 4, 0, 0, 0, time.UTC)), snapshotKey(WindowWeek, time.Date(2026, 9, 23, 4, 0, 0, 0, time.UTC)), metadataKey(WindowWeek, time.Date(2026, 9, 23, 4, 0, 0, 0, time.UTC)), keyPrefix+"lock")

	username := fmt.Sprintf("ranking_%d", time.Now().UnixNano())
	if err := db.Exec("INSERT INTO users (username,password_hash,nickname,status) VALUES (?, 'hash', '榜单作者', 1)", username).Error; err != nil {
		t.Fatal(err)
	}
	var userID uint64
	if err := db.Raw("SELECT id FROM users WHERE username = ?", username).Scan(&userID).Error; err != nil {
		t.Fatal(err)
	}
	insertVideo := func(title string, status, visibility int, moderation string) uint64 {
		objectKey := fmt.Sprintf("ranking/%d-%s.mp4", time.Now().UnixNano(), title)
		if err := db.Exec(`INSERT INTO videos (user_id,title,description,object_key,status,visibility,moderation_status,file_size,content_type,published_at) VALUES (?, ?, '', ?, ?, ?, ?, 1, 'video/mp4', CURRENT_TIMESTAMP(3))`, userID, title, objectKey, status, visibility, moderation).Error; err != nil {
			t.Fatal(err)
		}
		var id uint64
		if err := db.Raw("SELECT id FROM videos WHERE object_key = ?", objectKey).Scan(&id).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Exec("INSERT INTO video_stats (video_id) VALUES (?)", id).Error; err != nil {
			t.Fatal(err)
		}
		return id
	}
	visible := insertVideo("visible", 2, 1, "visible")
	hidden := insertVideo("hidden", 2, 1, "hidden")
	private := insertVideo("private", 2, 2, "visible")
	for id, views := range map[uint64]uint64{visible: 3, hidden: 99, private: 80} {
		if err := db.Exec("INSERT INTO video_daily_metrics (video_id,stat_date,effective_views,watch_time_ms) VALUES (?, ?, ?, ?)", id, "2026-09-23", views, views*100).Error; err != nil {
			t.Fatal(err)
		}
	}
	repo := NewRepository(db, redisClient)
	now := time.Date(2026, 9, 23, 4, 0, 0, 0, time.UTC)
	oldKey := snapshotKey(WindowDay, now.AddDate(0, 0, -8))
	if err := redisClient.ZAdd(ctx, oldKey, cache.ZMember{Score: 1, Member: encodeMember(999)}); err != nil {
		t.Fatal(err)
	}
	if err := redisClient.Set(ctx, metadataKey(WindowDay, now.AddDate(0, 0, -8)), now.Format(time.RFC3339Nano), metadataTTL); err != nil {
		t.Fatal(err)
	}
	generated, err := repo.Rebuild(ctx, WindowDay, now)
	if err != nil || !generated.Equal(now) {
		t.Fatalf("rebuild generated=%v err=%v", generated, err)
	}
	if members, err := redisClient.ZRevRangeWithScores(ctx, oldKey, 0, -1); err != nil || len(members) != 0 {
		t.Fatalf("old snapshot was not pruned: members=%v err=%v", members, err)
	}
	items, total, _, err := repo.List(ctx, WindowDay, 1, 10, now)
	if err != nil || total != 1 || len(items) != 1 || items[0].VideoID != visible {
		t.Fatalf("redis list items=%+v total=%d err=%v", items, total, err)
	}
	if err := redisClient.Expire(ctx, dayKey, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if err := redisClient.Expire(ctx, metadataKey(WindowDay, now), time.Millisecond); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	items, total, _, err = repo.List(ctx, WindowDay, 1, 10, now)
	if err != nil || total != 1 || len(items) != 1 || items[0].VideoID != visible {
		t.Fatalf("expired snapshot fallback items=%+v total=%d err=%v", items, total, err)
	}
	if err := redisClient.Delete(ctx, dayKey, metadataKey(WindowDay, now)); err != nil {
		t.Fatal(err)
	}
	items, total, _, err = repo.List(ctx, WindowDay, 1, 10, now)
	if err != nil || total != 1 || len(items) != 1 || items[0].VideoID != visible {
		t.Fatalf("mysql fallback items=%+v total=%d err=%v", items, total, err)
	}
	if ok, err := redisClient.SetNX(ctx, keyPrefix+"lock", "other-owner", lockTTL); err != nil || !ok {
		t.Fatalf("seed lock: ok=%v err=%v", ok, err)
	}
	if _, err := repo.Rebuild(ctx, WindowDay, now); !errors.Is(err, ErrLockBusy) {
		t.Fatalf("rebuild with active lock err=%v", err)
	}
	_ = redisClient.Delete(ctx, keyPrefix+"lock")
	if _, err := repo.Rebuild(ctx, WindowDay, now); err != nil {
		t.Fatalf("restore snapshot before crash simulation: %v", err)
	}
	latest := insertVideo("latest-without-metrics", 2, 1, "visible")
	if err := db.Exec("DELETE FROM video_daily_metrics WHERE video_id IN ?", []uint64{visible, hidden, private}).Error; err != nil {
		t.Fatal(err)
	}
	if err := redisClient.Delete(ctx, dayKey, metadataKey(WindowDay, now)); err != nil {
		t.Fatal(err)
	}
	items, total, _, err = repo.List(ctx, WindowDay, 1, 10, now)
	if err != nil || total != 2 || len(items) != 2 || items[0].VideoID != latest || items[0].Score != 0 {
		t.Fatalf("latest public fallback items=%+v total=%d err=%v", items, total, err)
	}
	if _, err := repo.Rebuild(ctx, WindowDay, now); err != nil {
		t.Fatalf("rebuild empty snapshot: %v", err)
	}
	items, total, _, err = repo.List(ctx, WindowDay, 1, 10, now)
	if err != nil || total != 2 || len(items) != 2 || items[0].VideoID != latest || items[0].Score != 0 {
		t.Fatalf("empty snapshot latest fallback items=%+v total=%d err=%v", items, total, err)
	}
	crashProbe := cache.NewClient(cache.Options{Addr: redisConfig.Addr, Password: redisConfig.Password, DB: redisConfig.DB, CommandTimeout: 2 * time.Second})
	defer crashProbe.Close()
	_ = redisClient.Close()
	if _, err := repo.Rebuild(ctx, WindowDay, now); err == nil {
		t.Fatal("rebuild with closed redis unexpectedly succeeded")
	}
	if members, err := crashProbe.ZRevRangeWithScores(ctx, dayKey, 0, -1); err != nil || len(members) == 0 {
		t.Fatalf("old complete snapshot was lost after failed rebuild: members=%v err=%v", members, err)
	}
}
