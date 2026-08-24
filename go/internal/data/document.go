package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const documentKey = "default"

type DocumentRepository struct {
	pool *pgxpool.Pool
}

type compatibilityDrawing struct {
	No         string  `json:"no"`
	Name       string  `json:"name"`
	Kind       string  `json:"kind"`
	Project    string  `json:"project"`
	Material   string  `json:"material"`
	Vendor     string  `json:"vendor"`
	Status     string  `json:"status"`
	Version    string  `json:"ver"`
	BorrowFrom *string `json:"borrowFrom"`
	Remark     *string `json:"remark"`
}

type compatibilityPart struct {
	No               string  `json:"no"`
	Name             string  `json:"name"`
	ParentNo         string  `json:"parentNo"`
	Project          string  `json:"project"`
	Material         string  `json:"material"`
	Spec             string  `json:"spec"`
	Weight           float64 `json:"weight"`
	SurfaceTreatment string  `json:"surfaceTreatment"`
	PartType         string  `json:"partType"`
	Quantity         float64 `json:"qty"`
	Status           string  `json:"status"`
	Version          string  `json:"ver"`
	Vendor           *string `json:"vendor"`
	BorrowFrom       *string `json:"borrowFrom"`
	Remark           *string `json:"remark"`
}

func NewDocumentRepository(pool *pgxpool.Pool) *DocumentRepository {
	return &DocumentRepository{pool: pool}
}

func (repository *DocumentRepository) Load(ctx context.Context) (map[string]any, error) {
	if repository == nil || repository.pool == nil {
		return nil, errors.New("数据库连接未配置")
	}

	var raw []byte
	err := repository.pool.QueryRow(ctx, `
		SELECT document
		FROM data_documents
		WHERE document_key = $1`, documentKey).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return emptyDocument(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取业务数据文档失败: %w", err)
	}

	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, fmt.Errorf("解析业务数据文档失败: %w", err)
	}
	return document, nil
}

