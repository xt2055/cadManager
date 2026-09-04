\set ON_ERROR_STOP on

-- 已删除附件不应继续占用当前存储键；历史记录保留，但同一键可以重新上传。
ALTER TABLE attachments DROP CONSTRAINT IF EXISTS attachments_storage_key_key;
CREATE UNIQUE INDEX IF NOT EXISTS uq_attachments_storage_key_active
    ON attachments(storage_key)
    WHERE deleted_at IS NULL;

GRANT SELECT, INSERT, UPDATE, DELETE ON attachments TO cadguanliq_app;
