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
			INSERT INTO attachments (drawing_id, file_role, storage_key, current_storage_key, original_name, current_name, mime_type, current_mime_type, size_bytes, current_size_bytes, sha256, current_sha256, version, previewable, uploaded_by)
			SELECT id, $2, $3, $3, $4, $4, $5, $5, $6, $6, $7, $7, $8, $9, $10::uuid
			FROM drawings WHERE drawing_no = $1
			RETURNING id::text`, input.DrawingNo, role, object.Key, input.Name, object.MimeType, object.Size, object.SHA256, version, input.Previewable, userID).Scan(&id)
	} else {
		err = repository.pool.QueryRow(ctx, `
			INSERT INTO attachments (part_id, file_role, storage_key, current_storage_key, original_name, current_name, mime_type, current_mime_type, size_bytes, current_size_bytes, sha256, current_sha256, version, previewable, uploaded_by)
			VALUES ($1::uuid, $2, $3, $3, $4, $4, $5, $5, $6, $6, $7, $7, $8, $9, $10::uuid)
			RETURNING id::text`, *partID, role, object.Key, input.Name, object.MimeType, object.Size, object.SHA256, version, input.Previewable, userID).Scan(&id)
	}
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Attachment{}, ErrNotFound
		}
		if strings.Contains(err.Error(), "duplicate key value") || strings.Contains(err.Error(), "unique constraint") {
			return Attachment{}, ErrConflict
		}
		return Attachment{}, fmt.Errorf("保存附件元数据失败: %w", err)
	}
	return repository.Find(ctx, object.Key)
}

func (repository *PGRepository) Find(ctx context.Context, storageKey string) (Attachment, error) {
	var item Attachment
	var currentStorageKey *string
	var currentName *string
	var currentMimeType *string
	var currentSize *int64
	var currentSHA256 *string
	var role string
	var drawingNo string
	var partNo *string
	var uploadedBy *string
	var createdAt time.Time
	err := repository.pool.QueryRow(ctx, `
		SELECT a.id::text, a.storage_key, a.current_storage_key, a.original_name, a.current_name,
		       a.current_mime_type, a.current_size_bytes, a.current_sha256,
		       COALESCE(d.drawing_no, parent.drawing_no, ''),
		       p.part_no, a.file_role, a.size_bytes, a.mime_type, COALESCE(a.sha256, ''), a.version,
		       a.previewable, COALESCE(a.uploaded_by::text, ''), a.created_at
		FROM attachments a
		LEFT JOIN drawings d ON d.id = a.drawing_id
		LEFT JOIN structure_parts p ON p.id = a.part_id
		LEFT JOIN drawings parent ON parent.id = p.drawing_id
		WHERE (a.storage_key = $1 OR a.current_storage_key = $1) AND a.deleted_at IS NULL`, storageKey).Scan(
		&item.ID, &item.StorageKey, &currentStorageKey, &item.Name, &currentName,
		&currentMimeType, &currentSize, &currentSHA256, &drawingNo, &partNo, &role, &item.Size, &item.MimeType,
		&item.SHA256, &item.Version, &item.Previewable, &uploadedBy, &createdAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Attachment{}, ErrNotFound
	}
	if err != nil {
		return Attachment{}, fmt.Errorf("查询附件元数据失败: %w", err)
	}
	item.DrawingNo = drawingNo
	if currentStorageKey != nil {
		item.CurrentStorageKey = *currentStorageKey
	}
	if currentName != nil {
		item.CurrentName = *currentName
	}
	if currentMimeType != nil {
		item.CurrentMimeType = *currentMimeType
	}
	if currentSize != nil {
		item.CurrentSize = *currentSize
	}
	if currentSHA256 != nil {
		item.CurrentSHA256 = *currentSHA256
	}
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

func (repository *PGRepository) UpdateContent(ctx context.Context, storageKey string, size int64, mimeType, sha256 string) error {
	result, err := repository.pool.Exec(ctx, `
		UPDATE attachments
		SET size_bytes = $2, mime_type = $3, sha256 = $4
		WHERE storage_key = $1 AND deleted_at IS NULL`, storageKey, size, mimeType, sha256)
	if err != nil {
		return fmt.Errorf("更新附件内容元数据失败: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (repository *PGRepository) SetCurrentContent(ctx context.Context, sourceStorageKey, currentStorageKey, name string, size int64, mimeType, sha256 string) error {
	result, err := repository.pool.Exec(ctx, `
		UPDATE attachments
		SET current_storage_key = $2, current_name = $3, current_size_bytes = $4,
		    current_mime_type = $5, current_sha256 = $6
		WHERE storage_key = $1 AND deleted_at IS NULL`, sourceStorageKey, currentStorageKey, name, size, mimeType, sha256)
	if err != nil {
		return fmt.Errorf("更新当前 CAD 文件元数据失败: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (repository *PGRepository) ListByDrawing(ctx context.Context, drawingNo string) ([]Attachment, error) {
	rows, err := repository.pool.Query(ctx, `
		SELECT a.id::text, a.storage_key, COALESCE(a.current_storage_key, a.storage_key),
		       a.original_name, COALESCE(a.current_name, a.original_name),
		       COALESCE(a.current_mime_type, a.mime_type),
		       COALESCE(a.current_size_bytes, a.size_bytes),
		       COALESCE(a.current_sha256, COALESCE(a.sha256, '')),
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
		var currentStorageKey string
		var currentName string
		var currentMimeType string
		var currentSize int64
		var currentSHA256 string
		var role string
		var drawingNoValue string
		var partNo *string
		var uploadedBy *string
		var createdAt time.Time
		if err := rows.Scan(
			&item.ID, &item.StorageKey, &currentStorageKey,
			&item.Name, &currentName, &currentMimeType, &currentSize, &currentSHA256,
			&drawingNoValue, &partNo, &role,
			&item.Size, &item.MimeType, &item.SHA256, &item.Version, &item.Previewable,
			&uploadedBy, &createdAt,
		); err != nil {
			return nil, fmt.Errorf("读取图纸附件失败: %w", err)
		}
		item.CurrentStorageKey = currentStorageKey
		item.CurrentName = currentName
		item.CurrentMimeType = currentMimeType
		item.CurrentSize = currentSize
		item.CurrentSHA256 = currentSHA256
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

