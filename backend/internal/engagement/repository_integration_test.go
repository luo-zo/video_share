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
	switch column {
	case "view_count", "like_count", "favorite_count", "comment_count":
	default:
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

func activeCommentCount(t *testing.T, db *gorm.DB, videoID uint64) int64 {
	t.Helper()
	var count int64
	if err := db.Raw("SELECT COUNT(*) FROM comments WHERE video_id = ? AND deleted_at IS NULL", videoID).
		Scan(&count).Error; err != nil {
		t.Fatalf("count comments: %v", err)
	}
	return count
}

func commentDeletedAt(t *testing.T, db *gorm.DB, commentID uint64) *time.Time {
	t.Helper()
	var row struct{ DeletedAt *time.Time }
	if err := db.Raw("SELECT deleted_at FROM comments WHERE id = ?", commentID).Scan(&row).Error; err != nil {
		t.Fatalf("read comment deleted_at: %v", err)
	}
	return row.DeletedAt
}

func historyRowCount(t *testing.T, db *gorm.DB, userID, videoID uint64) int64 {
	t.Helper()
	var count int64
	if err := db.Raw("SELECT COUNT(*) FROM watch_histories WHERE user_id = ? AND video_id = ?", userID, videoID).
		Scan(&count).Error; err != nil {
		t.Fatalf("count watch histories: %v", err)
	}
	return count
}

func readHistoryProgress(t *testing.T, db *gorm.DB, userID, videoID uint64) uint64 {
	t.Helper()
	var row struct{ Progress uint64 }
	if err := db.Raw("SELECT progress_ms AS progress FROM watch_histories WHERE user_id = ? AND video_id = ?", userID, videoID).
		Scan(&row).Error; err != nil {
		t.Fatalf("read watch progress: %v", err)
	}
	return row.Progress
}

func TestCommentTransaction(t *testing.T) {
	ctx := context.Background()

	t.Run("create records the comment and counter together", func(t *testing.T) {
		db := openEngagementTestDB(t)
		alice, _ := seedEngagementUsers(t, db)
		vid := seedEngagementVideo(t, db, alice, video.StatusReady, video.VisibilityPublic)
		repo := NewRepository(db)

		created, err := repo.CreateComment(ctx, alice, vid, "  第一条评论  ")
		if err != nil {
			t.Fatalf("CreateComment: %v", err)
		}
		if created.ID == 0 || created.Content != "第一条评论" || created.Author.Username != "alice" {
			t.Fatalf("created = %+v", created)
		}
		if got := activeCommentCount(t, db, vid); got != 1 {
			t.Fatalf("comment rows = %d, want 1", got)
		}
		if got := statsCount(t, db, vid, "comment_count"); got != 1 {
			t.Fatalf("comment_count = %d, want 1", got)
		}
	})

	t.Run("listing is newest first with author projection", func(t *testing.T) {
		db := openEngagementTestDB(t)
		alice, bob := seedEngagementUsers(t, db)
		vid := seedEngagementVideo(t, db, alice, video.StatusReady, video.VisibilityPublic)
		repo := NewRepository(db)

		if _, err := repo.CreateComment(ctx, alice, vid, "旧的"); err != nil {
			t.Fatalf("alice comment: %v", err)
		}
		if _, err := repo.CreateComment(ctx, bob, vid, "新的"); err != nil {
			t.Fatalf("bob comment: %v", err)
		}

		items, total, err := repo.ListComments(ctx, vid, 1, 12)
		if err != nil {
			t.Fatalf("ListComments: %v", err)
		}
		if total != 2 || len(items) != 2 {
			t.Fatalf("ListComments = %d items, total %d", len(items), total)
		}
		if items[0].Content != "新的" || items[0].Author.Username != "bob" {
			t.Fatalf("first item = %+v", items[0])
		}
		if items[1].Content != "旧的" || items[1].Author.Nickname == "" {
			t.Fatalf("second item = %+v", items[1])
		}
	})

	t.Run("owner soft delete hides the comment and decrements", func(t *testing.T) {
		db := openEngagementTestDB(t)
		alice, _ := seedEngagementUsers(t, db)
		vid := seedEngagementVideo(t, db, alice, video.StatusReady, video.VisibilityPublic)
		repo := NewRepository(db)

		created, err := repo.CreateComment(ctx, alice, vid, "待删除")
		if err != nil {
			t.Fatalf("CreateComment: %v", err)
		}
		if err := repo.DeleteComment(ctx, alice, created.ID); err != nil {
			t.Fatalf("DeleteComment: %v", err)
		}
		if commentDeletedAt(t, db, created.ID) == nil {
			t.Fatal("comment was not soft deleted")
		}
		if got := activeCommentCount(t, db, vid); got != 0 {
			t.Fatalf("active comment rows = %d, want 0", got)
		}
		if got := statsCount(t, db, vid, "comment_count"); got != 0 {
			t.Fatalf("comment_count = %d, want 0", got)
		}
		items, total, err := repo.ListComments(ctx, vid, 1, 12)
		if err != nil || total != 0 || len(items) != 0 {
			t.Fatalf("ListComments after delete = (%d items, %d total, %v)", len(items), total, err)
		}
	})

	t.Run("repeated delete reports not found", func(t *testing.T) {
		db := openEngagementTestDB(t)
		alice, _ := seedEngagementUsers(t, db)
		vid := seedEngagementVideo(t, db, alice, video.StatusReady, video.VisibilityPublic)
		repo := NewRepository(db)

		created, err := repo.CreateComment(ctx, alice, vid, "删除两次")
		if err != nil {
			t.Fatalf("CreateComment: %v", err)
		}
		if err := repo.DeleteComment(ctx, alice, created.ID); err != nil {
			t.Fatalf("first delete: %v", err)
		}
		if err := repo.DeleteComment(ctx, alice, created.ID); !errors.Is(err, ErrCommentNotFound) {
			t.Fatalf("second delete error = %v, want ErrCommentNotFound", err)
		}
		if got := statsCount(t, db, vid, "comment_count"); got != 0 {
			t.Fatalf("comment_count = %d, want 0", got)
		}
	})

	t.Run("foreign delete is forbidden and leaves the comment", func(t *testing.T) {
		db := openEngagementTestDB(t)
		alice, bob := seedEngagementUsers(t, db)
		vid := seedEngagementVideo(t, db, alice, video.StatusReady, video.VisibilityPublic)
		repo := NewRepository(db)

		created, err := repo.CreateComment(ctx, alice, vid, "爱丽丝的评论")
		if err != nil {
			t.Fatalf("CreateComment: %v", err)
		}
		if err := repo.DeleteComment(ctx, bob, created.ID); !errors.Is(err, ErrCommentForbidden) {
			t.Fatalf("foreign delete error = %v, want ErrCommentForbidden", err)
		}
		if commentDeletedAt(t, db, created.ID) != nil {
			t.Fatal("foreign delete removed the comment")
		}
		if got := statsCount(t, db, vid, "comment_count"); got != 1 {
			t.Fatalf("comment_count = %d, want 1", got)
		}
	})

	t.Run("comments on unavailable videos are rejected", func(t *testing.T) {
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
			if _, err := repo.CreateComment(ctx, alice, vid, "看不见的视频"); !errors.Is(err, ErrVideoNotFound) {
				t.Fatalf("%s CreateComment error = %v, want ErrVideoNotFound", tc.name, err)
			}
			if got := activeCommentCount(t, db, vid); got != 0 {
				t.Fatalf("%s comment rows = %d, want 0", tc.name, got)
			}
			if _, _, err := repo.ListComments(ctx, vid, 1, 12); !errors.Is(err, ErrVideoNotFound) {
				t.Fatalf("%s ListComments error = %v, want ErrVideoNotFound", tc.name, err)
			}
		}
	})

	t.Run("comment counter never falls below zero", func(t *testing.T) {
		db := openEngagementTestDB(t)
		alice, _ := seedEngagementUsers(t, db)
		vid := seedEngagementVideo(t, db, alice, video.StatusReady, video.VisibilityPublic)
		repo := NewRepository(db)

		created, err := repo.CreateComment(ctx, alice, vid, "计数漂移")
		if err != nil {
			t.Fatalf("CreateComment: %v", err)
		}
		if err := db.Exec("UPDATE video_stats SET comment_count = 0 WHERE video_id = ?", vid).Error; err != nil {
			t.Fatalf("drift comment_count: %v", err)
		}
		if err := repo.DeleteComment(ctx, alice, created.ID); err != nil {
			t.Fatalf("DeleteComment: %v", err)
		}
		if got := statsCount(t, db, vid, "comment_count"); got != 0 {
			t.Fatalf("comment_count = %d, want 0", got)
		}
	})
}

