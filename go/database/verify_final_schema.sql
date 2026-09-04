\set ON_ERROR_STOP on

-- 最终模型约束验证。所有测试数据都在事务内创建并回滚。
BEGIN;

DO $$
DECLARE
    drawing_a UUID;
    drawing_b UUID;
    part_a UUID;
    part_b UUID;
    draft_revision UUID;
    published_revision UUID;
    owner_relation UUID;
    other_relation UUID;
BEGIN
    ASSERT to_regclass('public.parts') IS NOT NULL, 'parts 表不存在';
    ASSERT to_regclass('public.part_revisions') IS NOT NULL, 'part_revisions 表不存在';
    ASSERT to_regclass('public.drawing_part_relations') IS NOT NULL, 'drawing_part_relations 表不存在';
    ASSERT to_regclass('public.attachment_versions') IS NOT NULL, 'attachment_versions 表不存在';
    ASSERT to_regclass('public.part_revision_attachments') IS NOT NULL, 'part_revision_attachments 表不存在';
    ASSERT to_regclass('public.drawing_boms') IS NOT NULL, 'drawing_boms 表不存在';
    ASSERT to_regclass('public.structure_parts') IS NULL, '旧 structure_parts 表仍存在';
    ASSERT to_regclass('public.file_versions') IS NULL, '旧 file_versions 表仍存在';
    ASSERT to_regclass('public.borrow_records') IS NULL, '旧 borrow_records 表仍存在';

    INSERT INTO drawings (drawing_no, name, project)
    VALUES ('__schema-test-a', 'Schema Test A', 'Phase 1')
    RETURNING id INTO drawing_a;
    INSERT INTO drawings (drawing_no, name, project)
    VALUES ('__schema-test-b', 'Schema Test B', 'Phase 1')
    RETURNING id INTO drawing_b;

    INSERT INTO parts (part_no, normalized_part_no)
    VALUES ('__schema-part-a', '__SCHEMA-PART-A')
    RETURNING id INTO part_a;
    INSERT INTO parts (part_no, normalized_part_no)
    VALUES ('__schema-part-b', '__SCHEMA-PART-B')
    RETURNING id INTO part_b;

    INSERT INTO part_revisions (part_id, revision_no, name, workflow_status)
    VALUES (part_a, 1, 'Draft Part', 'draft')
    RETURNING id INTO draft_revision;

    BEGIN
        UPDATE parts SET published_revision_id = draft_revision WHERE id = part_a;
        RAISE EXCEPTION 'Draft revision was accepted as Published pointer';
    EXCEPTION WHEN SQLSTATE '23514' THEN
        NULL;
    END;

    INSERT INTO part_revisions (part_id, revision_no, name, workflow_status, published_at)
    VALUES (part_b, 1, 'Published Part', 'published', now())
    RETURNING id INTO published_revision;
    UPDATE parts SET published_revision_id = published_revision WHERE id = part_b;

    BEGIN
        UPDATE part_revisions SET name = 'Illegal Published Mutation' WHERE id = published_revision;
        RAISE EXCEPTION 'Published revision was mutable';
    EXCEPTION WHEN SQLSTATE '55000' THEN
        NULL;
    END;

    INSERT INTO drawing_part_relations (drawing_id, part_id, relation_type)
    VALUES (drawing_a, part_b, 'owned')
    RETURNING id INTO owner_relation;

    BEGIN
        INSERT INTO drawing_part_relations (drawing_id, part_id, relation_type)
        VALUES (drawing_b, part_b, 'owned');
        RAISE EXCEPTION 'Part accepted a second active owned relation';
    EXCEPTION WHEN SQLSTATE '23505' THEN
        NULL;
    END;

    INSERT INTO drawing_part_relations (drawing_id, part_id, relation_type)
    VALUES (drawing_b, part_a, 'owned')
    RETURNING id INTO other_relation;

    BEGIN
        INSERT INTO drawing_part_relations (drawing_id, part_id, parent_relation_id, relation_type)
        VALUES (drawing_a, part_a, other_relation, 'borrowed');
        RAISE EXCEPTION 'Cross-Drawing parent relation was accepted';
    EXCEPTION WHEN SQLSTATE '23503' THEN
        NULL;
    END;
END
$$;

ROLLBACK;
