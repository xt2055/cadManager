\set ON_ERROR_STOP on

CREATE TABLE IF NOT EXISTS cad_conversion_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    upload_item_id UUID NOT NULL UNIQUE REFERENCES upload_session_items(id) ON DELETE CASCADE,
    attachment_id UUID REFERENCES attachments(id) ON DELETE CASCADE,
    source_blob_id UUID NOT NULL REFERENCES file_blobs(id) ON DELETE RESTRICT,
    source_storage_key VARCHAR(500) NOT NULL,
    source_name VARCHAR(255) NOT NULL,
    source_mime_type VARCHAR(255) NOT NULL DEFAULT 'application/octet-stream',
    source_size_bytes BIGINT NOT NULL CHECK (source_size_bytes >= 0),
    source_sha256 CHAR(64) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'processing', 'retry', 'failed', 'cancelled')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    lease_until TIMESTAMPTZ,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_cad_conversion_jobs_ready
    ON cad_conversion_jobs(status, next_attempt_at, created_at)
    WHERE attachment_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_cad_conversion_jobs_attachment
    ON cad_conversion_jobs(attachment_id)
    WHERE attachment_id IS NOT NULL;

GRANT SELECT, INSERT, UPDATE, DELETE ON cad_conversion_jobs TO cadguanliq_app;
