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

type PGRepository struct{ pool *pgxpool.Pool }

func NewPGRepository(pool *pgxpool.Pool) *PGRepository { return &PGRepository{pool: pool} }

func (repository *PGRepository) Create(ctx context.Context, input CreateInput, storageKey string) (Version, error) {
	var id string
		err := repository.pool.QueryRow(ctx, `WITH blob AS (INSERT INTO file_blobs (sha256, size_bytes, mime_type, storage_key) VALUES ($9, $6, COALESCE(NULLIF($5, ''), 'application/octet-stream'), $3) ON CONFLICT (sha256, size_bytes) DO UPDATE SET mime_type = EXCLUDED.mime_type RETURNING id) INSERT INTO attachment_versions (attachment_id, version, blob_id, original_name, mime_type, size_bytes, version_kind, created_by) VALUES ($1::uuid, $2, (SELECT id FROM blob), COALESCE(NULLIF($4, ''), '未命名文件'), COALESCE(NULLIF($5, ''), 'application/octet-stream'), $6, COALESCE(NULLIF($7, ''), 'working'), NULLIF($8, '')::uuid) RETURNING id::text`, input.AttachmentID, input.Version, storageKey, input.CurrentName, input.MimeType, input.Size, input.VersionKind, input.CreatedBy, input.SHA256).Scan(&id)
		if err != nil {
			return Version{}, fmt.Errorf("保存文件版本失败: %w", err)
		}
		if _, err := repository.pool.Exec(ctx, `UPDATE attachments SET original_version_id = COALESCE(original_version_id, $2::uuid) WHERE id = $1::uuid`, input.AttachmentID, id); err != nil {
			return Version{}, fmt.Errorf("记录原始文件版本失败: %w", err)
		}
		return repository.find(ctx, id)
	}

func (repository *PGRepository) GetByID(ctx context.Context, versionID string) (Version, error) {
	return repository.find(ctx, versionID)
}
func (repository *PGRepository) GetByStorageKey(ctx context.Context, storageKey string) (Version, error) {
	item, err := scanVersion(repository.pool.QueryRow(ctx, versionSelect+` WHERE b.storage_key = $1 AND v.deleted_at IS NULL LIMIT 1`, storageKey))
	if errors.Is(err, pgx.ErrNoRows) {
		return Version{}, ErrNotFound
	}
	return item, err
}

func (repository *PGRepository) CreateWithPromotion(ctx context.Context, input CreateInput, storageKey string) (Version, error) {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return Version{}, fmt.Errorf("开始版本登记事务失败: %w", err)
	}
	defer tx.Rollback(ctx)
	var attachmentID string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM attachments WHERE id = $1::uuid AND deleted_at IS NULL FOR UPDATE`, input.AttachmentID).Scan(&attachmentID); errors.Is(err, pgx.ErrNoRows) {
		return Version{}, ErrNotFound
	} else if err != nil {
		return Version{}, err
	}
	var id string
	err = tx.QueryRow(ctx, `WITH blob AS (INSERT INTO file_blobs (sha256, size_bytes, mime_type, storage_key) VALUES ($9, $6, COALESCE(NULLIF($5, ''), 'application/octet-stream'), $3) ON CONFLICT (sha256, size_bytes) DO UPDATE SET mime_type = EXCLUDED.mime_type RETURNING id) INSERT INTO attachment_versions (attachment_id, version, blob_id, original_name, mime_type, size_bytes, version_kind, created_by) VALUES ($1::uuid, $2, (SELECT id FROM blob), COALESCE(NULLIF($4, ''), '未命名文件'), COALESCE(NULLIF($5, ''), 'application/octet-stream'), $6, COALESCE(NULLIF($7, ''), 'working'), NULLIF($8, '')::uuid) RETURNING id::text`, attachmentID, input.Version, storageKey, input.CurrentName, input.MimeType, input.Size, input.VersionKind, input.CreatedBy, input.SHA256).Scan(&id)
		if err != nil {
			return Version{}, fmt.Errorf("保存文件版本失败: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE attachments SET current_version_id = $2::uuid, revision = revision + 1, original_version_id = COALESCE(original_version_id, $2::uuid) WHERE id = $1::uuid`, attachmentID, id); err != nil {
			return Version{}, fmt.Errorf("切换附件当前版本失败: %w", err)
		}
	if err := tx.Commit(ctx); err != nil {
		return Version{}, fmt.Errorf("提交版本登记事务失败: %w", err)
	}
	return repository.find(ctx, id)
}

