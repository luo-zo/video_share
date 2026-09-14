//go:build integration

package video

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"video_share/internal/database"
	"video_share/internal/testutil"
)

// openTestDB creates a fresh uniquely named MySQL database, migrates it, and
// returns a GORM handle. Cleanup drops only this database.
func openTestDB(t *testing.T) *gorm.DB {
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

func seedUsers(t *testing.T, db *gorm.DB) (alice, bob uint64) {
	t.Helper()
	if err := db.Exec(`INSERT INTO users (username, password_hash, nickname) VALUES (?, ?, ?)`,
		"alice", "hash", "爱丽丝").Error; err != nil {
		t.Fatalf("insert alice: %v", err)
	}
	if err := db.Exec(`INSERT INTO users (username, password_hash, nickname) VALUES (?, ?, ?)`,
		"bob", "hash", "鲍勃").Error; err != nil {
		t.Fatalf("insert bob: %v", err)
	}
	alice = lookupID(t, db, "SELECT id FROM users WHERE username = ?", "alice")
	bob = lookupID(t, db, "SELECT id FROM users WHERE username = ?", "bob")
	return alice, bob
}

func lookupID(t *testing.T, db *gorm.DB, query string, args ...any) uint64 {
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

func seedVideo(t *testing.T, db *gorm.DB, userID uint64, title, description string, status Status, visibility Visibility, publishedAt string, counts [4]uint64) uint64 {
	t.Helper()
	objectKey := "videos/" + title + ".mp4"
	if err := db.Exec(`INSERT INTO videos (user_id, title, description, object_key, status, visibility, file_size, content_type, published_at)
		VALUES (?, ?, ?, ?, ?, ?, 1024, 'video/mp4', ?)`,
		userID, title, description, objectKey, status, visibility, publishedAt).Error; err != nil {
		t.Fatalf("insert video %q: %v", title, err)
	}
	id := lookupID(t, db, "SELECT id FROM videos WHERE object_key = ?", objectKey)
	if err := db.Exec(`INSERT INTO video_stats (video_id, view_count, like_count, favorite_count, comment_count)
		VALUES (?, ?, ?, ?, ?)`, id, counts[0], counts[1], counts[2], counts[3]).Error; err != nil {
		t.Fatalf("insert stats for %q: %v", title, err)
	}
	return id
}

func titles(videos []Video) []string {
	result := make([]string, 0, len(videos))
	for i := range videos {
		result = append(result, videos[i].Title)
	}
	return result
}

func TestRepositoryListPublicSearchVisibilityAndSort(t *testing.T) {
	db := openTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()
	alice, bob := seedUsers(t, db)

	cat := seedVideo(t, db, alice, "猫咪散步", "傍晚记录", StatusReady, VisibilityPublic,
		"2026-09-10 10:00:00", [4]uint64{30, 5, 1, 2})
	percent := seedVideo(t, db, alice, "进度 100% 完成", "普通描述", StatusReady, VisibilityPublic,
		"2026-09-12 10:00:00", [4]uint64{100, 1, 0, 0})
	underscore := seedVideo(t, db, bob, "under_score", "demo", StatusReady, VisibilityPublic,
		"2026-09-11 10:00:00", [4]uint64{50, 20, 0, 0})
	privateCat := seedVideo(t, db, alice, "隐藏猫咪", "私有", StatusReady, VisibilityPrivate,
		"2026-09-09 10:00:00", [4]uint64{0, 0, 0, 0})
	uploadingCat := seedVideo(t, db, alice, "猫咪上传中", "未完成", StatusUploading, VisibilityPublic,
		"2026-09-08 10:00:00", [4]uint64{0, 0, 0, 0})
	deletedCat := seedVideo(t, db, alice, "猫咪已删除", "删除", StatusDeleted, VisibilityPublic,
		"2026-09-07 10:00:00", [4]uint64{0, 0, 0, 0})
	failedCat := seedVideo(t, db, alice, "猫咪失败", "转码失败", StatusFailed, VisibilityPublic,
		"2026-09-06 10:00:00", [4]uint64{0, 0, 0, 0})

	t.Run("chinese title excludes non public or non ready", func(t *testing.T) {
		items, total, err := repo.ListPublic(ctx, ListQuery{Page: 1, PageSize: 12, Query: "猫咪", Sort: SortLatest})
		if err != nil {
			t.Fatalf("ListPublic: %v", err)
		}
		if total != 1 || len(items) != 1 || items[0].ID != cat {
			t.Fatalf("items=%v total=%d, want only %d", titles(items), total, cat)
		}
		for _, excluded := range []uint64{privateCat, uploadingCat, deletedCat, failedCat} {
			for _, item := range items {
				if item.ID == excluded {
					t.Fatalf("excluded video %d leaked into discovery", excluded)
				}
			}
		}
	})

	t.Run("author username and nickname match every owned video", func(t *testing.T) {
		for _, keyword := range []string{"alice", "爱丽丝"} {
			items, total, err := repo.ListPublic(ctx, ListQuery{Page: 1, PageSize: 12, Query: keyword, Sort: SortLatest})
			if err != nil {
				t.Fatalf("ListPublic(%q): %v", keyword, err)
			}
			if total != 2 || len(items) != 2 || items[0].ID != percent || items[1].ID != cat {
				t.Fatalf("keyword %q: items=%v total=%d, want [%d %d]", keyword, titles(items), total, percent, cat)
			}
		}
	})

	t.Run("wildcards are matched literally", func(t *testing.T) {
		items, total, err := repo.ListPublic(ctx, ListQuery{Page: 1, PageSize: 50, Query: "%", Sort: SortLatest})
		if err != nil {
			t.Fatalf("ListPublic(%%): %v", err)
		}
		if total != 1 || len(items) != 1 || items[0].ID != percent {
			t.Fatalf("literal %% search: items=%v total=%d, want only %d", titles(items), total, percent)
		}

		items, total, err = repo.ListPublic(ctx, ListQuery{Page: 1, PageSize: 50, Query: "_", Sort: SortLatest})
		if err != nil {
			t.Fatalf("ListPublic(_): %v", err)
		}
		if total != 1 || len(items) != 1 || items[0].ID != underscore {
			t.Fatalf("literal _ search: items=%v total=%d, want only %d", titles(items), total, underscore)
		}
	})

	t.Run("latest orders by publication time then id", func(t *testing.T) {
		items, total, err := repo.ListPublic(ctx, ListQuery{Page: 1, PageSize: 12, Sort: SortLatest})
		if err != nil {
			t.Fatalf("ListPublic: %v", err)
		}
		if total != 3 {
			t.Fatalf("total=%d, want 3", total)
		}
		want := []uint64{percent, underscore, cat}
		for i, id := range want {
			if items[i].ID != id {
				t.Fatalf("latest order = %v, want %v", titles(items), want)
			}
		}
	})

	t.Run("popular orders by views then likes", func(t *testing.T) {
		items, total, err := repo.ListPublic(ctx, ListQuery{Page: 1, PageSize: 12, Sort: SortPopular})
		if err != nil {
			t.Fatalf("ListPublic: %v", err)
		}
		if total != 3 {
			t.Fatalf("total=%d, want 3", total)
		}
		want := []uint64{percent, underscore, cat}
		for i, id := range want {
			if items[i].ID != id {
				t.Fatalf("popular order = %v, want %v", titles(items), want)
			}
		}
		if items[0].Stats.ViewCount != 100 || items[1].Stats.ViewCount != 50 || items[2].Stats.ViewCount != 30 {
			t.Fatalf("stats not projected: %+v", []Stats{items[0].Stats, items[1].Stats, items[2].Stats})
		}
	})

	t.Run("pagination bounds the result set", func(t *testing.T) {
		items, total, err := repo.ListPublic(ctx, ListQuery{Page: 2, PageSize: 2, Sort: SortLatest})
		if err != nil {
			t.Fatalf("ListPublic: %v", err)
		}
		if total != 3 || len(items) != 1 || items[0].ID != cat {
			t.Fatalf("page 2 items=%v total=%d", titles(items), total)
		}
	})

	t.Run("empty keyword is an ordinary public listing", func(t *testing.T) {
		_, total, err := repo.ListPublic(ctx, ListQuery{Page: 1, PageSize: 12, Query: "", Sort: SortLatest})
		if err != nil {
			t.Fatalf("ListPublic: %v", err)
		}
		if total != 3 {
			t.Fatalf("total=%d, want 3", total)
		}
	})

	t.Run("service trims a whitespace keyword", func(t *testing.T) {
		svc := NewService(repo, nil, 1024, 0, 0)
		result, err := svc.ListPublic(ctx, ListQuery{Page: 1, PageSize: 12, Query: " 猫咪 ", Sort: SortLatest})
		if err != nil {
			t.Fatalf("ListPublic: %v", err)
		}
		if result.Total != 1 || len(result.Items) != 1 || result.Items[0].ID != cat {
			t.Fatalf("trimmed search: total=%d items=%v", result.Total, result.Items)
		}
	})
}

func TestRepositoryFindPublicByIDHidesUnavailableVideos(t *testing.T) {
	db := openTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()
	alice, _ := seedUsers(t, db)

	visible := seedVideo(t, db, alice, "公开视频", "ok", StatusReady, VisibilityPublic, "2026-09-10 10:00:00", [4]uint64{1, 2, 3, 4})
	private := seedVideo(t, db, alice, "私有视频", "no", StatusReady, VisibilityPrivate, "2026-09-10 10:00:00", [4]uint64{0, 0, 0, 0})
	uploading := seedVideo(t, db, alice, "上传中", "no", StatusUploading, VisibilityPublic, "2026-09-10 10:00:00", [4]uint64{0, 0, 0, 0})
	deleted := seedVideo(t, db, alice, "已删除", "no", StatusDeleted, VisibilityPublic, "2026-09-10 10:00:00", [4]uint64{0, 0, 0, 0})

	got, err := repo.FindPublicByID(ctx, visible)
	if err != nil {
		t.Fatalf("FindPublicByID: %v", err)
	}
	if got.Title != "公开视频" || got.Author.Username != "alice" || got.Author.Nickname != "爱丽丝" {
		t.Fatalf("public video = %+v", got)
	}
	if got.Stats.ViewCount != 1 || got.Stats.CommentCount != 4 {
		t.Fatalf("stats = %+v", got.Stats)
	}
	if got.Visibility != VisibilityPublic || got.PublishedAt == nil {
		t.Fatalf("visibility=%v published_at=%v", got.Visibility, got.PublishedAt)
	}

	for _, id := range []uint64{private, uploading, deleted, 999999} {
		if _, err := repo.FindPublicByID(ctx, id); !errors.Is(err, ErrNotFound) {
			t.Fatalf("FindPublicByID(%d) error = %v, want ErrNotFound", id, err)
		}
	}
}

func TestRepositoryViewerState(t *testing.T) {
	db := openTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()
	alice, bob := seedUsers(t, db)
	videoID := seedVideo(t, db, alice, "互动视频", "ok", StatusReady, VisibilityPublic, "2026-09-10 10:00:00", [4]uint64{0, 0, 0, 0})

	state, err := repo.ViewerState(ctx, bob, videoID, alice)
	if err != nil {
		t.Fatalf("ViewerState: %v", err)
	}
	if state.Liked || state.Favorited || state.FollowingAuthor {
		t.Fatalf("fresh viewer state = %+v", state)
	}

	if err := db.Exec(`INSERT INTO video_likes (user_id, video_id) VALUES (?, ?)`, bob, videoID).Error; err != nil {
		t.Fatalf("like: %v", err)
	}
	if err := db.Exec(`INSERT INTO video_favorites (user_id, video_id) VALUES (?, ?)`, bob, videoID).Error; err != nil {
		t.Fatalf("favorite: %v", err)
	}
	if err := db.Exec(`INSERT INTO user_follows (follower_id, followee_id) VALUES (?, ?)`, bob, alice).Error; err != nil {
		t.Fatalf("follow: %v", err)
	}

	state, err = repo.ViewerState(ctx, bob, videoID, alice)
	if err != nil {
		t.Fatalf("ViewerState: %v", err)
	}
	if !state.Liked || !state.Favorited || !state.FollowingAuthor {
		t.Fatalf("viewer state = %+v, want all true", state)
	}

	anonymous, err := repo.ViewerState(ctx, 0, videoID, alice)
	if err != nil {
		t.Fatalf("anonymous ViewerState: %v", err)
	}
	if anonymous.Liked || anonymous.Favorited || anonymous.FollowingAuthor {
		t.Fatalf("anonymous state = %+v", anonymous)
	}
}

func TestRepositoryUpdateOwnedAndDeleteOwned(t *testing.T) {
	db := openTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()
	alice, bob := seedUsers(t, db)

	id := seedVideo(t, db, alice, "可管理视频", "描述", StatusReady, VisibilityPrivate,
		"2026-09-10 10:00:00", [4]uint64{0, 0, 0, 0})
	if err := db.Exec(`UPDATE videos SET published_at = NULL WHERE id = ?`, id).Error; err != nil {
		t.Fatalf("clear published_at: %v", err)
	}

	public := VisibilityPublic
	private := VisibilityPrivate
	title := "新标题"
	description := "新简介"

	type snapshot struct {
		Title       string
		Description string
		Status      Status
		Visibility  Visibility
		PublishedAt *time.Time
	}
	var row snapshot
	read := func() snapshot {
		t.Helper()
		if err := db.Raw(`SELECT title, description, status, visibility, published_at FROM videos WHERE id = ?`, id).
			Scan(&row).Error; err != nil {
			t.Fatalf("read video: %v", err)
		}
		return row
	}

	if err := repo.UpdateOwned(ctx, bob, id, VideoPatch{Title: &title}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("non-owner update error = %v, want ErrNotFound", err)
	}
	if err := repo.UpdateOwned(ctx, alice, id, VideoPatch{Title: &title, Description: &description}); err != nil {
		t.Fatalf("UpdateOwned: %v", err)
	}
	// Values that are already stored must not be mistaken for a missing row.
	if err := repo.UpdateOwned(ctx, alice, id, VideoPatch{Title: &title, Description: &description}); err != nil {
		t.Fatalf("repeated UpdateOwned: %v", err)
	}
	if got := read(); got.Title != "新标题" || got.Description != "新简介" {
		t.Fatalf("patch not applied: %+v", got)
	}
	if got := read(); got.PublishedAt != nil {
		t.Fatalf("private video published_at = %v, want NULL", got.PublishedAt)
	}

	if err := repo.UpdateOwned(ctx, alice, id, VideoPatch{Visibility: &public}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	first := read().PublishedAt
	if first == nil {
		t.Fatal("first publish must record published_at")
	}
	if _, err := repo.FindPublicByID(ctx, id); err != nil {
		t.Fatalf("published video must be discoverable: %v", err)
	}

	if err := repo.UpdateOwned(ctx, alice, id, VideoPatch{Visibility: &private}); err != nil {
		t.Fatalf("unpublish: %v", err)
	}
	if got := read(); got.PublishedAt == nil || !got.PublishedAt.Equal(*first) {
		t.Fatalf("unpublish published_at = %v, want %v", got.PublishedAt, first)
	}
	if _, err := repo.FindPublicByID(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("private video error = %v, want ErrNotFound", err)
	}

	if err := repo.UpdateOwned(ctx, alice, id, VideoPatch{Visibility: &public}); err != nil {
		t.Fatalf("republish: %v", err)
	}
	if got := read(); got.PublishedAt == nil || !got.PublishedAt.Equal(*first) {
		t.Fatalf("republish published_at = %v, want the original %v", got.PublishedAt, first)
	}

	// A video that is not ready yet never records a publish time.
	uploadingKey := "videos/uploading-management.mp4"
	if err := db.Exec(`INSERT INTO videos (user_id, title, description, object_key, status, visibility, file_size, content_type)
		VALUES (?, ?, '', ?, ?, ?, 1024, 'video/mp4')`,
		alice, "上传中的视频", uploadingKey, StatusUploading, VisibilityPrivate).Error; err != nil {
		t.Fatalf("insert uploading video: %v", err)
	}
	uploadingID := lookupID(t, db, `SELECT id FROM videos WHERE object_key = ?`, uploadingKey)
	if err := repo.UpdateOwned(ctx, alice, uploadingID, VideoPatch{Visibility: &public}); err != nil {
		t.Fatalf("publish uploading video: %v", err)
	}
	var uploadingRow struct{ PublishedAt *time.Time }
	if err := db.Raw(`SELECT published_at FROM videos WHERE id = ?`, uploadingID).Scan(&uploadingRow).Error; err != nil {
		t.Fatalf("read uploading published_at: %v", err)
	}
	if uploadingRow.PublishedAt != nil {
		t.Fatalf("uploading video published_at = %v, want NULL", uploadingRow.PublishedAt)
	}

	if err := repo.DeleteOwned(ctx, alice, id); err != nil {
		t.Fatalf("DeleteOwned: %v", err)
	}
	if got := read(); got.Status != StatusDeleted || got.Visibility != VisibilityPrivate {
		t.Fatalf("deleted row = %+v", got)
	}
	if _, err := repo.FindPublicByID(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted video error = %v, want ErrNotFound", err)
	}
	if err := repo.DeleteOwned(ctx, alice, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("repeated DeleteOwned error = %v, want ErrNotFound", err)
	}
	if err := repo.UpdateOwned(ctx, alice, id, VideoPatch{Title: &title}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update deleted video error = %v, want ErrNotFound", err)
	}
}

func TestRepositoryCreateCreatesZeroedStats(t *testing.T) {
	db := openTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()
	alice, _ := seedUsers(t, db)

	v := &Video{
		UserID:      alice,
		Title:       "新建投稿",
		Description: "介绍",
		ObjectKey:   "pending/1/abc",
		Status:      StatusUploading,
		Visibility:  VisibilityPublic,
		FileSize:    2048,
		ContentType: "video/mp4",
	}
	if err := repo.Create(ctx, v); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if v.ID == 0 {
		t.Fatal("expected generated id")
	}

	var stats struct {
		ViewCount     uint64
		LikeCount     uint64
		FavoriteCount uint64
		CommentCount  uint64
	}
	if err := db.Raw(`SELECT view_count, like_count, favorite_count, comment_count FROM video_stats WHERE video_id = ?`, v.ID).
		Scan(&stats).Error; err != nil {
		t.Fatalf("read stats: %v", err)
	}
	if stats.ViewCount != 0 || stats.LikeCount != 0 || stats.FavoriteCount != 0 || stats.CommentCount != 0 {
		t.Fatalf("stats = %+v, want zeroed row", stats)
	}

	var count int64
	if err := db.Raw(`SELECT COUNT(*) FROM video_stats WHERE video_id = ?`, v.ID).Scan(&count).Error; err != nil {
		t.Fatalf("count stats: %v", err)
	}
	if count != 1 {
		t.Fatalf("stats rows = %d, want 1", count)
	}
}
