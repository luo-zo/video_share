package token

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestGenerateAndParse(t *testing.T) {
	m := NewManager("test-secret", "video-share", time.Hour)
	signed, ttl, err := m.Generate(42)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if ttl != time.Hour {
		t.Fatalf("ttl = %v, want %v", ttl, time.Hour)
	}

	claims, err := m.Parse(signed)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if claims.UserID != 42 {
		t.Fatalf("UserID = %d, want 42", claims.UserID)
	}
	if claims.Issuer != "video-share" {
		t.Fatalf("Issuer = %q, want video-share", claims.Issuer)
	}
}

func TestParseRejectsExpiredToken(t *testing.T) {
	m := NewManager("test-secret", "video-share", time.Hour)
	m.now = func() time.Time { return time.Now().Add(-2 * time.Hour) }
	signed, _, err := m.Generate(1)
	if err != nil {
		t.Fatal(err)
	}
	m.now = time.Now

	if _, err := m.Parse(signed); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestParseRejectsWrongSecret(t *testing.T) {
	signer := NewManager("secret-a", "video-share", time.Hour)
	verifier := NewManager("secret-b", "video-share", time.Hour)
	signed, _, _ := signer.Generate(1)

	if _, err := verifier.Parse(signed); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestParseRejectsWrongAlgorithm(t *testing.T) {
	m := NewManager("test-secret", "video-share", time.Hour)
	now := time.Now()
	claims := Claims{
		UserID: 1,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "video-share",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS384, claims).SignedString([]byte("test-secret"))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := m.Parse(signed); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestParseRejectsTamperedToken(t *testing.T) {
	m := NewManager("test-secret", "video-share", time.Hour)
	signed, _, _ := m.Generate(1)

	parts := strings.Split(signed, ".")
	parts[2] = strings.Repeat("A", len(parts[2]))
	tampered := strings.Join(parts, ".")

	if _, err := m.Parse(tampered); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestParseRejectsMalformedToken(t *testing.T) {
	m := NewManager("test-secret", "video-share", time.Hour)
	if _, err := m.Parse("not-a-jwt"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestParseRejectsWrongIssuer(t *testing.T) {
	m := NewManager("test-secret", "video-share", time.Hour)
	now := time.Now()
	claims := Claims{
		UserID: 1,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "evil-issuer",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		},
	}
	signed, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("test-secret"))

	if _, err := m.Parse(signed); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestParseRejectsMissingUserID(t *testing.T) {
	m := NewManager("test-secret", "video-share", time.Hour)
	now := time.Now()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "video-share",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		},
	}
	signed, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("test-secret"))

	if _, err := m.Parse(signed); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}
