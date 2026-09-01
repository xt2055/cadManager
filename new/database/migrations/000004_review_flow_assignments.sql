\set ON_ERROR_STOP on

ALTER TABLE review_flow_nodes
    ADD COLUMN IF NOT EXISTS assigned_user_id UUID REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE review_flow_nodes
    ADD COLUMN IF NOT EXISTS assigned_name VARCHAR(100) NOT NULL DEFAULT '待定';

DO $$
DECLARE
    target_flow_id UUID;
    admin_id UUID;
BEGIN
    SELECT id INTO target_flow_id
    FROM review_flows
    WHERE name = '企业标准图纸审核流程'
    LIMIT 1;

    SELECT id INTO admin_id
    FROM users
    WHERE lower(account) = 'admin'
    LIMIT 1;

    IF target_flow_id IS NULL THEN
        INSERT INTO review_flows (name, description, enabled, created_by)
        VALUES ('企业标准图纸审核流程', '默认审核流程，可由管理员按项目需要调整节点身份和指定人员。', true, admin_id)
        RETURNING id INTO target_flow_id;
    END IF;

    INSERT INTO review_flow_nodes (flow_id, name, signer_role, candidate_role, assigned_name, required, node_order)
    SELECT target_flow_id, source.name, source.signer_role, 'reviewer', '待定', source.required, source.node_order
    FROM (VALUES
        ('设计自检', '设计', true, 1),
        ('校对复核', '校对', true, 2),
        ('专业审核', '审核', true, 3),
        ('工艺会签', '工艺', true, 4),
        ('标准化审查', '标准化', false, 5),
        ('主管批准', '批准', true, 6)
    ) AS source(name, signer_role, required, node_order)
    WHERE NOT EXISTS (
        SELECT 1 FROM review_flow_nodes existing
        WHERE existing.flow_id = target_flow_id AND existing.name = source.name
    );
END
$$;

GRANT SELECT, INSERT, UPDATE, DELETE ON review_flow_nodes TO cadguanliq_app;
