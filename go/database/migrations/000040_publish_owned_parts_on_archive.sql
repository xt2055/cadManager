\set ON_ERROR_STOP on

BEGIN;

CREATE FUNCTION publish_owned_parts_for_drawing(p_drawing UUID, p_actor UUID) RETURNS VOID
LANGUAGE plpgsql
AS $$
BEGIN
    -- 图纸存档即冻结结构：把自有零件当前修订标为已发布，供后续借用。
    -- 借用件仍归源图纸管，这里不改它们的发布指针。
    WITH owned AS (
        SELECT DISTINCT r.part_id
        FROM drawing_part_relations r
        WHERE r.drawing_id = p_drawing
          AND r.relation_type = 'owned'
          AND r.status = 'active'
    ),
    latest AS (
        SELECT DISTINCT ON (pr.part_id) pr.id
        FROM part_revisions pr
        JOIN owned o ON o.part_id = pr.part_id
        ORDER BY pr.part_id, pr.revision_no DESC, pr.created_at DESC, pr.id DESC
    )
    UPDATE part_revisions pr
    SET workflow_status = 'published',
        published_by = COALESCE(pr.published_by, p_actor),
        published_at = COALESCE(pr.published_at, now())
    FROM latest
    WHERE pr.id = latest.id
      AND pr.workflow_status IS DISTINCT FROM 'published';

    UPDATE parts p
    SET published_revision_id = latest.id,
        updated_by = COALESCE(p_actor, p.updated_by),
        updated_at = now()
    FROM (
        SELECT DISTINCT ON (pr.part_id) pr.id, pr.part_id
        FROM part_revisions pr
        JOIN drawing_part_relations r ON r.part_id = pr.part_id
        WHERE r.drawing_id = p_drawing
          AND r.relation_type = 'owned'
          AND r.status = 'active'
          AND pr.workflow_status = 'published'
        ORDER BY pr.part_id, pr.revision_no DESC, pr.created_at DESC, pr.id DESC
    ) latest
    WHERE p.id = latest.part_id
      AND p.published_revision_id IS DISTINCT FROM latest.id;
END;
$$;

CREATE FUNCTION publish_owned_parts_on_archive() RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.status = 'archived' AND OLD.status IS DISTINCT FROM 'archived' THEN
        PERFORM publish_owned_parts_for_drawing(NEW.id, NEW.updated_by);
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER drawings_archive_publish_owned_parts
AFTER UPDATE OF status ON drawings
FOR EACH ROW EXECUTE FUNCTION publish_owned_parts_on_archive();

SELECT publish_owned_parts_for_drawing(id, updated_by)
FROM drawings
WHERE status = 'archived';

GRANT EXECUTE ON FUNCTION publish_owned_parts_for_drawing(UUID, UUID), publish_owned_parts_on_archive() TO cadguanliq_app;

COMMIT;
