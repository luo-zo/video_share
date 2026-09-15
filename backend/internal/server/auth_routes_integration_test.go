//go:build integration

package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"video_share/internal/config"
	"video_share/internal/database"
	"video_share/internal/testutil"
	"video_share/internal/token"
)

func openRouterTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(mysql.Open(testutil.MySQLDSN(t)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
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
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func tokenFor(t *testing.T, tm *token.Manager, userID uint64) string {
	t.Helper()
	value, _, err := tm.Generate(userID)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	return value
}

func authenticatedRequest(method, path, tokenValue, body string) *http.Request {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+tokenValue)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	return request
}

func TestRouterValidatesTokenSubjectAndLimitsWatchBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openRouterTestDB(t)
	if err := db.Exec(`INSERT INTO users (username, password_hash, nickname, status) VALUES
		('normal_user', 'hash', 'normal', 1), ('disabled_user', 'hash', 'disabled', 2)`).Error; err != nil {
		t.Fatalf("seed users: %v", err)
	}
	var users []struct {
		ID       uint64
		Username string
	}
	if err := db.Raw("SELECT id, username FROM users ORDER BY id").Scan(&users).Error; err != nil {
		t.Fatalf("read users: %v", err)
	}
	ids := make(map[string]uint64, len(users))
	for _, user := range users {
		ids[user.Username] = user.ID
	}

	if err := db.Exec(`INSERT INTO videos
		(user_id, title, description, object_key, status, visibility, file_size, content_type, hls_master_key, published_at)
		VALUES (?, 'public', '', 'videos/public/source.mp4', 2, 1, 1, 'video/mp4', 'videos/public/hls/master.m3u8', UTC_TIMESTAMP(3))`,
		ids["normal_user"]).Error; err != nil {
		t.Fatalf("seed video: %v", err)
	}
	var videoID uint64
	if err := db.Raw("SELECT id FROM videos WHERE object_key = 'videos/public/source.mp4'").Scan(&videoID).Error; err != nil || videoID == 0 {
		t.Fatalf("read video id: id=%d err=%v", videoID, err)
	}
	if err := db.Exec("INSERT INTO video_stats (video_id) VALUES (?)", videoID).Error; err != nil {
		t.Fatalf("seed video stats: %v", err)
	}

	tm := token.NewManager("test-secret", "video-share", time.Hour)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouter(&config.Config{}, db, log, tm, nil)
	disabledToken := tokenFor(t, tm, ids["disabled_user"])
	missingToken := tokenFor(t, tm, 999999)

	protected := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/users/me"},
		{http.MethodGet, "/api/v1/users/me/videos"},
		{http.MethodGet, "/api/v1/users/me/videos/1"},
		{http.MethodGet, "/api/v1/users/me/favorites"},
		{http.MethodGet, "/api/v1/users/me/history"},
		{http.MethodGet, "/api/v1/users/me/follows"},
		{http.MethodPost, "/api/v1/videos"},
		{http.MethodPost, "/api/v1/videos/1/complete"},
		{http.MethodPatch, "/api/v1/users/me/videos/1"},
		{http.MethodDelete, "/api/v1/users/me/videos/1"},
		{http.MethodPut, "/api/v1/videos/1/like"},
		{http.MethodDelete, "/api/v1/videos/1/like"},
		{http.MethodPut, "/api/v1/videos/1/favorite"},
		{http.MethodDelete, "/api/v1/videos/1/favorite"},
		{http.MethodPost, "/api/v1/videos/1/watch"},
		{http.MethodPost, "/api/v1/videos/1/comments"},
		{http.MethodDelete, "/api/v1/comments/1"},
		{http.MethodPut, "/api/v1/users/1/follow"},
		{http.MethodDelete, "/api/v1/users/1/follow"},
	}
	for _, subjectToken := range []string{disabledToken, missingToken} {
		for _, route := range protected {
			request := authenticatedRequest(route.method, route.path, subjectToken, "")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("%s %s with inactive subject = %d %s, want 401",
					route.method, route.path, response.Code, response.Body.String())
			}
		}
	}

	for _, subjectToken := range []string{disabledToken, missingToken} {
		request := authenticatedRequest(http.MethodGet, "/api/v1/videos/"+strconv.FormatUint(videoID, 10), subjectToken, "")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK || strings.Contains(response.Body.String(), `"viewer_state"`) {
			t.Fatalf("optional auth response = %d %s, want anonymous detail", response.Code, response.Body.String())
		}
	}

	normalToken := tokenFor(t, tm, ids["normal_user"])
	oversized := `{"padding":"` + strings.Repeat("x", 40*1024) + `","progress_ms":1}`
	request := authenticatedRequest(http.MethodPost, "/api/v1/videos/"+strconv.FormatUint(videoID, 10)+"/watch", normalToken, oversized)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusRequestEntityTooLarge || !strings.Contains(response.Body.String(), `"code":"REQUEST_TOO_LARGE"`) {
		t.Fatalf("oversized watch = %d %s, want 413 REQUEST_TOO_LARGE", response.Code, response.Body.String())
	}
}
