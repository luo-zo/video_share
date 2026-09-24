package moderation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"video_share/internal/notification"
	"video_share/internal/user"
	"video_share/internal/video"
)

var (
	ErrInvalidReport       = errors.New("invalid report")
	ErrActiveReport        = errors.New("active report already exists")
	ErrReportNotFound      = errors.New("report not found")
	ErrReportStateConflict = errors.New("report state conflict")
	ErrTargetNotFound      = errors.New("moderation target not found")
	ErrTargetDeleted       = errors.New("moderation target deleted")
	ErrSelfModeration      = errors.New("cannot disable or moderate yourself")
	ErrLastAdmin           = errors.New("cannot disable the last administrator")
	ErrActionConflict      = errors.New("moderation request conflict")
	ErrReasonRequired      = errors.New("moderation reason required")
	ErrAdminRequired       = errors.New("administrator required")
)

type Repository interface {
	CreateReport(ctx context.Context, reporterID uint64, input CreateReportRequest) (*Report, error)
	ListReports(ctx context.Context, reporterID uint64, admin bool, page, pageSize int) ([]Report, int64, error)
	AssignReport(ctx context.Context, adminID, reportID uint64, requestID string) (*Report, error)
	ResolveReport(ctx context.Context, adminID, reportID uint64, input ReportDecisionRequest) (*Report, error)
	ModerateTarget(ctx context.Context, adminID uint64, targetType string, targetID uint64, action string, input ModerationActionRequest) (*ModerationAction, error)
	ListActions(ctx context.Context, page, pageSize int) ([]ModerationAction, int64, error)
}

type gormRepository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) Repository { return &gormRepository{db: db} }

func (r *gormRepository) transaction(ctx context.Context, fn func(*gorm.DB) error) error {
	return retryDeadlock(ctx, func() error { return r.db.WithContext(ctx).Transaction(fn) })
}

// InnoDB rolls the victim transaction back on 1213. Retrying the complete
// transaction is safe here: audit rows, receipts, counters and notifications
// are all written through the same tx, and no external side effect occurs in
// the callback. A bounded retry also handles gap locks on absent receipts.
func retryDeadlock(ctx context.Context, run func() error) error {
	const maxRetries = 4
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := run()
		var mysqlErr *mysql.MySQLError
		if !errors.As(err, &mysqlErr) || mysqlErr.Number != 1213 || attempt >= maxRetries {
			return err
		}
		wait := time.NewTimer(time.Duration(1<<attempt) * 10 * time.Millisecond)
		select {
		case <-ctx.Done():
			wait.Stop()
			return ctx.Err()
		case <-wait.C:
		}
	}
}

