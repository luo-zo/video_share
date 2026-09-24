package session

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"video_share/internal/token"
)

type fakeRepository struct {
	createFamilyFn       func(context.Context, *Family, *RefreshToken) error
	rotateFn             func(context.Context, []byte, time.Time, *RefreshToken, time.Duration) (*Rotation, error)
	revokeFamilyFn       func(context.Context, string, time.Time) error
	revokeByRefreshFn    func(context.Context, []byte, time.Time) error
	revokeUserFamiliesFn func(context.Context, uint64, time.Time) error
	validateFamilyFn     func(context.Context, uint64, string, time.Time) (bool, error)
}

func (f *fakeRepository) CreateFamily(ctx context.Context, family *Family, refresh *RefreshToken) error {
	return f.createFamilyFn(ctx, family, refresh)
}
func (f *fakeRepository) Rotate(ctx context.Context, hash []byte, now time.Time, replacement *RefreshToken, window time.Duration) (*Rotation, error) {
	return f.rotateFn(ctx, hash, now, replacement, window)
}
func (f *fakeRepository) RevokeFamily(ctx context.Context, familyID string, now time.Time) error {
	return f.revokeFamilyFn(ctx, familyID, now)
}
func (f *fakeRepository) RevokeByRefreshToken(ctx context.Context, hash []byte, now time.Time) error {
	if f.revokeByRefreshFn == nil {
		return nil
	}
	return f.revokeByRefreshFn(ctx, hash, now)
}
func (f *fakeRepository) RevokeUserFamilies(ctx context.Context, userID uint64, now time.Time) error {
	return f.revokeUserFamiliesFn(ctx, userID, now)
}
func (f *fakeRepository) ValidateFamily(ctx context.Context, userID uint64, familyID string, now time.Time) (bool, error) {
	return f.validateFamilyFn(ctx, userID, familyID, now)
}

func TestLogoutByRefreshTokenHashesOpaqueValue(t *testing.T) {
	var gotHash []byte
	repo := &fakeRepository{
		revokeByRefreshFn: func(_ context.Context, hash []byte, _ time.Time) error {
			gotHash = append([]byte(nil), hash...)
			return nil
		},
	}
	service := newSessionService(repo)
	if err := service.LogoutByRefreshToken(context.Background(), "refresh-secret"); err != nil {
		t.Fatalf("logout by refresh token: %v", err)
	}
	if len(gotHash) != 32 || string(gotHash) == "refresh-secret" {
		t.Fatalf("refresh token was not stored as a SHA-256 digest: %x", gotHash)
	}
}

func newSessionService(repo Repository) *Service {
	return NewService(repo, token.NewManager("test-secret", "video-share", 15*time.Minute), Options{
		Now:            func() time.Time { return time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC) },
		FamilyTTL:      30 * 24 * time.Hour,
		RefreshTTL:     30 * 24 * time.Hour,
		ConflictWindow: 10 * time.Second,
	})
}

func TestIssueStoresOpaqueRefreshTokenAndSIDAccessToken(t *testing.T) {
	var family Family
	var refresh RefreshToken
	repo := &fakeRepository{
		createFamilyFn: func(_ context.Context, gotFamily *Family, gotRefresh *RefreshToken) error {
			family = *gotFamily
			refresh = *gotRefresh
			return nil
		},
	}
	svc := newSessionService(repo)
	issued, err := svc.Issue(context.Background(), 7)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if issued.AccessToken == "" || issued.RefreshToken == "" || len(issued.RefreshToken) < 40 {
		t.Fatalf("issued credentials are incomplete: %+v", issued)
	}
	if strings.Contains(issued.RefreshToken, string(refresh.TokenHash)) || string(refresh.TokenHash) == issued.RefreshToken {
		t.Fatal("refresh token was stored in plaintext")
	}
	if len(refresh.TokenHash) != 32 || refresh.FamilyID != family.ID || family.UserID != 7 {
		t.Fatalf("stored session = family=%+v refresh=%+v", family, refresh)
	}
	claims, err := svc.tokens.Parse(issued.AccessToken)
	if err != nil || claims.SessionID != family.ID {
		t.Fatalf("access claims = %+v, err=%v", claims, err)
	}
}

func TestRefreshMapsConflictAndReuseWithoutLeakingToken(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want error
	}{
		{name: "conflict", err: ErrRefreshConflict, want: ErrRefreshConflict},
		{name: "reused", err: ErrSessionReused, want: ErrSessionReused},
		{name: "expired", err: ErrSessionExpired, want: ErrSessionExpired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRepository{
				rotateFn: func(_ context.Context, _ []byte, _ time.Time, replacement *RefreshToken, _ time.Duration) (*Rotation, error) {
					if len(replacement.TokenHash) != 32 || replacement.TokenHash == nil {
						t.Fatalf("replacement hash = %x", replacement.TokenHash)
					}
					return nil, tt.err
				},
			}
			svc := newSessionService(repo)
			_, err := svc.Refresh(context.Background(), "old-refresh-token")
			if !errors.Is(err, tt.want) {
				t.Fatalf("Refresh error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestValidateAccessRequiresActiveFamily(t *testing.T) {
	repo := &fakeRepository{
		validateFamilyFn: func(_ context.Context, userID uint64, familyID string, _ time.Time) (bool, error) {
			return userID == 7 && familyID == "family-7", nil
		},
	}
	svc := newSessionService(repo)
	if ok, err := svc.ValidateAccess(context.Background(), 7, "family-7"); err != nil || !ok {
		t.Fatalf("active family = %v, err=%v", ok, err)
	}
	if ok, err := svc.ValidateAccess(context.Background(), 7, "revoked"); err != nil || ok {
		t.Fatalf("revoked family = %v, err=%v", ok, err)
	}
}
