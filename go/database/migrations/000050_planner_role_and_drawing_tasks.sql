\set ON_ERROR_STOP on

-- 计划员身份与图纸任务（负责人指派）。
-- 业务约束：
--   1) 只有计划员与管理员可以创建图纸；
--   2) 一张图纸同一时刻只有一名有效负责人（历史行保留）；
--   3) 图纸存在有效负责人时，原创建人级控制权（编辑、存档、送审、文件与模型管理、
--      新增零件图）归负责人与管理员；无有效负责人时回落给创建人，保证存量数据零回归。
-- 判定规则由 drawing_decision_owner 单一实现，Go 侧 Drawing.Decides 与其保持同构，
-- 一致性由 internal/dbtest 断言。

-- 角色枚举扩展：新增 planner（计划员）。旧约束名由 PostgreSQL 自动生成，
-- 因此按 pg_constraint 查找后重建，避免依赖具体名称。
DO $$
DECLARE
    constraint_name TEXT;
BEGIN
    FOR constraint_name IN
        SELECT con.conname
        FROM pg_constraint con
        JOIN pg_class rel ON rel.oid = con.conrelid
        JOIN pg_namespace ns ON ns.oid = rel.relnamespace
        WHERE rel.relname = 'user_roles'
          AND ns.nspname = current_schema()
          AND con.contype = 'c'
          AND pg_get_constraintdef(con.oid) LIKE '%role%'
    LOOP
        EXECUTE format('ALTER TABLE user_roles DROP CONSTRAINT %I', constraint_name);
    END LOOP;
END $$;

ALTER TABLE user_roles
    ADD CONSTRAINT user_roles_role_check
    CHECK (role IN ('admin', 'designer', 'reviewer', 'planner'));

-- 任务通知类型：图纸任务指派/改派通知使用独立 kind，避免与变更、审核通知混淆。
DO $$
DECLARE
    constraint_name TEXT;
BEGIN
    FOR constraint_name IN
        SELECT con.conname
        FROM pg_constraint con
        JOIN pg_class rel ON rel.oid = con.conrelid
        JOIN pg_namespace ns ON ns.oid = rel.relnamespace
        WHERE rel.relname = 'notifications'
          AND ns.nspname = current_schema()
          AND con.contype = 'c'
          AND pg_get_constraintdef(con.oid) LIKE '%kind%'
    LOOP
        EXECUTE format('ALTER TABLE notifications DROP CONSTRAINT %I', constraint_name);
    END LOOP;
END $$;

ALTER TABLE notifications
    ADD CONSTRAINT notifications_kind_check
    CHECK (kind IN ('change', 'review', 'announcement', 'task'));

-- 图纸任务：指派记录即任务本身。改派把旧行置为 replaced 并新增 active 行，
-- 因此同一图纸的有效任务唯一，而历史指派与改派原因可追溯。
CREATE TABLE drawing_tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    drawing_id UUID NOT NULL REFERENCES drawings(id) ON DELETE CASCADE,
    assignee_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    assigned_by UUID REFERENCES users(id) ON DELETE SET NULL,
    note TEXT NOT NULL DEFAULT '',
    due_date DATE,
    status VARCHAR(20) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'replaced', 'cancelled')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at TIMESTAMPTZ,
    ended_by UUID REFERENCES users(id) ON DELETE SET NULL,
    end_reason TEXT NOT NULL DEFAULT ''
);

-- 同一图纸最多一条有效任务：唯一性由部分索引保证，而不是靠应用层判断。
CREATE UNIQUE INDEX drawing_tasks_one_active
    ON drawing_tasks(drawing_id) WHERE status = 'active';
CREATE INDEX drawing_tasks_assignee
    ON drawing_tasks(assignee_id, status, created_at DESC);
CREATE INDEX drawing_tasks_drawing_history
    ON drawing_tasks(drawing_id, created_at DESC);

-- 控制权判定：管理员 > 有效负责人 > 无负责人时回落创建人。
-- 供事务内需要 FOR UPDATE 的路径（如 3D 模型写入授权）复用同一规则。
CREATE FUNCTION drawing_decision_owner(drawing_uuid UUID, user_uuid UUID)
RETURNS BOOLEAN LANGUAGE sql STABLE AS $$
    SELECT
        EXISTS (SELECT 1 FROM user_roles r WHERE r.user_id = user_uuid AND r.role = 'admin')
        OR CASE
            WHEN EXISTS (SELECT 1 FROM drawing_tasks t
                         WHERE t.drawing_id = drawing_uuid AND t.status = 'active')
            THEN EXISTS (SELECT 1 FROM drawing_tasks t
                         WHERE t.drawing_id = drawing_uuid AND t.status = 'active'
                           AND t.assignee_id = user_uuid)
            ELSE EXISTS (SELECT 1 FROM drawings d
                         WHERE d.id = drawing_uuid AND d.created_by = user_uuid)
        END
$$;

GRANT SELECT, INSERT, UPDATE, DELETE ON drawing_tasks TO cadguanliq_app;
GRANT EXECUTE ON FUNCTION drawing_decision_owner(UUID, UUID) TO cadguanliq_app;
