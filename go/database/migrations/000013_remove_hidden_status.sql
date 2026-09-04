-- 开发阶段清理旧隐藏状态：统一使用 disabled 表示禁用且对普通图纸库隐藏。
UPDATE drawings SET status = 'disabled' WHERE status = 'hidden';
UPDATE structure_parts SET status = 'disabled' WHERE status = 'hidden';

ALTER TABLE drawings DROP CONSTRAINT IF EXISTS drawings_status_check;
ALTER TABLE drawings ADD CONSTRAINT drawings_status_check
    CHECK (status IN ('published', 'reviewing', 'draft', 'disabled', 'archived'));

ALTER TABLE structure_parts DROP CONSTRAINT IF EXISTS structure_parts_status_check;
ALTER TABLE structure_parts ADD CONSTRAINT structure_parts_status_check
    CHECK (status IN ('published', 'reviewing', 'draft', 'disabled', 'archived'));

ALTER TABLE drawings ADD COLUMN IF NOT EXISTS status_before_disabled VARCHAR(20);
ALTER TABLE structure_parts ADD COLUMN IF NOT EXISTS status_before_disabled VARCHAR(20);
