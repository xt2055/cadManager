package editing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PGRepository struct {
	pool *pgxpool.Pool
}

func NewPGRepository(pool *pgxpool.Pool) *PGRepository {
	return &PGRepository{pool: pool}
}

func (repository *PGRepository) CreateSession(ctx context.Context, session Session) error {
	_, err := repository.pool.Exec(ctx, `
		INSERT INTO edit_sessions (id, attachment_id, user_id, storage_key, work_storage_key, status, started_at, last_seen_at)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7, $8)`,
		session.ID, session.AttachmentID, session.UserID, session.StorageKey, session.WorkStorageKey, session.Status, session.StartedAt, session.LastSeenAt)
	if err != nil {
		if strings.Contains(err.Error(), "uq_edit_sessions_one_active_attachment") {
			return ErrFileBusy
		}
		return fmt.Errorf("保存编辑会话失败: %w", err)
	}
	return nil
}

func (repository *PGRepository) FindActiveByStorageKey(ctx context.Context, storageKey string, now time.Time) (Session, error) {
	var session Session
	var userName string
	var closedAt *time.Time
	var workStorageKey *string
	// 占用与会话在线状态解耦：只要会话未关闭就一直占用，
	// 用户关闭页面/退出软件后仍可重新认领自己的会话继续编辑。
	err := repository.pool.QueryRow(ctx, `
		SELECT s.id::text, s.attachment_id::text, s.storage_key, s.work_storage_key, s.user_id::text,
		       COALESCE(u.display_name, u.account, ''), s.status, s.started_at, s.last_seen_at, s.closed_at
		FROM edit_sessions s JOIN users u ON u.id = s.user_id
		WHERE s.storage_key = $1 AND s.status = 'active'
		ORDER BY s.started_at DESC LIMIT 1`, storageKey).Scan(
		&session.ID, &session.AttachmentID, &session.StorageKey, &workStorageKey, &session.UserID, &userName,
		&session.Status, &session.StartedAt, &session.LastSeenAt, &closedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrSessionNotFound
	}
	if err != nil {
		return Session{}, fmt.Errorf("查询活动编辑会话失败: %w", err)
	}
	if workStorageKey != nil {
		session.WorkStorageKey = *workStorageKey
	}
	session.UserName = userName
	session.ClosedAt = closedAt
	return session, nil
}

// FindActiveByID 按会话 ID 读取活动会话（结束编辑时用于获取工作文件键）。
func (repository *PGRepository) FindActiveByID(ctx context.Context, sessionID string) (Session, error) {
	var session Session
	var userName string
	var closedAt *time.Time
	var workStorageKey *string
	err := repository.pool.QueryRow(ctx, `
		SELECT s.id::text, s.attachment_id::text, s.storage_key, s.work_storage_key, s.user_id::text,
		       COALESCE(u.display_name, u.account, ''), s.status, s.started_at, s.last_seen_at, s.closed_at
		FROM edit_sessions s JOIN users u ON u.id = s.user_id
		WHERE s.id = $1::uuid AND s.status = 'active'`, sessionID).Scan(
		&session.ID, &session.AttachmentID, &session.StorageKey, &workStorageKey, &session.UserID, &userName,
		&session.Status, &session.StartedAt, &session.LastSeenAt, &closedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrSessionNotFound
	}
	if err != nil {
		return Session{}, fmt.Errorf("查询编辑会话失败: %w", err)
	}
	if workStorageKey != nil {
		session.WorkStorageKey = *workStorageKey
	}
	session.UserName = userName
	session.ClosedAt = closedAt
	return session, nil
}

