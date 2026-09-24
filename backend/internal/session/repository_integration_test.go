//go:build integration

package session

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"

	"video_share/internal/database"
	"video_share/internal/testutil"
	"video_share/internal/token"
)

func TestRefreshRotationSerializesConcurrentRequestsAndRevokesLateReplay(t *testing.T) {
	dsn := testutil.MySQLDSN(t)
	sqlDB, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open gorm: %v", err)
	}
	migrator, err := database.NewMigrator(sqlDB)
	if err != nil {
		t.Fatalf("new migrator: %v", err)
	}
	if _, err := migrator.Up(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	username := fmt.Sprintf("session_%d", time.Now().UnixNano())
	if err := db.Exec("INSERT INTO users (username, password_hash, nickname, status) VALUES (?, 'hash', '会话测试', 1)", username).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	var userID uint64
	if err := db.Raw("SELECT id FROM users WHERE username = ?", username).Scan(&userID).Error; err != nil || userID == 0 {
		t.Fatalf("read user id=%d err=%v", userID, err)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	service := NewService(NewRepository(db), token.NewManager("integration-secret", "video-share", 15*time.Minute), Options{
		Now:            func() time.Time { return now },
		FamilyTTL:      30 * 24 * time.Hour,
		RefreshTTL:     30 * 24 * time.Hour,
		ConflictWindow: 10 * time.Second,
	})
	issued, err := service.Issue(context.Background(), userID)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	var wg sync.WaitGroup
	results := make(chan *Issued, 2)
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			value, refreshErr := service.Refresh(context.Background(), issued.RefreshToken)
			results <- value
			errs <- refreshErr
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	var success *Issued
	var conflictCount int
	for err := range errs {
		switch {
		case err == nil:
			for value := range results {
				if value != nil {
					success = value
					break
				}
			}
		case errors.Is(err, ErrRefreshConflict):
			conflictCount++
		default:
			t.Fatalf("concurrent refresh error = %v, want one conflict", err)
		}
	}
	if success == nil || conflictCount != 1 {
		t.Fatalf("concurrent refresh success=%v conflicts=%d, want one success and one conflict", success != nil, conflictCount)
	}

	// Replaying the old token after the grace window is a theft signal: the
	// whole family is revoked, including the winner's replacement token.
	now = now.Add(11 * time.Second)
	if _, err := service.Refresh(context.Background(), issued.RefreshToken); !errors.Is(err, ErrSessionReused) {
		t.Fatalf("late old-token replay = %v, want ErrSessionReused", err)
	}
	if _, err := service.Refresh(context.Background(), success.RefreshToken); !errors.Is(err, ErrSessionRevoked) {
		t.Fatalf("replacement after late replay = %v, want ErrSessionRevoked", err)
	}

	logoutSession, err := service.Issue(context.Background(), userID)
	if err != nil {
		t.Fatalf("issue logout session: %v", err)
	}
	if err := service.Logout(context.Background(), logoutSession.FamilyID); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, err := service.Refresh(context.Background(), logoutSession.RefreshToken); !errors.Is(err, ErrSessionRevoked) {
		t.Fatalf("refresh after logout = %v, want ErrSessionRevoked", err)
	}
	if active, err := service.ValidateAccess(context.Background(), userID, logoutSession.FamilyID); err != nil || active {
		t.Fatalf("access after logout active=%v err=%v, want inactive", active, err)
	}

	refreshOnly, err := service.Issue(context.Background(), userID)
	if err != nil {
		t.Fatalf("issue refresh-only logout session: %v", err)
	}
	if err := service.LogoutByRefreshToken(context.Background(), refreshOnly.RefreshToken); err != nil {
		t.Fatalf("logout by refresh token: %v", err)
	}
	if _, err := service.Refresh(context.Background(), refreshOnly.RefreshToken); !errors.Is(err, ErrSessionRevoked) {
		t.Fatalf("refresh after refresh-only logout = %v, want ErrSessionRevoked", err)
	}
	if active, err := service.ValidateAccess(context.Background(), userID, refreshOnly.FamilyID); err != nil || active {
		t.Fatalf("access after refresh-only logout active=%v err=%v, want inactive", active, err)
	}

	// A disabled account must not be able to rotate a token even if a caller
	// races the moderation transaction before its family-revocation update.
	disabledSession, err := service.Issue(context.Background(), userID)
	if err != nil {
		t.Fatalf("issue disabled-account session: %v", err)
	}
	if err := db.Exec("UPDATE users SET status = 2 WHERE id = ?", userID).Error; err != nil {
		t.Fatalf("disable test user: %v", err)
	}
	if _, err := service.Refresh(context.Background(), disabledSession.RefreshToken); !errors.Is(err, ErrSessionRevoked) {
		t.Fatalf("refresh for disabled account = %v, want ErrSessionRevoked", err)
	}
}
