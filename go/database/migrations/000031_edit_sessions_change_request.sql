\set ON_ERROR_STOP on

-- 编辑会话绑定变更工单：存档图纸经工单放行的编辑，其成果登记为工作版本、
-- 与正式版本隔离，验收后才发布。change_request_id 非空即标识该会话受工单授权。
ALTER TABLE edit_sessions
    ADD COLUMN change_request_id UUID REFERENCES change_requests(id) ON DELETE SET NULL;

CREATE INDEX idx_edit_sessions_change_request
    ON edit_sessions(change_request_id)
    WHERE change_request_id IS NOT NULL;
