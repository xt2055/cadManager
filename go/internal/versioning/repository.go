package versioning

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("文件版本不存在")

type PGRepository struct {
	pool *pgxpool.Pool
}

func NewPGRepository(pool *pgxpool.Pool) *PGRepository {
	return &PGRepository{pool: pool}
}

func (repository *PGRepository) Create(ctx context.Context, input CreateInput, storageKey string) (Version, error) {
	var id string
	err := repository.pool.QueryRow(ctx, `
		INSERT INTO file_versions (attachment_id, storage_key, source_storage_key, version, version_kind, size_bytes, mime_type, sha256, created_by, expires_at)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, NULLIF($9, '')::uuid, $10)
		RETURNING id::text`, input.AttachmentID, storageKey, input.SourceStorageKey, input.Version, input.VersionKind,
		input.Size, input.MimeType, input.SHA256, input.CreatedBy, input.ExpiresAt).Scan(&id)
	if err != nil {
		return Version{}, fmt.Errorf("保存文件版本失败: %w", err)
	}
	return repository.find(ctx, id)
}

func (repository *PGRepository) GetByID(ctx context.Context, versionID string) (Version, error) {
	return repository.find(ctx, versionID)
}

// PromoteInitial 将登记的初始版本标记为当前正式版本（不过期、置顶）。
func (repository *PGRepository) PromoteInitial(ctx context.Context, versionID string) error {
	_, err := repository.pool.Exec(ctx, `
		UPDATE file_versions
		SET version_kind = 'release', is_pinned = true, expires_at = NULL, is_current_release = true
		WHERE id = $1::uuid AND deleted_at IS NULL`, versionID)
	if err != nil {
		return fmt.Errorf("登记初始正式版本失败: %w", err)
	}
	return nil
}