func (repository *PGRepository) ReassignPart(ctx context.Context, storageKey, partID string) error {
	result, err := repository.pool.Exec(ctx, `
		UPDATE attachments
		SET drawing_id = NULL, part_id = $2::uuid, file_role = 'part'
		WHERE storage_key = $1 AND deleted_at IS NULL`, storageKey, partID)
	if err != nil {
		return fmt.Errorf("更新附件所属零件失败: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (repository *PGRepository) ReidentifyPart(ctx context.Context, storageKey, partNo, userID string) (ReidentifyResult, error) {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return ReidentifyResult{}, fmt.Errorf("开始校正附件图号事务失败: %w", err)
	}
	defer tx.Rollback(ctx)

	var result ReidentifyResult
	var currentPartID *string
	var drawingID string
	err = tx.QueryRow(ctx, `
		SELECT a.part_id::text, COALESCE(a.drawing_id, p.drawing_id)::text,
		       COALESCE(parent.drawing_no, d.drawing_no, ''), COALESCE(p.part_no, '')
		FROM attachments a
		LEFT JOIN drawings d ON d.id = a.drawing_id
		LEFT JOIN structure_parts p ON p.id = a.part_id
		LEFT JOIN drawings parent ON parent.id = p.drawing_id
		WHERE a.storage_key = $1 AND a.deleted_at IS NULL`, storageKey).
		Scan(&currentPartID, &drawingID, &result.DrawingNo, &result.OldPartNo)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReidentifyResult{}, ErrNotFound
	}
	if err != nil {
		return ReidentifyResult{}, fmt.Errorf("读取待校正附件失败: %w", err)
	}
	result.StorageKey = storageKey
	result.PartNo = strings.TrimSpace(partNo)
	if result.PartNo == "" {
		return ReidentifyResult{}, errors.New("新零件图号不能为空")
	}
	if result.OldPartNo == result.PartNo && currentPartID != nil {
		return result, nil
	}

	var targetPartID *string
	err = tx.QueryRow(ctx, `
		SELECT id::text FROM structure_parts
		WHERE drawing_id = $1::uuid AND part_no = $2`, drawingID, result.PartNo).Scan(&targetPartID)
	if errors.Is(err, pgx.ErrNoRows) {
		targetPartID = nil
	} else if err != nil {
		return ReidentifyResult{}, fmt.Errorf("查询目标零件失败: %w", err)
	}

	if targetPartID != nil && (currentPartID == nil || *targetPartID != *currentPartID) {
		_, err = tx.Exec(ctx, `
			UPDATE attachments
			SET drawing_id = NULL, part_id = $2::uuid, file_role = 'part'
			WHERE storage_key = $1 AND deleted_at IS NULL`, storageKey, *targetPartID)
	} else if currentPartID != nil {
		_, err = tx.Exec(ctx, `
			UPDATE structure_parts
			SET part_no = $2, updated_by = $3::uuid, updated_at = now()
			WHERE id = $1::uuid`, *currentPartID, result.PartNo, userID)
	} else {
		parentPartNo := directParentPartNo(result.PartNo)
		var parentPartID *string
		if parentPartNo != "" {
			_ = tx.QueryRow(ctx, `
				SELECT id::text FROM structure_parts
				WHERE drawing_id = $1::uuid AND part_no = $2`, drawingID, parentPartNo).Scan(&parentPartID)
		}
		var createdPartID string
		err = tx.QueryRow(ctx, `
			INSERT INTO structure_parts (drawing_id, parent_part_id, part_no, name, project, material, status, version, created_by, updated_by)
			SELECT d.id, $2::uuid, $3, $3, d.project, d.material, d.status, d.version, $4::uuid, $4::uuid
			FROM drawings d WHERE d.id = $1::uuid
			RETURNING id::text`, drawingID, parentPartID, result.PartNo, userID).Scan(&createdPartID)
		if err == nil {
			_, err = tx.Exec(ctx, `
				UPDATE attachments
				SET drawing_id = NULL, part_id = $2::uuid, file_role = 'part'
				WHERE storage_key = $1 AND deleted_at IS NULL`, storageKey, createdPartID)
		}
	}
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key value") || strings.Contains(err.Error(), "unique constraint") {
			return ReidentifyResult{}, ErrConflict
		}
		return ReidentifyResult{}, fmt.Errorf("更新历史零件图号失败: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return ReidentifyResult{}, fmt.Errorf("提交历史零件图号校正失败: %w", err)
	}
	return result, nil
}

func directParentPartNo(partNo string) string {
	index := strings.LastIndex(partNo, "-")
	if index <= 0 {
		return ""
	}
	parent := partNo[:index]
	for _, value := range partNo[index+1:] {
		if value < '0' || value > '9' {
			return ""
		}
	}
	return parent
}

func (repository *PGRepository) FolderForDrawing(ctx context.Context, drawingNo string) (string, error) {
	var project string
	var name string
	err := repository.pool.QueryRow(ctx, `SELECT project, name FROM drawings WHERE drawing_no = $1`, drawingNo).Scan(&project, &name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("查询图号目录失败: %w", err)
	}
	if strings.TrimSpace(project) == "" {
		return "", errors.New("图纸项目号为空，无法生成附件目录")
	}
	return project + "(" + name + ")", nil
}
