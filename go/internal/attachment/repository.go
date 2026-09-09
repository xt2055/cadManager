package attachment

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/text/unicode/norm"
)

var (
	ErrNotFound = errors.New("attachment not found")
	ErrConflict = errors.New("attachment conflict")
)

type PGRepository struct{ pool *pgxpool.Pool }

func NewPGRepository(pool *pgxpool.Pool) *PGRepository { return &PGRepository{pool: pool} }

func normalizePartNo(value string) string {
	return strings.ToUpper(strings.TrimSpace(norm.NFKC.String(value)))
}

func (repository *PGRepository) Create(ctx context.Context, input CreateInput, object StorageObject, userID string) (Attachment, error) {
	if repository == nil || repository.pool == nil {
		return Attachment{}, errors.New("数据库连接未配置")
	}
	if strings.TrimSpace(input.DrawingNo) == "" || strings.TrimSpace(input.Name) == "" {
		return Attachment{}, errors.New("附件图纸和文件名不能为空")
	}
	role, version := input.Role, input.Version
	if role == "" {
		role = RoleOther
	}
	if version == "" {
		version = "v1.0"
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return Attachment{}, fmt.Errorf("开始创建附件事务失败: %w", err)
	}
	defer tx.Rollback(ctx)
	var drawingID string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM drawings WHERE drawing_no = $1`, input.DrawingNo).Scan(&drawingID); errors.Is(err, pgx.ErrNoRows) {
		return Attachment{}, ErrNotFound
	} else if err != nil {
		return Attachment{}, err
	}
	var partID string
	if strings.TrimSpace(input.PartNo) != "" {
		if err := tx.QueryRow(ctx, `SELECT p.id::text FROM parts p WHERE p.normalized_part_no = $1`, normalizePartNo(input.PartNo)).Scan(&partID); errors.Is(err, pgx.ErrNoRows) {
			return Attachment{}, ErrNotFound
		} else if err != nil {
			return Attachment{}, err
		}
	}
	blobID, storedKey, err := ensureBlobTx(ctx, tx, object)
	if err != nil {
		return Attachment{}, err
	}
	var attachmentID string
	if partID == "" {
		err = tx.QueryRow(ctx, `INSERT INTO attachments (drawing_id, file_role, logical_name, uploaded_by) VALUES ($1::uuid, $2, $3, $4::uuid) RETURNING id::text`, drawingID, role, input.Name, userID).Scan(&attachmentID)
	} else {
		err = tx.QueryRow(ctx, `INSERT INTO attachments (part_id, file_role, logical_name, uploaded_by) VALUES ($1::uuid, $2, $3, $4::uuid) RETURNING id::text`, partID, role, input.Name, userID).Scan(&attachmentID)
	}
	if err != nil {
		if isUnique(err) {
			return Attachment{}, ErrConflict
		}
		return Attachment{}, fmt.Errorf("保存附件失败: %w", err)
	}
	var versionID string
	if err := tx.QueryRow(ctx, `INSERT INTO attachment_versions (attachment_id, version, blob_id, original_name, mime_type, size_bytes, previewable, version_kind, created_by) VALUES ($1::uuid, $2, $3::uuid, $4, $5, $6, $7, 'release', $8::uuid) RETURNING id::text`, attachmentID, version, blobID, input.Name, object.MimeType, object.Size, input.Previewable, userID).Scan(&versionID); err != nil {
		return Attachment{}, fmt.Errorf("保存附件版本失败: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE attachments SET current_version_id = $2::uuid WHERE id = $1::uuid`, attachmentID, versionID); err != nil {
		return Attachment{}, fmt.Errorf("设置附件当前版本失败: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Attachment{}, fmt.Errorf("提交附件事务失败: %w", err)
	}
	return repository.findByID(ctx, attachmentID, storedKey)
}