func TestWatchUpsert(t *testing.T) {
	ctx := context.Background()

	t.Run("first watch records history and increments views", func(t *testing.T) {
		db := openEngagementTestDB(t)
		alice, _ := seedEngagementUsers(t, db)
		vid := seedEngagementVideo(t, db, alice, video.StatusReady, video.VisibilityPublic)
		repo := NewRepository(db)

		if err := repo.RecordWatch(ctx, alice, vid, 5, 10); err != nil {
			t.Fatalf("RecordWatch: %v", err)
		}
		if got := historyRowCount(t, db, alice, vid); got != 1 {
			t.Fatalf("history rows = %d, want 1", got)
		}
		if got := readHistoryProgress(t, db, alice, vid); got != 5 {
			t.Fatalf("progress = %d, want 5", got)
		}
		if got := statsCount(t, db, vid, "view_count"); got != 1 {
			t.Fatalf("view_count = %d, want 1", got)
		}
	})

	t.Run("repeated watch updates progress without another view", func(t *testing.T) {
		db := openEngagementTestDB(t)
		alice, _ := seedEngagementUsers(t, db)
		vid := seedEngagementVideo(t, db, alice, video.StatusReady, video.VisibilityPublic)
		repo := NewRepository(db)

		if err := repo.RecordWatch(ctx, alice, vid, 3, 10); err != nil {
			t.Fatalf("first watch: %v", err)
		}
		if err := repo.RecordWatch(ctx, alice, vid, 8, 10); err != nil {
			t.Fatalf("second watch: %v", err)
		}
		if got := historyRowCount(t, db, alice, vid); got != 1 {
			t.Fatalf("history rows = %d, want 1", got)
		}
		if got := readHistoryProgress(t, db, alice, vid); got != 8 {
			t.Fatalf("progress = %d, want 8", got)
		}
		if got := statsCount(t, db, vid, "view_count"); got != 1 {
			t.Fatalf("view_count = %d, want 1", got)
		}
	})

	t.Run("separate viewers each add a view", func(t *testing.T) {
		db := openEngagementTestDB(t)
		alice, bob := seedEngagementUsers(t, db)
		vid := seedEngagementVideo(t, db, alice, video.StatusReady, video.VisibilityPublic)
		repo := NewRepository(db)

		if err := repo.RecordWatch(ctx, alice, vid, 1, 10); err != nil {
			t.Fatalf("alice watch: %v", err)
		}
		if err := repo.RecordWatch(ctx, bob, vid, 2, 10); err != nil {
			t.Fatalf("bob watch: %v", err)
		}
		if got := statsCount(t, db, vid, "view_count"); got != 2 {
			t.Fatalf("view_count = %d, want 2", got)
		}
	})

	t.Run("progress beyond a known duration is rejected", func(t *testing.T) {
		db := openEngagementTestDB(t)
		alice, _ := seedEngagementUsers(t, db)
		vid := seedEngagementVideo(t, db, alice, video.StatusReady, video.VisibilityPublic)
		repo := NewRepository(db)

		if err := repo.RecordWatch(ctx, alice, vid, 11, 10); !errors.Is(err, ErrProgressInvalid) {
			t.Fatalf("RecordWatch error = %v, want ErrProgressInvalid", err)
		}
		if got := historyRowCount(t, db, alice, vid); got != 0 {
			t.Fatalf("history rows = %d, want 0", got)
		}
		if got := statsCount(t, db, vid, "view_count"); got != 0 {
			t.Fatalf("view_count = %d, want 0", got)
		}
	})

	t.Run("progress is accepted while the duration is unknown", func(t *testing.T) {
		db := openEngagementTestDB(t)
		alice, _ := seedEngagementUsers(t, db)
		vid := seedEngagementVideo(t, db, alice, video.StatusReady, video.VisibilityPublic)
		repo := NewRepository(db)

		if err := repo.RecordWatch(ctx, alice, vid, 50, 0); err != nil {
			t.Fatalf("RecordWatch: %v", err)
		}
		if got := statsCount(t, db, vid, "view_count"); got != 1 {
			t.Fatalf("view_count = %d, want 1", got)
		}
	})

	t.Run("watch on unavailable videos is rejected", func(t *testing.T) {
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
			if err := repo.RecordWatch(ctx, alice, vid, 1, 10); !errors.Is(err, ErrVideoNotFound) {
				t.Fatalf("%s RecordWatch error = %v, want ErrVideoNotFound", tc.name, err)
			}
			if got := historyRowCount(t, db, alice, vid); got != 0 {
				t.Fatalf("%s history rows = %d, want 0", tc.name, got)
			}
		}
	})

	t.Run("history lists only public ready videos newest first", func(t *testing.T) {
		db := openEngagementTestDB(t)
		alice, _ := seedEngagementUsers(t, db)
		first := seedEngagementVideo(t, db, alice, video.StatusReady, video.VisibilityPublic)
		second := seedEngagementVideo(t, db, alice, video.StatusReady, video.VisibilityPublic)
		repo := NewRepository(db)

		if err := repo.RecordWatch(ctx, alice, first, 1, 10); err != nil {
			t.Fatalf("watch first: %v", err)
		}
		if err := repo.RecordWatch(ctx, alice, second, 2, 10); err != nil {
			t.Fatalf("watch second: %v", err)
		}
		// 让第二次观看的时间戳严格晚于第一次，避免同毫秒排序歧义。
		if err := db.Exec("UPDATE watch_histories SET last_watched_at = UTC_TIMESTAMP(3) + INTERVAL 1 SECOND WHERE video_id = ?", second).Error; err != nil {
			t.Fatalf("advance second timestamp: %v", err)
		}

		videos, total, err := repo.ListHistory(ctx, alice, 1, 12)
		if err != nil {
			t.Fatalf("ListHistory: %v", err)
		}
		if total != 2 || len(videos) != 2 {
			t.Fatalf("ListHistory = %d videos, total %d", len(videos), total)
		}
		if videos[0].ID != second {
			t.Fatalf("history[0] = %d, want %d", videos[0].ID, second)
		}
		if videos[0].Author.Username != "alice" || videos[0].Stats.ViewCount != 1 {
			t.Fatalf("history[0] projection = %+v", videos[0])
		}

		if err := db.Exec("UPDATE videos SET visibility = ? WHERE id = ?", video.VisibilityPrivate, second).Error; err != nil {
			t.Fatalf("hide second video: %v", err)
		}
		videos, total, err = repo.ListHistory(ctx, alice, 1, 12)
		if err != nil {
			t.Fatalf("ListHistory after hide: %v", err)
		}
		if total != 1 || len(videos) != 1 || videos[0].ID != first {
			t.Fatalf("hidden history = %+v, total %d", videos, total)
		}
	})

	t.Run("favorites list only public ready videos newest first", func(t *testing.T) {
		db := openEngagementTestDB(t)
		alice, _ := seedEngagementUsers(t, db)
		first := seedEngagementVideo(t, db, alice, video.StatusReady, video.VisibilityPublic)
		second := seedEngagementVideo(t, db, alice, video.StatusReady, video.VisibilityPublic)
		repo := NewRepository(db)

		if _, err := repo.SetFavorite(ctx, alice, first, true); err != nil {
			t.Fatalf("favorite first: %v", err)
		}
		if _, err := repo.SetFavorite(ctx, alice, second, true); err != nil {
			t.Fatalf("favorite second: %v", err)
		}
		if err := db.Exec("UPDATE video_favorites SET created_at = UTC_TIMESTAMP(3) + INTERVAL 1 SECOND WHERE user_id = ? AND video_id = ?", alice, second).Error; err != nil {
			t.Fatalf("advance second favorite timestamp: %v", err)
		}

		videos, total, err := repo.ListFavorites(ctx, alice, 1, 12)
		if err != nil {
			t.Fatalf("ListFavorites: %v", err)
		}
		if total != 2 || len(videos) != 2 || videos[0].ID != second {
			t.Fatalf("ListFavorites = %+v, total %d", videos, total)
		}
		if videos[0].Author.Username != "alice" || videos[0].Stats.FavoriteCount != 1 {
			t.Fatalf("favorite projection = %+v", videos[0])
		}

		if err := db.Exec("UPDATE videos SET status = ? WHERE id = ?", video.StatusDeleted, second).Error; err != nil {
			t.Fatalf("delete second video: %v", err)
		}
		videos, total, err = repo.ListFavorites(ctx, alice, 1, 12)
		if err != nil {
			t.Fatalf("ListFavorites after delete: %v", err)
		}
		if total != 1 || len(videos) != 1 || videos[0].ID != first {
			t.Fatalf("deleted favorite = %+v, total %d", videos, total)
		}
	})
}
