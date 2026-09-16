package drawing

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound            = errors.New("drawing resource not found")
	ErrConflict            = errors.New("drawing resource conflict")
	ErrRevisionConflict    = errors.New("drawing resource revision conflict")
	ErrRevisionRequired    = errors.New("drawing resource revision required")
	ErrInvalidTransition   = errors.New("drawing status transition not allowed")
	ErrIdempotencyConflict = errors.New("idempotency key conflict")
	ErrArchivedLocked      = errors.New("archived drawing requires an approved change request to modify")
)

type PGRepository struct {
	pool *pgxpool.Pool
}

func NewPGRepository(pool *pgxpool.Pool) *PGRepository {
	return &PGRepository{pool: pool}
}

func (repository *PGRepository) List(ctx context.Context, filter ListFilter) (Page[Drawing], error) {
	keyword := "%" + strings.ToLower(strings.TrimSpace(filter.Keyword)) + "%"
	vendor := "%" + strings.ToLower(strings.TrimSpace(filter.Vendor)) + "%"
	where := `WHERE ($1 = '%' OR lower(d.drawing_no) LIKE $1 OR lower(d.name) LIKE $1 OR lower(d.project) LIKE $1)
		AND ($2 = '' OR d.status = $2)
		AND ($3 = '%' OR lower(d.vendor) LIKE $3)`
	var total int
	if err := repository.pool.QueryRow(ctx, `SELECT count(*) FROM drawings d `+where, keyword, string(filter.Status), vendor).Scan(&total); err != nil {
		return Page[Drawing]{}, fmt.Errorf("统计图纸失败: %w", err)
	}
	offset := (filter.Page - 1) * filter.PageSize
	rows, err := repository.pool.Query(ctx, `
		SELECT d.id::text, d.drawing_no, d.name, d.kind, d.project, d.material, d.vendor, d.status, d.version, d.revision,
		       d.borrow_from, d.remark,
		       COALESCE(created_user.display_name, created_user.account, ''), d.created_at,
		       COALESCE(updated_user.display_name, updated_user.account, created_user.display_name, created_user.account, ''), d.updated_at,
		       COALESCE(d.created_by::text, '')
		FROM drawings d
		LEFT JOIN users created_user ON created_user.id = d.created_by
		LEFT JOIN users updated_user ON updated_user.id = d.updated_by `+where+`
		ORDER BY d.updated_at DESC, d.drawing_no
		LIMIT $4 OFFSET $5`, keyword, string(filter.Status), vendor, filter.PageSize, offset)
	if err != nil {
		return Page[Drawing]{}, fmt.Errorf("查询图纸失败: %w", err)
	}
	defer rows.Close()
	items := make([]Drawing, 0)
	for rows.Next() {
		item, err := scanDrawing(rows)
		if err != nil {
			return Page[Drawing]{}, err
		}
		item.Signers, err = repository.loadDrawingSigners(ctx, item.ID)
		if err != nil {
			return Page[Drawing]{}, err
		}
		item.AttributeValues, err = repository.loadDrawingAttributeValues(ctx, item.ID)
		if err != nil {
			return Page[Drawing]{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return Page[Drawing]{}, fmt.Errorf("读取图纸失败: %w", err)
	}
	if err := repository.attachAssignees(ctx, items); err != nil {
		return Page[Drawing]{}, err
	}
	return Page[Drawing]{List: items, Total: total, Page: filter.Page, PageSize: filter.PageSize}, nil
}

// attachAssignees 为一批图纸补上当前负责人（一次查询，避免逐行往返）。
func (repository *PGRepository) attachAssignees(ctx context.Context, items []Drawing) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		if item.ID != "" {
			ids = append(ids, item.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	assignees, err := repository.loadAssignees(ctx, ids)
	if err != nil {
		return err
	}
	for index := range items {
		if list, ok := assignees[items[index].ID]; ok {
			items[index].Assignees = list
		}
	}
	return nil
}

// loadAssignees 按图纸 ID 批量读取有效负责人（含姓名）。
func (repository *PGRepository) loadAssignees(ctx context.Context, drawingIDs []string) (map[string][]Assignee, error) {
	rows, err := repository.pool.Query(ctx, `
		SELECT t.drawing_id::text, t.assignee_id::text, COALESCE(u.display_name, u.account, '未命名账号')
		FROM drawing_tasks t
		LEFT JOIN users u ON u.id = t.assignee_id
		WHERE t.status = 'active' AND t.drawing_id = ANY($1::uuid[])
		ORDER BY t.created_at`, drawingIDs)
	if err != nil {
		return nil, fmt.Errorf("读取图纸负责人失败: %w", err)
	}
	defer rows.Close()
	result := make(map[string][]Assignee, len(drawingIDs))
	for rows.Next() {
		var drawingID string
		var assignee Assignee
		if err := rows.Scan(&drawingID, &assignee.UserID, &assignee.Name); err != nil {
			return nil, fmt.Errorf("解析图纸负责人失败: %w", err)
		}
		result[drawingID] = append(result[drawingID], assignee)
	}
	return result, rows.Err()
}

func (repository *PGRepository) Find(ctx context.Context, id string) (Drawing, error) {
	return repository.find(ctx, `WHERE d.id = $1::uuid`, id)
}

func (repository *PGRepository) FindByNo(ctx context.Context, no string) (Drawing, error) {
	return repository.find(ctx, `WHERE d.drawing_no = $1`, no)
}

func (repository *PGRepository) find(ctx context.Context, condition string, argument string) (Drawing, error) {
	row := repository.pool.QueryRow(ctx, `
		SELECT d.id::text, d.drawing_no, d.name, d.kind, d.project, d.material, d.vendor, d.status,
		       d.version, d.revision, d.borrow_from, d.remark,
		       COALESCE(created_user.display_name, created_user.account, ''), d.created_at,
		       COALESCE(updated_user.display_name, updated_user.account, created_user.display_name, created_user.account, ''), d.updated_at,
		       COALESCE(d.created_by::text, '')
		FROM drawings d
		LEFT JOIN users created_user ON created_user.id = d.created_by
		LEFT JOIN users updated_user ON updated_user.id = d.updated_by `+condition, argument)
	item, err := scanDrawing(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Drawing{}, ErrNotFound
	}
	if err != nil {
		return Drawing{}, fmt.Errorf("查询图纸详情失败: %w", err)
	}
	item.Signers, err = repository.loadDrawingSigners(ctx, item.ID)
	if err != nil {
		return Drawing{}, err
	}
	item.AttributeValues, err = repository.loadDrawingAttributeValues(ctx, item.ID)
	if err != nil {
		return Drawing{}, err
	}
	assignees, err := repository.loadAssignees(ctx, []string{item.ID})
	if err != nil {
		return Drawing{}, err
	}
	item.Assignees = assignees[item.ID]
	return item, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanDrawing(row rowScanner) (Drawing, error) {
	var item Drawing
	var status string
	var createdAt time.Time
	var updatedAt time.Time
	if err := row.Scan(&item.ID, &item.No, &item.Name, &item.Kind, &item.Project, &item.Material, &item.Vendor, &status, &item.Version, &item.Revision, &item.BorrowFrom, &item.Remark, &item.CreatedBy, &createdAt, &item.UpdatedBy, &updatedAt, &item.CreatedByID); err != nil {
		return Drawing{}, err
	}
	item.Status = Status(status)
	item.CreatedAt = createdAt.Format(time.RFC3339)
	item.By = item.UpdatedBy
	if item.By == "" {
		item.By = item.CreatedBy
	}
	item.UpdatedAt = updatedAt.Format(time.RFC3339)
	item.Updated = updatedAt.Format(time.RFC3339)
	return item, nil
}

func (repository *PGRepository) Create(ctx context.Context, input CreateDrawingInput, userID string) (Drawing, error) {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return Drawing{}, fmt.Errorf("开始创建图纸事务失败: %w", err)
	}
	defer tx.Rollback(ctx)

	status := input.Status
	if status == "" {
		status = StatusDraft
	}
	version := input.Version
	if version == "" {
		version = "v1.0"
	}
	material := input.Material
	if material == "" {
		material = "—"
	}
	kind := input.Kind
	if kind == "" {
		kind = "总图"
	}
	var id string
	err = tx.QueryRow(ctx, `
		INSERT INTO drawings (drawing_no, name, project, kind, material, vendor, status, version, borrow_from, remark, created_by, updated_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::uuid, $11::uuid)
		RETURNING id::text`, input.No, input.Name, input.Project, kind, material, input.Vendor, status, version, input.BorrowFrom, input.Remark, userID).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			return Drawing{}, ErrConflict
		}
		return Drawing{}, fmt.Errorf("创建图纸失败: %w", err)
	}
	if err := repository.saveSigners(ctx, tx, id, input.Signers); err != nil {
		return Drawing{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Drawing{}, fmt.Errorf("提交创建图纸事务失败: %w", err)
	}
	return repository.Find(ctx, id)
}

func (repository *PGRepository) Update(ctx context.Context, id string, input UpdateDrawingInput, userID string) (Drawing, error) {
	if input.ExpectedRevision == nil || *input.ExpectedRevision < 1 {
		return Drawing{}, ErrRevisionRequired
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return Drawing{}, fmt.Errorf("开始修改图纸事务失败: %w", err)
	}
	defer tx.Rollback(ctx)
	// 存档图纸受变更工单保护：其技术属性/版本/状态等正式成果只能经变更工单验收后由服务端应用，
	// 禁止通过普通 PATCH 直接修改，避免绕过工单。
	var currentStatus Status
	if err := tx.QueryRow(ctx, `SELECT status FROM drawings WHERE id = $1::uuid FOR UPDATE`, id).Scan(&currentStatus); errors.Is(err, pgx.ErrNoRows) {
		return Drawing{}, ErrNotFound
	} else if err != nil {
		return Drawing{}, fmt.Errorf("读取图纸状态失败: %w", err)
	}
	if currentStatus == StatusArchived {
		return Drawing{}, ErrArchivedLocked
	}
	if input.Status != nil && *input.Status != currentStatus && (*input.Status == StatusPublished || *input.Status == StatusArchived) {
		return Drawing{}, ErrInvalidTransition
	}
	tag, err := tx.Exec(ctx, `
		UPDATE drawings
		SET name = COALESCE($2, name), kind = COALESCE($3, kind), project = COALESCE($4, project), material = COALESCE($5, material),
		    vendor = COALESCE($6, vendor), status = COALESCE($7, status), version = COALESCE($8, version),
		    borrow_from = COALESCE($9, borrow_from), remark = COALESCE($10, remark), updated_by = $11::uuid,
		    revision = revision + 1
		WHERE id = $1::uuid AND revision = $12`, id, input.Name, input.Kind, input.Project, input.Material, input.Vendor, input.Status, input.Version, input.BorrowFrom, input.Remark, userID, *input.ExpectedRevision)
	if err != nil {
		if isUniqueViolation(err) {
			return Drawing{}, ErrConflict
		}
		return Drawing{}, fmt.Errorf("修改图纸失败: %w", err)
	}
	if tag.RowsAffected() == 0 {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM drawings WHERE id = $1::uuid)`, id).Scan(&exists); err != nil {
			return Drawing{}, fmt.Errorf("检查图纸版本失败: %w", err)
		}
		if !exists {
			return Drawing{}, ErrNotFound
		}
		return Drawing{}, ErrRevisionConflict
	}
	if input.AttributeValues != nil {
		if err := repository.replaceDrawingAttributeValues(ctx, tx, id, input.AttributeValues); err != nil {
			return Drawing{}, err
		}
	}
	if input.Signers != nil {
		if _, err := tx.Exec(ctx, `DELETE FROM drawing_signers WHERE drawing_id = $1::uuid`, id); err != nil {
			return Drawing{}, fmt.Errorf("清理图纸签署人失败: %w", err)
		}
		if err := repository.saveSigners(ctx, tx, id, input.Signers); err != nil {
			return Drawing{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Drawing{}, fmt.Errorf("提交图纸修改事务失败: %w", err)
	}
	item, err := repository.Find(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return Drawing{}, ErrNotFound
	}
	return item, err
}

// SetStatusByNo 受控状态流转：仅当图纸当前状态等于 from 时更新为 to，否则返回 ErrInvalidTransition。
func (repository *PGRepository) SetStatusByNo(ctx context.Context, no string, from, to Status, userID string) (Drawing, error) {
	tag, err := repository.pool.Exec(ctx, `
		UPDATE drawings
		SET status = $2, updated_by = $3::uuid, updated_at = now(), revision = revision + 1
		WHERE drawing_no = $1 AND status = $4`, no, to, userID, from)
	if err != nil {
		return Drawing{}, fmt.Errorf("更新图纸状态失败: %w", err)
	}
	if tag.RowsAffected() == 0 {
		if _, findErr := repository.FindByNo(ctx, no); findErr != nil {
			return Drawing{}, ErrNotFound
		}
		return Drawing{}, ErrInvalidTransition
	}
	return repository.FindByNo(ctx, no)
}

func (repository *PGRepository) ListParts(ctx context.Context, drawingID string) ([]Part, error) {
	rows, err := repository.pool.Query(ctx, `
			SELECT p.id::text, r.id::text, r.drawing_id::text, p.part_no,
			       COALESCE(pr.name, p.part_no), COALESCE(parent_part.part_no, d.drawing_no, ''), d.project,
			       COALESCE(pr.material, '—'), COALESCE(pr.spec, ''), COALESCE(pr.weight, 0),
			       COALESCE(pr.surface_treatment, ''), COALESCE(pr.part_type, '自制件'),
			       r.qty, COALESCE(pr.workflow_status, p.lifecycle_status), COALESCE(pr.version, 'v1.0'),
			       COALESCE(pr.row_revision, 1), r.revision, r.relation_type, COALESCE(NULLIF(r.source_drawing_no, ''), source_drawing.drawing_no, ''), p.lifecycle_status,
			       COALESCE(NULLIF(created_user.display_name, ''), created_user.account, ''), COALESCE(pr.created_at, p.created_at),
			       COALESCE(pr.published_by::text, p.updated_by::text, ''), COALESCE(pr.published_at, p.updated_at)
		FROM drawing_part_relations r
		JOIN parts p ON p.id = r.part_id
		JOIN drawings d ON d.id = r.drawing_id
			LEFT JOIN drawing_part_relations parent_rel ON parent_rel.id = r.parent_relation_id
			LEFT JOIN parts parent_part ON parent_part.id = parent_rel.part_id
			LEFT JOIN LATERAL (SELECT source_drawing.drawing_no FROM drawing_part_relations source_rel JOIN drawings source_drawing ON source_drawing.id = source_rel.drawing_id WHERE source_rel.part_id = p.id AND source_rel.relation_type = 'owned' AND source_rel.status = 'active' AND source_rel.drawing_id <> r.drawing_id ORDER BY source_rel.created_at LIMIT 1) source_drawing ON true
		LEFT JOIN part_revisions pr ON pr.id = COALESCE(p.published_revision_id, (SELECT latest.id FROM part_revisions latest WHERE latest.part_id = p.id ORDER BY latest.revision_no DESC LIMIT 1))
		LEFT JOIN users created_user ON created_user.id = COALESCE(pr.created_by, p.created_by)
		WHERE r.drawing_id = $1::uuid AND r.status = 'active'
		ORDER BY p.part_no, r.created_at`, drawingID)
	if err != nil {
		return nil, fmt.Errorf("查询结构树失败: %w", err)
	}
	defer rows.Close()
	parts := make([]Part, 0)
	for rows.Next() {
		part, err := scanFinalPart(rows)
		if err != nil {
			return nil, err
		}
		part.Signers, err = repository.loadPartSigners(ctx, part.ID)
		if err != nil {
			return nil, err
		}
		parts = append(parts, part)
	}
	return parts, rows.Err()
}

func (repository *PGRepository) FindPartWithRelation(ctx context.Context, id string, relationID string) (Part, error) {
	part, err := findPartSnapshot(ctx, repository.pool, id, relationID, "")
	if err != nil {
		return Part{}, err
	}
	part.Signers, err = repository.loadPartSigners(ctx, part.ID)
	return part, err
}

func findPartSnapshot(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, id, relationID, revisionID string) (Part, error) {
	query := `
			SELECT p.id::text, r.id::text, r.drawing_id::text, p.part_no,
		       COALESCE(pr.name, p.part_no), COALESCE(parent_part.part_no, d.drawing_no, ''), d.project,
		       COALESCE(pr.material, '—'), COALESCE(pr.spec, ''), COALESCE(pr.weight, 0),
		       COALESCE(pr.surface_treatment, ''), COALESCE(pr.part_type, '自制件'),
		       r.qty, COALESCE(pr.workflow_status, p.lifecycle_status), COALESCE(pr.version, 'v1.0'),
			       COALESCE(pr.row_revision, 1), r.revision, r.relation_type, COALESCE(NULLIF(r.source_drawing_no, ''), source_drawing.drawing_no, ''), p.lifecycle_status,
			       COALESCE(NULLIF(created_user.display_name, ''), created_user.account, ''), COALESCE(pr.created_at, p.created_at),
			       COALESCE(pr.published_by::text, p.updated_by::text, ''), COALESCE(pr.published_at, p.updated_at)
		FROM parts p
		JOIN drawing_part_relations r ON r.part_id = p.id AND r.status = 'active'
		JOIN drawings d ON d.id = r.drawing_id
			LEFT JOIN drawing_part_relations parent_rel ON parent_rel.id = r.parent_relation_id
			LEFT JOIN parts parent_part ON parent_part.id = parent_rel.part_id
		LEFT JOIN LATERAL (SELECT source_drawing.drawing_no FROM drawing_part_relations source_rel JOIN drawings source_drawing ON source_drawing.id = source_rel.drawing_id WHERE source_rel.part_id = p.id AND source_rel.relation_type = 'owned' AND source_rel.status = 'active' AND source_rel.drawing_id <> r.drawing_id ORDER BY source_rel.created_at LIMIT 1) source_drawing ON true
		LEFT JOIN part_revisions pr ON pr.part_id = p.id AND pr.id = COALESCE(NULLIF($2, '')::uuid, p.published_revision_id, (SELECT latest.id FROM part_revisions latest WHERE latest.part_id = p.id ORDER BY latest.revision_no DESC LIMIT 1))
		LEFT JOIN users created_user ON created_user.id = COALESCE(pr.created_by, p.created_by)
		WHERE p.id = $1::uuid`
	var row pgx.Row
	if strings.TrimSpace(relationID) != "" {
		row = db.QueryRow(ctx, query+` AND r.id = $3::uuid LIMIT 1`, id, revisionID, relationID)
	} else {
		row = db.QueryRow(ctx, query+` ORDER BY CASE WHEN r.relation_type = 'owned' THEN 0 ELSE 1 END, r.created_at DESC LIMIT 1`, id, revisionID)
	}
	part, err := scanFinalPart(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Part{}, ErrNotFound
	}
	if err != nil {
		return Part{}, fmt.Errorf("查询零件详情失败: %w", err)
	}
	return part, err
}

func (repository *PGRepository) FindPart(ctx context.Context, id string) (Part, error) {
	return repository.FindPartWithRelation(ctx, id, "")
}

func scanFinalPart(row rowScanner) (Part, error) {
	var part Part
	var status, relationType, borrowFrom, lifecycle string
	var createdAt, updatedAt time.Time
	if err := row.Scan(&part.ID, &part.RelationID, &part.DrawingID, &part.No, &part.Name, &part.ParentNo,
		&part.Project, &part.Material, &part.Spec, &part.Weight, &part.SurfaceTreatment,
		&part.ManufacturingType, &part.Quantity, &status, &part.Version, &part.Revision,
		&part.RelationRevision, &relationType, &borrowFrom, &lifecycle, &part.CreatedBy, &createdAt, &part.UpdatedBy, &updatedAt); err != nil {
		return Part{}, err
	}
	part.Status = Status(status)
	part.RelationType = relationType
	if borrowFrom != "" {
		part.BorrowFrom = &borrowFrom
	}
	part.LifecycleStatus = lifecycle
	part.CreatedAt = createdAt.Format(time.RFC3339)
	part.UpdatedAt = updatedAt.Format(time.RFC3339)
	return part, nil
}

func scanPart(row rowScanner) (Part, error) {
	var part Part
	var status string
	var createdAt time.Time
	var updatedAt time.Time
	if err := row.Scan(&part.ID, &part.DrawingID, &part.No, &part.Name, &part.ParentNo, &part.Project, &part.Material, &part.Spec, &part.Weight, &part.SurfaceTreatment, &part.ManufacturingType, &part.Quantity, &status, &part.Version, &part.Revision, &part.Vendor, &part.BorrowFrom, &part.Remark, &part.CreatedBy, &createdAt, &part.UpdatedBy, &updatedAt); err != nil {
		return Part{}, err
	}
	part.Status = Status(status)
	part.CreatedAt = createdAt.Format(time.RFC3339)
	part.UpdatedAt = updatedAt.Format(time.RFC3339)
	return part, nil
}

func (repository *PGRepository) CreatePart(ctx context.Context, drawingID string, input CreatePartInput, userID string) (Part, error) {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return Part{}, fmt.Errorf("开始创建零件事务失败: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, drawingID); err != nil {
		return Part{}, fmt.Errorf("锁定图纸结构失败: %w", err)
	}
	var drawingStatus string
	if err := tx.QueryRow(ctx, `SELECT status FROM drawings WHERE id = $1::uuid FOR UPDATE`, drawingID).Scan(&drawingStatus); errors.Is(err, pgx.ErrNoRows) {
		return Part{}, ErrNotFound
	} else if err != nil {
		return Part{}, fmt.Errorf("校验图纸状态失败: %w", err)
	}
	if drawingStatus == string(StatusArchived) {
		return Part{}, ErrArchivedLocked
	}
	var parentID *string
	if strings.TrimSpace(input.ParentNo) != "" {
		var value string
		err := tx.QueryRow(ctx, `SELECT r.id::text FROM drawing_part_relations r JOIN parts p ON p.id = r.part_id WHERE r.drawing_id = $1::uuid AND p.normalized_part_no = $2 AND r.status = 'active' LIMIT 1`, drawingID, NormalizePartNo(input.ParentNo)).Scan(&value)
		if errors.Is(err, pgx.ErrNoRows) {
			return Part{}, ErrNotFound
		}
		if err != nil {
			return Part{}, fmt.Errorf("查询零件父级失败: %w", err)
		}
		parentID = &value
	}
	status := input.Status
	if status == "" {
		status = StatusDraft
	}
	version := input.Version
	if version == "" {
		version = "v1.0"
	}
	partType := input.ManufacturingType
	if partType == "" {
		partType = "自制件"
	}
	quantity := input.Quantity
	if quantity == 0 {
		quantity = 1
	}
	var partID string
	partNo := strings.TrimSpace(input.No)
	err = tx.QueryRow(ctx, `INSERT INTO parts (part_no, normalized_part_no, created_by, updated_by) VALUES ($1, $2, $3::uuid, $3::uuid) RETURNING id::text`, partNo, NormalizePartNo(partNo), userID).Scan(&partID)
	if err != nil {
		if isUniqueViolation(err) {
			return Part{}, ErrConflict
		}
		return Part{}, fmt.Errorf("创建零件失败: %w", err)
	}
	var revisionID string
	err = tx.QueryRow(ctx, `INSERT INTO part_revisions (part_id, revision_no, version, name, material, spec, weight, surface_treatment, part_type, workflow_status, created_by) VALUES ($1::uuid, 1, $2, $3, COALESCE(NULLIF($4, ''), '—'), $5, $6, COALESCE(NULLIF($7, ''), ''), COALESCE(NULLIF($8, ''), '自制件'), $9, $10::uuid) RETURNING id::text`, partID, version, input.Name, input.Material, input.Spec, input.Weight, input.SurfaceTreatment, partType, status, userID).Scan(&revisionID)
	if err != nil {
		return Part{}, fmt.Errorf("创建零件版本失败: %w", err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO drawing_part_relations (drawing_id, part_id, parent_relation_id, relation_type, qty, remark, created_by, updated_by) VALUES ($1::uuid, $2::uuid, NULLIF($3, '')::uuid, 'owned', $4, COALESCE($5, ''), $6::uuid, $6::uuid)`, drawingID, partID, nullableString(parentID), quantity, input.Remark, userID); err != nil {
		return Part{}, fmt.Errorf("创建零件结构关系失败: %w", err)
	}
	if _, err = tx.Exec(ctx, `UPDATE parts SET published_revision_id = CASE WHEN $2 = 'published' THEN $1::uuid ELSE NULL END WHERE id = $3::uuid`, revisionID, status, partID); err != nil {
		return Part{}, fmt.Errorf("设置零件发布指针失败: %w", err)
	}
	if err := savePartSigners(ctx, tx, revisionID, input.Signers); err != nil {
		return Part{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Part{}, fmt.Errorf("提交创建零件事务失败: %w", err)
	}
	return repository.FindPart(ctx, partID)
}

func (repository *PGRepository) UpdatePart(ctx context.Context, id string, input UpdatePartInput, userID string) (Part, error) {
	if input.ExpectedRevision == nil || *input.ExpectedRevision < 1 {
		return Part{}, ErrRevisionRequired
	}
	changesRelation := input.Quantity != nil || input.Remark != nil
	if changesRelation && (input.ExpectedRelationRevision == nil || *input.ExpectedRelationRevision < 1) {
		return Part{}, ErrRevisionRequired
	}
	if changesRelation && strings.TrimSpace(nullableString(input.RelationID)) == "" {
		return Part{}, errors.New("修改结构关系必须指定 relationId")
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return Part{}, fmt.Errorf("开始修改零件事务失败: %w", err)
	}
	defer tx.Rollback(ctx)

	targetRelID := strings.TrimSpace(nullableString(input.RelationID))
	if changesRelation || targetRelID != "" {
		if input.RelationID == nil || strings.TrimSpace(*input.RelationID) == "" {
			return Part{}, errors.New("修改零件所属图纸的数量或备注时，必须指定明确的 relationId")
		}
		targetRelID = strings.TrimSpace(*input.RelationID)
		var targetDrawingID, relStatus string
		err := tx.QueryRow(ctx, `SELECT drawing_id::text, status FROM drawing_part_relations WHERE id = $1::uuid AND part_id = $2::uuid`, targetRelID, id).Scan(&targetDrawingID, &relStatus)
		if errors.Is(err, pgx.ErrNoRows) {
			return Part{}, errors.New("指定的结构关系不存在或不属于该零件")
		} else if err != nil {
			return Part{}, fmt.Errorf("读取结构关系失败: %w", err)
		}
		if relStatus != "active" {
			return Part{}, errors.New("已归档的结构关系禁止修改")
		}

		// 统一锁顺序：先锁定图纸与目标关系，再锁定零件
		if err := lockDrawing(ctx, tx, targetDrawingID); err != nil {
			return Part{}, err
		}

		err = tx.QueryRow(ctx, `SELECT status FROM drawing_part_relations WHERE id = $1::uuid FOR UPDATE`, targetRelID).Scan(&relStatus)
		if errors.Is(err, pgx.ErrNoRows) {
			return Part{}, errors.New("指定的结构关系不存在")
		} else if err != nil {
			return Part{}, fmt.Errorf("锁定结构关系失败: %w", err)
		}
		if relStatus != "active" {
			return Part{}, errors.New("已归档的结构关系禁止修改")
		}
	}

	var partNo string
	if err := tx.QueryRow(ctx, `SELECT part_no FROM parts WHERE id = $1::uuid FOR UPDATE`, id).Scan(&partNo); errors.Is(err, pgx.ErrNoRows) {
		return Part{}, ErrNotFound
	} else if err != nil {
		return Part{}, fmt.Errorf("读取零件失败: %w", err)
	}
	var revisionID, workflow string
	if err := tx.QueryRow(ctx, `SELECT id::text, workflow_status FROM part_revisions WHERE part_id = $1::uuid ORDER BY revision_no DESC LIMIT 1 FOR UPDATE`, id).Scan(&revisionID, &workflow); err != nil {
		return Part{}, fmt.Errorf("读取零件版本失败: %w", err)
	}
	if workflow == "published" || workflow == "reviewing" {
		return Part{}, ErrInvalidTransition
	}
	var newNo string
	if input.No != nil {
		newNo = strings.TrimSpace(*input.No)
		if newNo == "" {
			return Part{}, ErrConflict
		}
	}
	if newNo != "" {
		if _, err := tx.Exec(ctx, `UPDATE parts SET part_no = $2, normalized_part_no = $3, updated_by = $4::uuid WHERE id = $1::uuid`, id, newNo, NormalizePartNo(newNo), userID); err != nil {
			if isUniqueViolation(err) {
				return Part{}, ErrConflict
			}
			return Part{}, fmt.Errorf("更新零件图号失败: %w", err)
		}
	}
	tag, err := tx.Exec(ctx, `UPDATE part_revisions SET name = COALESCE($2, name), material = COALESCE($3, material), spec = COALESCE($4, spec), weight = COALESCE($5, weight), surface_treatment = COALESCE($6, surface_treatment), part_type = COALESCE($7, part_type), version = COALESCE($8, version), row_revision = row_revision + 1 WHERE id = $1::uuid AND row_revision = $9`, revisionID, input.Name, input.Material, input.Spec, input.Weight, input.SurfaceTreatment, input.ManufacturingType, input.Version, *input.ExpectedRevision)
	if err != nil {
		if isUniqueViolation(err) {
			return Part{}, ErrConflict
		}
		return Part{}, fmt.Errorf("修改零件失败: %w", err)
	}
	if tag.RowsAffected() == 0 {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM parts WHERE id = $1::uuid)`, id).Scan(&exists); err != nil {
			return Part{}, fmt.Errorf("检查零件版本失败: %w", err)
		}
		if !exists {
			return Part{}, ErrNotFound
		}
		return Part{}, ErrRevisionConflict
	}
	if changesRelation {
		tag, err := tx.Exec(ctx, `UPDATE drawing_part_relations SET qty = COALESCE($2, qty), remark = COALESCE($3, remark), revision = revision + 1, updated_by = $4::uuid WHERE id = $1::uuid AND part_id = $5::uuid AND status = 'active' AND revision = $6`, targetRelID, input.Quantity, input.Remark, userID, id, *input.ExpectedRelationRevision)
		if err != nil {
			return Part{}, fmt.Errorf("更新结构关系失败: %w", err)
		}
		if tag.RowsAffected() != 1 {
			return Part{}, ErrRevisionConflict
		}
	}
	part, err := findPartSnapshot(ctx, tx, id, targetRelID, revisionID)
	if err != nil {
		return Part{}, err
	}
	rows, err := tx.Query(ctx, `SELECT role, signer_name FROM drawing_signers WHERE part_revision_id = $1::uuid`, revisionID)
	if err != nil {
		return Part{}, err
	}
	part.Signers = Signers{}
	for rows.Next() {
		var role, name string
		if err := rows.Scan(&role, &name); err != nil {
			rows.Close()
			return Part{}, err
		}
		part.Signers[role] = name
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return Part{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Part{}, fmt.Errorf("提交零件修改事务失败: %w", err)
	}
	return part, nil
}

func nullableString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func savePartSigners(ctx context.Context, tx pgx.Tx, revisionID string, signers Signers) error {
	for role, name := range signers {
		if strings.TrimSpace(name) == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO drawing_signers (part_revision_id, role, signer_name) VALUES ($1::uuid, $2, $3)`, revisionID, role, name); err != nil {
			return fmt.Errorf("保存零件签署人员失败: %w", err)
		}
	}
	return nil
}

