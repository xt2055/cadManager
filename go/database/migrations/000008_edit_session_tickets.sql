\set ON_ERROR_STOP on

CREATE TABLE IF NOT EXISTS edit_session_tickets (
    token_hash CHAR(64) PRIMARY KEY,
    session_id UUID NOT NULL REFERENCES edit_sessions(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_edit_session_tickets_expiry
    ON edit_session_tickets(expires_at)
    WHERE used_at IS NULL;

GRANT SELECT, INSERT, UPDATE, DELETE ON edit_session_tickets TO cadguanliq_app;
