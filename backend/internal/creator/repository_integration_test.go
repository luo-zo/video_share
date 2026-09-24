//go:build integration

package creator

import (
	"context"
	"errors"
	"testing"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"video_share/internal/database"
	"video_share/internal/follow"
	"video_share/internal/testutil"
	"video_share/internal/user"
	"video_share/internal/video"
)

func openCreatorTestDB(t *testing.T) *gorm.DB {
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

func lookupCreatorID(t *testing.T, db *gorm.DB, username string) uint64 {
	t.Helper()
	var row struct{ ID uint64 }
	if err := db.Raw("SELECT id FROM users WHERE username = ?", username).Scan(&row).Error; err != nil || row.ID == 0 {
		t.Fatalf("lookup %s: %v", username, err)
	}
	return row.ID
}

func TestPublicCreatorFiltersHiddenStateAndRelations(t *testing.T) {
	db := openCreatorTestDB(t)
	for _, row := range []struct {
		name   string
		status user.Status
	}{
		{"creator_stage5", user.StatusNormal},
		{"follower_stage5", user.StatusNormal},
		{"viewer_stage5", user.StatusNormal},
		{"disabled_stage5", user.StatusDisabled},
	} {
		if err := db.Exec("INSERT INTO users (username, password_hash, nickname, bio, status) VALUES (?, 'hash', ?, ?, ?)", row.name, row.name, "公开简介", row.status).Error; err != nil {
			t.Fatalf("insert user %s: %v", row.name, err)
		}
	}
	creatorUser := lookupCreatorID(t, db, "creator_stage5")
	follower := lookupCreatorID(t, db, "follower_stage5")
	viewer := lookupCreatorID(t, db, "viewer_stage5")
	disabled := lookupCreatorID(t, db, "disabled_stage5")
	if err := db.Exec("INSERT INTO user_follows (follower_id, followee_id) VALUES (?, ?), (?, ?), (?, ?)", follower, creatorUser, disabled, creatorUser, creatorUser, viewer).Error; err != nil {
		t.Fatalf("insert follows: %v", err)
	}
	insertVideo := func(owner uint64, suffix string, status video.Status, visibility video.Visibility) uint64 {
		result := db.Exec(`INSERT INTO videos (user_id, title, description, object_key, status, visibility, file_size, content_type, published_at)
			VALUES (?, ?, '', ?, ?, ?, 1, 'video/mp4', UTC_TIMESTAMP(3))`, owner, suffix, "creator/"+suffix, status, visibility)
		if result.Error != nil {
			t.Fatalf("insert video %s: %v", suffix, result.Error)
		}
		var id uint64
		if err := db.Raw("SELECT id FROM videos WHERE object_key = ?", "creator/"+suffix).Scan(&id).Error; err != nil || id == 0 {
			t.Fatalf("lookup video %s: %v", suffix, err)
		}
		if err := db.Exec("INSERT INTO video_stats (video_id) VALUES (?)", id).Error; err != nil {
			t.Fatalf("insert stats %s: %v", suffix, err)
		}
		return id
	}
	publicID := insertVideo(creatorUser, "public", video.StatusReady, video.VisibilityPublic)
	insertVideo(creatorUser, "private", video.StatusReady, video.VisibilityPrivate)
	insertVideo(creatorUser, "processing", video.StatusProcessing, video.VisibilityPublic)
	insertVideo(disabled, "disabled-owner", video.StatusReady, video.VisibilityPublic)
	_ = publicID

	follows := follow.NewService(follow.NewRepository(db))
	service := NewService(NewRepository(db), video.NewRepository(db), follows)
	profile, err := service.Profile(context.Background(), creatorUser, follower)
	if err != nil {
		t.Fatalf("profile: %v", err)
	}
	if profile.VideoCount != 1 || profile.FollowerCount != 1 || profile.FollowingCount != 1 || !profile.Following {
		t.Fatalf("profile counts/state = %+v", profile)
	}
	videos, err := service.Videos(context.Background(), creatorUser, 1, 20)
	if err != nil || videos.Total != 1 || len(videos.Items) != 1 || videos.Items[0].ID != publicID {
		t.Fatalf("public videos = %+v, %v", videos, err)
	}
	followers, err := service.Followers(context.Background(), creatorUser, 1, 20)
	if err != nil || followers.Total != 1 || len(followers.Items) != 1 || followers.Items[0].ID != follower {
		t.Fatalf("followers = %+v, %v", followers, err)
	}
	following, err := service.Following(context.Background(), creatorUser, 1, 20)
	if err != nil || following.Total != 1 || following.Items[0].ID != viewer {
		t.Fatalf("following = %+v, %v", following, err)
	}
	if _, err := service.Profile(context.Background(), disabled, follower); !errors.Is(err, ErrNotFound) {
		t.Fatalf("disabled creator error = %v, want not found", err)
	}
	if _, err := service.Videos(context.Background(), disabled, 1, 20); !errors.Is(err, ErrNotFound) {
		t.Fatalf("disabled creator videos error = %v, want not found", err)
	}
	if _, err := service.Followers(context.Background(), disabled, 1, 20); !errors.Is(err, ErrNotFound) {
		t.Fatalf("disabled creator followers error = %v, want not found", err)
	}
	if _, err := service.Profile(context.Background(), 999999, follower); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing creator error = %v, want not found", err)
	}
}
