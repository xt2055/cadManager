\set ON_ERROR_STOP on
BEGIN;

-- 年费自申请日（start_date）起按固定月数周期缴纳，本期缴费截止日期由后端自动推算。
ALTER TABLE patent_records ADD COLUMN IF NOT EXISTS start_date date;
ALTER TABLE patent_records ADD COLUMN IF NOT EXISTS fee_cycle_months integer NOT NULL DEFAULT 12 CHECK(fee_cycle_months BETWEEN 1 AND 120);

COMMIT;
