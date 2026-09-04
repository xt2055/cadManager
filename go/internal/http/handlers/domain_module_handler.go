package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"cadguanliq/internal/http/middleware"
	"cadguanliq/internal/response"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type moduleDrawing struct {
	ID              string            `json:"id,omitempty"`
	No              string            `json:"no"`
	Name            string            `json:"name"`
	Kind            string            `json:"kind"`
	Project         string            `json:"project"`
	Material        string            `json:"material"`
	Vendor          string            `json:"vendor"`
	Status          string            `json:"status"`
	Version         string            `json:"ver"`
	BorrowFrom      *string           `json:"borrowFrom,omitempty"`
	Remark          *string           `json:"remark,omitempty"`
	AttributeValues map[string]string `json:"attributeValues,omitempty"`
	Signers         map[string]string `json:"signers,omitempty"`
}

type modulePart struct {
	ID                string            `json:"id,omitempty"`
	DrawingID         string            `json:"drawingId,omitempty"`
	No                string            `json:"no"`
	Name              string            `json:"name"`
	ParentNo          string            `json:"parentNo"`
	Project           string            `json:"project,omitempty"`
	Material          string            `json:"material"`
	Spec              string            `json:"spec"`
	Weight            float64           `json:"weight"`
	SurfaceTreatment  string            `json:"surfaceTreatment"`
	ManufacturingType string            `json:"partType"`
	Quantity          float64           `json:"qty"`
	Status            string            `json:"status"`
	Version           string            `json:"ver"`
	Vendor            *string           `json:"vendor,omitempty"`
	BorrowFrom        *string           `json:"borrowFrom,omitempty"`
	Remark            *string           `json:"remark,omitempty"`
	Signers           map[string]string `json:"signers,omitempty"`
}

type moduleAttribute struct {
	ID        string                 `json:"id,omitempty"`
	Name      string                 `json:"name"`
	Required  bool                   `json:"required"`
	Enabled   bool                   `json:"enabled"`
	SortOrder int                    `json:"sortOrder"`
	Fields    []moduleAttributeField `json:"fields"`
}

type moduleAttributeField struct {
	ID        string `json:"id,omitempty"`
	Name      string `json:"name"`
	Enabled   bool   `json:"enabled"`
	SortOrder int    `json:"sortOrder"`
}

