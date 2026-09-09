package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"cadguanliq/internal/audit"
	"cadguanliq/internal/auth"
	"cadguanliq/internal/editing"
	"cadguanliq/internal/http/middleware"
	"cadguanliq/internal/response"
	"cadguanliq/internal/storage"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AdminDrawingSummary struct {
	ID              string `json:"id"`
	No              string `json:"no"`
	Name            string `json:"name"`
	Kind            string `json:"kind"`
	Project         string `json:"project"`
	Material        string `json:"material"`
	Vendor          string `json:"vendor"`
	Status          string `json:"status"`
	Version         string `json:"version"`
	CreatedBy       string `json:"createdBy"`
	CreatedAt       string `json:"createdAt"`
	UpdatedBy       string `json:"updatedBy"`
	UpdatedAt       string `json:"updatedAt"`
	PartCount       int    `json:"partCount"`
	AttachmentCount int    `json:"attachmentCount"`
	SessionCount    int    `json:"sessionCount"`
}

type AdminPartSummary struct {
	ID              string `json:"id"`
	DrawingID       string `json:"drawingId"`
	DrawingNo       string `json:"drawingNo"`
	No              string `json:"no"`
	Name            string `json:"name"`
	ParentNo        string `json:"parentNo"`
	Project         string `json:"project"`
	Material        string `json:"material"`
	Status          string `json:"status"`
	Version         string `json:"version"`
	CreatedBy       string `json:"createdBy"`
	CreatedAt       string `json:"createdAt"`
	UpdatedBy       string `json:"updatedBy"`
	UpdatedAt       string `json:"updatedAt"`
	AttachmentCount int    `json:"attachmentCount"`
	SessionCount    int    `json:"sessionCount"`
}

type AdminDrawingPage struct {
	List     []AdminDrawingSummary `json:"list"`
	Total    int                   `json:"total"`
	Page     int                   `json:"page"`
	PageSize int                   `json:"pageSize"`
}

type AdminPartPage struct {
	List     []AdminPartSummary `json:"list"`
	Total    int                `json:"total"`
	Page     int                `json:"page"`
	PageSize int                `json:"pageSize"`
}

type AdminAttachment struct {
	ID                string `json:"id"`
	OriginalName      string `json:"originalName"`
	CurrentName       string `json:"currentName"`
	Role              string `json:"role"`
	StorageKey        string `json:"storageKey"`
	CurrentStorageKey string `json:"currentStorageKey"`
	Version           string `json:"version"`
	Size              int64  `json:"size"`
	MimeType          string `json:"mimeType"`
	UploadedBy        string `json:"uploadedBy"`
	CreatedAt         string `json:"createdAt"`
}

type AdminFileVersion struct {
	ID           string    `json:"id"`
	AttachmentID string    `json:"attachmentId"`
	StorageKey   string    `json:"storageKey"`
	Version      string    `json:"version"`
	VersionKind  string    `json:"versionKind"`
	Size         int64     `json:"size"`
	MimeType     string    `json:"mimeType"`
	CreatedBy    string    `json:"createdBy"`
	CreatedAt    time.Time `json:"createdAt"`
}

type AdminDrawingDetail struct {
	Drawing     AdminDrawingSummary `json:"drawing"`
	Parts       []AdminPartSummary  `json:"parts"`
	Attachments []AdminAttachment   `json:"attachments"`
	Versions    []AdminFileVersion  `json:"versions"`
	Sessions    []map[string]any    `json:"sessions"`
}

func AdminDrawings(pool *pgxpool.Pool) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if _, ok := middleware.UserFromContext(request.Context()); !ok {
			response.WriteError(writer, http.StatusUnauthorized, "登录已失效，请重新登录")
			return
		}
		if request.Method != http.MethodGet {
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		page, err := listAdminDrawings(request.Context(), pool, request)
		if err != nil {
			response.WriteError(writer, http.StatusInternalServerError, "后台图纸列表读取失败")
			return
		}
		response.WriteData(writer, http.StatusOK, page)
	}
}

