\set ON_ERROR_STOP on

CREATE TABLE IF NOT EXISTS drawing_attributes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL UNIQUE,
    required BOOLEAN NOT NULL DEFAULT false,
    enabled BOOLEAN NOT NULL DEFAULT true,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS drawing_attribute_fields (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    attribute_id UUID NOT NULL REFERENCES drawing_attributes(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(attribute_id, name),
    UNIQUE(attribute_id, id)
);

CREATE TABLE IF NOT EXISTS drawing_attribute_values (
    drawing_id UUID NOT NULL REFERENCES drawings(id) ON DELETE CASCADE,
    attribute_id UUID NOT NULL REFERENCES drawing_attributes(id) ON DELETE CASCADE,
    field_id UUID NOT NULL REFERENCES drawing_attribute_fields(id) ON DELETE RESTRICT,
    PRIMARY KEY(drawing_id, attribute_id)
);

DROP TABLE IF EXISTS drawing_branches;
CREATE TABLE drawing_branches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_drawing_id UUID REFERENCES drawings(id) ON DELETE SET NULL,
    target_drawing_id UUID REFERENCES drawings(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    status VARCHAR(20) NOT NULL DEFAULT '使用中'
        CHECK (status IN ('使用中', '已禁用')),
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_drawing_attribute_fields_attribute
    ON drawing_attribute_fields(attribute_id, sort_order);
CREATE INDEX IF NOT EXISTS idx_drawing_attribute_values_drawing
    ON drawing_attribute_values(drawing_id);
CREATE INDEX IF NOT EXISTS idx_drawing_branches_source
    ON drawing_branches(source_drawing_id);
CREATE INDEX IF NOT EXISTS idx_drawing_branches_target
    ON drawing_branches(target_drawing_id);

DROP TRIGGER IF EXISTS drawing_attributes_set_updated_at ON drawing_attributes;
CREATE TRIGGER drawing_attributes_set_updated_at
BEFORE UPDATE ON drawing_attributes
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

DROP TRIGGER IF EXISTS drawing_attribute_fields_set_updated_at ON drawing_attribute_fields;
CREATE TRIGGER drawing_attribute_fields_set_updated_at
BEFORE UPDATE ON drawing_attribute_fields
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

GRANT SELECT, INSERT, UPDATE, DELETE ON drawing_attributes TO cadguanliq_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON drawing_attribute_fields TO cadguanliq_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON drawing_attribute_values TO cadguanliq_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON drawing_branches TO cadguanliq_app;