type moduleBranch struct {
	ID              string `json:"id,omitempty"`
	SourceDrawingNo string `json:"sourceDrawingNo,omitempty"`
	TargetDrawingNo string `json:"targetDrawingNo,omitempty"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	Status          string `json:"status"`
}

type moduleBorrow struct {
	ID              string `json:"id,omitempty"`
	Direction       string `json:"dir"`
	Project         string `json:"project"`
	Part            string `json:"part"`
	PartNo          string `json:"partNo,omitempty"`
	PartName        string `json:"partName,omitempty"`
	SourceDrawingNo string `json:"sourceDrawingNo,omitempty"`
	TargetDrawingNo string `json:"targetDrawingNo,omitempty"`
	Date            string `json:"date"`
	Status          string `json:"status"`
}

type moduleBom struct {
	No           int     `json:"no"`
	ID           string  `json:"id,omitempty"`
	DrawingNo    string  `json:"drawingNo"`
	SourceFileID *string `json:"sourceFileId,omitempty"`
	Name         string  `json:"name"`
	Spec         string  `json:"spec"`
	Quantity     float64 `json:"qty"`
	Weight       float64 `json:"weight"`
	Remark       string  `json:"remark"`
}

type moduleAttachment struct {
	ID                string  `json:"id"`
	StorageKey        string  `json:"storageKey"`
	CurrentStorageKey string  `json:"currentStorageKey,omitempty"`
	Name              string  `json:"name"`
	CurrentName       string  `json:"currentName,omitempty"`
	CurrentMimeType   string  `json:"currentMimeType,omitempty"`
	CurrentSize       int64   `json:"currentSize,omitempty"`
	DrawingNo         string  `json:"drawingNo"`
	PartNo            *string `json:"partNo,omitempty"`
	Role              string  `json:"role"`
	Size              int64   `json:"size"`
	MimeType          string  `json:"mimeType"`
	Version           string  `json:"version"`
	Previewable       bool    `json:"previewable"`
	UploadedBy        string  `json:"uploadedBy,omitempty"`
	CreatedAt         string  `json:"createdAt,omitempty"`
	Revision          int64   `json:"revision"`
}

func DataModules(pool *pgxpool.Pool) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if _, ok := middleware.UserFromContext(request.Context()); !ok {
			response.WriteError(writer, http.StatusUnauthorized, "登录已失效，请重新登录")
			return
		}
		module := strings.Trim(strings.TrimPrefix(request.URL.Path, "/api/data/"), "/")
		switch module {
		case "drawings":
			moduleDrawings(writer, request, pool)
		case "structure":
			moduleStructure(writer, request, pool)
		case "attributes":
			moduleAttributes(writer, request, pool)
		case "bom":
			moduleBOM(writer, request, pool)
		case "branches":
			moduleBranches(writer, request, pool)
		case "borrows":
			moduleBorrows(writer, request, pool)
		case "attachments":
			moduleAttachments(writer, request, pool)
		case "versions":
			moduleVersions(writer, request, pool)
		case "crafts":
			moduleCrafts(writer, request, pool)
		default:
			response.WriteError(writer, http.StatusNotFound, "数据模块不存在")
		}
	}
}

func moduleDrawings(writer http.ResponseWriter, request *http.Request, pool *pgxpool.Pool) {
	if request.Method == http.MethodGet {
		rows, err := pool.Query(request.Context(), `
			SELECT d.id::text, d.drawing_no, d.name, d.kind, d.project, d.material, d.vendor, d.status, d.version,
			       d.borrow_from, d.remark, COALESCE(u.display_name, u.account, ''), d.created_at, d.updated_at,
			       COALESCE((SELECT jsonb_object_agg(v.attribute_id::text, v.field_id::text)
			                  FROM drawing_attribute_values v WHERE v.drawing_id = d.id), '{}'::jsonb)
			FROM drawings d LEFT JOIN users u ON u.id = d.updated_by
			ORDER BY d.updated_at DESC, d.drawing_no`)
		if err != nil {
			response.WriteError(writer, http.StatusInternalServerError, "图纸模块读取失败")
			return
		}
		defer rows.Close()
		items := make([]moduleDrawing, 0)
		for rows.Next() {
			var item moduleDrawing
			var updatedBy string
			var createdAt, updatedAt time.Time
			var attributeValues []byte
			if err := rows.Scan(&item.ID, &item.No, &item.Name, &item.Kind, &item.Project, &item.Material, &item.Vendor, &item.Status, &item.Version, &item.BorrowFrom, &item.Remark, &updatedBy, &createdAt, &updatedAt, &attributeValues); err != nil {
				response.WriteError(writer, http.StatusInternalServerError, "图纸模块读取失败")
				return
			}
			if err := json.Unmarshal(attributeValues, &item.AttributeValues); err != nil {
				response.WriteError(writer, http.StatusInternalServerError, "图纸属性值读取失败")
				return
			}
			item.Signers, err = readSigners(request, pool, item.ID)
			if err != nil {
				response.WriteError(writer, http.StatusInternalServerError, "图纸签署人读取失败")
				return
			}
			_ = updatedBy
			_ = createdAt
			_ = updatedAt
			items = append(items, item)
		}
		response.WriteData(writer, http.StatusOK, items)
		return
	}
	if request.Method != http.MethodPut {
		response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var items []moduleDrawing
	if err := json.NewDecoder(request.Body).Decode(&items); err != nil {
		response.WriteError(writer, http.StatusBadRequest, "图纸模块格式无效")
		return
	}
	user, _ := middleware.UserFromContext(request.Context())
	tx, err := pool.Begin(request.Context())
	if err != nil {
		response.WriteError(writer, http.StatusInternalServerError, "图纸模块保存失败")
		return
	}
	defer tx.Rollback(request.Context())
	for _, item := range items {
		if strings.TrimSpace(item.No) == "" || strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.Project) == "" {
			continue
		}
		if _, err := tx.Exec(request.Context(), `
			INSERT INTO drawings (drawing_no, name, project, kind, material, vendor, status, version, borrow_from, remark, created_by, updated_by)
			VALUES ($1, $2, $3, COALESCE(NULLIF($4, ''), '总图'), COALESCE(NULLIF($5, ''), '—'), $6, COALESCE(NULLIF($7, ''), 'draft'), COALESCE(NULLIF($8, ''), 'v1.0'), $9, $10, $11::uuid, $11::uuid)
			ON CONFLICT (drawing_no) DO UPDATE SET name = EXCLUDED.name, project = EXCLUDED.project,
				kind = EXCLUDED.kind, material = EXCLUDED.material, vendor = EXCLUDED.vendor,
				status = EXCLUDED.status, version = EXCLUDED.version, borrow_from = EXCLUDED.borrow_from, remark = EXCLUDED.remark,
			updated_by = EXCLUDED.updated_by, updated_at = now()`, item.No, item.Name, item.Project, item.Kind, item.Material, item.Vendor, item.Status, item.Version, item.BorrowFrom, item.Remark, user.ID); err != nil {
			response.WriteError(writer, http.StatusBadRequest, "图纸模块保存失败")
			return
		}
		var drawingID string
		if err := tx.QueryRow(request.Context(), `SELECT id::text FROM drawings WHERE drawing_no = $1`, item.No).Scan(&drawingID); err != nil {
			response.WriteError(writer, http.StatusBadRequest, "图纸模块保存失败")
			return
		}
		if _, err := tx.Exec(request.Context(), `DELETE FROM drawing_attribute_values WHERE drawing_id = $1::uuid`, drawingID); err != nil {
			response.WriteError(writer, http.StatusBadRequest, "图纸属性值保存失败")
			return
		}
		for attributeID, fieldID := range item.AttributeValues {
			result, err := tx.Exec(request.Context(), `
				INSERT INTO drawing_attribute_values (drawing_id, attribute_id, field_id)
				SELECT $1::uuid, f.attribute_id, f.id
				FROM drawing_attribute_fields f
				WHERE f.attribute_id = $2::uuid AND f.id = $3::uuid`, drawingID, attributeID, fieldID)
			if err != nil || result.RowsAffected() == 0 {
				response.WriteError(writer, http.StatusBadRequest, "图纸属性值无效")
				return
			}
		}
		if err := saveModuleSigners(request, tx, drawingID, item.Signers, true); err != nil {
			response.WriteError(writer, http.StatusBadRequest, "图纸签署人保存失败")
			return
		}
	}
	if err := tx.Commit(request.Context()); err != nil {
		response.WriteError(writer, http.StatusInternalServerError, "图纸模块保存失败")
		return
	}
	response.WriteData(writer, http.StatusOK, nil)
}

func moduleStructure(writer http.ResponseWriter, request *http.Request, pool *pgxpool.Pool) {
	if request.Method == http.MethodGet {
		rows, err := pool.Query(request.Context(), `
			SELECT p.id::text, p.drawing_id::text, p.part_no, p.name, COALESCE(parent.part_no, ''),
			       COALESCE(p.project, d.project, ''), p.material, p.spec, p.weight, p.surface_treatment,
			       p.manufacturing_type, p.quantity, p.status, p.version, p.vendor, p.borrow_from, p.remark
			FROM structure_parts p JOIN drawings d ON d.id = p.drawing_id
			LEFT JOIN structure_parts parent ON parent.id = p.parent_part_id
			ORDER BY p.part_no`)
		if err != nil {
			response.WriteError(writer, http.StatusInternalServerError, "结构模块读取失败")
			return
		}
		defer rows.Close()
		items := make([]modulePart, 0)
		for rows.Next() {
			var item modulePart
			if err := rows.Scan(&item.ID, &item.DrawingID, &item.No, &item.Name, &item.ParentNo, &item.Project, &item.Material, &item.Spec, &item.Weight, &item.SurfaceTreatment, &item.ManufacturingType, &item.Quantity, &item.Status, &item.Version, &item.Vendor, &item.BorrowFrom, &item.Remark); err != nil {
				response.WriteError(writer, http.StatusInternalServerError, "结构模块读取失败")
				return
			}
			item.Signers, err = readSigners(request, pool, item.ID)
			if err != nil {
				response.WriteError(writer, http.StatusInternalServerError, "零件签署人读取失败")
				return
			}
			items = append(items, item)
		}
		response.WriteData(writer, http.StatusOK, items)
		return
	}
	if request.Method != http.MethodPut {
		response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var items []modulePart
	if err := json.NewDecoder(request.Body).Decode(&items); err != nil {
		response.WriteError(writer, http.StatusBadRequest, "结构模块格式无效")
		return
	}
	user, _ := middleware.UserFromContext(request.Context())
	tx, err := pool.Begin(request.Context())
	if err != nil {
		response.WriteError(writer, http.StatusInternalServerError, "结构模块保存失败")
		return
	}
	defer tx.Rollback(request.Context())
	for _, item := range items {
		if strings.TrimSpace(item.No) == "" || strings.TrimSpace(item.Name) == "" {
			continue
		}
		var drawingID string
		if item.DrawingID != "" {
			drawingID = item.DrawingID
		} else {
			rootNo := item.ParentNo
			for _, candidate := range items {
				if candidate.No == rootNo && candidate.ParentNo != "" {
					rootNo = candidate.ParentNo
				}
			}
			if err := tx.QueryRow(request.Context(), `SELECT id::text FROM drawings WHERE drawing_no = $1`, rootNo).Scan(&drawingID); err != nil {
				continue
			}
		}
		if _, err := tx.Exec(request.Context(), `
			INSERT INTO structure_parts (drawing_id, part_no, name, project, material, spec, weight, surface_treatment, manufacturing_type, quantity, status, version, vendor, borrow_from, remark, created_by, updated_by)
			VALUES ($1::uuid, $2, $3, NULLIF($4, ''), COALESCE(NULLIF($5, ''), '—'), $6, $7, $8, COALESCE(NULLIF($9, ''), '自制件'), COALESCE(NULLIF($10, 0), 1), COALESCE(NULLIF($11, ''), 'draft'), COALESCE(NULLIF($12, ''), 'v1.0'), $13, $14, $15, $16::uuid, $16::uuid)
			ON CONFLICT (part_no) DO UPDATE SET drawing_id = EXCLUDED.drawing_id, name = EXCLUDED.name, project = EXCLUDED.project,
				material = EXCLUDED.material, spec = EXCLUDED.spec, weight = EXCLUDED.weight, surface_treatment = EXCLUDED.surface_treatment,
				manufacturing_type = EXCLUDED.manufacturing_type, quantity = EXCLUDED.quantity, status = EXCLUDED.status, version = EXCLUDED.version,
				vendor = EXCLUDED.vendor, borrow_from = EXCLUDED.borrow_from, remark = EXCLUDED.remark, updated_by = EXCLUDED.updated_by, updated_at = now()`, drawingID, item.No, item.Name, item.Project, item.Material, item.Spec, item.Weight, item.SurfaceTreatment, item.ManufacturingType, item.Quantity, item.Status, item.Version, item.Vendor, item.BorrowFrom, item.Remark, user.ID); err != nil {
			response.WriteError(writer, http.StatusBadRequest, "结构模块保存失败")
			return
		}
		var partID string
		if err := tx.QueryRow(request.Context(), `SELECT id::text FROM structure_parts WHERE part_no = $1`, item.No).Scan(&partID); err != nil {
			response.WriteError(writer, http.StatusBadRequest, "结构模块保存失败")
			return
		}
		if err := saveModuleSigners(request, tx, partID, item.Signers, false); err != nil {
			response.WriteError(writer, http.StatusBadRequest, "零件签署人保存失败")
			return
		}
	}
	for _, item := range items {
		if item.ParentNo == "" {
			continue
		}
		if _, err := tx.Exec(request.Context(), `
			UPDATE structure_parts child SET parent_part_id = parent.id
			FROM structure_parts parent WHERE child.part_no = $1 AND parent.part_no = $2`, item.No, item.ParentNo); err != nil {
			response.WriteError(writer, http.StatusBadRequest, "零件层级保存失败")
			return
		}
	}
	if err := tx.Commit(request.Context()); err != nil {
		response.WriteError(writer, http.StatusInternalServerError, "结构模块保存失败")
		return
	}
	response.WriteData(writer, http.StatusOK, nil)
}

func moduleAttributes(writer http.ResponseWriter, request *http.Request, pool *pgxpool.Pool) {
	if request.Method == http.MethodGet {
		items, err := listModuleAttributes(request, pool)
		if err != nil {
			response.WriteError(writer, http.StatusInternalServerError, "属性模块读取失败")
			return
		}
		response.WriteData(writer, http.StatusOK, items)
		return
	}
	if request.Method != http.MethodPut {
		response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var items []moduleAttribute
	if err := json.NewDecoder(request.Body).Decode(&items); err != nil {
		response.WriteError(writer, http.StatusBadRequest, "属性模块格式无效")
		return
	}
	tx, err := pool.Begin(request.Context())
	if err != nil {
		response.WriteError(writer, http.StatusInternalServerError, "属性模块保存失败")
		return
	}
	defer tx.Rollback(request.Context())
	for _, item := range items {
		if strings.TrimSpace(item.Name) == "" {
			continue
		}
		var attributeID string
		if err := tx.QueryRow(request.Context(), `INSERT INTO drawing_attributes (name, required, enabled, sort_order) VALUES ($1, $2, $3, $4) ON CONFLICT (name) DO UPDATE SET required = EXCLUDED.required, enabled = EXCLUDED.enabled, sort_order = EXCLUDED.sort_order RETURNING id::text`, item.Name, item.Required, item.Enabled, item.SortOrder).Scan(&attributeID); err != nil {
			response.WriteError(writer, http.StatusBadRequest, "属性模块保存失败")
			return
		}
		for _, field := range item.Fields {
			if strings.TrimSpace(field.Name) == "" {
				continue
			}
			if _, err := tx.Exec(request.Context(), `INSERT INTO drawing_attribute_fields (attribute_id, name, enabled, sort_order) VALUES ($1::uuid, $2, $3, $4) ON CONFLICT (attribute_id, name) DO UPDATE SET enabled = EXCLUDED.enabled, sort_order = EXCLUDED.sort_order`, attributeID, field.Name, field.Enabled, field.SortOrder); err != nil {
				response.WriteError(writer, http.StatusBadRequest, "属性字段保存失败")
				return
			}
		}
	}
	if err := tx.Commit(request.Context()); err != nil {
		response.WriteError(writer, http.StatusInternalServerError, "属性模块保存失败")
		return
	}
	response.WriteData(writer, http.StatusOK, nil)
}

func listModuleAttributes(request *http.Request, pool *pgxpool.Pool) ([]moduleAttribute, error) {
	rows, err := pool.Query(request.Context(), `SELECT id::text, name, required, enabled, sort_order FROM drawing_attributes ORDER BY sort_order, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]moduleAttribute, 0)
	for rows.Next() {
		var item moduleAttribute
		if err := rows.Scan(&item.ID, &item.Name, &item.Required, &item.Enabled, &item.SortOrder); err != nil {
			return nil, err
		}
		fieldRows, err := pool.Query(request.Context(), `SELECT id::text, name, enabled, sort_order FROM drawing_attribute_fields WHERE attribute_id = $1::uuid ORDER BY sort_order, name`, item.ID)
		if err != nil {
			return nil, err
		}
		item.Fields = make([]moduleAttributeField, 0)
		for fieldRows.Next() {
			var field moduleAttributeField
			if err := fieldRows.Scan(&field.ID, &field.Name, &field.Enabled, &field.SortOrder); err != nil {
				fieldRows.Close()
				return nil, err
			}
			item.Fields = append(item.Fields, field)
		}
		fieldRows.Close()
		items = append(items, item)
	}
	return items, rows.Err()
}

