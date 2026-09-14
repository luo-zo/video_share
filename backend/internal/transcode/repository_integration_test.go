//go:build integration

package transcode

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"video_share/internal/database"
	"video_share/internal/testutil"
	"video_share/internal/video"
)

// openJobTestDB creates a fresh uniquely named MySQL database, migrates it, and
// returns a GORM handle. Cleanup drops only this database.
func openJobTestDB(t *testing.T) *gorm.DB {
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

func seedJobOwner(t *testing.T, db *gorm.DB) uint64 {
	t.Helper()
	if err := db.Exec(`INSERT INTO users (username, password_hash, nickname) VALUES (?, ?, ?)`,
		"alice", "hash", "爱丽丝").Error; err != nil {
		t.Fatalf("insert owner: %v", err)
	}
	var row struct{ ID uint64 }
	if err := db.Raw(`SELECT id FROM users WHERE username = ?`, "alice").Scan(&row).Error; err != nil {
		t.Fatalf("lookup owner: %v", err)
	}
	if row.ID == 0 {
		t.Fatal("owner id not generated")
	}
	return row.ID
}

// seedProcessingVideo inserts a video already in the processing state, so the
// transcode worker can publish it.
func seedProcessingVideo(t *testing.T, db *gorm.DB, userID uint64, visibility video.Visibility, publishedAt *time.Time) uint64 {
	t.Helper()
	objectKey := fmt.Sprintf("videos/%d-%d/source.mp4", userID, time.Now().UnixNano())
	if err := db.Exec(`INSERT INTO videos (user_id, title, description, object_key, status, visibility, published_at, file_size, content_type)
		VALUES (?, ?, '', ?, ?, ?, ?, 1024, 'video/mp4')`,
		userID, "发布测试", objectKey, video.StatusProcessing, visibility, publishedAt).Error; err != nil {
		t.Fatalf("insert video: %v", err)
	}
	var row struct{ ID uint64 }
	if err := db.Raw(`SELECT id FROM videos WHERE object_key = ?`, objectKey).Scan(&row).Error; err != nil {
		t.Fatalf("lookup video: %v", err)
	}
	if row.ID == 0 {
		t.Fatal("video id not generated")
	}
	return row.ID
}

func seedRunningJob(t *testing.T, db *gorm.DB, videoID uint64) *Job {
	t.Helper()
	job := &Job{
		JobID:          fmt.Sprintf("job-%d", time.Now().UnixNano()),
		VideoID:        videoID,
		IdempotencyKey: fmt.Sprintf("video:%d:transcode:v1", videoID),
		Status:         JobRunning,
		Attempts:       1,
		MaxAttempts:    3,
		NextRetryAt:    time.Now().UTC(),
	}
	if err := db.Create(job).Error; err != nil {
		t.Fatalf("insert job: %v", err)
	}
	return job
}

func readPublishedAt(t *testing.T, db *gorm.DB, videoID uint64) *time.Time {
	t.Helper()
	var row struct{ PublishedAt *time.Time }
	if err := db.Raw(`SELECT published_at FROM videos WHERE id = ?`, videoID).Scan(&row).Error; err != nil {
		t.Fatalf("read published_at: %v", err)
	}
	return row.PublishedAt
}

func TestMarkSucceededSetsPublishedAtForPublicVideos(t *testing.T) {
	db := openJobTestDB(t)
	repo := NewJobRepository(db, "video.transcode.completed")
	ctx := context.Background()
	owner := seedJobOwner(t, db)

	t.Run("public video records the first publish time", func(t *testing.T) {
		videoID := seedProcessingVideo(t, db, owner, video.VisibilityPublic, nil)
		job := seedRunningJob(t, db, videoID)

		if err := repo.Succeed(ctx, job, Output{HLSMasterKey: "hls/master.m3u8", CoverObjectKey: "cover.jpg", DurationMS: 1000, Width: 640, Height: 360}); err != nil {
			t.Fatalf("Succeed: %v", err)
		}

		if published := readPublishedAt(t, db, videoID); published == nil {
			t.Fatal("public ready video must record published_at")
		}
		var status video.Status
		if err := db.Raw(`SELECT status FROM videos WHERE id = ?`, videoID).Scan(&status).Error; err != nil {
			t.Fatalf("read status: %v", err)
		}
		if status != video.StatusReady {
			t.Fatalf("status = %d, want ready", status)
		}
	})

	t.Run("private video stays unpublished", func(t *testing.T) {
		videoID := seedProcessingVideo(t, db, owner, video.VisibilityPrivate, nil)
		job := seedRunningJob(t, db, videoID)

		if err := repo.Succeed(ctx, job, Output{HLSMasterKey: "hls/master.m3u8", CoverObjectKey: "cover.jpg", DurationMS: 1000, Width: 640, Height: 360}); err != nil {
			t.Fatalf("Succeed: %v", err)
		}

		if published := readPublishedAt(t, db, videoID); published != nil {
			t.Fatalf("private video published_at = %v, want NULL", published)
		}
	})

	t.Run("existing publish time is preserved", func(t *testing.T) {
		first := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
		videoID := seedProcessingVideo(t, db, owner, video.VisibilityPublic, &first)
		job := seedRunningJob(t, db, videoID)

		if err := repo.Succeed(ctx, job, Output{HLSMasterKey: "hls/master.m3u8", CoverObjectKey: "cover.jpg", DurationMS: 1000, Width: 640, Height: 360}); err != nil {
			t.Fatalf("Succeed: %v", err)
		}

		published := readPublishedAt(t, db, videoID)
		if published == nil || !published.UTC().Equal(first) {
			t.Fatalf("published_at = %v, want %v", published, first)
		}
	})
}
