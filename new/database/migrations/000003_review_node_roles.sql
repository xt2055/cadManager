\set ON_ERROR_STOP on

ALTER TABLE review_flow_nodes
    ADD COLUMN IF NOT EXISTS candidate_role VARCHAR(30) NOT NULL DEFAULT 'reviewer';

ALTER TABLE review_case_nodes
    ADD COLUMN IF NOT EXISTS candidate_role VARCHAR(30) NOT NULL DEFAULT 'reviewer';

ALTER TABLE sessions
    ADD COLUMN IF NOT EXISTS last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now();

CREATE INDEX IF NOT EXISTS idx_sessions_last_seen_at ON sessions(last_seen_at);

CREATE TABLE IF NOT EXISTS drawing_branches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_drawing_id UUID NOT NULL REFERENCES drawings(id) ON DELETE CASCADE,
    target_drawing_id UUID NOT NULL REFERENCES drawings(id) ON DELETE CASCADE,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    description TEXT NOT NULL DEFAULT '',
    status VARCHAR(20) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'disabled')),
    UNIQUE(source_drawing_id, target_drawing_id)
);

CREATE INDEX IF NOT EXISTS idx_drawing_branches_source ON drawing_branches(source_drawing_id);
CREATE INDEX IF NOT EXISTS idx_drawing_branches_target ON drawing_branches(target_drawing_id);

GRANT SELECT, INSERT, UPDATE, DELETE ON drawing_branches TO cadguanliq_app;