func moduleBOM(writer http.ResponseWriter, request *http.Request, pool *pgxpool.Pool) {
	if request.Method == http.MethodGet {
		rows, err := pool.Query(request.Context(), `SELECT d.drawing_no, b.item_no, b.source_attachment_id::text, b.name, b.spec, b.quantity, b.weight, b.remark FROM bom_items b JOIN drawings d ON d.id = b.drawing_id ORDER BY d.drawing_no, b.item_no`)
		if err != nil {
			response.WriteError(writer, http.StatusInternalServerError, "BOM 模块读取失败")
			return
		}
		defer rows.Close()
		items := make([]moduleBom, 0)
		for rows.Next() {
			var item moduleBom
			if err := rows.Scan(&item.DrawingNo, &item.No, &item.SourceFileID, &item.Name, &item.Spec, &item.Quantity, &item.Weight, &item.Remark); err != nil {
				response.WriteError(writer, http.StatusInternalServerError, "BOM 模块读取失败")
				return
			}
			items = append(items, item)
		}
		response.WriteData(writer, http.StatusOK, items)
		return
	}
	if request.Method != http.MethodPut {
		response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var items []moduleBom
	if err := json.NewDecoder(request.Body).Decode(&items); err != nil {
		response.WriteError(writer, http.StatusBadRequest, "BOM 模块格式无效")
		return
	}
	tx, err := pool.Begin(request.Context())
	if err != nil {
		response.WriteError(writer, http.StatusInternalServerError, "BOM 模块保存失败")
		return
	}
	defer tx.Rollback(request.Context())
	if _, err := tx.Exec(request.Context(), `DELETE FROM bom_items WHERE drawing_id IS NOT NULL`); err != nil {
		response.WriteError(writer, http.StatusInternalServerError, "BOM 模块保存失败")
		return
	}
	for index, item := range items {
		var drawingID string
		if err := tx.QueryRow(request.Context(), `SELECT id::text FROM drawings WHERE drawing_no = $1`, item.DrawingNo).Scan(&drawingID); err != nil {
			continue
		}
		if _, err := tx.Exec(request.Context(), `INSERT INTO bom_items (drawing_id, source_attachment_id, item_no, name, spec, quantity, weight, remark) VALUES ($1::uuid, NULLIF($2, '')::uuid, $3, $4, $5, $6, $7, $8)`, drawingID, nullableModuleString(item.SourceFileID), index+1, item.Name, item.Spec, item.Quantity, item.Weight, item.Remark); err != nil {
			response.WriteError(writer, http.StatusBadRequest, "BOM 模块保存失败")
			return
		}
	}
	if err := tx.Commit(request.Context()); err != nil {
		response.WriteError(writer, http.StatusInternalServerError, "BOM 模块保存失败")
		return
	}
	response.WriteData(writer, http.StatusOK, nil)
}

func moduleBranches(writer http.ResponseWriter, request *http.Request, pool *pgxpool.Pool) {
	if request.Method == http.MethodGet {
		rows, err := pool.Query(request.Context(), `SELECT COALESCE(source.drawing_no, ''), COALESCE(target.drawing_no, ''), b.name, b.description, b.status FROM drawing_branches b LEFT JOIN drawings source ON source.id = b.source_drawing_id LEFT JOIN drawings target ON target.id = b.target_drawing_id ORDER BY b.created_at DESC`)
		if err != nil {
			response.WriteError(writer, http.StatusInternalServerError, "分支模块读取失败")
			return
		}
		defer rows.Close()
		items := make([]moduleBranch, 0)
		for rows.Next() {
			var item moduleBranch
			if err := rows.Scan(&item.SourceDrawingNo, &item.TargetDrawingNo, &item.Name, &item.Description, &item.Status); err != nil {
				response.WriteError(writer, http.StatusInternalServerError, "分支模块读取失败")
				return
			}
			items = append(items, item)
		}
		response.WriteData(writer, http.StatusOK, items)
		return
	}
	if request.Method != http.MethodPut {
		response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var items []moduleBranch
	if err := json.NewDecoder(request.Body).Decode(&items); err != nil {
		response.WriteError(writer, http.StatusBadRequest, "分支模块格式无效")
		return
	}
	user, _ := middleware.UserFromContext(request.Context())
	tx, err := pool.Begin(request.Context())
	if err != nil {
		response.WriteError(writer, http.StatusInternalServerError, "分支模块保存失败")
		return
	}
	defer tx.Rollback(request.Context())
	for _, item := range items {
		var sourceID, targetID *string
		var sourceValue, targetValue string
		if err := tx.QueryRow(request.Context(), `SELECT id::text FROM drawings WHERE drawing_no = $1`, item.SourceDrawingNo).Scan(&sourceValue); err == nil {
			sourceID = &sourceValue
		}
		if err := tx.QueryRow(request.Context(), `SELECT id::text FROM drawings WHERE drawing_no = $1`, item.TargetDrawingNo).Scan(&targetValue); err == nil {
			targetID = &targetValue
		}
		if _, err := tx.Exec(request.Context(), `INSERT INTO drawing_branches (source_drawing_id, target_drawing_id, name, description, status, created_by) VALUES (NULLIF($1, '')::uuid, NULLIF($2, '')::uuid, $3, $4, COALESCE(NULLIF($5, ''), '使用中'), $6::uuid)`, nullableModuleString(sourceID), nullableModuleString(targetID), item.Name, item.Description, item.Status, user.ID); err != nil {
			response.WriteError(writer, http.StatusBadRequest, "分支模块保存失败")
			return
		}
	}
	if err := tx.Commit(request.Context()); err != nil {
		response.WriteError(writer, http.StatusInternalServerError, "分支模块保存失败")
		return
	}
	response.WriteData(writer, http.StatusOK, nil)
}

func moduleBorrows(writer http.ResponseWriter, request *http.Request, pool *pgxpool.Pool) {
	if request.Method == http.MethodGet {
		rows, err := pool.Query(request.Context(), `SELECT r.id::text, r.direction, COALESCE(target.name, source.name, ''), COALESCE(target.drawing_no, source.drawing_no, ''), COALESCE(targetPart.part_no, sourcePart.part_no, ''), COALESCE(targetPart.name, sourcePart.name, ''), COALESCE(source.drawing_no, ''), COALESCE(target.drawing_no, ''), COALESCE(r.created_at::date::text, ''), r.status FROM borrow_records r LEFT JOIN drawings source ON source.id = r.source_drawing_id LEFT JOIN drawings target ON target.id = r.target_drawing_id LEFT JOIN structure_parts sourcePart ON sourcePart.id = r.source_part_id LEFT JOIN structure_parts targetPart ON targetPart.id = r.target_part_id ORDER BY r.created_at DESC`)
		if err != nil {
			response.WriteError(writer, http.StatusInternalServerError, "借用模块读取失败")
			return
		}
		defer rows.Close()
		items := make([]moduleBorrow, 0)
		for rows.Next() {
			var item moduleBorrow
			if err := rows.Scan(&item.ID, &item.Direction, &item.Project, &item.Part, &item.PartNo, &item.PartName, &item.SourceDrawingNo, &item.TargetDrawingNo, &item.Date, &item.Status); err != nil {
				response.WriteError(writer, http.StatusInternalServerError, "借用模块读取失败")
				return
			}
			items = append(items, item)
		}
		response.WriteData(writer, http.StatusOK, items)
		return
	}
	if request.Method != http.MethodPut {
		response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var items []moduleBorrow
	if err := json.NewDecoder(request.Body).Decode(&items); err != nil {
		response.WriteError(writer, http.StatusBadRequest, "借用模块格式无效")
		return
	}
	user, _ := middleware.UserFromContext(request.Context())
	tx, err := pool.Begin(request.Context())
	if err != nil {
		response.WriteError(writer, http.StatusInternalServerError, "借用模块保存失败")
		return
	}
	defer tx.Rollback(request.Context())
	if _, err := tx.Exec(request.Context(), `DELETE FROM borrow_records WHERE source_drawing_id IS NOT NULL OR target_drawing_id IS NOT NULL`); err != nil {
		response.WriteError(writer, http.StatusInternalServerError, "借用模块保存失败")
		return
	}
	for _, item := range items {
		var sourceDrawingID, targetDrawingID, sourcePartID, targetPartID string
		_ = tx.QueryRow(request.Context(), `SELECT id::text FROM drawings WHERE drawing_no = $1`, item.SourceDrawingNo).Scan(&sourceDrawingID)
		_ = tx.QueryRow(request.Context(), `SELECT id::text FROM drawings WHERE drawing_no = $1`, item.TargetDrawingNo).Scan(&targetDrawingID)
		_ = tx.QueryRow(request.Context(), `SELECT id::text FROM structure_parts WHERE part_no = $1`, item.PartNo).Scan(&sourcePartID)
		if _, err := tx.Exec(request.Context(), `INSERT INTO borrow_records (source_drawing_id, source_part_id, target_drawing_id, target_part_id, direction, status, created_by) VALUES (NULLIF($1, '')::uuid, NULLIF($2, '')::uuid, NULLIF($3, '')::uuid, NULLIF($4, '')::uuid, COALESCE(NULLIF($5, ''), 'in'), CASE WHEN $6 = '已归档' THEN 'archived' ELSE 'active' END, $7::uuid)`, sourceDrawingID, sourcePartID, targetDrawingID, targetPartID, item.Direction, item.Status, user.ID); err != nil {
			response.WriteError(writer, http.StatusBadRequest, "借用模块保存失败")
			return
		}
	}
	if err := tx.Commit(request.Context()); err != nil {
		response.WriteError(writer, http.StatusInternalServerError, "借用模块保存失败")
		return
	}
	response.WriteData(writer, http.StatusOK, nil)
}

func moduleAttachments(writer http.ResponseWriter, request *http.Request, pool *pgxpool.Pool) {
	if request.Method != http.MethodGet {
		response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	rows, err := pool.Query(request.Context(), `SELECT a.id::text, COALESCE(ob.storage_key, a.storage_key), COALESCE(cb.storage_key, a.current_storage_key, ''), a.original_name, COALESCE(a.current_name, ''), COALESCE(a.current_mime_type, a.mime_type), COALESCE(a.current_size_bytes, a.size_bytes), COALESCE(d.drawing_no, parent.drawing_no, ''), p.part_no, a.file_role, a.size_bytes, a.mime_type, a.version, a.previewable, COALESCE(a.uploaded_by::text, ''), a.created_at, a.revision FROM attachments a LEFT JOIN file_blobs ob ON ob.id = a.blob_id LEFT JOIN file_blobs cb ON cb.id = a.current_blob_id LEFT JOIN drawings d ON d.id = a.drawing_id LEFT JOIN structure_parts p ON p.id = a.part_id LEFT JOIN drawings parent ON parent.id = p.drawing_id WHERE a.deleted_at IS NULL ORDER BY a.created_at DESC`)
	if err != nil {
		response.WriteError(writer, http.StatusInternalServerError, "附件模块读取失败")
		return
	}
	defer rows.Close()
	items := make([]moduleAttachment, 0)
	for rows.Next() {
		var item moduleAttachment
		var createdAt time.Time
		if err := rows.Scan(&item.ID, &item.StorageKey, &item.CurrentStorageKey, &item.Name, &item.CurrentName, &item.CurrentMimeType, &item.CurrentSize, &item.DrawingNo, &item.PartNo, &item.Role, &item.Size, &item.MimeType, &item.Version, &item.Previewable, &item.UploadedBy, &createdAt, &item.Revision); err != nil {
			response.WriteError(writer, http.StatusInternalServerError, "附件模块读取失败")
			return
		}
		item.CreatedAt = createdAt.Format(time.RFC3339)
		items = append(items, item)
	}
	response.WriteData(writer, http.StatusOK, items)
}

func moduleVersions(writer http.ResponseWriter, request *http.Request, pool *pgxpool.Pool) {
	if request.Method != http.MethodGet {
		response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	rows, err := pool.Query(request.Context(), `
		SELECT v.id::text, v.attachment_id::text, v.storage_key, v.version, v.version_kind,
		       v.size_bytes, v.mime_type, COALESCE(v.sha256, ''), COALESCE(u.display_name, u.account, ''), v.created_at,
		       v.is_current_release
		FROM file_versions v
		LEFT JOIN users u ON u.id = v.created_by
		WHERE v.deleted_at IS NULL
		ORDER BY v.created_at DESC`)
	if err != nil {
		response.WriteError(writer, http.StatusInternalServerError, "版本模块读取失败")
		return
	}
	defer rows.Close()
	type moduleVersion struct {
		ID               string    `json:"id"`
		AttachmentID     string    `json:"attachmentId"`
		StorageKey       string    `json:"storageKey"`
		Version          string    `json:"version"`
		VersionKind      string    `json:"versionKind"`
		Size             int64     `json:"size"`
		MimeType         string    `json:"mimeType"`
		SHA256           string    `json:"sha256,omitempty"`
		CreatedByName    string    `json:"createdByName,omitempty"`
		CreatedAt        time.Time `json:"createdAt"`
		IsCurrentRelease bool      `json:"isCurrentRelease"`
	}
	items := make([]moduleVersion, 0)
	for rows.Next() {
		var item moduleVersion
		if err := rows.Scan(&item.ID, &item.AttachmentID, &item.StorageKey, &item.Version, &item.VersionKind, &item.Size, &item.MimeType, &item.SHA256, &item.CreatedByName, &item.CreatedAt, &item.IsCurrentRelease); err != nil {
			response.WriteError(writer, http.StatusInternalServerError, "版本模块读取失败")
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		response.WriteError(writer, http.StatusInternalServerError, "版本模块读取失败")
		return
	}
	response.WriteData(writer, http.StatusOK, items)
}

func moduleCrafts(writer http.ResponseWriter, request *http.Request, pool *pgxpool.Pool) {
	if request.Method != http.MethodGet {
		response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	rows, err := pool.Query(request.Context(), `
		SELECT a.id::text, COALESCE(d.drawing_no, parent.drawing_no, ''),
		       COALESCE(a.current_name, a.original_name), a.version,
		       COALESCE(u.display_name, u.account, ''), a.created_at,
		       COALESCE(a.current_size_bytes, a.size_bytes),
		       COALESCE(a.current_storage_key, a.storage_key),
		       COALESCE(a.current_mime_type, a.mime_type), a.previewable
		FROM attachments a
		LEFT JOIN drawings d ON d.id = a.drawing_id
		LEFT JOIN structure_parts p ON p.id = a.part_id
		LEFT JOIN drawings parent ON parent.id = p.drawing_id
		LEFT JOIN users u ON u.id = a.uploaded_by
		WHERE a.file_role = 'craft' AND a.deleted_at IS NULL
		ORDER BY a.created_at DESC`)
	if err != nil {
		response.WriteError(writer, http.StatusInternalServerError, "工艺文件模块读取失败")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, drawingNo, name, version, by, storageKey, mimeType string
		var createdAt time.Time
		var size int64
		var previewable bool
		if err := rows.Scan(&id, &drawingNo, &name, &version, &by, &createdAt, &size, &storageKey, &mimeType, &previewable); err != nil {
			response.WriteError(writer, http.StatusInternalServerError, "工艺文件模块读取失败")
			return
		}
		items = append(items, map[string]any{
			"id": id, "drawingNo": drawingNo, "name": name, "op": "未分类工艺", "ver": version,
			"by": by, "date": createdAt.Format(time.RFC3339), "size": formatModuleSize(size),
			"storageKey": storageKey, "mimeType": mimeType, "previewable": previewable, "scanned": false,
		})
	}
	if err := rows.Err(); err != nil {
		response.WriteError(writer, http.StatusInternalServerError, "工艺文件模块读取失败")
		return
	}
	response.WriteData(writer, http.StatusOK, items)
}

func formatModuleSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	if size < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(size)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
}

func readSigners(request *http.Request, pool *pgxpool.Pool, ownerID string) (map[string]string, error) {
	rows, err := pool.Query(request.Context(), `SELECT role, signer_name FROM drawing_signers WHERE drawing_id = $1::uuid OR part_id = $1::uuid`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make(map[string]string)
	for rows.Next() {
		var role, name string
		if err := rows.Scan(&role, &name); err != nil {
			return nil, err
		}
		items[role] = name
	}
	return items, rows.Err()
}

func saveModuleSigners(request *http.Request, tx pgx.Tx, ownerID string, signers map[string]string, drawing bool) error {
	if drawing {
		if _, err := tx.Exec(request.Context(), `DELETE FROM drawing_signers WHERE drawing_id = $1::uuid`, ownerID); err != nil {
			return err
		}
	} else {
		if _, err := tx.Exec(request.Context(), `DELETE FROM drawing_signers WHERE part_id = $1::uuid`, ownerID); err != nil {
			return err
		}
	}
	for role, name := range signers {
		if strings.TrimSpace(role) == "" || strings.TrimSpace(name) == "" {
			continue
		}
		var err error
		if drawing {
			_, err = tx.Exec(request.Context(), `INSERT INTO drawing_signers (drawing_id, role, signer_name) VALUES ($1::uuid, $2, $3)`, ownerID, role, name)
		} else {
			_, err = tx.Exec(request.Context(), `INSERT INTO drawing_signers (part_id, role, signer_name) VALUES ($1::uuid, $2, $3)`, ownerID, role, name)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func nullableModuleString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func writeModuleNotFound(writer http.ResponseWriter, err error) {
	if errors.Is(err, pgx.ErrNoRows) {
		response.WriteError(writer, http.StatusNotFound, "模块资源不存在")
		return
	}
	response.WriteError(writer, http.StatusInternalServerError, "模块资源操作失败")
}
