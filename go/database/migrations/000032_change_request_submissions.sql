\set ON_ERROR_STOP on

-- 提交快照轮次：每次"提交完成"形成一个不可变轮次，
-- 记录本轮实际修改说明、拟发布属性、基线/成果文件版本；差异按轮次归属，
-- 使验收所见即所发布，退回重提不再与上一轮差异混淆。

CREATE TABLE change_request_submissions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    request_id UUID NOT NULL REFERENCES change_requests(id) ON DELETE CASCADE,
    round INTEGER NOT NULL CHECK (round > 0),
    submitted_by UUID REFERENCES users(id) ON DELETE SET NULL,
    actual_changes TEXT NOT NULL DEFAULT '',
    proposed_attributes JSONB NOT NULL DEFAULT '{}'::jsonb,
    base_attachment_version_id UUID REFERENCES attachment_versions(id) ON DELETE SET NULL,
    submitted_attachment_version_id UUID REFERENCES attachment_versions(id) ON DELETE SET NULL,
    -- pending=当前待验收，returned=已被退回（历史），accepted=已通过验收发布。
    status VARCHAR(20) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'returned', 'accepted')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (request_id, round)
);

CREATE INDEX idx_change_request_submissions_request ON change_request_submissions(request_id, round DESC);

-- 工单指向当前待验收/最近一次提交快照；发布时以该快照为准。
ALTER TABLE change_requests
    ADD COLUMN IF NOT EXISTS current_submission_id UUID REFERENCES change_request_submissions(id) ON DELETE SET NULL;

-- 差异按提交轮次归属，便于区分"本次待验收差异"与历史轮次差异。
ALTER TABLE change_request_diffs
    ADD COLUMN IF NOT EXISTS submission_id UUID REFERENCES change_request_submissions(id) ON DELETE CASCADE;
CREATE INDEX idx_change_request_diffs_submission ON change_request_diffs(submission_id, created_at);

-- 回填历史数据：为已有提交记录（submitted_at 非空）的旧工单补建第 1 轮快照，
-- 并把该工单既有差异挂到该轮次，保证旧格式数据仍可被读取和发布。
INSERT INTO change_request_submissions (
    request_id, round, submitted_by, actual_changes, proposed_attributes,
    base_attachment_version_id, submitted_attachment_version_id, status, created_at)
SELECT cr.id, 1, cr.executor_id, cr.actual_changes, cr.proposed_attributes,
       cr.base_attachment_version_id, cr.submitted_attachment_version_id,
       CASE WHEN cr.status = 'completed' THEN 'accepted'
            WHEN cr.status = 'executing' THEN 'returned'
            ELSE 'pending' END,
       COALESCE(cr.submitted_at, cr.created_at)
FROM change_requests cr
WHERE cr.submitted_at IS NOT NULL AND cr.current_submission_id IS NULL;

UPDATE change_requests cr
SET current_submission_id = sub.id
FROM change_request_submissions sub
WHERE sub.request_id = cr.id AND sub.round = 1 AND cr.current_submission_id IS NULL
  AND cr.submitted_at IS NOT NULL;

UPDATE change_request_diffs d
SET submission_id = sub.id
FROM change_request_submissions sub
WHERE sub.request_id = d.request_id AND sub.round = 1 AND d.submission_id IS NULL;

GRANT SELECT, INSERT, UPDATE, DELETE ON change_request_submissions TO cadguanliq_app;
