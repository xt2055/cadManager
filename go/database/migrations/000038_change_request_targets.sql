\set ON_ERROR_STOP on

CREATE TABLE change_request_targets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    request_id UUID NOT NULL REFERENCES change_requests(id) ON DELETE CASCADE,
    attachment_id UUID NOT NULL REFERENCES attachments(id) ON DELETE CASCADE,
    base_attachment_version_id UUID REFERENCES attachment_versions(id) ON DELETE SET NULL,
    work_attachment_version_id UUID REFERENCES attachment_versions(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (request_id, attachment_id)
);

CREATE INDEX idx_change_request_targets_attachment
    ON change_request_targets(attachment_id, request_id);

CREATE TABLE change_request_submission_targets (
    submission_id UUID NOT NULL REFERENCES change_request_submissions(id) ON DELETE CASCADE,
    attachment_id UUID NOT NULL REFERENCES attachments(id) ON DELETE RESTRICT,
    base_attachment_version_id UUID REFERENCES attachment_versions(id) ON DELETE SET NULL,
    submitted_attachment_version_id UUID REFERENCES attachment_versions(id) ON DELETE SET NULL,
    PRIMARY KEY (submission_id, attachment_id)
);

CREATE INDEX idx_change_request_submission_targets_attachment
    ON change_request_submission_targets(attachment_id, submission_id);

-- 兼容旧工单：把原单文件基线及工作版本迁移为附件级目标。
INSERT INTO change_request_targets (
    request_id,
    attachment_id,
    base_attachment_version_id,
    work_attachment_version_id
)
SELECT cr.id,
       COALESCE(base_version.attachment_id, work_version.attachment_id),
       cr.base_attachment_version_id,
       cr.submitted_attachment_version_id
FROM change_requests cr
LEFT JOIN attachment_versions base_version ON base_version.id = cr.base_attachment_version_id
LEFT JOIN attachment_versions work_version ON work_version.id = cr.submitted_attachment_version_id
WHERE COALESCE(base_version.attachment_id, work_version.attachment_id) IS NOT NULL
ON CONFLICT (request_id, attachment_id) DO UPDATE
SET base_attachment_version_id = EXCLUDED.base_attachment_version_id,
    work_attachment_version_id = EXCLUDED.work_attachment_version_id,
    updated_at = now();

-- 兼容旧提交快照；000033 已将旧轮次标记为 legacy_history，仍保留其文件指向用于历史查看。
INSERT INTO change_request_submission_targets (
    submission_id,
    attachment_id,
    base_attachment_version_id,
    submitted_attachment_version_id
)
SELECT sub.id,
       COALESCE(base_version.attachment_id, submitted_version.attachment_id),
       sub.base_attachment_version_id,
       sub.submitted_attachment_version_id
FROM change_request_submissions sub
LEFT JOIN attachment_versions base_version ON base_version.id = sub.base_attachment_version_id
LEFT JOIN attachment_versions submitted_version ON submitted_version.id = sub.submitted_attachment_version_id
WHERE COALESCE(base_version.attachment_id, submitted_version.attachment_id) IS NOT NULL
ON CONFLICT DO NOTHING;

GRANT SELECT, INSERT, UPDATE, DELETE ON change_request_targets TO cadguanliq_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON change_request_submission_targets TO cadguanliq_app;
