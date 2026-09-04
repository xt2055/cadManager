package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type schemaRequirement struct {
	table   string
	columns []string
}

var uploadSchemaRequirements = []schemaRequirement{
	{table: "attachments", columns: []string{"revision", "blob_id", "current_blob_id"}},
	{table: "file_versions", columns: []string{"blob_id"}},
	{table: "file_blobs", columns: []string{"id", "sha256", "size_bytes", "mime_type", "storage_key", "created_at"}},
	{table: "upload_sessions", columns: []string{"id", "user_id", "kind", "idempotency_key", "metadata", "status", "created_at", "last_activity_at", "expires_at", "absolute_expires_at", "committed_at", "result", "error_message"}},
	{table: "upload_session_items", columns: []string{"id", "session_id", "client_ref", "attachment_id", "drawing_no", "part_no", "file_role", "original_name", "mime_type", "expected_revision", "status", "object_key", "staging_object_key", "blob_id", "processed_object_key", "processed_blob_id", "processed_size_bytes", "processed_sha256", "processed_mime_type", "size_bytes", "sha256", "error_message", "attempts", "created_at", "updated_at", "expected_size_bytes", "expected_sha256", "chunk_size_bytes"}},
	{table: "upload_session_chunks", columns: []string{"id", "item_id", "part_number", "offset_bytes", "size_bytes", "sha256", "storage_key", "created_at", "updated_at"}},
	{table: "storage_cleanup_jobs", columns: []string{"id", "storage_key", "reason", "status", "attempts", "next_attempt_at", "last_error", "created_at", "completed_at", "processing_started_at"}},
}

const repairUploadSchemaSQL = `
	ALTER TABLE upload_session_items
		ADD COLUMN IF NOT EXISTS staging_object_key VARCHAR(500),
		ADD COLUMN IF NOT EXISTS processed_blob_id UUID,
		ADD COLUMN IF NOT EXISTS processed_size_bytes BIGINT,
		ADD COLUMN IF NOT EXISTS processed_sha256 CHAR(64),
		ADD COLUMN IF NOT EXISTS processed_mime_type VARCHAR(255),
		ADD COLUMN IF NOT EXISTS expected_size_bytes BIGINT,
		ADD COLUMN IF NOT EXISTS expected_sha256 CHAR(64),
		ADD COLUMN IF NOT EXISTS chunk_size_bytes BIGINT;
	UPDATE upload_session_items SET processed_size_bytes = 0 WHERE processed_size_bytes IS NULL;
	ALTER TABLE upload_session_items
		ALTER COLUMN processed_size_bytes SET DEFAULT 0,
		ALTER COLUMN processed_size_bytes SET NOT NULL;
	CREATE TABLE IF NOT EXISTS upload_session_chunks (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		item_id UUID NOT NULL REFERENCES upload_session_items(id) ON DELETE CASCADE,
		part_number INTEGER NOT NULL CHECK (part_number >= 0),
		offset_bytes BIGINT NOT NULL CHECK (offset_bytes >= 0),
		size_bytes BIGINT NOT NULL CHECK (size_bytes > 0),
		sha256 CHAR(64) NOT NULL,
		storage_key VARCHAR(500) NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		UNIQUE (item_id, part_number)
	);
	CREATE INDEX IF NOT EXISTS idx_upload_session_chunks_item ON upload_session_chunks(item_id, part_number);
	ALTER TABLE storage_cleanup_jobs ADD COLUMN IF NOT EXISTS processing_started_at TIMESTAMPTZ;
`

// ensureUploadSchema prevents a new binary from serving requests against an older,
// partially migrated upload schema. It only attempts DDL when required fields are
// actually missing, so a least-privilege runtime account can still start after an
// administrator has applied the migrations.
func ensureUploadSchema(ctx context.Context, pool *pgxpool.Pool) error {
	missing, err := findMissingUploadSchema(ctx, pool)
	if err != nil {
		return fmt.Errorf("检查上传模块数据库结构失败: %w", err)
	}
	if len(missing) == 0 {
		return nil
	}
	if _, err := pool.Exec(ctx, repairUploadSchemaSQL); err != nil {
		return fmt.Errorf("上传模块数据库结构不兼容（缺少 %s），自动修复失败，请执行 000019_repair_upload_schema.sql: %w", strings.Join(missing, ", "), err)
	}
	remaining, err := findMissingUploadSchema(ctx, pool)
	if err != nil {
		return fmt.Errorf("复查上传模块数据库结构失败: %w", err)
	}
	if len(remaining) != 0 {
		return fmt.Errorf("上传模块数据库结构仍不完整（缺少 %s），请执行 000017、000018 和 000019 迁移", strings.Join(remaining, ", "))
	}
	return nil
}

func findMissingUploadSchema(ctx context.Context, pool *pgxpool.Pool) ([]string, error) {
	missing := make([]string, 0)
	for _, requirement := range uploadSchemaRequirements {
		rows, err := pool.Query(ctx, `
			SELECT column_name
			FROM information_schema.columns
			WHERE table_schema = 'public' AND table_name = $1`, requirement.table)
		if err != nil {
			return nil, err
		}
		present := make(map[string]struct{}, len(requirement.columns))
		for rows.Next() {
			var column string
			if err := rows.Scan(&column); err != nil {
				rows.Close()
				return nil, err
			}
			present[column] = struct{}{}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
		if len(present) == 0 {
			missing = append(missing, requirement.table+".*")
			continue
		}
		for _, column := range requirement.columns {
			if _, ok := present[column]; !ok {
				missing = append(missing, requirement.table+"."+column)
			}
		}
	}
	return missing, nil
}
