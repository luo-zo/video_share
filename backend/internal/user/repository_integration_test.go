//go:build integration

package user

import (
	"context"
	"testing"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"video_share/internal/database"
	"video_share/internal/testutil"
)

// openTestDB 为本次测试运行创建一个全新的、唯一命名的 MySQL 数据库，在其上运行
// 迁移，并返回 GORM 句柄。清理时仅删除该数据库——绝不触及 DSN 指定的原始数据库。
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

func TestRepositoryIntegration(t *testing.T) {
	db := openTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()

	u := &User{Username: "integration_user", PasswordHash: "hash", Nickname: "测试", Status: StatusNormal}
	if err := repo.Create(ctx, u); err != nil {
		t.Fatalf("create: %v", err)
	}
	if u.ID == 0 {
		t.Fatal("expected auto-increment id")
	}

	got, err := repo.FindByUsername(ctx, "integration_user")
	if err != nil {
		t.Fatalf("find by username: %v", err)
	}
	if got.ID != u.ID || got.Username != "integration_user" || got.Nickname != "测试" {
		t.Fatalf("unexpected user: %+v", got)
	}

	got, err = repo.FindByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("find by id: %v", err)
	}
	if got.Username != "integration_user" {
		t.Fatalf("username = %q", got.Username)
	}

	dup := &User{Username: "integration_user", PasswordHash: "hash", Nickname: "x", Status: StatusNormal}
	if err := repo.Create(ctx, dup); err == nil || !IsDuplicateKey(err) {
		t.Fatalf("expected duplicate key error, got %v", err)
	}

	hash, _ := bcrypt.GenerateFromPassword([]byte("secret123"), bcrypt.DefaultCost)
	su := &User{Username: "integration_hash", PasswordHash: string(hash), Nickname: "h", Status: StatusNormal}
	if err := repo.Create(ctx, su); err != nil {
		t.Fatalf("create hash user: %v", err)
	}
	got2, err := repo.FindByUsername(ctx, "integration_hash")
	if err != nil {
		t.Fatalf("find hash user: %v", err)
	}
	if got2.PasswordHash == "secret123" {
		t.Fatal("password stored in plaintext")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(got2.PasswordHash), []byte("secret123")); err != nil {
		t.Fatalf("stored hash not verifiable: %v", err)
	}

	if _, err := repo.FindByUsername(ctx, "no_such_user"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