func (repository *DocumentRepository) Save(ctx context.Context, document map[string]any, userID string) error {
	if repository == nil || repository.pool == nil {
		return errors.New("数据库连接未配置")
	}
	raw, err := json.Marshal(document)
	if err != nil {
		return fmt.Errorf("编码业务数据文档失败: %w", err)
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("保存业务数据事务失败: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := syncCompatibilityRecords(ctx, tx, raw, userID); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO data_documents (document_key, document)
		VALUES ($1, $2::jsonb)
		ON CONFLICT (document_key) DO UPDATE
		SET document = EXCLUDED.document,
		    updated_at = now()`, documentKey, raw)
	if err != nil {
		return fmt.Errorf("保存业务数据文档失败: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("提交业务数据事务失败: %w", err)
	}
	return nil
}

func syncCompatibilityRecords(ctx context.Context, tx pgx.Tx, raw []byte, userID string) error {
	var payload struct {
		Drawings  []compatibilityDrawing `json:"drawings"`
		Structure []compatibilityPart    `json:"structure"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return fmt.Errorf("解析兼容图纸数据失败: %w", err)
	}
	for _, item := range payload.Drawings {
		if item.No == "" || item.Name == "" || item.Project == "" {
			continue
		}
		kind := item.Kind
		if kind == "" {
			kind = "总图"
		}
		material := item.Material
		if material == "" {
			material = "—"
		}
		status := item.Status
		if status == "" {
			status = "draft"
		}
		version := item.Version
		if version == "" {
			version = "v1.0"
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO drawings (drawing_no, name, project, kind, material, vendor, status, version, borrow_from, remark, created_by, updated_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NULLIF($11, '')::uuid, NULLIF($11, '')::uuid)
			ON CONFLICT (drawing_no) DO UPDATE SET
				name = EXCLUDED.name, project = EXCLUDED.project, kind = EXCLUDED.kind, material = EXCLUDED.material,
				vendor = EXCLUDED.vendor, status = EXCLUDED.status, version = EXCLUDED.version,
				borrow_from = EXCLUDED.borrow_from, remark = EXCLUDED.remark, updated_by = EXCLUDED.updated_by,
				updated_at = now()`, item.No, item.Name, item.Project, kind, material, item.Vendor, status, version, item.BorrowFrom, item.Remark, userID); err != nil {
			return fmt.Errorf("同步图纸 %s 失败: %w", item.No, err)
		}
	}
	for _, item := range payload.Structure {
		if item.No == "" || item.Name == "" || item.ParentNo == "" {
			continue
		}
		material := item.Material
		if material == "" {
			material = "—"
		}
		partType := item.PartType
		if partType == "" {
			partType = "自制件"
		}
		quantity := item.Quantity
		if quantity <= 0 {
			quantity = 1
		}
		status := item.Status
		if status == "" {
			status = "draft"
		}
		version := item.Version
		if version == "" {
			version = "v1.0"
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO structure_parts (drawing_id, part_no, name, project, material, spec, weight, surface_treatment,
				manufacturing_type, quantity, status, version, vendor, borrow_from, remark, created_by, updated_by)
			SELECT d.id, $2, $3, NULLIF($4, ''), $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15,
				NULLIF($16, '')::uuid, NULLIF($16, '')::uuid
			FROM drawings d WHERE d.drawing_no = $1
			ON CONFLICT (part_no) DO UPDATE SET
				drawing_id = EXCLUDED.drawing_id, name = EXCLUDED.name, project = EXCLUDED.project,
				material = EXCLUDED.material, spec = EXCLUDED.spec, weight = EXCLUDED.weight,
				surface_treatment = EXCLUDED.surface_treatment, manufacturing_type = EXCLUDED.manufacturing_type,
				quantity = EXCLUDED.quantity, status = EXCLUDED.status, version = EXCLUDED.version,
				vendor = EXCLUDED.vendor, borrow_from = EXCLUDED.borrow_from, remark = EXCLUDED.remark,
				updated_by = EXCLUDED.updated_by, updated_at = now()`, findRootDrawingNo(payload.Drawings, item.ParentNo), item.No, item.Name, item.Project, material, item.Spec, item.Weight, item.SurfaceTreatment, partType, quantity, status, version, item.Vendor, item.BorrowFrom, item.Remark, userID); err != nil {
			return fmt.Errorf("同步零件 %s 失败: %w", item.No, err)
		}
	}
	for _, item := range payload.Structure {
		if item.No == "" || item.ParentNo == "" || item.ParentNo == findRootDrawingNo(payload.Drawings, item.ParentNo) {
			continue
		}
		if _, err := tx.Exec(ctx, `
			UPDATE structure_parts child
			SET parent_part_id = parent.id
			FROM structure_parts parent
			WHERE child.part_no = $1 AND parent.part_no = $2 AND child.drawing_id = parent.drawing_id`, item.No, item.ParentNo); err != nil {
			return fmt.Errorf("同步零件 %s 父级失败: %w", item.No, err)
		}
	}
	return nil
}

func findRootDrawingNo(drawings []compatibilityDrawing, parentNo string) string {
	root := ""
	for _, item := range drawings {
		if parentNo == item.No || strings.HasPrefix(parentNo, item.No+"-") {
			if len(item.No) > len(root) {
				root = item.No
			}
		}
	}
	return root
}

func emptyDocument() map[string]any {
	return map[string]any{
		"version":          2,
		"drawings":         []any{},
		"structure":        []any{},
		"versions":         []any{},
		"branches":         []any{},
		"borrows":          []any{},
		"bom":              []any{},
		"crafts":           []any{},
		"logs":             []any{},
		"reviewCases":      []any{},
		"myReviews":        []any{},
		"completedReviews": []any{},
		"users":            []any{},
		"flows":            []any{},
		"hiddenList":       []any{},
		"adminLogs":        []any{},
	}
}