func (repository *PGRepository) ListActiveSessions(ctx context.Context, now time.Time, drawingNo string) ([]ActiveSessionInfo, error) {
	if repository == nil || repository.pool == nil {
		return nil, errors.New("数据库连接未配置")
	}
	onlineSince := now.Add(-5 * time.Minute)
	var rows pgx.Rows
	var err error

	// 列出所有未关闭的会话（含离线），Online 标记最近 5 分钟内有心跳的会话。
	baseQuery := `
		SELECT s.id::text, s.attachment_id::text, s.storage_key, COALESCE(s.work_storage_key, ''),
		       COALESCE(a.current_name, a.original_name, ''),
		       COALESCE(d.drawing_no, parent.drawing_no, ''),
		       COALESCE(p.part_no, ''),
		       s.user_id::text,
		       COALESCE(u.display_name, u.account, ''),
		       COALESCE(u.account, ''),
		       s.status, s.started_at, s.last_seen_at, s.last_seen_at >= $1
		FROM edit_sessions s
		JOIN users u ON u.id = s.user_id
		LEFT JOIN attachments a ON a.id = s.attachment_id
		LEFT JOIN drawings d ON d.id = a.drawing_id
		LEFT JOIN structure_parts p ON p.id = a.part_id
		LEFT JOIN drawings parent ON parent.id = p.drawing_id
		WHERE s.status = 'active'`

	if strings.TrimSpace(drawingNo) != "" {
		query := baseQuery + ` AND (d.drawing_no = $2 OR parent.drawing_no = $2) ORDER BY s.started_at DESC`
		rows, err = repository.pool.Query(ctx, query, onlineSince, strings.TrimSpace(drawingNo))
	} else {
		query := baseQuery + ` ORDER BY s.started_at DESC`
		rows, err = repository.pool.Query(ctx, query, onlineSince)
	}
	if err != nil {
		return nil, fmt.Errorf("查询活动编辑会话列表失败: %w", err)
	}
	defer rows.Close()

	var list []ActiveSessionInfo
	for rows.Next() {
		var item ActiveSessionInfo
		var workKey string
		if err := rows.Scan(
			&item.ID, &item.AttachmentID, &item.StorageKey, &workKey,
			&item.FileName, &item.DrawingNo, &item.PartNo,
			&item.UserID, &item.UserName, &item.UserAccount,
			&item.Status, &item.StartedAt, &item.LastSeenAt, &item.Online,
		); err != nil {
			return nil, fmt.Errorf("读取活动编辑会话行失败: %w", err)
		}
		item.WorkStorageKey = workKey
		list = append(list, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历活动编辑会话列表失败: %w", err)
	}
	return list, nil
}

// ExpireStale 不再按心跳过期会话（占用与在线解耦，退出软件后会话保留、可重新认领）。
// 仅自动关闭长期无人认领的遗留会话，避免垃圾数据堆积。
func (repository *PGRepository) ExpireStale(ctx context.Context, now time.Time) error {
	abandonedBefore := now.Add(-7 * 24 * time.Hour)
	_, err := repository.pool.Exec(ctx, `
		UPDATE edit_sessions
		SET status = 'closed', closed_at = $1, last_seen_at = $1
		WHERE status = 'active' AND last_seen_at < $2`, now, abandonedBefore)
	if err != nil {
		return fmt.Errorf("清理遗留编辑会话失败: %w", err)
	}
	return nil
}

func (repository *PGRepository) CleanupTickets(ctx context.Context, now time.Time) error {
	usedBefore := now.Add(-24 * time.Hour)
	_, err := repository.pool.Exec(ctx, `
		DELETE FROM edit_session_tickets
		WHERE expires_at < $1 OR used_at < $2`, now, usedBefore)
	if err != nil {
		return fmt.Errorf("清理编辑票据失败: %w", err)
	}
	return nil
}

func (repository *PGRepository) CreateTicket(ctx context.Context, token string, sessionID, userID string, expiresAt time.Time) error {
	_, err := repository.pool.Exec(ctx, `
		INSERT INTO edit_session_tickets (token_hash, session_id, user_id, expires_at)
		VALUES ($1, $2::uuid, $3::uuid, $4)`, hashToken(token), sessionID, userID, expiresAt)
	if err != nil {
		return fmt.Errorf("保存编辑票据失败: %w", err)
	}
	return nil
}

func (repository *PGRepository) ConsumeTicket(ctx context.Context, token, userID string, now time.Time) (Session, error) {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return Session{}, fmt.Errorf("开始交换编辑票据事务失败: %w", err)
	}
	defer tx.Rollback(ctx)

	var session Session
	var userName string
	var closedAt *time.Time
	var ticketWorkKey *string
	err = tx.QueryRow(ctx, `
		UPDATE edit_session_tickets
		SET used_at = $3
		WHERE token_hash = $1 AND user_id = $2::uuid AND used_at IS NULL AND expires_at >= $3
		RETURNING session_id::text`, hashToken(token), userID, now).Scan(&session.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrInvalidTicket
	}
	if err != nil {
		return Session{}, fmt.Errorf("消费编辑票据失败: %w", err)
	}
	err = tx.QueryRow(ctx, `
		SELECT s.id::text, s.attachment_id::text, s.storage_key, s.work_storage_key, s.user_id::text,
		       COALESCE(u.display_name, u.account, ''), s.status, s.started_at, s.last_seen_at, s.closed_at
		FROM edit_sessions s JOIN users u ON u.id = s.user_id
		WHERE s.id = $1::uuid`, session.ID).Scan(
		&session.ID, &session.AttachmentID, &session.StorageKey, &ticketWorkKey, &session.UserID, &userName,
		&session.Status, &session.StartedAt, &session.LastSeenAt, &closedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrSessionNotFound
	}
	if err != nil {
		return Session{}, fmt.Errorf("读取编辑会话失败: %w", err)
	}
	if ticketWorkKey != nil {
		session.WorkStorageKey = *ticketWorkKey
	}
	session.UserName = userName
	session.ClosedAt = closedAt
	if session.Status != "active" || session.UserID != userID {
		return Session{}, ErrSessionNotFound
	}
	if err := tx.Commit(ctx); err != nil {
		return Session{}, fmt.Errorf("提交编辑票据事务失败: %w", err)
	}
	return session, nil
}

