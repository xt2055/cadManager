package upload

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"cadguanliq/internal/attachment"
	"cadguanliq/internal/converter"
	"cadguanliq/internal/storage"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound            = errors.New("上传会话或文件不存在")
	ErrConflict            = errors.New("上传会话状态冲突")
	ErrIdempotencyConflict = errors.New("Idempotency-Key conflict")
	ErrExpired             = errors.New("上传会话已过期")
	ErrIncomplete          = errors.New("上传会话仍有文件未准备完成")
)

const absoluteTTL = 7 * 24 * time.Hour

type Service struct {
	pool      *pgxpool.Pool
	storage   storage.ObjectStorage
	converter *converter.Service
	ttl       time.Duration
	cleanupMu sync.Mutex
	commitMu  sync.Mutex
}

type Session struct {
	ID                string          `json:"id"`
	Kind              string          `json:"kind"`
	IdempotencyKey    string          `json:"idempotencyKey"`
	Status            string          `json:"status"`
	Metadata          json.RawMessage `json:"metadata"`
	CreatedAt         time.Time       `json:"createdAt"`
	LastActivityAt    time.Time       `json:"lastActivityAt"`
	ExpiresAt         time.Time       `json:"expiresAt"`
	AbsoluteExpiresAt time.Time       `json:"absoluteExpiresAt"`
	CommittedAt       *time.Time      `json:"committedAt,omitempty"`
	ErrorMessage      string          `json:"errorMessage,omitempty"`
}

type Item struct {
	ID               string    `json:"id"`
	ClientRef        string    `json:"clientRef"`
	AttachmentID     string    `json:"attachmentId,omitempty"`
	DrawingNo        string    `json:"drawingNo"`
	PartNo           string    `json:"partNo"`
	Role             string    `json:"role"`
	OriginalName     string    `json:"originalName"`
	MimeType         string    `json:"mimeType"`
	ExpectedRevision *int64    `json:"expectedRevision,omitempty"`
	Status           string    `json:"status"`
	Size             int64     `json:"size"`
	SHA256           string    `json:"sha256,omitempty"`
	Attempts         int       `json:"attempts"`
	ErrorMessage     string    `json:"errorMessage,omitempty"`
	UpdatedAt        time.Time `json:"updatedAt"`
	ExpectedSize     *int64    `json:"-"`
	ExpectedSHA256   string    `json:"-"`
}

type Snapshot struct {
	Session Session `json:"session"`
	Items   []Item  `json:"items"`
}

type CreateSessionInput struct {
	Kind           string
	IdempotencyKey string
	Metadata       json.RawMessage
}

type CreateItemInput struct {
	ClientRef        string
	AttachmentID     string
	DrawingNo        string
	PartNo           string
	Role             string
	OriginalName     string
	MimeType         string
	ExpectedRevision *int64
	SHA256           string
	Size             int64
	BlobID           string
}

type HashCheckInput struct {
	SHA256   string
	Size     int64
	MimeType string
}

type HashCheckResult struct {
	Exists     bool   `json:"exists"`
	SHA256     string `json:"sha256"`
	BlobID     string `json:"blobId,omitempty"`
	StorageKey string `json:"storageKey,omitempty"`
	Size       int64  `json:"size,omitempty"`
	MimeType   string `json:"mimeType,omitempty"`
}

type ChunkManifest struct {
	TotalSize int64  `json:"totalSize"`
	ChunkSize int64  `json:"chunkSize"`
	SHA256    string `json:"sha256"`
}

type ChunkInfo struct {
	PartNumber int    `json:"partNumber"`
	Offset     int64  `json:"offset"`
	Size       int64  `json:"size"`
	SHA256     string `json:"sha256"`
}

type ChunkSnapshot struct {
	Manifest ChunkManifest `json:"manifest"`
	Parts    []ChunkInfo   `json:"parts"`
}

type ReconciliationReport struct {
	AttachmentsTotal       int64 `json:"attachmentsTotal"`
	AttachmentsBlobLinked  int64 `json:"attachmentsBlobLinked"`
	AttachmentsCurrentBlob int64 `json:"attachmentsCurrentBlobLinked"`
	VersionsTotal          int64 `json:"versionsTotal"`
	VersionsBlobLinked     int64 `json:"versionsBlobLinked"`
	BlobsTotal             int64 `json:"blobsTotal"`
	OrphanBlobs            int64 `json:"orphanBlobs"`
	MissingPhysicalObjects int64 `json:"missingPhysicalObjects"`
	CleanupPending         int64 `json:"cleanupPending"`
	PhysicalObjectsTotal   int64 `json:"physicalObjectsTotal"`
	UnreferencedPhysical   int64 `json:"unreferencedPhysicalObjects"`
}

type physicalObjectLister interface {
	List(context.Context) ([]storage.ObjectInfo, error)
}

type DrawingCreateManifest struct {
	Drawing    json.RawMessage   `json:"drawing"`
	Parts      json.RawMessage   `json:"parts"`
	Attributes map[string]string `json:"attributeValues,omitempty"`
}

type manifestBOM struct {
	No        int     `json:"no"`
	DrawingNo string  `json:"drawingNo"`
	PartNo    string  `json:"partNo"`
	Name      string  `json:"name"`
	Spec      string  `json:"spec"`
	Quantity  float64 `json:"qty"`
	Weight    float64 `json:"weight"`
	Remark    string  `json:"remark"`
}

type manifestBorrow struct {
	Direction       string `json:"direction"`
	SourceDrawingNo string `json:"sourceDrawingNo"`
	SourcePartNo    string `json:"sourcePartNo"`
	TargetPartNo    string `json:"targetPartNo"`
	Status          string `json:"status"`
}

