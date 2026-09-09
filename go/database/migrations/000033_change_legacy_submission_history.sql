\set ON_ERROR_STOP on

-- Existing snapshots may contain merged, untraceable pre-upgrade differences.
-- Preserve them, but never present them as a precisely reconstructed round.
ALTER TABLE change_request_submissions ADD COLUMN legacy_history BOOLEAN NOT NULL DEFAULT false;
UPDATE change_request_submissions SET legacy_history = true;