func ensureBlobTx(ctx context.Context, tx pgx.Tx, object StorageObject) (string, string, error) {
	hash := strings.ToLower(strings.TrimSpace(object.SHA256))
	if len(hash) != 64 {
		return "", "", errors.New("附件缺少有效 SHA-256")
	}
	if object.Size < 0 || strings.TrimSpace(object.Key) == "" {
		return "", "", errors.New("附件对象元数据无效")
	}
	var id, key string
	err := tx.QueryRow(ctx, `INSERT INTO file_blobs (sha256, size_bytes, mime_type, storage_key) VALUES ($1, $2, COALESCE(NULLIF($3, ''), 'application/octet-stream'), $4) ON CONFLICT (sha256, size_bytes) DO UPDATE SET mime_type = EXCLUDED.mime_type RETURNING id::text, storage_key`, hash, object.Size, object.MimeType, object.Key).Scan(&id, &key)
	if err != nil {
		return "", "", fmt.Errorf("保存内容对象失败: %w", err)
	}
	return id, key, nil
}

const attachmentSelect = `SELECT a.id::text, COALESCE(d.drawing_no, parent.drawing_no, ''), p.part_no, a.file_role, a.logical_name, COALESCE(v.original_name, a.logical_name), COALESCE(b.storage_key, ''), COALESCE(b.storage_key, ''), COALESCE(v.mime_type, b.mime_type, 'application/octet-stream'), COALESCE(v.size_bytes, b.size_bytes, 0), COALESCE(v.size_bytes, b.size_bytes, 0), COALESCE(b.sha256, ''), COALESCE(b.sha256, ''), COALESCE(v.version, 'v1.0'), COALESCE(v.previewable, false), COALESCE(a.uploaded_by::text, ''), COALESCE(NULLIF(u.display_name, ''), u.account, ''), a.created_at, a.revision, a.current_version_id::text, COALESCE(a.author, '') FROM attachments a LEFT JOIN attachment_versions v ON v.id = a.current_version_id LEFT JOIN file_blobs b ON b.id = v.blob_id LEFT JOIN drawings d ON d.id = a.drawing_id LEFT JOIN parts p ON p.id = a.part_id LEFT JOIN drawing_part_relations owner_relation ON owner_relation.part_id = p.id AND owner_relation.relation_type = 'owned' AND owner_relation.status = 'active' LEFT JOIN drawings parent ON parent.id = owner_relation.drawing_id LEFT JOIN users u ON u.id = a.uploaded_by`

func (repository *PGRepository) findByID(ctx context.Context, id, key string) (Attachment, error) {
	query := attachmentSelect + ` WHERE a.id = $1::uuid AND a.deleted_at IS NULL`
	args := []any{id}
	if key != "" {
		query = attachmentSelect + ` WHERE a.id = $1::uuid AND a.deleted_at IS NULL AND EXISTS (SELECT 1 FROM attachment_versions av JOIN file_blobs fb ON fb.id = av.blob_id WHERE av.attachment_id = a.id AND fb.storage_key = $2)`
		args = append(args, key)
	}
	item, err := scanAttachment(repository.pool.QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return Attachment{}, ErrNotFound
	}
	return item, err
}

func (repository *PGRepository) FindByID(ctx context.Context, attachmentID string) (Attachment, error) {
	return repository.findByID(ctx, attachmentID, "")
}

