package user

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"

	"video_share/internal/token"
)

const (
	minUsernameLen   = 3
	maxUsernameLen   = 32
	minPasswordLen   = 8  // 字符
	maxPasswordBytes = 72 // bcrypt 上限
	maxNicknameLen   = 64
)

var (
	ErrUsernameInvalid    = errors.New("invalid username")
	ErrPasswordInvalid    = errors.New("invalid password")
	ErrNicknameInvalid    = errors.New("invalid nickname")
	ErrUsernameTaken      = errors.New("username already taken")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrAccountDisabled    = errors.New("account disabled")
	ErrBioInvalid         = errors.New("invalid bio")
)

var usernameRe = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

type Service struct {
	repo  Repository
	token *token.Manager
}

func NewService(repo Repository, tm *token.Manager) *Service {
	return &Service{repo: repo, token: tm}
}

func (s *Service) Register(ctx context.Context, username, password, nickname string) (*User, error) {
	username, err := normalizeUsername(username)
	if err != nil {
		return nil, err
	}
	if err := validatePassword(password); err != nil {
		return nil, err
	}
	nickname = strings.TrimSpace(nickname)
	if err := validateNickname(nickname); err != nil {
		return nil, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	u := &User{
		Username:     username,
		PasswordHash: string(hash),
		Nickname:     nickname,
		Status:       StatusNormal,
	}

	if err := s.repo.Create(ctx, u); err != nil {
		if IsDuplicateKey(err) {
			return nil, ErrUsernameTaken
		}
		return nil, err
	}
	return u, nil
}

func (s *Service) Login(ctx context.Context, username, password string) (*User, string, time.Duration, error) {
	u, err := s.Authenticate(ctx, username, password)
	if err != nil {
		return nil, "", 0, err
	}
	tokenStr, ttl, err := s.token.Generate(u.ID)
	if err != nil {
		return nil, "", 0, err
	}
	return u, tokenStr, ttl, nil
}

// Authenticate verifies credentials without creating a legacy access token;
// the session package issues the sid-bound token after creating a family.
func (s *Service) Authenticate(ctx context.Context, username, password string) (*User, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	if username == "" {
		return nil, ErrInvalidCredentials
	}
	u, err := s.repo.FindByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}
	if u.Status != StatusNormal || bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		return nil, ErrInvalidCredentials
	}
	return u, nil
}

func (s *Service) UpdateProfile(ctx context.Context, userID uint64, nickname, bio string) (*User, error) {
	if userID == 0 {
		return nil, ErrNotFound
	}
	nickname = strings.TrimSpace(nickname)
	bio = strings.TrimSpace(bio)
	if err := validateNickname(nickname); err != nil {
		return nil, err
	}
	if utf8.RuneCountInString(bio) > 200 {
		return nil, ErrBioInvalid
	}
	return s.repo.UpdateProfile(ctx, userID, nickname, bio)
}

func (s *Service) ChangePassword(ctx context.Context, userID uint64, oldPassword, newPassword string) error {
	if userID == 0 {
		return ErrInvalidCredentials
	}
	if err := validatePassword(newPassword); err != nil {
		return err
	}
	u, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if u.Status != StatusNormal || bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(oldPassword)) != nil {
		return ErrInvalidCredentials
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	return s.repo.ChangePasswordAndRevokeSessions(ctx, userID, string(hash), time.Now().UTC())
}

func (s *Service) GetByID(ctx context.Context, id uint64) (*User, error) {
	u, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if u.Status != StatusNormal {
		return nil, ErrAccountDisabled
	}
	return u, nil
}

func normalizeUsername(username string) (string, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	if len(username) < minUsernameLen || len(username) > maxUsernameLen {
		return "", ErrUsernameInvalid
	}
	if !usernameRe.MatchString(username) {
		return "", ErrUsernameInvalid
	}
	return username, nil
}

func validatePassword(password string) error {
	if utf8.RuneCountInString(password) < minPasswordLen {
		return ErrPasswordInvalid
	}
	if len(password) > maxPasswordBytes {
		return ErrPasswordInvalid
	}
	return nil
}

func validateNickname(nickname string) error {
	n := utf8.RuneCountInString(nickname)
	if n == 0 || n > maxNicknameLen {
		return ErrNicknameInvalid
	}
	return nil
}
