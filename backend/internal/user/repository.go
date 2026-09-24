package user

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

var ErrNotFound = errors.New("user not found")

// Repository 抽象用户持久化，使业务逻辑可以用测试替身进行测试。
type Repository interface {
	Create(ctx context.Context, u *User) error
	FindByUsername(ctx context.Context, username string) (*User, error)
	FindByID(ctx context.Context, id uint64) (*User, error)
	UpdateProfile(ctx context.Context, id uint64, nickname, bio string) (*User, error)
	ChangePasswordAndRevokeSessions(ctx context.Context, id uint64, passwordHash string, revokedAt time.Time) error
}

type gormRepository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

func (r *gormRepository) Create(ctx context.Context, u *User) error {
	if err := r.db.WithContext(ctx).Create(u).Error; err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

func (r *gormRepository) FindByUsername(ctx context.Context, username string) (*User, error) {
	var u User
	err := r.db.WithContext(ctx).Where("username = ?", username).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find user by username: %w", err)
	}
	return &u, nil
}

func (r *gormRepository) FindByID(ctx context.Context, id uint64) (*User, error) {
	var u User
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find user by id: %w", err)
	}
	return &u, nil
}

func (r *gormRepository) UpdateProfile(ctx context.Context, id uint64, nickname, bio string) (*User, error) {
	result := r.db.WithContext(ctx).Model(&User{}).Where("id = ? AND status = ?", id, StatusNormal).Updates(map[string]any{
		"nickname": nickname,
		"bio":      bio,
	})
	if result.Error != nil {
		return nil, fmt.Errorf("update profile: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, ErrNotFound
	}
	return r.FindByID(ctx, id)
}

// ChangePasswordAndRevokeSessions is one transaction so a successful password
// change cannot leave an old refresh family usable after the response returns.
func (r *gormRepository) ChangePasswordAndRevokeSessions(ctx context.Context, id uint64, passwordHash string, revokedAt time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&User{}).Where("id = ? AND status = ?", id, StatusNormal).Update("password_hash", passwordHash)
		if result.Error != nil {
			return fmt.Errorf("update password: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}
		if err := tx.Table("session_families").Where("user_id = ? AND revoked_at IS NULL", id).Update("revoked_at", revokedAt).Error; err != nil {
			return fmt.Errorf("revoke sessions after password change: %w", err)
		}
		return nil
	})
}

// IsDuplicateKey 报告 err 是否为 MySQL 唯一键冲突错误（1062）。
func IsDuplicateKey(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == 1062
}
