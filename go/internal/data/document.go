package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
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
		rootNo := findRootDrawingNo(payload.Drawings, item.ParentNo)
		if rootNo == "" {
			// 零件可能先于总图存在（总图未创建、已删除或图号体系不完整），
			// 跳过该零件的镜像同步，不能阻塞整个文档保存。
			log.Printf("[数据同步] 跳过零件 %s：父级 %s 找不到所属总图", item.No, item.ParentNo)
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
				updated_by = EXCLUDED.updated_by, updated_at = now()`, rootNo, item.No, item.Name, item.Project, material, item.Spec, item.Weight, item.SurfaceTreatment, partType, quantity, status, version, item.Vendor, item.BorrowFrom, item.Remark, userID); err != nil {
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
			WHERE child.part_no = $1 AND parent.part_no = $2 AND child.id <> parent.id AND child.drawing_id = parent.drawing_id`, item.No, item.ParentNo); err != nil {
			return fmt.Errorf("同步零件 %s 父级失败: %w", item.No, err)
		}
	}
	return nil
}

// normalizeDrawingNoKey 生成图号的可比对键。标题栏真值可能含 /（如 JG9055e-50/32-00），
// 而 Windows 文件名不允许 /，文件名变体会丢失斜杠（如 JG9055e-5032-01）；
// 图纸编号中大小写混用也很常见（如 2000W.02.03c 与 2000W.02.03C）。
// 匹配时统一去除斜杠并忽略大小写，存储时保留原始写法。
func normalizeDrawingNoKey(value string) string {
	return strings.ReplaceAll(strings.ToUpper(strings.TrimSpace(value)), "/", "")
}

// drawingFamilyBase 返回图号去掉末段序号后的族基座。
// 例：JG9055e-50/32-00、JG9055e-5032-01、JG9055e-5032-01-1 的族基座均为 JG9055e-5032。
func drawingFamilyBase(value string) string {
	normalized := normalizeDrawingNoKey(value)
	if index := strings.LastIndex(normalized, "-"); index > 0 {
		return normalized[:index]
	}
	return normalized
}

// belongsToDrawingFamily 判断零件号是否属于某总图的图号族，兼容两种工程编号规则：
//  1. 前缀规则：零件号 = 总图号 + 序号后缀（JG-001 与 JG-001-01）。
//  2. 兄弟序号规则：零件号与总图号共享族基座（JG9055e-50/32-00 与 JG9055e-5032-01），
//     子件逐级去尾段后仍能回溯到同一基座（JG9055e-5032-01-1）。
func belongsToDrawingFamily(partNo, drawingNo string) bool {
	if partNo == "" || drawingNo == "" {
		return false
	}
	if partNo == drawingNo || strings.HasPrefix(partNo, drawingNo+"-") {
		return true
	}
	family := drawingFamilyBase(drawingNo)
	base := partNo
	for base != "" {
		if base == family {
			return true
		}
		index := strings.LastIndex(base, "-")
		if index <= 0 {
			return false
		}
		base = base[:index]
	}
	return false
}

// findRootDrawingNo 返回零件父级所属总图的图号；找不到时返回空串。
func findRootDrawingNo(drawings []compatibilityDrawing, parentNo string) string {
	normalizedParent := normalizeDrawingNoKey(parentNo)
	root := ""
	for _, item := range drawings {
		if !belongsToDrawingFamily(normalizedParent, normalizeDrawingNoKey(item.No)) {
			continue
		}
		if len(item.No) > len(root) {
			root = item.No
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
