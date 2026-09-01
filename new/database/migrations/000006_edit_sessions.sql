\set ON_ERROR_STOP on

CREATE TABLE IF NOT EXISTS edit_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    attachment_id UUID NOT NULL REFERENCES attachments(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    storage_key VARCHAR(500) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'closed', 'expired', 'conflict')),
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    closed_at TIMESTAMPTZ
);

ALTER TABLE edit_sessions
    ADD COLUMN IF NOT EXISTS storage_key VARCHAR(500);

CREATE INDEX IF NOT EXISTS idx_edit_sessions_attachment_active
    ON edit_sessions(attachment_id, status)
    WHERE status = 'active';

CREATE UNIQUE INDEX IF NOT EXISTS uq_edit_sessions_one_active_attachment
    ON edit_sessions(attachment_id)
    WHERE status = 'active';

GRANT SELECT, INSERT, UPDATE, DELETE ON edit_sessions TO cadguanliq_app;