func AdminDrawingResource(pool *pgxpool.Pool, objectStorage storage.ObjectStorage, auditRepository audit.Repository) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		user, ok := middleware.UserFromContext(request.Context())
		if !ok {
			response.WriteError(writer, http.StatusUnauthorized, "登录已失效，请重新登录")
			return
		}
		id, action, err := adminResourceParts(request.URL.Path, "/api/admin/drawings/")
		if err != nil {
			response.WriteError(writer, http.StatusBadRequest, err.Error())
			return
		}
		switch {
		case request.Method == http.MethodGet && action == "":
			detail, findErr := findAdminDrawing(request.Context(), pool, id)
			if findErr != nil {
				writeAdminDrawingError(writer, findErr)
				return
			}
			response.WriteData(writer, http.StatusOK, detail)
		case request.Method == http.MethodPost && (action == "disable" || action == "enable"):
			var no string
			var status string
			// 存档图纸处于变更管控中，禁止通过「停用→启用为草稿」绕过工单修改正式成果。
			var curStatus string
			findErr := pool.QueryRow(request.Context(), `SELECT status FROM drawings WHERE id = $1::uuid`, id).Scan(&curStatus)
			if errors.Is(findErr, pgx.ErrNoRows) {
				writeAdminDrawingError(writer, ErrAdminDrawingNotFound)
				return
			}
			if findErr != nil {
				response.WriteError(writer, http.StatusInternalServerError, "图纸状态读取失败")
				return
			}
			if curStatus == "archived" {
				response.WriteError(writer, http.StatusConflict, "图纸已存档，如需修改请通过变更工单，不能直接停用/启用")
				return
			}
			if action == "disable" {
				err = pool.QueryRow(request.Context(), `UPDATE drawings SET status = 'disabled', updated_by = $2::uuid, updated_at = now(), revision = revision + 1 WHERE id = $1::uuid RETURNING drawing_no, status`, id, user.ID).Scan(&no, &status)
			} else {
				err = pool.QueryRow(request.Context(), `UPDATE drawings SET status = 'draft', updated_by = $2::uuid, updated_at = now(), revision = revision + 1 WHERE id = $1::uuid AND status = 'disabled' RETURNING drawing_no, status`, id, user.ID).Scan(&no, &status)
			}
			if errors.Is(err, pgx.ErrNoRows) {
				writeAdminDrawingError(writer, ErrAdminDrawingNotFound)
				return
			}
			if err != nil {
				response.WriteError(writer, http.StatusInternalServerError, "图纸状态更新失败")
				return
			}
			writeAdminAudit(request.Context(), auditRepository, user, audit.CreateInput{DrawingNo: no, TargetType: "drawing", Action: "edit", Summary: actionLabel(action) + "图纸", Result: "success", Detail: map[string]any{"status": status}})
			response.WriteData(writer, http.StatusOK, map[string]string{"id": id, "status": status})
		case request.Method == http.MethodDelete && action == "":
			result, deleteErr := hardDeleteDrawing(request.Context(), pool, objectStorage, id)
			if deleteErr != nil {
				writeAdminDrawingError(writer, deleteErr)
				return
			}
			writeAdminAudit(request.Context(), auditRepository, user, audit.CreateInput{DrawingNo: result, TargetType: "drawing", Action: "delete", Summary: "永久删除图纸「" + result + "」", Result: "success"})
			response.WriteData(writer, http.StatusOK, map[string]string{"no": result})
		default:
			response.WriteError(writer, http.StatusNotFound, "后台图纸接口不存在")
		}
	}
}

func AdminPartResource(pool *pgxpool.Pool, objectStorage storage.ObjectStorage, auditRepository audit.Repository) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		user, ok := middleware.UserFromContext(request.Context())
		if !ok {
			response.WriteError(writer, http.StatusUnauthorized, "登录已失效，请重新登录")
			return
		}
		id, action, err := adminResourceParts(request.URL.Path, "/api/admin/parts/")
		if err != nil {
			response.WriteError(writer, http.StatusBadRequest, err.Error())
			return
		}
		if request.Method != http.MethodPost || (action != "disable" && action != "enable") {
			if request.Method == http.MethodDelete && action == "" {
				no, drawingNo, keys, deleteErr := hardDeletePart(request.Context(), pool, objectStorage, id)
				if deleteErr != nil {
					writeAdminDrawingError(writer, deleteErr)
					return
				}
				writeAdminAudit(request.Context(), auditRepository, user, audit.CreateInput{DrawingNo: drawingNo, TargetType: "part", Action: "delete", Summary: "永久删除零件「" + no + "」", Result: "success", Detail: map[string]any{"partNo": no, "storageKeys": keys}})
				response.WriteData(writer, http.StatusOK, map[string]string{"no": no})
				return
			}
			response.WriteError(writer, http.StatusNotFound, "后台零件接口不存在")
			return
		}
		var no string
		var status string
		if action == "disable" {
			err = pool.QueryRow(request.Context(), `UPDATE parts SET lifecycle_status = 'archived', updated_by = $2::uuid, updated_at = now() WHERE id = $1::uuid RETURNING part_no, lifecycle_status`, id, user.ID).Scan(&no, &status)
		} else {
			err = pool.QueryRow(request.Context(), `UPDATE parts SET lifecycle_status = 'active', updated_by = $2::uuid, updated_at = now() WHERE id = $1::uuid AND lifecycle_status = 'archived' RETURNING part_no, lifecycle_status`, id, user.ID).Scan(&no, &status)
		}
		if errors.Is(err, pgx.ErrNoRows) {
			writeAdminDrawingError(writer, ErrAdminDrawingNotFound)
			return
		}
		if err != nil {
			response.WriteError(writer, http.StatusInternalServerError, "零件状态更新失败")
			return
		}
		writeAdminAudit(request.Context(), auditRepository, user, audit.CreateInput{DrawingNo: no, TargetType: "part", Action: "edit", Summary: actionLabel(action) + "零件", Result: "success", Detail: map[string]any{"status": status}})
		response.WriteData(writer, http.StatusOK, map[string]string{"id": id, "status": status})
	}
}

