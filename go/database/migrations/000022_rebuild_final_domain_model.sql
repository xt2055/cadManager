\set ON_ERROR_STOP on

-- 开发环境最终模型重建：不迁移旧 structure_parts、borrow_records、file_versions 数据。
-- 认证、审核流程、审计和更新管理基础表由前序迁移保留；业务数据按最终模型重新建立。

-- 这里连基础表也一并重建，是为了让开发账号成为全部对象的 owner，
-- 避免旧环境由 postgres 创建的表阻止最终外键和约束落地。
DROP TABLE IF EXISTS review_flow_nodes, review_flows, audit_logs, update_manifests CASCADE;
DROP TABLE IF EXISTS sessions, user_roles, users CASCADE;

DROP TABLE IF EXISTS upload_session_chunks, upload_session_items, upload_sessions, storage_cleanup_jobs CASCADE;
DROP TABLE IF EXISTS edit_session_tickets, edit_sessions CASCADE;
DROP TABLE IF EXISTS file_versions, attachment_versions, part_revision_attachments, attachments, file_blobs CASCADE;
DROP TABLE IF EXISTS bom_items, drawing_boms, drawing_versions CASCADE;
DROP TABLE IF EXISTS borrow_records, drawing_part_relations CASCADE;
DROP TABLE IF EXISTS drawing_signers, review_actions, review_case_nodes, review_cases CASCADE;
DROP TABLE IF EXISTS drawing_attribute_values, drawing_attribute_fields, drawing_attributes, drawing_branches CASCADE;
DROP TABLE IF EXISTS structure_parts, part_revisions, parts, drawings CASCADE;

CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account VARCHAR(100) NOT NULL UNIQUE,
    display_name VARCHAR(100) NOT NULL,
    password_hash TEXT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'disabled')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_login_at TIMESTAMPTZ
);

CREATE TABLE user_roles (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role VARCHAR(30) NOT NULL CHECK (role IN ('admin', 'designer', 'reviewer')),
    PRIMARY KEY (user_id, role)
);

CREATE TABLE sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash CHAR(64) NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ
);

CREATE INDEX idx_sessions_user_id ON sessions(user_id);
CREATE INDEX idx_sessions_expires_at ON sessions(expires_at);

