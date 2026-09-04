\set ON_ERROR_STOP on

-- 000017 曾在开发期间继续追加字段。已经执行过早期版本的数据库不会
-- 自动重放同名迁移，因此用新的迁移编号显式修复所有上传会话增量字段。
ALTER TABLE upload_session_items
    ADD COLUMN IF NOT EXISTS staging_object_key VARCHAR(500),
    ADD COLUMN IF NOT EXISTS processed_blob_id UUID,
    ADD COLUMN IF NOT EXISTS processed_size_bytes BIGINT,
    ADD COLUMN IF NOT EXISTS processed_sha256 CHAR(64),
    ADD COLUMN IF NOT EXISTS processed_mime_type VARCHAR(255),
    ADD COLUMN IF NOT EXISTS expected_size_bytes BIGINT,
    ADD COLUMN IF NOT EXISTS expected_sha256 CHAR(64),
    ADD COLUMN IF NOT EXISTS chunk_size_bytes BIGINT;

UPDATE upload_session_items
SET processed_size_bytes = 0
WHERE processed_size_bytes IS NULL;

ALTER TABLE upload_session_items
    ALTER COLUMN processed_size_bytes SET DEFAULT 0,
    ALTER COLUMN processed_size_bytes SET NOT NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conrelid = 'upload_session_items'::regclass
          AND conname = 'upload_session_items_processed_size_bytes_check'
    ) THEN
        ALTER TABLE upload_session_items
            ADD CONSTRAINT upload_session_items_processed_size_bytes_check
            CHECK (processed_size_bytes >= 0);
    END IF;
END
$$;

ALTER TABLE upload_session_items
    DROP CONSTRAINT IF EXISTS upload_session_items_processed_blob_id_fkey;
ALTER TABLE upload_session_items
    ADD CONSTRAINT upload_session_items_processed_blob_id_fkey
    FOREIGN KEY (processed_blob_id) REFERENCES file_blobs(id) ON DELETE SET NULL;

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

CREATE INDEX IF NOT EXISTS idx_upload_session_chunks_item
    ON upload_session_chunks(item_id, part_number);

ALTER TABLE storage_cleanup_jobs
    ADD COLUMN IF NOT EXISTS processing_started_at TIMESTAMPTZ;

GRANT SELECT, INSERT, UPDATE, DELETE ON upload_session_items TO cadguanliq_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON upload_session_chunks TO cadguanliq_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON storage_cleanup_jobs TO cadguanliq_app;