func AdminAttachmentResource(pool *pgxpool.Pool, objectStorage storage.ObjectStorage, auditRepository audit.Repository) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		user, ok := middleware.UserFromContext(request.Context())
		if !ok {
			response.WriteError(writer, http.StatusUnauthorized, "登录已失效，请重新登录")
			return
		}
		id, action, err := adminResourceParts(request.URL.Path, "/api/admin/attachments/")
		if err != nil {
			response.WriteError(writer, http.StatusNotFound, "后台附件接口不存在")
			return
		}
		switch {
		case action == "" && request.Method == http.MethodDelete:
			no, name, keys, err := hardDeleteAttachment(request.Context(), pool, objectStorage, id)
			if err != nil {
				writeAdminDrawingError(writer, err)
				return
			}
			writeAdminAudit(request.Context(), auditRepository, user, audit.CreateInput{DrawingNo: no, TargetType: "file", Action: "delete", Summary: "永久删除附件「" + name + "」", Result: "success", Detail: map[string]any{"attachmentId": id, "storageKeys": keys}})
			response.WriteData(writer, http.StatusOK, map[string]string{"id": id, "name": name})
		case action == "reconvert" && request.Method == http.MethodPost:
			no, name, err := requeueAttachmentConversion(request.Context(), pool, id)
			if err != nil {
				writeAdminDrawingError(writer, err)
				return
			}
			writeAdminAudit(request.Context(), auditRepository, user, audit.CreateInput{DrawingNo: no, TargetType: "file", Action: "convert", Summary: "重新转换附件「" + name + "」", Result: "success", Detail: map[string]any{"attachmentId": id}})
			response.WriteData(writer, http.StatusOK, map[string]string{"id": id, "name": name})
		default:
			response.WriteError(writer, http.StatusNotFound, "后台附件接口不存在")
		}
	}
}

func AdminEditSessionResource(service *editing.Service, auditRepository audit.Repository) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		user, ok := middleware.UserFromContext(request.Context())
		if !ok {
			response.WriteError(writer, http.StatusUnauthorized, "登录已失效，请重新登录")
			return
		}
		id, action, err := adminResourceParts(request.URL.Path, "/api/admin/edit-sessions/")
		if err != nil || action != "close" || request.Method != http.MethodPost {
			response.WriteError(writer, http.StatusNotFound, "后台编辑会话接口不存在")
			return
		}
		result, err := service.Close(request.Context(), user, id)
		if err != nil {
			writeEditingError(writer, err)
			return
		}
		writeAdminAudit(request.Context(), auditRepository, user, audit.CreateInput{TargetType: "file", Action: "edit", Summary: "管理员强制结束编辑会话", Result: "success", Detail: map[string]any{"sessionId": id, "changed": result.Changed, "version": result.Version}})
		response.WriteData(writer, http.StatusOK, result)
	})
}

var ErrAdminDrawingNotFound = errors.New("后台图纸不存在")
var ErrAdminDrawingBusy = errors.New("图纸存在活动编辑会话，请先结束编辑后再删除")
var ErrAdminPartHasChildren = errors.New("该零件存在子零件，请先处理子零件后再删除")
var ErrAdminAttachmentNoJob = errors.New("该附件没有转换队列任务")