CREATE TABLE review_flows (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE review_flow_nodes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    flow_id UUID NOT NULL REFERENCES review_flows(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    signer_role VARCHAR(20),
    candidate_role VARCHAR(30) NOT NULL DEFAULT 'reviewer',
    assigned_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    assigned_name VARCHAR(100) NOT NULL DEFAULT '待定',
    required BOOLEAN NOT NULL DEFAULT true,
    node_order INTEGER NOT NULL CHECK (node_order > 0),
    UNIQUE (flow_id, node_order),
    UNIQUE (flow_id, name)
);

CREATE TABLE audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_id UUID REFERENCES users(id) ON DELETE SET NULL,
    action VARCHAR(100) NOT NULL,
    resource_type VARCHAR(50) NOT NULL,
    resource_id UUID,
    summary TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    ip_address INET,
    user_agent TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_audit_logs_actor_id ON audit_logs(actor_id);
CREATE INDEX idx_audit_logs_resource ON audit_logs(resource_type, resource_id);
CREATE INDEX idx_audit_logs_created_at ON audit_logs(created_at DESC);
CREATE INDEX idx_audit_logs_drawing_no
    ON audit_logs ((metadata->>'drawingNo'))
    WHERE metadata ? 'drawingNo';

CREATE TABLE update_manifests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    version VARCHAR(50) NOT NULL,
    platform VARCHAR(100) NOT NULL,
    notes TEXT NOT NULL DEFAULT '',
    published_at TIMESTAMPTZ,
    download_url TEXT,
    mandatory BOOLEAN NOT NULL DEFAULT false,
    enabled BOOLEAN NOT NULL DEFAULT true,
    file_name VARCHAR(255) NOT NULL DEFAULT '',
    size_bytes BIGINT NOT NULL DEFAULT 0 CHECK (size_bytes >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (version, platform)
);

CREATE TABLE drawings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    drawing_no VARCHAR(150) NOT NULL UNIQUE,
    name VARCHAR(255) NOT NULL,
    project VARCHAR(255) NOT NULL,
    kind VARCHAR(20) NOT NULL DEFAULT '总图'
        CHECK (kind IN ('总图', '零件图')),
    vendor VARCHAR(255) NOT NULL DEFAULT '',
    material VARCHAR(255) NOT NULL DEFAULT '—',
    status VARCHAR(20) NOT NULL DEFAULT 'draft'
        CHECK (status IN ('published', 'reviewing', 'draft', 'disabled', 'archived')),
    version VARCHAR(50) NOT NULL DEFAULT 'v1.0',
    borrow_from VARCHAR(150),
    remark TEXT,
    revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    updated_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_drawings_project ON drawings(project);
CREATE INDEX idx_drawings_status ON drawings(status);
CREATE INDEX idx_drawings_updated_at ON drawings(updated_at DESC);

CREATE TABLE parts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    part_no VARCHAR(150) NOT NULL,
    normalized_part_no VARCHAR(150) NOT NULL UNIQUE,
    published_revision_id UUID,
    lifecycle_status VARCHAR(20) NOT NULL DEFAULT 'active'
        CHECK (lifecycle_status IN ('active', 'obsolete', 'archived')),
    forked_from_part_id UUID REFERENCES parts(id) ON DELETE SET NULL,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by UUID REFERENCES users(id) ON DELETE SET NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_parts_lifecycle_status ON parts(lifecycle_status);
CREATE INDEX idx_parts_forked_from ON parts(forked_from_part_id);

CREATE TABLE part_revisions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    part_id UUID NOT NULL REFERENCES parts(id) ON DELETE CASCADE,
    revision_no INTEGER NOT NULL CHECK (revision_no > 0),
    version VARCHAR(50) NOT NULL DEFAULT 'v1.0',
    row_revision BIGINT NOT NULL DEFAULT 1 CHECK (row_revision > 0),
    name VARCHAR(255) NOT NULL,
    material VARCHAR(255) NOT NULL DEFAULT '—',
    spec VARCHAR(255) NOT NULL DEFAULT '',
    weight NUMERIC(14, 4) NOT NULL DEFAULT 0 CHECK (weight >= 0),
    surface_treatment VARCHAR(255) NOT NULL DEFAULT '',
    part_type VARCHAR(20) NOT NULL DEFAULT '自制件'
        CHECK (part_type IN ('自制件', '外协件', '标准件', '外购件')),
    workflow_status VARCHAR(20) NOT NULL DEFAULT 'draft'
        CHECK (workflow_status IN ('draft', 'reviewing', 'published', 'rejected')),
    based_on_revision_id UUID,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_by UUID REFERENCES users(id) ON DELETE SET NULL,
    published_at TIMESTAMPTZ,
    UNIQUE (part_id, revision_no),
    UNIQUE (part_id, id),
    FOREIGN KEY (part_id, based_on_revision_id)
        REFERENCES part_revisions(part_id, id)
);

ALTER TABLE parts
    ADD CONSTRAINT parts_published_revision_same_part_fkey
    FOREIGN KEY (id, published_revision_id)
    REFERENCES part_revisions(part_id, id);

CREATE INDEX idx_part_revisions_part_created ON part_revisions(part_id, created_at DESC);
CREATE INDEX idx_part_revisions_status ON part_revisions(workflow_status);

