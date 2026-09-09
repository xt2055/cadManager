ALTER TABLE attachments
    ADD COLUMN IF NOT EXISTS author VARCHAR(100) NOT NULL DEFAULT '';

ALTER TABLE bom_items
    ADD COLUMN IF NOT EXISTS item_code VARCHAR(255) NOT NULL DEFAULT '';

UPDATE bom_items bi
SET item_code = p.part_no
FROM parts p
WHERE bi.part_id = p.id
  AND bi.item_code = '';
