-- 更新管理：为 update_manifests 补充安装包文件信息
ALTER TABLE update_manifests ADD COLUMN IF NOT EXISTS file_name VARCHAR(255) NOT NULL DEFAULT '';
ALTER TABLE update_manifests ADD COLUMN IF NOT EXISTS size_bytes BIGINT NOT NULL DEFAULT 0;
