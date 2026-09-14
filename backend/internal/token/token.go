package token

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var ErrInvalidToken = errors.New("invalid token")

// Claims 携带已认证的用户 ID 以及标准的注册声明。
type Claims struct {
	UserID uint64 `json:"uid"`
	jwt.RegisteredClaims
}

// Manager 使用固定的签发者和 TTL 签发并验证 HS256 JWT。
type Manager struct {
	secret []byte
	issuer string
	ttl    time.Duration
	now    func() time.Time
}

func NewManager(secret, issuer string, ttl time.Duration) *Manager {
	return &Manager{
		secret: []byte(secret),
		issuer: issuer,
		ttl:    ttl,
		now:    time.Now,
	}
}

// Generate 为给定用户 ID 签发令牌，并连同其有效时长一起返回。
func (m *Manager) Generate(userID uint64) (string, time.Duration, error) {
	now := m.now()
	claims := Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   strconv.FormatUint(userID, 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := t.SignedString(m.secret)
	if err != nil {
		return "", 0, fmt.Errorf("sign token: %w", err)
	}
	return signed, m.ttl, nil
}

// Parse 验证令牌的签名、签名算法、签发者和过期时间。
func (m *Manager) Parse(tokenString string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return m.secret, nil
	},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(m.issuer),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	if !token.Valid || claims.UserID == 0 {
		return nil, ErrInvalidToken
	}
	return claims, nil
}
