\set ON_ERROR_STOP on

-- 队列改为"永不放弃"策略：快速重试(5次)用尽后进入 backoff 慢速重试（每 10 分钟），
-- 旧 CHECK 约束不含 backoff，会导致重试状态写入失败（23514）。
ALTER TABLE cad_conversion_jobs DROP CONSTRAINT IF EXISTS cad_conversion_jobs_status_check;

ALTER TABLE cad_conversion_jobs ADD CONSTRAINT cad_conversion_jobs_status_check
    CHECK (status IN ('pending', 'processing', 'retry', 'backoff', 'failed', 'cancelled'));