func listAdminDrawings(ctx context.Context, pool *pgxpool.Pool, request *http.Request) (any, error) {
	page := parseAdminInt(request.URL.Query().Get("page"), 1)
	pageSize := parseAdminInt(request.URL.Query().Get("page_size"), 20)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	kind := strings.TrimSpace(request.URL.Query().Get("kind"))
	if kind == "part" {
		return listAdminParts(ctx, pool, request, page, pageSize)
	}
	keyword := "%" + strings.ToLower(strings.TrimSpace(request.URL.Query().Get("keyword"))) + "%"
	status := strings.TrimSpace(request.URL.Query().Get("status"))
	var total int
	where := `WHERE ($1 = '%' OR lower(d.drawing_no) LIKE $1 OR lower(d.name) LIKE $1 OR lower(d.project) LIKE $1)
		AND ($2 = '' OR d.status = $2)`
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM drawings d `+where, keyword, status).Scan(&total); err != nil {
		return nil, err
	}
	offset := (page - 1) * pageSize
	rows, err := pool.Query(ctx, `SELECT d.id::text, d.drawing_no, d.name, d.kind, d.project, d.material, d.vendor, d.status, d.version,
		COALESCE(c.display_name, c.account, ''), d.created_at, COALESCE(u.display_name, u.account, ''), d.updated_at,
		(SELECT count(*) FROM drawing_part_relations r WHERE r.drawing_id = d.id AND r.status = 'active'),
		(SELECT count(*) FROM attachments a WHERE a.drawing_id = d.id AND a.deleted_at IS NULL),
		(SELECT count(*) FROM edit_sessions s JOIN attachments a ON a.id = s.attachment_id WHERE a.drawing_id = d.id AND s.status = 'active')
		FROM drawings d LEFT JOIN users c ON c.id = d.created_by LEFT JOIN users u ON u.id = d.updated_by `+where+` ORDER BY d.updated_at DESC, d.drawing_no LIMIT $3 OFFSET $4`, keyword, status, pageSize, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]AdminDrawingSummary, 0)
	for rows.Next() {
		var item AdminDrawingSummary
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&item.ID, &item.No, &item.Name, &item.Kind, &item.Project, &item.Material, &item.Vendor, &item.Status, &item.Version, &item.CreatedBy, &createdAt, &item.UpdatedBy, &updatedAt, &item.PartCount, &item.AttachmentCount, &item.SessionCount); err != nil {
			return nil, err
		}
		item.CreatedAt = createdAt.Format(time.RFC3339)
		item.UpdatedAt = updatedAt.Format(time.RFC3339)
		items = append(items, item)
	}
	return AdminDrawingPage{List: items, Total: total, Page: page, PageSize: pageSize}, rows.Err()
}

func listAdminParts(ctx context.Context, pool *pgxpool.Pool, request *http.Request, page, pageSize int) (AdminPartPage, error) {
	keyword := "%" + strings.ToLower(strings.TrimSpace(request.URL.Query().Get("keyword"))) + "%"
	status := strings.TrimSpace(request.URL.Query().Get("status"))
	where := `WHERE ($1 = '%' OR lower(p.part_no) LIKE $1 OR lower(pr.name) LIKE $1 OR lower(d.drawing_no) LIKE $1)
		AND ($2 = '' OR p.lifecycle_status = $2)`
	var total int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM parts p JOIN drawing_part_relations r ON r.part_id = p.id AND r.status = 'active' JOIN drawings d ON d.id = r.drawing_id LEFT JOIN LATERAL (SELECT * FROM part_revisions x WHERE x.part_id = p.id ORDER BY x.revision_no DESC LIMIT 1) pr ON true `+where, keyword, status).Scan(&total); err != nil {
		return AdminPartPage{}, err
	}
	rows, err := pool.Query(ctx, `SELECT p.id::text, r.drawing_id::text, d.drawing_no, p.part_no, pr.name, COALESCE(parentPart.part_no, ''), d.project, pr.material, p.lifecycle_status, pr.version,
		COALESCE(c.display_name, c.account, ''), p.created_at, COALESCE(u.display_name, u.account, ''), p.updated_at,
		(SELECT count(*) FROM attachments a WHERE a.part_id = p.id AND a.deleted_at IS NULL),
		(SELECT count(*) FROM edit_sessions s JOIN attachments a ON a.id = s.attachment_id WHERE a.part_id = p.id AND s.status = 'active')
		FROM parts p JOIN drawing_part_relations r ON r.part_id = p.id AND r.status = 'active' JOIN drawings d ON d.id = r.drawing_id LEFT JOIN part_revisions pr ON pr.id = COALESCE(p.published_revision_id, (SELECT x.id FROM part_revisions x WHERE x.part_id = p.id ORDER BY x.revision_no DESC LIMIT 1)) LEFT JOIN drawing_part_relations parentRelation ON parentRelation.id = r.parent_relation_id LEFT JOIN parts parentPart ON parentPart.id = parentRelation.part_id LEFT JOIN users c ON c.id = p.created_by LEFT JOIN users u ON u.id = p.updated_by `+where+` ORDER BY p.updated_at DESC, p.part_no LIMIT $3 OFFSET $4`, keyword, status, pageSize, (page-1)*pageSize)
	if err != nil {
		return AdminPartPage{}, err
	}
	defer rows.Close()
	items := make([]AdminPartSummary, 0)
	for rows.Next() {
		var item AdminPartSummary
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&item.ID, &item.DrawingID, &item.DrawingNo, &item.No, &item.Name, &item.ParentNo, &item.Project, &item.Material, &item.Status, &item.Version, &item.CreatedBy, &createdAt, &item.UpdatedBy, &updatedAt, &item.AttachmentCount, &item.SessionCount); err != nil {
			return AdminPartPage{}, err
		}
		item.CreatedAt = createdAt.Format(time.RFC3339)
		item.UpdatedAt = updatedAt.Format(time.RFC3339)
		items = append(items, item)
	}
	return AdminPartPage{List: items, Total: total, Page: page, PageSize: pageSize}, rows.Err()
}