func (repository *PGRepository) ListByAttachment(ctx context.Context, attachmentID string) ([]Version, error) {
	rows, err := repository.pool.Query(ctx, versionSelect+` WHERE v.attachment_id = $1::uuid ORDER BY v.created_at DESC`, attachmentID)
	if err != nil {
		return nil, fmt.Errorf("查询文件版本失败: %w", err)
	}
	defer rows.Close()
	versions := make([]Version, 0)
	for rows.Next() {
		item, scanErr := scanVersion(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		versions = append(versions, item)
	}
	return versions, rows.Err()
}

func (repository *PGRepository) LatestByAttachment(ctx context.Context, attachmentID string) (Version, error) {
	return repository.find(ctx, "", attachmentID)
}

func (repository *PGRepository) Retain(ctx context.Context, versionID, userID string) (Version, error) {
	return repository.updateProtection(ctx, versionID, userID, false)
}

func (repository *PGRepository) Release(ctx context.Context, versionID, userID string) (Version, error) {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return Version{}, fmt.Errorf("开始发布文件版本事务失败: %w", err)
	}
	defer tx.Rollback(ctx)

	var attachmentID string
	if err := tx.QueryRow(ctx, `SELECT attachment_id::text FROM file_versions WHERE id = $1::uuid AND deleted_at IS NULL`, versionID).Scan(&attachmentID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Version{}, ErrNotFound
		}
		return Version{}, fmt.Errorf("读取待发布文件版本失败: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE file_versions SET is_current_release = false WHERE attachment_id = $1::uuid AND is_current_release = true`, attachmentID); err != nil {
		return Version{}, fmt.Errorf("取消旧正式版本标记失败: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE file_versions
		SET version_kind = 'release', is_pinned = true, expires_at = NULL,
		    released_by = NULLIF($2, '')::uuid, released_at = now(),
		    pinned_by = NULLIF($2, '')::uuid, pinned_at = now(), is_current_release = true
		WHERE id = $1::uuid AND deleted_at IS NULL`, versionID, userID); err != nil {
		return Version{}, fmt.Errorf("发布文件版本失败: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Version{}, fmt.Errorf("提交文件版本发布事务失败: %w", err)
	}
	return repository.find(ctx, versionID)
}

func (repository *PGRepository) ListExpired(ctx context.Context, now time.Time) ([]Version, error) {
	rows, err := repository.pool.Query(ctx, versionSelect+`
		WHERE v.version_kind = 'working' AND v.is_pinned = false AND v.deleted_at IS NULL
		  AND v.expires_at IS NOT NULL AND v.expires_at < $1
		ORDER BY v.expires_at`, now)
	if err != nil {
		return nil, fmt.Errorf("查询过期文件版本失败: %w", err)
	}
	defer rows.Close()
	versions := make([]Version, 0)
	for rows.Next() {
		item, scanErr := scanVersion(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		versions = append(versions, item)
	}
	return versions, rows.Err()
}

func (repository *PGRepository) MarkDeleted(ctx context.Context, versionID string) error {
	result, err := repository.pool.Exec(ctx, `UPDATE file_versions SET deleted_at = now() WHERE id = $1::uuid AND deleted_at IS NULL`, versionID)
	if err != nil {
		return fmt.Errorf("标记文件版本已清理失败: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (repository *PGRepository) find(ctx context.Context, versionID string, attachmentID ...string) (Version, error) {
	query := versionSelect + ` WHERE v.id = $1::uuid`
	args := []any{versionID}
	if versionID == "" {
		query = versionSelect + ` WHERE v.attachment_id = $1::uuid AND v.deleted_at IS NULL ORDER BY (v.version_kind = 'working') DESC, v.created_at DESC LIMIT 1`
		args = []any{attachmentID[0]}
	}
	row := repository.pool.QueryRow(ctx, query, args...)
	item, err := scanVersion(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Version{}, ErrNotFound
	}
	return item, err
}

func (repository *PGRepository) updateProtection(ctx context.Context, versionID, userID string, release bool) (Version, error) {
	var query string
	if release {
		query = `UPDATE file_versions SET version_kind = 'release', is_pinned = true, expires_at = NULL, released_by = NULLIF($2, '')::uuid, released_at = now(), pinned_by = NULLIF($2, '')::uuid, pinned_at = now() WHERE id = $1::uuid AND deleted_at IS NULL`
	} else {
		query = `UPDATE file_versions SET is_pinned = true, expires_at = NULL, pinned_by = NULLIF($2, '')::uuid, pinned_at = now() WHERE id = $1::uuid AND deleted_at IS NULL`
	}
	result, err := repository.pool.Exec(ctx, query, versionID, userID)
	if err != nil {
		return Version{}, fmt.Errorf("更新文件版本保护状态失败: %w", err)
	}
	if result.RowsAffected() == 0 {
		return Version{}, ErrNotFound
	}
	return repository.find(ctx, versionID)
}

const versionSelect = `
	SELECT v.id::text, v.attachment_id::text, v.storage_key, v.source_storage_key, v.version, v.version_kind,
	       v.size_bytes, v.mime_type, v.sha256, COALESCE(v.created_by::text, ''),
	       COALESCE(cu.display_name, cu.account, ''), v.created_at, v.expires_at,
	       v.is_pinned, COALESCE(v.pinned_by::text, ''), v.pinned_at, COALESCE(v.released_by::text, ''),
	       v.released_at, v.is_current_release, v.deleted_at
	FROM file_versions v
	LEFT JOIN users cu ON cu.id = v.created_by`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanVersion(row rowScanner) (Version, error) {
	var item Version
	err := row.Scan(&item.ID, &item.AttachmentID, &item.StorageKey, &item.SourceStorageKey, &item.Version,
		&item.VersionKind, &item.Size, &item.MimeType, &item.SHA256, &item.CreatedBy, &item.CreatedByName,
		&item.CreatedAt, &item.ExpiresAt, &item.IsPinned, &item.PinnedBy, &item.PinnedAt, &item.ReleasedBy,
		&item.ReleasedAt, &item.IsCurrentRelease, &item.DeletedAt)
	return item, err
}
