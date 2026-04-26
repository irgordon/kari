package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type contextKey string

const UserContextKey contextKey = "kari_user_claims"

type UserClaims struct {
	UserID      uuid.UUID
	Subject     uuid.UUID
	Permissions []string
}

type User struct {
	ID           uuid.UUID `json:"id"`
	Username     string    `json:"username"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	IsActive     bool      `json:"is_active"`
	RoleID       uuid.UUID `json:"role_id"`
	Role         Role      `json:"role"`
	Rank         string    `json:"rank,omitempty"`
	Permissions  []string  `json:"permissions,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Role struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
	Rank int       `json:"rank"`
}

type UserRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	UpdateRefreshToken(ctx context.Context, id uuid.UUID, token string) error
	GetRoleByID(ctx context.Context, id uuid.UUID) (*Role, error)
	CountAdmins(ctx context.Context) (int, error)
	UpdateUserRole(ctx context.Context, userID uuid.UUID, roleID uuid.UUID) error
	HasPermission(ctx context.Context, userID uuid.UUID, resource string, action string) (bool, error)
}

type AuthService interface {
	Login(ctx context.Context, email string, password string) (*TokenPair, *User, error)
	RefreshTokens(ctx context.Context, refreshToken string) (*TokenPair, error)
	GenerateTokenPair(ctx context.Context, user *User) (*TokenPair, error)
	ValidateAccessToken(ctx context.Context, token string) (*UserClaims, error)
}

type RoleService interface {
	AssignRole(ctx context.Context, actorID uuid.UUID, targetUserID uuid.UUID, newRoleID uuid.UUID) error
}

type TokenPair struct {
	AccessToken  string
	RefreshToken string
}