func (r *gormRepository) CreateReport(ctx context.Context, reporterID uint64, input CreateReportRequest) (*Report, error) {
	if err := validateReportInput(input); err != nil {
		return nil, err
	}
	payload, _ := json.Marshal(struct {
		TargetType         string `json:"target_type"`
		TargetID           uint64 `json:"target_id"`
		ReasonCode, Detail string
	}{input.TargetType, input.TargetID, input.ReasonCode, input.Detail})
	hash := notification.RequestHash(string(payload))
	var report Report
	err := r.transaction(ctx, func(tx *gorm.DB) error {
		if receipt, err := notification.ExistingReceipt(tx, reporterID, "report.create", input.RequestID, hash); err != nil {
			return err
		} else if receipt != nil && receipt.ResourceID != nil {
			return tx.Where("id = ?", *receipt.ResourceID).Take(&report).Error
		}
		activeKey := fmt.Sprintf("%d:%s:%d", reporterID, input.TargetType, input.TargetID)
		var existing Report
		// Resolve holds the primary record before clearing the unique active_key.
		// A locking read via that secondary index takes the opposite order and
		// deadlocks with Resolve. Find its ID without a lock, then lock by PK and
		// recheck the current value after any concurrent resolution commits.
		if err := tx.Model(&Report{}).Select("id").Where("active_key = ?", activeKey).Take(&existing).Error; err == nil {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", existing.ID).Take(&existing).Error; err != nil {
				return fmt.Errorf("lock active report: %w", err)
			}
			if existing.ActiveKey != nil && *existing.ActiveKey == activeKey {
				return ErrActiveReport
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("check active report: %w", err)
		}
		// The unique active_key constraint remains the arbiter if two creators
		// both observe no active row. Target validation follows report-row locks.
		if err := ensureTarget(tx, input.TargetType, input.TargetID); err != nil {
			return err
		}
		report = Report{ReporterID: reporterID, TargetType: input.TargetType, TargetID: input.TargetID, ReasonCode: input.ReasonCode, Detail: input.Detail, Status: ReportOpen, ActiveKey: &activeKey, CreatedAt: time.Now().UTC()}
		if err := tx.Create(&report).Error; err != nil {
			return fmt.Errorf("create report: %w", err)
		}
		if err := notification.SaveReceipt(tx, reporterID, "report.create", input.RequestID, hash, &report.ID); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, mapDBError(err)
	}
	return &report, nil
}

func (r *gormRepository) ListReports(ctx context.Context, reporterID uint64, admin bool, page, pageSize int) ([]Report, int64, error) {
	query := r.db.WithContext(ctx).Model(&Report{})
	if !admin {
		query = query.Where("reporter_id = ?", reporterID)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count reports: %w", err)
	}
	rows := make([]Report, 0, pageSize)
	if err := query.Order("created_at DESC, id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("list reports: %w", err)
	}
	return rows, total, nil
}

func (r *gormRepository) AssignReport(ctx context.Context, adminID, reportID uint64, requestID string) (*Report, error) {
	if err := notification.ValidateRequestID(requestID); err != nil {
		return nil, err
	}
	hash := notification.RequestHash(fmt.Sprintf("report:%d:assign", reportID))
	var report Report
	err := r.transaction(ctx, func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", reportID).Take(&report).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrReportNotFound
		} else if err != nil {
			return err
		}
		// Lock the report before the receipt. All assigners therefore use the
		// same lock order and a simultaneous pair resolves to one winner plus a
		// deterministic state conflict instead of a deadlock.
		if receipt, err := notification.ExistingReceipt(tx, adminID, "report.assign", requestID, hash); err != nil {
			return err
		} else if receipt != nil {
			return nil
		}
		if report.Status != ReportOpen {
			return ErrReportStateConflict
		}
		if err := tx.Model(&Report{}).Where("id = ? AND status = ?", reportID, ReportOpen).Updates(map[string]any{"status": ReportInvestigating, "assigned_to": adminID}).Error; err != nil {
			return fmt.Errorf("assign report: %w", err)
		}
		action := ModerationAction{ActorID: adminID, ReportID: &reportID, TargetType: report.TargetType, TargetID: report.TargetID, Action: ActionAssignReport, Reason: "接单", BeforeState: ReportOpen, AfterState: ReportInvestigating, RequestID: requestID, CreatedAt: time.Now().UTC()}
		if err := tx.Create(&action).Error; err != nil {
			return fmt.Errorf("audit assign: %w", err)
		}
		if err := notification.SaveReceipt(tx, adminID, "report.assign", requestID, hash, &reportID); err != nil {
			return err
		}
		return tx.Where("id = ?", reportID).Take(&report).Error
	})
	if err != nil {
		return nil, mapDBError(err)
	}
	return &report, nil
}

