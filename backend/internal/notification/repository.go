package notification

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrNotFound          = errors.New("notification not found")
	ErrReceiptConflict   = errors.New("idempotency request conflict")
	ErrInvalidRequestID  = errors.New("invalid request id")
	ErrPaginationInvalid = errors.New("invalid pagination")
)

type Event struct {
	RecipientID uint64
	ActorID     *uint64
	EventKey    string
	Type        string
	VideoID     *uint64
	CommentID   *uint64
	ReportID    *uint64
}

// Emit is used by comment/follow transactions. The unique recipient/event key
// makes retries harmless and deliberately does not copy user-generated text.
func Emit(tx *gorm.DB, event Event) error {
	if event.RecipientID == 0 || event.EventKey == "" || event.Type == "" {
		return nil
	}
	if event.ActorID != nil && *event.ActorID == event.RecipientID {
		return nil
	}
	row := Notification{
		RecipientID: event.RecipientID,
		ActorID:     event.ActorID,
		EventKey:    event.EventKey,
		Type:        event.Type,
		VideoID:     event.VideoID,
		CommentID:   event.CommentID,
		ReportID:    event.ReportID,
		CreatedAt:   time.Now().UTC(),
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		return fmt.Errorf("emit notification: %w", err)
	}
	return nil
}

// RequestHash is stable across retries and covers the action payload rather
// than trusting a client-provided hash.
func RequestHash(payload string) string {
	digest := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(digest[:])
}

func ValidateRequestID(value string) error {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 {
		return ErrInvalidRequestID
	}
	return nil
}

// ExistingReceipt returns a previous result while locking it for the current
// transaction. A different body under the same actor/action/request ID is a
// conflict, never a second business operation.
func ExistingReceipt(tx *gorm.DB, actorID uint64, action, requestID, requestHash string) (*OperationReceipt, error) {
	if strings.TrimSpace(requestID) == "" {
		return nil, nil
	}
	if err := ValidateRequestID(requestID); err != nil {
		return nil, err
	}
	var receipt OperationReceipt
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(
		"actor_id = ? AND action = ? AND request_id = ?", actorID, action, requestID,
	).Take(&receipt).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load operation receipt: %w", err)
	}
	if receipt.RequestHash != requestHash {
		return nil, ErrReceiptConflict
	}
	return &receipt, nil
}

func SaveReceipt(tx *gorm.DB, actorID uint64, action, requestID, requestHash string, resourceID *uint64) error {
	if strings.TrimSpace(requestID) == "" {
		return nil
	}
	if err := ValidateRequestID(requestID); err != nil {
		return err
	}
	receipt := OperationReceipt{ActorID: actorID, Action: action, RequestID: requestID, RequestHash: requestHash, ResourceID: resourceID, CreatedAt: time.Now().UTC()}
	if err := tx.Create(&receipt).Error; err != nil {
		return fmt.Errorf("save operation receipt: %w", err)
	}
	return nil
}

type Repository interface {
	List(ctx context.Context, recipientID uint64, page, pageSize int) ([]Notification, int64, int64, error)
	MarkRead(ctx context.Context, recipientID, id uint64) error
	MarkAllRead(ctx context.Context, recipientID uint64) (int64, error)
}

type gormRepository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) Repository { return &gormRepository{db: db} }

func (r *gormRepository) List(ctx context.Context, recipientID uint64, page, pageSize int) ([]Notification, int64, int64, error) {
	var total, unread int64
	base := r.db.WithContext(ctx).Model(&Notification{}).Where("recipient_id = ?", recipientID)
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, 0, fmt.Errorf("count notifications: %w", err)
	}
	if err := r.db.WithContext(ctx).Model(&Notification{}).Where("recipient_id = ? AND read_at IS NULL", recipientID).Count(&unread).Error; err != nil {
		return nil, 0, 0, fmt.Errorf("count unread notifications: %w", err)
	}
	var rows []Notification
	if err := base.Order("created_at DESC, id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, 0, 0, fmt.Errorf("list notifications: %w", err)
	}
	return rows, total, unread, nil
}

func (r *gormRepository) MarkRead(ctx context.Context, recipientID, id uint64) error {
	result := r.db.WithContext(ctx).Model(&Notification{}).Where("id = ? AND recipient_id = ?", id, recipientID).Update("read_at", gorm.Expr("COALESCE(read_at, UTC_TIMESTAMP(3))"))
	if result.Error != nil {
		return fmt.Errorf("mark notification read: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormRepository) MarkAllRead(ctx context.Context, recipientID uint64) (int64, error) {
	result := r.db.WithContext(ctx).Model(&Notification{}).Where("recipient_id = ? AND read_at IS NULL", recipientID).Update("read_at", gorm.Expr("UTC_TIMESTAMP(3)"))
	if result.Error != nil {
		return 0, fmt.Errorf("mark all notifications read: %w", result.Error)
	}
	return result.RowsAffected, nil
}
