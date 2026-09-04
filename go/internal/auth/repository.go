package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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
		SELECT u.id::text, u.account, u.display_name, u.password_hash, u.status, u.created_at, u.last_login_at,
		       COALESCE(array_agg(ur.role ORDER BY ur.role) FILTER (WHERE ur.role IS NOT NULL), '{}')
		FROM users u
		LEFT JOIN user_roles ur ON ur.user_id = u.id
		WHERE lower(u.account) = lower($1)
		GROUP BY u.id`, account)
}

func (repository *PGRepository) FindByID(ctx context.Context, userID string) (User, error) {
	return repository.findUser(ctx, `
		SELECT u.id::text, u.account, u.display_name, u.password_hash, u.status, u.created_at, u.last_login_at,
		       COALESCE(array_agg(ur.role ORDER BY ur.role) FILTER (WHERE ur.role IS NOT NULL), '{}')
		FROM users u
		LEFT JOIN user_roles ur ON ur.user_id = u.id
		WHERE u.id = $1::uuid
		GROUP BY u.id`, userID)
}

func (repository *PGRepository) FindBySessionToken(ctx context.Context, tokenHash string) (User, error) {
	return repository.findUser(ctx, `
		SELECT u.id::text, u.account, u.display_name, u.password_hash, u.status, u.created_at, u.last_login_at,
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
		&user.CreatedAt,
		&user.LastLoginAt,
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
			SELECT u.id::text, u.account, u.display_name, u.status, u.created_at, u.last_login_at,
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
			SELECT u.id::text, u.account, u.display_name, u.status, u.created_at, u.last_login_at,
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
		if err := rows.Scan(&user.ID, &user.Account, &user.DisplayName, &user.Status, &user.CreatedAt, &user.LastLoginAt, &user.Roles); err != nil {
			return nil, fmt.Errorf("读取人员列表失败: %w", err)
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (repository *PGRepository) ListUsers(ctx context.Context) ([]AuthUser, error) {
	if repository == nil || repository.pool == nil {
		return nil, errors.New("数据库连接未配置")
	}
	rows, err := repository.pool.Query(ctx, `
		SELECT u.id::text, u.account, u.display_name, u.status, u.created_at, u.last_login_at,
		       COALESCE(array_agg(ur.role ORDER BY ur.role) FILTER (WHERE ur.role IS NOT NULL), '{}')
		FROM users u
		LEFT JOIN user_roles ur ON ur.user_id = u.id
		GROUP BY u.id
		ORDER BY u.display_name, u.account`)
	if err != nil {
		return nil, fmt.Errorf("查询账号列表失败: %w", err)
	}
	defer rows.Close()
	users := make([]AuthUser, 0)
	for rows.Next() {
		var user AuthUser
		if err := rows.Scan(&user.ID, &user.Account, &user.DisplayName, &user.Status, &user.CreatedAt, &user.LastLoginAt, &user.Roles); err != nil {
			return nil, fmt.Errorf("读取账号列表失败: %w", err)
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (repository *PGRepository) CreateUser(ctx context.Context, input UserInput) (AuthUser, error) {
	if repository == nil || repository.pool == nil {
		return AuthUser{}, errors.New("数据库连接未配置")
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return AuthUser{}, fmt.Errorf("创建账号事务失败: %w", err)
	}
	defer tx.Rollback(ctx)
	var userID string
	err = tx.QueryRow(ctx, `
		INSERT INTO users (account, display_name, password_hash, status)
		VALUES ($1, $2, crypt($3, gen_salt('bf')), 'active')
		RETURNING id::text`, input.Account, input.DisplayName, input.Password).Scan(&userID)
	if err != nil {
		if isUniqueViolation(err) {
			return AuthUser{}, ErrUserConflict
		}
		return AuthUser{}, fmt.Errorf("创建账号失败: %w", err)
	}
	if err := insertRoles(ctx, tx, userID, input.Roles); err != nil {
		return AuthUser{}, err
	}
	user, err := readUserTx(ctx, tx, userID)
	if err != nil {
		return AuthUser{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return AuthUser{}, fmt.Errorf("提交创建账号失败: %w", err)
	}
	return toAuthUser(user), nil
}

func (repository *PGRepository) UpdateUser(ctx context.Context, userID string, input UserInput) (AuthUser, error) {
	if repository == nil || repository.pool == nil {
		return AuthUser{}, errors.New("数据库连接未配置")
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return AuthUser{}, fmt.Errorf("修改账号事务失败: %w", err)
	}
	defer tx.Rollback(ctx)
	var oldStatus string
	var oldIsAdmin bool
	err = tx.QueryRow(ctx, `
		SELECT u.status, EXISTS (SELECT 1 FROM user_roles WHERE user_id = u.id AND role = 'admin')
		FROM users u WHERE u.id = $1::uuid FOR UPDATE`, userID).Scan(&oldStatus, &oldIsAdmin)
	if errors.Is(err, pgx.ErrNoRows) {
		return AuthUser{}, ErrUserNotFound
	}
	if err != nil {
		return AuthUser{}, fmt.Errorf("读取账号失败: %w", err)
	}
	newIsAdmin := containsRole(input.Roles, "admin")
	if oldStatus == "active" && oldIsAdmin && (input.Status == "disabled" || !newIsAdmin) {
		var activeAdmins int
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM users u
			WHERE u.status = 'active'
			  AND EXISTS (SELECT 1 FROM user_roles WHERE user_id = u.id AND role = 'admin')`).Scan(&activeAdmins); err != nil {
			return AuthUser{}, fmt.Errorf("检查管理员数量失败: %w", err)
		}
		if activeAdmins <= 1 {
			return AuthUser{}, ErrLastAdmin
		}
	}
	_, err = tx.Exec(ctx, `
		UPDATE users
		SET account = $2, display_name = $3, status = $4,
		    password_hash = CASE WHEN $5 = '' THEN password_hash ELSE crypt($5, gen_salt('bf')) END,
		    updated_at = now()
		WHERE id = $1::uuid`, userID, input.Account, input.DisplayName, input.Status, input.Password)
	if err != nil {
		if isUniqueViolation(err) {
			return AuthUser{}, ErrUserConflict
		}
		return AuthUser{}, fmt.Errorf("修改账号失败: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM user_roles WHERE user_id = $1::uuid`, userID); err != nil {
		return AuthUser{}, fmt.Errorf("清理账号角色失败: %w", err)
	}
	if err := insertRoles(ctx, tx, userID, input.Roles); err != nil {
		return AuthUser{}, err
	}
	user, err := readUserTx(ctx, tx, userID)
	if err != nil {
		return AuthUser{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return AuthUser{}, fmt.Errorf("提交修改账号失败: %w", err)
	}
	return toAuthUser(user), nil
}

type txQuerier interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func insertRoles(ctx context.Context, queryer txQuerier, userID string, roles []string) error {
	for _, role := range roles {
		if _, err := queryer.Exec(ctx, `
			INSERT INTO user_roles (user_id, role)
			VALUES ($1::uuid, $2)`, userID, role); err != nil {
			return fmt.Errorf("写入账号角色失败: %w", err)
		}
	}
	return nil
}

func readUserTx(ctx context.Context, queryer txQuerier, userID string) (User, error) {
	var user User
	err := queryer.QueryRow(ctx, `
		SELECT u.id::text, u.account, u.display_name, u.password_hash, u.status, u.created_at, u.last_login_at,
		       COALESCE(array_agg(ur.role ORDER BY ur.role) FILTER (WHERE ur.role IS NOT NULL), '{}')
		FROM users u
		LEFT JOIN user_roles ur ON ur.user_id = u.id
		WHERE u.id = $1::uuid
		GROUP BY u.id`, userID).Scan(
		&user.ID, &user.Account, &user.DisplayName, &user.PasswordHash, &user.Status, &user.CreatedAt, &user.LastLoginAt, &user.Roles,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("读取账号失败: %w", err)
	}
	return user, nil
}

func containsRole(roles []string, target string) bool {
	for _, role := range roles {
		if role == target {
			return true
		}
	}
	return false
}

func isUniqueViolation(err error) bool {
	var pgError *pgconn.PgError
	return errors.As(err, &pgError) && pgError.Code == "23505"
}

func NormalizeAccount(account string) string {
	return strings.ToLower(strings.TrimSpace(account))
}
