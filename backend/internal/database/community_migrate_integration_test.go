//go:build integration

package database

import (
	"context"
	"database/sql"
	"testing"
)

func TestCommunitySchemaConstraintsAndBackfill(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	migrator, err := NewMigrator(db)
	if err != nil {
		t.Fatalf("NewMigrator: %v", err)
	}
	if _, err := migrator.Up(ctx); err != nil {
		t.Fatalf("initial migrate up: %v", err)
	}
	// Stage5 now has migrations 000006–000010. Roll them back as well as the
	// original community/data migrations, but leave 000003 in place because the
	// partial 000004 simulation references its processed_at column.
	for i := 0; i < 7; i++ {
		if _, err := migrator.Down(ctx); err != nil {
			t.Fatalf("roll back migration %d: %v", i+1, err)
		}
	}

	// Simulate a connection loss after MySQL has committed the first ALTER but
	// before Goose records migration 4. The next Up must recognize and preserve
	// the columns while completing all remaining tables.
	mustExec(t, db, `ALTER TABLE videos
		ADD COLUMN visibility TINYINT NOT NULL DEFAULT 1 COMMENT '1=public 2=private' AFTER status,
		ADD COLUMN published_at DATETIME(3) NULL AFTER processed_at,
		ADD CONSTRAINT chk_videos_visibility CHECK (visibility IN (1, 2)),
		ADD KEY idx_videos_discovery_latest (status, visibility, published_at, id)`)

	mustExec(t, db, `INSERT INTO users (id, username, password_hash, nickname) VALUES
		(1, 'author', 'hash', '作者'), (2, 'viewer', 'hash', '观众')`)
	mustExec(t, db, `INSERT INTO videos
		(id, user_id, title, description, object_key, status, file_size, content_type, processed_at)
		VALUES (1, 1, '猫咪散步', '傍晚记录', 'videos/1/source.mp4', 2, 1024, 'video/mp4', CURRENT_TIMESTAMP(3))`)

	if _, err := migrator.Up(ctx); err != nil {
		t.Fatalf("recover partially applied community migration: %v", err)
	}

	assertSchemaColumnCount(t, db, "videos", []string{"visibility", "published_at"}, 2)
	for _, table := range []string{
		"video_stats", "video_likes", "video_favorites", "comments", "user_follows", "watch_histories",
	} {
		assertTableExists(t, db, table)
	}

	var visibility uint8
	var publishedAt sql.NullTime
	if err := db.QueryRow(`SELECT visibility, published_at FROM videos WHERE id = 1`).Scan(&visibility, &publishedAt); err != nil {
		t.Fatalf("read backfilled video: %v", err)
	}
	if visibility != 1 || !publishedAt.Valid {
		t.Fatalf("backfilled video visibility=%d published_at.Valid=%v", visibility, publishedAt.Valid)
	}
	var statsRows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM video_stats WHERE video_id = 1`).Scan(&statsRows); err != nil {
		t.Fatalf("count stats: %v", err)
	}
	if statsRows != 1 {
		t.Fatalf("video_stats rows=%d, want 1", statsRows)
	}

	mustExec(t, db, `INSERT INTO video_likes (user_id, video_id) VALUES (2, 1)`)
	if _, err := db.Exec(`INSERT INTO video_likes (user_id, video_id) VALUES (2, 1)`); err == nil {
		t.Fatal("duplicate like succeeded")
	}
	if _, err := db.Exec(`INSERT INTO user_follows (follower_id, followee_id) VALUES (2, 2)`); err == nil {
		t.Fatal("self-follow succeeded")
	}
	mustExec(t, db, `INSERT INTO watch_histories (user_id, video_id, progress_ms, duration_ms)
		VALUES (2, 1, 100, 0)`)
	if _, err := db.Exec(`UPDATE watch_histories SET duration_ms = 50 WHERE user_id = 2 AND video_id = 1`); err == nil {
		t.Fatal("progress greater than a known duration succeeded")
	}

	for i := 0; i < 7; i++ {
		if _, err := migrator.Down(ctx); err != nil {
			t.Fatalf("down migration %d: %v", i+1, err)
		}
	}
	assertSchemaColumnCount(t, db, "videos", []string{"visibility", "published_at"}, 0)
}

func TestSessionMigrationRecoversPartiallyAppliedUserAlter(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	migrator, err := NewMigrator(db)
	if err != nil {
		t.Fatalf("NewMigrator: %v", err)
	}
	if _, err := migrator.Up(ctx); err != nil {
		t.Fatalf("initial migrate up: %v", err)
	}
	// Leave migration 000006 pending while simulating a connection loss after
	// MySQL committed only its first additive ALTER statement.
	for i := 0; i < 5; i++ {
		if _, err := migrator.Down(ctx); err != nil {
			t.Fatalf("roll back stage5 migration %d: %v", i+1, err)
		}
	}
	mustExec(t, db, "ALTER TABLE users ADD COLUMN bio VARCHAR(200) NOT NULL DEFAULT '' AFTER nickname")
	if _, err := migrator.Up(ctx); err != nil {
		t.Fatalf("recover sessions migration: %v", err)
	}
	assertSchemaColumnCount(t, db, "users", []string{"bio", "role"}, 2)
	var constraintCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.table_constraints
		WHERE table_schema = DATABASE() AND table_name = 'users' AND constraint_name = 'chk_users_role'`).Scan(&constraintCount); err != nil {
		t.Fatalf("query role constraint: %v", err)
	}
	if constraintCount != 1 {
		t.Fatalf("role constraint count=%d, want 1", constraintCount)
	}
}

