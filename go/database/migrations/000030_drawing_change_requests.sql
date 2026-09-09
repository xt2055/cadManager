\set ON_ERROR_STOP on

-- 图纸变更管理：已存档图纸的修改必须通过变更工单，经管理员审批后由指定执行人修改，
-- 默认还需结果验收才能发布正式版；管理员可免验收但必须留原因。所有审批/意见/差异单独留痕。

CREATE TABLE change_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    request_no VARCHAR(50) NOT NULL UNIQUE,
    -- 工单历史不可随图纸物理删除而消失：存在工单时禁止物理删除图纸。
    drawing_id UUID NOT NULL REFERENCES drawings(id) ON DELETE RESTRICT,
    drawing_no VARCHAR(150) NOT NULL,
    title VARCHAR(255) NOT NULL DEFAULT '',
    -- 变更原因（申请）与计划修改范围，二者必填。
    reason TEXT NOT NULL,
    scope TEXT NOT NULL,
    -- 基准：创建工单时图纸行 revision，用于执行期乐观并发保护。
    base_drawing_revision BIGINT NOT NULL CHECK (base_drawing_revision > 0),
    -- 主文件基线版本（创建工单时被保护附件的当前版本），可为空（无 CAD 附件时）。
    base_attachment_version_id UUID REFERENCES attachment_versions(id) ON DELETE SET NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending_approval'
        CHECK (status IN ('pending_approval', 'executing', 'pending_verify', 'completed', 'rejected', 'cancelled')),
    -- 结果验收开关：默认需要验收；管理员可在审批时关闭，但需填免验收原因。
    require_verify BOOLEAN NOT NULL DEFAULT true,
    -- 申请人与执行人是留痕主体，不允许因用户删除被置空（NOT NULL 与 SET NULL 矛盾）。
    applicant_id UUID NOT NULL REFERENCES users(id),
    executor_id UUID NOT NULL REFERENCES users(id),
    approver_id UUID REFERENCES users(id) ON DELETE SET NULL,
    -- 审批意见与免验收原因分开存储，互不覆盖。
    approver_opinion TEXT NOT NULL DEFAULT '',
    -- 管理员直接批准（申请人与批准人为同一管理员）时置真，强制留痕。
    direct_admin_approval BOOLEAN NOT NULL DEFAULT false,
    approved_at TIMESTAMPTZ,
    verify_waived_reason TEXT NOT NULL DEFAULT '',
    -- 执行期暂存、验收时才生效的数据，避免修改未验收就污染正式成果。
    proposed_attributes JSONB NOT NULL DEFAULT '{}'::jsonb,
    submitted_attachment_version_id UUID REFERENCES attachment_versions(id) ON DELETE SET NULL,
    actual_changes TEXT NOT NULL DEFAULT '',
    submitted_at TIMESTAMPTZ,
    verified_at TIMESTAMPTZ,
    verifier_id UUID REFERENCES users(id) ON DELETE SET NULL,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 同一张图纸最多只有一个未结束的变更工单。
CREATE UNIQUE INDEX uq_change_requests_open_drawing
    ON change_requests(drawing_id)
    WHERE status IN ('pending_approval', 'executing', 'pending_verify');
CREATE INDEX idx_change_requests_drawing ON change_requests(drawing_id, created_at DESC);
CREATE INDEX idx_change_requests_status ON change_requests(status);
CREATE INDEX idx_change_requests_executor ON change_requests(executor_id, status);

CREATE TABLE change_request_actions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    request_id UUID NOT NULL REFERENCES change_requests(id) ON DELETE CASCADE,
    actor_id UUID REFERENCES users(id) ON DELETE SET NULL,
    action VARCHAR(30) NOT NULL
        CHECK (action IN ('create', 'approve', 'reject', 'submit', 'verify', 'return', 'cancel', 'waive_verify')),
    -- 审批意见、驳回意见、免验收原因、提交说明、验收意见统一在此留痕。
    opinion TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_change_request_actions_request ON change_request_actions(request_id, created_at);

-- 结构化的"修改了什么"差异：属性字段前后值、文件版本变化等。提交完成与验收时写入。
CREATE TABLE change_request_diffs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    request_id UUID NOT NULL REFERENCES change_requests(id) ON DELETE CASCADE,
    kind VARCHAR(20) NOT NULL
        CHECK (kind IN ('attribute', 'material', 'file', 'attachment')),
    field VARCHAR(255) NOT NULL DEFAULT '',
    old_value TEXT NOT NULL DEFAULT '',
    new_value TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_change_request_diffs_request ON change_request_diffs(request_id, created_at);

CREATE TRIGGER change_requests_set_updated_at
    BEFORE UPDATE ON change_requests
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

GRANT SELECT, INSERT, UPDATE, DELETE ON change_requests, change_request_actions, change_request_diffs TO cadguanliq_app;