func (repository *PGRepository) UpdateWorkStorageKey(ctx context.Context, userID, sessionID, workStorageKey string) error {
	result, err := repository.pool.Exec(ctx, `
		UPDATE edit_sessions
		SET work_storage_key = $3, last_seen_at = now()
		WHERE id = $1::uuid AND user_id = $2::uuid AND status = 'active'`, sessionID, userID, workStorageKey)
	if err != nil {
		return fmt.Errorf("更新编辑工作文件键失败: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrSessionNotFound
	}
	return nil
}

func (repository *PGRepository) Heartbeat(ctx context.Context, userID, sessionID string, now time.Time) error {
	result, err := repository.pool.Exec(ctx, `
		UPDATE edit_sessions
		SET last_seen_at = $3
		WHERE id = $1::uuid AND user_id = $2::uuid AND status = 'active'`, sessionID, userID, now)
	if err != nil {
		return fmt.Errorf("更新编辑会话心跳失败: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrSessionNotFound
	}
	return nil
}

func (repository *PGRepository) Close(ctx context.Context, userID, sessionID string, isAdmin bool, now time.Time) error {
	if isAdmin {
		res, err := repository.pool.Exec(ctx, `
			UPDATE edit_sessions
			SET status = 'closed', closed_at = $2, last_seen_at = $2
			WHERE id = $1::uuid AND status = 'active'`, sessionID, now)
		if err != nil {
			return fmt.Errorf("管理员关闭编辑会话失败: %w", err)
		}
		if res.RowsAffected() == 0 {
			return ErrSessionNotFound
		}
		return nil
	}

	res, err := repository.pool.Exec(ctx, `
		UPDATE edit_sessions
		SET status = 'closed', closed_at = $3, last_seen_at = $3
		WHERE id = $1::uuid AND user_id = $2::uuid AND status = 'active'`, sessionID, userID, now)
	if err != nil {
		return fmt.Errorf("关闭编辑会话失败: %w", err)
	}
	if res.RowsAffected() == 0 {
		return ErrSessionNotFound
	}
	return nil
}

func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}
