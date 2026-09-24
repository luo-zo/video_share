package user

import "time"

type RegisterRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	Nickname string `json:"nickname" binding:"required"`
}

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// UserResponse 是用户的对外表示；它有意省略 password_hash 字段。
type UserResponse struct {
	ID        uint64    `json:"id"`
	Username  string    `json:"username"`
	Nickname  string    `json:"nickname"`
	Bio       string    `json:"bio"`
	Role      string    `json:"role,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type LoginResponse struct {
	AccessToken string       `json:"access_token"`
	TokenType   string       `json:"token_type"`
	ExpiresIn   int64        `json:"expires_in"`
	User        UserResponse `json:"user,omitempty"`
}

func NewLoginResponse(u *User, accessToken string, expiresIn time.Duration) LoginResponse {
	return LoginResponse{AccessToken: accessToken, TokenType: "Bearer", ExpiresIn: int64(expiresIn.Seconds()), User: toUserResponse(u)}
}

func Response(u *User) UserResponse {
	return toUserResponse(u)
}

func toUserResponse(u *User) UserResponse {
	return UserResponse{
		ID:        u.ID,
		Username:  u.Username,
		Nickname:  u.Nickname,
		Bio:       u.Bio,
		Role:      u.Role,
		CreatedAt: u.CreatedAt,
	}
}
