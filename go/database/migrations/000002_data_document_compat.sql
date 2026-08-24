\set ON_ERROR_STOP on

CREATE TABLE IF NOT EXISTS data_documents (
    document_key VARCHAR(100) PRIMARY KEY,
    document JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

GRANT SELECT, INSERT, UPDATE, DELETE ON data_documents TO cadguanliq_app;
