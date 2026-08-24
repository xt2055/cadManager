package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrUserNotFound = errors.New("user not found")

type PGRepository struct {
	pool *pgxpool.Pool
}

func NewPGRepository(pool *pgxpool.Pool) *PGRepository {
	return &PGRepository{pool: pool}
}

func (repository *PGRepository) FindByAccount(ctx context.Context, account string) (User, error) {
	return repository.findUser(ctx, `
		SELECT u.id::text, u.account, u.display_name, u.password_hash, u.status,
		       COALESCE(array_agg(ur.role ORDER BY ur.role) FILTER (WHERE ur.role IS NOT NULL), '{}')
		FROM users u
		LEFT JOIN user_roles ur ON ur.user_id = u.id
		WHERE lower(u.account) = lower($1)
		GROUP BY u.id`, account)
}

func (repository *PGRepository) FindBySessionToken(ctx context.Context, tokenHash string) (User, error) {
	return repository.findUser(ctx, `
		SELECT u.id::text, u.account, u.display_name, u.password_hash, u.status,
		       COALESCE(array_agg(ur.role ORDER BY ur.role) FILTER (WHERE ur.role IS NOT NULL), '{}')
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		LEFT JOIN user_roles ur ON ur.user_id = u.id
		WHERE s.token_hash = $1
		  AND s.revoked_at IS NULL
		  AND s.expires_at > now()
		GROUP BY u.id`, tokenHash)
}

func (repository *PGRepository) findUser(ctx context.Context, query string, argument string) (User, error) {
	if repository == nil || repository.pool == nil {
		return User{}, errors.New("数据库连接未配置")
	}

	var user User
	err := repository.pool.QueryRow(ctx, query, argument).Scan(
		&user.ID,
		&user.Account,
		&user.DisplayName,
		&user.PasswordHash,
		&user.Status,
		&user.Roles,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("查询用户失败: %w", err)
	}
	return user, nil
}

func (repository *PGRepository) CreateSession(ctx context.Context, userID string, tokenHash string, expiresAt time.Time) error {
	if repository == nil || repository.pool == nil {
		return errors.New("数据库连接未配置")
	}
	_, err := repository.pool.Exec(ctx, `
		INSERT INTO sessions (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)`, userID, tokenHash, expiresAt)
	if err != nil {
		return fmt.Errorf("创建登录会话失败: %w", err)
	}
	return nil
}

func (repository *PGRepository) RevokeSession(ctx context.Context, tokenHash string) error {
	if repository == nil || repository.pool == nil {
		return errors.New("数据库连接未配置")
	}
	_, err := repository.pool.Exec(ctx, `
		UPDATE sessions
		SET revoked_at = COALESCE(revoked_at, now())
		WHERE token_hash = $1`, tokenHash)
	if err != nil {
		return fmt.Errorf("注销登录会话失败: %w", err)
	}
	return nil
}

func (repository *PGRepository) TouchSession(ctx context.Context, tokenHash string) error {
	if repository == nil || repository.pool == nil {
		return errors.New("数据库连接未配置")
	}
	_, err := repository.pool.Exec(ctx, `
		UPDATE sessions
		SET last_seen_at = now()
		WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > now()`, tokenHash)
	if err != nil {
		return fmt.Errorf("更新在线状态失败: %w", err)
	}
	return nil
}

func (repository *PGRepository) UpdateLastLogin(ctx context.Context, userID string) error {
	if repository == nil || repository.pool == nil {
		return errors.New("数据库连接未配置")
	}
	_, err := repository.pool.Exec(ctx, `
		UPDATE users SET last_login_at = $2 WHERE id = $1`, userID, time.Now())
	if err != nil {
		return fmt.Errorf("更新用户登录时间失败: %w", err)
	}
	return nil
}

func (repository *PGRepository) ListActiveUsersByRole(ctx context.Context, role string) ([]AuthUser, error) {
	if repository == nil || repository.pool == nil {
		return nil, errors.New("数据库连接未配置")
	}
	role = strings.TrimSpace(role)
	var query string
	var args []any
	if role != "" {
		query = `
			SELECT u.id::text, u.account, u.display_name, u.status,
			       COALESCE(array_agg(ur.role ORDER BY ur.role) FILTER (WHERE ur.role IS NOT NULL), '{}')
			FROM users u
			JOIN user_roles target_ur ON target_ur.user_id = u.id AND target_ur.role = $1
			LEFT JOIN user_roles ur ON ur.user_id = u.id
			WHERE u.status = 'active'
			GROUP BY u.id
			ORDER BY u.display_name, u.account`
		args = append(args, role)
	} else {
		query = `
			SELECT u.id::text, u.account, u.display_name, u.status,
			       COALESCE(array_agg(ur.role ORDER BY ur.role) FILTER (WHERE ur.role IS NOT NULL), '{}')
			FROM users u
			LEFT JOIN user_roles ur ON ur.user_id = u.id
			WHERE u.status = 'active'
			GROUP BY u.id
			ORDER BY u.display_name, u.account`
	}

	rows, err := repository.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("查询人员列表失败: %w", err)
	}
	defer rows.Close()

	users := make([]AuthUser, 0)
	for rows.Next() {
		var user AuthUser
		if err := rows.Scan(&user.ID, &user.Account, &user.DisplayName, &user.Status, &user.Roles); err != nil {
			return nil, fmt.Errorf("读取人员列表失败: %w", err)
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func NormalizeAccount(account string) string {
	return strings.ToLower(strings.TrimSpace(account))
}
