package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("账号或密码错误")
	ErrAccountNotFound    = errors.New("账号不存在")
	ErrInvalidPassword    = errors.New("密码错误")
	ErrDisabledUser       = errors.New("账号已被禁用，请联系管理员")
	ErrInvalidToken       = errors.New("登录会话无效或已过期")
	ErrInvalidUserInput   = errors.New("账号信息不完整或角色无效")
	ErrUserConflict       = errors.New("登录账号已存在")
	ErrLastAdmin          = errors.New("不能禁用或移除最后一个管理员")
)

type Service struct {
	repository Repository
	now        func() time.Time
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository, now: time.Now}
}

func (service *Service) Login(ctx context.Context, request LoginRequest) (LoginResult, error) {
	account := NormalizeAccount(request.Account)
	if account == "" || strings.TrimSpace(request.Password) == "" {
		return LoginResult{}, ErrInvalidCredentials
	}

	user, err := service.repository.FindByAccount(ctx, account)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return LoginResult{}, ErrAccountNotFound
		}
		return LoginResult{}, err
	}
	if user.Status != "active" {
		return LoginResult{}, ErrDisabledUser
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(request.Password)); err != nil {
		return LoginResult{}, ErrInvalidPassword
	}

	token, tokenHash, err := createToken()
	if err != nil {
		return LoginResult{}, err
	}
	expiresIn := 8 * time.Hour
	if request.RememberMe {
		expiresIn = 30 * 24 * time.Hour
	}
	if err := service.repository.CreateSession(ctx, user.ID, tokenHash, service.now().Add(expiresIn)); err != nil {
		return LoginResult{}, err
	}
	if err := service.repository.UpdateLastLogin(ctx, user.ID); err != nil {
		return LoginResult{}, err
	}

	return LoginResult{Token: token, User: toAuthUser(user)}, nil
}

func (service *Service) CurrentUser(ctx context.Context, token string) (AuthUser, error) {
	tokenHash := hashToken(token)
	if tokenHash == "" {
		return AuthUser{}, ErrInvalidToken
	}
	user, err := service.repository.FindBySessionToken(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return AuthUser{}, ErrInvalidToken
		}
		return AuthUser{}, err
	}
	if user.Status != "active" {
		return AuthUser{}, ErrDisabledUser
	}
	return toAuthUser(user), nil
}

func (service *Service) Logout(ctx context.Context, token string) error {
	tokenHash := hashToken(token)
	if tokenHash == "" {
		return nil
	}
	return service.repository.RevokeSession(ctx, tokenHash)
}

func (service *Service) Heartbeat(ctx context.Context, token string) error {
	tokenHash := hashToken(token)
	if tokenHash == "" {
		return ErrInvalidToken
	}
	if _, err := service.CurrentUser(ctx, token); err != nil {
		return err
	}
	return service.repository.TouchSession(ctx, tokenHash)
}

func (service *Service) ListActiveUsersByRole(ctx context.Context, role string) ([]AuthUser, error) {
	return service.repository.ListActiveUsersByRole(ctx, role)
}

func (service *Service) ListUsers(ctx context.Context) ([]AuthUser, error) {
	return service.repository.ListUsers(ctx)
}

func (service *Service) CreateUser(ctx context.Context, input UserInput) (AuthUser, error) {
	input.Account = NormalizeAccount(input.Account)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Password = strings.TrimSpace(input.Password)
	input.Roles = normalizeRoles(input.Roles)
	if input.Account == "" || input.DisplayName == "" || input.Password == "" || len(input.Roles) == 0 {
		return AuthUser{}, ErrInvalidUserInput
	}
	input.Status = "active"
	return service.repository.CreateUser(ctx, input)
}

func (service *Service) UpdateUser(ctx context.Context, userID string, input UserInput) (AuthUser, error) {
	input.Account = NormalizeAccount(input.Account)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Password = strings.TrimSpace(input.Password)
	input.Roles = normalizeRoles(input.Roles)
	if userID == "" || input.Account == "" || input.DisplayName == "" || len(input.Roles) == 0 {
		return AuthUser{}, ErrInvalidUserInput
	}
	if input.Status != "disabled" {
		input.Status = "active"
	}
	return service.repository.UpdateUser(ctx, userID, input)
}

func normalizeRoles(roles []string) []string {
	seen := make(map[string]struct{}, len(roles))
	result := make([]string, 0, len(roles))
	for _, role := range roles {
		if role != "admin" && role != "designer" && role != "reviewer" {
			continue
		}
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		result = append(result, role)
	}
	return result
}

func toAuthUser(user User) AuthUser {
	return AuthUser{
		ID:          user.ID,
		Account:     user.Account,
		DisplayName: user.DisplayName,
		Roles:       user.Roles,
		Status:      user.Status,
		CreatedAt:   user.CreatedAt,
		LastLoginAt: user.LastLoginAt,
	}
}

func createToken() (string, string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", fmt.Errorf("生成登录令牌失败: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(bytes)
	return token, hashToken(token), nil
}

func hashToken(token string) string {
	if strings.TrimSpace(token) == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", digest)
}