CREATE TABLE drawing_part_relations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    drawing_id UUID NOT NULL REFERENCES drawings(id) ON DELETE CASCADE,
    part_id UUID NOT NULL REFERENCES parts(id) ON DELETE RESTRICT,
    parent_relation_id UUID,
    relation_type VARCHAR(20) NOT NULL
        CHECK (relation_type IN ('owned', 'borrowed')),
    qty NUMERIC(14, 4) NOT NULL DEFAULT 1 CHECK (qty > 0),
    position VARCHAR(100),
    line_no INTEGER CHECK (line_no IS NULL OR line_no > 0),
    remark TEXT NOT NULL DEFAULT '',
    borrow_reason TEXT,
    borrowed_by UUID REFERENCES users(id) ON DELETE SET NULL,
    borrowed_at TIMESTAMPTZ,
    status VARCHAR(20) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'archived')),
    revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
    forked_from_relation_id UUID,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by UUID REFERENCES users(id) ON DELETE SET NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (drawing_id, id),
    CHECK (parent_relation_id IS NULL OR parent_relation_id <> id),
    FOREIGN KEY (drawing_id, parent_relation_id)
        REFERENCES drawing_part_relations(drawing_id, id),
    FOREIGN KEY (forked_from_relation_id)
        REFERENCES drawing_part_relations(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX uq_drawing_part_relations_active_owned
    ON drawing_part_relations(part_id)
    WHERE relation_type = 'owned' AND status = 'active';
CREATE INDEX idx_drawing_part_relations_drawing_parent
    ON drawing_part_relations(drawing_id, parent_relation_id);
CREATE INDEX idx_drawing_part_relations_part
    ON drawing_part_relations(part_id, status);

CREATE TABLE file_blobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    sha256 CHAR(64) NOT NULL,
    size_bytes BIGINT NOT NULL CHECK (size_bytes >= 0),
    mime_type VARCHAR(255) NOT NULL DEFAULT 'application/octet-stream',
    storage_key VARCHAR(500) NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (sha256, size_bytes)
);

CREATE TABLE attachments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    drawing_id UUID REFERENCES drawings(id) ON DELETE CASCADE,
    part_id UUID REFERENCES parts(id) ON DELETE CASCADE,
    file_role VARCHAR(20) NOT NULL
        CHECK (file_role IN ('assembly', 'part', 'material', 'craft', 'other')),
    logical_name VARCHAR(255) NOT NULL,
    current_version_id UUID,
    revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
    uploaded_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ,
    CHECK ((drawing_id IS NOT NULL) <> (part_id IS NOT NULL))
);

CREATE INDEX idx_attachments_drawing_id ON attachments(drawing_id);
CREATE INDEX idx_attachments_part_id ON attachments(part_id);
CREATE INDEX idx_attachments_active ON attachments(deleted_at) WHERE deleted_at IS NULL;

CREATE TABLE attachment_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    attachment_id UUID NOT NULL REFERENCES attachments(id) ON DELETE CASCADE,
    version VARCHAR(50) NOT NULL,
    blob_id UUID NOT NULL REFERENCES file_blobs(id) ON DELETE RESTRICT,
    original_name VARCHAR(255) NOT NULL,
    mime_type VARCHAR(255) NOT NULL DEFAULT 'application/octet-stream',
    size_bytes BIGINT NOT NULL DEFAULT 0 CHECK (size_bytes >= 0),
    previewable BOOLEAN NOT NULL DEFAULT false,
    version_kind VARCHAR(20) NOT NULL DEFAULT 'working'
        CHECK (version_kind IN ('working', 'release')),
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ,
    UNIQUE (attachment_id, id)
);

CREATE UNIQUE INDEX uq_attachment_versions_active_version
    ON attachment_versions(attachment_id, version)
    WHERE deleted_at IS NULL;
CREATE INDEX idx_attachment_versions_attachment_created
    ON attachment_versions(attachment_id, created_at DESC);

ALTER TABLE attachments
    ADD CONSTRAINT attachments_current_version_fkey
    FOREIGN KEY (id, current_version_id)
    REFERENCES attachment_versions(attachment_id, id);

CREATE TABLE part_revision_attachments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    part_revision_id UUID NOT NULL REFERENCES part_revisions(id) ON DELETE CASCADE,
    attachment_version_id UUID NOT NULL REFERENCES attachment_versions(id) ON DELETE RESTRICT,
    role VARCHAR(30) NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0 CHECK (sort_order >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (part_revision_id, role, sort_order)
);