func (repository *PGRepository) replaceDrawingAttributeValues(ctx context.Context, tx pgx.Tx, drawingID string, values map[string]string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM drawing_attribute_values WHERE drawing_id = $1::uuid`, drawingID); err != nil {
		return fmt.Errorf("清理图纸属性值失败: %w", err)
	}
	for attributeID, fieldID := range values {
		if strings.TrimSpace(attributeID) == "" || strings.TrimSpace(fieldID) == "" {
			continue
		}
		result, err := tx.Exec(ctx, `
			INSERT INTO drawing_attribute_values (drawing_id, attribute_id, field_id)
			SELECT $1::uuid, f.attribute_id, f.id
			FROM drawing_attribute_fields f
			WHERE f.attribute_id = $2::uuid AND f.id = $3::uuid`, drawingID, attributeID, fieldID)
		if err != nil || result.RowsAffected() == 0 {
			return fmt.Errorf("图纸属性值无效")
		}
	}
	return nil
}

func (repository *PGRepository) loadDrawingSigners(ctx context.Context, id string) (Signers, error) {
	return repository.loadSigners(ctx, "drawing_id", id)
}

func (repository *PGRepository) loadDrawingAttributeValues(ctx context.Context, id string) (map[string]string, error) {
	rows, err := repository.pool.Query(ctx, `
		SELECT attribute_id::text, field_id::text
		FROM drawing_attribute_values
		WHERE drawing_id = $1::uuid`, id)
	if err != nil {
		return nil, fmt.Errorf("读取图纸属性值失败: %w", err)
	}
	defer rows.Close()
	values := make(map[string]string)
	for rows.Next() {
		var attributeID, fieldID string
		if err := rows.Scan(&attributeID, &fieldID); err != nil {
			return nil, err
		}
		values[attributeID] = fieldID
	}
	return values, rows.Err()
}

func (repository *PGRepository) loadPartSigners(ctx context.Context, id string) (Signers, error) {
	rows, err := repository.pool.Query(ctx, `
		SELECT s.role, s.signer_name
		FROM drawing_signers s
		JOIN part_revisions pr ON pr.id = s.part_revision_id
		WHERE pr.part_id = $1::uuid
		ORDER BY pr.revision_no DESC`, id)
	if err != nil {
		return nil, fmt.Errorf("读取零件签署人员失败: %w", err)
	}
	defer rows.Close()
	signers := Signers{}
	for rows.Next() {
		var role, name string
		if err := rows.Scan(&role, &name); err != nil {
			return nil, err
		}
		signers[role] = name
	}
	return signers, rows.Err()
}

func (repository *PGRepository) loadSigners(ctx context.Context, ownerColumn string, id string) (Signers, error) {
	rows, err := repository.pool.Query(ctx, `SELECT role, signer_name FROM drawing_signers WHERE `+ownerColumn+` = $1::uuid`, id)
	if err != nil {
		return nil, fmt.Errorf("读取签署人员失败: %w", err)
	}
	defer rows.Close()
	signers := Signers{}
	for rows.Next() {
		var role string
		var name string
		if err := rows.Scan(&role, &name); err != nil {
			return nil, err
		}
		signers[role] = name
	}
	return signers, rows.Err()
}

func (repository *PGRepository) saveSigners(ctx context.Context, tx pgx.Tx, drawingID string, signers Signers) error {
	for role, name := range signers {
		if strings.TrimSpace(name) == "" {
			name = "待定"
		}
		if _, err := tx.Exec(ctx, `INSERT INTO drawing_signers (drawing_id, role, signer_name) VALUES ($1::uuid, $2, $3)`, drawingID, role, name); err != nil {
			return fmt.Errorf("保存签署人员失败: %w", err)
		}
	}
	return nil
}

func isUniqueViolation(err error) bool {
	return strings.Contains(err.Error(), "duplicate key value") || strings.Contains(err.Error(), "unique constraint")
}