func (repository *PGRepository) PromoteInitial(ctx context.Context, versionID string) error {
	result, err := repository.pool.Exec(ctx, `UPDATE attachment_versions SET version_kind = 'release' WHERE id = $1::uuid AND deleted_at IS NULL`, versionID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
func (repository *PGRepository) ListByAttachment(ctx context.Context, attachmentID string) ([]Version, error) {
		return repository.list(ctx, versionSelect+` WHERE v.attachment_id = $1::uuid AND v.deleted_at IS NULL AND (v.release_number IS NOT NULL OR a.original_version_id = v.id) ORDER BY CASE WHEN v.release_number IS NULL THEN 0 ELSE v.release_number END, v.created_at, v.id`, attachmentID)
	}
func (repository *PGRepository) LatestByAttachment(ctx context.Context, attachmentID string) (Version, error) {
	item, err := scanVersion(repository.pool.QueryRow(ctx, versionSelect+` WHERE v.attachment_id = $1::uuid AND v.deleted_at IS NULL ORDER BY v.created_at DESC LIMIT 1`, attachmentID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Version{}, ErrNotFound
	}
	return item, err
}
func (repository *PGRepository) Retain(ctx context.Context, versionID, _ string) (Version, error) {
	return repository.find(ctx, versionID)
}
func (repository *PGRepository) Release(ctx context.Context, versionID, _ string) (Version, error) {
	result, err := repository.pool.Exec(ctx, `UPDATE attachment_versions SET version_kind = 'release' WHERE id = $1::uuid AND deleted_at IS NULL`, versionID)
	if err != nil {
		return Version{}, err
	}
	if result.RowsAffected() == 0 {
		return Version{}, ErrNotFound
	}
	return repository.find(ctx, versionID)
}
func (repository *PGRepository) ListExpired(ctx context.Context, _ time.Time) ([]Version, error) {
	return []Version{}, nil
}
func (repository *PGRepository) MarkDeleted(ctx context.Context, versionID string) error {
	result, err := repository.pool.Exec(ctx, `UPDATE attachment_versions SET deleted_at = now() WHERE id = $1::uuid AND deleted_at IS NULL`, versionID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (repository *PGRepository) find(ctx context.Context, versionID string) (Version, error) {
	item, err := scanVersion(repository.pool.QueryRow(ctx, versionSelect+` WHERE v.id = $1::uuid`, versionID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Version{}, ErrNotFound
	}
	return item, err
}
func (repository *PGRepository) list(ctx context.Context, query string, args ...any) ([]Version, error) {
	rows, err := repository.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Version, 0)
	for rows.Next() {
		item, err := scanVersion(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

const versionSelect = `SELECT v.id::text, v.attachment_id::text, COALESCE(b.storage_key, ''), '', v.version, v.version_kind, v.size_bytes, v.mime_type, COALESCE(b.sha256, ''), COALESCE(v.created_by::text, ''), COALESCE(u.display_name, u.account, ''), v.created_at, NULL::timestamptz, false, '', NULL::timestamptz, '', NULL::timestamptz, (a.current_version_id = v.id), v.deleted_at, v.original_name, v.release_number, COALESCE(a.original_version_id = v.id, false) FROM attachment_versions v JOIN attachments a ON a.id = v.attachment_id LEFT JOIN file_blobs b ON b.id = v.blob_id LEFT JOIN users u ON u.id = v.created_by`

type rowScanner interface{ Scan(...any) error }

func scanVersion(row rowScanner) (Version, error) {
	var item Version
	err := row.Scan(&item.ID, &item.AttachmentID, &item.StorageKey, &item.SourceStorageKey, &item.Version, &item.VersionKind, &item.Size, &item.MimeType, &item.SHA256, &item.CreatedBy, &item.CreatedByName, &item.CreatedAt, &item.ExpiresAt, &item.IsPinned, &item.PinnedBy, &item.PinnedAt, &item.ReleasedBy, &item.ReleasedAt, &item.IsCurrentRelease, &item.DeletedAt, &item.OriginalName, &item.ReleaseNumber, &item.IsOriginal)
	return item, err
}
