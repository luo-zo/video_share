//go:build integration

package follow

import (
	"context"
	"errors"
	"sync"
	"testing"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"video_share/internal/database"
	"video_share/internal/testutil"
	"video_share/internal/user"
)

// openFollowTestDB 创建一个本次运行独有的 MySQL 数据库并完成迁移，返回 GORM 句柄。
func openFollowTestDB(t *testing.T) *gorm.DB {
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

func seedFollowUsers(t *testing.T, db *gorm.DB) (alice, bob, carol, disabled uint64) {
	t.Helper()
	insert := func(name string, status user.Status) {
		if err := db.Exec(`INSERT INTO users (username, password_hash, nickname, status) VALUES (?, ?, ?, ?)`,
			name, "hash", name, status).Error; err != nil {
			t.Fatalf("insert %s: %v", name, err)
		}
	}
	insert("alice", user.StatusNormal)
	insert("bob", user.StatusNormal)
	insert("carol", user.StatusNormal)
	insert("dave", user.StatusDisabled)
	return lookupFollowID(t, db, "SELECT id FROM users WHERE username = ?", "alice"),
		lookupFollowID(t, db, "SELECT id FROM users WHERE username = ?", "bob"),
		lookupFollowID(t, db, "SELECT id FROM users WHERE username = ?", "carol"),
		lookupFollowID(t, db, "SELECT id FROM users WHERE username = ?", "dave")
}

func lookupFollowID(t *testing.T, db *gorm.DB, query string, args ...any) uint64 {
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

func followRowCount(t *testing.T, db *gorm.DB, followerID, followeeID uint64) int64 {
	t.Helper()
	var count int64
	if err := db.Raw("SELECT COUNT(*) FROM user_follows WHERE follower_id = ? AND followee_id = ?", followerID, followeeID).
		Scan(&count).Error; err != nil {
		t.Fatalf("count follows: %v", err)
	}
	return count
}

func TestFollowGraphTransactions(t *testing.T) {
	ctx := context.Background()

	t.Run("first follow records the relation", func(t *testing.T) {
		db := openFollowTestDB(t)
		alice, bob, _, _ := seedFollowUsers(t, db)
		repo := NewRepository(db)

		state, err := repo.SetFollow(ctx, alice, bob, true)
		if err != nil {
			t.Fatalf("SetFollow: %v", err)
		}
		if !state.Following {
			t.Fatalf("state = %+v, want following", state)
		}
		if got := followRowCount(t, db, alice, bob); got != 1 {
			t.Fatalf("rows = %d, want 1", got)
		}
	})

	t.Run("repeated follow stays idempotent", func(t *testing.T) {
		db := openFollowTestDB(t)
		alice, bob, _, _ := seedFollowUsers(t, db)
		repo := NewRepository(db)

		for i := 0; i < 3; i++ {
			state, err := repo.SetFollow(ctx, alice, bob, true)
			if err != nil {
				t.Fatalf("SetFollow #%d: %v", i, err)
			}
			if !state.Following {
				t.Fatalf("state #%d = %+v, want following", i, state)
			}
		}
		if got := followRowCount(t, db, alice, bob); got != 1 {
			t.Fatalf("rows = %d, want exactly 1", got)
		}
	})

	t.Run("unfollow removes the relation", func(t *testing.T) {
		db := openFollowTestDB(t)
		alice, bob, _, _ := seedFollowUsers(t, db)
		repo := NewRepository(db)

		if _, err := repo.SetFollow(ctx, alice, bob, true); err != nil {
			t.Fatalf("follow: %v", err)
		}
		state, err := repo.SetFollow(ctx, alice, bob, false)
		if err != nil {
			t.Fatalf("unfollow: %v", err)
		}
		if state.Following {
			t.Fatalf("state = %+v, want not following", state)
		}
		if got := followRowCount(t, db, alice, bob); got != 0 {
			t.Fatalf("rows = %d, want 0", got)
		}
	})

	t.Run("repeated unfollow stays idempotent", func(t *testing.T) {
		db := openFollowTestDB(t)
		alice, bob, _, _ := seedFollowUsers(t, db)
		repo := NewRepository(db)

		for i := 0; i < 3; i++ {
			state, err := repo.SetFollow(ctx, alice, bob, false)
			if err != nil {
				t.Fatalf("SetFollow #%d: %v", i, err)
			}
			if state.Following {
				t.Fatalf("state #%d = %+v, want not following", i, state)
			}
		}
		if got := followRowCount(t, db, alice, bob); got != 0 {
			t.Fatalf("rows = %d, want 0", got)
		}
	})

	t.Run("concurrent duplicate follow and unfollow return current states", func(t *testing.T) {
		db := openFollowTestDB(t)
		alice, bob, _, _ := seedFollowUsers(t, db)
		repo := NewRepository(db)
		const workers = 8

		run := func(active bool) {
			t.Helper()
			states := make(chan *FollowState, workers)
			errs := make(chan error, workers)
			var wg sync.WaitGroup
			for i := 0; i < workers; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					state, err := repo.SetFollow(ctx, alice, bob, active)
					if err != nil {
						errs <- err
						return
					}
					states <- state
				}()
			}
			wg.Wait()
			close(errs)
			close(states)
			for err := range errs {
				t.Fatalf("SetFollow(%v): %v", active, err)
			}
			for state := range states {
				if state.Following != active {
					t.Fatalf("SetFollow(%v) state = %+v", active, state)
				}
			}
		}

		run(true)
		run(false)
	})

	t.Run("following a missing user is not found", func(t *testing.T) {
		db := openFollowTestDB(t)
		alice, _, _, _ := seedFollowUsers(t, db)
		repo := NewRepository(db)

		if _, err := repo.SetFollow(ctx, alice, 999999, true); !errors.Is(err, ErrUserNotFound) {
			t.Fatalf("err = %v, want ErrUserNotFound", err)
		}
	})

	t.Run("following a disabled user is not found", func(t *testing.T) {
		db := openFollowTestDB(t)
		alice, _, _, disabled := seedFollowUsers(t, db)
		repo := NewRepository(db)

		if _, err := repo.SetFollow(ctx, alice, disabled, true); !errors.Is(err, ErrUserNotFound) {
			t.Fatalf("err = %v, want ErrUserNotFound", err)
		}
		if got := followRowCount(t, db, alice, disabled); got != 0 {
			t.Fatalf("rows = %d, want 0", got)
		}
	})

	t.Run("follow list returns followed users newest first", func(t *testing.T) {
		db := openFollowTestDB(t)
		alice, bob, carol, _ := seedFollowUsers(t, db)
		repo := NewRepository(db)

		if _, err := repo.SetFollow(ctx, alice, bob, true); err != nil {
			t.Fatalf("follow bob: %v", err)
		}
		// 把 bob 的关注时间往前推，保证 carol 严格更新，避免同毫秒并列。
		if err := db.Exec(`UPDATE user_follows SET created_at = created_at - INTERVAL 1 MINUTE
			WHERE follower_id = ? AND followee_id = ?`, alice, bob).Error; err != nil {
			t.Fatalf("nudge bob: %v", err)
		}
		if _, err := repo.SetFollow(ctx, alice, carol, true); err != nil {
			t.Fatalf("follow carol: %v", err)
		}

		users, total, err := repo.ListFollows(ctx, alice, 1, 12)
		if err != nil {
			t.Fatalf("ListFollows: %v", err)
		}
		if total != 2 || len(users) != 2 {
			t.Fatalf("total=%d users=%+v, want 2", total, users)
		}
		if users[0].ID != carol || users[1].ID != bob {
			t.Fatalf("order = %+v, want carol then bob", users)
		}
		if users[0].Username != "carol" || users[0].Nickname != "carol" {
			t.Fatalf("projection = %+v, want carol fields", users[0])
		}
	})

	t.Run("follow list excludes disabled users", func(t *testing.T) {
		db := openFollowTestDB(t)
		alice, bob, _, disabled := seedFollowUsers(t, db)
		repo := NewRepository(db)

		// 直接写入禁用用户的关注行，绕过服务层的目标校验，验证查询会过滤。
		if err := db.Exec(`INSERT INTO user_follows (follower_id, followee_id) VALUES (?, ?)`,
			alice, disabled).Error; err != nil {
			t.Fatalf("insert disabled follow: %v", err)
		}
		if _, err := repo.SetFollow(ctx, alice, bob, true); err != nil {
			t.Fatalf("follow bob: %v", err)
		}

		users, total, err := repo.ListFollows(ctx, alice, 1, 12)
		if err != nil {
			t.Fatalf("ListFollows: %v", err)
		}
		if total != 1 || len(users) != 1 || users[0].ID != bob {
			t.Fatalf("total=%d users=%+v, want only bob", total, users)
		}
	})

	t.Run("follow list paginates newest first", func(t *testing.T) {
		db := openFollowTestDB(t)
		alice, bob, carol, _ := seedFollowUsers(t, db)
		repo := NewRepository(db)

		if _, err := repo.SetFollow(ctx, alice, bob, true); err != nil {
			t.Fatalf("follow bob: %v", err)
		}
		if err := db.Exec(`UPDATE user_follows SET created_at = created_at - INTERVAL 1 MINUTE
			WHERE follower_id = ? AND followee_id = ?`, alice, bob).Error; err != nil {
			t.Fatalf("nudge bob: %v", err)
		}
		if _, err := repo.SetFollow(ctx, alice, carol, true); err != nil {
			t.Fatalf("follow carol: %v", err)
		}

		first, total, err := repo.ListFollows(ctx, alice, 1, 1)
		if err != nil {
			t.Fatalf("page 1: %v", err)
		}
		if total != 2 || len(first) != 1 || first[0].ID != carol {
			t.Fatalf("page1 total=%d users=%+v", total, first)
		}

		second, _, err := repo.ListFollows(ctx, alice, 2, 1)
		if err != nil {
			t.Fatalf("page 2: %v", err)
		}
		if len(second) != 1 || second[0].ID != bob {
			t.Fatalf("page2 users=%+v, want bob", second)
		}
	})
}
