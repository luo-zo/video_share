package session

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"video_share/internal/user"
)

var (
	ErrRefreshInvalid  = errors.New("invalid refresh token")
	ErrRefreshConflict = errors.New("refresh token conflict")
	ErrSessionReused   = errors.New("refresh token reused")
	ErrSessionExpired  = errors.New("session expired")
	ErrSessionRevoked  = errors.New("session revoked")
)

type Repository interface {
	CreateFamily(ctx context.Context, family *Family, refresh *RefreshToken) error
	Rotate(ctx context.Context, tokenHash []byte, now time.Time, replacement *RefreshToken, conflictWindow time.Duration) (*Rotation, error)
	RevokeFamily(ctx context.Context, familyID string, now time.Time) error
	RevokeByRefreshToken(ctx context.Context, tokenHash []byte, now time.Time) error
	RevokeUserFamilies(ctx context.Context, userID uint64, now time.Time) error
	ValidateFamily(ctx context.Context, userID uint64, familyID string, now time.Time) (bool, error)
}

type gormRepository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) Repository { return &gormRepository{db: db} }

func (r *gormRepository) CreateFamily(ctx context.Context, family *Family, refresh *RefreshToken) error {
	if family == nil || refresh == nil || family.ID == "" || refresh.ID == "" || refresh.FamilyID != family.ID {
		return ErrRefreshInvalid
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(family).Error; err != nil {
			return fmt.Errorf("create session family: %w", err)
		}
		if err := tx.Create(refresh).Error; err != nil {
			return fmt.Errorf("create refresh token: %w", err)
		}
		return nil
	})
}

func (r *gormRepository) Rotate(ctx context.Context, tokenHash []byte, now time.Time, replacement *RefreshToken, conflictWindow time.Duration) (*Rotation, error) {
	if len(tokenHash) != 32 || replacement == nil {
		return nil, ErrRefreshInvalid
	}
	var result Rotation
	var terminalErr error
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Resolve the family first, then lock it before inspecting rotation state.
		// This serializes two refresh requests across API instances.
		var token RefreshToken
		if err := tx.Where("token_hash = ?", tokenHash).Take(&token).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrRefreshInvalid
			}
			return fmt.Errorf("find refresh token: %w", err)
		}
		var family Family
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", token.FamilyID).Take(&family).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrRefreshInvalid
			}
			return fmt.Errorf("lock session family: %w", err)
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", token.ID).Take(&token).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrRefreshInvalid
			}
			return fmt.Errorf("lock refresh token: %w", err)
		}
		if family.RevokedAt != nil {
			return ErrSessionRevoked
		}
		// Account disablement is authoritative even if a refresh request was
		// already holding a valid token. The moderation transaction revokes all
		// families, while this check closes the race at the refresh boundary.
		var activeUser struct{ ID uint64 }
		if err := tx.Table("users").Select("id").Where("id = ? AND status = ?", family.UserID, user.StatusNormal).Take(&activeUser).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			if revokeErr := tx.Model(&Family{}).Where("id = ? AND revoked_at IS NULL", family.ID).Update("revoked_at", now).Error; revokeErr != nil {
				return fmt.Errorf("revoke disabled user family: %w", revokeErr)
			}
			terminalErr = ErrSessionRevoked
			return nil
		} else if err != nil {
			return fmt.Errorf("check session user status: %w", err)
		}
		if !now.Before(family.AbsoluteExpiresAt) {
			if err := tx.Model(&Family{}).Where("id = ? AND revoked_at IS NULL", family.ID).Update("revoked_at", now).Error; err != nil {
				return fmt.Errorf("revoke expired family: %w", err)
			}
			// Returning a sentinel error from the transaction callback would roll
			// back the revocation. Commit the state change, then report the
			// terminal error after the transaction has completed.
			terminalErr = ErrSessionExpired
			return nil
		}
		if token.RotatedAt != nil {
			if now.Sub(*token.RotatedAt) <= conflictWindow {
				return ErrRefreshConflict
			}
			if err := tx.Model(&Family{}).Where("id = ? AND revoked_at IS NULL", family.ID).Update("revoked_at", now).Error; err != nil {
				return fmt.Errorf("revoke reused family: %w", err)
			}
			terminalErr = ErrSessionReused
			return nil
		}
		if !now.Before(token.ExpiresAt) {
			return ErrSessionExpired
		}
		nowCopy := now
		updated := tx.Model(&RefreshToken{}).Where("id = ? AND rotated_at IS NULL", token.ID).Update("rotated_at", &nowCopy)
		if updated.Error != nil {
			return fmt.Errorf("rotate refresh token: %w", updated.Error)
		}
		if updated.RowsAffected != 1 {
			return ErrRefreshConflict
		}
		if replacement.FamilyID != "" && replacement.FamilyID != family.ID {
			return ErrRefreshInvalid
		}
		replacement.FamilyID = family.ID
		if replacement.ExpiresAt.After(family.AbsoluteExpiresAt) {
			replacement.ExpiresAt = family.AbsoluteExpiresAt
		}
		if err := tx.Create(replacement).Error; err != nil {
			return fmt.Errorf("create rotated refresh token: %w", err)
		}
		result = Rotation{UserID: family.UserID, FamilyID: family.ID}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if terminalErr != nil {
		return nil, terminalErr
	}
	return &result, nil
}