func findAdminDrawing(ctx context.Context, pool *pgxpool.Pool, id string) (AdminDrawingDetail, error) {
	var item AdminDrawingSummary
	var createdAt, updatedAt time.Time
	err := pool.QueryRow(ctx, `SELECT d.id::text, d.drawing_no, d.name, d.kind, d.project, d.material, d.vendor, d.status, d.version, COALESCE(c.display_name, c.account, ''), d.created_at, COALESCE(u.display_name, u.account, ''), d.updated_at,
		(SELECT count(*) FROM drawing_part_relations r WHERE r.drawing_id = d.id AND r.status = 'active'), (SELECT count(*) FROM attachments a WHERE a.drawing_id = d.id AND a.deleted_at IS NULL), (SELECT count(*) FROM edit_sessions s JOIN attachments a ON a.id = s.attachment_id WHERE a.drawing_id = d.id AND s.status = 'active')
		FROM drawings d LEFT JOIN users c ON c.id = d.created_by LEFT JOIN users u ON u.id = d.updated_by WHERE d.id = $1::uuid`, id).Scan(&item.ID, &item.No, &item.Name, &item.Kind, &item.Project, &item.Material, &item.Vendor, &item.Status, &item.Version, &item.CreatedBy, &createdAt, &item.UpdatedBy, &updatedAt, &item.PartCount, &item.AttachmentCount, &item.SessionCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return AdminDrawingDetail{}, ErrAdminDrawingNotFound
	}
	if err != nil {
		return AdminDrawingDetail{}, err
	}
	item.CreatedAt = createdAt.Format(time.RFC3339)
	item.UpdatedAt = updatedAt.Format(time.RFC3339)
	parts, err := listAllAdminParts(ctx, pool, id)
	if err != nil {
		return AdminDrawingDetail{}, err
	}
	attachments, err := listAdminAttachments(ctx, pool, id)
	if err != nil {
		return AdminDrawingDetail{}, err
	}
	versions, err := listAdminVersions(ctx, pool, attachments)
	if err != nil {
		return AdminDrawingDetail{}, err
	}
	sessions, err := listAdminSessions(ctx, pool, item.No)
	if err != nil {
		return AdminDrawingDetail{}, err
	}
	return AdminDrawingDetail{Drawing: item, Parts: parts, Attachments: attachments, Versions: versions, Sessions: sessions}, nil
}

func listAllAdminParts(ctx context.Context, pool *pgxpool.Pool, drawingID string) ([]AdminPartSummary, error) {
	return queryAllAdminParts(ctx, pool, drawingID)
}

func queryAllAdminParts(ctx context.Context, pool *pgxpool.Pool, drawingID string) ([]AdminPartSummary, error) {
	rows, err := pool.Query(ctx, `SELECT p.id::text, r.drawing_id::text, d.drawing_no, p.part_no, pr.name, COALESCE(parentPart.part_no, ''), d.project, pr.material, p.lifecycle_status, pr.version, COALESCE(c.display_name, c.account, ''), p.created_at, COALESCE(u.display_name, u.account, ''), p.updated_at, (SELECT count(*) FROM attachments a WHERE a.part_id = p.id AND a.deleted_at IS NULL), (SELECT count(*) FROM edit_sessions s JOIN attachments a ON a.id = s.attachment_id WHERE a.part_id = p.id AND s.status = 'active') FROM parts p JOIN drawing_part_relations r ON r.part_id = p.id AND r.drawing_id = $1::uuid AND r.status = 'active' JOIN drawings d ON d.id = r.drawing_id LEFT JOIN part_revisions pr ON pr.id = COALESCE(p.published_revision_id, (SELECT x.id FROM part_revisions x WHERE x.part_id = p.id ORDER BY x.revision_no DESC LIMIT 1)) LEFT JOIN drawing_part_relations parentRelation ON parentRelation.id = r.parent_relation_id LEFT JOIN parts parentPart ON parentPart.id = parentRelation.part_id LEFT JOIN users c ON c.id = p.created_by LEFT JOIN users u ON u.id = p.updated_by ORDER BY p.part_no`, drawingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]AdminPartSummary, 0)
	for rows.Next() {
		var item AdminPartSummary
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&item.ID, &item.DrawingID, &item.DrawingNo, &item.No, &item.Name, &item.ParentNo, &item.Project, &item.Material, &item.Status, &item.Version, &item.CreatedBy, &createdAt, &item.UpdatedBy, &updatedAt, &item.AttachmentCount, &item.SessionCount); err != nil {
			return nil, err
		}
		item.CreatedAt = createdAt.Format(time.RFC3339)
		item.UpdatedAt = updatedAt.Format(time.RFC3339)
		items = append(items, item)
	}
	return items, rows.Err()
}

