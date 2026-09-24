package session

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"video_share/internal/token"
)

var ErrSessionInvalid = errors.New("invalid session")

type Options struct {
	Now            func() time.Time
	FamilyTTL      time.Duration
	RefreshTTL     time.Duration
	ConflictWindow time.Duration
}

type Issued struct {
	AccessToken  string
	ExpiresIn    time.Duration
	RefreshToken string
	FamilyID     string
}

type Service struct {
	repo           Repository
	tokens         *token.Manager
	now            func() time.Time
	familyTTL      time.Duration
	refreshTTL     time.Duration
	conflictWindow time.Duration
}

func NewService(repo Repository, tokens *token.Manager, options Options) *Service {
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.FamilyTTL <= 0 {
		options.FamilyTTL = 30 * 24 * time.Hour
	}
	if options.RefreshTTL <= 0 {
		options.RefreshTTL = 30 * 24 * time.Hour
	}
	if options.ConflictWindow <= 0 {
		options.ConflictWindow = 10 * time.Second
	}
	return &Service{repo: repo, tokens: tokens, now: options.Now, familyTTL: options.FamilyTTL, refreshTTL: options.RefreshTTL, conflictWindow: options.ConflictWindow}
}

func (s *Service) Issue(ctx context.Context, userID uint64) (*Issued, error) {
	if s == nil || s.repo == nil || s.tokens == nil || userID == 0 {
		return nil, ErrSessionInvalid
	}
	now := s.now().UTC()
	familyID := uuid.NewString()
	refresh, hash, err := newRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}
	familyExpires := now.Add(s.familyTTL)
	refreshExpires := now.Add(s.refreshTTL)
	if refreshExpires.After(familyExpires) {
		refreshExpires = familyExpires
	}
	family := &Family{ID: familyID, UserID: userID, CreatedAt: now, AbsoluteExpiresAt: familyExpires}
	stored := &RefreshToken{ID: uuid.NewString(), FamilyID: familyID, TokenHash: hash, CreatedAt: now, ExpiresAt: refreshExpires}
	if err := s.repo.CreateFamily(ctx, family, stored); err != nil {
		return nil, err
	}
	issued, err := s.issueAccess(userID, familyID, refresh)
	if err != nil {
		return nil, err
	}
	return issued, nil
}

func (s *Service) Refresh(ctx context.Context, presented string) (*Issued, error) {
	if s == nil || s.repo == nil || s.tokens == nil || presented == "" {
		return nil, ErrRefreshInvalid
	}
	hash := refreshHash(presented)
	replacement, replacementHash, err := newRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}
	now := s.now().UTC()
	// The repository performs lock-family, rotation, and insertion atomically.
	// Plaintext replacement is returned only after that transaction succeeds.
	rotated := &RefreshToken{ID: uuid.NewString(), TokenHash: replacementHash, CreatedAt: now, ExpiresAt: now.Add(s.refreshTTL)}
	result, err := s.repo.Rotate(ctx, hash, now, rotated, s.conflictWindow)
	if err != nil {
		return nil, err
	}
	issued, err := s.issueAccess(result.UserID, result.FamilyID, replacement)
	if err != nil {
		return nil, err
	}
	return issued, nil
}

func (s *Service) Logout(ctx context.Context, familyID string) error {
	if s == nil || s.repo == nil || familyID == "" {
		return ErrSessionInvalid
	}
	return s.repo.RevokeFamily(ctx, familyID, s.now().UTC())
}

func (s *Service) LogoutByRefreshToken(ctx context.Context, presented string) error {
	if s == nil || s.repo == nil || presented == "" {
		return ErrSessionInvalid
	}
	return s.repo.RevokeByRefreshToken(ctx, refreshHash(presented), s.now().UTC())
}

func (s *Service) RevokeUser(ctx context.Context, userID uint64) error {
	if s == nil || s.repo == nil || userID == 0 {
		return ErrSessionInvalid
	}
	return s.repo.RevokeUserFamilies(ctx, userID, s.now().UTC())
}

func (s *Service) ValidateAccess(ctx context.Context, userID uint64, familyID string) (bool, error) {
	if s == nil || s.repo == nil || userID == 0 || familyID == "" {
		return false, nil
	}
	return s.repo.ValidateFamily(ctx, userID, familyID, s.now().UTC())
}

func (s *Service) issueAccess(userID uint64, familyID, refresh string) (*Issued, error) {
	access, ttl, err := s.tokens.GenerateWithSession(userID, familyID)
	if err != nil {
		return nil, err
	}
	return &Issued{AccessToken: access, ExpiresIn: ttl, RefreshToken: refresh, FamilyID: familyID}, nil
}

func newRefreshToken() (string, []byte, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, err
	}
	encoded := base64.RawURLEncoding.EncodeToString(raw)
	return encoded, refreshHash(encoded), nil
}

func refreshHash(value string) []byte {
	digest := sha256.Sum256([]byte(value))
	return digest[:]
}
