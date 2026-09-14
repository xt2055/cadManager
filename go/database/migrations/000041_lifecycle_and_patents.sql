\set ON_ERROR_STOP on
BEGIN;

ALTER TABLE review_cases ADD COLUMN IF NOT EXISTS change_submission_id uuid REFERENCES change_request_submissions(id) ON DELETE RESTRICT;
ALTER TABLE review_case_nodes ADD COLUMN IF NOT EXISTS signer_role text NOT NULL DEFAULT '';
ALTER TABLE review_cases ADD COLUMN IF NOT EXISTS flow_name_snapshot text NOT NULL DEFAULT '';
UPDATE review_cases c SET flow_name_snapshot=f.name FROM review_flows f WHERE f.id=c.flow_id;
CREATE UNIQUE INDEX IF NOT EXISTS review_cases_change_submission_uq ON review_cases(change_submission_id) WHERE change_submission_id IS NOT NULL;

-- 旧版管理员待验收工单恢复为可提交，保留原轮次，防止升级后无法进入完整审核。
WITH migrated AS (
 UPDATE change_requests cr SET status='executing',require_verify=true
 WHERE status='pending_verify' AND NOT EXISTS(SELECT 1 FROM review_cases c WHERE c.change_submission_id=cr.current_submission_id)
 RETURNING cr.id,cr.executor_id,cr.current_submission_id
), actions AS (
 INSERT INTO change_request_actions(request_id,actor_id,action,opinion)
 SELECT id,executor_id,'return','系统升级：原待验收轮次保留，请重新提交完整审核流程' FROM migrated
)
UPDATE change_request_submissions SET status='returned' WHERE id IN(SELECT current_submission_id FROM migrated) AND status='pending';
UPDATE change_requests SET require_verify=true WHERE status IN('pending_approval','executing');

-- 工作稿也是历史证据，不再按发布动作自动清除。
CREATE OR REPLACE FUNCTION prune_attachment_work(p_attachment uuid) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
  RETURN;
END;
$$;

CREATE TABLE IF NOT EXISTS lifecycle_documents (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 drawing_id uuid REFERENCES drawings(id) ON DELETE RESTRICT,
 change_request_id uuid REFERENCES change_requests(id) ON DELETE RESTRICT,
 patent_id uuid,
 category text NOT NULL,
 folder_path text NOT NULL DEFAULT '',
 title text NOT NULL,
 description text NOT NULL DEFAULT '',
 storage_key text NOT NULL UNIQUE,
 file_name text NOT NULL,
 mime_type text NOT NULL,
 size_bytes bigint NOT NULL CHECK(size_bytes >= 0),
 sha256 text NOT NULL,
 created_by uuid NOT NULL REFERENCES users(id),
 created_at timestamptz NOT NULL DEFAULT now(),
 CHECK(num_nonnulls(drawing_id, patent_id) = 1),
 CHECK(change_request_id IS NULL OR drawing_id IS NOT NULL)
);
CREATE INDEX IF NOT EXISTS lifecycle_documents_drawing_idx ON lifecycle_documents(drawing_id,created_at);