type manifestBranch struct {
	SourceDrawingNo string `json:"sourceDrawingNo"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	Status          string `json:"status"`
}

type manifestDrawing struct {
	No              string            `json:"no"`
	Name            string            `json:"name"`
	Kind            string            `json:"kind"`
	Project         string            `json:"project"`
	Material        string            `json:"material"`
	Vendor          string            `json:"vendor"`
	Status          string            `json:"status"`
	Version         string            `json:"ver"`
	BorrowFrom      *string           `json:"borrowFrom"`
	Remark          *string           `json:"remark"`
	Signers         map[string]string `json:"signers"`
	AttributeValues map[string]string `json:"attributeValues"`
}

type manifestPart struct {
	No                string            `json:"no"`
	Name              string            `json:"name"`
	ParentNo          string            `json:"parentNo"`
	Project           string            `json:"project"`
	Material          string            `json:"material"`
	Spec              string            `json:"spec"`
	Weight            float64           `json:"weight"`
	SurfaceTreatment  string            `json:"surfaceTreatment"`
	ManufacturingType string            `json:"partType"`
	Quantity          float64           `json:"qty"`
	Status            string            `json:"status"`
	Version           string            `json:"ver"`
	Vendor            *string           `json:"vendor"`
	BorrowFrom        *string           `json:"borrowFrom"`
	Remark            *string           `json:"remark"`
	Signers           map[string]string `json:"signers"`
}

func NewService(pool *pgxpool.Pool, objectStorage storage.ObjectStorage, converterService *converter.Service, ttl time.Duration) *Service {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &Service{pool: pool, storage: objectStorage, converter: converterService, ttl: ttl}
}

func normalizeSHA256(value string) (string, error) {
	hash := strings.ToLower(strings.TrimSpace(value))
	if len(hash) != 64 {
		return "", errors.New("SHA-256 格式无效")
	}
	if _, err := hex.DecodeString(hash); err != nil {
		return "", errors.New("SHA-256 格式无效")
	}
	return hash, nil
}

func (service *Service) CreateSession(ctx context.Context, userID string, input CreateSessionInput) (Session, error) {
	if service == nil || service.pool == nil {
		return Session{}, errors.New("上传服务未配置")
	}
	kind := strings.TrimSpace(input.Kind)
	if kind != "attachment" && kind != "drawing-create" {
		return Session{}, errors.New("上传会话类型无效")
	}
	key := strings.TrimSpace(input.IdempotencyKey)
	if key == "" {
		return Session{}, errors.New("缺少幂等键")
	}
	metadata := input.Metadata
	if len(metadata) == 0 {
		metadata = json.RawMessage(`{}`)
	}
	if !json.Valid(metadata) {
		return Session{}, errors.New("上传会话元数据格式无效")
	}
	var id string
	err := service.pool.QueryRow(ctx, `
		INSERT INTO upload_sessions (user_id, kind, idempotency_key, metadata, expires_at, absolute_expires_at)
		VALUES ($1::uuid, $2, $3, $4::jsonb, LEAST(now() + $5::interval, now() + $6::interval), now() + $6::interval)
		ON CONFLICT (user_id, idempotency_key) DO NOTHING
		RETURNING id::text`, userID, kind, key, string(metadata), intervalText(service.ttl), intervalText(absoluteTTL)).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		var existingKind string
		var metadataMatches bool
		lookupErr := service.pool.QueryRow(ctx, `
			SELECT id::text, kind, metadata = $3::jsonb
			FROM upload_sessions WHERE user_id = $1::uuid AND idempotency_key = $2`, userID, key, string(metadata)).
			Scan(&id, &existingKind, &metadataMatches)
		if errors.Is(lookupErr, pgx.ErrNoRows) {
			return Session{}, fmt.Errorf("创建上传会话失败：幂等键竞争后记录不存在: %w", ErrConflict)
		}
		if lookupErr != nil {
			return Session{}, fmt.Errorf("读取幂等上传会话失败: %w", lookupErr)
		}
		if existingKind != kind || !metadataMatches {
			return Session{}, ErrIdempotencyConflict
		}
	} else if err != nil {
		return Session{}, fmt.Errorf("创建上传会话失败: %w", err)
	}
	return service.getSession(ctx, userID, id)
}

func (service *Service) HashCheck(ctx context.Context, _ string, input HashCheckInput) (HashCheckResult, error) {
	if service == nil || service.pool == nil || service.storage == nil {
		return HashCheckResult{}, errors.New("上传服务未配置")
	}
	hash, err := normalizeSHA256(input.SHA256)
	if err != nil {
		return HashCheckResult{}, err
	}
	if input.Size < 0 {
		return HashCheckResult{}, errors.New("文件大小无效")
	}
	var result HashCheckResult
	err = service.pool.QueryRow(ctx, `
		SELECT id::text, storage_key, size_bytes, mime_type
		FROM file_blobs WHERE sha256 = $1 AND size_bytes = $2`, hash, input.Size).
		Scan(&result.BlobID, &result.StorageKey, &result.Size, &result.MimeType)
	if errors.Is(err, pgx.ErrNoRows) {
		return HashCheckResult{Exists: false, SHA256: hash, Size: input.Size, MimeType: input.MimeType}, nil
	}
	if err != nil {
		return HashCheckResult{}, fmt.Errorf("哈希预检失败: %w", err)
	}
	reader, _, openErr := service.storage.Open(ctx, result.StorageKey)
	if openErr != nil {
		return HashCheckResult{Exists: false, SHA256: hash, Size: input.Size, MimeType: input.MimeType}, nil
	}
	_ = reader.Close()
	result.Exists = true
	result.SHA256 = hash
	return result, nil
}

// BackfillLegacyBlobs 将历史附件、当前文件和历史版本接入 file_blobs。
// 该操作只使用数据库中已经保存的 SHA-256 与大小，不会删除或覆盖任何旧物理文件；
// 同一内容对象通过 (sha256, size_bytes) 合并，重复执行安全幂等。
func (service *Service) BackfillLegacyBlobs(ctx context.Context) error {
	if service == nil || service.pool == nil {
		return errors.New("上传服务未配置")
	}
	_, err := service.pool.Exec(ctx, `
		INSERT INTO file_blobs (sha256, size_bytes, mime_type, storage_key)
		SELECT DISTINCT ON (trim(a.sha256), a.size_bytes)
		       trim(a.sha256), a.size_bytes, COALESCE(NULLIF(a.mime_type, ''), 'application/octet-stream'), a.storage_key
		FROM attachments a
		WHERE a.blob_id IS NULL AND a.storage_key <> '' AND length(trim(a.sha256)) = 64
		ORDER BY trim(a.sha256), a.size_bytes, a.created_at, a.id
		ON CONFLICT (sha256, size_bytes) DO NOTHING;
		INSERT INTO file_blobs (sha256, size_bytes, mime_type, storage_key)
		SELECT DISTINCT ON (trim(COALESCE(a.current_sha256, '')), COALESCE(a.current_size_bytes, a.size_bytes))
		       trim(a.current_sha256), COALESCE(a.current_size_bytes, a.size_bytes),
		       COALESCE(NULLIF(a.current_mime_type, ''), NULLIF(a.mime_type, ''), 'application/octet-stream'),
		       COALESCE(NULLIF(a.current_storage_key, ''), a.storage_key)
		FROM attachments a
		WHERE a.current_blob_id IS NULL AND COALESCE(NULLIF(a.current_storage_key, ''), a.storage_key) <> ''
		  AND length(trim(COALESCE(a.current_sha256, ''))) = 64
		ORDER BY trim(COALESCE(a.current_sha256, '')), COALESCE(a.current_size_bytes, a.size_bytes), a.created_at, a.id
		ON CONFLICT (sha256, size_bytes) DO NOTHING;
		INSERT INTO file_blobs (sha256, size_bytes, mime_type, storage_key)
		SELECT DISTINCT ON (trim(v.sha256), v.size_bytes)
		       trim(v.sha256), v.size_bytes, COALESCE(NULLIF(v.mime_type, ''), 'application/octet-stream'), v.storage_key
		FROM file_versions v
		WHERE v.blob_id IS NULL AND v.storage_key <> '' AND length(trim(v.sha256)) = 64
		ORDER BY trim(v.sha256), v.size_bytes, v.created_at, v.id
		ON CONFLICT (sha256, size_bytes) DO NOTHING;
		UPDATE attachments a SET blob_id = b.id
		FROM file_blobs b
		WHERE a.blob_id IS NULL AND b.sha256 = trim(a.sha256) AND b.size_bytes = a.size_bytes;
		UPDATE attachments a SET current_blob_id = b.id
		FROM file_blobs b
		WHERE a.current_blob_id IS NULL
		  AND b.sha256 = trim(COALESCE(a.current_sha256, a.sha256))
		  AND b.size_bytes = COALESCE(a.current_size_bytes, a.size_bytes);
		UPDATE file_versions v SET blob_id = b.id
		FROM file_blobs b
		WHERE v.blob_id IS NULL AND b.sha256 = trim(v.sha256) AND b.size_bytes = v.size_bytes;
	`)
	if err != nil {
		return fmt.Errorf("回填历史内容对象失败: %w", err)
	}
	return nil
}

// Reconcile 只读核对数据库引用与对象存储，不删除或修改任何数据。
func (service *Service) Reconcile(ctx context.Context) (ReconciliationReport, error) {
	if service == nil || service.pool == nil || service.storage == nil {
		return ReconciliationReport{}, errors.New("上传服务未配置")
	}
	var report ReconciliationReport
	queries := []struct {
		dest *int64
		sql  string
	}{
		{&report.AttachmentsTotal, `SELECT count(*) FROM attachments WHERE deleted_at IS NULL`},
		{&report.AttachmentsBlobLinked, `SELECT count(*) FROM attachments WHERE deleted_at IS NULL AND blob_id IS NOT NULL`},
		{&report.AttachmentsCurrentBlob, `SELECT count(*) FROM attachments WHERE deleted_at IS NULL AND current_blob_id IS NOT NULL`},
		{&report.VersionsTotal, `SELECT count(*) FROM file_versions WHERE deleted_at IS NULL`},
		{&report.VersionsBlobLinked, `SELECT count(*) FROM file_versions WHERE deleted_at IS NULL AND blob_id IS NOT NULL`},
		{&report.BlobsTotal, `SELECT count(*) FROM file_blobs`},
		{&report.OrphanBlobs, `SELECT count(*) FROM file_blobs b WHERE NOT EXISTS (SELECT 1 FROM attachments a WHERE a.blob_id = b.id OR a.current_blob_id = b.id) AND NOT EXISTS (SELECT 1 FROM file_versions v WHERE v.blob_id = b.id) AND NOT EXISTS (SELECT 1 FROM upload_session_items i WHERE i.blob_id = b.id OR i.processed_blob_id = b.id)`},
		{&report.CleanupPending, `SELECT count(*) FROM storage_cleanup_jobs WHERE status <> 'completed'`},
	}
	for _, query := range queries {
		if err := service.pool.QueryRow(ctx, query.sql).Scan(query.dest); err != nil {
			return ReconciliationReport{}, fmt.Errorf("读取对账统计失败: %w", err)
		}
	}
	rows, err := service.pool.Query(ctx, `SELECT storage_key FROM file_blobs`)
	if err != nil {
		return ReconciliationReport{}, fmt.Errorf("读取内容对象清单失败: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return ReconciliationReport{}, err
		}
		reader, _, openErr := service.storage.Open(ctx, key)
		if openErr != nil {
			report.MissingPhysicalObjects++
			continue
		}
		_ = reader.Close()
	}
	if err := rows.Err(); err != nil {
		return ReconciliationReport{}, err
	}
	if lister, ok := service.storage.(physicalObjectLister); ok {
		objects, err := lister.List(ctx)
		if err != nil {
			return ReconciliationReport{}, err
		}
		refs := make(map[string]struct{})
		refRows, err := service.pool.Query(ctx, `
			SELECT storage_key FROM file_blobs
			UNION SELECT storage_key FROM attachments WHERE deleted_at IS NULL
			UNION SELECT current_storage_key FROM attachments WHERE deleted_at IS NULL AND current_storage_key IS NOT NULL
			UNION SELECT storage_key FROM file_versions
			UNION SELECT staging_object_key FROM upload_session_items WHERE staging_object_key IS NOT NULL
			UNION SELECT object_key FROM upload_session_items WHERE object_key IS NOT NULL
			UNION SELECT processed_object_key FROM upload_session_items WHERE processed_object_key IS NOT NULL
			UNION SELECT storage_key FROM upload_session_chunks
			UNION SELECT storage_key FROM storage_cleanup_jobs WHERE status <> 'completed'`)
		if err != nil {
			return ReconciliationReport{}, fmt.Errorf("读取物理对象引用失败: %w", err)
		}
		for refRows.Next() {
			var key string
			if err := refRows.Scan(&key); err != nil {
				refRows.Close()
				return ReconciliationReport{}, err
			}
			if key != "" {
				refs[key] = struct{}{}
			}
		}
		if err := refRows.Err(); err != nil {
			refRows.Close()
			return ReconciliationReport{}, err
		}
		refRows.Close()
		report.PhysicalObjectsTotal = int64(len(objects))
		for _, object := range objects {
			if _, found := refs[object.Key]; !found {
				report.UnreferencedPhysical++
			}
		}
	}
	return report, nil
}

func (service *Service) CreateItem(ctx context.Context, userID, sessionID string, input CreateItemInput) (Item, error) {
	if strings.TrimSpace(input.ClientRef) == "" || strings.TrimSpace(input.OriginalName) == "" {
		return Item{}, errors.New("文件引用和文件名不能为空")
	}
	if input.Role == "" {
		input.Role = "other"
	}
	if input.MimeType == "" {
		input.MimeType = "application/octet-stream"
	}
	if err := service.touchSession(ctx, userID, sessionID); err != nil {
		return Item{}, err
	}
	status := "pending"
	objectKey := ""
	blobID := ""
	sha256 := strings.ToLower(strings.TrimSpace(input.SHA256))
	size := int64(0)
	if sha256 != "" {
		var err error
		sha256, err = normalizeSHA256(sha256)
		if err != nil {
			return Item{}, err
		}
	}
	if strings.TrimSpace(input.BlobID) != "" {
		if isCAD(input.OriginalName) {
			return Item{}, errors.New("CAD 文件需要先上传内容并生成处理结果")
		}
		if len(sha256) != 64 || input.Size < 0 {
			return Item{}, errors.New("秒传文件校验参数无效")
		}
		if err := service.pool.QueryRow(ctx, `SELECT storage_key FROM file_blobs WHERE id = $1::uuid AND sha256 = $2 AND size_bytes = $3`, input.BlobID, sha256, input.Size).Scan(&objectKey); err != nil {
			return Item{}, errors.New("秒传内容对象不存在或校验不匹配")
		}
		if reader, _, err := service.storage.Open(ctx, objectKey); err != nil {
			return Item{}, errors.New("秒传内容对象已丢失")
		} else {
			_ = reader.Close()
		}
		status, blobID, size = "ready", input.BlobID, input.Size
	}
	var id string
	err := service.pool.QueryRow(ctx, `
		INSERT INTO upload_session_items (session_id, client_ref, attachment_id, drawing_no, part_no, file_role, original_name, mime_type, expected_revision, status, object_key, blob_id, size_bytes, sha256)
		VALUES ($1::uuid, $2, NULLIF($3, '')::uuid, $4, $5, $6, $7, $8, $9, $10, NULLIF($11, ''), NULLIF($12, '')::uuid, $13, NULLIF($14, ''))
		ON CONFLICT (session_id, client_ref) DO UPDATE SET
			attachment_id = EXCLUDED.attachment_id, drawing_no = EXCLUDED.drawing_no, part_no = EXCLUDED.part_no,
			file_role = EXCLUDED.file_role, original_name = EXCLUDED.original_name, mime_type = EXCLUDED.mime_type,
			expected_revision = EXCLUDED.expected_revision, status = EXCLUDED.status, staging_object_key = NULL,
			object_key = EXCLUDED.object_key, blob_id = EXCLUDED.blob_id, processed_object_key = NULL, processed_blob_id = NULL,
			size_bytes = EXCLUDED.size_bytes, sha256 = EXCLUDED.sha256, error_message = NULL, updated_at = now()
		RETURNING id::text`, sessionID, input.ClientRef, input.AttachmentID, input.DrawingNo, input.PartNo, input.Role, input.OriginalName, input.MimeType, input.ExpectedRevision, status, objectKey, blobID, size, sha256).Scan(&id)
	if err != nil {
		return Item{}, fmt.Errorf("创建上传文件项失败: %w", err)
	}
	if _, err := service.pool.Exec(ctx, `
		UPDATE upload_session_items
		SET expected_size_bytes = NULLIF($2, 0), expected_sha256 = NULLIF($3, ''), updated_at = now()
		WHERE id = $1::uuid`, id, input.Size, sha256); err != nil {
		return Item{}, fmt.Errorf("登记文件校验信息失败: %w", err)
	}
	return service.getItem(ctx, userID, sessionID, id)
}

func (service *Service) InitChunks(ctx context.Context, userID, sessionID, itemID string, input ChunkManifest) (ChunkSnapshot, error) {
	if service == nil || service.pool == nil || service.storage == nil {
		return ChunkSnapshot{}, errors.New("上传服务未配置")
	}
	if input.TotalSize <= 0 || input.ChunkSize < 64*1024 || input.ChunkSize > 32*1024*1024 {
		return ChunkSnapshot{}, errors.New("分片大小或文件总大小无效")
	}
	hash, err := normalizeSHA256(input.SHA256)
	if err != nil {
		return ChunkSnapshot{}, fmt.Errorf("最终 %w", err)
	}
	if err := service.touchSession(ctx, userID, sessionID); err != nil {
		return ChunkSnapshot{}, err
	}
	var currentSize, currentChunkSize *int64
	var currentHash string
	if err := service.pool.QueryRow(ctx, `SELECT expected_size_bytes, chunk_size_bytes, COALESCE(expected_sha256, '') FROM upload_session_items WHERE id = $1::uuid AND session_id = $2::uuid`, itemID, sessionID).Scan(&currentSize, &currentChunkSize, &currentHash); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ChunkSnapshot{}, ErrNotFound
		}
		return ChunkSnapshot{}, err
	}
	if currentChunkSize != nil && (currentSize == nil || *currentSize != input.TotalSize || *currentChunkSize != input.ChunkSize || currentHash != hash) {
		return ChunkSnapshot{}, ErrConflict
	}
	if _, err := service.pool.Exec(ctx, `UPDATE upload_session_items SET expected_size_bytes = $2, expected_sha256 = $3, chunk_size_bytes = $4, status = CASE WHEN status = 'failed' THEN 'pending' ELSE status END, error_message = NULL, updated_at = now() WHERE id = $1::uuid`, itemID, input.TotalSize, hash, input.ChunkSize); err != nil {
		return ChunkSnapshot{}, fmt.Errorf("初始化分片上传失败: %w", err)
	}
	return service.ListChunks(ctx, userID, sessionID, itemID)
}

func (service *Service) UploadChunk(ctx context.Context, userID, sessionID, itemID string, partNumber int, reader io.Reader, declaredHash string) (ChunkInfo, error) {
	if partNumber < 0 {
		return ChunkInfo{}, errors.New("分片编号无效")
	}
	if err := service.touchSession(ctx, userID, sessionID); err != nil {
		return ChunkInfo{}, err
	}
	var totalSize, chunkSize int64
	if err := service.pool.QueryRow(ctx, `SELECT expected_size_bytes, chunk_size_bytes FROM upload_session_items WHERE id = $1::uuid AND session_id = $2::uuid AND expected_size_bytes IS NOT NULL AND chunk_size_bytes IS NOT NULL`, itemID, sessionID).Scan(&totalSize, &chunkSize); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ChunkInfo{}, ErrConflict
		}
		return ChunkInfo{}, err
	}
	offset := int64(partNumber) * chunkSize
	if offset >= totalSize {
		return ChunkInfo{}, errors.New("分片偏移超出文件总大小")
	}
	key := filepath.ToSlash(filepath.Join(".staging", sessionID, itemID, "chunks", fmt.Sprintf("%08d", partNumber)))
	object, err := service.storage.Put(ctx, key, reader, "application/octet-stream")
	if err != nil {
		return ChunkInfo{}, fmt.Errorf("写入分片失败: %w", err)
	}
	if object.Size > chunkSize || offset+object.Size > totalSize {
		_ = service.storage.Delete(ctx, key)
		return ChunkInfo{}, errors.New("分片大小超出声明范围")
	}
	if declaredHash != "" && !strings.EqualFold(strings.TrimSpace(declaredHash), object.SHA256) {
		_ = service.storage.Delete(ctx, key)
		return ChunkInfo{}, errors.New("分片 SHA-256 校验失败")
	}
	_, err = service.pool.Exec(ctx, `
		INSERT INTO upload_session_chunks (item_id, part_number, offset_bytes, size_bytes, sha256, storage_key)
		VALUES ($1::uuid, $2, $3, $4, $5, $6)
		ON CONFLICT (item_id, part_number) DO UPDATE SET offset_bytes = EXCLUDED.offset_bytes, size_bytes = EXCLUDED.size_bytes, sha256 = EXCLUDED.sha256, storage_key = EXCLUDED.storage_key, updated_at = now()`, itemID, partNumber, offset, object.Size, object.SHA256, key)
	if err != nil {
		_ = service.storage.Delete(ctx, key)
		return ChunkInfo{}, fmt.Errorf("登记分片失败: %w", err)
	}
	return ChunkInfo{PartNumber: partNumber, Offset: offset, Size: object.Size, SHA256: object.SHA256}, nil
}

func (service *Service) ListChunks(ctx context.Context, userID, sessionID, itemID string) (ChunkSnapshot, error) {
	if err := service.getItemExists(ctx, userID, sessionID, itemID); err != nil {
		return ChunkSnapshot{}, err
	}
	var manifest ChunkManifest
	if err := service.pool.QueryRow(ctx, `SELECT COALESCE(expected_size_bytes, 0), COALESCE(chunk_size_bytes, 0), COALESCE(expected_sha256, '') FROM upload_session_items WHERE id = $1::uuid`, itemID).Scan(&manifest.TotalSize, &manifest.ChunkSize, &manifest.SHA256); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ChunkSnapshot{}, ErrConflict
		}
		return ChunkSnapshot{}, err
	}
	rows, err := service.pool.Query(ctx, `SELECT part_number, offset_bytes, size_bytes, sha256 FROM upload_session_chunks WHERE item_id = $1::uuid ORDER BY part_number`, itemID)
	if err != nil {
		return ChunkSnapshot{}, err
	}
	defer rows.Close()
	parts := make([]ChunkInfo, 0)
	for rows.Next() {
		var part ChunkInfo
		if err := rows.Scan(&part.PartNumber, &part.Offset, &part.Size, &part.SHA256); err != nil {
			return ChunkSnapshot{}, err
		}
		parts = append(parts, part)
	}
	return ChunkSnapshot{Manifest: manifest, Parts: parts}, rows.Err()
}

func (service *Service) CompleteChunks(ctx context.Context, userID, sessionID, itemID string) (Item, error) {
	snapshot, err := service.ListChunks(ctx, userID, sessionID, itemID)
	if err != nil {
		return Item{}, err
	}
	if snapshot.Manifest.TotalSize <= 0 || len(snapshot.Parts) == 0 {
		return Item{}, ErrIncomplete
	}
	var total int64
	readers := make([]io.ReadCloser, 0, len(snapshot.Parts))
	keys := make([]string, 0, len(snapshot.Parts))
	for index, part := range snapshot.Parts {
		if part.PartNumber != index || part.Offset != total || part.Size <= 0 {
			closeReaders(readers)
			return Item{}, ErrIncomplete
		}
		var key string
		if err := service.pool.QueryRow(ctx, `SELECT storage_key FROM upload_session_chunks WHERE item_id = $1::uuid AND part_number = $2`, itemID, part.PartNumber).Scan(&key); err != nil {
			closeReaders(readers)
			return Item{}, err
		}
		reader, _, openErr := service.storage.Open(ctx, key)
		if openErr != nil {
			closeReaders(readers)
			return Item{}, fmt.Errorf("缺失分片 %d: %w", part.PartNumber, openErr)
		}
		readers = append(readers, reader)
		keys = append(keys, key)
		total += part.Size
	}
	if total != snapshot.Manifest.TotalSize {
		closeReaders(readers)
		return Item{}, ErrIncomplete
	}
	mergedKey := filepath.ToSlash(filepath.Join(".staging", sessionID, itemID, "chunks-merged"))
	merged, putErr := service.storage.Put(ctx, mergedKey, io.MultiReader(readersToReaders(readers)...), "application/octet-stream")
	closeReaders(readers)
	if putErr != nil {
		return Item{}, fmt.Errorf("合并分片失败: %w", putErr)
	}
	if merged.Size != snapshot.Manifest.TotalSize || !strings.EqualFold(merged.SHA256, snapshot.Manifest.SHA256) {
		_ = service.storage.Delete(ctx, mergedKey)
		return Item{}, errors.New("合并文件最终 SHA-256 校验失败")
	}
	reader, _, err := service.storage.Open(ctx, mergedKey)
	if err != nil {
		return Item{}, err
	}
	item, uploadErr := service.UploadItem(ctx, userID, sessionID, itemID, "", "", reader)
	_ = reader.Close()
	_ = service.storage.Delete(ctx, mergedKey)
	if uploadErr != nil {
		return Item{}, uploadErr
	}
	for _, key := range keys {
		if deleteErr := service.storage.Delete(ctx, key); deleteErr != nil {
			service.scheduleCleanup(ctx, key, "upload-chunk")
		}
	}
	_, _ = service.pool.Exec(ctx, `DELETE FROM upload_session_chunks WHERE item_id = $1::uuid`, itemID)
	return item, nil
}

func (service *Service) getItemExists(ctx context.Context, userID, sessionID, itemID string) error {
	if _, err := service.getItem(ctx, userID, sessionID, itemID); err != nil {
		return err
	}
	return nil
}

func readersToReaders(readers []io.ReadCloser) []io.Reader {
	result := make([]io.Reader, len(readers))
	for index, reader := range readers {
		result[index] = reader
	}
	return result
}

func closeReaders(readers []io.ReadCloser) {
	for _, reader := range readers {
		_ = reader.Close()
	}
}

func (service *Service) UploadItem(ctx context.Context, userID, sessionID, itemID, name, mimeType string, reader io.Reader) (Item, error) {
	if service == nil || service.pool == nil || service.storage == nil {
		return Item{}, errors.New("上传服务未配置")
	}
	item, err := service.getItem(ctx, userID, sessionID, itemID)
	if err != nil {
		return Item{}, err
	}
	if item.Status == "ready" || item.Status == "committed" {
		return item, nil
	}
	session, err := service.getSession(ctx, userID, sessionID)
	if err != nil {
		return Item{}, err
	}
	if session.Status != "open" && session.Status != "failed" {
		return Item{}, ErrConflict
	}
	if name == "" {
		name = item.OriginalName
	}
	if mimeType == "" {
		mimeType = item.MimeType
	}
	if _, err := service.pool.Exec(ctx, `UPDATE upload_session_items SET status = 'uploading', attempts = attempts + 1, error_message = NULL, updated_at = now() WHERE id = $1::uuid AND status IN ('pending', 'failed')`, itemID); err != nil {
		return Item{}, fmt.Errorf("更新上传状态失败: %w", err)
	}
	stagingKey := filepath.ToSlash(filepath.Join(".staging", sessionID, itemID, fmt.Sprintf("%d-%s", time.Now().UnixNano(), safeName(name))))
	if _, err := service.pool.Exec(ctx, `UPDATE upload_session_items SET staging_object_key = $2, updated_at = now() WHERE id = $1::uuid`, itemID, stagingKey); err != nil {
		return Item{}, fmt.Errorf("登记暂存文件路径失败: %w", err)
	}
	object, err := service.storage.Put(ctx, stagingKey, reader, mimeType)
	if err != nil {
		service.markItemFailed(ctx, itemID, err)
		return Item{}, fmt.Errorf("写入暂存文件失败: %w", err)
	}
	if (item.ExpectedSize != nil && object.Size != *item.ExpectedSize) || (item.ExpectedSHA256 != "" && !strings.EqualFold(object.SHA256, item.ExpectedSHA256)) {
		service.markItemFailed(ctx, itemID, errors.New("文件内容校验失败"))
		_ = service.storage.Delete(ctx, stagingKey)
		return Item{}, errors.New("文件内容校验失败")
	}
	blobID, blobKey, err := service.ensureBlob(ctx, stagingKey, object)
	if err != nil {
		service.markItemFailed(ctx, itemID, err)
		service.scheduleCleanup(ctx, stagingKey, "upload-staging")
		return Item{}, err
	}
	processedKey := ""
	processedBlobID := ""
	processedSize := int64(0)
	processedSHA256 := ""
	processedMimeType := ""
	convertedSourceKey := ""
	if isCAD(name) {
		if service.converter == nil {
			err = errors.New("CAD 转换服务未配置")
		} else {
			convertedSourceKey, err = service.converter.EnsureDwg(ctx, attachment.Attachment{
				StorageKey:        stagingKey,
				CurrentStorageKey: stagingKey,
				Name:              name,
				CurrentName:       name,
				DrawingNo:         item.DrawingNo,
				PartNo:            nullableString(item.PartNo),
				Size:              object.Size,
				MimeType:          object.MimeType,
				SHA256:            object.SHA256,
				CurrentSHA256:     object.SHA256,
			})
			if err == nil {
				convertedObject, openErr := service.openObjectInfo(ctx, convertedSourceKey)
				if openErr != nil {
					err = openErr
				} else {
					processedBlobID, processedKey, err = service.ensureBlob(ctx, convertedSourceKey, convertedObject)
					processedSize = convertedObject.Size
					processedSHA256 = convertedObject.SHA256
					processedMimeType = convertedObject.MimeType
				}
			}
		}
	}
	if err != nil {
		service.markItemFailed(ctx, itemID, err)
		service.scheduleCleanup(ctx, stagingKey, "upload-staging")
		if processedKey != "" {
			service.scheduleCleanup(ctx, processedKey, "cad-processing")
		}
		return Item{}, fmt.Errorf("CAD 文件处理失败: %w", err)
	}
	if _, err := service.pool.Exec(ctx, `
		UPDATE upload_session_items
		SET status = 'ready', staging_object_key = NULL, object_key = $2, blob_id = $3::uuid,
			processed_object_key = NULLIF($4, ''), processed_blob_id = NULLIF($5, '')::uuid,
			processed_size_bytes = $6, processed_sha256 = NULLIF($7, ''), processed_mime_type = NULLIF($8, ''),
			size_bytes = $9, sha256 = $10, original_name = $11, mime_type = $12,
			error_message = NULL, updated_at = now()
		WHERE id = $1::uuid`, itemID, blobKey, blobID, processedKey, processedBlobID, processedSize, processedSHA256, processedMimeType, object.Size, object.SHA256, name, mimeType); err != nil {
		service.markItemFailed(ctx, itemID, err)
		service.scheduleCleanup(ctx, stagingKey, "upload-staging")
		if processedKey != "" {
			service.scheduleCleanup(ctx, processedKey, "cad-processing")
		}
		return Item{}, fmt.Errorf("登记暂存文件失败: %w", err)
	}
	if deleteErr := service.storage.Delete(ctx, stagingKey); deleteErr != nil {
		service.scheduleCleanup(ctx, stagingKey, "upload-staging")
	}
	if processedKey != "" && processedKey != blobKey {
		if deleteErr := service.storage.Delete(ctx, convertedSourceKey); deleteErr != nil {
			service.scheduleCleanup(ctx, convertedSourceKey, "cad-staging")
		}
	}
	if err := service.touchSession(ctx, userID, sessionID); err != nil {
		return Item{}, err
	}
	return service.getItem(ctx, userID, sessionID, itemID)
}

func (service *Service) RetryItem(ctx context.Context, userID, sessionID, itemID string) (Item, error) {
	if err := service.touchSession(ctx, userID, sessionID); err != nil {
		return Item{}, err
	}
	var stagingKey, objectKey, processedKey, blobID, processedBlobID string
	if err := service.pool.QueryRow(ctx, `SELECT COALESCE(staging_object_key, ''), COALESCE(object_key, ''), COALESCE(processed_object_key, ''), COALESCE(blob_id::text, ''), COALESCE(processed_blob_id::text, '') FROM upload_session_items WHERE id = $1::uuid AND session_id = $2::uuid AND status = 'failed'`, itemID, sessionID).Scan(&stagingKey, &objectKey, &processedKey, &blobID, &processedBlobID); err != nil {
		return Item{}, ErrConflict
	}
	result, err := service.pool.Exec(ctx, `
		UPDATE upload_session_items
		SET status = 'pending', staging_object_key = NULL, object_key = NULL, blob_id = NULL,
			processed_object_key = NULL, processed_blob_id = NULL, processed_size_bytes = 0, processed_sha256 = NULL, processed_mime_type = NULL, size_bytes = 0, sha256 = NULL,
			error_message = NULL, updated_at = now()
		WHERE id = $1::uuid AND session_id = $2::uuid AND status = 'failed'`, itemID, sessionID)
	if err != nil {
		return Item{}, fmt.Errorf("重置失败文件项失败: %w", err)
	}
	if result.RowsAffected() == 0 {
		return Item{}, ErrConflict
	}
	for _, key := range uniqueKeys(stagingKey, processedKey) {
		if err := service.storage.Delete(ctx, key); err != nil {
			service.scheduleCleanup(ctx, key, "upload-retry")
		}
	}
	for _, value := range uniqueKeys(blobID, processedBlobID) {
		var used bool
		if err := service.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM attachments WHERE blob_id = $1::uuid OR current_blob_id = $1::uuid UNION ALL SELECT 1 FROM upload_session_items WHERE (blob_id = $1::uuid OR processed_blob_id = $1::uuid) AND id <> $2::uuid)`, value, itemID).Scan(&used); err == nil && !used {
			if err := service.pool.QueryRow(ctx, `SELECT storage_key FROM file_blobs WHERE id = $1::uuid`, value).Scan(&objectKey); err == nil {
				if deleteErr := service.storage.Delete(ctx, objectKey); deleteErr != nil {
					service.scheduleCleanup(ctx, objectKey, "upload-retry-blob")
				}
			}
			_, _ = service.pool.Exec(ctx, `DELETE FROM file_blobs WHERE id = $1::uuid AND NOT EXISTS (SELECT 1 FROM attachments WHERE blob_id = $1::uuid OR current_blob_id = $1::uuid) AND NOT EXISTS (SELECT 1 FROM upload_session_items WHERE blob_id = $1::uuid OR processed_blob_id = $1::uuid)`, value)
		}
	}
	return service.getItem(ctx, userID, sessionID, itemID)
}

