-- 开发环境 seed。执行前请确认目标数据库是开发库；本文件不包含真实业务数据。
BEGIN;

INSERT INTO users (account, display_name, password_hash, status)
VALUES ('admin', '系统管理员', crypt('admin', gen_salt('bf')), 'active')
ON CONFLICT (account) DO UPDATE
SET display_name = EXCLUDED.display_name,
    status = 'active';

INSERT INTO user_roles (user_id, role)
SELECT id, 'admin'
FROM users
WHERE account = 'admin'
ON CONFLICT (user_id, role) DO NOTHING;

INSERT INTO review_flows (name, description, enabled)
VALUES ('开发默认流程', 'Phase 1 开发测试审核流程', true)
ON CONFLICT (name) DO UPDATE
SET description = EXCLUDED.description, enabled = EXCLUDED.enabled
RETURNING id;

INSERT INTO review_flow_nodes (flow_id, name, signer_role, candidate_role, assigned_name, required, node_order)
SELECT id, '设计自检', '设计', 'designer', '开发测试用户', true, 1
FROM review_flows WHERE name = '开发默认流程'
ON CONFLICT (flow_id, name) DO UPDATE
SET signer_role = EXCLUDED.signer_role,
    candidate_role = EXCLUDED.candidate_role,
    assigned_name = EXCLUDED.assigned_name,
    required = EXCLUDED.required,
    node_order = EXCLUDED.node_order;

INSERT INTO review_flow_nodes (flow_id, name, signer_role, candidate_role, assigned_name, required, node_order)
SELECT id, '主管批准', '批准', 'reviewer', '开发测试用户', true, 2
FROM review_flows WHERE name = '开发默认流程'
ON CONFLICT (flow_id, name) DO UPDATE
SET signer_role = EXCLUDED.signer_role,
    candidate_role = EXCLUDED.candidate_role,
    assigned_name = EXCLUDED.assigned_name,
    required = EXCLUDED.required,
    node_order = EXCLUDED.node_order;

INSERT INTO drawings (drawing_no, name, project, kind, status)
VALUES ('DEV-ASM-001', '开发测试总图', '架构重构测试项目', '总图', 'published')
ON CONFLICT (drawing_no) DO UPDATE SET name = EXCLUDED.name, project = EXCLUDED.project;

INSERT INTO drawings (drawing_no, name, project, kind, status)
VALUES ('DEV-ASM-002', '开发测试借用图', '架构重构测试项目', '总图', 'draft')
ON CONFLICT (drawing_no) DO UPDATE SET name = EXCLUDED.name, project = EXCLUDED.project;

INSERT INTO parts (part_no, normalized_part_no, lifecycle_status)
VALUES ('DEV-P-001', 'DEV-P-001', 'active')
ON CONFLICT (normalized_part_no) DO UPDATE SET part_no = EXCLUDED.part_no;

INSERT INTO part_revisions (part_id, revision_no, version, name, part_type, workflow_status, published_at)
SELECT id, 1, 'v1.0', '开发测试源零件', '自制件', 'published', now()
FROM parts WHERE normalized_part_no = 'DEV-P-001'
ON CONFLICT (part_id, revision_no) DO NOTHING;

UPDATE parts p
SET published_revision_id = r.id
FROM part_revisions r
WHERE p.normalized_part_no = 'DEV-P-001'
  AND r.part_id = p.id
  AND r.revision_no = 1
  AND r.workflow_status = 'published';

INSERT INTO drawing_part_relations (drawing_id, part_id, relation_type, qty, created_by)
SELECT d.id, p.id, 'owned', 1, NULL
FROM drawings d, parts p
WHERE d.drawing_no = 'DEV-ASM-001'
  AND p.normalized_part_no = 'DEV-P-001'
  AND NOT EXISTS (
      SELECT 1 FROM drawing_part_relations r
      WHERE r.drawing_id = d.id AND r.part_id = p.id
        AND r.relation_type = 'owned' AND r.status = 'active'
  );

WITH target_drawing AS (
    SELECT id FROM drawings WHERE drawing_no = 'DEV-ASM-002'
), source_part AS (
    SELECT id FROM parts WHERE normalized_part_no = 'DEV-P-001'
)
INSERT INTO drawing_part_relations (drawing_id, part_id, relation_type, qty, borrow_reason)
SELECT target_drawing.id, source_part.id, 'borrowed', 2, 'Phase 1 动态借用测试'
FROM target_drawing, source_part
WHERE NOT EXISTS (
    SELECT 1 FROM drawing_part_relations r
    WHERE r.drawing_id = target_drawing.id
      AND r.part_id = source_part.id
      AND r.relation_type = 'borrowed'
      AND r.status = 'active'
);

INSERT INTO drawing_boms (drawing_id, revision)
SELECT id, 1 FROM drawings WHERE drawing_no IN ('DEV-ASM-001', 'DEV-ASM-002')
ON CONFLICT (drawing_id) DO NOTHING;

COMMIT;
