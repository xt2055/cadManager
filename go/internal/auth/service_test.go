package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type memoryRepository struct {
	user      User
	created   bool
	tokenHash string
	expiresAt time.Time
	lastLogin bool
	revoked   bool
}

func (repository *memoryRepository) FindByAccount(_ context.Context, account string) (User, error) {
	if repository.user.Account != NormalizeAccount(account) {
		return User{}, ErrUserNotFound
	}
	return repository.user, nil
}

func (repository *memoryRepository) FindByID(_ context.Context, userID string) (User, error) {
	if repository.user.ID != userID {
		return User{}, ErrUserNotFound
	}
	return repository.user, nil
}

func (repository *memoryRepository) FindBySessionToken(_ context.Context, tokenHash string) (User, error) {
	if !repository.created || repository.tokenHash != tokenHash || repository.revoked || time.Now().After(repository.expiresAt) {
		return User{}, ErrUserNotFound
	}
	return repository.user, nil
}

func (repository *memoryRepository) CreateSession(_ context.Context, _ string, tokenHash string, expiresAt time.Time) error {
	repository.created = true
	repository.tokenHash = tokenHash
	repository.expiresAt = expiresAt
	return nil
}

func (repository *memoryRepository) RevokeSession(_ context.Context, _ string) error {
	repository.revoked = true
	return nil
}

func (repository *memoryRepository) UpdateLastLogin(_ context.Context, _ string) error {
	repository.lastLogin = true
	return nil
}

func (repository *memoryRepository) ListActiveUsersByRole(_ context.Context, role string) ([]AuthUser, error) {
	if repository.user.Status != "active" {
		return nil, nil
	}
	if role != "" {
		matched := false
		for _, r := range repository.user.Roles {
			if r == role {
				matched = true
				break
			}
		}
		if !matched {
			return nil, nil
		}
	}
	return []AuthUser{toAuthUser(repository.user)}, nil
}

func (repository *memoryRepository) TouchSession(_ context.Context, _ string) error {
	return nil
}

func (repository *memoryRepository) ListUsers(_ context.Context) ([]AuthUser, error) {
	return []AuthUser{toAuthUser(repository.user)}, nil
}

func (repository *memoryRepository) CreateUser(_ context.Context, _ UserInput) (AuthUser, error) {
	return AuthUser{}, nil
}

func (repository *memoryRepository) UpdateUser(_ context.Context, _ string, _ UserInput) (AuthUser, error) {
	return AuthUser{}, nil
}

func (repository *memoryRepository) MigrateLegacyUsers(_ context.Context) error {
	return nil
}

func TestServiceLoginAndLogout(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	repository := &memoryRepository{user: User{
		ID:           "user-1",
		Account:      "admin",
		DisplayName:  "系统管理员",
		PasswordHash: string(hash),
		Status:       "active",
		Roles:        []string{"admin"},
	}}
	service := NewService(repository)

	result, err := service.Login(context.Background(), LoginRequest{Account: " ADMIN ", Password: "secret", RememberMe: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Token == "" || result.User.Account != "admin" {
		t.Fatalf("unexpected login result: %+v", result)
	}
	if !repository.lastLogin || repository.expiresAt.Before(time.Now().Add(29*24*time.Hour)) {
		t.Fatal("login session was not initialized correctly")
	}

	current, err := service.CurrentUser(context.Background(), result.Token)
	if err != nil || current.ID != "user-1" {
		t.Fatalf("current user = %+v, error = %v", current, err)
	}
	if err := service.Logout(context.Background(), result.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CurrentUser(context.Background(), result.Token); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("current user after logout error = %v", err)
	}
}

func TestServiceRejectsInvalidPassword(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.DefaultCost)
	service := NewService(&memoryRepository{user: User{Account: "admin", PasswordHash: string(hash), Status: "active"}})
	if _, err := service.Login(context.Background(), LoginRequest{Account: "admin", Password: "wrong"}); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("login error = %v", err)
	}
}
