\set ON_ERROR_STOP on

-- 单资源接口以 revision 作为比较并交换（CAS）条件。旧数据统一从 1 起步，
-- 后续每次成功修改递增，避免旧页面的内存快照覆盖较新的数据。
ALTER TABLE drawings
    ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1;

ALTER TABLE structure_parts
    ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'drawings_revision_positive') THEN
        ALTER TABLE drawings ADD CONSTRAINT drawings_revision_positive CHECK (revision > 0);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'structure_parts_revision_positive') THEN
        ALTER TABLE structure_parts ADD CONSTRAINT structure_parts_revision_positive CHECK (revision > 0);
    END IF;
END
$$;

GRANT SELECT, INSERT, UPDATE, DELETE ON drawings TO cadguanliq_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON structure_parts TO cadguanliq_app;