CREATE INDEX idx_part_revision_attachments_revision
    ON part_revision_attachments(part_revision_id, sort_order);

CREATE TABLE drawing_signers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    drawing_id UUID REFERENCES drawings(id) ON DELETE CASCADE,
    part_revision_id UUID REFERENCES part_revisions(id) ON DELETE CASCADE,
    role VARCHAR(20) NOT NULL
        CHECK (role IN ('设计', '校对', '审核', '工艺', '标准化', '批准')),
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    signer_name VARCHAR(100) NOT NULL DEFAULT '待定',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((drawing_id IS NOT NULL) <> (part_revision_id IS NOT NULL)),
    CHECK (length(trim(signer_name)) > 0)
);

CREATE UNIQUE INDEX uq_drawing_signers_drawing_role
    ON drawing_signers(drawing_id, role) WHERE drawing_id IS NOT NULL;
CREATE UNIQUE INDEX uq_drawing_signers_revision_role
    ON drawing_signers(part_revision_id, role) WHERE part_revision_id IS NOT NULL;

CREATE TABLE drawing_boms (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    drawing_id UUID NOT NULL REFERENCES drawings(id) ON DELETE CASCADE,
    revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
    updated_by UUID REFERENCES users(id) ON DELETE SET NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (drawing_id)
);

CREATE TABLE bom_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    bom_id UUID NOT NULL REFERENCES drawing_boms(id) ON DELETE CASCADE,
    item_no INTEGER NOT NULL CHECK (item_no > 0),
    part_id UUID REFERENCES parts(id) ON DELETE SET NULL,
    source_attachment_version_id UUID REFERENCES attachment_versions(id) ON DELETE SET NULL,
    name VARCHAR(255) NOT NULL,
    spec VARCHAR(255) NOT NULL DEFAULT '—',
    quantity NUMERIC(14, 4) NOT NULL DEFAULT 0 CHECK (quantity >= 0),
    weight NUMERIC(14, 4) NOT NULL DEFAULT 0 CHECK (weight >= 0),
    remark TEXT NOT NULL DEFAULT '',
    UNIQUE (bom_id, item_no)
);

CREATE INDEX idx_bom_items_bom_id ON bom_items(bom_id, item_no);

CREATE TABLE drawing_branches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_drawing_id UUID REFERENCES drawings(id) ON DELETE SET NULL,
    target_drawing_id UUID NOT NULL REFERENCES drawings(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    status VARCHAR(20) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'archived')),
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_drawing_branches_source ON drawing_branches(source_drawing_id);
CREATE INDEX idx_drawing_branches_target ON drawing_branches(target_drawing_id);

CREATE TABLE review_cases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    drawing_id UUID REFERENCES drawings(id) ON DELETE CASCADE,
    part_revision_id UUID REFERENCES part_revisions(id) ON DELETE CASCADE,
    flow_id UUID REFERENCES review_flows(id) ON DELETE RESTRICT,
    status VARCHAR(20) NOT NULL DEFAULT 'reviewing'
        CHECK (status IN ('pending', 'reviewing', 'published', 'rejected', 'cancelled')),
    initiator_id UUID REFERENCES users(id) ON DELETE SET NULL,
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    CHECK ((drawing_id IS NOT NULL) <> (part_revision_id IS NOT NULL))
);

CREATE INDEX idx_review_cases_drawing ON review_cases(drawing_id, status);
CREATE INDEX idx_review_cases_revision ON review_cases(part_revision_id, status);
CREATE UNIQUE INDEX uq_review_cases_active_drawing
    ON review_cases(drawing_id)
    WHERE drawing_id IS NOT NULL AND status IN ('pending', 'reviewing');
CREATE UNIQUE INDEX uq_review_cases_active_revision
    ON review_cases(part_revision_id)
    WHERE part_revision_id IS NOT NULL AND status IN ('pending', 'reviewing');

