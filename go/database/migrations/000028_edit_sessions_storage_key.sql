\set ON_ERROR_STOP on

-- 000022 重建域模型时遗漏了 edit_sessions.storage_key（原始附件键）。
-- 编辑会话占用判定（FindActiveByStorageKey）按该列查询，缺失时创建会话直接报 42703。
ALTER TABLE edit_sessions ADD COLUMN IF NOT EXISTS storage_key VARCHAR(500);

-- 存量会话回填：键在 file_blobs（域模型重建后 attachments 无 storage_key 列），
-- 取当前版本指向的 blob 键，与会话创建时代码写入的值一致。
UPDATE edit_sessions s
SET storage_key = b.storage_key
FROM attachments a
JOIN attachment_versions v ON v.id = a.current_version_id
JOIN file_blobs b ON b.id = v.blob_id
WHERE s.attachment_id = a.id AND (s.storage_key IS NULL OR s.storage_key = '');

-- 会话创建后该列必有值；新增索引加速占用查询。
CREATE INDEX IF NOT EXISTS idx_edit_sessions_storage_key
    ON edit_sessions(storage_key) WHERE status = 'active';
