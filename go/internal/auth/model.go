package auth

import (
	"context"
	"time"
)

type User struct {
	ID           string
	Account      string
	DisplayName  string
	PasswordHash string
	Status       string
	Roles        []string
}

type AuthUser struct {
	ID          string   `json:"id"`
	Account     string   `json:"account"`
	DisplayName string   `json:"displayName"`
	Roles       []string `json:"roles"`
	Status      string   `json:"status"`
}

type LoginRequest struct {
	Account    string `json:"account"`
	Password   string `json:"password"`
	RememberMe bool   `json:"rememberMe"`
}

type LoginResult struct {
	Token string   `json:"token"`
	User  AuthUser `json:"user"`
}

type Repository interface {
	FindByAccount(ctx context.Context, account string) (User, error)
	FindBySessionToken(ctx context.Context, tokenHash string) (User, error)
	CreateSession(ctx context.Context, userID string, tokenHash string, expiresAt time.Time) error
	RevokeSession(ctx context.Context, tokenHash string) error
	UpdateLastLogin(ctx context.Context, userID string) error
	TouchSession(ctx context.Context, tokenHash string) error
	ListActiveUsersByRole(ctx context.Context, role string) ([]AuthUser, error)
}