CREATE TABLE review_case_nodes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    review_case_id UUID NOT NULL REFERENCES review_cases(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    assigned_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    assigned_name VARCHAR(100) NOT NULL DEFAULT '待定',
    status VARCHAR(20) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pass', 'pending', 'rejected')),
    opinion TEXT NOT NULL DEFAULT '',
    required BOOLEAN NOT NULL DEFAULT true,
    node_order INTEGER NOT NULL CHECK (node_order > 0),
    reviewed_at TIMESTAMPTZ,
    UNIQUE (review_case_id, node_order),
    UNIQUE (review_case_id, name)
);

CREATE TABLE review_actions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    review_case_id UUID NOT NULL REFERENCES review_cases(id) ON DELETE CASCADE,
    node_id UUID REFERENCES review_case_nodes(id) ON DELETE SET NULL,
    actor_id UUID REFERENCES users(id) ON DELETE SET NULL,
    action VARCHAR(30) NOT NULL
        CHECK (action IN ('start', 'pass', 'reject', 'cancel', 'reopen')),
    opinion TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE drawing_attributes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL UNIQUE,
    required BOOLEAN NOT NULL DEFAULT false,
    enabled BOOLEAN NOT NULL DEFAULT true,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE drawing_attribute_fields (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    attribute_id UUID NOT NULL REFERENCES drawing_attributes(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (attribute_id, name),
    UNIQUE (attribute_id, id)
);

CREATE TABLE drawing_attribute_values (
    drawing_id UUID NOT NULL REFERENCES drawings(id) ON DELETE CASCADE,
    attribute_id UUID NOT NULL REFERENCES drawing_attributes(id) ON DELETE CASCADE,
    field_id UUID NOT NULL REFERENCES drawing_attribute_fields(id) ON DELETE RESTRICT,
    PRIMARY KEY (drawing_id, attribute_id)
);

CREATE TABLE edit_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    attachment_id UUID NOT NULL REFERENCES attachments(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    work_storage_key VARCHAR(500) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'closed', 'expired', 'conflict')),
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    closed_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX uq_edit_sessions_one_active_attachment
    ON edit_sessions(attachment_id) WHERE status = 'active';

CREATE TABLE edit_session_tickets (
    token_hash CHAR(64) PRIMARY KEY,
    session_id UUID NOT NULL REFERENCES edit_sessions(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ
);

CREATE TABLE upload_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind VARCHAR(30) NOT NULL CHECK (kind IN ('attachment', 'drawing-create')),
    idempotency_key VARCHAR(255) NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    status VARCHAR(20) NOT NULL DEFAULT 'open'
        CHECK (status IN ('open', 'committing', 'committed', 'cancelled', 'expired', 'failed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_activity_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    absolute_expires_at TIMESTAMPTZ NOT NULL,
    committed_at TIMESTAMPTZ,
    result JSONB,
    error_message TEXT,
    UNIQUE (user_id, idempotency_key)
);

CREATE TABLE upload_session_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID NOT NULL REFERENCES upload_sessions(id) ON DELETE CASCADE,
    client_ref VARCHAR(255) NOT NULL,
    attachment_id UUID REFERENCES attachments(id) ON DELETE SET NULL,
    drawing_no VARCHAR(150) NOT NULL DEFAULT '',
    part_no VARCHAR(150) NOT NULL DEFAULT '',
    file_role VARCHAR(20) NOT NULL DEFAULT 'other'
        CHECK (file_role IN ('assembly', 'part', 'material', 'craft', 'other')),
    original_name VARCHAR(255) NOT NULL,
    mime_type VARCHAR(255) NOT NULL DEFAULT 'application/octet-stream',
    expected_revision BIGINT,
    expected_size_bytes BIGINT,
    expected_sha256 CHAR(64),
    chunk_size_bytes BIGINT,
    status VARCHAR(20) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'uploading', 'uploaded', 'processing', 'ready', 'failed', 'committed')),
    object_key VARCHAR(500),
    staging_object_key VARCHAR(500),
    blob_id UUID REFERENCES file_blobs(id) ON DELETE SET NULL,
    processed_object_key VARCHAR(500),
    processed_blob_id UUID REFERENCES file_blobs(id) ON DELETE SET NULL,
    processed_size_bytes BIGINT NOT NULL DEFAULT 0 CHECK (processed_size_bytes >= 0),
    processed_sha256 CHAR(64),
    processed_mime_type VARCHAR(255),
    size_bytes BIGINT NOT NULL DEFAULT 0 CHECK (size_bytes >= 0),
    sha256 CHAR(64),
    error_message TEXT,
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (session_id, client_ref)
);

