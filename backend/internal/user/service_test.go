package user

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"

	"video_share/internal/token"
)

type mockRepo struct {
	createFn         func(ctx context.Context, u *User) error
	findByUsernameFn func(ctx context.Context, username string) (*User, error)
	findByIDFn       func(ctx context.Context, id uint64) (*User, error)
}

func (m *mockRepo) Create(ctx context.Context, u *User) error {
	return m.createFn(ctx, u)
}

func (m *mockRepo) FindByUsername(ctx context.Context, username string) (*User, error) {
	return m.findByUsernameFn(ctx, username)
}

func (m *mockRepo) FindByID(ctx context.Context, id uint64) (*User, error) {
	return m.findByIDFn(ctx, id)
}

func testService() (*Service, *mockRepo, *token.Manager) {
	repo := &mockRepo{}
	tm := token.NewManager("test-secret", "video-share", time.Hour)
	return NewService(repo, tm), repo, tm
}

func TestRegisterNormalizesAndHashes(t *testing.T) {
	svc, repo, _ := testService()
	var created *User
	repo.createFn = func(ctx context.Context, u *User) error {
		u.ID = 1
		created = u
		return nil
	}

	u, err := svc.Register(context.Background(), "  Alice_01  ", "password123", "爱丽丝")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if u.Username != "alice_01" {
		t.Fatalf("username = %q, want alice_01", u.Username)
	}
	if u.Status != StatusNormal {
		t.Fatalf("status = %v, want normal", u.Status)
	}
	if created.PasswordHash == "password123" {
		t.Fatal("password stored in plaintext")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(created.PasswordHash), []byte("password123")); err != nil {
		t.Fatalf("stored hash does not verify: %v", err)
	}
}

func TestRegisterInvalidUsername(t *testing.T) {
	svc, _, _ := testService()
	cases := []string{"ab", "a_b-c", "a!b", strings.Repeat("a", 33)}
	for _, un := range cases {
		if _, err := svc.Register(context.Background(), un, "password123", "nick"); !errors.Is(err, ErrUsernameInvalid) {
			t.Errorf("username %q: expected ErrUsernameInvalid, got %v", un, err)
		}
	}
}

func TestRegisterInvalidPassword(t *testing.T) {
	svc, _, _ := testService()
	if _, err := svc.Register(context.Background(), "alice", "short", "nick"); !errors.Is(err, ErrPasswordInvalid) {
		t.Errorf("short password: expected ErrPasswordInvalid, got %v", err)
	}
	if _, err := svc.Register(context.Background(), "alice", strings.Repeat("a", 73), "nick"); !errors.Is(err, ErrPasswordInvalid) {
		t.Errorf("long password: expected ErrPasswordInvalid, got %v", err)
	}
}

func TestRegisterInvalidNickname(t *testing.T) {
	svc, _, _ := testService()
	if _, err := svc.Register(context.Background(), "alice", "password123", ""); !errors.Is(err, ErrNicknameInvalid) {
		t.Errorf("empty nickname: expected ErrNicknameInvalid, got %v", err)
	}
	if _, err := svc.Register(context.Background(), "alice", "password123", strings.Repeat("名", 65)); !errors.Is(err, ErrNicknameInvalid) {
		t.Errorf("long nickname: expected ErrNicknameInvalid, got %v", err)
	}
}

func TestRegisterDuplicate(t *testing.T) {
	svc, repo, _ := testService()
	repo.createFn = func(ctx context.Context, u *User) error {
		return &mysql.MySQLError{Number: 1062, Message: "Duplicate entry"}
	}

	if _, err := svc.Register(context.Background(), "alice", "password123", "nick"); !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("expected ErrUsernameTaken, got %v", err)
	}
}

func TestLoginSuccess(t *testing.T) {
	svc, repo, tm := testService()
	hash, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	repo.findByUsernameFn = func(ctx context.Context, username string) (*User, error) {
		return &User{ID: 7, Username: "alice", PasswordHash: string(hash), Nickname: "爱丽丝", Status: StatusNormal}, nil
	}

	u, tokenStr, ttl, err := svc.Login(context.Background(), "Alice", "password123")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if u.ID != 7 {
		t.Fatalf("id = %d, want 7", u.ID)
	}
	if tokenStr == "" || ttl <= 0 {
		t.Fatalf("token/ttl invalid: %q %v", tokenStr, ttl)
	}
	claims, err := tm.Parse(tokenStr)
	if err != nil {
		t.Fatalf("generated token not parseable: %v", err)
	}
	if claims.UserID != 7 {
		t.Fatalf("token uid = %d, want 7", claims.UserID)
	}
}

func TestLoginWrongPassword(t *testing.T) {
	svc, repo, _ := testService()
	hash, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	repo.findByUsernameFn = func(ctx context.Context, username string) (*User, error) {
		return &User{ID: 7, Username: "alice", PasswordHash: string(hash), Status: StatusNormal}, nil
	}

	if _, _, _, err := svc.Login(context.Background(), "alice", "wrongpass"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestLoginUserNotFound(t *testing.T) {
	svc, repo, _ := testService()
	repo.findByUsernameFn = func(ctx context.Context, username string) (*User, error) {
		return nil, ErrNotFound
	}

	if _, _, _, err := svc.Login(context.Background(), "ghost", "password123"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestLoginDisabledUser(t *testing.T) {
	svc, repo, _ := testService()
	hash, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	repo.findByUsernameFn = func(ctx context.Context, username string) (*User, error) {
		return &User{ID: 7, Username: "alice", PasswordHash: string(hash), Status: StatusDisabled}, nil
	}

	if _, _, _, err := svc.Login(context.Background(), "alice", "password123"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestGetByID(t *testing.T) {
	svc, repo, _ := testService()
	repo.findByIDFn = func(ctx context.Context, id uint64) (*User, error) {
		return &User{ID: id, Username: "alice", Status: StatusNormal}, nil
	}

	u, err := svc.GetByID(context.Background(), 7)
	if err != nil || u.ID != 7 {
		t.Fatalf("GetByID = (%v, %v), want id 7, nil", u, err)
	}
}

func TestGetByIDNotFound(t *testing.T) {
	svc, repo, _ := testService()
	repo.findByIDFn = func(ctx context.Context, id uint64) (*User, error) {
		return nil, ErrNotFound
	}

	if _, err := svc.GetByID(context.Background(), 7); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestGetByIDDisabled(t *testing.T) {
	svc, repo, _ := testService()
	repo.findByIDFn = func(ctx context.Context, id uint64) (*User, error) {
		return &User{ID: id, Status: StatusDisabled}, nil
	}

	if _, err := svc.GetByID(context.Background(), 7); !errors.Is(err, ErrAccountDisabled) {
		t.Fatalf("expected ErrAccountDisabled, got %v", err)
	}
}
