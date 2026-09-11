\set ON_ERROR_STOP on

BEGIN;

ALTER TABLE attachments ADD COLUMN original_version_id UUID REFERENCES attachment_versions(id) ON DELETE SET NULL;
ALTER TABLE attachment_versions ADD COLUMN release_number INTEGER CHECK (release_number > 0);
CREATE UNIQUE INDEX uq_attachment_formal_number ON attachment_versions(attachment_id, release_number) WHERE release_number IS NOT NULL;

-- 原始文件独立留存；不把开发过程中自动登记的 release 标签当成正式发布。
UPDATE attachments a SET original_version_id = (
    SELECT v.id FROM attachment_versions v WHERE v.attachment_id = a.id AND v.deleted_at IS NULL
    ORDER BY v.created_at, v.id LIMIT 1
);
WITH formal AS (
    SELECT v.id, row_number() OVER (PARTITION BY v.attachment_id ORDER BY v.created_at, v.id)::integer AS number
    FROM attachment_versions v
    WHERE v.deleted_at IS NULL AND (
        EXISTS (SELECT 1 FROM change_request_submission_targets t JOIN change_request_submissions s ON s.id = t.submission_id
                WHERE s.status = 'accepted' AND (t.submitted_attachment_version_id = v.id OR t.base_attachment_version_id = v.id))
        OR EXISTS (SELECT 1 FROM attachments a WHERE a.id = v.attachment_id AND a.current_version_id = v.id AND (
            EXISTS (SELECT 1 FROM drawings d WHERE d.id = a.drawing_id AND d.status = 'archived')
            OR EXISTS (SELECT 1 FROM drawing_part_relations r JOIN drawings d ON d.id = r.drawing_id
                       WHERE r.part_id = a.part_id AND r.status = 'active' AND d.status = 'archived')))
    )
)
UPDATE attachment_versions v SET release_number = f.number, version = '_formal_' || v.id::text, version_kind = 'release'
FROM formal f WHERE f.id = v.id;
-- 避免旧工作标签与新的正式编号冲突。
UPDATE attachment_versions v SET version = '_work_' || v.id::text
WHERE release_number IS NULL AND version ~ '^V[0-9]+$';
UPDATE attachment_versions SET version = 'V' || release_number::text WHERE release_number IS NOT NULL;

CREATE FUNCTION publish_attachment_formal(p_attachment UUID, p_version UUID) RETURNS TEXT
LANGUAGE plpgsql AS $$
DECLARE n INTEGER; original_id UUID;
BEGIN
    PERFORM 1 FROM attachments WHERE id = p_attachment AND deleted_at IS NULL FOR UPDATE;
    IF NOT FOUND THEN RAISE EXCEPTION '待发布附件不存在'; END IF;
    IF EXISTS (SELECT 1 FROM edit_sessions WHERE attachment_id = p_attachment AND status = 'active') THEN
        RAISE EXCEPTION '请先结束本地编辑后再存档或发布';
    END IF;
    SELECT release_number INTO n FROM attachment_versions
    WHERE id = p_version AND attachment_id = p_attachment AND deleted_at IS NULL;
    IF NOT FOUND THEN RAISE EXCEPTION '待发布版本不存在'; END IF;
    SELECT id INTO original_id FROM attachment_versions WHERE attachment_id = p_attachment AND deleted_at IS NULL ORDER BY created_at, id LIMIT 1;
    UPDATE attachments SET original_version_id = COALESCE(original_version_id, original_id) WHERE id = p_attachment;
    IF n IS NULL THEN
        SELECT COALESCE(MAX(release_number), 0) + 1 INTO n FROM attachment_versions WHERE attachment_id = p_attachment;
        UPDATE attachment_versions SET version = '_work_' || id::text
        WHERE attachment_id = p_attachment AND release_number IS NULL AND version = 'V' || n::text AND id <> p_version;
        UPDATE attachment_versions SET release_number = n, version = 'V' || n::text, version_kind = 'release' WHERE id = p_version;
    END IF;
    UPDATE attachments SET current_version_id = p_version, revision = revision + 1 WHERE id = p_attachment;
    RETURN 'V' || n::text;
END $$;

CREATE FUNCTION prune_attachment_work(p_attachment UUID) RETURNS VOID
LANGUAGE plpgsql AS $$
DECLARE candidate RECORD;
BEGIN
    PERFORM 1 FROM attachments WHERE id = p_attachment FOR UPDATE;
    IF EXISTS (SELECT 1 FROM edit_sessions WHERE attachment_id = p_attachment AND status = 'active') THEN RETURN; END IF;
    -- 其他项目也可能引用同一零件；有未结束工单时延后清理。
    IF EXISTS (SELECT 1 FROM change_request_targets t JOIN change_requests r ON r.id = t.request_id
               WHERE t.attachment_id = p_attachment AND r.status IN ('pending_approval', 'executing', 'pending_verify')) THEN RETURN; END IF;
    FOR candidate IN
        DELETE FROM attachment_versions v USING attachments a
        WHERE a.id = p_attachment AND v.attachment_id = a.id AND v.release_number IS NULL
          AND v.id IS DISTINCT FROM a.original_version_id AND v.id IS DISTINCT FROM a.current_version_id
          AND NOT EXISTS (SELECT 1 FROM part_revision_attachments p WHERE p.attachment_version_id = v.id)
        RETURNING v.blob_id
    LOOP
        -- 同内容文件可能被多份图纸共用，只有完全失去引用的对象才进入物理清理队列。
        WITH orphan AS (
            DELETE FROM file_blobs b WHERE b.id = candidate.blob_id
              AND NOT EXISTS (SELECT 1 FROM attachment_versions v WHERE v.blob_id = b.id)
              AND NOT EXISTS (SELECT 1 FROM upload_session_items i WHERE i.blob_id = b.id OR i.processed_blob_id = b.id)
              AND NOT EXISTS (SELECT 1 FROM cad_conversion_jobs c WHERE c.source_blob_id = b.id)
            RETURNING b.storage_key
        )
        INSERT INTO storage_cleanup_jobs(storage_key, reason)
        SELECT storage_key, 'formal-version-prune' FROM orphan
        ON CONFLICT (storage_key) DO UPDATE SET status = 'pending', next_attempt_at = now(), last_error = NULL;
    END LOOP;
END $$;

CREATE FUNCTION archive_drawing_formal_versions() RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE target RECORD;
BEGIN
    IF NEW.status <> 'archived' OR OLD.status = 'archived' THEN RETURN NEW; END IF;
    FOR target IN
        SELECT a.id, a.current_version_id FROM attachments a
        WHERE a.deleted_at IS NULL AND a.current_version_id IS NOT NULL
          AND (a.drawing_id = NEW.id OR EXISTS (
              SELECT 1 FROM drawing_part_relations r WHERE r.drawing_id = NEW.id AND r.part_id = a.part_id AND r.status = 'active'))
        ORDER BY a.id
    LOOP
        PERFORM publish_attachment_formal(target.id, target.current_version_id);
        PERFORM prune_attachment_work(target.id);
    END LOOP;
    NEW.version := 'V1';
    RETURN NEW;
END $$;
CREATE TRIGGER drawings_archive_formal_versions BEFORE UPDATE OF status ON drawings
FOR EACH ROW EXECUTE FUNCTION archive_drawing_formal_versions();

GRANT EXECUTE ON FUNCTION publish_attachment_formal(UUID, UUID), prune_attachment_work(UUID) TO cadguanliq_app;
COMMIT;
