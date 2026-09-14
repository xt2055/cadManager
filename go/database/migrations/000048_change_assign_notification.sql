\set ON_ERROR_STOP on
BEGIN;

-- 后台审批指定设计员时，为被指定人单独推送“你被指定为本次变更负责人”，
-- 避免与申请人共享同一条“变更申请已通过”通知。
CREATE OR REPLACE FUNCTION notify_change_action() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    ticket change_requests%ROWTYPE;
    heading TEXT;
BEGIN
    SELECT * INTO ticket FROM change_requests WHERE id = NEW.request_id;
    heading := CASE NEW.action
        WHEN 'create' THEN '收到新的变更工单'
        WHEN 'approve' THEN '变更申请已通过'
        WHEN 'reject' THEN '变更申请被驳回'
        WHEN 'submit' THEN '变更成果已提交审核'
        WHEN 'verify' THEN '变更审核已通过'
        WHEN 'return' THEN '变更审核被退回'
        WHEN 'cancel' THEN '变更工单已终止'
        ELSE NULL END;
    IF heading IS NULL THEN RETURN NEW; END IF;
    INSERT INTO notifications(recipient_id, sender_id, kind, title, content, drawing_id, event_key)
    SELECT u.id, NEW.actor_id, 'change', heading,
        ticket.request_no || ' · ' || ticket.drawing_no || ' · ' || ticket.title || E'\n' ||
        CASE WHEN NEW.action = 'create' THEN ticket.reason ELSE NEW.opinion END,
        ticket.drawing_id, 'change:' || NEW.id::text
    FROM users u WHERE u.status = 'active' AND (
        (NEW.action IN ('create', 'submit') AND EXISTS (
            SELECT 1 FROM user_roles r WHERE r.user_id = u.id AND r.role = 'admin'))
        OR (NEW.action = 'approve' AND u.id = ticket.applicant_id)
        OR (NEW.action IN ('reject', 'verify', 'return', 'cancel')
            AND u.id IN (ticket.applicant_id, ticket.executor_id))
    ) ON CONFLICT (recipient_id, event_key) DO NOTHING;

    -- 指定了申请人以外的人负责修改时，向被指定人发送专属通知。
    IF NEW.action = 'approve' AND ticket.executor_id IS NOT NULL
        AND ticket.executor_id <> ticket.applicant_id THEN
        INSERT INTO notifications(recipient_id, sender_id, kind, title, content, drawing_id, event_key)
        SELECT u.id, NEW.actor_id, 'change', '你被指定为本次变更负责人',
            ticket.request_no || ' · ' || ticket.drawing_no || ' · ' || ticket.title || E'\n' ||
            '请在变更工单中完成图纸修改并提交审核。' ||
            CASE WHEN COALESCE(NEW.opinion, '') = '' THEN '' ELSE E'\n审批意见：' || NEW.opinion END,
            ticket.drawing_id, 'change-assign:' || NEW.id::text
        FROM users u WHERE u.id = ticket.executor_id AND u.status = 'active'
        ON CONFLICT (recipient_id, event_key) DO NOTHING;
    END IF;
    RETURN NEW;
END;
$$;

COMMIT;