func (r *gormRepository) ResolveReport(ctx context.Context, adminID, reportID uint64, input ReportDecisionRequest) (*Report, error) {
	if err := validateDecision(input); err != nil {
		return nil, err
	}
	payload, _ := json.Marshal(input)
	hash := notification.RequestHash(fmt.Sprintf("%d:%s", reportID, payload))
	var report Report
	err := r.transaction(ctx, func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", reportID).Take(&report).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrReportNotFound
		} else if err != nil {
			return err
		}
		// Lock the report before looking up a potentially absent receipt. A
		// missing receipt's InnoDB gap lock must not be held while waiting for
		// the report row (or a peer can hold the row while inserting a receipt).
		if receipt, err := notification.ExistingReceipt(tx, adminID, "report.resolve", input.RequestID, hash); err != nil {
			return err
		} else if receipt != nil && receipt.ResourceID != nil {
			return tx.Where("id = ?", *receipt.ResourceID).Take(&report).Error
		}
		if report.Status != ReportOpen && report.Status != ReportInvestigating {
			return ErrReportStateConflict
		}
		var actionID *uint64
		if input.Status == ReportResolved {
			if err := validateActionForTarget(input.Action, report.TargetType); err != nil {
				return err
			}
			action, err := r.applyTargetTx(tx, adminID, report.TargetType, report.TargetID, input.Action, ModerationActionRequest{Reason: input.Reason, ReportID: reportID, RequestID: input.RequestID})
			if err != nil {
				return err
			}
			actionID = &action.ID
		}
		now := time.Now().UTC()
		updates := map[string]any{"status": input.Status, "resolution_reason": input.ResolutionReason, "action_id": actionID, "closed_at": now, "active_key": nil}
		if err := tx.Model(&Report{}).Where("id = ? AND status IN ?", reportID, []string{ReportOpen, ReportInvestigating}).Updates(updates).Error; err != nil {
			return fmt.Errorf("resolve report: %w", err)
		}
		actor := adminID
		typeName := "report.rejected"
		if input.Status == ReportResolved {
			typeName = "report.resolved"
		}
		if err := notification.Emit(tx, notification.Event{RecipientID: report.ReporterID, ActorID: &actor, EventKey: fmt.Sprintf("report:%d:%s", reportID, input.Status), Type: typeName, ReportID: &reportID}); err != nil {
			return err
		}
		if err := notification.SaveReceipt(tx, adminID, "report.resolve", input.RequestID, hash, &reportID); err != nil {
			return err
		}
		return tx.Where("id = ?", reportID).Take(&report).Error
	})
	if err != nil {
		return nil, mapDBError(err)
	}
	return &report, nil
}

func (r *gormRepository) ModerateTarget(ctx context.Context, adminID uint64, targetType string, targetID uint64, action string, input ModerationActionRequest) (*ModerationAction, error) {
	if err := validateActionInput(targetType, targetID, action, input); err != nil {
		return nil, err
	}
	payload, _ := json.Marshal(struct {
		TargetType     string
		TargetID       uint64
		Action, Reason string
		ReportID       uint64
	}{targetType, targetID, action, input.Reason, input.ReportID})
	hash := notification.RequestHash(string(payload))
	var result *ModerationAction
	err := r.transaction(ctx, func(tx *gorm.DB) error {
		if input.ReportID != 0 {
			// ResolveReport already locks the report before the target. Direct
			// actions carrying a report association must use the same order;
			// otherwise the action's FK check takes the report lock *after*
			// locking the target and deadlocks with a concurrent resolution.
			var linked Report
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", input.ReportID).Take(&linked).Error; errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrReportNotFound
			} else if err != nil {
				return fmt.Errorf("lock associated report: %w", err)
			}
			if linked.TargetType != targetType || linked.TargetID != targetID {
				return ErrInvalidReport
			}
		}
		if receipt, err := notification.ExistingReceipt(tx, adminID, "moderation."+action, input.RequestID, hash); err != nil {
			return err
		} else if receipt != nil && receipt.ResourceID != nil {
			var row ModerationAction
			if err := tx.Where("id = ?", *receipt.ResourceID).Take(&row).Error; err != nil {
				return err
			}
			result = &row
			return nil
		}
		actionRow, err := r.applyTargetTx(tx, adminID, targetType, targetID, action, input)
		if err != nil {
			return err
		}
		result = actionRow
		return notification.SaveReceipt(tx, adminID, "moderation."+action, input.RequestID, hash, &actionRow.ID)
	})
	if err != nil {
		return nil, mapDBError(err)
	}
	return result, nil
}

