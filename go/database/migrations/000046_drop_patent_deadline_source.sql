\set ON_ERROR_STOP on
BEGIN;

-- 缴费期限改由申请日与缴费周期自动推算，不再保存日期依据。
ALTER TABLE patent_records DROP COLUMN IF EXISTS deadline_source;

COMMIT;
