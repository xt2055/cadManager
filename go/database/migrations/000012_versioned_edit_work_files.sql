\set ON_ERROR_STOP on

-- 本地编辑会话记录实际复制到 SMB 的工作文件（当前版本 key），
-- 原始附件可能是 .exb，工作文件位于 history 版本目录，不能再靠后缀替换推算。
ALTER TABLE edit_sessions
    ADD COLUMN IF NOT EXISTS work_storage_key VARCHAR(500);

-- 同一附件同一版本号只允许一条记录，配合事务内行锁防止重复结束编辑生成重复版本。
-- 旧数据可能因时间戳文件名产生重复版本号，先保留每组最新一条再建唯一索引。
DELETE FROM file_versions a
    USING file_versions b
    WHERE a.attachment_id = b.attachment_id
      AND a.version = b.version
      AND a.deleted_at IS NULL
      AND b.deleted_at IS NULL
      AND a.created_at < b.created_at;

CREATE UNIQUE INDEX IF NOT EXISTS uq_file_versions_attachment_version
    ON file_versions(attachment_id, version)
    WHERE deleted_at IS NULL;

GRANT SELECT, INSERT, UPDATE, DELETE ON edit_sessions TO cadguanliq_app;
