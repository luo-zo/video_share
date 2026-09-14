//go:build integration

package database

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	"github.com/pressly/goose/v3"

	"video_share/internal/testutil"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dsn := testutil.MySQLDSN(t)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		t.Fatalf("ping: %v", err)
	}

	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close test db: %v", err)
		}
	})
	return db
}

// TestMigrateMySQLDialect 验证迁移路径固定使用 MySQL 方言：在空数据库上首次
// 迁移必须成功，重复执行必须是空操作，status 必须报告迁移已应用，down 必须
// 将其回滚。
func TestMigrateMySQLDialect(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	migrator, err := NewMigrator(db)
	if err != nil {
		t.Fatalf("NewMigrator: %v", err)
	}

	if _, err := migrator.Up(ctx); err != nil {
		t.Fatalf("first migrate up: %v", err)
	}

	if _, err := migrator.Up(ctx); err != nil {
		t.Fatalf("repeat migrate up: %v", err)
	}

	statuses, err := migrator.Status(ctx)
	if err != nil {
		t.Fatalf("migrate status: %v", err)
	}
	if len(statuses) == 0 {
		t.Fatal("expected at least one migration status")
	}
	for _, s := range statuses {
		if s.State != goose.StateApplied {
			t.Errorf("migration %d state = %q, want applied", s.Source.Version, s.State)
		}
	}

	if _, err := migrator.Down(ctx); err != nil {
		t.Fatalf("migrate down: %v", err)
	}
}
