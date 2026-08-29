\set ON_ERROR_STOP on

CREATE TABLE IF NOT EXISTS file_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    attachment_id UUID NOT NULL REFERENCES attachments(id) ON DELETE CASCADE,
    storage_key VARCHAR(500) NOT NULL UNIQUE,
    source_storage_key VARCHAR(500) NOT NULL,
    version VARCHAR(50) NOT NULL,
    version_kind VARCHAR(20) NOT NULL DEFAULT 'working'
        CHECK (version_kind IN ('working', 'release')),
    size_bytes BIGINT NOT NULL DEFAULT 0 CHECK (size_bytes >= 0),
    mime_type VARCHAR(255) NOT NULL DEFAULT 'application/octet-stream',
    sha256 CHAR(64) NOT NULL,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ,
    is_pinned BOOLEAN NOT NULL DEFAULT false,
    pinned_by UUID REFERENCES users(id) ON DELETE SET NULL,
    pinned_at TIMESTAMPTZ,
    released_by UUID REFERENCES users(id) ON DELETE SET NULL,
    released_at TIMESTAMPTZ,
    is_current_release BOOLEAN NOT NULL DEFAULT false,
    deleted_at TIMESTAMPTZ
);

ALTER TABLE file_versions
    ADD COLUMN IF NOT EXISTS is_current_release BOOLEAN NOT NULL DEFAULT false;

CREATE INDEX IF NOT EXISTS idx_file_versions_attachment_created
    ON file_versions(attachment_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_file_versions_cleanup
    ON file_versions(expires_at)
    WHERE version_kind = 'working' AND is_pinned = false AND deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_file_versions_current_release
    ON file_versions(attachment_id)
    WHERE is_current_release = true AND deleted_at IS NULL;

GRANT SELECT, INSERT, UPDATE, DELETE ON file_versions TO cadguanliq_app;
