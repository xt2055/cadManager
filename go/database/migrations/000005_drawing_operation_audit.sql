\set ON_ERROR_STOP on

CREATE INDEX IF NOT EXISTS idx_audit_logs_drawing_no
    ON audit_logs ((metadata->>'drawingNo'))
    WHERE metadata ? 'drawingNo';