CREATE TABLE upload_session_chunks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    item_id UUID NOT NULL REFERENCES upload_session_items(id) ON DELETE CASCADE,
    part_number INTEGER NOT NULL CHECK (part_number >= 0),
    offset_bytes BIGINT NOT NULL CHECK (offset_bytes >= 0),
    size_bytes BIGINT NOT NULL CHECK (size_bytes > 0),
    sha256 CHAR(64) NOT NULL,
    storage_key VARCHAR(500) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (item_id, part_number)
);

CREATE TABLE storage_cleanup_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    storage_key VARCHAR(500) NOT NULL UNIQUE,
    reason VARCHAR(50) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'processing', 'completed')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_error TEXT,
    processing_started_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ
);

CREATE INDEX idx_upload_sessions_expiration ON upload_sessions(status, expires_at);
CREATE INDEX idx_upload_session_items_session ON upload_session_items(session_id, status);
CREATE INDEX idx_upload_session_chunks_item ON upload_session_chunks(item_id, part_number);
CREATE INDEX idx_storage_cleanup_jobs_due ON storage_cleanup_jobs(status, next_attempt_at);

CREATE OR REPLACE FUNCTION reject_published_revision_mutation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF OLD.workflow_status = 'published' AND to_jsonb(OLD) IS DISTINCT FROM to_jsonb(NEW) THEN
        RAISE EXCEPTION 'published part revision is immutable' USING ERRCODE = '55000';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER part_revisions_published_immutable
BEFORE UPDATE ON part_revisions
FOR EACH ROW EXECUTE FUNCTION reject_published_revision_mutation();

CREATE OR REPLACE FUNCTION validate_published_revision_pointer()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    revision_part_id UUID;
    revision_status VARCHAR(20);
BEGIN
    IF NEW.published_revision_id IS NULL THEN
        RETURN NEW;
    END IF;
    SELECT part_id, workflow_status
    INTO revision_part_id, revision_status
    FROM part_revisions
    WHERE id = NEW.published_revision_id;
    IF revision_part_id IS NULL OR revision_part_id <> NEW.id OR revision_status <> 'published' THEN
        RAISE EXCEPTION 'published revision pointer is invalid' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER parts_published_revision_valid
BEFORE INSERT OR UPDATE OF published_revision_id ON parts
FOR EACH ROW EXECUTE FUNCTION validate_published_revision_pointer();

CREATE TRIGGER drawings_set_updated_at
BEFORE UPDATE ON drawings
FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER parts_set_updated_at
BEFORE UPDATE ON parts
FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER drawing_part_relations_set_updated_at
BEFORE UPDATE ON drawing_part_relations
FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER drawing_signers_set_updated_at
BEFORE UPDATE ON drawing_signers
FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER drawing_attributes_set_updated_at
BEFORE UPDATE ON drawing_attributes
FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER drawing_attribute_fields_set_updated_at
BEFORE UPDATE ON drawing_attribute_fields
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

GRANT SELECT, INSERT, UPDATE, DELETE ON drawings, parts, part_revisions, drawing_part_relations TO cadguanliq_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON file_blobs, attachments, attachment_versions, part_revision_attachments TO cadguanliq_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON drawing_signers, drawing_boms, bom_items, drawing_branches TO cadguanliq_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON review_cases, review_case_nodes, review_actions TO cadguanliq_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON drawing_attributes, drawing_attribute_fields, drawing_attribute_values TO cadguanliq_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON edit_sessions, edit_session_tickets TO cadguanliq_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON upload_sessions, upload_session_items, upload_session_chunks, storage_cleanup_jobs TO cadguanliq_app;
