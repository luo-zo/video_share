package session

import "time"

// Family is the durable revocation boundary for one login session. Refresh
// tokens and access JWTs carry its ID, while MySQL remains the authority.
type Family struct {
	ID                string     `gorm:"type:char(36);primaryKey"`
	UserID            uint64     `gorm:"not null;index"`
	CreatedAt         time.Time  `gorm:"not null"`
	AbsoluteExpiresAt time.Time  `gorm:"not null;index"`
	RevokedAt         *time.Time `gorm:"index"`
}

func (Family) TableName() string { return "session_families" }

// RefreshToken stores only the SHA-256 digest of the opaque browser token.
type RefreshToken struct {
	ID        string     `gorm:"type:char(36);primaryKey"`
	FamilyID  string     `gorm:"type:char(36);not null;index"`
	TokenHash []byte     `gorm:"type:binary(32);not null;uniqueIndex"`
	CreatedAt time.Time  `gorm:"not null"`
	ExpiresAt time.Time  `gorm:"not null;index"`
	RotatedAt *time.Time `gorm:"index"`
}

func (RefreshToken) TableName() string { return "refresh_tokens" }

type Rotation struct {
	UserID   uint64
	FamilyID string
}
