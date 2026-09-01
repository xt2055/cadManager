-- 图纸生命周期：新增「已存档」状态（生产 ⇄ 存档，全部审核通过后由创建者存档）
ALTER TABLE drawings DROP CONSTRAINT drawings_status_check;
ALTER TABLE drawings ADD CONSTRAINT drawings_status_check
    CHECK (status IN ('published', 'reviewing', 'draft', 'hidden', 'disabled', 'archived'));

ALTER TABLE structure_parts DROP CONSTRAINT structure_parts_status_check;
ALTER TABLE structure_parts ADD CONSTRAINT structure_parts_status_check
    CHECK (status IN ('published', 'reviewing', 'draft', 'hidden', 'disabled', 'archived'));
