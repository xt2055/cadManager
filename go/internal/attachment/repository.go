package attachment

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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
	var createdAt time.Time
	err := repository.pool.QueryRow(ctx, `
		SELECT a.id::text, a.storage_key, a.original_name, COALESCE(d.drawing_no, parent.drawing_no, ''),
		       p.part_no, a.file_role, a.size_bytes, a.mime_type, COALESCE(a.sha256, ''), a.version,
		       a.previewable, COALESCE(a.uploaded_by::text, ''), a.created_at
		FROM attachments a
		LEFT JOIN drawings d ON d.id = a.drawing_id
		LEFT JOIN structure_parts p ON p.id = a.part_id
		LEFT JOIN drawings parent ON parent.id = p.drawing_id
		WHERE a.storage_key = $1 AND a.deleted_at IS NULL`, storageKey).Scan(
		&item.ID, &item.StorageKey, &item.Name, &drawingNo, &partNo, &role, &item.Size, &item.MimeType,
		&item.SHA256, &item.Version, &item.Previewable, &uploadedBy, &createdAt,
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
	item.CreatedAt = createdAt.Format("2006-01-02 15:04:05")
	return item, nil
}

func (repository *PGRepository) FindByOwnerAndName(ctx context.Context, drawingNo, partNo, name string) (Attachment, error) {
	var storageKey string
	err := repository.pool.QueryRow(ctx, `
		SELECT a.storage_key
		FROM attachments a
		LEFT JOIN drawings d ON d.id = a.drawing_id
		LEFT JOIN structure_parts p ON p.id = a.part_id
		LEFT JOIN drawings parent ON parent.id = p.drawing_id
		WHERE a.original_name = $3
		  AND a.deleted_at IS NULL
		  AND COALESCE(d.drawing_no, parent.drawing_no, '') = $1
		  AND ($2 = '' OR p.part_no = $2)
		ORDER BY a.created_at DESC
		LIMIT 1`, drawingNo, partNo, name).Scan(&storageKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return Attachment{}, ErrNotFound
	}
	if err != nil {
		return Attachment{}, fmt.Errorf("按图号和文件名查询附件失败: %w", err)
	}
	return repository.Find(ctx, storageKey)
}

func (repository *PGRepository) ListByDrawing(ctx context.Context, drawingNo string) ([]Attachment, error) {
	rows, err := repository.pool.Query(ctx, `
		SELECT a.id::text, a.storage_key, a.original_name,
		       COALESCE(d.drawing_no, parent.drawing_no, ''), p.part_no, a.file_role,
		       a.size_bytes, a.mime_type, COALESCE(a.sha256, ''), a.version,
		       a.previewable, COALESCE(a.uploaded_by::text, ''), a.created_at
		FROM attachments a
		LEFT JOIN drawings d ON d.id = a.drawing_id
		LEFT JOIN structure_parts p ON p.id = a.part_id
		LEFT JOIN drawings parent ON parent.id = p.drawing_id
		WHERE COALESCE(d.drawing_no, parent.drawing_no, '') = $1
		  AND a.deleted_at IS NULL
		ORDER BY
		  CASE WHEN a.file_role = 'assembly' THEN 0 ELSE 1 END,
		  CASE WHEN lower(a.original_name) LIKE '%.exb' THEN 0 ELSE 1 END,
		  a.created_at DESC`, drawingNo)
	if err != nil {
		return nil, fmt.Errorf("查询图纸附件失败: %w", err)
	}
	defer rows.Close()

	list := make([]Attachment, 0)
	for rows.Next() {
		var item Attachment
		var role string
		var drawingNoValue string
		var partNo *string
		var uploadedBy *string
		var createdAt time.Time
		if err := rows.Scan(
			&item.ID, &item.StorageKey, &item.Name, &drawingNoValue, &partNo, &role,
			&item.Size, &item.MimeType, &item.SHA256, &item.Version, &item.Previewable,
			&uploadedBy, &createdAt,
		); err != nil {
			return nil, fmt.Errorf("读取图纸附件失败: %w", err)
		}
		item.DrawingNo = drawingNoValue
		item.PartNo = partNo
		item.Role = Role(role)
		if uploadedBy != nil {
			item.UploadedBy = *uploadedBy
		}
		item.CreatedAt = createdAt.Format("2006-01-02 15:04:05")
		list = append(list, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("读取图纸附件失败: %w", err)
	}
	return list, nil
}

func (repository *PGRepository) ListAllCad(ctx context.Context) ([]Attachment, error) {
	if repository == nil || repository.pool == nil {
		return nil, nil
	}
	rows, err := repository.pool.Query(ctx, `
		SELECT a.id::text, a.storage_key, a.original_name, COALESCE(d.drawing_no, parent.drawing_no, ''),
		       p.part_no, a.file_role, a.size_bytes, a.mime_type, COALESCE(a.sha256, ''), a.version,
		       a.previewable, COALESCE(a.uploaded_by::text, '')
		FROM attachments a
		LEFT JOIN drawings d ON d.id = a.drawing_id
		LEFT JOIN structure_parts p ON p.id = a.part_id
		LEFT JOIN drawings parent ON parent.id = p.drawing_id
		WHERE (a.storage_key ILIKE '%.exb' OR a.original_name ILIKE '%.exb'
		    OR a.storage_key ILIKE '%.dwg' OR a.original_name ILIKE '%.dwg')
		  AND a.deleted_at IS NULL
		ORDER BY a.created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("查询所有 CAD 附件失败: %w", err)
	}
	defer rows.Close()

	var list []Attachment
	for rows.Next() {
		var item Attachment
		var role string
		var drawingNo string
		var partNo *string
		var uploadedBy *string
		if err := rows.Scan(
			&item.ID, &item.StorageKey, &item.Name, &drawingNo, &partNo, &role, &item.Size, &item.MimeType,
			&item.SHA256, &item.Version, &item.Previewable, &uploadedBy,
		); err != nil {
			return nil, err
		}
		item.DrawingNo = drawingNo
		item.PartNo = partNo
		item.Role = Role(role)
		if uploadedBy != nil {
			item.UploadedBy = *uploadedBy
		}
		list = append(list, item)
	}
	return list, nil
}

func (repository *PGRepository) ListAllExb(ctx context.Context) ([]Attachment, error) {
	return repository.ListAllCad(ctx)
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
