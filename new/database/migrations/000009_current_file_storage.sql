\set ON_ERROR_STOP on

-- 原始附件与当前可预览/可编辑文件分离。原始文件只读保留，当前文件统一为 CAD 工作格式。
ALTER TABLE attachments
    ADD COLUMN IF NOT EXISTS current_storage_key VARCHAR(500),
    ADD COLUMN IF NOT EXISTS current_name VARCHAR(255),
    ADD COLUMN IF NOT EXISTS current_mime_type VARCHAR(255),
    ADD COLUMN IF NOT EXISTS current_size_bytes BIGINT,
    ADD COLUMN IF NOT EXISTS current_sha256 CHAR(64);

UPDATE attachments
SET current_storage_key = COALESCE(current_storage_key, storage_key),
    current_name = COALESCE(current_name, original_name),
    current_mime_type = COALESCE(current_mime_type, mime_type),
    current_size_bytes = COALESCE(current_size_bytes, size_bytes),
    current_sha256 = COALESCE(current_sha256, sha256)
WHERE current_storage_key IS NULL
   OR current_name IS NULL
   OR current_mime_type IS NULL
   OR current_size_bytes IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_attachments_current_storage_key
    ON attachments(current_storage_key)
    WHERE current_storage_key IS NOT NULL AND deleted_at IS NULL;

GRANT SELECT, INSERT, UPDATE, DELETE ON attachments TO cadguanliq_app;