CREATE TABLE IF NOT EXISTS patent_records (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 number text NOT NULL UNIQUE CHECK(length(trim(number)) > 0),
 title text NOT NULL CHECK(length(trim(title)) > 0),
 patent_type text NOT NULL DEFAULT '',
 jurisdiction text NOT NULL DEFAULT '中国',
 owner_name text NOT NULL DEFAULT '',
 responsible_id uuid NOT NULL REFERENCES users(id),
 drawing_id uuid REFERENCES drawings(id) ON DELETE RESTRICT,
 fee_due date,
 expires_on date,
 deadline_source text NOT NULL,
 reminder_days integer NOT NULL DEFAULT 90 CHECK(reminder_days BETWEEN 1 AND 365),
 notes text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 revision integer NOT NULL DEFAULT 1
);
ALTER TABLE lifecycle_documents ADD CONSTRAINT lifecycle_documents_patent_fk FOREIGN KEY(patent_id) REFERENCES patent_records(id) ON DELETE RESTRICT;
CREATE TABLE IF NOT EXISTS patent_events (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 patent_id uuid NOT NULL REFERENCES patent_records(id) ON DELETE RESTRICT,
 actor_id uuid REFERENCES users(id),
 action text NOT NULL,
 detail jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS patent_events_patent_idx ON patent_events(patent_id,created_at);
CREATE UNIQUE INDEX patent_reminder_once ON patent_events(patent_id,(detail->>'kind'),(detail->>'deadline'),(detail->>'stage')) WHERE action='reminder';

CREATE OR REPLACE FUNCTION protect_lifecycle_evidence() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION '历史资料与记录不可覆盖或删除，请追加新资料'; END;
$$;
CREATE TRIGGER lifecycle_documents_immutable BEFORE UPDATE OR DELETE ON lifecycle_documents FOR EACH ROW EXECUTE FUNCTION protect_lifecycle_evidence();
CREATE TRIGGER patent_events_immutable BEFORE UPDATE OR DELETE ON patent_events FOR EACH ROW EXECUTE FUNCTION protect_lifecycle_evidence();
CREATE TABLE change_submission_documents (
 submission_id uuid NOT NULL REFERENCES change_request_submissions(id) ON DELETE RESTRICT,
 document_id uuid NOT NULL REFERENCES lifecycle_documents(id) ON DELETE RESTRICT,
 PRIMARY KEY(submission_id,document_id)
);
CREATE TRIGGER change_submission_documents_immutable BEFORE UPDATE OR DELETE ON change_submission_documents FOR EACH ROW EXECUTE FUNCTION protect_lifecycle_evidence();
CREATE TRIGGER change_submission_targets_immutable BEFORE UPDATE OR DELETE ON change_request_submission_targets FOR EACH ROW EXECUTE FUNCTION protect_lifecycle_evidence();
CREATE TRIGGER change_actions_immutable BEFORE UPDATE OR DELETE ON change_request_actions FOR EACH ROW EXECUTE FUNCTION protect_lifecycle_evidence();

CREATE OR REPLACE FUNCTION protect_historical_version() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.release_number IS NOT NULL OR EXISTS(SELECT 1 FROM change_request_submission_targets WHERE base_attachment_version_id=OLD.id OR submitted_attachment_version_id=OLD.id)
    OR EXISTS(SELECT 1 FROM attachments WHERE original_version_id=OLD.id)
    OR EXISTS(SELECT 1 FROM change_request_targets WHERE base_attachment_version_id=OLD.id OR work_attachment_version_id=OLD.id) THEN
   IF TG_OP='DELETE' THEN RAISE EXCEPTION '正式版本、原始版本和提交快照永久保留'; END IF;
   IF NEW.blob_id IS DISTINCT FROM OLD.blob_id OR NEW.attachment_id IS DISTINCT FROM OLD.attachment_id OR NEW.deleted_at IS DISTINCT FROM OLD.deleted_at
      OR NEW.original_name IS DISTINCT FROM OLD.original_name THEN RAISE EXCEPTION '历史版本内容不可覆盖或隐藏'; END IF;
 END IF;
 IF TG_OP='DELETE' THEN RETURN OLD; END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER attachment_versions_history_guard BEFORE UPDATE OR DELETE ON attachment_versions FOR EACH ROW EXECUTE FUNCTION protect_historical_version();
CREATE OR REPLACE FUNCTION protect_historical_attachment() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF EXISTS(SELECT 1 FROM attachment_versions WHERE attachment_id=OLD.id AND release_number IS NOT NULL)
 OR EXISTS(SELECT 1 FROM change_request_submission_targets WHERE attachment_id=OLD.id) THEN
  IF TG_OP='DELETE' THEN RAISE EXCEPTION '已有正式版本或变更记录的图纸不可删除'; END IF;
  IF NEW.deleted_at IS DISTINCT FROM OLD.deleted_at THEN RAISE EXCEPTION '历史图纸不可隐藏，请通过业务状态停用'; END IF;
 END IF;
 IF TG_OP='DELETE' THEN RETURN OLD; END IF; RETURN NEW;
END;
$$;
CREATE TRIGGER attachments_history_guard BEFORE UPDATE OF deleted_at OR DELETE ON attachments FOR EACH ROW EXECUTE FUNCTION protect_historical_attachment();

CREATE OR REPLACE FUNCTION protect_submission_content() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION '提交轮次永久保留'; END IF;
 IF (to_jsonb(NEW)-'status') IS DISTINCT FROM (to_jsonb(OLD)-'status') THEN RAISE EXCEPTION '已提交内容不可覆盖，请重新提交下一轮'; END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER submission_content_immutable BEFORE UPDATE OR DELETE ON change_request_submissions FOR EACH ROW EXECUTE FUNCTION protect_submission_content();
CREATE TRIGGER review_actions_immutable BEFORE UPDATE OR DELETE ON review_actions FOR EACH ROW EXECUTE FUNCTION protect_lifecycle_evidence();
CREATE OR REPLACE FUNCTION protect_signed_review_node() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.status IN('pass','rejected') THEN
  IF TG_OP='DELETE' THEN RAISE EXCEPTION '已签署节点永久保留'; END IF;
  IF to_jsonb(NEW) IS DISTINCT FROM to_jsonb(OLD) THEN RAISE EXCEPTION '已签署节点不可更改，请发起新一轮审核'; END IF;
 END IF;
 IF TG_OP='DELETE' THEN RETURN OLD; END IF; RETURN NEW;
END;
$$;
CREATE TRIGGER review_nodes_signed_guard BEFORE UPDATE OR DELETE ON review_case_nodes FOR EACH ROW EXECUTE FUNCTION protect_signed_review_node();

CREATE TABLE drawing_release_snapshots (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 drawing_id uuid NOT NULL REFERENCES drawings(id) ON DELETE RESTRICT,
 version text NOT NULL,
 snapshot jsonb NOT NULL,
 source text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(drawing_id,version)
);
CREATE TRIGGER drawing_release_snapshots_immutable BEFORE UPDATE OR DELETE ON drawing_release_snapshots FOR EACH ROW EXECUTE FUNCTION protect_lifecycle_evidence();
CREATE OR REPLACE FUNCTION capture_drawing_release(p_drawing uuid,p_source text) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
 INSERT INTO drawing_release_snapshots(drawing_id,version,snapshot,source)
 SELECT d.id,d.version,jsonb_build_object('drawing',to_jsonb(d),
  'files',COALESCE((SELECT jsonb_agg(jsonb_build_object('attachmentId',a.id,'name',a.logical_name,'versionId',a.current_version_id,'partId',a.part_id)) FROM attachments a WHERE a.deleted_at IS NULL AND (a.drawing_id=d.id OR EXISTS(SELECT 1 FROM drawing_part_relations r WHERE r.drawing_id=d.id AND r.part_id=a.part_id AND r.status='active'))),'[]'::jsonb),
  'structure',COALESCE((SELECT jsonb_agg(to_jsonb(r)||jsonb_build_object('partNo',p.part_no,'partRevision',to_jsonb(pr))) FROM drawing_part_relations r JOIN parts p ON p.id=r.part_id LEFT JOIN part_revisions pr ON pr.id=p.published_revision_id WHERE r.drawing_id=d.id AND r.status='active'),'[]'::jsonb),
  'signers',COALESCE((SELECT jsonb_agg(to_jsonb(s)) FROM drawing_signers s WHERE s.drawing_id=d.id),'[]'::jsonb),
  'bom',COALESCE((SELECT jsonb_agg(to_jsonb(i) ORDER BY i.item_no) FROM drawing_boms b JOIN bom_items i ON i.bom_id=b.id WHERE b.drawing_id=d.id),'[]'::jsonb)),p_source
 FROM drawings d WHERE d.id=p_drawing ON CONFLICT(drawing_id,version) DO NOTHING;
END;
$$;
CREATE OR REPLACE FUNCTION capture_release_on_archive() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.status='archived' AND (OLD.status IS DISTINCT FROM NEW.status OR OLD.version IS DISTINCT FROM NEW.version) THEN
   PERFORM capture_drawing_release(NEW.id,'正式发布');
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER zz_drawing_release_snapshot AFTER UPDATE OF status,version ON drawings FOR EACH ROW EXECUTE FUNCTION capture_release_on_archive();
CREATE FUNCTION publish_changed_part_revisions(p_request uuid,p_actor uuid) RETURNS void LANGUAGE plpgsql AS $$
DECLARE item record; baseline uuid; next_revision uuid; next_no integer;
BEGIN
 FOR item IN
  SELECT DISTINCT a.part_id FROM change_requests cr
  JOIN change_request_submission_targets t ON t.submission_id=cr.current_submission_id
  JOIN attachments a ON a.id=t.attachment_id
  WHERE cr.id=p_request AND a.part_id IS NOT NULL AND t.submitted_attachment_version_id IS NOT NULL
  ORDER BY a.part_id
 LOOP
  SELECT published_revision_id INTO baseline FROM parts WHERE id=item.part_id FOR UPDATE;
  IF baseline IS NULL THEN SELECT id INTO baseline FROM part_revisions WHERE part_id=item.part_id ORDER BY revision_no DESC LIMIT 1; END IF;
  IF baseline IS NULL THEN RAISE EXCEPTION '零件缺少基础修订，无法发布'; END IF;
  SELECT COALESCE(MAX(revision_no),0)+1 INTO next_no FROM part_revisions WHERE part_id=item.part_id;
  INSERT INTO part_revisions(part_id,revision_no,version,name,material,spec,weight,surface_treatment,part_type,workflow_status,based_on_revision_id,created_by,published_by,published_at)
  SELECT part_id,next_no,'V'||next_no::text,name,material,spec,weight,surface_treatment,part_type,'published',id,p_actor,p_actor,now() FROM part_revisions WHERE id=baseline RETURNING id INTO next_revision;
  INSERT INTO part_revision_attachments(part_revision_id,attachment_version_id,role,sort_order)
  SELECT next_revision,a.current_version_id,a.file_role,(row_number() OVER(PARTITION BY a.file_role ORDER BY a.id)-1)::integer
  FROM attachments a WHERE a.part_id=item.part_id AND a.deleted_at IS NULL AND a.current_version_id IS NOT NULL;
  UPDATE parts SET published_revision_id=next_revision,updated_by=p_actor,updated_at=now() WHERE id=item.part_id;
 END LOOP;
END;
$$;
GRANT EXECUTE ON FUNCTION publish_changed_part_revisions(uuid,uuid) TO cadguanliq_app;
SELECT capture_drawing_release(id,'历史在用版本导入快照') FROM drawings WHERE status='archived';
GRANT SELECT,INSERT ON change_submission_documents,drawing_release_snapshots TO cadguanliq_app;
GRANT EXECUTE ON FUNCTION capture_drawing_release(uuid,text) TO cadguanliq_app;
GRANT SELECT,INSERT ON lifecycle_documents,patent_events TO cadguanliq_app;
GRANT SELECT,INSERT,UPDATE ON patent_records TO cadguanliq_app;
COMMIT;
