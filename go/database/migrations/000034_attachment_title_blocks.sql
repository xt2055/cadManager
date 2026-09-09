CREATE TABLE IF NOT EXISTS attachment_title_blocks (
    attachment_id UUID NOT NULL,
    version_id UUID NOT NULL,
    payload JSONB NOT NULL,
    extracted_by UUID REFERENCES users(id) ON DELETE SET NULL,
    extracted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (attachment_id, version_id),
    FOREIGN KEY (attachment_id, version_id)
        REFERENCES attachment_versions(attachment_id, id) ON DELETE CASCADE
);
