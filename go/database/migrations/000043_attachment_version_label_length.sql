\set ON_ERROR_STOP on

-- 工作版本在已有标签后追加 -wNNN，合法的旧标签追加后可能超过 50 字符。
-- 保留完整标签和历史引用，避免结束编辑时因截断限制而无法登记版本。
ALTER TABLE attachment_versions ALTER COLUMN version TYPE TEXT;
