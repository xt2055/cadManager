\set ON_ERROR_STOP on
BEGIN;

-- 记录资料由哪个业务入口上传（图纸档案、变更工单、专利缴费等），用于资料档案展示来源。
ALTER TABLE lifecycle_documents ADD COLUMN IF NOT EXISTS source text NOT NULL DEFAULT '';

COMMIT;
