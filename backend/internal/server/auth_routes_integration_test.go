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
	"github.com/google/uuid"
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

func tokenForSession(t *testing.T, tm *token.Manager, userID uint64, familyID string) string {
	t.Helper()
	value, _, err := tm.GenerateWithSession(userID, familyID)
	if err != nil {
		t.Fatalf("generate session token: %v", err)
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
	router := NewRouter(&config.Config{}, db, log, tm, nil, nil)
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

	familyID := uuid.NewString()
	if err := db.Exec(`INSERT INTO session_families (id, user_id, created_at, absolute_expires_at)
		VALUES (?, ?, UTC_TIMESTAMP(3), DATE_ADD(UTC_TIMESTAMP(3), INTERVAL 30 DAY))`, familyID, ids["normal_user"]).Error; err != nil {
		t.Fatalf("seed session family: %v", err)
	}
	normalToken := tokenForSession(t, tm, ids["normal_user"], familyID)
	oversized := `{"padding":"` + strings.Repeat("x", 40*1024) + `","progress_ms":1}`
	request := authenticatedRequest(http.MethodPost, "/api/v1/videos/"+strconv.FormatUint(videoID, 10)+"/watch", normalToken, oversized)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusRequestEntityTooLarge || !strings.Contains(response.Body.String(), `"code":"REQUEST_TOO_LARGE"`) {
		t.Fatalf("oversized watch = %d %s, want 413 REQUEST_TOO_LARGE", response.Code, response.Body.String())
	}
}

func TestLogoutRevokesValidAccessFamilyWithoutRefreshCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openRouterTestDB(t)
	if err := db.Exec(`INSERT INTO users (username, password_hash, nickname, status) VALUES ('logout_user', 'hash', 'logout', 1)`).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	var userID uint64
	if err := db.Raw("SELECT id FROM users WHERE username = 'logout_user'").Scan(&userID).Error; err != nil || userID == 0 {
		t.Fatalf("read user id: %d %v", userID, err)
	}
	familyID := uuid.NewString()
	if err := db.Exec(`INSERT INTO session_families (id, user_id, created_at, absolute_expires_at)
		VALUES (?, ?, UTC_TIMESTAMP(3), DATE_ADD(UTC_TIMESTAMP(3), INTERVAL 30 DAY))`, familyID, userID).Error; err != nil {
		t.Fatalf("seed session family: %v", err)
	}
	tm := token.NewManager("test-secret", "video-share", time.Hour)
	router := NewRouter(&config.Config{}, db, slog.New(slog.NewTextHandler(io.Discard, nil)), tm, nil, nil)
	access := tokenForSession(t, tm, userID, familyID)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", strings.NewReader(`{}`))
	request.Header.Set("Authorization", "Bearer "+access)
	request.Header.Set("Origin", "http://127.0.0.1:5173")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Cookie", "video_share_csrf=csrf")
	request.Header.Set("X-CSRF-Token", "csrf")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d, body=%s", response.Code, response.Body.String())
	}

	protected := httptest.NewRecorder()
	profile := authenticatedRequest(http.MethodGet, "/api/v1/users/me", access, "")
	router.ServeHTTP(protected, profile)
	if protected.Code != http.StatusUnauthorized {
		t.Fatalf("old access token after logout = %d, body=%s", protected.Code, protected.Body.String())
	}
}

func TestReportRouteFallsBackToPerUserRateLimitWhenRedisIsUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openRouterTestDB(t)
	if err := db.Exec(`INSERT INTO users (username, password_hash, nickname, status) VALUES ('report_limiter', 'hash', '举报测试', 1)`).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	var userID uint64
	if err := db.Raw("SELECT id FROM users WHERE username = 'report_limiter'").Scan(&userID).Error; err != nil || userID == 0 {
		t.Fatalf("read user id: %d %v", userID, err)
	}
	if err := db.Exec(`INSERT INTO videos (user_id, title, description, object_key, status, visibility, file_size, content_type)
		VALUES (?, '举报目标', '', 'reports/limiter.mp4', 2, 1, 1, 'video/mp4')`, userID).Error; err != nil {
		t.Fatalf("seed video: %v", err)
	}
	var videoID uint64
	if err := db.Raw("SELECT id FROM videos WHERE object_key = 'reports/limiter.mp4'").Scan(&videoID).Error; err != nil || videoID == 0 {
		t.Fatalf("read video id: %d %v", videoID, err)
	}
	familyID := uuid.NewString()
	if err := db.Exec(`INSERT INTO session_families (id, user_id, created_at, absolute_expires_at)
		VALUES (?, ?, UTC_TIMESTAMP(3), DATE_ADD(UTC_TIMESTAMP(3), INTERVAL 1 DAY))`, familyID, userID).Error; err != nil {
		t.Fatalf("seed session family: %v", err)
	}
	tm := token.NewManager("test-secret", "video-share", time.Hour)
	router := NewRouter(&config.Config{AppOrigin: "http://127.0.0.1:5173", ReportRateLimit: 1, ReportRateWindow: time.Minute}, db, slog.New(slog.NewTextHandler(io.Discard, nil)), tm, nil, nil)
	access := tokenForSession(t, tm, userID, familyID)
	requestReport := func(requestID string) *httptest.ResponseRecorder {
		request := authenticatedRequest(http.MethodPost, "/api/v1/reports", access, `{"target_type":"video","target_id":`+strconv.FormatUint(videoID, 10)+`,"reason_code":"spam","detail":"重复内容","request_id":"`+requestID+`"}`)
		request.Header.Set("Origin", "http://127.0.0.1:5173")
		request.Header.Set("Cookie", "video_share_csrf=csrf")
		request.Header.Set("X-CSRF-Token", "csrf")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	first := requestReport("report-limit-1")
	if first.Code != http.StatusCreated {
		t.Fatalf("first report = %d %s, want 201", first.Code, first.Body.String())
	}
	second := requestReport("report-limit-2")
	if second.Code != http.StatusTooManyRequests || !strings.Contains(second.Body.String(), `"code":"RATE_LIMITED"`) {
		t.Fatalf("second report = %d %s, want 429 RATE_LIMITED", second.Code, second.Body.String())
	}
}