func (r *gormRepository) applyTargetTx(tx *gorm.DB, adminID uint64, targetType string, targetID uint64, action string, input ModerationActionRequest) (*ModerationAction, error) {
	if err := validateActionForTarget(action, targetType); err != nil {
		return nil, err
	}
	// User governance transitions use one deterministic lock order. Locking
	// every administrator row before the target row prevents two concurrent
	// disable requests from both observing themselves as the last admin.
	var lockedAdmins []user.User
	if targetType == TargetUser && (action == ActionDisableUser || action == ActionEnableUser) {
		var err error
		lockedAdmins, err = lockAdministratorSet(tx)
		if err != nil {
			return nil, err
		}
	}
	before, ownerID, err := lockTarget(tx, targetType, targetID)
	if err != nil {
		return nil, err
	}
	if (targetType == TargetUser && targetID == adminID) && action == ActionDisableUser {
		return nil, ErrSelfModeration
	}
	if (action == ActionRestoreVideo || action == ActionRestoreComment) && before == StateVisible {
		return nil, ErrActionConflict
	}
	if action == ActionRestoreVideo || action == ActionRestoreComment { /* deleted targets were rejected by lockTarget */
	}
	after := StateHidden
	if action == ActionRestoreVideo || action == ActionRestoreComment {
		after = StateVisible
	}
	if action == ActionEnableUser {
		after = StateNormal
	}
	if action == ActionDisableUser {
		admins := 0
		targetIsAdmin := false
		for _, admin := range lockedAdmins {
			if admin.Status == user.StatusNormal {
				admins++
			}
			if admin.ID == targetID {
				targetIsAdmin = true
			}
		}
		if before == StateNormal && targetIsAdmin && admins <= 1 {
			return nil, ErrLastAdmin
		}
		after = StateDisabled
	}
	// lockTarget already proved the row exists. A second administrator can
	// reach the same state first; that is an auditable no-op, not a missing
	// target or a reason to strand the report in its old state.
	if before != after {
		if err := writeTargetState(tx, targetType, targetID, action, after); err != nil {
			return nil, err
		}
	}
	if targetType == TargetUser && action == ActionDisableUser {
		if err := tx.Table("session_families").Where("user_id = ? AND revoked_at IS NULL", targetID).Update("revoked_at", time.Now().UTC()).Error; err != nil {
			return nil, fmt.Errorf("revoke disabled user sessions: %w", err)
		}
	}
	if targetType == TargetComment && before != after {
		var comment struct{ VideoID uint64 }
		if err := tx.Table("comments").Select("video_id").Where("id = ?", targetID).Take(&comment).Error; err != nil {
			return nil, fmt.Errorf("load comment counter target: %w", err)
		}
		delta := -1
		if after == StateVisible {
			delta = 1
		}
		if err := tx.Exec("UPDATE video_stats SET comment_count = GREATEST(0, comment_count + ?), updated_at = UTC_TIMESTAMP(3) WHERE video_id = ?", delta, comment.VideoID).Error; err != nil {
			return nil, fmt.Errorf("adjust comment counter: %w", err)
		}
	}
	actionRow := &ModerationAction{ActorID: adminID, ReportID: optionalReportID(input.ReportID), TargetType: targetType, TargetID: targetID, Action: action, Reason: strings.TrimSpace(input.Reason), BeforeState: before, AfterState: after, RequestID: input.RequestID, CreatedAt: time.Now().UTC()}
	if err := tx.Create(actionRow).Error; err != nil {
		return nil, fmt.Errorf("create moderation action: %w", err)
	}
	if ownerID != 0 && before != after {
		actor := adminID
		if err := notification.Emit(tx, notification.Event{RecipientID: ownerID, ActorID: &actor, EventKey: fmt.Sprintf("moderation:%d:%d", actionRow.ID, ownerID), Type: "moderation." + action, VideoID: optionalTargetVideo(targetType, targetID), CommentID: optionalTargetComment(targetType, targetID)}); err != nil {
			return nil, err
		}
	}
	return actionRow, nil
}

func lockAdministratorSet(tx *gorm.DB) ([]user.User, error) {
	var admins []user.User
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("role = ?", "admin").Order("id ASC").Find(&admins).Error; err != nil {
		return nil, fmt.Errorf("lock administrator set: %w", err)
	}
	return admins, nil
}

