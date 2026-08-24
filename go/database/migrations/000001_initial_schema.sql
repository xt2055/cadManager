\set ON_ERROR_STOP on

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS users (
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

CREATE TABLE IF NOT EXISTS user_roles (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role VARCHAR(30) NOT NULL
        CHECK (role IN ('admin', 'designer', 'reviewer')),
    PRIMARY KEY (user_id, role)
);

CREATE TABLE IF NOT EXISTS sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash CHAR(64) NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);

CREATE TABLE IF NOT EXISTS drawings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    drawing_no VARCHAR(150) NOT NULL UNIQUE,
    name VARCHAR(255) NOT NULL,
    project VARCHAR(255) NOT NULL,
    kind VARCHAR(20) NOT NULL DEFAULT '总图'
        CHECK (kind IN ('总图', '零件图')),
    vendor VARCHAR(255) NOT NULL DEFAULT '',
    material VARCHAR(255) NOT NULL DEFAULT '—',
    status VARCHAR(20) NOT NULL DEFAULT 'draft'
        CHECK (status IN ('published', 'reviewing', 'draft', 'hidden', 'disabled')),
    version VARCHAR(50) NOT NULL DEFAULT 'v1.0',
    borrow_from VARCHAR(150),
    remark TEXT,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    updated_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_drawings_project ON drawings(project);
CREATE INDEX IF NOT EXISTS idx_drawings_status ON drawings(status);
CREATE INDEX IF NOT EXISTS idx_drawings_updated_at ON drawings(updated_at DESC);

CREATE TABLE IF NOT EXISTS structure_parts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    drawing_id UUID NOT NULL REFERENCES drawings(id) ON DELETE CASCADE,
    parent_part_id UUID REFERENCES structure_parts(id) ON DELETE RESTRICT,
    part_no VARCHAR(150) NOT NULL UNIQUE,
    name VARCHAR(255) NOT NULL,
    project VARCHAR(255),
    material VARCHAR(255) NOT NULL DEFAULT '—',
    spec VARCHAR(255) NOT NULL DEFAULT '',
    weight NUMERIC(14, 4) NOT NULL DEFAULT 0 CHECK (weight >= 0),
    surface_treatment VARCHAR(255) NOT NULL DEFAULT '',
    manufacturing_type VARCHAR(20) NOT NULL DEFAULT '自制件'
        CHECK (manufacturing_type IN ('自制件', '外协件', '标准件', '外购件')),
    quantity NUMERIC(14, 4) NOT NULL DEFAULT 1 CHECK (quantity > 0),
    status VARCHAR(20) NOT NULL DEFAULT 'draft'
        CHECK (status IN ('published', 'reviewing', 'draft', 'hidden', 'disabled')),
    version VARCHAR(50) NOT NULL DEFAULT 'v1.0',
    vendor VARCHAR(255),
    borrow_from VARCHAR(150),
    remark TEXT,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    updated_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT structure_parts_parent_not_self CHECK (parent_part_id IS NULL OR parent_part_id <> id)
);

CREATE INDEX IF NOT EXISTS idx_structure_parts_drawing_id ON structure_parts(drawing_id);
CREATE INDEX IF NOT EXISTS idx_structure_parts_parent_part_id ON structure_parts(parent_part_id);
CREATE INDEX IF NOT EXISTS idx_structure_parts_name ON structure_parts(name);

