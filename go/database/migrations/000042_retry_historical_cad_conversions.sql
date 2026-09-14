\set ON_ERROR_STOP on

-- 转换器已生成文件，但旧队列实现覆盖历史版本被拒绝；新版新增工作版本。
-- 仅恢复这一已知错误，不重启被取消的任务或其他原因的失败任务。
UPDATE cad_conversion_jobs
SET status = 'retry', attempts = 0, next_attempt_at = now(),
    lease_until = NULL, last_error = NULL, updated_at = now()
WHERE status IN ('failed', 'retry', 'backoff')
  AND last_error LIKE '%历史版本内容不可覆盖或隐藏%';
