ALTER TABLE attachments ADD COLUMN IF NOT EXISTS file_category text NOT NULL DEFAULT 'auto';
ALTER TABLE attachments ADD COLUMN IF NOT EXISTS is_primary_model boolean NOT NULL DEFAULT false;
ALTER TABLE attachments ADD CONSTRAINT attachments_file_category_check CHECK (file_category IN ('auto', 'drawing2d', 'model3d', 'other'));
CREATE UNIQUE INDEX attachments_primary_drawing_model ON attachments (drawing_id)
    WHERE is_primary_model AND deleted_at IS NULL AND drawing_id IS NOT NULL;
CREATE UNIQUE INDEX attachments_primary_part_model ON attachments (part_id)
    WHERE is_primary_model AND deleted_at IS NULL AND part_id IS NOT NULL;
