//go:build integration

package engagement

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"video_share/internal/database"
	"video_share/internal/testutil"
	"video_share/internal/video"
)

// openEngagementTestDB 创建一个本次运行独有的 MySQL 数据库并完成迁移，返回 GORM
// 句柄。清理时只删除这个数据库。
func openEngagementTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := testutil.MySQLDSN(t)
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql.DB: %v", err)
	}
	migrator, err := database.NewMigrator(sqlDB)
	if err != nil {
		t.Fatalf("new migrator: %v", err)
	}
	if _, err := migrator.Up(context.Background()); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Errorf("close test db: %v", err)
		}
	})
	return db
}

func seedEngagementUsers(t *testing.T, db *gorm.DB) (alice, bob uint64) {
	t.Helper()
	for _, name := range []string{"alice", "bob"} {
		if err := db.Exec(`INSERT INTO users (username, password_hash, nickname) VALUES (?, ?, ?)`,
			name, "hash", name).Error; err != nil {
			t.Fatalf("insert %s: %v", name, err)
		}
	}
	alice = lookupEngagementID(t, db, "SELECT id FROM users WHERE username = ?", "alice")
	bob = lookupEngagementID(t, db, "SELECT id FROM users WHERE username = ?", "bob")
	return alice, bob
}

func lookupEngagementID(t *testing.T, db *gorm.DB, query string, args ...any) uint64 {
	t.Helper()
	var row struct{ ID uint64 }
	if err := db.Raw(query, args...).Scan(&row).Error; err != nil {
		t.Fatalf("lookup id: %v", err)
	}
	if row.ID == 0 {
		t.Fatalf("lookup returned no id for %q %v", query, args)
	}
	return row.ID
}

// seedEngagementVideo 插入一条视频及其归零的统计行，返回视频 ID。
func seedEngagementVideo(t *testing.T, db *gorm.DB, userID uint64, status video.Status, visibility video.Visibility) uint64 {
	t.Helper()
	objectKey := fmt.Sprintf("videos/%d-%d/source.mp4", userID, time.Now().UnixNano())
	if err := db.Exec(`INSERT INTO videos (user_id, title, description, object_key, status, visibility, file_size, content_type)
		VALUES (?, ?, '', ?, ?, ?, 1024, 'video/mp4')`,
		userID, "互动测试", objectKey, status, visibility).Error; err != nil {
		t.Fatalf("insert video: %v", err)
	}
	id := lookupEngagementID(t, db, "SELECT id FROM videos WHERE object_key = ?", objectKey)
	if err := db.Exec(`INSERT INTO video_stats (video_id) VALUES (?)`, id).Error; err != nil {
		t.Fatalf("insert stats: %v", err)
	}
	return id
}

func relationRowCount(t *testing.T, db *gorm.DB, table string, userID, videoID uint64) int64 {
	t.Helper()
	if table != "video_likes" && table != "video_favorites" {
		t.Fatalf("unsupported relation table %q", table)
	}
	var count int64
	if err := db.Raw("SELECT COUNT(*) FROM "+table+" WHERE user_id = ? AND video_id = ?", userID, videoID).
		Scan(&count).Error; err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return count
}

func statsCount(t *testing.T, db *gorm.DB, videoID uint64, column string) uint64 {
	t.Helper()
	if column != "like_count" && column != "favorite_count" {
		t.Fatalf("unsupported stats column %q", column)
	}
	var row struct{ Value uint64 }
	if err := db.Raw("SELECT "+column+" AS value FROM video_stats WHERE video_id = ?", videoID).
		Scan(&row).Error; err != nil {
		t.Fatalf("read %s: %v", column, err)
	}
	return row.Value
}

