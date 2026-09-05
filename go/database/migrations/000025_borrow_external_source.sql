\set ON_ERROR_STOP on

ALTER TABLE drawing_part_relations
    ADD COLUMN IF NOT EXISTS source_drawing_no VARCHAR(150);

CREATE INDEX IF NOT EXISTS idx_drawing_part_relations_source_drawing_no
    ON drawing_part_relations(source_drawing_no)
    WHERE source_drawing_no IS NOT NULL AND source_drawing_no <> '';

GRANT SELECT, INSERT, UPDATE, DELETE ON drawing_part_relations TO cadguanliq_app;
