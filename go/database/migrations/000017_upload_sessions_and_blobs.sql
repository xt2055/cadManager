\set ON_ERROR_STOP on

ALTER TABLE attachments
    ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS blob_id UUID,
    ADD COLUMN IF NOT EXISTS current_blob_id UUID;

CREATE TABLE IF NOT EXISTS file_blobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    sha256 CHAR(64) NOT NULL,
    size_bytes BIGINT NOT NULL CHECK (size_bytes >= 0),
    mime_type VARCHAR(255) NOT NULL DEFAULT 'application/octet-stream',
    storage_key VARCHAR(500) NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (sha256, size_bytes)
);

ALTER TABLE file_versions
    ADD COLUMN IF NOT EXISTS blob_id UUID;

ALTER TABLE attachments
    DROP CONSTRAINT IF EXISTS attachments_blob_id_fkey;
ALTER TABLE attachments
    ADD CONSTRAINT attachments_blob_id_fkey FOREIGN KEY (blob_id) REFERENCES file_blobs(id) ON DELETE SET NULL;
ALTER TABLE attachments
    DROP CONSTRAINT IF EXISTS attachments_current_blob_id_fkey;
ALTER TABLE attachments
    ADD CONSTRAINT attachments_current_blob_id_fkey FOREIGN KEY (current_blob_id) REFERENCES file_blobs(id) ON DELETE SET NULL;
ALTER TABLE file_versions
    DROP CONSTRAINT IF EXISTS file_versions_blob_id_fkey;
ALTER TABLE file_versions
    ADD CONSTRAINT file_versions_blob_id_fkey FOREIGN KEY (blob_id) REFERENCES file_blobs(id) ON DELETE SET NULL;

CREATE TABLE IF NOT EXISTS upload_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind VARCHAR(30) NOT NULL CHECK (kind IN ('attachment', 'drawing-create')),
    idempotency_key VARCHAR(255) NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    status VARCHAR(20) NOT NULL DEFAULT 'open'
        CHECK (status IN ('open', 'committing', 'committed', 'cancelled', 'expired', 'failed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_activity_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    absolute_expires_at TIMESTAMPTZ NOT NULL,
    committed_at TIMESTAMPTZ,
    result JSONB,
    error_message TEXT,
    UNIQUE (user_id, idempotency_key)
);

CREATE TABLE IF NOT EXISTS upload_session_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID NOT NULL REFERENCES upload_sessions(id) ON DELETE CASCADE,
    client_ref VARCHAR(255) NOT NULL,
    attachment_id UUID REFERENCES attachments(id) ON DELETE SET NULL,
    drawing_no VARCHAR(150) NOT NULL DEFAULT '',
    part_no VARCHAR(150) NOT NULL DEFAULT '',
    file_role VARCHAR(20) NOT NULL DEFAULT 'other'
        CHECK (file_role IN ('assembly', 'part', 'material', 'craft', 'other')),
    original_name VARCHAR(255) NOT NULL,
    mime_type VARCHAR(255) NOT NULL DEFAULT 'application/octet-stream',
    expected_revision BIGINT,
    status VARCHAR(20) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'uploading', 'ready', 'failed', 'committed')),
    object_key VARCHAR(500),
    staging_object_key VARCHAR(500),
    blob_id UUID REFERENCES file_blobs(id) ON DELETE SET NULL,
    processed_object_key VARCHAR(500),
    processed_blob_id UUID REFERENCES file_blobs(id) ON DELETE SET NULL,
    processed_size_bytes BIGINT NOT NULL DEFAULT 0 CHECK (processed_size_bytes >= 0),
    processed_sha256 CHAR(64),
    processed_mime_type VARCHAR(255),
    size_bytes BIGINT NOT NULL DEFAULT 0 CHECK (size_bytes >= 0),
    sha256 CHAR(64),
    error_message TEXT,
    attempts INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (session_id, client_ref)
);

CREATE TABLE IF NOT EXISTS storage_cleanup_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    storage_key VARCHAR(500) NOT NULL UNIQUE,
    reason VARCHAR(50) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'processing', 'completed')),
    attempts INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_upload_sessions_expiration
    ON upload_sessions(status, expires_at);
CREATE INDEX IF NOT EXISTS idx_upload_session_items_session
    ON upload_session_items(session_id, status);
CREATE INDEX IF NOT EXISTS idx_storage_cleanup_jobs_due
    ON storage_cleanup_jobs(status, next_attempt_at);

ALTER TABLE upload_sessions
    ADD COLUMN IF NOT EXISTS absolute_expires_at TIMESTAMPTZ;
UPDATE upload_sessions
SET absolute_expires_at = COALESCE(absolute_expires_at, expires_at);
ALTER TABLE upload_sessions
    ALTER COLUMN absolute_expires_at SET NOT NULL;
ALTER TABLE upload_session_items
    ADD COLUMN IF NOT EXISTS staging_object_key VARCHAR(500),
    ADD COLUMN IF NOT EXISTS processed_blob_id UUID,
    ADD COLUMN IF NOT EXISTS processed_size_bytes BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS processed_sha256 CHAR(64),
    ADD COLUMN IF NOT EXISTS processed_mime_type VARCHAR(255);
ALTER TABLE upload_session_items
    DROP CONSTRAINT IF EXISTS upload_session_items_processed_blob_id_fkey;
ALTER TABLE upload_session_items
    ADD CONSTRAINT upload_session_items_processed_blob_id_fkey FOREIGN KEY (processed_blob_id) REFERENCES file_blobs(id) ON DELETE SET NULL;

ALTER TABLE file_versions DROP CONSTRAINT IF EXISTS file_versions_storage_key_key;
ALTER TABLE file_versions DROP CONSTRAINT IF EXISTS file_versions_storage_key_unique;

-- 允许多个附件/版本引用同一个内容对象；版本号在同一附件内仍必须唯一。
CREATE UNIQUE INDEX IF NOT EXISTS uq_file_versions_attachment_version_active
    ON file_versions(attachment_id, version)
    WHERE deleted_at IS NULL;

ALTER TABLE storage_cleanup_jobs
    ADD COLUMN IF NOT EXISTS processing_started_at TIMESTAMPTZ;

GRANT SELECT, INSERT, UPDATE, DELETE ON attachments TO cadguanliq_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON file_blobs TO cadguanliq_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON upload_sessions TO cadguanliq_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON upload_session_items TO cadguanliq_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON storage_cleanup_jobs TO cadguanliq_app;
