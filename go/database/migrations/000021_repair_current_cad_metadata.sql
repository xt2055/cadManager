\set ON_ERROR_STOP on

-- 早期上传会话已经把 EXB/DXF 转换产物写入 current_blob_id，
-- 但 current_name、current_mime_type、current_size_bytes、current_sha256
-- 仍沿用了原始文件信息。修正后原始名称继续保存在 original_name，
-- 当前对象的名称和元数据与实际 DWG 内容保持一致。
UPDATE attachments AS a
SET current_name = regexp_replace(a.original_name, '\\.[^.]+$', '.dwg'),
    current_mime_type = COALESCE(NULLIF(b.mime_type, ''), a.current_mime_type, a.mime_type),
    current_size_bytes = COALESCE(b.size_bytes, a.current_size_bytes, a.size_bytes),
    current_sha256 = COALESCE(b.sha256, a.current_sha256, a.sha256)
FROM file_blobs AS b
WHERE b.id = a.current_blob_id
  AND a.deleted_at IS NULL
  AND lower(a.original_name) ~ '\\.(exb|dxf)$'
  AND lower(COALESCE(a.current_name, '')) !~ '\\.dwg$'
  AND lower(COALESCE(b.mime_type, '')) LIKE '%acad%';
