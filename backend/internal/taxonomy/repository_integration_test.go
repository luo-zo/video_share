//go:build integration

package taxonomy_test

import (
	"context"
	"errors"
	"testing"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"video_share/internal/database"
	"video_share/internal/taxonomy"
	"video_share/internal/testutil"
	"video_share/internal/user"
	"video_share/internal/video"
)

func openTaxonomyTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(mysql.Open(testutil.MySQLDSN(t)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	migrator, err := database.NewMigrator(sqlDB)
	if err != nil {
		t.Fatalf("new migrator: %v", err)
	}
	if _, err := migrator.Up(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func taxonomyUser(t *testing.T, db *gorm.DB, username string, status user.Status) uint64 {
	t.Helper()
	if err := db.Exec("INSERT INTO users (username, password_hash, nickname, status) VALUES (?, 'hash', ?, ?)", username, username, status).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}
	var id uint64
	if err := db.Raw("SELECT id FROM users WHERE username = ?", username).Scan(&id).Error; err != nil || id == 0 {
		t.Fatalf("lookup user: %v", err)
	}
	return id
}

func taxonomyVideo(t *testing.T, db *gorm.DB, owner, category uint64, title string) uint64 {
	t.Helper()
	if err := db.Exec(`INSERT INTO videos (user_id, category_id, title, description, object_key, status, visibility, file_size, content_type, published_at)
        VALUES (?, ?, ?, '', ?, ?, ?, 1, 'video/mp4', UTC_TIMESTAMP(3))`, owner, category, title, "tax/"+title, video.StatusReady, video.VisibilityPublic).Error; err != nil {
		t.Fatalf("insert video: %v", err)
	}
	var id uint64
	if err := db.Raw("SELECT id FROM videos WHERE object_key = ?", "tax/"+title).Scan(&id).Error; err != nil {
		t.Fatalf("lookup video: %v", err)
	}
	if err := db.Exec("INSERT INTO video_stats (video_id) VALUES (?)", id).Error; err != nil {
		t.Fatalf("insert stats: %v", err)
	}
	return id
}

func TestTaxonomyApplyAndPublicFiltering(t *testing.T) {
	db := openTaxonomyTestDB(t)
	ctx := context.Background()
	owner := taxonomyUser(t, db, "taxonomy_owner", user.StatusNormal)
	viewer := taxonomyUser(t, db, "taxonomy_viewer", user.StatusNormal)
	disabled := taxonomyUser(t, db, "taxonomy_disabled", user.StatusDisabled)
	if err := db.Exec("INSERT INTO user_follows (follower_id, followee_id) VALUES (?, ?)", viewer, owner).Error; err != nil {
		t.Fatalf("follow: %v", err)
	}
	repo := taxonomy.NewRepository(db)
	service := taxonomy.NewService(repo, nil)
	selection, err := service.ValidateSelection(ctx, 4, []string{" Café ", "CAFÉ", "go"})
	if err != nil || len(selection.Tags) != 2 || selection.Tags[0].Normalized != "café" {
		t.Fatalf("selection = %+v, err=%v", selection, err)
	}
	first := taxonomyVideo(t, db, owner, 4, "first")
	second := taxonomyVideo(t, db, owner, 4, "second")
	other := taxonomyVideo(t, db, owner, 5, "other")
	hidden := taxonomyVideo(t, db, owner, 4, "private")
	if err := db.Exec("UPDATE videos SET visibility = ? WHERE id = ?", video.VisibilityPrivate, hidden).Error; err != nil {
		t.Fatalf("hide: %v", err)
	}
	disabledVideo := taxonomyVideo(t, db, disabled, 4, "disabled")
	if err := service.ApplyVideo(ctx, first, selection); err != nil {
		t.Fatalf("apply first: %v", err)
	}
	if err := service.ApplyVideo(ctx, second, selection); err != nil {
		t.Fatalf("apply second: %v", err)
	}
	otherSelection, err := service.ValidateSelection(ctx, 5, []string{"go"})
	if err != nil {
		t.Fatalf("other selection: %v", err)
	}
	if err := service.ApplyVideo(ctx, other, otherSelection); err != nil {
		t.Fatalf("apply other: %v", err)
	}
	if err := service.ApplyVideo(ctx, disabledVideo, selection); err != nil {
		t.Fatalf("apply disabled: %v", err)
	}
	loaded, err := repo.ForVideos(ctx, []uint64{first, second})
	if err != nil || len(loaded[first].Tags) != 2 || loaded[first].Category == nil || loaded[first].Category.Slug != "technology" {
		t.Fatalf("loaded taxonomy = %+v, err=%v", loaded, err)
	}
	videoRepo := video.NewRepository(db)
	items, total, err := videoRepo.ListPublic(ctx, video.ListQuery{Page: 1, PageSize: 20, CategoryID: 4, Tags: []string{"café", "go"}})
	if err != nil || total != 2 || len(items) != 2 {
		t.Fatalf("filtered videos = %v total=%d err=%v", items, total, err)
	}
	following, total, err := videoRepo.ListFollowing(ctx, viewer, 1, 20, video.ListQuery{Page: 1, PageSize: 20, Sort: video.SortLatest})
	if err != nil || total != 3 {
		t.Fatalf("following videos = %v total=%d err=%v", following, total, err)
	}
	related, err := videoRepo.ListRelated(ctx, first, 6)
	if err != nil || len(related) == 0 || related[0].ID != second {
		t.Fatalf("related = %v err=%v", related, err)
	}
	for _, item := range following {
		if item.ID == hidden || item.ID == disabledVideo {
			t.Fatalf("private/disabled video leaked: %d", item.ID)
		}
	}
	if _, err := service.ValidateSelection(ctx, 999, nil); !errors.Is(err, taxonomy.ErrCategoryNotFound) {
		t.Fatalf("missing category err=%v", err)
	}
}