func (service *Service) Commit(ctx context.Context, userID, sessionID string) (json.RawMessage, error) {
	if service == nil || service.pool == nil {
		return nil, errors.New("上传服务未配置")
	}
	tx, err := service.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("开始上传提交事务失败: %w", err)
	}
	defer tx.Rollback(ctx)
	var session Session
	var metadata []byte
	err = tx.QueryRow(ctx, `
		SELECT id::text, kind, idempotency_key, status, metadata, created_at, last_activity_at,
		       expires_at, absolute_expires_at, committed_at, COALESCE(error_message, '')
		FROM upload_sessions WHERE id = $1::uuid AND user_id = $2::uuid FOR UPDATE`, sessionID, userID).Scan(&session.ID, &session.Kind, &session.IdempotencyKey, &session.Status, &metadata, &session.CreatedAt, &session.LastActivityAt, &session.ExpiresAt, &session.AbsoluteExpiresAt, &session.CommittedAt, &session.ErrorMessage)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("锁定上传会话失败: %w", err)
	}
	if session.Status == "committed" {
		var result []byte
		if err := tx.QueryRow(ctx, `SELECT COALESCE(result, '{}'::jsonb) FROM upload_sessions WHERE id = $1::uuid`, sessionID).Scan(&result); err != nil {
			return nil, err
		}
		return json.RawMessage(result), nil
	}
	if session.Status != "open" && session.Status != "failed" {
		return nil, ErrConflict
	}
	if session.ExpiresAt.Before(time.Now()) || session.AbsoluteExpiresAt.Before(time.Now()) {
		return nil, ErrExpired
	}
	if session.Kind != "attachment" && session.Kind != "drawing-create" {
		return nil, errors.New("上传会话类型无效")
	}
	if session.Kind == "drawing-create" {
		result, err := service.commitDrawingCreateTx(ctx, tx, userID, sessionID, session, metadata)
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("提交项目上传事务失败: %w", err)
		}
		return result, nil
	}
	var item Item
	var objectKey, blobID, processedKey, processedBlobID string
	err = tx.QueryRow(ctx, `
		SELECT i.id::text, i.client_ref, COALESCE(i.attachment_id::text, ''), i.drawing_no, i.part_no,
		       i.file_role, i.original_name, i.mime_type, i.expected_revision, i.status,
		       COALESCE(i.object_key, ''), COALESCE(i.blob_id::text, ''), COALESCE(i.processed_object_key, ''),
		       COALESCE(i.processed_blob_id::text, ''), i.size_bytes, COALESCE(i.sha256, ''), i.attempts,
		       COALESCE(i.error_message, ''), i.updated_at
		FROM upload_session_items i WHERE i.session_id = $1::uuid ORDER BY i.created_at, i.id LIMIT 1`, sessionID).Scan(&item.ID, &item.ClientRef, &item.AttachmentID, &item.DrawingNo, &item.PartNo, &item.Role, &item.OriginalName, &item.MimeType, &item.ExpectedRevision, &item.Status, &objectKey, &blobID, &processedKey, &processedBlobID, &item.Size, &item.SHA256, &item.Attempts, &item.ErrorMessage, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrIncomplete
	}
	if err != nil {
		return nil, fmt.Errorf("读取待提交文件失败: %w", err)
	}
	var itemCount, readyCount int
	if err := tx.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE status = 'ready') FROM upload_session_items WHERE session_id = $1::uuid`, sessionID).Scan(&itemCount, &readyCount); err != nil {
		return nil, err
	}
	if itemCount != 1 || readyCount != itemCount || item.Status != "ready" {
		return nil, ErrIncomplete
	}
	currentKey := objectKey
	currentBlobID := blobID
	if processedKey != "" {
		currentKey = processedKey
		currentBlobID = processedBlobID
	}
	if currentBlobID == "" {
		return nil, errors.New("待提交文件缺少内容对象")
	}
	var attachmentID, version string
	if item.AttachmentID == "" {
		var ownerID string
		if item.PartNo == "" {
			err = tx.QueryRow(ctx, `SELECT id::text FROM drawings WHERE drawing_no = $1`, item.DrawingNo).Scan(&ownerID)
		} else {
			err = tx.QueryRow(ctx, `SELECT p.id::text FROM structure_parts p JOIN drawings d ON d.id = p.drawing_id WHERE d.drawing_no = $1 AND p.part_no = $2`, item.DrawingNo, item.PartNo).Scan(&ownerID)
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, fmt.Errorf("查询附件所属对象失败: %w", err)
		}
		if item.PartNo == "" {
			err = tx.QueryRow(ctx, `INSERT INTO attachments (drawing_id, file_role, storage_key, current_storage_key, original_name, current_name, mime_type, current_mime_type, size_bytes, current_size_bytes, sha256, current_sha256, version, previewable, uploaded_by, blob_id, current_blob_id) VALUES ($1::uuid, $2, $3, $4, $5, $5, $6, $6, $7, $7, $8, $8, 'v1.0', true, $9::uuid, $10::uuid, $11::uuid) RETURNING id::text`, ownerID, item.Role, objectKey, currentKey, item.OriginalName, item.MimeType, item.Size, item.SHA256, userID, blobID, currentBlobID).Scan(&attachmentID)
		} else {
			err = tx.QueryRow(ctx, `INSERT INTO attachments (part_id, file_role, storage_key, current_storage_key, original_name, current_name, mime_type, current_mime_type, size_bytes, current_size_bytes, sha256, current_sha256, version, previewable, uploaded_by, blob_id, current_blob_id) VALUES ($1::uuid, $2, $3, $4, $5, $5, $6, $6, $7, $7, $8, $8, 'v1.0', true, $9::uuid, $10::uuid, $11::uuid) RETURNING id::text`, ownerID, item.Role, objectKey, currentKey, item.OriginalName, item.MimeType, item.Size, item.SHA256, userID, blobID, currentBlobID).Scan(&attachmentID)
		}
		version = "v1.0"
	} else {
		if item.ExpectedRevision == nil {
			return nil, errors.New("替换附件必须提供原始版本号")
		}
		var currentRevision int64
		if err := tx.QueryRow(ctx, `SELECT revision FROM attachments WHERE id = $1::uuid AND deleted_at IS NULL FOR UPDATE`, item.AttachmentID).Scan(&currentRevision); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, ErrNotFound
			}
			return nil, err
		}
		if currentRevision != *item.ExpectedRevision {
			return nil, fmt.Errorf("文件版本已被其他用户修改，请刷新后重试: %w", ErrConflict)
		}
		version = fmt.Sprintf("v1.0-w%03d", currentRevision)
		result, updateErr := tx.Exec(ctx, `UPDATE attachments SET current_storage_key = $2, current_blob_id = $3::uuid, current_name = $4, current_mime_type = $5, current_size_bytes = $6, current_sha256 = $7, revision = revision + 1, version = $8 WHERE id = $1::uuid AND revision = $9 AND deleted_at IS NULL`, item.AttachmentID, currentKey, currentBlobID, item.OriginalName, item.MimeType, item.Size, item.SHA256, version, *item.ExpectedRevision)
		if updateErr != nil {
			return nil, fmt.Errorf("切换附件当前版本失败: %w", updateErr)
		}
		if result.RowsAffected() == 0 {
			return nil, fmt.Errorf("文件版本已被其他用户修改，请刷新后重试: %w", ErrConflict)
		}
		attachmentID = item.AttachmentID
	}
	if _, err := tx.Exec(ctx, `INSERT INTO file_versions (attachment_id, storage_key, source_storage_key, version, version_kind, size_bytes, mime_type, sha256, created_by, expires_at, is_pinned, is_current_release, blob_id) VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9::uuid, CASE WHEN $5 = 'working' THEN now() + interval '90 days' ELSE NULL END, $5 = 'release', $5 = 'release', $10::uuid)`, attachmentID, currentKey, objectKey, version, map[bool]string{true: "release", false: "working"}[item.AttachmentID == ""], item.Size, item.MimeType, item.SHA256, userID, currentBlobID); err != nil {
		return nil, fmt.Errorf("登记文件版本失败: %w", err)
	}
	result := map[string]any{"sessionId": sessionID, "attachmentId": attachmentID, "storageKey": objectKey, "currentStorageKey": currentKey, "version": version, "status": "committed"}
	resultBytes, _ := json.Marshal(result)
	if _, err := tx.Exec(ctx, `UPDATE upload_sessions SET status = 'committed', committed_at = now(), result = $2::jsonb, last_activity_at = now(), error_message = NULL WHERE id = $1::uuid AND status IN ('open', 'failed')`, sessionID, string(resultBytes)); err != nil {
		return nil, fmt.Errorf("保存上传会话结果失败: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE upload_session_items SET status = 'committed', attachment_id = $2::uuid, updated_at = now() WHERE session_id = $1::uuid`, sessionID, attachmentID); err != nil {
		return nil, fmt.Errorf("更新上传文件项状态失败: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("提交上传事务失败: %w", err)
	}
	return resultBytes, nil
}

