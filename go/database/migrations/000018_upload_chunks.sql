\set ON_ERROR_STOP on

ALTER TABLE upload_session_items
    ADD COLUMN IF NOT EXISTS expected_size_bytes BIGINT,
    ADD COLUMN IF NOT EXISTS expected_sha256 CHAR(64),
    ADD COLUMN IF NOT EXISTS chunk_size_bytes BIGINT;

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

GRANT SELECT, INSERT, UPDATE, DELETE ON upload_session_chunks TO cadguanliq_app;
