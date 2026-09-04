package handlers

import (
	"errors"
	"net/http"
	"strings"

	"cadguanliq/internal/http/middleware"
	"cadguanliq/internal/response"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type attributeCommand struct {
	Name      string                  `json:"name"`
	Required  bool                    `json:"required"`
	Enabled   bool                    `json:"enabled"`
	SortOrder int                     `json:"sortOrder"`
	Fields    []attributeFieldCommand `json:"fields"`
}

type attributeFieldCommand struct {
	Name      string `json:"name"`
	Enabled   bool   `json:"enabled"`
	SortOrder int    `json:"sortOrder"`
}

func DrawingAttributes(pool *pgxpool.Pool) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		user, ok := middleware.UserFromContext(request.Context())
		if !ok {
			response.WriteError(writer, http.StatusUnauthorized, "登录已失效，请重新登录")
			return
		}
		path := strings.Trim(strings.TrimPrefix(request.URL.Path, "/api/drawing-attributes"), "/")
		if path == "" {
			switch request.Method {
			case http.MethodGet:
				list, err := listDrawingAttributes(request, pool)
				if err != nil {
					response.WriteError(writer, http.StatusInternalServerError, "属性列表读取失败")
					return
				}
				response.WriteData(writer, http.StatusOK, list)
			case http.MethodPost:
				var input attributeCommand
				if err := decodeJSON(request, &input); err != nil || strings.TrimSpace(input.Name) == "" {
					response.WriteError(writer, http.StatusBadRequest, "属性参数无效")
					return
				}
				item, err := createDrawingAttribute(request, pool, input, user.ID)
				if err != nil {
					writeAttributeError(writer, err)
					return
				}
				response.WriteData(writer, http.StatusCreated, item)
			default:
				response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
			}
			return
		}
		parts := strings.Split(path, "/")
		if len(parts) == 1 && request.Method == http.MethodPatch {
			var input attributeCommand
			if err := decodeJSON(request, &input); err != nil || strings.TrimSpace(input.Name) == "" {
				response.WriteError(writer, http.StatusBadRequest, "属性参数无效")
				return
			}
			item, err := updateDrawingAttribute(request, pool, parts[0], input, user.ID)
			if err != nil {
				writeAttributeError(writer, err)
				return
			}
			response.WriteData(writer, http.StatusOK, item)
			return
		}
		if len(parts) == 1 && request.Method == http.MethodDelete {
			if err := deleteDrawingAttribute(request, pool, parts[0], user.ID); err != nil {
				writeAttributeError(writer, err)
				return
			}
			response.WriteData(writer, http.StatusOK, map[string]string{"id": parts[0]})
			return
		}
			if len(parts) == 2 && parts[1] == "fields" && request.Method == http.MethodPost {
			var input attributeFieldCommand
			if err := decodeJSON(request, &input); err != nil || strings.TrimSpace(input.Name) == "" {
				response.WriteError(writer, http.StatusBadRequest, "属性字段参数无效")
				return
			}
			var id string
			err := pool.QueryRow(request.Context(), `INSERT INTO drawing_attribute_fields (attribute_id, name, enabled, sort_order) VALUES ($1::uuid, $2, $3, $4) RETURNING id::text`, parts[0], input.Name, input.Enabled, input.SortOrder).Scan(&id)
			if err != nil {
				writeAttributeError(writer, err)
				return
			}
			response.WriteData(writer, http.StatusCreated, map[string]any{"id": id, "attributeId": parts[0], "name": input.Name, "enabled": input.Enabled, "sortOrder": input.SortOrder})
				return
			}
			if len(parts) == 3 && parts[1] == "fields" {
				fieldID := parts[2]
				switch request.Method {
				case http.MethodPatch:
					var input attributeFieldCommand
					if err := decodeJSON(request, &input); err != nil || strings.TrimSpace(input.Name) == "" {
						response.WriteError(writer, http.StatusBadRequest, "属性字段参数无效")
						return
					}
					result, err := pool.Exec(request.Context(), `UPDATE drawing_attribute_fields SET name = $3, enabled = $4, sort_order = $5 WHERE id = $1::uuid AND attribute_id = $2::uuid`, fieldID, parts[0], strings.TrimSpace(input.Name), input.Enabled, input.SortOrder)
					if err != nil {
						writeAttributeError(writer, err)
						return
					}
					if result.RowsAffected() == 0 {
						writeAttributeError(writer, pgx.ErrNoRows)
						return
					}
					response.WriteData(writer, http.StatusOK, map[string]any{"id": fieldID, "attributeId": parts[0], "name": strings.TrimSpace(input.Name), "enabled": input.Enabled, "sortOrder": input.SortOrder})
				case http.MethodDelete:
					result, err := pool.Exec(request.Context(), `DELETE FROM drawing_attribute_fields WHERE id = $1::uuid AND attribute_id = $2::uuid`, fieldID, parts[0])
					if err != nil {
						writeAttributeError(writer, err)
						return
					}
					if result.RowsAffected() == 0 {
						writeAttributeError(writer, pgx.ErrNoRows)
						return
					}
					response.WriteData(writer, http.StatusOK, map[string]string{"id": fieldID})
				default:
					response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
				}
				return
			}
			response.WriteError(writer, http.StatusNotFound, "属性接口不存在")
	})
}