func listAdminAttachments(ctx context.Context, pool *pgxpool.Pool, drawingID string) ([]AdminAttachment, error) {
	rows, err := pool.Query(ctx, `SELECT a.id::text, a.logical_name, COALESCE(v.original_name, a.logical_name), a.file_role, COALESCE(b.storage_key, ''), COALESCE(b.storage_key, ''), COALESCE(v.version, 'v1.0'), COALESCE(v.size_bytes, 0), COALESCE(v.mime_type, b.mime_type, 'application/octet-stream'), COALESCE(u.display_name, u.account, ''), a.created_at FROM attachments a LEFT JOIN attachment_versions v ON v.id = a.current_version_id LEFT JOIN file_blobs b ON b.id = v.blob_id LEFT JOIN users u ON u.id = a.uploaded_by WHERE (a.drawing_id = $1::uuid OR a.part_id IN (SELECT r.part_id FROM drawing_part_relations r WHERE r.drawing_id = $1::uuid AND r.status = 'active')) AND a.deleted_at IS NULL ORDER BY a.created_at DESC`, drawingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]AdminAttachment, 0)
	for rows.Next() {
		var item AdminAttachment
		var createdAt time.Time
		if err := rows.Scan(&item.ID, &item.OriginalName, &item.CurrentName, &item.Role, &item.StorageKey, &item.CurrentStorageKey, &item.Version, &item.Size, &item.MimeType, &item.UploadedBy, &createdAt); err != nil {
			return nil, err
		}
		item.CreatedAt = createdAt.Format(time.RFC3339)
		items = append(items, item)
	}
	return items, rows.Err()
}

