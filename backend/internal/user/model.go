package user

import "time"

// 用户账号的状态值。
type Status uint8

const (
	StatusNormal   Status = 1
	StatusDisabled Status = 2
)

// User 是持久化的用户实体。PasswordHash 绝不能序列化到响应或日志中。
type User struct {
	ID           uint64    `gorm:"primaryKey;autoIncrement"`
	Username     string    `gorm:"type:varchar(32);uniqueIndex;not null"`
	PasswordHash string    `gorm:"type:varchar(255);not null"`
	Nickname     string    `gorm:"type:varchar(64);not null"`
	Bio          string    `gorm:"type:varchar(200);not null;default:''"`
	Role         string    `gorm:"type:varchar(32);not null;default:user"`
	Status       Status    `gorm:"type:tinyint;not null;default:1"`
	CreatedAt    time.Time `gorm:"not null"`
	UpdatedAt    time.Time `gorm:"not null"`
}

func (User) TableName() string { return "users" }
