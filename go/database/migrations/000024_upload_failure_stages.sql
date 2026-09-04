\set ON_ERROR_STOP on

ALTER TABLE upload_session_items
    ADD COLUMN IF NOT EXISTS failure_stage VARCHAR(32);

ALTER TABLE upload_session_items
    DROP CONSTRAINT IF EXISTS upload_session_items_failure_stage_check;

ALTER TABLE upload_session_items
    ADD CONSTRAINT upload_session_items_failure_stage_check
    CHECK (failure_stage IS NULL OR failure_stage IN ('hash', 'upload', 'validation', 'conversion', 'commit'));

GRANT SELECT, INSERT, UPDATE, DELETE ON upload_session_items TO cadguanliq_app;