func listDrawingAttributes(request *http.Request, pool *pgxpool.Pool) ([]map[string]any, error) {
	rows, err := pool.Query(request.Context(), `SELECT id::text, name, required, enabled, sort_order FROM drawing_attributes ORDER BY sort_order, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, name string
		var required, enabled bool
		var sortOrder int
		if err := rows.Scan(&id, &name, &required, &enabled, &sortOrder); err != nil {
			return nil, err
		}
		fields, err := listAttributeFields(request, pool, id)
		if err != nil {
			return nil, err
		}
		items = append(items, map[string]any{"id": id, "name": name, "required": required, "enabled": enabled, "sortOrder": sortOrder, "fields": fields})
	}
	return items, rows.Err()
}

func listAttributeFields(request *http.Request, pool *pgxpool.Pool, attributeID string) ([]map[string]any, error) {
	rows, err := pool.Query(request.Context(), `SELECT id::text, name, enabled, sort_order FROM drawing_attribute_fields WHERE attribute_id = $1::uuid ORDER BY sort_order, name`, attributeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, name string
		var enabled bool
		var sortOrder int
		if err := rows.Scan(&id, &name, &enabled, &sortOrder); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{"id": id, "name": name, "enabled": enabled, "sortOrder": sortOrder})
	}
	return items, rows.Err()
}

func createDrawingAttribute(request *http.Request, pool *pgxpool.Pool, input attributeCommand, userID string) (map[string]any, error) {
	tx, err := pool.Begin(request.Context())
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(request.Context())
	var id string
	if err := tx.QueryRow(request.Context(), `INSERT INTO drawing_attributes (name, required, enabled, sort_order) VALUES ($1, $2, $3, $4) RETURNING id::text`, strings.TrimSpace(input.Name), input.Required, input.Enabled, input.SortOrder).Scan(&id); err != nil {
		return nil, err
	}
	for _, field := range input.Fields {
		if strings.TrimSpace(field.Name) == "" {
			continue
		}
		if _, err := tx.Exec(request.Context(), `INSERT INTO drawing_attribute_fields (attribute_id, name, enabled, sort_order) VALUES ($1::uuid, $2, $3, $4)`, id, strings.TrimSpace(field.Name), field.Enabled, field.SortOrder); err != nil {
			return nil, err
		}
	}
	if _, err := tx.Exec(request.Context(), `INSERT INTO audit_logs (actor_id, action, resource_type, resource_id, summary) VALUES ($1::uuid, 'create', 'drawing_attribute', $2::uuid, '创建图纸属性')`, userID, id); err != nil {
		return nil, err
	}
	if err := tx.Commit(request.Context()); err != nil {
		return nil, err
	}
	return map[string]any{"id": id, "name": input.Name, "required": input.Required, "enabled": input.Enabled, "sortOrder": input.SortOrder}, nil
}

func updateDrawingAttribute(request *http.Request, pool *pgxpool.Pool, id string, input attributeCommand, userID string) (map[string]any, error) {
	result, err := pool.Exec(request.Context(), `UPDATE drawing_attributes SET name = $2, required = $3, enabled = $4, sort_order = $5, updated_at = now() WHERE id = $1::uuid`, id, strings.TrimSpace(input.Name), input.Required, input.Enabled, input.SortOrder)
	if err != nil {
		return nil, err
	}
	if result.RowsAffected() == 0 {
		return nil, pgx.ErrNoRows
	}
	_, err = pool.Exec(request.Context(), `INSERT INTO audit_logs (actor_id, action, resource_type, resource_id, summary) VALUES ($1::uuid, 'update', 'drawing_attribute', $2::uuid, '修改图纸属性')`, userID, id)
	if err != nil {
		return nil, err
	}
	return map[string]any{"id": id, "name": input.Name, "required": input.Required, "enabled": input.Enabled, "sortOrder": input.SortOrder}, nil
}

func deleteDrawingAttribute(request *http.Request, pool *pgxpool.Pool, id, userID string) error {
	result, err := pool.Exec(request.Context(), `DELETE FROM drawing_attributes WHERE id = $1::uuid`, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	_, err = pool.Exec(request.Context(), `INSERT INTO audit_logs (actor_id, action, resource_type, resource_id, summary) VALUES ($1::uuid, 'delete', 'drawing_attribute', $2::uuid, '删除图纸属性')`, userID, id)
	return err
}

func writeAttributeError(writer http.ResponseWriter, err error) {
	if errors.Is(err, pgx.ErrNoRows) {
		response.WriteError(writer, http.StatusNotFound, "属性不存在")
		return
	}
	if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
		response.WriteError(writer, http.StatusConflict, "属性名称已存在")
		return
	}
	response.WriteError(writer, http.StatusInternalServerError, "属性操作失败")
}
