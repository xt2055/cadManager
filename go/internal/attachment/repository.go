package attachment

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound = errors.New("attachment not found")
	ErrConflict = errors.New("attachment conflict")
)

type PGRepository struct {
	pool *pgxpool.Pool
}

func NewPGRepository(pool *pgxpool.Pool) *PGRepository {
	return &PGRepository{pool: pool}
}

func (repository *PGRepository) Create(ctx context.Context, input CreateInput, object StorageObject, userID string) (Attachment, error) {
	if repository == nil || repository.pool == nil {
		return Attachment{}, errors.New("数据库连接未配置")
	}
	role := input.Role
	if role == "" {
		role = RoleOther
	}
	version := input.Version
	if version == "" {
		version = "v1.0"
	}
	var id string
	var partID *string
	if strings.TrimSpace(input.PartNo) != "" {
		err := repository.pool.QueryRow(ctx, `
			SELECT id::text
			FROM structure_parts
			WHERE drawing_id = (SELECT id FROM drawings WHERE drawing_no = $1)
			  AND part_no = $2`, input.DrawingNo, input.PartNo).Scan(&partID)
		if errors.Is(err, pgx.ErrNoRows) {
			return Attachment{}, ErrNotFound
		}
		if err != nil {
			return Attachment{}, fmt.Errorf("查询附件所属零件失败: %w", err)
		}
	}
	var err error
	if partID == nil {
		err = repository.pool.QueryRow(ctx, `
			INSERT INTO attachments (drawing_id, file_role, storage_key, original_name, mime_type, size_bytes, sha256, version, previewable, uploaded_by)
			SELECT id, $2, $3, $4, $5, $6, $7, $8, $9, $10::uuid
			FROM drawings WHERE drawing_no = $1
			RETURNING id::text`, input.DrawingNo, role, object.Key, input.Name, object.MimeType, object.Size, object.SHA256, version, input.Previewable, userID).Scan(&id)
	} else {
		err = repository.pool.QueryRow(ctx, `
			INSERT INTO attachments (part_id, file_role, storage_key, original_name, mime_type, size_bytes, sha256, version, previewable, uploaded_by)
			VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10::uuid)
			RETURNING id::text`, *partID, role, object.Key, input.Name, object.MimeType, object.Size, object.SHA256, version, input.Previewable, userID).Scan(&id)
	}
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key value") || strings.Contains(err.Error(), "unique constraint") {
			return Attachment{}, ErrConflict
		}
		return Attachment{}, fmt.Errorf("保存附件元数据失败: %w", err)
	}
	return repository.Find(ctx, object.Key)
}

func (repository *PGRepository) Find(ctx context.Context, storageKey string) (Attachment, error) {
	var item Attachment
	var role string
	var drawingNo string
	var partNo *string
	var uploadedBy *string
	err := repository.pool.QueryRow(ctx, `
		SELECT a.id::text, a.storage_key, a.original_name, COALESCE(d.drawing_no, parent.drawing_no, ''),
		       p.part_no, a.file_role, a.size_bytes, a.mime_type, COALESCE(a.sha256, ''), a.version,
		       a.previewable, COALESCE(a.uploaded_by::text, '')
		FROM attachments a
		LEFT JOIN drawings d ON d.id = a.drawing_id
		LEFT JOIN structure_parts p ON p.id = a.part_id
		LEFT JOIN drawings parent ON parent.id = p.drawing_id
		WHERE a.storage_key = $1 AND a.deleted_at IS NULL`, storageKey).Scan(
		&item.ID, &item.StorageKey, &item.Name, &drawingNo, &partNo, &role, &item.Size, &item.MimeType,
		&item.SHA256, &item.Version, &item.Previewable, &uploadedBy,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Attachment{}, ErrNotFound
	}
	if err != nil {
		return Attachment{}, fmt.Errorf("查询附件元数据失败: %w", err)
	}
	item.DrawingNo = drawingNo
	item.PartNo = partNo
	item.Role = Role(role)
	if uploadedBy != nil {
		item.UploadedBy = *uploadedBy
	}
	return item, nil
}

func (repository *PGRepository) Delete(ctx context.Context, storageKey string, userID string) error {
	result, err := repository.pool.Exec(ctx, `
		UPDATE attachments
		SET deleted_at = now()
		WHERE storage_key = $1 AND deleted_at IS NULL`, storageKey)
	if err != nil {
		return fmt.Errorf("删除附件元数据失败: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (repository *PGRepository) FolderForDrawing(ctx context.Context, drawingNo string) (string, error) {
	var name string
	err := repository.pool.QueryRow(ctx, `SELECT name FROM drawings WHERE drawing_no = $1`, drawingNo).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("查询图号目录失败: %w", err)
	}
	return drawingNo + "(" + name + ")", nil
}