CREATE TABLE IF NOT EXISTS drawing_signers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    drawing_id UUID REFERENCES drawings(id) ON DELETE CASCADE,
    part_id UUID REFERENCES structure_parts(id) ON DELETE CASCADE,
    role VARCHAR(20) NOT NULL
        CHECK (role IN ('设计', '校对', '审核', '工艺', '标准化', '批准')),
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    signer_name VARCHAR(100) NOT NULL DEFAULT '待定',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT drawing_signers_one_owner CHECK ((drawing_id IS NOT NULL) <> (part_id IS NOT NULL)),
    CONSTRAINT drawing_signers_name_not_empty CHECK (length(trim(signer_name)) > 0)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_drawing_signers_drawing_role
    ON drawing_signers(drawing_id, role)
    WHERE drawing_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_drawing_signers_part_role
    ON drawing_signers(part_id, role)
    WHERE part_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS attachments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    drawing_id UUID REFERENCES drawings(id) ON DELETE CASCADE,
    part_id UUID REFERENCES structure_parts(id) ON DELETE CASCADE,
    file_role VARCHAR(20) NOT NULL
        CHECK (file_role IN ('assembly', 'part', 'material', 'craft', 'other')),
    storage_key VARCHAR(500) NOT NULL UNIQUE,
    original_name VARCHAR(255) NOT NULL,
    mime_type VARCHAR(255) NOT NULL DEFAULT 'application/octet-stream',
    size_bytes BIGINT NOT NULL DEFAULT 0 CHECK (size_bytes >= 0),
    sha256 CHAR(64),
    version VARCHAR(50) NOT NULL DEFAULT 'v1.0',
    previewable BOOLEAN NOT NULL DEFAULT false,
    uploaded_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT attachments_one_owner CHECK ((drawing_id IS NOT NULL) <> (part_id IS NOT NULL))
);

CREATE INDEX IF NOT EXISTS idx_attachments_drawing_id ON attachments(drawing_id);
CREATE INDEX IF NOT EXISTS idx_attachments_part_id ON attachments(part_id);
CREATE INDEX IF NOT EXISTS idx_attachments_file_role ON attachments(file_role);

CREATE TABLE IF NOT EXISTS drawing_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    drawing_id UUID REFERENCES drawings(id) ON DELETE CASCADE,
    part_id UUID REFERENCES structure_parts(id) ON DELETE CASCADE,
    version VARCHAR(50) NOT NULL,
    note TEXT NOT NULL DEFAULT '',
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT drawing_versions_one_owner CHECK ((drawing_id IS NOT NULL) <> (part_id IS NOT NULL))
);

CREATE INDEX IF NOT EXISTS idx_drawing_versions_drawing_id ON drawing_versions(drawing_id);
CREATE INDEX IF NOT EXISTS idx_drawing_versions_part_id ON drawing_versions(part_id);

CREATE TABLE IF NOT EXISTS bom_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    drawing_id UUID REFERENCES drawings(id) ON DELETE CASCADE,
    part_id UUID REFERENCES structure_parts(id) ON DELETE CASCADE,
    source_attachment_id UUID REFERENCES attachments(id) ON DELETE SET NULL,
    item_no INTEGER NOT NULL,
    name VARCHAR(255) NOT NULL,
    spec VARCHAR(255) NOT NULL DEFAULT '—',
    quantity NUMERIC(14, 4) NOT NULL DEFAULT 0 CHECK (quantity >= 0),
    weight NUMERIC(14, 4) NOT NULL DEFAULT 0 CHECK (weight >= 0),
    remark TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT bom_items_one_owner CHECK ((drawing_id IS NOT NULL) <> (part_id IS NOT NULL))
);

CREATE INDEX IF NOT EXISTS idx_bom_items_drawing_id ON bom_items(drawing_id);
CREATE INDEX IF NOT EXISTS idx_bom_items_part_id ON bom_items(part_id);

CREATE TABLE IF NOT EXISTS review_flows (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS review_flow_nodes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    flow_id UUID NOT NULL REFERENCES review_flows(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    signer_role VARCHAR(20),
    required BOOLEAN NOT NULL DEFAULT true,
    node_order INTEGER NOT NULL CHECK (node_order > 0),
    UNIQUE(flow_id, node_order),
    UNIQUE(flow_id, name)
);

CREATE TABLE IF NOT EXISTS review_cases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    drawing_id UUID REFERENCES drawings(id) ON DELETE CASCADE,
    part_id UUID REFERENCES structure_parts(id) ON DELETE CASCADE,
    flow_id UUID REFERENCES review_flows(id) ON DELETE RESTRICT,
    status VARCHAR(20) NOT NULL DEFAULT 'reviewing'
        CHECK (status IN ('pending', 'reviewing', 'published', 'rejected', 'cancelled')),
    initiator_id UUID REFERENCES users(id) ON DELETE SET NULL,
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    CONSTRAINT review_cases_one_owner CHECK ((drawing_id IS NOT NULL) <> (part_id IS NOT NULL))
);