func listAdminVersions(ctx context.Context, pool *pgxpool.Pool, attachments []AdminAttachment) ([]AdminFileVersion, error) {
	if len(attachments) == 0 {
		return []AdminFileVersion{}, nil
	}
	rows, err := pool.Query(ctx, `SELECT v.id::text, v.attachment_id::text, b.storage_key, v.version, v.version_kind, v.size_bytes, v.mime_type, COALESCE(u.display_name, u.account, ''), v.created_at FROM attachment_versions v JOIN file_blobs b ON b.id = v.blob_id LEFT JOIN users u ON u.id = v.created_by WHERE v.attachment_id = ANY($1::uuid[]) AND v.deleted_at IS NULL ORDER BY v.created_at DESC`, attachmentIDs(attachments))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]AdminFileVersion, 0)
	for rows.Next() {
		var item AdminFileVersion
		if err := rows.Scan(&item.ID, &item.AttachmentID, &item.StorageKey, &item.Version, &item.VersionKind, &item.Size, &item.MimeType, &item.CreatedBy, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func listAdminSessions(ctx context.Context, pool *pgxpool.Pool, drawingNo string) ([]map[string]any, error) {
	rows, err := pool.Query(ctx, `SELECT s.id::text, s.attachment_id::text, COALESCE(v.original_name, a.logical_name, ''), COALESCE(p.part_no, ''), COALESCE(u.display_name, u.account, ''), s.status, s.started_at, s.last_seen_at FROM edit_sessions s JOIN attachments a ON a.id = s.attachment_id LEFT JOIN attachment_versions v ON v.id = a.current_version_id LEFT JOIN parts p ON p.id = a.part_id JOIN users u ON u.id = s.user_id LEFT JOIN drawings d ON d.id = a.drawing_id LEFT JOIN drawing_part_relations ownerRelation ON ownerRelation.part_id = p.id AND ownerRelation.relation_type = 'owned' AND ownerRelation.status = 'active' LEFT JOIN drawings parent ON parent.id = ownerRelation.drawing_id WHERE s.status = 'active' AND (d.drawing_no = $1 OR parent.drawing_no = $1) ORDER BY s.started_at DESC`, drawingNo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, attachmentID, name, partNo, user, status string
		var started, seen time.Time
		if err := rows.Scan(&id, &attachmentID, &name, &partNo, &user, &status, &started, &seen); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{"id": id, "attachmentId": attachmentID, "fileName": name, "partNo": partNo, "user": user, "status": status, "startedAt": started, "lastSeenAt": seen})
	}
	return items, rows.Err()
}

func hardDeleteDrawing(ctx context.Context, pool *pgxpool.Pool, objectStorage storage.ObjectStorage, id string) (string, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	var no string
	if err := tx.QueryRow(ctx, `SELECT drawing_no FROM drawings WHERE id = $1::uuid FOR UPDATE`, id).Scan(&no); errors.Is(err, pgx.ErrNoRows) {
		return "", ErrAdminDrawingNotFound
	} else if err != nil {
		return "", err
	}
	keys := make([]string, 0)
	relatedPartIDs := make([]string, 0)
	// 收集本图纸引用的全部零件（含借用）：关系随图纸级联删除后，
	// 不再被任何图纸引用的零件在此一并清理，避免孤儿零件占用图号。
	partRows, err := tx.Query(ctx, `SELECT DISTINCT part_id::text FROM drawing_part_relations WHERE drawing_id = $1::uuid`, id)
	if err != nil {
		return "", err
	}
	for partRows.Next() {
		var partID string
		if err := partRows.Scan(&partID); err != nil {
			partRows.Close()
			return "", err
		}
		relatedPartIDs = append(relatedPartIDs, partID)
	}
	if err := partRows.Err(); err != nil {
		partRows.Close()
		return "", err
	}
	partRows.Close()
	rows, err := tx.Query(ctx, `SELECT b.storage_key FROM attachment_versions v JOIN file_blobs b ON b.id = v.blob_id JOIN attachments a ON a.id = v.attachment_id WHERE a.drawing_id = $1::uuid OR a.part_id IN (SELECT r.part_id FROM drawing_part_relations r WHERE r.drawing_id = $1::uuid)`, id)
	if err != nil {
		return "", err
	}
	for rows.Next() {
		var original string
		if err := rows.Scan(&original); err != nil {
			rows.Close()
			return "", err
		}
		keys = append(keys, original)
	}
	rows.Close()
	// 管理员删除是强制操作：丢弃未提交的本地编辑并释放会话，
	// 不等待 CAXA，也不让活动编辑锁阻塞图纸删除。
	if _, err := tx.Exec(ctx, `
		UPDATE edit_sessions s
		SET status = 'closed', closed_at = now(), last_seen_at = now()
		FROM attachments a
		LEFT JOIN drawing_part_relations ownerRelation ON ownerRelation.part_id = a.part_id AND ownerRelation.relation_type = 'owned' AND ownerRelation.status = 'active'
		WHERE s.attachment_id = a.id
		  AND s.status = 'active'
		  AND (a.drawing_id = $1::uuid OR ownerRelation.drawing_id = $1::uuid)`, id); err != nil {
		return "", fmt.Errorf("释放图纸编辑会话失败: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM drawings WHERE id = $1::uuid`, id); err != nil {
		return "", err
	}
	if len(relatedPartIDs) > 0 {
		// 删除总图会级联删除其关系，但不会级联删除 parts。只有不再被
		// 任何关系引用的零件才安全删除；仍被其他项目借用的源零件保留。
		if _, err := tx.Exec(ctx, `DELETE FROM parts WHERE id = ANY($1::uuid[]) AND NOT EXISTS (SELECT 1 FROM drawing_part_relations r WHERE r.part_id = parts.id)`, relatedPartIDs); err != nil {
			return "", fmt.Errorf("删除项目零件失败: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	// Content-addressed blobs may still belong to another project or upload
	// session. Retain physical content here; only reference-aware GC may remove it.
	return no, nil
}

func hardDeletePart(ctx context.Context, pool *pgxpool.Pool, objectStorage storage.ObjectStorage, id string) (string, string, []string, error) {
	var no, drawingNo string
	err := pool.QueryRow(ctx, `SELECT p.part_no, d.drawing_no FROM parts p JOIN drawing_part_relations r ON r.part_id = p.id AND r.relation_type = 'owned' AND r.status = 'active' JOIN drawings d ON d.id = r.drawing_id WHERE p.id = $1::uuid LIMIT 1`, id).Scan(&no, &drawingNo)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", nil, ErrAdminDrawingNotFound
	}
	if err != nil {
		return "", "", nil, err
	}
	keys := make([]string, 0)
	rows, err := pool.Query(ctx, `SELECT b.storage_key FROM attachment_versions v JOIN file_blobs b ON b.id = v.blob_id JOIN attachments a ON a.id = v.attachment_id WHERE a.part_id = $1::uuid`, id)
	if err != nil {
		return "", "", nil, err
	}
	for rows.Next() {
		var original string
		if err := rows.Scan(&original); err != nil {
			rows.Close()
			return "", "", nil, err
		}
		keys = append(keys, original)
	}
	rows.Close()
	tx, err := pool.Begin(ctx)
	if err != nil {
		return "", "", nil, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		UPDATE edit_sessions
		SET status = 'closed', closed_at = now(), last_seen_at = now()
		WHERE attachment_id IN (SELECT id FROM attachments WHERE part_id = $1::uuid)
		  AND status = 'active'`, id); err != nil {
		return "", "", nil, fmt.Errorf("释放零件编辑会话失败: %w", err)
	}
	var childCount int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM drawing_part_relations WHERE parent_relation_id IN (SELECT id FROM drawing_part_relations WHERE part_id = $1::uuid) AND status = 'active'`, id).Scan(&childCount); err != nil {
		return "", "", nil, err
	}
	if childCount > 0 {
		return "", "", nil, ErrAdminPartHasChildren
	}
	if _, err := tx.Exec(ctx, `DELETE FROM drawing_part_relations WHERE part_id = $1::uuid`, id); err != nil {
		return "", "", nil, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM parts WHERE id = $1::uuid AND NOT EXISTS (SELECT 1 FROM drawing_part_relations WHERE part_id = $1::uuid)`, id); err != nil {
		return "", "", nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", "", nil, err
	}
	if objectStorage != nil {
		for _, key := range uniqueStrings(keys) {
			if err := objectStorage.Delete(ctx, key); err != nil {
				return "", "", nil, err
			}
		}
	}
	return no, drawingNo, uniqueStrings(keys), nil
}

func hardDeleteAttachment(ctx context.Context, pool *pgxpool.Pool, objectStorage storage.ObjectStorage, id string) (string, string, []string, error) {
	var no, name string
	var keys []string
	err := pool.QueryRow(ctx, `SELECT COALESCE(d.drawing_no, parent.drawing_no, ''), COALESCE(v.original_name, a.logical_name), COALESCE(array_agg(DISTINCT b.storage_key) FILTER (WHERE b.storage_key IS NOT NULL), ARRAY[]::text[]) FROM attachments a LEFT JOIN attachment_versions v ON v.id = a.current_version_id LEFT JOIN drawings d ON d.id = a.drawing_id LEFT JOIN parts p ON p.id = a.part_id LEFT JOIN drawing_part_relations ownerRelation ON ownerRelation.part_id = p.id AND ownerRelation.relation_type = 'owned' AND ownerRelation.status = 'active' LEFT JOIN drawings parent ON parent.id = ownerRelation.drawing_id LEFT JOIN attachment_versions allVersions ON allVersions.attachment_id = a.id LEFT JOIN file_blobs b ON b.id = allVersions.blob_id WHERE a.id = $1::uuid GROUP BY d.drawing_no, parent.drawing_no, v.original_name, a.logical_name`, id).Scan(&no, &name, &keys)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", nil, ErrAdminDrawingNotFound
	}
	if err != nil {
		return "", "", nil, err
	}
	if _, err := pool.Exec(ctx, `DELETE FROM attachments WHERE id = $1::uuid`, id); err != nil {
		return "", "", nil, err
	}
	keys = uniqueStrings(keys)
	if objectStorage != nil {
		for _, key := range keys {
			if err := objectStorage.Delete(ctx, key); err != nil {
				return "", "", nil, err
			}
		}
	}
	return no, name, keys, nil
}

// requeueAttachmentConversion 将附件对应的 CAD 转换任务重置为待处理，
// 用于后台手动触发重新转换（例如插件落盘失败后人工补救）。
func requeueAttachmentConversion(ctx context.Context, pool *pgxpool.Pool, attachmentID string) (string, string, error) {
	var no, name string
	var requeued int64
	err := pool.QueryRow(ctx, `
		WITH job AS (
			UPDATE cad_conversion_jobs
			SET status = 'pending', attempts = 0, next_attempt_at = now(), lease_until = NULL, last_error = NULL, updated_at = now()
			WHERE attachment_id = $1::uuid
			RETURNING id
		)
		SELECT COALESCE(d.drawing_no, parent.drawing_no, ''), COALESCE(v.original_name, a.logical_name),
		       (SELECT count(*) FROM job)
		FROM attachments a
		LEFT JOIN attachment_versions v ON v.id = a.current_version_id
		LEFT JOIN drawings d ON d.id = a.drawing_id
		LEFT JOIN parts p ON p.id = a.part_id
		LEFT JOIN drawing_part_relations ownerRelation ON ownerRelation.part_id = p.id AND ownerRelation.relation_type = 'owned' AND ownerRelation.status = 'active'
		LEFT JOIN drawings parent ON parent.id = ownerRelation.drawing_id
		WHERE a.id = $1::uuid`, attachmentID).Scan(&no, &name, &requeued)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrAdminDrawingNotFound
	}
	if err != nil {
		return "", "", err
	}
	if requeued == 0 {
		return "", "", ErrAdminAttachmentNoJob
	}
	return no, name, nil
}

func attachmentIDs(items []AdminAttachment) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}
func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}
func parseAdminInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
func actionLabel(action string) string {
	if action == "disable" {
		return "禁用"
	}
	return "启用"
}
func writeAdminAudit(ctx context.Context, repository audit.Repository, user auth.AuthUser, input audit.CreateInput) {
	if repository != nil {
		_, _ = repository.Create(ctx, input, user.ID, user.DisplayName, "", "")
	}
}
func adminResourceParts(path, prefix string) (string, string, error) {
	raw := strings.TrimPrefix(path, prefix)
	if raw == path {
		return "", "", errors.New("资源路径无效")
	}
	parts := strings.Split(strings.Trim(raw, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		return "", "", errors.New("资源 ID 不能为空")
	}
	id, err := url.PathUnescape(parts[0])
	if err != nil {
		return "", "", errors.New("资源 ID 无效")
	}
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}
	return id, action, nil
}
func writeAdminDrawingError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrAdminDrawingNotFound):
		response.WriteError(writer, http.StatusNotFound, "图纸不存在")
	case errors.Is(err, ErrAdminDrawingBusy):
		response.WriteError(writer, http.StatusConflict, err.Error())
	case errors.Is(err, ErrAdminPartHasChildren):
		response.WriteError(writer, http.StatusConflict, err.Error())
	case errors.Is(err, ErrAdminAttachmentNoJob):
		response.WriteError(writer, http.StatusConflict, err.Error())
	default:
		response.WriteError(writer, http.StatusInternalServerError, "后台图纸操作失败")
	}
}