func (r *gormRepository) ListActions(ctx context.Context, page, pageSize int) ([]ModerationAction, int64, error) {
	var total int64
	if err := r.db.WithContext(ctx).Model(&ModerationAction{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := make([]ModerationAction, 0, pageSize)
	if err := r.db.WithContext(ctx).Order("created_at DESC, id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func lockTarget(tx *gorm.DB, targetType string, targetID uint64) (string, uint64, error) {
	switch targetType {
	case TargetVideo:
		var row struct {
			ID, UserID       uint64
			Status           video.Status
			ModerationStatus string
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Table("videos").Where("id = ?", targetID).Take(&row).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return "", 0, ErrTargetNotFound
		} else if err != nil {
			return "", 0, err
		}
		if row.Status == video.StatusDeleted {
			return "", 0, ErrTargetDeleted
		}
		if row.ModerationStatus == "" {
			row.ModerationStatus = StateVisible
		}
		return row.ModerationStatus, row.UserID, nil
	case TargetComment:
		var row struct {
			ID, UserID       uint64
			DeletedAt        *time.Time
			ModerationStatus string
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Table("comments").Where("id = ?", targetID).Take(&row).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return "", 0, ErrTargetNotFound
		} else if err != nil {
			return "", 0, err
		}
		if row.DeletedAt != nil {
			return "", 0, ErrTargetDeleted
		}
		if row.ModerationStatus == "" {
			row.ModerationStatus = StateVisible
		}
		return row.ModerationStatus, row.UserID, nil
	case TargetUser:
		var row user.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", targetID).Take(&row).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return "", 0, ErrTargetNotFound
		} else if err != nil {
			return "", 0, err
		}
		state := StateNormal
		if row.Status == user.StatusDisabled {
			state = StateDisabled
		}
		return state, row.ID, nil
	default:
		return "", 0, ErrInvalidReport
	}
}

func writeTargetState(tx *gorm.DB, targetType string, targetID uint64, action, after string) error {
	var result *gorm.DB
	switch targetType {
	case TargetVideo, TargetComment:
		result = tx.Table(map[string]string{TargetVideo: "videos", TargetComment: "comments"}[targetType]).Where("id = ?", targetID).Update("moderation_status", after)
	case TargetUser:
		status := user.StatusNormal
		if after == StateDisabled {
			status = user.StatusDisabled
		}
		result = tx.Model(&user.User{}).Where("id = ?", targetID).Update("status", status)
	default:
		return ErrInvalidReport
	}
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrTargetNotFound
	}
	return nil
}

func ensureTarget(tx *gorm.DB, targetType string, targetID uint64) error {
	_, _, err := lockTarget(tx, targetType, targetID)
	return err
}

func validateReportInput(input CreateReportRequest) error {
	if input.TargetID == 0 || !validTarget(input.TargetType) || !validReason(input.ReasonCode) || utf8.RuneCountInString(strings.TrimSpace(input.Detail)) > 500 {
		return ErrInvalidReport
	}
	if err := notification.ValidateRequestID(input.RequestID); err != nil {
		return err
	}
	return nil
}

func validateDecision(input ReportDecisionRequest) error {
	if input.Status != ReportResolved && input.Status != ReportRejected {
		return ErrInvalidReport
	}
	if strings.TrimSpace(input.ResolutionReason) == "" || utf8.RuneCountInString(input.ResolutionReason) > 500 {
		return ErrReasonRequired
	}
	if input.Status == ReportResolved && strings.TrimSpace(input.Reason) == "" {
		return ErrReasonRequired
	}
	return notification.ValidateRequestID(input.RequestID)
}

func validateActionInput(targetType string, targetID uint64, action string, input ModerationActionRequest) error {
	if targetID == 0 || strings.TrimSpace(input.Reason) == "" || utf8.RuneCountInString(input.Reason) > 500 {
		return ErrReasonRequired
	}
	if err := validateActionForTarget(action, targetType); err != nil {
		return err
	}
	return notification.ValidateRequestID(input.RequestID)
}

func validateActionForTarget(action, targetType string) error {
	ok := (targetType == TargetVideo && (action == ActionHideVideo || action == ActionRestoreVideo)) ||
		(targetType == TargetComment && (action == ActionHideComment || action == ActionRestoreComment)) ||
		(targetType == TargetUser && (action == ActionDisableUser || action == ActionEnableUser))
	if !ok {
		return ErrInvalidReport
	}
	return nil
}

func validTarget(value string) bool {
	return value == TargetVideo || value == TargetComment || value == TargetUser
}
func validReason(value string) bool {
	return value == ReasonSpam || value == ReasonAbuse || value == ReasonCopyright || value == ReasonIllegal || value == ReasonOther
}
func optionalReportID(id uint64) *uint64 {
	if id == 0 {
		return nil
	}
	return &id
}
func optionalTargetVideo(targetType string, id uint64) *uint64 {
	if targetType != TargetVideo {
		return nil
	}
	return &id
}
func optionalTargetComment(targetType string, id uint64) *uint64 {
	if targetType != TargetComment {
		return nil
	}
	return &id
}

func mapDBError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrTargetNotFound
	}
	if user.IsDuplicateKey(err) {
		return ErrActionConflict
	}
	return err
}