func (repository *PGRepository) UpdateAuthorByID(ctx context.Context, attachmentID, author string) error {
	result, err := repository.pool.Exec(ctx, `UPDATE attachments SET author = $2 WHERE id = $1::uuid AND deleted_at IS NULL`, attachmentID, strings.TrimSpace(author))
	if err != nil {
		return fmt.Errorf("更新附件编制人失败: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (repository *PGRepository) Find(ctx context.Context, storageKey string) (Attachment, error) {
	item, err := scanAttachment(repository.pool.QueryRow(ctx, attachmentSelect+` WHERE a.deleted_at IS NULL AND EXISTS (SELECT 1 FROM attachment_versions av JOIN file_blobs fb ON fb.id = av.blob_id WHERE av.attachment_id = a.id AND fb.storage_key = $1) LIMIT 1`, storageKey))
	if errors.Is(err, pgx.ErrNoRows) {
		return Attachment{}, ErrNotFound
	}
	if err != nil {
		return Attachment{}, fmt.Errorf("查询附件元数据失败: %w", err)
	}
	return item, nil
}

func scanAttachment(row interface{ Scan(...any) error }) (Attachment, error) {
	var item Attachment
	var partNo *string
	var createdAt time.Time
	var revision int64
	err := row.Scan(&item.ID, &item.DrawingNo, &partNo, &item.Role, &item.Name, &item.CurrentName, &item.StorageKey, &item.CurrentStorageKey, &item.CurrentMimeType, &item.Size, &item.CurrentSize, &item.SHA256, &item.CurrentSHA256, &item.Version, &item.Previewable, &item.UploadedByID, &item.UploadedBy, &createdAt, &revision, &item.CurrentVersionID, &item.Author)
	if err != nil {
		return Attachment{}, err
	}
	item.PartNo = partNo
	item.Revision = revision
	item.MimeType = item.CurrentMimeType
	item.CreatedAt = createdAt.Format("2006-01-02 15:04:05")
	return item, nil
}

func (repository *PGRepository) FindByOwnerAndName(ctx context.Context, drawingNo, partNo, name string) (Attachment, error) {
	item, err := scanAttachment(repository.pool.QueryRow(ctx, attachmentSelect+` WHERE a.deleted_at IS NULL AND (a.logical_name = $3 OR EXISTS (SELECT 1 FROM attachment_versions av WHERE av.attachment_id = a.id AND av.original_name = $3)) AND COALESCE(d.drawing_no, parent.drawing_no, '') = $1 AND ($2 = '' OR p.normalized_part_no = $4) ORDER BY a.created_at DESC LIMIT 1`, drawingNo, partNo, name, normalizePartNo(partNo)))
	if errors.Is(err, pgx.ErrNoRows) {
		return Attachment{}, ErrNotFound
	}
	return item, err
}

func (repository *PGRepository) UpdateContent(ctx context.Context, storageKey string, size int64, mimeType, sha256 string) error {
	item, err := repository.Find(ctx, storageKey)
	if err != nil {
		return err
	}
	_, err = repository.pool.Exec(ctx, `UPDATE attachment_versions SET size_bytes = $2, mime_type = $3 WHERE id = (SELECT current_version_id FROM attachments WHERE id = $1::uuid)`, item.ID, size, mimeType)
	if err != nil {
		return fmt.Errorf("更新附件内容元数据失败: %w", err)
	}
	return nil
}

func (repository *PGRepository) SetCurrentVersion(ctx context.Context, sourceStorageKey, currentStorageKey, name, version string, size int64, mimeType, sha256 string) error {
	item, err := repository.Find(ctx, sourceStorageKey)
	if err != nil {
		return err
	}
	return repository.SetCurrentVersionByID(ctx, item.ID, currentStorageKey, name, version, size, mimeType)
}

// SetCurrentVersionByID switches one known logical attachment to an already
// stored content object. Conversion workers must use the attachment identity:
// a deduplicated source blob may belong to more than one attachment.
func (repository *PGRepository) SetCurrentVersionByID(ctx context.Context, attachmentID, currentStorageKey, name, version string, size int64, mimeType string) error {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var blobID string
	err = tx.QueryRow(ctx, `SELECT id::text FROM file_blobs WHERE storage_key = $1 AND size_bytes = $2`, currentStorageKey, size).Scan(&blobID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("读取转换内容对象失败: %w", err)
	}
	var versionID string
	err = tx.QueryRow(ctx, `INSERT INTO attachment_versions (attachment_id, version, blob_id, original_name, mime_type, size_bytes, version_kind, created_by) VALUES ($1::uuid, $2, $3::uuid, $4, $5, $6, 'working', NULL) ON CONFLICT (attachment_id, version) WHERE deleted_at IS NULL DO UPDATE SET blob_id = EXCLUDED.blob_id, original_name = EXCLUDED.original_name, mime_type = EXCLUDED.mime_type, size_bytes = EXCLUDED.size_bytes RETURNING id::text`, attachmentID, version, blobID, name, mimeType, size).Scan(&versionID)
	if err != nil {
		return fmt.Errorf("保存当前附件版本失败: %w", err)
	}
	if result, err := tx.Exec(ctx, `UPDATE attachments SET current_version_id = $2::uuid, revision = revision + 1 WHERE id = $1::uuid AND deleted_at IS NULL`, attachmentID, versionID); err != nil {
		return err
	} else if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return tx.Commit(ctx)
}

func (repository *PGRepository) ListByDrawing(ctx context.Context, drawingNo string) ([]Attachment, error) {
	return repository.list(ctx, attachmentSelect+` WHERE a.deleted_at IS NULL AND COALESCE(d.drawing_no, parent.drawing_no, '') = $1 ORDER BY CASE WHEN a.file_role = 'assembly' THEN 0 ELSE 1 END, a.created_at DESC`, drawingNo)
}

func (repository *PGRepository) ListAll(ctx context.Context) ([]Attachment, error) {
	return repository.list(ctx, attachmentSelect+` WHERE a.deleted_at IS NULL ORDER BY a.created_at DESC`)
}

func (repository *PGRepository) list(ctx context.Context, query string, args ...any) ([]Attachment, error) {
	rows, err := repository.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Attachment, 0)
	for rows.Next() {
		item, err := scanAttachment(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (repository *PGRepository) ListAllCad(ctx context.Context) ([]Attachment, error) {
	if repository == nil || repository.pool == nil {
		return nil, nil
	}
	return repository.list(ctx, attachmentSelect+` WHERE a.deleted_at IS NULL AND (lower(COALESCE(v.original_name, '')) LIKE '%.exb' OR lower(COALESCE(v.original_name, '')) LIKE '%.dwg' OR lower(a.logical_name) LIKE '%.exb' OR lower(a.logical_name) LIKE '%.dwg') ORDER BY a.created_at DESC`)
}
func (repository *PGRepository) ListAllExb(ctx context.Context) ([]Attachment, error) {
	return repository.ListAllCad(ctx)
}

func (repository *PGRepository) Delete(ctx context.Context, storageKey string, _ string) error {
	var count int
	if err := repository.pool.QueryRow(ctx, `SELECT count(DISTINCT av.attachment_id)
		FROM attachment_versions av JOIN file_blobs b ON b.id = av.blob_id
		JOIN attachments a ON a.id = av.attachment_id
		WHERE b.storage_key = $1 AND a.deleted_at IS NULL`, storageKey).Scan(&count); err != nil {
		return err
	}
	if count > 1 {
		return fmt.Errorf("存储对象被多个附件共享，请使用 attachmentId 删除: %w", ErrConflict)
	}
	result, err := repository.pool.Exec(ctx, `UPDATE attachments SET deleted_at = now(), revision = revision + 1 WHERE id = (SELECT av.attachment_id FROM attachment_versions av JOIN file_blobs b ON b.id = av.blob_id JOIN attachments a ON a.id = av.attachment_id WHERE b.storage_key = $1 AND a.deleted_at IS NULL LIMIT 1)`, storageKey)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (repository *PGRepository) DeleteByID(ctx context.Context, attachmentID, _ string) (Attachment, error) {
	item, err := repository.FindByID(ctx, attachmentID)
	if err != nil {
		return Attachment{}, err
	}
	result, err := repository.pool.Exec(ctx, `UPDATE attachments SET deleted_at = now(), revision = revision + 1 WHERE id = $1::uuid AND deleted_at IS NULL`, attachmentID)
	if err != nil {
		return Attachment{}, err
	}
	if result.RowsAffected() == 0 {
		return Attachment{}, ErrNotFound
	}
	return item, nil
}

func (repository *PGRepository) HasStorageKeyReference(ctx context.Context, storageKey string) (bool, error) {
	var referenced bool
	err := repository.pool.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM attachment_versions av
		JOIN file_blobs b ON b.id = av.blob_id
		JOIN attachments a ON a.id = av.attachment_id
		WHERE b.storage_key = $1 AND a.deleted_at IS NULL
	)`, storageKey).Scan(&referenced)
	return referenced, err
}
func (repository *PGRepository) ReassignPart(ctx context.Context, storageKey, partID string) error {
	result, err := repository.pool.Exec(ctx, `UPDATE attachments SET drawing_id = NULL, part_id = $2::uuid, file_role = 'part', revision = revision + 1 WHERE id IN (SELECT av.attachment_id FROM attachment_versions av JOIN file_blobs b ON b.id = av.blob_id WHERE b.storage_key = $1) AND deleted_at IS NULL`, storageKey, partID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (repository *PGRepository) ReidentifyPart(ctx context.Context, storageKey, partNo, userID string) (ReidentifyResult, error) {
	item, err := repository.Find(ctx, storageKey)
	if err != nil {
		return ReidentifyResult{}, err
	}
	return repository.reidentifyPart(ctx, item, storageKey, partNo, userID)
}

func (repository *PGRepository) ReidentifyPartByID(ctx context.Context, attachmentID, partNo, userID string) (ReidentifyResult, error) {
	item, err := repository.FindByID(ctx, attachmentID)
	if err != nil {
		return ReidentifyResult{}, err
	}
	return repository.reidentifyPart(ctx, item, item.CurrentStorageKey, partNo, userID)
}

func (repository *PGRepository) reidentifyPart(ctx context.Context, item Attachment, storageKey, partNo, userID string) (ReidentifyResult, error) {
	no := strings.TrimSpace(partNo)
	if no == "" {
		return ReidentifyResult{}, errors.New("新零件图号不能为空")
	}
	if item.PartNo != nil && normalizePartNo(*item.PartNo) == normalizePartNo(no) {
		return ReidentifyResult{StorageKey: storageKey, DrawingNo: item.DrawingNo, PartNo: no}, nil
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return ReidentifyResult{}, err
	}
	defer tx.Rollback(ctx)
	var drawingID string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM drawings WHERE drawing_no = $1`, item.DrawingNo).Scan(&drawingID); err != nil {
		return ReidentifyResult{}, err
	}
	var partID string
	err = tx.QueryRow(ctx, `SELECT id::text FROM parts WHERE normalized_part_no = $1`, normalizePartNo(no)).Scan(&partID)
	if errors.Is(err, pgx.ErrNoRows) {
		if err := tx.QueryRow(ctx, `INSERT INTO parts (part_no, normalized_part_no, created_by, updated_by) VALUES ($1, $2, $3::uuid, $3::uuid) RETURNING id::text`, no, normalizePartNo(no), userID).Scan(&partID); err != nil {
			return ReidentifyResult{}, err
		}
		var revisionID string
		if err := tx.QueryRow(ctx, `INSERT INTO part_revisions (part_id, revision_no, name, workflow_status, created_by) VALUES ($1::uuid, 1, $2, 'draft', $3::uuid) RETURNING id::text`, partID, no, userID).Scan(&revisionID); err != nil {
			return ReidentifyResult{}, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO drawing_part_relations (drawing_id, part_id, relation_type, created_by, updated_by) VALUES ($1::uuid, $2::uuid, 'owned', $3::uuid, $3::uuid)`, drawingID, partID, userID); err != nil {
			return ReidentifyResult{}, err
		}
	} else if err != nil {
		return ReidentifyResult{}, err
	}
	var relationExists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM drawing_part_relations WHERE drawing_id = $1::uuid AND part_id = $2::uuid AND status = 'active')`, drawingID, partID).Scan(&relationExists); err != nil {
		return ReidentifyResult{}, fmt.Errorf("检查零件结构关系失败: %w", err)
	}
	if !relationExists {
		if _, err := tx.Exec(ctx, `INSERT INTO drawing_part_relations (drawing_id, part_id, relation_type, created_by, updated_by) VALUES ($1::uuid, $2::uuid, 'owned', $3::uuid, $3::uuid)`, drawingID, partID, userID); err != nil {
			return ReidentifyResult{}, fmt.Errorf("创建零件结构关系失败: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE attachments SET drawing_id = NULL, part_id = $2::uuid, file_role = 'part', revision = revision + 1 WHERE id = $1::uuid`, item.ID, partID); err != nil {
		return ReidentifyResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ReidentifyResult{}, err
	}
	return ReidentifyResult{StorageKey: storageKey, DrawingNo: item.DrawingNo, OldPartNo: nullablePart(item.PartNo), PartNo: no}, nil
}

func nullablePart(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
func (repository *PGRepository) FolderForDrawing(ctx context.Context, drawingNo string) (string, error) {
	var project, name string
	err := repository.pool.QueryRow(ctx, `SELECT project, name FROM drawings WHERE drawing_no = $1`, drawingNo).Scan(&project, &name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(project) == "" {
		return "", errors.New("图纸项目号为空，无法生成附件目录")
	}
	return project + "(" + name + ")", nil
}
func isUnique(err error) bool {
	message := err.Error()
	return strings.Contains(message, "duplicate key") || strings.Contains(message, "unique constraint") || strings.Contains(message, "SQLSTATE 23505")
}