func TestModerationMigrationRecoversPartiallyAppliedAdditiveDDL(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	migrator, err := NewMigrator(db)
	if err != nil {
		t.Fatalf("NewMigrator: %v", err)
	}
	if _, err := migrator.Up(ctx); err != nil {
		t.Fatalf("initial migrate up: %v", err)
	}
	// Roll back 000010 and 000009, then simulate a connection loss after its first
	// ALTER committed but before the remaining statements and Goose version
	// record were written.
	for i := 0; i < 2; i++ {
		if _, err := migrator.Down(ctx); err != nil {
			t.Fatalf("roll back moderation migration: %v", err)
		}
	}
	mustExec(t, db, "ALTER TABLE videos ADD COLUMN moderation_status VARCHAR(16) NOT NULL DEFAULT 'visible' AFTER visibility")
	if _, err := migrator.Up(ctx); err != nil {
		t.Fatalf("recover moderation migration: %v", err)
	}
	assertSchemaColumnCount(t, db, "videos", []string{"moderation_status", "visibility"}, 2)
	assertSchemaColumnCount(t, db, "comments", []string{"moderation_status", "deleted_at"}, 2)
	assertTableExists(t, db, "reports")
	assertTableExists(t, db, "moderation_actions")
}

func TestMetricsMigrationRecoversPartiallyAppliedWatchHistoryAlter(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	migrator, err := NewMigrator(db)
	if err != nil {
		t.Fatalf("NewMigrator: %v", err)
	}
	if _, err := migrator.Up(ctx); err != nil {
		t.Fatalf("initial migrate up: %v", err)
	}
	if _, err := migrator.Down(ctx); err != nil {
		t.Fatalf("roll back metrics migration: %v", err)
	}
	mustExec(t, db, "ALTER TABLE watch_histories ADD COLUMN effective_watch_ms BIGINT UNSIGNED NOT NULL DEFAULT 0 AFTER duration_ms")
	if _, err := migrator.Up(ctx); err != nil {
		t.Fatalf("recover metrics migration: %v", err)
	}
	assertSchemaColumnCount(t, db, "watch_histories", []string{"effective_watch_ms", "last_watch_credit_at"}, 2)
	assertTableExists(t, db, "watch_sessions")
	assertTableExists(t, db, "video_daily_metrics")
	assertTableExists(t, db, "creator_daily_metrics")
}

func mustExec(t *testing.T, db *sql.DB, statement string) {
	t.Helper()
	if _, err := db.Exec(statement); err != nil {
		t.Fatalf("exec %q: %v", statement, err)
	}
}

func assertTableExists(t *testing.T, db *sql.DB, table string) {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = DATABASE() AND table_name = ?`, table).Scan(&count); err != nil {
		t.Fatalf("query table %s: %v", table, err)
	}
	if count != 1 {
		t.Fatalf("table %s count=%d, want 1", table, count)
	}
}

func assertSchemaColumnCount(t *testing.T, db *sql.DB, table string, columns []string, want int) {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema = DATABASE() AND table_name = ? AND column_name IN (?, ?)`,
		table, columns[0], columns[1]).Scan(&count); err != nil {
		t.Fatalf("query columns for %s: %v", table, err)
	}
	if count != want {
		t.Fatalf("columns for %s count=%d, want %d", table, count, want)
	}
}
