CREATE TABLE IF NOT EXISTS notifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    recipient_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    sender_id UUID REFERENCES users(id) ON DELETE SET NULL,
    kind TEXT NOT NULL CHECK (kind IN ('change', 'review', 'announcement')),
    title TEXT NOT NULL,
    content TEXT NOT NULL DEFAULT '',
    drawing_id UUID REFERENCES drawings(id) ON DELETE SET NULL,
    event_key TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    read_at TIMESTAMPTZ,
    UNIQUE (recipient_id, event_key)
);
CREATE INDEX IF NOT EXISTS notifications_inbox ON notifications(recipient_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS notifications_unread ON notifications(recipient_id) WHERE read_at IS NULL;
GRANT SELECT, INSERT, UPDATE, DELETE ON notifications TO cadguanliq_app;

-- PostgreSQL only delivers NOTIFY after commit. Payloads contain the recipient
-- identity only; clients retrieve inbox data through the authenticated API.
CREATE OR REPLACE FUNCTION push_notification_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'INSERT' OR OLD.read_at IS DISTINCT FROM NEW.read_at THEN
        PERFORM pg_notify('cad_notifications', NEW.recipient_id::text);
    END IF;
    RETURN NEW;
END;
$$;
DROP TRIGGER IF EXISTS notification_realtime ON notifications;
CREATE TRIGGER notification_realtime AFTER INSERT OR UPDATE OF read_at ON notifications
    FOR EACH ROW EXECUTE FUNCTION push_notification_change();

-- 动作日志与通知共享事务；回滚不发送，按动作 ID 去重，重新提交的新轮次仍会通知。
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
        OR (NEW.action IN ('approve', 'reject', 'verify', 'return', 'cancel')
            AND u.id IN (ticket.applicant_id, ticket.executor_id))
    ) ON CONFLICT (recipient_id, event_key) DO NOTHING;
    RETURN NEW;
END;
$$;
DROP TRIGGER IF EXISTS change_action_notification ON change_request_actions;
CREATE TRIGGER change_action_notification AFTER INSERT ON change_request_actions
    FOR EACH ROW EXECUTE FUNCTION notify_change_action();

CREATE OR REPLACE FUNCTION notify_review_result() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    label TEXT;
    reason TEXT;
    actor UUID;
BEGIN
    -- 变更审核由工单动作通知申请人和执行人，避免同一结果重复提醒。
    IF NEW.change_submission_id IS NOT NULL OR NEW.initiator_id IS NULL
        OR NEW.status NOT IN ('published', 'rejected') OR NEW.status = OLD.status THEN
        RETURN NEW;
    END IF;
    SELECT drawing_no || ' · ' || name INTO label FROM drawings WHERE id = NEW.drawing_id;
    SELECT opinion, actor_id INTO reason, actor FROM review_actions
        WHERE review_case_id = NEW.id
          AND action = CASE WHEN NEW.status = 'published' THEN 'pass' ELSE 'reject' END
        ORDER BY created_at DESC, id DESC LIMIT 1;
    INSERT INTO notifications(recipient_id, sender_id, kind, title, content, drawing_id, event_key)
    VALUES(NEW.initiator_id, actor, 'review',
        CASE WHEN NEW.status = 'published' THEN '图纸审批已通过' ELSE '图纸审批被驳回' END,
        COALESCE(label, '图纸审核') || E'\n' || COALESCE(reason, ''), NEW.drawing_id,
        'review:' || NEW.id::text || ':' || NEW.status)
    ON CONFLICT (recipient_id, event_key) DO NOTHING;
    RETURN NEW;
END;
$$;
DROP TRIGGER IF EXISTS review_result_notification ON review_cases;
CREATE TRIGGER review_result_notification AFTER UPDATE OF status ON review_cases
    FOR EACH ROW EXECUTE FUNCTION notify_review_result();