func (r *gormRepository) RevokeFamily(ctx context.Context, familyID string, now time.Time) error {
	if familyID == "" {
		return ErrSessionInvalid
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var family Family
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", familyID).Take(&family).Error
		if errors.Is(err, gorm.ErrRecordNotFound) || family.RevokedAt != nil {
			// Logout is intentionally idempotent, including after a concurrent
			// logout or password-change revocation.
			return nil
		}
		if err != nil {
			return fmt.Errorf("lock session family: %w", err)
		}
		if err := tx.Model(&Family{}).Where("id = ? AND revoked_at IS NULL", familyID).Update("revoked_at", now).Error; err != nil {
			return fmt.Errorf("revoke session family: %w", err)
		}
		return nil
	})
}

// RevokeByRefreshToken revokes the complete session family identified by a
// browser refresh token. It is deliberately idempotent: a missing, expired,
// already-rotated, or already-revoked token still leaves logout successful.
func (r *gormRepository) RevokeByRefreshToken(ctx context.Context, tokenHash []byte, now time.Time) error {
	if len(tokenHash) != 32 {
		return ErrSessionInvalid
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var token RefreshToken
		err := tx.Where("token_hash = ?", tokenHash).Take(&token).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("find refresh token for logout: %w", err)
		}
		var family Family
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", token.FamilyID).Take(&family).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("lock session family for logout: %w", err)
		}
		if family.RevokedAt != nil {
			return nil
		}
		if err := tx.Model(&Family{}).Where("id = ? AND revoked_at IS NULL", family.ID).Update("revoked_at", now).Error; err != nil {
			return fmt.Errorf("revoke session family by refresh token: %w", err)
		}
		return nil
	})
}

func (r *gormRepository) RevokeUserFamilies(ctx context.Context, userID uint64, now time.Time) error {
	if userID == 0 {
		return ErrSessionInvalid
	}
	result := r.db.WithContext(ctx).Model(&Family{}).Where("user_id = ? AND revoked_at IS NULL", userID).Update("revoked_at", now)
	if result.Error != nil {
		return fmt.Errorf("revoke user session families: %w", result.Error)
	}
	return nil
}

func (r *gormRepository) ValidateFamily(ctx context.Context, userID uint64, familyID string, now time.Time) (bool, error) {
	if userID == 0 || familyID == "" {
		return false, nil
	}
	var count int64
	if err := r.db.WithContext(ctx).Model(&Family{}).Where("id = ? AND user_id = ? AND revoked_at IS NULL AND absolute_expires_at > ?", familyID, userID, now).Count(&count).Error; err != nil {
		return false, fmt.Errorf("validate session family: %w", err)
	}
	return count == 1, nil
}
