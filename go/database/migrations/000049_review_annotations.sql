\set ON_ERROR_STOP on

-- 固定审核所见的文件版本。批注从不写入 CAD 文件。
CREATE TABLE review_annotation_files (
    review_case_id UUID NOT NULL REFERENCES review_cases(id) ON DELETE RESTRICT,
    attachment_id UUID NOT NULL REFERENCES attachments(id) ON DELETE RESTRICT,
    version_id UUID NOT NULL REFERENCES attachment_versions(id) ON DELETE RESTRICT,
    PRIMARY KEY (review_case_id, attachment_id),
    UNIQUE (review_case_id, attachment_id, version_id),
    FOREIGN KEY (attachment_id, version_id) REFERENCES attachment_versions(attachment_id, id)
);

CREATE FUNCTION capture_review_annotation_files() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.change_submission_id IS NOT NULL THEN
        INSERT INTO review_annotation_files
        SELECT NEW.id, attachment_id, submitted_attachment_version_id
        FROM change_request_submission_targets
        WHERE submission_id=NEW.change_submission_id AND submitted_attachment_version_id IS NOT NULL;
    ELSE
        INSERT INTO review_annotation_files
        SELECT NEW.id, a.id, a.current_version_id FROM attachments a
        WHERE a.deleted_at IS NULL AND a.current_version_id IS NOT NULL AND
          (a.drawing_id=NEW.drawing_id OR EXISTS
           (SELECT 1 FROM drawing_part_relations r WHERE r.drawing_id=NEW.drawing_id AND r.part_id=a.part_id AND r.status='active'));
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER review_capture_annotation_files AFTER INSERT ON review_cases
FOR EACH ROW EXECUTE FUNCTION capture_review_annotation_files();

-- 历史变更有冻结版本，可精确补齐；普通旧案例只补当前仍在审的案例。
INSERT INTO review_annotation_files
SELECT c.id,t.attachment_id,t.submitted_attachment_version_id FROM review_cases c
JOIN change_request_submission_targets t ON t.submission_id=c.change_submission_id
WHERE t.submitted_attachment_version_id IS NOT NULL ON CONFLICT DO NOTHING;
INSERT INTO review_annotation_files
SELECT c.id,a.id,a.current_version_id FROM review_cases c JOIN attachments a ON
 (a.drawing_id=c.drawing_id OR EXISTS (SELECT 1 FROM drawing_part_relations r WHERE r.drawing_id=c.drawing_id AND r.part_id=a.part_id AND r.status='active'))
WHERE c.change_submission_id IS NULL AND c.status='reviewing' AND a.deleted_at IS NULL AND a.current_version_id IS NOT NULL
ON CONFLICT DO NOTHING;

CREATE TABLE review_annotation_documents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    review_case_id UUID NOT NULL,
    attachment_id UUID NOT NULL,
    version_id UUID NOT NULL,
    node_id UUID NOT NULL REFERENCES review_case_nodes(id) ON DELETE RESTRICT,
    author_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    revision BIGINT NOT NULL DEFAULT 1 CHECK (revision>0),
    content JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (review_case_id,attachment_id,node_id,author_id),
    FOREIGN KEY (review_case_id,attachment_id,version_id) REFERENCES review_annotation_files(review_case_id,attachment_id,version_id)
);
CREATE TABLE review_annotation_events (
    id BIGSERIAL PRIMARY KEY,
    document_id UUID NOT NULL REFERENCES review_annotation_documents(id) ON DELETE RESTRICT,
    actor_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    revision BIGINT NOT NULL,
    content JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE review_annotation_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id UUID REFERENCES users(id) ON DELETE CASCADE,
    category VARCHAR(40) NOT NULL,
    text VARCHAR(500) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO review_annotation_templates(category,text) VALUES
 ('尺寸','此处缺少定位尺寸，请补充'),('尺寸','请核对尺寸与明细表是否一致'),
 ('公差','请核对配合公差'),('公差','请补充形位公差要求'),
 ('材料','请核对材料牌号'),('工艺','请补充表面处理要求'),
 ('工艺','请补充技术要求'),('核对','此处已核对'),('其他','与明细表不一致，请核实');

CREATE FUNCTION protect_annotation_version() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.deleted_at IS NOT NULL AND OLD.deleted_at IS NULL AND EXISTS
       (SELECT 1 FROM review_annotation_files WHERE version_id=OLD.id) THEN
       RAISE EXCEPTION '审核批注引用的文件版本不能删除';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER attachment_version_annotation_guard BEFORE UPDATE ON attachment_versions
FOR EACH ROW EXECUTE FUNCTION protect_annotation_version();

GRANT SELECT,INSERT ON review_annotation_files,review_annotation_events TO cadguanliq_app;
GRANT SELECT,INSERT,UPDATE ON review_annotation_documents TO cadguanliq_app;
GRANT SELECT,INSERT,UPDATE,DELETE ON review_annotation_templates TO cadguanliq_app;
GRANT USAGE,SELECT ON SEQUENCE review_annotation_events_id_seq TO cadguanliq_app;
