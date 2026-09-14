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
	username = strings.ToLower(strings.TrimSpace(username))
	if username == "" {
		return nil, "", 0, ErrInvalidCredentials
	}

	u, err := s.repo.FindByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, "", 0, ErrInvalidCredentials
		}
		return nil, "", 0, err
	}

	// 禁用用户无法登录；返回相同的通用错误以避免账号枚举。
	if u.Status != StatusNormal {
		return nil, "", 0, ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return nil, "", 0, ErrInvalidCredentials
	}

	tokenStr, ttl, err := s.token.Generate(u.ID)
	if err != nil {
		return nil, "", 0, err
	}
	return u, tokenStr, ttl, nil
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
