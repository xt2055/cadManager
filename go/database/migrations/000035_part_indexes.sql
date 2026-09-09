ALTER TABLE attachment_title_blocks
    ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1
    CHECK (revision > 0);

CREATE TABLE IF NOT EXISTS part_indexes (
    attachment_id UUID NOT NULL,
    version_id UUID NOT NULL,
    revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
    source_snapshot_revision BIGINT NOT NULL DEFAULT 0
        CHECK (source_snapshot_revision >= 0),
    extraction_status VARCHAR(20) NOT NULL DEFAULT 'pending'
        CHECK (extraction_status IN ('pending', 'extracted', 'failed')),
    extraction_error TEXT NOT NULL DEFAULT '',
    selected_space_id VARCHAR(128),
    selection_mode VARCHAR(10) NOT NULL DEFAULT 'auto'
        CHECK (selection_mode IN ('auto', 'manual')),
    auto_fields JSONB NOT NULL DEFAULT '{}'::jsonb,
    manual_fields JSONB NOT NULL DEFAULT '{}'::jsonb,
    drawing_no VARCHAR(500) NOT NULL DEFAULT '',
    part_name VARCHAR(500) NOT NULL DEFAULT '',
    material VARCHAR(500) NOT NULL DEFAULT '',
    designer VARCHAR(500) NOT NULL DEFAULT '',
    checker VARCHAR(500) NOT NULL DEFAULT '',
    approver VARCHAR(500) NOT NULL DEFAULT '',
    drawing_date_raw VARCHAR(500) NOT NULL DEFAULT '',
    drawing_date DATE,
    scale VARCHAR(500) NOT NULL DEFAULT '',
    sheet_size VARCHAR(500) NOT NULL DEFAULT '',
    metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    confirmed_by UUID REFERENCES users(id) ON DELETE SET NULL,
    confirmed_at TIMESTAMPTZ,
    confirmed_snapshot_revision BIGINT
        CHECK (confirmed_snapshot_revision >= 0),
    edited_by UUID REFERENCES users(id) ON DELETE SET NULL,
    edited_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (attachment_id, version_id),
    FOREIGN KEY (attachment_id, version_id)
        REFERENCES attachment_versions(attachment_id, id) ON DELETE CASCADE,
    CHECK (jsonb_typeof(auto_fields) = 'object'),
    CHECK (jsonb_typeof(manual_fields) = 'object'),
    CHECK (jsonb_typeof(metadata_json) = 'object'),
    CHECK (
      (confirmed_at IS NULL AND confirmed_snapshot_revision IS NULL)
      OR
      (confirmed_at IS NOT NULL AND confirmed_snapshot_revision IS NOT NULL)
    )
);

CREATE INDEX IF NOT EXISTS idx_part_indexes_material ON part_indexes(material);
CREATE INDEX IF NOT EXISTS idx_part_indexes_designer ON part_indexes(designer);
CREATE INDEX IF NOT EXISTS idx_part_indexes_drawing_date ON part_indexes(drawing_date);