func TestRelationTransactions(t *testing.T) {
	ctx := context.Background()

	t.Run("first like records the relation and counter together", func(t *testing.T) {
		db := openEngagementTestDB(t)
		alice, _ := seedEngagementUsers(t, db)
		vid := seedEngagementVideo(t, db, alice, video.StatusReady, video.VisibilityPublic)
		repo := NewRepository(db)

		state, err := repo.SetLike(ctx, alice, vid, true)
		if err != nil {
			t.Fatalf("SetLike: %v", err)
		}
		if !state.Active || state.Count != 1 {
			t.Fatalf("state = %+v, want active count 1", state)
		}
		if got := relationRowCount(t, db, "video_likes", alice, vid); got != 1 {
			t.Fatalf("like rows = %d, want 1", got)
		}
		if got := statsCount(t, db, vid, "like_count"); got != 1 {
			t.Fatalf("like_count = %d, want 1", got)
		}
	})

	t.Run("repeated like keeps exactly one relation", func(t *testing.T) {
		db := openEngagementTestDB(t)
		alice, _ := seedEngagementUsers(t, db)
		vid := seedEngagementVideo(t, db, alice, video.StatusReady, video.VisibilityPublic)
		repo := NewRepository(db)

		for i := 0; i < 3; i++ {
			state, err := repo.SetLike(ctx, alice, vid, true)
			if err != nil {
				t.Fatalf("SetLike #%d: %v", i, err)
			}
			if !state.Active || state.Count != 1 {
				t.Fatalf("SetLike #%d state = %+v, want active count 1", i, state)
			}
		}
		if got := relationRowCount(t, db, "video_likes", alice, vid); got != 1 {
			t.Fatalf("like rows = %d, want 1", got)
		}
		if got := statsCount(t, db, vid, "like_count"); got != 1 {
			t.Fatalf("like_count = %d, want 1", got)
		}
	})

	t.Run("viewers accumulate and detach independently", func(t *testing.T) {
		db := openEngagementTestDB(t)
		alice, bob := seedEngagementUsers(t, db)
		vid := seedEngagementVideo(t, db, alice, video.StatusReady, video.VisibilityPublic)
		repo := NewRepository(db)

		if _, err := repo.SetLike(ctx, alice, vid, true); err != nil {
			t.Fatalf("alice like: %v", err)
		}
		state, err := repo.SetLike(ctx, bob, vid, true)
		if err != nil {
			t.Fatalf("bob like: %v", err)
		}
		if state.Count != 2 {
			t.Fatalf("count after two viewers = %d, want 2", state.Count)
		}

		state, err = repo.SetLike(ctx, alice, vid, false)
		if err != nil {
			t.Fatalf("alice unlike: %v", err)
		}
		if state.Active || state.Count != 1 {
			t.Fatalf("alice unlike state = %+v, want inactive count 1", state)
		}
		if got := relationRowCount(t, db, "video_likes", bob, vid); got != 1 {
			t.Fatalf("bob like rows = %d, want 1", got)
		}
		if got := relationRowCount(t, db, "video_likes", alice, vid); got != 0 {
			t.Fatalf("alice like rows = %d, want 0", got)
		}
	})

	t.Run("repeated unlike stays at zero", func(t *testing.T) {
		db := openEngagementTestDB(t)
		alice, _ := seedEngagementUsers(t, db)
		vid := seedEngagementVideo(t, db, alice, video.StatusReady, video.VisibilityPublic)
		repo := NewRepository(db)

		if _, err := repo.SetLike(ctx, alice, vid, true); err != nil {
			t.Fatalf("like: %v", err)
		}
		for i := 0; i < 2; i++ {
			state, err := repo.SetLike(ctx, alice, vid, false)
			if err != nil {
				t.Fatalf("unlike #%d: %v", i, err)
			}
			if state.Active || state.Count != 0 {
				t.Fatalf("unlike #%d state = %+v, want inactive count 0", i, state)
			}
		}
		if got := relationRowCount(t, db, "video_likes", alice, vid); got != 0 {
			t.Fatalf("like rows = %d, want 0", got)
		}
	})

	t.Run("favorites mirror likes without sharing counters", func(t *testing.T) {
		db := openEngagementTestDB(t)
		alice, _ := seedEngagementUsers(t, db)
		vid := seedEngagementVideo(t, db, alice, video.StatusReady, video.VisibilityPublic)
		repo := NewRepository(db)

		state, err := repo.SetFavorite(ctx, alice, vid, true)
		if err != nil {
			t.Fatalf("SetFavorite: %v", err)
		}
		if !state.Active || state.Count != 1 {
			t.Fatalf("favorite state = %+v, want active count 1", state)
		}
		if got := statsCount(t, db, vid, "favorite_count"); got != 1 {
			t.Fatalf("favorite_count = %d, want 1", got)
		}
		if got := statsCount(t, db, vid, "like_count"); got != 0 {
			t.Fatalf("like_count = %d, want 0", got)
		}
		if got := relationRowCount(t, db, "video_likes", alice, vid); got != 0 {
			t.Fatalf("like rows = %d, want 0", got)
		}

		state, err = repo.SetFavorite(ctx, alice, vid, false)
		if err != nil {
			t.Fatalf("Unfavorite: %v", err)
		}
		if state.Active || state.Count != 0 {
			t.Fatalf("unfavorite state = %+v, want inactive count 0", state)
		}
		if got := statsCount(t, db, vid, "favorite_count"); got != 0 {
			t.Fatalf("favorite_count = %d, want 0", got)
		}
	})

	t.Run("unavailable videos reject both relations", func(t *testing.T) {
		db := openEngagementTestDB(t)
		alice, _ := seedEngagementUsers(t, db)
		repo := NewRepository(db)

		cases := []struct {
			name       string
			status     video.Status
			visibility video.Visibility
		}{
			{name: "private", status: video.StatusReady, visibility: video.VisibilityPrivate},
			{name: "uploading", status: video.StatusUploading, visibility: video.VisibilityPublic},
			{name: "deleted", status: video.StatusDeleted, visibility: video.VisibilityPublic},
		}
		for _, tc := range cases {
			vid := seedEngagementVideo(t, db, alice, tc.status, tc.visibility)
			if _, err := repo.SetLike(ctx, alice, vid, true); !errors.Is(err, ErrVideoNotFound) {
				t.Fatalf("%s SetLike error = %v, want ErrVideoNotFound", tc.name, err)
			}
			if _, err := repo.SetFavorite(ctx, alice, vid, true); !errors.Is(err, ErrVideoNotFound) {
				t.Fatalf("%s SetFavorite error = %v, want ErrVideoNotFound", tc.name, err)
			}
			if _, err := repo.SetLike(ctx, alice, vid, false); !errors.Is(err, ErrVideoNotFound) {
				t.Fatalf("%s unlike error = %v, want ErrVideoNotFound", tc.name, err)
			}
			if got := relationRowCount(t, db, "video_likes", alice, vid); got != 0 {
				t.Fatalf("%s like rows = %d, want 0", tc.name, got)
			}
			if got := statsCount(t, db, vid, "like_count"); got != 0 {
				t.Fatalf("%s like_count = %d, want 0", tc.name, got)
			}
		}

		if _, err := repo.SetLike(ctx, alice, 99999999, true); !errors.Is(err, ErrVideoNotFound) {
			t.Fatalf("missing video SetLike error = %v, want ErrVideoNotFound", err)
		}
	})

	t.Run("counters never fall below zero", func(t *testing.T) {
		db := openEngagementTestDB(t)
		alice, _ := seedEngagementUsers(t, db)
		vid := seedEngagementVideo(t, db, alice, video.StatusReady, video.VisibilityPublic)
		repo := NewRepository(db)

		// 制造“关系存在但计数器已被清零”的漂移状态：此时删除关系会真正影响一行，
		// 递减必须被夹在 0，而不能触发 BIGINT UNSIGNED 下溢。
		if err := db.Exec(`INSERT INTO video_likes (user_id, video_id) VALUES (?, ?)`, alice, vid).Error; err != nil {
			t.Fatalf("seed drifted like: %v", err)
		}
		state, err := repo.SetLike(ctx, alice, vid, false)
		if err != nil {
			t.Fatalf("unlike drifted row: %v", err)
		}
		if state.Count != 0 {
			t.Fatalf("drifted unlike count = %d, want 0", state.Count)
		}
		if got := statsCount(t, db, vid, "like_count"); got != 0 {
			t.Fatalf("like_count = %d, want 0", got)
		}
	})

	t.Run("concurrent likes of one viewer count once", func(t *testing.T) {
		db := openEngagementTestDB(t)
		alice, _ := seedEngagementUsers(t, db)
		vid := seedEngagementVideo(t, db, alice, video.StatusReady, video.VisibilityPublic)
		repo := NewRepository(db)

		const workers = 8
		var wg sync.WaitGroup
		errs := make(chan error, workers)
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := repo.SetLike(ctx, alice, vid, true); err != nil {
					errs <- err
				}
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Fatalf("concurrent SetLike: %v", err)
		}

		if got := relationRowCount(t, db, "video_likes", alice, vid); got != 1 {
			t.Fatalf("like rows = %d, want 1", got)
		}
		if got := statsCount(t, db, vid, "like_count"); got != 1 {
			t.Fatalf("like_count = %d, want 1", got)
		}
	})
}