CREATE INDEX IF NOT EXISTS idx_review_cases_drawing_id ON review_cases(drawing_id);
CREATE INDEX IF NOT EXISTS idx_review_cases_part_id ON review_cases(part_id);
CREATE INDEX IF NOT EXISTS idx_review_cases_status ON review_cases(status);

CREATE UNIQUE INDEX IF NOT EXISTS uq_review_cases_active_drawing
    ON review_cases(drawing_id)
    WHERE drawing_id IS NOT NULL AND status IN ('pending', 'reviewing');
CREATE UNIQUE INDEX IF NOT EXISTS uq_review_cases_active_part
    ON review_cases(part_id)
    WHERE part_id IS NOT NULL AND status IN ('pending', 'reviewing');

CREATE TABLE IF NOT EXISTS review_case_nodes (
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
    UNIQUE(review_case_id, node_order),
    UNIQUE(review_case_id, name)
);

CREATE TABLE IF NOT EXISTS review_actions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    review_case_id UUID NOT NULL REFERENCES review_cases(id) ON DELETE CASCADE,
    node_id UUID REFERENCES review_case_nodes(id) ON DELETE SET NULL,
    actor_id UUID REFERENCES users(id) ON DELETE SET NULL,
    action VARCHAR(30) NOT NULL
        CHECK (action IN ('start', 'pass', 'reject', 'cancel', 'reopen')),
    opinion TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS borrow_records (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_drawing_id UUID REFERENCES drawings(id) ON DELETE SET NULL,
    source_part_id UUID REFERENCES structure_parts(id) ON DELETE SET NULL,
    target_drawing_id UUID REFERENCES drawings(id) ON DELETE SET NULL,
    target_part_id UUID REFERENCES structure_parts(id) ON DELETE SET NULL,
    direction VARCHAR(10) NOT NULL CHECK (direction IN ('in', 'out')),
    status VARCHAR(20) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'archived')),
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    archived_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS audit_logs (
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

CREATE INDEX IF NOT EXISTS idx_audit_logs_actor_id ON audit_logs(actor_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_resource ON audit_logs(resource_type, resource_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at ON audit_logs(created_at DESC);

CREATE TABLE IF NOT EXISTS update_manifests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    version VARCHAR(50) NOT NULL,
    platform VARCHAR(100) NOT NULL,
    notes TEXT NOT NULL DEFAULT '',
    published_at TIMESTAMPTZ,
    download_url TEXT,
    mandatory BOOLEAN NOT NULL DEFAULT false,
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(version, platform)
);

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS users_set_updated_at ON users;
CREATE TRIGGER users_set_updated_at
BEFORE UPDATE ON users
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

DROP TRIGGER IF EXISTS drawings_set_updated_at ON drawings;
CREATE TRIGGER drawings_set_updated_at
BEFORE UPDATE ON drawings
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

DROP TRIGGER IF EXISTS structure_parts_set_updated_at ON structure_parts;
CREATE TRIGGER structure_parts_set_updated_at
BEFORE UPDATE ON structure_parts
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

DROP TRIGGER IF EXISTS drawing_signers_set_updated_at ON drawing_signers;
CREATE TRIGGER drawing_signers_set_updated_at
BEFORE UPDATE ON drawing_signers
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

DROP TRIGGER IF EXISTS review_flows_set_updated_at ON review_flows;
CREATE TRIGGER review_flows_set_updated_at
BEFORE UPDATE ON review_flows
FOR EACH ROW EXECUTE FUNCTION set_updated_at();
