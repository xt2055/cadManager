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
	ErrNotFound          = errors.New("drawing resource not found")
	ErrConflict          = errors.New("drawing resource conflict")
	ErrRevisionConflict  = errors.New("drawing resource revision conflict")
	ErrRevisionRequired  = errors.New("drawing resource revision required")
	ErrInvalidTransition = errors.New("drawing status transition not allowed")
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
	return Page[Drawing]{List: items, Total: total, Page: filter.Page, PageSize: filter.PageSize}, nil
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
		SELECT p.id::text, p.drawing_id::text, p.part_no, p.name, COALESCE(parent.part_no, ''),
		       COALESCE(p.project, d.project, ''), p.material, p.spec, p.weight, p.surface_treatment,
		       p.manufacturing_type, p.quantity, p.status, p.version, p.revision, p.vendor, p.borrow_from, p.remark,
		       COALESCE(created_user.display_name, created_user.account, ''), p.created_at,
		       COALESCE(updated_user.display_name, updated_user.account, created_user.display_name, created_user.account, ''), p.updated_at
		FROM structure_parts p
		JOIN drawings d ON d.id = p.drawing_id
		LEFT JOIN structure_parts parent ON parent.id = p.parent_part_id
		LEFT JOIN users created_user ON created_user.id = p.created_by
		LEFT JOIN users updated_user ON updated_user.id = p.updated_by
		WHERE p.drawing_id = $1::uuid
		ORDER BY p.part_no`, drawingID)
	if err != nil {
		return nil, fmt.Errorf("查询结构树失败: %w", err)
	}
	defer rows.Close()
	parts := make([]Part, 0)
	for rows.Next() {
		part, err := scanPart(rows)
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

func (repository *PGRepository) FindPart(ctx context.Context, id string) (Part, error) {
	row := repository.pool.QueryRow(ctx, `
		SELECT p.id::text, p.drawing_id::text, p.part_no, p.name, COALESCE(parent.part_no, ''),
		       COALESCE(p.project, d.project, ''), p.material, p.spec, p.weight, p.surface_treatment,
		       p.manufacturing_type, p.quantity, p.status, p.version, p.revision, p.vendor, p.borrow_from, p.remark,
		       COALESCE(created_user.display_name, created_user.account, ''), p.created_at,
		       COALESCE(updated_user.display_name, updated_user.account, created_user.display_name, created_user.account, ''), p.updated_at
		FROM structure_parts p
		JOIN drawings d ON d.id = p.drawing_id
		LEFT JOIN structure_parts parent ON parent.id = p.parent_part_id
		LEFT JOIN users created_user ON created_user.id = p.created_by
		LEFT JOIN users updated_user ON updated_user.id = p.updated_by
		WHERE p.id = $1::uuid`, id)
	part, err := scanPart(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Part{}, ErrNotFound
	}
	if err != nil {
		return Part{}, fmt.Errorf("查询零件详情失败: %w", err)
	}
	part.Signers, err = repository.loadPartSigners(ctx, part.ID)
	return part, err
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
	var parentID *string
	if strings.TrimSpace(input.ParentNo) != "" {
		var value string
		err := repository.pool.QueryRow(ctx, `SELECT id::text FROM structure_parts WHERE drawing_id = $1::uuid AND part_no = $2`, drawingID, input.ParentNo).Scan(&value)
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
	var id string
	err := repository.pool.QueryRow(ctx, `
		INSERT INTO structure_parts (drawing_id, parent_part_id, part_no, name, project, material, spec, weight,
			surface_treatment, manufacturing_type, quantity, status, version, vendor, borrow_from, remark, created_by, updated_by)
		VALUES ($1::uuid, $2::uuid, $3, $4, NULLIF($5, ''), $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17::uuid, $17::uuid)
		RETURNING id::text`, drawingID, parentID, input.No, input.Name, input.Project, input.Material, input.Spec, input.Weight, input.SurfaceTreatment, partType, quantity, status, version, input.Vendor, input.BorrowFrom, input.Remark, userID).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			return Part{}, ErrConflict
		}
		return Part{}, fmt.Errorf("创建零件失败: %w", err)
	}
	return repository.FindPart(ctx, id)
}

func (repository *PGRepository) UpdatePart(ctx context.Context, id string, input UpdatePartInput, userID string) (Part, error) {
	if input.ExpectedRevision == nil || *input.ExpectedRevision < 1 {
		return Part{}, ErrRevisionRequired
	}
	tag, err := repository.pool.Exec(ctx, `
		UPDATE structure_parts
		SET part_no = COALESCE($2, part_no), name = COALESCE($3, name), material = COALESCE($4, material), spec = COALESCE($5, spec),
		    weight = COALESCE($6, weight), surface_treatment = COALESCE($7, surface_treatment),
		    manufacturing_type = COALESCE($8, manufacturing_type), quantity = COALESCE($9, quantity),
		    status = COALESCE($10, status), version = COALESCE($11, version), vendor = COALESCE($12, vendor),
		    borrow_from = COALESCE($13, borrow_from), remark = COALESCE($14, remark), updated_by = $15::uuid,
		    revision = revision + 1
		WHERE id = $1::uuid AND revision = $16`, id, input.No, input.Name, input.Material, input.Spec, input.Weight, input.SurfaceTreatment, input.ManufacturingType, input.Quantity, input.Status, input.Version, input.Vendor, input.BorrowFrom, input.Remark, userID, *input.ExpectedRevision)
	if err != nil {
		if isUniqueViolation(err) {
			return Part{}, ErrConflict
		}
		return Part{}, fmt.Errorf("修改零件失败: %w", err)
	}
	if tag.RowsAffected() == 0 {
		var exists bool
		if err := repository.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM structure_parts WHERE id = $1::uuid)`, id).Scan(&exists); err != nil {
			return Part{}, fmt.Errorf("检查零件版本失败: %w", err)
		}
		if !exists {
			return Part{}, ErrNotFound
		}
		return Part{}, ErrRevisionConflict
	}
	return repository.FindPart(ctx, id)
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
	return repository.loadSigners(ctx, "part_id", id)
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
