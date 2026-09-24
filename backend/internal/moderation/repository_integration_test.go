//go:build integration

package moderation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"video_share/internal/database"
	"video_share/internal/engagement"
	"video_share/internal/notification"
	"video_share/internal/testutil"
	"video_share/internal/video"
)

func openModerationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(mysql.Open(testutil.MySQLDSN(t)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	migrator, err := database.NewMigrator(sqlDB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := migrator.Up(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func TestModerationReportLifecycleAndVisibility(t *testing.T) {
	db := openModerationTestDB(t)
	ctx := context.Background()
	var ids struct{ AdminA, AdminB, Reporter, Author uint64 }
	for _, row := range []struct {
		name string
		out  *uint64
	}{{"admin_a", &ids.AdminA}, {"admin_b", &ids.AdminB}, {"reporter", &ids.Reporter}, {"author", &ids.Author}} {
		if err := db.Exec("INSERT INTO users (username, password_hash, nickname, role) VALUES (?, 'hash', ?, ?)", row.name, row.name, map[bool]string{true: "admin", false: "user"}[row.name == "admin_a" || row.name == "admin_b"]).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Raw("SELECT id FROM users WHERE username = ?", row.name).Scan(row.out).Error; err != nil {
			t.Fatal(err)
		}
	}
	objectKey := fmt.Sprintf("moderation/%d.mp4", time.Now().UnixNano())
	if err := db.Exec(`INSERT INTO videos (user_id,title,description,object_key,status,visibility,file_size,content_type) VALUES (?, '待治理', '', ?, 2, 1, 1, 'video/mp4')`, ids.Author, objectKey).Error; err != nil {
		t.Fatal(err)
	}
	var videoID uint64
	if err := db.Raw("SELECT id FROM videos WHERE object_key = ?", objectKey).Scan(&videoID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO video_stats (video_id) VALUES (?)", videoID).Error; err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(db)
	engagementService := engagement.NewService(engagement.NewRepository(db))

	report, err := repo.CreateReport(ctx, ids.Reporter, CreateReportRequest{TargetType: TargetVideo, TargetID: videoID, ReasonCode: ReasonSpam, Detail: "重复内容", RequestID: "report-1"})
	if err != nil {
		t.Fatalf("create report: %v", err)
	}
	replay, err := repo.CreateReport(ctx, ids.Reporter, CreateReportRequest{TargetType: TargetVideo, TargetID: videoID, ReasonCode: ReasonSpam, Detail: "重复内容", RequestID: "report-1"})
	if err != nil || replay.ID != report.ID {
		t.Fatalf("report replay = %+v, %v", replay, err)
	}
	if _, err := repo.CreateReport(ctx, ids.Reporter, CreateReportRequest{TargetType: TargetVideo, TargetID: videoID, ReasonCode: ReasonAbuse, Detail: "另一条", RequestID: "report-2"}); !errors.Is(err, ErrActiveReport) {
		t.Fatalf("active duplicate = %v", err)
	}

	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, adminID := range []uint64{ids.AdminA, ids.AdminB} {
		wg.Add(1)
		go func(id uint64) {
			defer wg.Done()
			_, callErr := repo.AssignReport(ctx, id, report.ID, fmt.Sprintf("assign-%d", id))
			results <- callErr
		}(adminID)
	}
	wg.Wait()
	close(results)
	var assigned, conflicts int
	for callErr := range results {
		if callErr == nil {
			assigned++
		}
		if errors.Is(callErr, ErrReportStateConflict) {
			conflicts++
		}
	}
	if assigned != 1 || conflicts != 1 {
		t.Fatalf("concurrent assign assigned=%d conflicts=%d", assigned, conflicts)
	}

	resolved, err := repo.ResolveReport(ctx, ids.AdminA, report.ID, ReportDecisionRequest{Status: ReportResolved, ResolutionReason: "确认违规并隐藏", Action: ActionHideVideo, Reason: "违反社区规则", RequestID: "resolve-1"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved.Status != ReportResolved || resolved.ActionID == nil {
		t.Fatalf("resolved = %+v", resolved)
	}
	var moderationStatus string
	if err := db.Raw("SELECT moderation_status FROM videos WHERE id = ?", videoID).Scan(&moderationStatus).Error; err != nil {
		t.Fatal(err)
	}
	if moderationStatus != StateHidden {
		t.Fatalf("video moderation status=%q", moderationStatus)
	}
	report3, err := repo.CreateReport(ctx, ids.Reporter, CreateReportRequest{TargetType: TargetVideo, TargetID: videoID, ReasonCode: ReasonOther, Detail: "再次举报", RequestID: "report-3"})
	if err != nil {
		t.Fatalf("new report after close: %v", err)
	}
	// Re-reporting and resolving the same target use the same report→target
	// lock order. Run them together to guard against a MySQL deadlock that
	// would otherwise surface as an opaque 500.
	raceCtx, cancelRace := context.WithTimeout(ctx, 5*time.Second)
	defer cancelRace()
	type raceResult struct {
		kind string
		err  error
	}
	raceDone := make(chan raceResult, 2)
	go func() {
		_, callErr := repo.ResolveReport(raceCtx, ids.AdminA, report3.ID, ReportDecisionRequest{Status: ReportRejected, ResolutionReason: "复核后驳回", RequestID: "race-resolve"})
		raceDone <- raceResult{kind: "resolve", err: callErr}
	}()
	go func() {
		_, callErr := repo.CreateReport(raceCtx, ids.Reporter, CreateReportRequest{TargetType: TargetVideo, TargetID: videoID, ReasonCode: ReasonAbuse, Detail: "并发复核", RequestID: "race-rereport"})
		raceDone <- raceResult{kind: "create", err: callErr}
	}()
	var raceResolveErr, raceCreateErr error
	for i := 0; i < 2; i++ {
		select {
		case result := <-raceDone:
			if result.kind == "resolve" {
				raceResolveErr = result.err
			} else {
				raceCreateErr = result.err
			}
		case <-raceCtx.Done():
			t.Fatal("concurrent report create/resolve timed out")
		}
	}
	if raceResolveErr != nil && !errors.Is(raceResolveErr, ErrReportStateConflict) {
		t.Fatalf("concurrent resolve error = %v", raceResolveErr)
	}
	if raceCreateErr != nil && !errors.Is(raceCreateErr, ErrActiveReport) {
		t.Fatalf("concurrent re-report error = %v", raceCreateErr)
	}
	// An administrator may also report content. In that case report.create
	// and report.resolve receipts share the same actor's unique-index range;
	// absent FOR UPDATE receipts must not deadlock with the report row.
	adminReport, err := repo.CreateReport(ctx, ids.AdminA, CreateReportRequest{TargetType: TargetVideo, TargetID: videoID, ReasonCode: ReasonSpam, RequestID: "admin-report"})
	if err != nil {
		t.Fatal(err)
	}
	sameCtx, cancelSame := context.WithTimeout(ctx, 5*time.Second)
	defer cancelSame()
	sameDone := make(chan raceResult, 2)
	go func() {
		_, callErr := repo.ResolveReport(sameCtx, ids.AdminA, adminReport.ID, ReportDecisionRequest{Status: ReportRejected, ResolutionReason: "管理员复核", RequestID: "admin-resolve"})
		sameDone <- raceResult{kind: "resolve", err: callErr}
	}()
	go func() {
		_, callErr := repo.CreateReport(sameCtx, ids.AdminA, CreateReportRequest{TargetType: TargetVideo, TargetID: videoID, ReasonCode: ReasonOther, RequestID: "admin-rereport"})
		sameDone <- raceResult{kind: "create", err: callErr}
	}()
	for i := 0; i < 2; i++ {
		select {
		case outcome := <-sameDone:
			if outcome.err != nil && !(outcome.kind == "create" && errors.Is(outcome.err, ErrActiveReport)) {
				t.Fatalf("same-actor %s failed: %v", outcome.kind, outcome.err)
			}
		case <-sameCtx.Done():
			t.Fatal("same-actor report create/resolve timed out")
		}
	}
	if _, err := repo.ModerateTarget(ctx, ids.AdminA, TargetVideo, videoID, ActionRestoreVideo, ModerationActionRequest{Reason: "复核后恢复", RequestID: "restore-1"}); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if _, err := repo.ModerateTarget(ctx, ids.AdminA, TargetVideo, videoID, ActionHideVideo, ModerationActionRequest{Reason: "故意错误关联", ReportID: 999999999, RequestID: "rollback-1"}); err == nil {
		t.Fatal("invalid report association unexpectedly succeeded")
	}
	if err := db.Raw("SELECT moderation_status FROM videos WHERE id = ?", videoID).Scan(&moderationStatus).Error; err != nil {
		t.Fatal(err)
	}
	if moderationStatus != StateVisible {
		t.Fatalf("failed action changed video status=%q", moderationStatus)
	}

	var actionCount, notificationCount int64
	if err := db.Model(&ModerationAction{}).Where("target_type = ? AND target_id = ? AND action IN ?", TargetVideo, videoID, []string{ActionHideVideo, ActionRestoreVideo}).Count(&actionCount).Error; err != nil {
		t.Fatal(err)
	}
	if actionCount != 2 {
		t.Fatalf("action count=%d, want hide + restore", actionCount)
	}
	if err := db.Model(&notification.Notification{}).Where("recipient_id = ? AND report_id = ?", ids.Reporter, report.ID).Count(&notificationCount).Error; err != nil {
		t.Fatal(err)
	}
	if notificationCount != 1 {
		t.Fatalf("report notification count=%d", notificationCount)
	}
	if err := db.Exec(`INSERT INTO comments (video_id, user_id, root_id, content, created_at, updated_at) VALUES (?, ?, 0, '需治理的评论', CURRENT_TIMESTAMP(3), CURRENT_TIMESTAMP(3))`, videoID, ids.Reporter).Error; err != nil {
		t.Fatal(err)
	}
	var commentID uint64
	if err := db.Raw("SELECT id FROM comments WHERE video_id = ? ORDER BY id DESC LIMIT 1", videoID).Scan(&commentID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE comments SET root_id = ? WHERE id = ?", commentID, commentID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE video_stats SET comment_count = 1 WHERE video_id = ?", videoID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ModerateTarget(ctx, ids.AdminA, TargetComment, commentID, ActionHideComment, ModerationActionRequest{Reason: "评论违规", RequestID: "comment-hide"}); err != nil {
		t.Fatalf("hide comment: %v", err)
	}
	if _, err := engagementService.CreateCommentWithRequest(ctx, ids.Reporter, videoID, "不应回复隐藏评论", &commentID, "hidden-root-reply"); !errors.Is(err, engagement.ErrParentCommentInvalid) {
		t.Fatalf("reply to hidden root = %v, want ErrParentCommentInvalid", err)
	}
	var commentCount uint64
	if err := db.Raw("SELECT comment_count FROM video_stats WHERE video_id = ?", videoID).Scan(&commentCount).Error; err != nil {
		t.Fatal(err)
	}
	if commentCount != 0 {
		t.Fatalf("hidden comment count=%d, want 0", commentCount)
	}
	if _, err := repo.ModerateTarget(ctx, ids.AdminA, TargetComment, commentID, ActionRestoreComment, ModerationActionRequest{Reason: "评论复核通过", RequestID: "comment-restore"}); err != nil {
		t.Fatalf("restore comment: %v", err)
	}
	if err := db.Raw("SELECT comment_count FROM video_stats WHERE video_id = ?", videoID).Scan(&commentCount).Error; err != nil {
		t.Fatal(err)
	}
	if commentCount != 1 {
		t.Fatalf("restored comment count=%d, want 1", commentCount)
	}
	reply, err := engagementService.CreateCommentWithRequest(ctx, ids.Reporter, videoID, "待隐藏的回复", &commentID, "reply-create")
	if err != nil {
		t.Fatalf("create reply: %v", err)
	}
	if _, err := repo.ModerateTarget(ctx, ids.AdminA, TargetComment, reply.ID, ActionHideComment, ModerationActionRequest{Reason: "回复违规", RequestID: "reply-hide"}); err != nil {
		t.Fatalf("hide reply: %v", err)
	}
	if _, err := engagementService.CreateCommentWithRequest(ctx, ids.Reporter, videoID, "不应嵌套隐藏回复", &reply.ID, "hidden-reply-reply"); !errors.Is(err, engagement.ErrParentCommentInvalid) {
		t.Fatalf("reply to hidden reply = %v, want ErrParentCommentInvalid", err)
	}

	// Disabling an account revokes every refresh family in the same
	// transaction, so a concurrent refresh cannot mint a replacement session.
	familyID := uuid.NewString()
	if err := db.Exec(`INSERT INTO session_families (id, user_id, created_at, absolute_expires_at)
		VALUES (?, ?, CURRENT_TIMESTAMP(3), DATE_ADD(CURRENT_TIMESTAMP(3), INTERVAL 1 DAY))`, familyID, ids.Author).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ModerateTarget(ctx, ids.AdminA, TargetUser, ids.Author, ActionDisableUser, ModerationActionRequest{Reason: "账号违规", RequestID: "disable-author"}); err != nil {
		t.Fatalf("disable user: %v", err)
	}
	var revokedAt sql.NullTime
	if err := db.Raw("SELECT revoked_at FROM session_families WHERE id = ?", familyID).Scan(&revokedAt).Error; err != nil {
		t.Fatal(err)
	}
	if !revokedAt.Valid {
		t.Fatal("disabled user session family was not revoked")
	}

	// The two administrators race to disable one another. Stable locking of
	// the full administrator set must leave exactly one active administrator.
	results = make(chan error, 2)
	for _, pair := range [][2]uint64{{ids.AdminA, ids.AdminB}, {ids.AdminB, ids.AdminA}} {
		wg.Add(1)
		go func(actor, target uint64) {
			defer wg.Done()
			_, callErr := repo.ModerateTarget(ctx, actor, TargetUser, target, ActionDisableUser, ModerationActionRequest{Reason: "管理员并发治理", RequestID: fmt.Sprintf("disable-admin-%d", target)})
			results <- callErr
		}(pair[0], pair[1])
	}
	wg.Wait()
	close(results)
	var disabledAdmin, lastAdminConflicts int
	for callErr := range results {
		if callErr == nil {
			disabledAdmin++
		}
		if errors.Is(callErr, ErrLastAdmin) {
			lastAdminConflicts++
		}
	}
	if disabledAdmin != 1 || lastAdminConflicts != 1 {
		t.Fatalf("concurrent admin disable successes=%d last-admin-conflicts=%d", disabledAdmin, lastAdminConflicts)
	}
}

func TestModerationCannotRestoreDeletedTarget(t *testing.T) {
	db := openModerationTestDB(t)
	var admin, author uint64
	if err := db.Exec("INSERT INTO users (username,password_hash,nickname,role) VALUES ('admin_restore','hash','admin','admin'),('author_restore','hash','author','user')").Error; err != nil {
		t.Fatal(err)
	}
	db.Raw("SELECT id FROM users WHERE username = 'admin_restore'").Scan(&admin)
	db.Raw("SELECT id FROM users WHERE username = 'author_restore'").Scan(&author)
	if err := db.Exec("INSERT INTO videos (user_id,title,description,object_key,status,visibility,file_size,content_type) VALUES (?, 'deleted', '', 'moderation/deleted.mp4', 4, 2, 1, 'video/mp4')", author).Error; err != nil {
		t.Fatal(err)
	}
	var id uint64
	db.Raw("SELECT id FROM videos WHERE object_key = 'moderation/deleted.mp4'").Scan(&id)
	_, err := NewRepository(db).ModerateTarget(context.Background(), admin, TargetVideo, id, ActionRestoreVideo, ModerationActionRequest{Reason: "不可恢复", RequestID: "restore-deleted"})
	if !errors.Is(err, ErrTargetDeleted) {
		t.Fatalf("restore deleted = %v", err)
	}
}

func TestDirectActionWithReportAndResolutionKeepOneLockOrder(t *testing.T) {
	db := openModerationTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.Exec(`INSERT INTO users (username,password_hash,nickname,role) VALUES
		('race_admin','hash','admin','admin'),
		('race_reporter','hash','reporter','user'),
		('race_author','hash','author','user')`).Error; err != nil {
		t.Fatal(err)
	}
	var adminID, reporterID, authorID uint64
	for _, pair := range []struct {
		name string
		id   *uint64
	}{{"race_admin", &adminID}, {"race_reporter", &reporterID}, {"race_author", &authorID}} {
		if err := db.Raw("SELECT id FROM users WHERE username = ?", pair.name).Scan(pair.id).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Exec(`INSERT INTO videos (user_id,title,description,object_key,status,visibility,file_size,content_type)
		VALUES (?, 'Race target', '', 'moderation/race-target.mp4', 2, 1, 1, 'video/mp4')`, authorID).Error; err != nil {
		t.Fatal(err)
	}
	var videoID uint64
	if err := db.Raw("SELECT id FROM videos WHERE object_key = 'moderation/race-target.mp4'").Scan(&videoID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO video_stats (video_id) VALUES (?)", videoID).Error; err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(db)
	report, err := repo.CreateReport(ctx, reporterID, CreateReportRequest{TargetType: TargetVideo, TargetID: videoID, ReasonCode: ReasonSpam, RequestID: "race-create"})
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	go func() {
		<-start
		_, err := repo.ModerateTarget(ctx, adminID, TargetVideo, videoID, ActionHideVideo, ModerationActionRequest{Reason: "直连隐藏", ReportID: report.ID, RequestID: "race-direct"})
		results <- err
	}()
	go func() {
		<-start
		_, err := repo.ResolveReport(ctx, adminID, report.ID, ReportDecisionRequest{Status: ReportResolved, ResolutionReason: "确认违规", Action: ActionHideVideo, Reason: "结案隐藏", RequestID: "race-close"})
		results <- err
	}()
	close(start)
	var actionErrors []error
	for i := 0; i < 2; i++ {
		select {
		case err := <-results:
			if err != nil {
				actionErrors = append(actionErrors, err)
			}
		case <-ctx.Done():
			t.Fatal("concurrent action timed out")
		}
	}
	if len(actionErrors) != 0 {
		t.Fatalf("concurrent action failed: %v", actionErrors)
	}
	var state string
	if err := db.Raw("SELECT moderation_status FROM videos WHERE id = ?", videoID).Scan(&state).Error; err != nil {
		t.Fatal(err)
	}
	if state != StateHidden {
		t.Fatalf("moderation status=%q, want hidden", state)
	}
	if _, err := repo.ModerateTarget(ctx, adminID, TargetVideo, videoID, ActionHideVideo, ModerationActionRequest{Reason: "直连隐藏", RequestID: "race-direct"}); !errors.Is(err, notification.ErrReceiptConflict) {
		t.Fatalf("changed report association reused request ID: %v", err)
	}
	// An unassociated direct action has no report row to lock first, but its
	// absent receipt can still gap-lock a concurrent resolution's insert.
	second, err := repo.CreateReport(ctx, reporterID, CreateReportRequest{TargetType: TargetVideo, TargetID: videoID, ReasonCode: ReasonAbuse, RequestID: "race-create-2"})
	if err != nil {
		t.Fatal(err)
	}
	start = make(chan struct{})
	results = make(chan error, 2)
	go func() {
		<-start
		_, callErr := repo.ModerateTarget(ctx, adminID, TargetVideo, videoID, ActionHideVideo, ModerationActionRequest{Reason: "无关联隐藏", RequestID: "race-direct-2"})
		results <- callErr
	}()
	go func() {
		<-start
		_, callErr := repo.ResolveReport(ctx, adminID, second.ID, ReportDecisionRequest{Status: ReportResolved, ResolutionReason: "再次确认", Action: ActionHideVideo, Reason: "再次结案", RequestID: "race-close-2"})
		results <- callErr
	}()
	close(start)
	for i := 0; i < 2; i++ {
		select {
		case callErr := <-results:
			if callErr != nil {
				actionErrors = append(actionErrors, callErr)
			}
		case <-ctx.Done():
			t.Fatal("concurrent unassociated action timed out")
		}
	}
	if len(actionErrors) != 0 {
		t.Fatalf("concurrent unassociated action failed: %v", actionErrors)
	}
}

var _ = video.StatusReady