func (service *Service) commitDrawingCreateTx(ctx context.Context, tx pgx.Tx, userID, sessionID string, session Session, metadata []byte) (json.RawMessage, error) {
	var manifest struct {
		Drawing  manifestDrawing  `json:"drawing"`
		Parts    []manifestPart   `json:"parts"`
		BOM      []manifestBOM    `json:"bom"`
		Borrows  []manifestBorrow `json:"borrows"`
		Branches []manifestBranch `json:"branches"`
	}
	if err := json.Unmarshal(metadata, &manifest); err != nil {
		return nil, fmt.Errorf("项目清单格式无效: %w", err)
	}
	if strings.TrimSpace(manifest.Drawing.No) == "" || strings.TrimSpace(manifest.Drawing.Name) == "" || strings.TrimSpace(manifest.Drawing.Project) == "" {
		return nil, errors.New("图号、名称和项目不能为空")
	}
	if manifest.Drawing.Kind == "" {
		manifest.Drawing.Kind = "总图"
	}
	if manifest.Drawing.Material == "" {
		manifest.Drawing.Material = "—"
	}
	if manifest.Drawing.Status == "" {
		manifest.Drawing.Status = "draft"
	}
	if manifest.Drawing.Version == "" {
		manifest.Drawing.Version = "v1.0"
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM upload_session_items WHERE session_id = $1::uuid`, sessionID).Scan(&count); err != nil {
		return nil, err
	}
	var ready int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM upload_session_items WHERE session_id = $1::uuid AND status = 'ready'`, sessionID).Scan(&ready); err != nil {
		return nil, err
	}
	if count != ready {
		return nil, ErrIncomplete
	}
	var drawingID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO drawings (drawing_no, name, project, kind, material, vendor, status, version, borrow_from, remark, created_by, updated_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::uuid, $11::uuid)
		RETURNING id::text`, manifest.Drawing.No, manifest.Drawing.Name, manifest.Drawing.Project, manifest.Drawing.Kind, manifest.Drawing.Material, manifest.Drawing.Vendor, manifest.Drawing.Status, manifest.Drawing.Version, manifest.Drawing.BorrowFrom, manifest.Drawing.Remark, userID).Scan(&drawingID); err != nil {
		if isUniqueViolation(err) {
			return nil, fmt.Errorf("图号已存在，请刷新后重试: %w", ErrConflict)
		}
		return nil, fmt.Errorf("创建项目图纸失败: %w", err)
	}
	for attributeID, fieldID := range manifest.Drawing.AttributeValues {
		if _, err := tx.Exec(ctx, `
			INSERT INTO drawing_attribute_values (drawing_id, attribute_id, field_id)
			SELECT $1::uuid, f.attribute_id, f.id
			FROM drawing_attribute_fields f
			WHERE f.attribute_id = $2::uuid AND f.id = $3::uuid`, drawingID, attributeID, fieldID); err != nil {
			return nil, fmt.Errorf("保存图纸属性失败: %w", err)
		}
	}
	if err := saveUploadSigners(ctx, tx, drawingID, manifest.Drawing.Signers, true); err != nil {
		return nil, fmt.Errorf("保存图纸签署人失败: %w", err)
	}
	partIDs := make(map[string]string, len(manifest.Parts))
	for _, part := range manifest.Parts {
		if strings.TrimSpace(part.No) == "" || strings.TrimSpace(part.Name) == "" {
			return nil, errors.New("零件图号和名称不能为空")
		}
		var id string
		if err := tx.QueryRow(ctx, `
			INSERT INTO structure_parts (drawing_id, part_no, name, project, material, spec, weight, surface_treatment, manufacturing_type, quantity, status, version, vendor, borrow_from, remark, created_by, updated_by)
			VALUES ($1::uuid, $2, $3, NULLIF($4, ''), COALESCE(NULLIF($5, ''), '—'), $6, $7, $8, COALESCE(NULLIF($9, ''), '自制件'), COALESCE(NULLIF($10, 0), 1), COALESCE(NULLIF($11, ''), 'draft'), COALESCE(NULLIF($12, ''), 'v1.0'), $13, $14, $15, $16::uuid, $16::uuid)
			RETURNING id::text`, drawingID, part.No, part.Name, firstNonEmpty(part.Project, manifest.Drawing.Project), part.Material, part.Spec, part.Weight, part.SurfaceTreatment, part.ManufacturingType, part.Quantity, part.Status, part.Version, part.Vendor, part.BorrowFrom, part.Remark, userID).Scan(&id); err != nil {
			if isUniqueViolation(err) {
				return nil, fmt.Errorf("零件图号已存在，请刷新后重试: %w", ErrConflict)
			}
			return nil, fmt.Errorf("创建零件失败: %w", err)
		}
		partIDs[part.No] = id
	}
	for _, part := range manifest.Parts {
		if strings.TrimSpace(part.ParentNo) == "" || part.ParentNo == manifest.Drawing.No {
			continue
		}
		parentID := partIDs[part.ParentNo]
		if parentID == "" {
			return nil, fmt.Errorf("零件父级不存在: %s", part.ParentNo)
		}
		if _, err := tx.Exec(ctx, `UPDATE structure_parts SET parent_part_id = $2::uuid WHERE id = $1::uuid`, partIDs[part.No], parentID); err != nil {
			return nil, fmt.Errorf("保存零件层级失败: %w", err)
		}
		if err := saveUploadSigners(ctx, tx, partIDs[part.No], part.Signers, false); err != nil {
			return nil, fmt.Errorf("保存零件签署人失败: %w", err)
		}
	}
	for _, part := range manifest.Parts {
		if part.ParentNo == "" || part.ParentNo == manifest.Drawing.No {
			if err := saveUploadSigners(ctx, tx, partIDs[part.No], part.Signers, false); err != nil {
				return nil, fmt.Errorf("保存零件签署人失败: %w", err)
			}
		}
	}
	type projectAttachment struct {
		id, partNo, role, name, mime, objectKey, blobID string
		processedKey, processedBlobID, sha256           string
		size                                            int64
	}
	rows, err := tx.Query(ctx, `
		SELECT id::text, part_no, file_role, original_name, mime_type,
		       object_key, blob_id::text, COALESCE(processed_object_key, ''), COALESCE(processed_blob_id::text, ''), size_bytes, sha256
		FROM upload_session_items WHERE session_id = $1::uuid AND status = 'ready' ORDER BY created_at, id`, sessionID)
	if err != nil {
		return nil, err
	}
	projectAttachments := make([]projectAttachment, 0, count)
	for rows.Next() {
		var item projectAttachment
		if err := rows.Scan(&item.id, &item.partNo, &item.role, &item.name, &item.mime, &item.objectKey, &item.blobID, &item.processedKey, &item.processedBlobID, &item.size, &item.sha256); err != nil {
			rows.Close()
			return nil, err
		}
		projectAttachments = append(projectAttachments, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	// pgx does not allow another command on the same transaction connection while
	// query rows are still open. Materialize the ready items first, then perform
	// attachment/version writes after closing the result set.
	rows.Close()
	for _, item := range projectAttachments {
		ownerID := drawingID
		if item.partNo != "" {
			ownerID = partIDs[item.partNo]
			if ownerID == "" {
				return nil, fmt.Errorf("附件所属零件不存在: %s", item.partNo)
			}
		}
		currentKey := item.objectKey
		currentBlobID := item.blobID
		if item.processedKey != "" {
			currentKey = item.processedKey
			currentBlobID = item.processedBlobID
		}
		var attachmentID string
		if item.partNo == "" {
			err = tx.QueryRow(ctx, `INSERT INTO attachments (drawing_id, file_role, storage_key, current_storage_key, original_name, current_name, mime_type, current_mime_type, size_bytes, current_size_bytes, sha256, current_sha256, version, previewable, uploaded_by, blob_id, current_blob_id) VALUES ($1::uuid, $2, $3, $4, $5, $5, $6, $6, $7, $7, $8, $8, 'v1.0', true, $9::uuid, $10::uuid, $11::uuid) RETURNING id::text`, ownerID, item.role, item.objectKey, currentKey, item.name, item.mime, item.size, item.sha256, userID, item.blobID, currentBlobID).Scan(&attachmentID)
		} else {
			err = tx.QueryRow(ctx, `INSERT INTO attachments (part_id, file_role, storage_key, current_storage_key, original_name, current_name, mime_type, current_mime_type, size_bytes, current_size_bytes, sha256, current_sha256, version, previewable, uploaded_by, blob_id, current_blob_id) VALUES ($1::uuid, $2, $3, $4, $5, $5, $6, $6, $7, $7, $8, $8, 'v1.0', true, $9::uuid, $10::uuid, $11::uuid) RETURNING id::text`, ownerID, item.role, item.objectKey, currentKey, item.name, item.mime, item.size, item.sha256, userID, item.blobID, currentBlobID).Scan(&attachmentID)
		}
		if err != nil {
			return nil, fmt.Errorf("创建项目附件失败: %w", err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO file_versions (attachment_id, storage_key, source_storage_key, version, version_kind, size_bytes, mime_type, sha256, created_by, is_pinned, is_current_release, blob_id) VALUES ($1::uuid, $2, $3, 'v1.0', 'release', $4, $5, $6, $7::uuid, true, true, $8::uuid)`, attachmentID, currentKey, item.objectKey, item.size, item.mime, item.sha256, userID, currentBlobID); err != nil {
			return nil, fmt.Errorf("创建项目初始版本失败: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE upload_session_items SET attachment_id = $2::uuid, status = 'committed', updated_at = now() WHERE id = $1::uuid`, item.id, attachmentID); err != nil {
			return nil, err
		}
	}
	for _, bom := range manifest.BOM {
		if bom.No <= 0 || strings.TrimSpace(bom.Name) == "" || bom.Quantity < 0 || bom.Weight < 0 {
			return nil, errors.New("BOM 项目字段无效")
		}
		if bom.DrawingNo != "" && bom.DrawingNo != manifest.Drawing.No {
			continue
		}
		if bom.PartNo == "" {
			if _, err := tx.Exec(ctx, `INSERT INTO bom_items (drawing_id, item_no, name, spec, quantity, weight, remark) VALUES ($1::uuid, $2, $3, COALESCE(NULLIF($4, ''), '—'), $5, $6, COALESCE($7, ''))`, drawingID, bom.No, bom.Name, bom.Spec, bom.Quantity, bom.Weight, bom.Remark); err != nil {
				return nil, fmt.Errorf("保存项目 BOM 失败: %w", err)
			}
			continue
		}
		partID := partIDs[bom.PartNo]
		if partID == "" {
			return nil, fmt.Errorf("BOM 所属零件不存在: %s", bom.PartNo)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO bom_items (part_id, item_no, name, spec, quantity, weight, remark) VALUES ($1::uuid, $2, $3, COALESCE(NULLIF($4, ''), '—'), $5, $6, COALESCE($7, ''))`, partID, bom.No, bom.Name, bom.Spec, bom.Quantity, bom.Weight, bom.Remark); err != nil {
			return nil, fmt.Errorf("保存零件 BOM 失败: %w", err)
		}
	}
	for _, borrow := range manifest.Borrows {
		if borrow.Direction != "in" && borrow.Direction != "out" {
			return nil, errors.New("借用记录方向无效")
		}
		if borrow.TargetPartNo != "" && partIDs[borrow.TargetPartNo] == "" {
			return nil, fmt.Errorf("借用目标零件不存在: %s", borrow.TargetPartNo)
		}
		status := borrow.Status
		if status == "" {
			status = "active"
		}
		if status == "使用中" {
			status = "active"
		} else if status == "已归档" {
			status = "archived"
		}
		if status != "active" && status != "archived" {
			return nil, errors.New("借用记录状态无效")
		}
		if _, err := tx.Exec(ctx, `
				INSERT INTO borrow_records (source_drawing_id, source_part_id, target_drawing_id, target_part_id, direction, status, created_by)
				VALUES (
					NULLIF((SELECT id::text FROM drawings WHERE drawing_no = NULLIF($1, '')), '')::uuid,
					NULLIF((SELECT p.id::text FROM structure_parts p JOIN drawings d ON d.id = p.drawing_id WHERE d.drawing_no = NULLIF($1, '') AND p.part_no = NULLIF($2, '')), '')::uuid,
					$3::uuid, NULLIF($4, '')::uuid, $5, $6, $7::uuid)`,
			borrow.SourceDrawingNo, borrow.SourcePartNo, drawingID, partIDs[borrow.TargetPartNo], borrow.Direction, status, userID); err != nil {
			return nil, fmt.Errorf("保存借用记录失败: %w", err)
		}
	}
	for _, branch := range manifest.Branches {
		if strings.TrimSpace(branch.SourceDrawingNo) == "" {
			return nil, errors.New("分支来源图号不能为空")
		}
		status := branch.Status
		if status == "" {
			status = "使用中"
		}
		result, err := tx.Exec(ctx, `
				INSERT INTO drawing_branches (source_drawing_id, target_drawing_id, name, description, status, created_by)
				SELECT id, $1::uuid, COALESCE(NULLIF($2, ''), $3), COALESCE($4, ''), $5, $6::uuid
				FROM drawings WHERE drawing_no = $7`, drawingID, branch.Name, manifest.Drawing.No, branch.Description, status, userID, branch.SourceDrawingNo)
		if err != nil {
			return nil, fmt.Errorf("保存项目分支记录失败: %w", err)
		}
		if result.RowsAffected() == 0 {
			return nil, fmt.Errorf("分支来源图纸不存在: %s", branch.SourceDrawingNo)
		}
	}
	auditMetadata, _ := json.Marshal(map[string]any{"uploadSessionId": sessionID, "partCount": len(manifest.Parts), "fileCount": count})
	if _, err := tx.Exec(ctx, `INSERT INTO audit_logs (actor_id, action, resource_type, resource_id, summary, metadata) VALUES ($1::uuid, 'create', 'drawing', $2::uuid, $3, $4::jsonb)`, userID, drawingID, "创建图纸 "+manifest.Drawing.No, string(auditMetadata)); err != nil {
		return nil, fmt.Errorf("保存项目操作日志失败: %w", err)
	}
	result := map[string]any{"sessionId": sessionID, "drawingId": drawingID, "drawingNo": manifest.Drawing.No, "status": "committed"}
	resultBytes, _ := json.Marshal(result)
	if _, err := tx.Exec(ctx, `UPDATE upload_sessions SET status = 'committed', committed_at = now(), result = $2::jsonb, last_activity_at = now(), error_message = NULL WHERE id = $1::uuid`, sessionID, string(resultBytes)); err != nil {
		return nil, err
	}
	return resultBytes, nil
}

func (service *Service) Snapshot(ctx context.Context, userID, sessionID string) (Snapshot, error) {
	session, err := service.getSession(ctx, userID, sessionID)
	if err != nil {
		return Snapshot{}, err
	}
	rows, err := service.pool.Query(ctx, `
		SELECT id::text, client_ref, COALESCE(attachment_id::text, ''), drawing_no, part_no, file_role,
		       original_name, mime_type, expected_revision, status, size_bytes, COALESCE(sha256, ''),
		       attempts, COALESCE(error_message, ''), updated_at
		FROM upload_session_items WHERE session_id = $1::uuid ORDER BY created_at, id`, sessionID)
	if err != nil {
		return Snapshot{}, fmt.Errorf("读取上传文件项失败: %w", err)
	}
	defer rows.Close()
	items := make([]Item, 0)
	for rows.Next() {
		var item Item
		if err := rows.Scan(&item.ID, &item.ClientRef, &item.AttachmentID, &item.DrawingNo, &item.PartNo, &item.Role, &item.OriginalName, &item.MimeType, &item.ExpectedRevision, &item.Status, &item.Size, &item.SHA256, &item.Attempts, &item.ErrorMessage, &item.UpdatedAt); err != nil {
			return Snapshot{}, fmt.Errorf("读取上传文件项失败: %w", err)
		}
		items = append(items, item)
	}
	return Snapshot{Session: session, Items: items}, rows.Err()
}

func (service *Service) Cancel(ctx context.Context, userID, sessionID string) error {
	result, err := service.pool.Exec(ctx, `
		UPDATE upload_sessions SET status = 'cancelled', last_activity_at = now()
		WHERE id = $1::uuid AND user_id = $2::uuid AND status IN ('open', 'failed')`, sessionID, userID)
	if err != nil {
		return fmt.Errorf("取消上传会话失败: %w", err)
	}
	if result.RowsAffected() == 0 {
		var status string
		if err := service.pool.QueryRow(ctx, `SELECT status FROM upload_sessions WHERE id = $1::uuid AND user_id = $2::uuid`, sessionID, userID).Scan(&status); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return fmt.Errorf("读取上传会话状态失败: %w", err)
		}
		if status != "expired" && status != "cancelled" {
			return ErrConflict
		}
	}
	return service.cleanupSessionObjects(ctx, sessionID, true)
}

func (service *Service) StartCleanup(ctx context.Context) {
	if service == nil || service.pool == nil || service.storage == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			_ = service.CleanupOnce(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (service *Service) CleanupOnce(ctx context.Context) error {
	service.cleanupMu.Lock()
	defer service.cleanupMu.Unlock()
	rows, err := service.pool.Query(ctx, `
		UPDATE upload_sessions SET status = 'expired'
		WHERE status IN ('open', 'failed') AND (expires_at <= now() OR absolute_expires_at <= now())
		RETURNING id::text`)
	if err != nil {
		return err
	}
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		if err := service.cleanupSessionObjects(ctx, id, true); err != nil {
			return err
		}
	}
	if err := service.processCleanupJobs(ctx); err != nil {
		return err
	}
	return service.cleanupOrphanBlobs(ctx)
}

func (service *Service) ensureBlob(ctx context.Context, stagingKey string, object storage.ObjectInfo) (string, string, error) {
	if len(object.SHA256) != 64 {
		return "", "", errors.New("内容对象缺少有效 SHA-256")
	}
	blobKey := filepath.ToSlash(filepath.Join("blobs", object.SHA256[:2], object.SHA256[2:4], object.SHA256))
	var blobID, existingKey string
	err := service.pool.QueryRow(ctx, `SELECT id::text, storage_key FROM file_blobs WHERE sha256 = $1 AND size_bytes = $2`, object.SHA256, object.Size).Scan(&blobID, &existingKey)
	if err == nil {
		if _, _, openErr := service.storage.Open(ctx, existingKey); openErr != nil {
			if _, putErr := service.copyObject(ctx, stagingKey, existingKey, object.MimeType); putErr != nil {
				return "", "", putErr
			}
		}
		return blobID, existingKey, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", "", fmt.Errorf("查询内容对象失败: %w", err)
	}
	if _, _, openErr := service.storage.Open(ctx, blobKey); openErr != nil {
		if _, putErr := service.copyObject(ctx, stagingKey, blobKey, object.MimeType); putErr != nil {
			return "", "", putErr
		}
	}
	err = service.pool.QueryRow(ctx, `
		INSERT INTO file_blobs (sha256, size_bytes, mime_type, storage_key)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (sha256, size_bytes) DO UPDATE SET mime_type = file_blobs.mime_type
		RETURNING id::text, storage_key`, object.SHA256, object.Size, object.MimeType, blobKey).Scan(&blobID, &existingKey)
	if err != nil {
		return "", "", fmt.Errorf("登记内容对象失败: %w", err)
	}
	return blobID, existingKey, nil
}

func (service *Service) openObjectInfo(ctx context.Context, key string) (storage.ObjectInfo, error) {
	reader, info, err := service.storage.Open(ctx, key)
	if err != nil {
		return storage.ObjectInfo{}, fmt.Errorf("打开 CAD 转换结果失败: %w", err)
	}
	hasher := sha256.New()
	size, copyErr := io.Copy(hasher, reader)
	closeErr := reader.Close()
	if copyErr != nil {
		return storage.ObjectInfo{}, fmt.Errorf("读取 CAD 转换结果失败: %w", copyErr)
	}
	if closeErr != nil {
		return storage.ObjectInfo{}, fmt.Errorf("关闭 CAD 转换结果失败: %w", closeErr)
	}
	info.Size = size
	info.SHA256 = hex.EncodeToString(hasher.Sum(nil))
	if info.Size <= 0 {
		return storage.ObjectInfo{}, errors.New("CAD 转换结果为空")
	}
	return info, nil
}

func (service *Service) copyObject(ctx context.Context, sourceKey, targetKey, mimeType string) (storage.ObjectInfo, error) {
	reader, _, err := service.storage.Open(ctx, sourceKey)
	if err != nil {
		return storage.ObjectInfo{}, fmt.Errorf("打开暂存文件失败: %w", err)
	}
	defer reader.Close()
	object, err := service.storage.Put(ctx, targetKey, reader, mimeType)
	if err != nil {
		return storage.ObjectInfo{}, fmt.Errorf("保存内容对象失败: %w", err)
	}
	return object, nil
}

func (service *Service) getSession(ctx context.Context, userID, sessionID string) (Session, error) {
	var item Session
	var metadata []byte
	err := service.pool.QueryRow(ctx, `
		SELECT id::text, kind, idempotency_key, status, metadata, created_at, last_activity_at,
		       expires_at, absolute_expires_at, committed_at, COALESCE(error_message, '')
		FROM upload_sessions WHERE id = $1::uuid AND user_id = $2::uuid`, sessionID, userID).Scan(&item.ID, &item.Kind, &item.IdempotencyKey, &item.Status, &metadata, &item.CreatedAt, &item.LastActivityAt, &item.ExpiresAt, &item.AbsoluteExpiresAt, &item.CommittedAt, &item.ErrorMessage)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, fmt.Errorf("读取上传会话失败: %w", err)
	}
	item.Metadata = json.RawMessage(metadata)
	if item.Status == "open" && (item.ExpiresAt.Before(time.Now()) || item.AbsoluteExpiresAt.Before(time.Now())) {
		return Session{}, ErrExpired
	}
	return item, nil
}

func (service *Service) getItem(ctx context.Context, userID, sessionID, itemID string) (Item, error) {
	if _, err := service.getSession(ctx, userID, sessionID); err != nil {
		return Item{}, err
	}
	var item Item
	err := service.pool.QueryRow(ctx, `
		SELECT id::text, client_ref, COALESCE(attachment_id::text, ''), drawing_no, part_no, file_role,
		       original_name, mime_type, expected_revision, expected_size_bytes, COALESCE(expected_sha256, ''), status, size_bytes, COALESCE(sha256, ''),
		       attempts, COALESCE(error_message, ''), updated_at
		FROM upload_session_items WHERE id = $1::uuid AND session_id = $2::uuid`, itemID, sessionID).Scan(&item.ID, &item.ClientRef, &item.AttachmentID, &item.DrawingNo, &item.PartNo, &item.Role, &item.OriginalName, &item.MimeType, &item.ExpectedRevision, &item.ExpectedSize, &item.ExpectedSHA256, &item.Status, &item.Size, &item.SHA256, &item.Attempts, &item.ErrorMessage, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	if err != nil {
		return Item{}, fmt.Errorf("读取上传文件项失败: %w", err)
	}
	return item, nil
}

func (service *Service) touchSession(ctx context.Context, userID, sessionID string) error {
	result, err := service.pool.Exec(ctx, `
		UPDATE upload_sessions
		SET last_activity_at = now(), expires_at = LEAST(now() + $3::interval, absolute_expires_at)
		WHERE id = $1::uuid AND user_id = $2::uuid AND status IN ('open', 'failed')
		  AND expires_at > now() AND absolute_expires_at > now()`, sessionID, userID, intervalText(service.ttl))
	if err != nil {
		return fmt.Errorf("更新上传会话活动时间失败: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrExpired
	}
	return nil
}

func (service *Service) markItemFailed(ctx context.Context, itemID string, err error) {
	_, _ = service.pool.Exec(ctx, `UPDATE upload_session_items SET status = 'failed', error_message = $2, updated_at = now() WHERE id = $1::uuid`, itemID, err.Error())
}

func (service *Service) cleanupSessionObjects(ctx context.Context, sessionID string, removeSession bool) error {
	rows, err := service.pool.Query(ctx, `
		SELECT staging_object_key, object_key, processed_object_key, blob_id::text, processed_blob_id::text
		FROM upload_session_items WHERE session_id = $1::uuid`, sessionID)
	if err != nil {
		return err
	}
	type objectRef struct {
		staging, object, processed, blobID, processedBlobID string
	}
	refs := make([]objectRef, 0)
	for rows.Next() {
		var ref objectRef
		if err := rows.Scan(&ref.staging, &ref.object, &ref.processed, &ref.blobID, &ref.processedBlobID); err != nil {
			rows.Close()
			return err
		}
		refs = append(refs, ref)
	}
	rows.Close()
	for _, ref := range refs {
		for _, key := range uniqueKeys(ref.staging, ref.processed) {
			if err := service.storage.Delete(ctx, key); err != nil {
				service.scheduleCleanup(ctx, key, "upload-session")
			}
		}
		for _, blobID := range uniqueKeys(ref.blobID, ref.processedBlobID) {
			if ref.object != "" && blobID != "" {
				var used bool
				if err := service.pool.QueryRow(ctx, `
					SELECT EXISTS (
						SELECT 1 FROM attachments WHERE blob_id = $1::uuid OR current_blob_id = $1::uuid
						UNION ALL
						SELECT 1 FROM file_versions WHERE blob_id = $1::uuid
						UNION ALL
						SELECT 1 FROM upload_session_items WHERE (blob_id = $1::uuid OR processed_blob_id = $1::uuid) AND session_id <> $2::uuid
				)`, blobID, sessionID).Scan(&used); err == nil && !used {
					var blobKey string
					if err := service.pool.QueryRow(ctx, `SELECT storage_key FROM file_blobs WHERE id = $1::uuid`, blobID).Scan(&blobKey); err != nil {
						continue
					}
					if err := service.storage.Delete(ctx, blobKey); err != nil {
						service.scheduleCleanup(ctx, blobKey, "upload-session-blob")
					} else {
						_, _ = service.pool.Exec(ctx, `DELETE FROM file_blobs WHERE id = $1::uuid AND NOT EXISTS (SELECT 1 FROM attachments WHERE blob_id = $1::uuid OR current_blob_id = $1::uuid) AND NOT EXISTS (SELECT 1 FROM file_versions WHERE blob_id = $1::uuid) AND NOT EXISTS (SELECT 1 FROM upload_session_items WHERE blob_id = $1::uuid OR processed_blob_id = $1::uuid)`, blobID)
					}
				}
			}
		}
	}
	chunkRows, chunkErr := service.pool.Query(ctx, `
			SELECT c.storage_key
			FROM upload_session_chunks c
			JOIN upload_session_items i ON i.id = c.item_id
			WHERE i.session_id = $1::uuid`, sessionID)
	if chunkErr != nil {
		return chunkErr
	}
	chunkKeys := make([]string, 0)
	for chunkRows.Next() {
		var key string
		if err := chunkRows.Scan(&key); err != nil {
			chunkRows.Close()
			return err
		}
		chunkKeys = append(chunkKeys, key)
	}
	chunkErr = chunkRows.Err()
	chunkRows.Close()
	if chunkErr != nil {
		return chunkErr
	}
	for _, key := range uniqueKeys(chunkKeys...) {
		if err := service.storage.Delete(ctx, key); err != nil {
			service.scheduleCleanup(ctx, key, "upload-session-chunk")
		}
	}
	if removeSession {
		_, err = service.pool.Exec(ctx, `DELETE FROM upload_sessions WHERE id = $1::uuid AND status IN ('expired', 'cancelled')`, sessionID)
	}
	return err
}

func (service *Service) processCleanupJobs(ctx context.Context) error {
	if _, err := service.pool.Exec(ctx, `
		UPDATE storage_cleanup_jobs
		SET status = 'pending', next_attempt_at = now(), last_error = '处理进程超时恢复', processing_started_at = NULL
		WHERE status = 'processing' AND processing_started_at < now() - interval '15 minutes'`); err != nil {
		return err
	}
	rows, err := service.pool.Query(ctx, `
		WITH candidates AS (
			SELECT id FROM storage_cleanup_jobs
			WHERE status = 'pending' AND next_attempt_at <= now()
			ORDER BY next_attempt_at, id LIMIT 100
			FOR UPDATE SKIP LOCKED
		)
		UPDATE storage_cleanup_jobs j
		SET status = 'processing', attempts = attempts + 1, processing_started_at = now()
		FROM candidates c WHERE j.id = c.id
		RETURNING j.id::text, j.storage_key`)
	if err != nil {
		return err
	}
	type job struct{ id, key string }
	jobs := make([]job, 0)
	for rows.Next() {
		var item job
		if err := rows.Scan(&item.id, &item.key); err != nil {
			rows.Close()
			return err
		}
		jobs = append(jobs, item)
	}
	rows.Close()
	for _, item := range jobs {
		if err := service.storage.Delete(ctx, item.key); err != nil {
			_, _ = service.pool.Exec(ctx, `UPDATE storage_cleanup_jobs SET status = 'pending', processing_started_at = NULL, last_error = $2, next_attempt_at = now() + interval '1 hour' WHERE id = $1::uuid AND status = 'processing'`, item.id, err.Error())
			continue
		}
		_, _ = service.pool.Exec(ctx, `UPDATE storage_cleanup_jobs SET status = 'completed', processing_started_at = NULL, completed_at = now(), last_error = NULL WHERE id = $1::uuid AND status = 'processing'`, item.id)
	}
	return nil
}

func (service *Service) cleanupOrphanBlobs(ctx context.Context) error {
	rows, err := service.pool.Query(ctx, `
		SELECT b.id::text, b.storage_key FROM file_blobs b
		WHERE b.created_at < now() - interval '1 hour'
		  AND NOT EXISTS (SELECT 1 FROM attachments a WHERE a.blob_id = b.id OR a.current_blob_id = b.id)
		  AND NOT EXISTS (SELECT 1 FROM file_versions v WHERE v.blob_id = b.id)
		  AND NOT EXISTS (SELECT 1 FROM upload_session_items i WHERE i.blob_id = b.id OR i.processed_blob_id = b.id)`)
	if err != nil {
		return err
	}
	type blob struct{ id, key string }
	blobs := make([]blob, 0)
	for rows.Next() {
		var item blob
		if err := rows.Scan(&item.id, &item.key); err != nil {
			rows.Close()
			return err
		}
		blobs = append(blobs, item)
	}
	rows.Close()
	for _, item := range blobs {
		if err := service.storage.Delete(ctx, item.key); err != nil {
			service.scheduleCleanup(ctx, item.key, "orphan-blob")
			continue
		}
		_, _ = service.pool.Exec(ctx, `DELETE FROM file_blobs WHERE id = $1::uuid AND NOT EXISTS (SELECT 1 FROM attachments WHERE blob_id = $1::uuid OR current_blob_id = $1::uuid) AND NOT EXISTS (SELECT 1 FROM file_versions WHERE blob_id = $1::uuid) AND NOT EXISTS (SELECT 1 FROM upload_session_items WHERE blob_id = $1::uuid OR processed_blob_id = $1::uuid)`, item.id)
	}
	return nil
}

func (service *Service) scheduleCleanup(ctx context.Context, key, reason string) {
	if strings.TrimSpace(key) == "" {
		return
	}
	_, _ = service.pool.Exec(ctx, `
		INSERT INTO storage_cleanup_jobs (storage_key, reason)
		VALUES ($1, $2)
		ON CONFLICT (storage_key) DO UPDATE SET status = 'pending', next_attempt_at = now(), last_error = NULL`, key, reason)
}

func intervalText(value time.Duration) string {
	return fmt.Sprintf("%f seconds", value.Seconds())
}

func uniqueKeys(values ...string) []string {
	seen := make(map[string]struct{}, len(values))
	keys := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		keys = append(keys, value)
	}
	return keys
}

func isCAD(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".exb" || ext == ".dwg" || ext == ".dxf"
}

func safeName(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		return "file"
	}
	return name
}

func nullableString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func isUniqueViolation(err error) bool {
	return strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") || strings.Contains(err.Error(), "SQLSTATE 23505")
}

func saveUploadSigners(ctx context.Context, tx pgx.Tx, ownerID string, signers map[string]string, drawing bool) error {
	column := "part_id"
	if drawing {
		column = "drawing_id"
	}
	if _, err := tx.Exec(ctx, `DELETE FROM drawing_signers WHERE `+column+` = $1::uuid`, ownerID); err != nil {
		return err
	}
	for role, name := range signers {
		if strings.TrimSpace(role) == "" || strings.TrimSpace(name) == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO drawing_signers (`+column+`, role, signer_name) VALUES ($1::uuid, $2, $3)`, ownerID, role, name); err != nil {
			return err
		}
	}
	return nil
}
