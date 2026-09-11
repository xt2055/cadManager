package drawing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// AtomicRepository 是 Phase 2 的命令端口。查询端仍可使用 Repository，
// 结构写入、借用和 Fork 必须经过这些事务方法。
type AtomicRepository interface {
	UpdateRelation(context.Context, string, UpdateRelationInput, string) (Relation, error)
	Borrow(context.Context, string, BorrowInput, string, string) (Relation, error)
	ForkBorrowedPart(context.Context, string, ForkInput, string, string) (Part, error)
	CreateDraftRevision(context.Context, string, CreateRevisionInput, string) (PartRevision, error)
	UpdateDraftRevision(context.Context, string, UpdateRevisionInput, string) (PartRevision, error)
	TransitionRevision(context.Context, string, string, string) (PartRevision, error)
	GetRevision(context.Context, string) (PartRevision, error)
	GetBOM(context.Context, string) (BOM, error)
	ReplaceBOM(context.Context, string, UpdateBOMInput, string) (BOM, error)
}

func (repository *PGRepository) UpdateRelation(ctx context.Context, id string, input UpdateRelationInput, userID string) (Relation, error) {
	if input.ExpectedRevision == nil || *input.ExpectedRevision < 1 {
		return Relation{}, ErrRevisionRequired
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return Relation{}, fmt.Errorf("开始修改结构关系事务失败: %w", err)
	}
	defer tx.Rollback(ctx)

	var drawingID, relationType, status string
	var currentRevision int64
	if err := tx.QueryRow(ctx, `SELECT drawing_id::text, relation_type, status, revision FROM drawing_part_relations WHERE id = $1::uuid`, id).
		Scan(&drawingID, &relationType, &status, &currentRevision); errors.Is(err, pgx.ErrNoRows) {
		return Relation{}, ErrNotFound
	} else if err != nil {
		return Relation{}, fmt.Errorf("读取结构关系失败: %w", err)
	}
	if err := lockDrawing(ctx, tx, drawingID); err != nil {
		return Relation{}, err
	}
	if err := tx.QueryRow(ctx, `SELECT relation_type, status, revision FROM drawing_part_relations WHERE id = $1::uuid FOR UPDATE`, id).
		Scan(&relationType, &status, &currentRevision); errors.Is(err, pgx.ErrNoRows) {
		return Relation{}, ErrNotFound
	} else if err != nil {
		return Relation{}, fmt.Errorf("锁定结构关系失败: %w", err)
	}
	if status != "active" {
		return Relation{}, ErrInvalidTransition
	}

	parentID := ""
	if input.ParentRelationID != nil {
		parentID = strings.TrimSpace(*input.ParentRelationID)
		if parentID != "" {
			var parentDrawing string
			err := tx.QueryRow(ctx, `SELECT drawing_id::text FROM drawing_part_relations WHERE id = $1::uuid AND status = 'active'`, parentID).Scan(&parentDrawing)
			if errors.Is(err, pgx.ErrNoRows) {
				return Relation{}, ErrNotFound
			}
			if err != nil {
				return Relation{}, fmt.Errorf("校验结构父关系失败: %w", err)
			}
			if parentDrawing != drawingID {
				return Relation{}, errors.New("父关系必须属于同一张图纸")
			}
			var cycle bool
			if err := tx.QueryRow(ctx, `WITH RECURSIVE ancestors(id) AS (
				SELECT $2::uuid
				UNION ALL
				SELECT r.parent_relation_id FROM drawing_part_relations r JOIN ancestors a ON r.id = a.id WHERE r.parent_relation_id IS NOT NULL
			) SELECT EXISTS (SELECT 1 FROM ancestors WHERE id = $1::uuid)`, id, parentID).Scan(&cycle); err != nil {
				return Relation{}, fmt.Errorf("检查结构环失败: %w", err)
			}
			if cycle {
				return Relation{}, errors.New("结构关系不能形成环")
			}
		}
	}
	var parentArg any
	if input.ParentRelationID != nil {
		parentArg = parentID
	}

	tag, err := tx.Exec(ctx, `UPDATE drawing_part_relations
			SET qty = COALESCE($2, qty), remark = COALESCE($3, remark), position = COALESCE($4, position),
				parent_relation_id = CASE WHEN $5::text IS NULL THEN parent_relation_id ELSE NULLIF($5::text, '')::uuid END,
				revision = revision + 1, updated_by = $6::uuid
			WHERE id = $1::uuid AND status = 'active' AND revision = $7`, id, input.Qty, input.Remark, input.Position, parentArg, userID, *input.ExpectedRevision)
	if err != nil {
		return Relation{}, fmt.Errorf("修改结构关系失败: %w", err)
	}
	if tag.RowsAffected() == 0 {
		if currentRevision < 1 {
			return Relation{}, ErrNotFound
		}
		return Relation{}, ErrRevisionConflict
	}
	if err := writeAuditTx(ctx, tx, userID, "update", "drawing_part_relation", id, "修改图纸结构关系"); err != nil {
		return Relation{}, err
	}
	relation, err := scanRelation(tx.QueryRow(ctx, relationSelect+` WHERE r.id = $1::uuid`, id))
	if err != nil {
		return Relation{}, fmt.Errorf("读取修改后的结构关系失败: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Relation{}, fmt.Errorf("提交结构关系修改失败: %w", err)
	}
	return relation, nil
}

func (repository *PGRepository) Borrow(ctx context.Context, drawingID string, input BorrowInput, userID string, idempotencyKey string) (Relation, error) {
	if strings.TrimSpace(input.SourcePartID) == "" {
		return Relation{}, errors.New("sourcePartId 不能为空")
	}
	if input.Qty <= 0 {
		input.Qty = 1
	}
	key := strings.TrimSpace(idempotencyKey)
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return Relation{}, fmt.Errorf("开始借用零件事务失败: %w", err)
	}
	defer tx.Rollback(ctx)

	type borrowIdempotencyPayload struct {
		Input    BorrowInput `json:"input"`
		Relation Relation    `json:"relation"`
	}

	scope := "borrow-drawing:" + drawingID
	if key != "" {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, scope+":"+key); err != nil {
			return Relation{}, fmt.Errorf("锁定借用幂等命令失败: %w", err)
		}
		var storedUser string
		var storedResult []byte
		err = tx.QueryRow(ctx, `SELECT user_id::text, result FROM domain_idempotency_records WHERE scope = $1 AND idempotency_key = $2`, scope, key).Scan(&storedUser, &storedResult)
		if err == nil {
			if storedUser != userID {
				return Relation{}, ErrIdempotencyConflict
			}
			var payload borrowIdempotencyPayload
			if err := json.Unmarshal(storedResult, &payload); err == nil && payload.Relation.ID != "" {
				if payload.Input.SourcePartID != input.SourcePartID ||
					payload.Input.Qty != input.Qty ||
					nullableString(payload.Input.ParentRelationID) != nullableString(input.ParentRelationID) ||
					!equalStringPtr(payload.Input.Position, input.Position) ||
					!equalIntPtr(payload.Input.LineNo, input.LineNo) ||
					payload.Input.Remark != input.Remark ||
					payload.Input.BorrowReason != input.BorrowReason {
					return Relation{}, ErrIdempotencyConflict
				}
				return payload.Relation, nil
			}
			// 兼容旧格式（纯 Relation JSON）
			var rel Relation
			if err := json.Unmarshal(storedResult, &rel); err != nil {
				return Relation{}, fmt.Errorf("读取借用幂等结果失败: %w", err)
			}
			if rel.PartID != input.SourcePartID ||
				rel.Quantity != input.Qty ||
				nullableString(rel.ParentRelationID) != nullableString(input.ParentRelationID) ||
				!equalStringPtr(rel.Position, input.Position) ||
				!equalIntPtr(rel.LineNo, input.LineNo) ||
				rel.Remark != input.Remark ||
				nullableString(rel.BorrowReason) != input.BorrowReason {
				return Relation{}, ErrIdempotencyConflict
			}
			return rel, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return Relation{}, fmt.Errorf("读取借用幂等记录失败: %w", err)
		}
	}

	if err := lockDrawing(ctx, tx, drawingID); err != nil {
		return Relation{}, err
	}
	var lifecycle string
	var publishedRevision *string
	if err := tx.QueryRow(ctx, `SELECT lifecycle_status, published_revision_id::text FROM parts WHERE id = $1::uuid FOR SHARE`, input.SourcePartID).Scan(&lifecycle, &publishedRevision); errors.Is(err, pgx.ErrNoRows) {
		return Relation{}, ErrNotFound
	} else if err != nil {
		return Relation{}, fmt.Errorf("读取借用零件失败: %w", err)
	}
	if lifecycle != "active" {
		return Relation{}, errors.New("已停用或归档的零件不能借用")
	}
		if publishedRevision == nil || *publishedRevision == "" {
			return Relation{}, errors.New("该零件尚未随图纸正式发布，不能借用")
		}
	if err := validateParentRelation(ctx, tx, drawingID, input.ParentRelationID); err != nil {
		return Relation{}, err
	}
	var relationID string
	err = tx.QueryRow(ctx, `INSERT INTO drawing_part_relations
		(drawing_id, part_id, parent_relation_id, relation_type, qty, position, line_no, remark, borrow_reason, borrowed_by, borrowed_at, created_by, updated_by)
		VALUES ($1::uuid, $2::uuid, NULLIF($3, '')::uuid, 'borrowed', $4, $5, $6, $7, NULLIF($8, ''), $9::uuid, now(), $9::uuid, $9::uuid)
		RETURNING id::text`, drawingID, input.SourcePartID, nullableString(input.ParentRelationID), input.Qty, input.Position, input.LineNo, input.Remark, input.BorrowReason, userID).Scan(&relationID)
	if err != nil {
		return Relation{}, fmt.Errorf("创建借用关系失败: %w", err)
	}
	if err := writeAuditTx(ctx, tx, userID, "borrow", "drawing_part_relation", relationID, "借用零件"); err != nil {
		return Relation{}, err
	}
	relation, err := scanRelation(tx.QueryRow(ctx, relationSelect+` WHERE r.id = $1::uuid`, relationID))
	if err != nil {
		return Relation{}, fmt.Errorf("读取借用关系失败: %w", err)
	}
	if key != "" {
		payload := borrowIdempotencyPayload{
			Input:    input,
			Relation: relation,
		}
		result, _ := json.Marshal(payload)
		if _, err := tx.Exec(ctx, `INSERT INTO domain_idempotency_records (scope, idempotency_key, user_id, result) VALUES ($1, $2, $3::uuid, $4::jsonb)`, scope, key, userID, result); err != nil {
			return Relation{}, fmt.Errorf("保存借用幂等结果失败: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Relation{}, fmt.Errorf("提交借用零件事务失败: %w", err)
	}
	return relation, nil
}

func (repository *PGRepository) ForkBorrowedPart(ctx context.Context, relationID string, input ForkInput, userID, idempotencyKey string) (Part, error) {
	newNo := strings.TrimSpace(input.NewPartNo)
	if newNo == "" {
		return Part{}, errors.New("新零件图号不能为空")
	}
	key := strings.TrimSpace(idempotencyKey)
	if key == "" {
		return Part{}, errors.New("缺少幂等键")
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return Part{}, fmt.Errorf("开始 Fork 零件事务失败: %w", err)
	}
	defer tx.Rollback(ctx)
	scope := "fork-relation:" + relationID
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, scope+":"+key); err != nil {
		return Part{}, fmt.Errorf("锁定 Fork 幂等命令失败: %w", err)
	}
	var storedUser string
	var storedResult []byte
	err = tx.QueryRow(ctx, `SELECT user_id::text, result FROM domain_idempotency_records WHERE scope = $1 AND idempotency_key = $2`, scope, key).Scan(&storedUser, &storedResult)
	if err == nil {
		if storedUser != userID {
			return Part{}, ErrIdempotencyConflict
		}
		var part Part
		if err := json.Unmarshal(storedResult, &part); err != nil {
			return Part{}, fmt.Errorf("读取 Fork 幂等结果失败: %w", err)
		}
		return part, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Part{}, fmt.Errorf("读取 Fork 幂等记录失败: %w", err)
	}

	var drawingID, sourcePartID, relationType, relationStatus string
	err = tx.QueryRow(ctx, `SELECT drawing_id::text, part_id::text, relation_type, status FROM drawing_part_relations WHERE id = $1::uuid`, relationID).
		Scan(&drawingID, &sourcePartID, &relationType, &relationStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return Part{}, ErrNotFound
	}
	if err != nil {
		return Part{}, fmt.Errorf("读取待 Fork 关系失败: %w", err)
	}
	if relationType != "borrowed" || relationStatus != "active" {
		return Part{}, errors.New("只有活动借用零件关系可以 Fork")
	}

	// 统一锁顺序：图纸 advisory & 行锁 -> 目标借用关系行锁 -> 源零件行锁
	if err := lockDrawing(ctx, tx, drawingID); err != nil {
		return Part{}, err
	}

	var parentID *string
	var qty float64
	var remark string
	var position *string
	var lineNo *int
	err = tx.QueryRow(ctx, `SELECT parent_relation_id::text, relation_type, status, qty, remark, position, line_no
		FROM drawing_part_relations
		WHERE id = $1::uuid FOR UPDATE`, relationID).
		Scan(&parentID, &relationType, &relationStatus, &qty, &remark, &position, &lineNo)
	if errors.Is(err, pgx.ErrNoRows) {
		return Part{}, ErrNotFound
	} else if err != nil {
		return Part{}, fmt.Errorf("锁定待 Fork 借用关系失败: %w", err)
	}
	if relationType != "borrowed" || relationStatus != "active" {
		return Part{}, errors.New("只有活动借用零件关系可以 Fork")
	}

	var sourcePartNo, sourceRevisionID, sourceName, material, spec, surfaceTreatment, partType, version string
	var weight float64
	err = tx.QueryRow(ctx, `SELECT p.part_no, pr.id::text, pr.name, pr.material, pr.spec, pr.weight, pr.surface_treatment, pr.part_type, pr.version
		FROM parts p
		JOIN part_revisions pr ON pr.id = p.published_revision_id AND pr.workflow_status = 'published'
		WHERE p.id = $1::uuid FOR UPDATE OF p`, sourcePartID).
		Scan(&sourcePartNo, &sourceRevisionID, &sourceName, &material, &spec, &weight, &surfaceTreatment, &partType, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return Part{}, errors.New("源零件不存在或没有 Published 版本")
	} else if err != nil {
		return Part{}, fmt.Errorf("锁定源零件与版本失败: %w", err)
	}
	if input.Name == "" {
		input.Name = sourceName
	}
	var newPartID string
	if err := tx.QueryRow(ctx, `INSERT INTO parts (part_no, normalized_part_no, forked_from_part_id, created_by, updated_by) VALUES ($1, $2, $3::uuid, $4::uuid, $4::uuid) RETURNING id::text`, newNo, NormalizePartNo(newNo), sourcePartID, userID).Scan(&newPartID); err != nil {
		if isUniqueViolation(err) {
			return Part{}, ErrConflict
		}
		return Part{}, fmt.Errorf("创建 Fork 零件失败: %w", err)
	}
	var newRevisionID string
	if err := tx.QueryRow(ctx, `INSERT INTO part_revisions (part_id, revision_no, version, name, material, spec, weight, surface_treatment, part_type, workflow_status, based_on_revision_id, created_by) VALUES ($1::uuid, 1, $2, $3, $4, $5, $6, $7, $8, 'draft', $9::uuid, $10::uuid) RETURNING id::text`, newPartID, version, input.Name, material, spec, weight, surfaceTreatment, partType, sourceRevisionID, userID).Scan(&newRevisionID); err != nil {
		return Part{}, fmt.Errorf("创建 Fork 零件版本失败: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO part_revision_attachments (part_revision_id, attachment_version_id, role, sort_order) SELECT $1::uuid, attachment_version_id, role, sort_order FROM part_revision_attachments WHERE part_revision_id = $2::uuid`, newRevisionID, sourceRevisionID); err != nil {
		return Part{}, fmt.Errorf("复制 Fork 零件附件关系失败: %w", err)
	}
	var newRelationID string
	if err := tx.QueryRow(ctx, `INSERT INTO drawing_part_relations (drawing_id, part_id, parent_relation_id, relation_type, qty, remark, position, line_no, forked_from_relation_id, created_by, updated_by) VALUES ($1::uuid, $2::uuid, $3::uuid, 'owned', $4, $5, $6, $7, $8::uuid, $9::uuid, $9::uuid) RETURNING id::text`, drawingID, newPartID, parentID, qty, remark, position, lineNo, relationID, userID).Scan(&newRelationID); err != nil {
		return Part{}, fmt.Errorf("创建 Fork 自有关系失败: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE drawing_part_relations SET parent_relation_id = $1::uuid, revision = revision + 1, updated_by = $2::uuid WHERE drawing_id = $3::uuid AND parent_relation_id = $4::uuid AND status = 'active'`, newRelationID, userID, drawingID, relationID); err != nil {
		return Part{}, fmt.Errorf("迁移 Fork 零件子关系失败: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE drawing_part_relations SET status = 'archived', revision = revision + 1, updated_by = $2::uuid WHERE id = $1::uuid AND status = 'active'`, relationID, userID); err != nil {
		return Part{}, fmt.Errorf("归档原借用关系失败: %w", err)
	}
	part := Part{ID: newPartID, RelationID: newRelationID, DrawingID: drawingID, No: newNo, Name: input.Name, ParentNo: "", Material: material, Spec: spec, Weight: weight, SurfaceTreatment: surfaceTreatment, ManufacturingType: partType, Quantity: qty, Status: StatusDraft, Version: version, CreatedBy: userID, Revision: 1, RelationRevision: 1, RelationType: "owned", LifecycleStatus: "active", CreatedAt: time.Now().UTC().Format(time.RFC3339), UpdatedAt: time.Now().UTC().Format(time.RFC3339)}
	result, _ := json.Marshal(part)
	if _, err := tx.Exec(ctx, `INSERT INTO domain_idempotency_records (scope, idempotency_key, user_id, result) VALUES ($1, $2, $3::uuid, $4::jsonb)`, scope, key, userID, result); err != nil {
		return Part{}, fmt.Errorf("保存 Fork 幂等结果失败: %w", err)
	}
	if err := writeAuditTx(ctx, tx, userID, "fork", "part", newPartID, "Fork 借用零件"); err != nil {
		return Part{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Part{}, fmt.Errorf("提交 Fork 零件事务失败: %w", err)
	}
	return repository.FindPart(ctx, newPartID)
}

func (repository *PGRepository) CreateDraftRevision(ctx context.Context, partID string, input CreateRevisionInput, userID string) (PartRevision, error) {
	if strings.TrimSpace(input.Name) == "" {
		return PartRevision{}, errors.New("零件版本名称不能为空")
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return PartRevision{}, fmt.Errorf("开始创建零件版本事务失败: %w", err)
	}
	defer tx.Rollback(ctx)
	var publishedID *string
	if err := tx.QueryRow(ctx, `SELECT published_revision_id::text FROM parts WHERE id = $1::uuid FOR UPDATE`, partID).Scan(&publishedID); errors.Is(err, pgx.ErrNoRows) {
		return PartRevision{}, ErrNotFound
	} else if err != nil {
		return PartRevision{}, fmt.Errorf("读取零件发布指针失败: %w", err)
	}
	version := strings.TrimSpace(input.Version)
	if version == "" {
		version = "v1.0"
	}
	var revisionID string
	err = tx.QueryRow(ctx, `INSERT INTO part_revisions (part_id, revision_no, version, name, material, spec, weight, surface_treatment, part_type, workflow_status, based_on_revision_id, created_by)
		VALUES ($1::uuid, (SELECT COALESCE(MAX(revision_no), 0) + 1 FROM part_revisions WHERE part_id = $1::uuid), $2, $3, COALESCE(NULLIF($4, ''), '—'), $5, $6, COALESCE($7, ''), COALESCE(NULLIF($8, ''), '自制件'), 'draft', NULLIF($9, '')::uuid, $10::uuid) RETURNING id::text`, partID, version, input.Name, input.Material, input.Spec, input.Weight, input.SurfaceTreatment, input.PartType, nullableString(publishedID), userID).Scan(&revisionID)
	if err != nil {
		if isUniqueViolation(err) {
			return PartRevision{}, ErrConflict
		}
		return PartRevision{}, fmt.Errorf("创建零件 Draft Revision 失败: %w", err)
	}
	if err := writeAuditTx(ctx, tx, userID, "create", "part_revision", revisionID, "创建零件 Draft Revision"); err != nil {
		return PartRevision{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PartRevision{}, fmt.Errorf("提交零件版本创建失败: %w", err)
	}
	return repository.GetRevision(ctx, revisionID)
}

func (repository *PGRepository) UpdateDraftRevision(ctx context.Context, revisionID string, input UpdateRevisionInput, userID string) (PartRevision, error) {
	if input.ExpectedRowRevision == nil || *input.ExpectedRowRevision < 1 {
		return PartRevision{}, ErrRevisionRequired
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return PartRevision{}, fmt.Errorf("开始修改零件版本事务失败: %w", err)
	}
	defer tx.Rollback(ctx)
	var status string
	if err := tx.QueryRow(ctx, `SELECT workflow_status FROM part_revisions WHERE id = $1::uuid FOR UPDATE`, revisionID).Scan(&status); errors.Is(err, pgx.ErrNoRows) {
		return PartRevision{}, ErrNotFound
	} else if err != nil {
		return PartRevision{}, fmt.Errorf("读取零件版本状态失败: %w", err)
	}
	if status != "draft" && status != "rejected" {
		return PartRevision{}, ErrInvalidTransition
	}
	tag, err := tx.Exec(ctx, `UPDATE part_revisions SET name = COALESCE($2, name), material = COALESCE($3, material), spec = COALESCE($4, spec), weight = COALESCE($5, weight), surface_treatment = COALESCE($6, surface_treatment), part_type = COALESCE($7, part_type), version = COALESCE($8, version), row_revision = row_revision + 1, workflow_status = 'draft' WHERE id = $1::uuid AND row_revision = $9`, revisionID, input.Name, input.Material, input.Spec, input.Weight, input.SurfaceTreatment, input.PartType, input.Version, *input.ExpectedRowRevision)
	if err != nil {
		return PartRevision{}, fmt.Errorf("修改零件 Draft Revision 失败: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return PartRevision{}, ErrRevisionConflict
	}
	if err := writeAuditTx(ctx, tx, userID, "update", "part_revision", revisionID, "修改零件 Draft Revision"); err != nil {
		return PartRevision{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PartRevision{}, fmt.Errorf("提交零件版本修改失败: %w", err)
	}
	return repository.GetRevision(ctx, revisionID)
}

func (repository *PGRepository) TransitionRevision(ctx context.Context, revisionID, action, userID string) (PartRevision, error) {
	action = strings.ToLower(strings.TrimSpace(action))
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return PartRevision{}, fmt.Errorf("开始零件版本流转事务失败: %w", err)
	}
	defer tx.Rollback(ctx)
	var partID, status string
	if err := tx.QueryRow(ctx, `SELECT part_id::text FROM part_revisions WHERE id = $1::uuid`, revisionID).Scan(&partID); errors.Is(err, pgx.ErrNoRows) {
		return PartRevision{}, ErrNotFound
	} else if err != nil {
		return PartRevision{}, err
	}
	if err := tx.QueryRow(ctx, `SELECT id::text FROM parts WHERE id = $1::uuid FOR UPDATE`, partID).Scan(&partID); err != nil {
		return PartRevision{}, err
	}
	if err := tx.QueryRow(ctx, `SELECT part_id::text, workflow_status FROM part_revisions WHERE id = $1::uuid FOR UPDATE`, revisionID).Scan(&partID, &status); errors.Is(err, pgx.ErrNoRows) {
		return PartRevision{}, ErrNotFound
	} else if err != nil {
		return PartRevision{}, fmt.Errorf("读取待流转零件版本失败: %w", err)
	}
	next := ""
	switch action {
	case "submit":
		if status != "draft" {
			return PartRevision{}, ErrInvalidTransition
		}
		next = "reviewing"
	case "reject":
		if status != "reviewing" {
			return PartRevision{}, ErrInvalidTransition
		}
		next = "rejected"
	case "publish":
		if status != "reviewing" {
			return PartRevision{}, ErrInvalidTransition
		}
		next = "published"
	default:
		return PartRevision{}, errors.New("零件版本流转动作无效")
	}
	if next == "published" {
		if _, err := tx.Exec(ctx, `UPDATE part_revisions SET workflow_status = 'published', published_by = $2::uuid, published_at = now() WHERE id = $1::uuid`, revisionID, userID); err != nil {
			return PartRevision{}, fmt.Errorf("发布零件版本失败: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE parts SET published_revision_id = $2::uuid, updated_by = $3::uuid WHERE id = $1::uuid`, partID, revisionID, userID); err != nil {
			return PartRevision{}, fmt.Errorf("推进零件发布指针失败: %w", err)
		}
	} else if _, err := tx.Exec(ctx, `UPDATE part_revisions SET workflow_status = $2 WHERE id = $1::uuid`, revisionID, next); err != nil {
		return PartRevision{}, fmt.Errorf("更新零件版本状态失败: %w", err)
	}
	if err := writeAuditTx(ctx, tx, userID, action, "part_revision", revisionID, "零件版本"+action); err != nil {
		return PartRevision{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PartRevision{}, fmt.Errorf("提交零件版本流转失败: %w", err)
	}
	return repository.GetRevision(ctx, revisionID)
}

func (repository *PGRepository) GetRevision(ctx context.Context, revisionID string) (PartRevision, error) {
	var item PartRevision
	var createdAt, publishedAt *time.Time
	err := repository.pool.QueryRow(ctx, `SELECT id::text, part_id::text, revision_no, version, row_revision, name, material, spec, weight, surface_treatment, part_type, workflow_status, based_on_revision_id::text, COALESCE(created_by::text, ''), created_at, COALESCE(published_by::text, ''), published_at FROM part_revisions WHERE id = $1::uuid`, revisionID).Scan(&item.ID, &item.PartID, &item.RevisionNo, &item.Version, &item.RowRevision, &item.Name, &item.Material, &item.Spec, &item.Weight, &item.SurfaceTreatment, &item.PartType, &item.WorkflowStatus, &item.BasedOnRevisionID, &item.CreatedBy, &createdAt, &item.PublishedBy, &publishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return PartRevision{}, ErrNotFound
	}
	if err != nil {
		return PartRevision{}, fmt.Errorf("查询零件版本失败: %w", err)
	}
	if createdAt != nil {
		item.CreatedAt = createdAt.Format(time.RFC3339)
	}
	if publishedAt != nil {
		item.PublishedAt = publishedAt.Format(time.RFC3339)
	}
	return item, nil
}

func (repository *PGRepository) GetBOM(ctx context.Context, drawingID string) (BOM, error) {
	var result BOM
	result.DrawingID = drawingID
	if err := repository.pool.QueryRow(ctx, `SELECT revision FROM drawing_boms WHERE drawing_id = $1::uuid`, drawingID).Scan(&result.Revision); errors.Is(err, pgx.ErrNoRows) {
		result.Revision = 1
		result.Items = make([]BOMItem, 0)
		return result, nil
	} else if err != nil {
		return BOM{}, fmt.Errorf("查询 BOM 版本失败: %w", err)
	}
	rows, err := repository.pool.Query(ctx, `SELECT id::text, COALESCE(NULLIF(item_code, ''), id::text), item_no, part_id::text, source_attachment_version_id::text, name, spec, quantity, weight, remark FROM bom_items WHERE bom_id = (SELECT id FROM drawing_boms WHERE drawing_id = $1::uuid) ORDER BY item_no`, drawingID)
	if err != nil {
		return BOM{}, fmt.Errorf("查询 BOM 明细失败: %w", err)
	}
	defer rows.Close()
	result.Items = make([]BOMItem, 0)
	for rows.Next() {
		var item BOMItem
		if err := rows.Scan(&item.RowID, &item.ID, &item.ItemNo, &item.PartID, &item.SourceAttachmentVersion, &item.Name, &item.Spec, &item.Quantity, &item.Weight, &item.Remark); err != nil {
			return BOM{}, err
		}
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		return BOM{}, err
	}
	return result, nil
}

func (repository *PGRepository) ReplaceBOM(ctx context.Context, drawingID string, input UpdateBOMInput, userID string) (BOM, error) {
	if input.ExpectedRevision == nil || *input.ExpectedRevision < 1 {
		return BOM{}, ErrRevisionRequired
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return BOM{}, fmt.Errorf("开始修改 BOM 事务失败: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockDrawing(ctx, tx, drawingID); err != nil {
		return BOM{}, err
	}
	var bomID string
	var revision int64
	err = tx.QueryRow(ctx, `SELECT id::text, revision FROM drawing_boms WHERE drawing_id = $1::uuid FOR UPDATE`, drawingID).Scan(&bomID, &revision)
	if errors.Is(err, pgx.ErrNoRows) {
		if *input.ExpectedRevision != 1 {
			return BOM{}, ErrRevisionConflict
		}
		if err := tx.QueryRow(ctx, `INSERT INTO drawing_boms (drawing_id, revision, updated_by) VALUES ($1::uuid, 1, $2::uuid) RETURNING id::text`, drawingID, userID).Scan(&bomID); err != nil {
			return BOM{}, fmt.Errorf("创建 BOM 失败: %w", err)
		}
		revision = 1
	} else if err != nil {
		return BOM{}, fmt.Errorf("读取 BOM 失败: %w", err)
	} else {
		if revision != *input.ExpectedRevision {
			return BOM{}, ErrRevisionConflict
		}
		if _, err := tx.Exec(ctx, `UPDATE drawing_boms SET revision = revision + 1, updated_by = $2::uuid, updated_at = now() WHERE id = $1::uuid`, bomID, userID); err != nil {
			return BOM{}, fmt.Errorf("更新 BOM 版本失败: %w", err)
		}
		revision++
		if _, err := tx.Exec(ctx, `DELETE FROM bom_items WHERE bom_id = $1::uuid`, bomID); err != nil {
			return BOM{}, fmt.Errorf("清理 BOM 明细失败: %w", err)
		}
	}
	for index, item := range input.Items {
		itemNo := item.ItemNo
		if itemNo < 1 {
			itemNo = index + 1
		}
		if strings.TrimSpace(item.Name) == "" {
			return BOM{}, errors.New("BOM 明细名称不能为空")
		}
		if _, err := tx.Exec(ctx, `INSERT INTO bom_items (bom_id, item_no, item_code, part_id, source_attachment_version_id, name, spec, quantity, weight, remark) VALUES ($1::uuid, $2, $3, NULLIF($4, '')::uuid, NULLIF($5, '')::uuid, $6, COALESCE($7, '—'), $8, $9, COALESCE($10, ''))`, bomID, itemNo, strings.TrimSpace(item.ID), nullableString(item.PartID), nullableString(item.SourceAttachmentVersion), item.Name, item.Spec, item.Quantity, item.Weight, item.Remark); err != nil {
			return BOM{}, fmt.Errorf("保存 BOM 明细失败: %w", err)
		}
	}
	if err := writeAuditTx(ctx, tx, userID, "update", "drawing_bom", drawingID, "修改图纸 BOM"); err != nil {
		return BOM{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return BOM{}, fmt.Errorf("提交 BOM 修改失败: %w", err)
	}
	return repository.GetBOM(ctx, drawingID)
}

func equalStringPtr(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func equalIntPtr(a, b *int) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func lockDrawing(ctx context.Context, tx pgx.Tx, drawingID string) error {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, drawingID); err != nil {
		return fmt.Errorf("锁定图纸结构失败: %w", err)
	}
	var status string
	err := tx.QueryRow(ctx, `SELECT status FROM drawings WHERE id = $1::uuid FOR UPDATE`, drawingID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("校验图纸失败: %w", err)
	}
		if status == string(StatusArchived) {
			return ErrArchivedLocked
		}
	return nil
}

func validateParentRelation(ctx context.Context, tx pgx.Tx, drawingID string, parentID *string) error {
	if parentID == nil || strings.TrimSpace(*parentID) == "" {
		return nil
	}
	var parentDrawing string
	if err := tx.QueryRow(ctx, `SELECT drawing_id::text FROM drawing_part_relations WHERE id = $1::uuid AND status = 'active'`, *parentID).Scan(&parentDrawing); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return fmt.Errorf("校验父关系失败: %w", err)
	}
	if parentDrawing != drawingID {
		return errors.New("父关系必须属于同一张图纸")
	}
	return nil
}

const relationSelect = `SELECT r.id::text, r.drawing_id::text, r.part_id::text, r.parent_relation_id::text, r.relation_type, r.qty, r.position, r.line_no, r.remark, r.borrow_reason, r.revision, r.status FROM drawing_part_relations r`

func scanRelation(row rowScanner) (Relation, error) {
	var item Relation
	return item, row.Scan(&item.ID, &item.DrawingID, &item.PartID, &item.ParentRelationID, &item.RelationType, &item.Quantity, &item.Position, &item.LineNo, &item.Remark, &item.BorrowReason, &item.Revision, &item.Status)
}

func writeAuditTx(ctx context.Context, tx pgx.Tx, userID, action, resourceType, resourceID, summary string) error {
	if _, err := tx.Exec(ctx, `INSERT INTO audit_logs (actor_id, action, resource_type, resource_id, summary) VALUES ($1::uuid, $2, $3, $4::uuid, $5)`, userID, action, resourceType, resourceID, summary); err != nil {
		return fmt.Errorf("写入操作审计失败: %w", err)
	}
	return nil
}
