\set ON_ERROR_STOP on

-- Phase 2 原子命令的持久化幂等结果。
CREATE TABLE IF NOT EXISTS domain_idempotency_records (
    scope VARCHAR(255) NOT NULL,
    idempotency_key VARCHAR(255) NOT NULL,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    result JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (scope, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_domain_idempotency_created_at
    ON domain_idempotency_records(created_at);

GRANT SELECT, INSERT ON domain_idempotency_records TO cadguanliq_app;
