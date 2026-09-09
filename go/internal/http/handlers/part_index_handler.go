package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"cadguanliq/internal/http/middleware"
	"cadguanliq/internal/partindex"
	"cadguanliq/internal/response"
)

func PartIndexes(store partindex.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := middleware.UserFromContext(r.Context())
		if !ok {
			response.WriteError(w, http.StatusUnauthorized, "登录已失效，请重新登录")
			return
		}
		admin := userIsAdmin(user.Roles)
		path := strings.TrimPrefix(r.URL.Path, "/api/part-indexes")
		switch {
		case path == "":
			handlePartIndexList(w, r, store, user.ID, admin)
		case path == "/options":
			handlePartIndexOptions(w, r, store, user.ID, admin)
		case strings.HasSuffix(path, "/rebuild"):
			handlePartIndexRebuild(w, r, store, strings.TrimSuffix(strings.TrimPrefix(path, "/"), "/rebuild"), user.ID, admin)
		default:
			handlePartIndexDetail(w, r, store, strings.TrimPrefix(path, "/"), user.ID, admin)
		}
	}
}

func PartIndexBackfill(store partindex.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			response.WriteError(w, http.StatusMethodNotAllowed, "不支持的请求方法")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
		var input struct {
			AfterAttachmentID *string `json:"afterAttachmentId"`
			Limit             *int    `json:"limit"`
		}
		if decodeJSON(r, &input) != nil {
			response.WriteError(w, http.StatusBadRequest, "补建参数格式无效")
			return
		}
		afterAttachmentID := ""
		if input.AfterAttachmentID != nil {
			if strings.TrimSpace(*input.AfterAttachmentID) != "" {
				var normalizeErr error
				afterAttachmentID, normalizeErr = partindex.NormalizeUUID(*input.AfterAttachmentID)
				if normalizeErr != nil {
					response.WriteError(w, http.StatusBadRequest, "补建游标附件 ID 无效")
					return
				}
			}
		}
		limit := 50
		if input.Limit != nil {
			limit = *input.Limit
		}
		if limit < 1 || limit > 100 {
			response.WriteError(w, http.StatusBadRequest, "补建数量必须在 1 到 100 之间")
			return
		}
		result, err := store.Backfill(r.Context(), partindex.BackfillInput{
			AfterAttachmentID: afterAttachmentID,
			Limit:             limit,
		})
		if err != nil {
			writePartIndexError(w, err)
			return
		}
		response.WriteData(w, http.StatusOK, result)
	}
}

func handlePartIndexList(w http.ResponseWriter, r *http.Request, store partindex.Store, userID string, admin bool) {
	if r.Method != http.MethodGet {
		response.WriteError(w, http.StatusMethodNotAllowed, "不支持的请求方法")
		return
	}
	filter := partIndexFilterFromQuery(r)
	result, err := store.List(r.Context(), filter, userID, admin)
	if err != nil {
		writePartIndexError(w, err)
		return
	}
	response.WriteData(w, http.StatusOK, result)
}

func handlePartIndexOptions(w http.ResponseWriter, r *http.Request, store partindex.Store, userID string, admin bool) {
	if r.Method != http.MethodGet {
		response.WriteError(w, http.StatusMethodNotAllowed, "不支持的请求方法")
		return
	}
	limit := queryPositiveInt(r, "limit", 30)
	result, err := store.Options(
		r.Context(),
		strings.TrimSpace(r.URL.Query().Get("kind")),
		strings.TrimSpace(r.URL.Query().Get("keyword")),
		limit,
		userID,
		admin,
	)
	if err != nil {
		writePartIndexError(w, err)
		return
	}
	response.WriteData(w, http.StatusOK, result)
}

func handlePartIndexDetail(w http.ResponseWriter, r *http.Request, store partindex.Store, attachmentID, userID string, admin bool) {
	var err error
	attachmentID, err = partindex.NormalizeUUID(attachmentID)
	if err != nil {
		response.WriteError(w, http.StatusBadRequest, "附件 ID 无效")
		return
	}
	switch r.Method {
	case http.MethodGet:
		detail, err := store.Detail(r.Context(), attachmentID, userID, admin)
		if err != nil {
			writePartIndexError(w, err)
			return
		}
		response.WriteData(w, http.StatusOK, detail)
	case http.MethodPatch:
		r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
		var request struct {
			VersionID                *string         `json:"versionId"`
			ExpectedRevision         *int64          `json:"expectedRevision"`
			ExpectedSnapshotRevision *int64          `json:"expectedSnapshotRevision"`
			Action                   *string         `json:"action"`
			SelectedSpaceID          json.RawMessage `json:"selectedSpaceId"`
			Fields                   json.RawMessage `json:"fields"`
		}
		if decodeJSON(r, &request) != nil {
			response.WriteError(w, http.StatusBadRequest, "零件索引保存参数格式无效")
			return
		}
		if request.VersionID == nil || request.ExpectedRevision == nil || request.ExpectedSnapshotRevision == nil || request.Action == nil {
			response.WriteError(w, http.StatusBadRequest, "零件索引保存缺少必要参数")
			return
		}
		versionID, normalizeErr := partindex.NormalizeUUID(*request.VersionID)
		if normalizeErr != nil {
			response.WriteError(w, http.StatusBadRequest, "版本 ID 无效")
			return
		}
		action := strings.TrimSpace(*request.Action)
		selectedSpaceID, fields, err := partIndexEditValues(action, request.SelectedSpaceID, request.Fields)
		if err != nil {
			response.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		detail, err := store.Edit(r.Context(), attachmentID, userID, admin, partindex.EditInput{
			VersionID:                versionID,
			ExpectedRevision:         *request.ExpectedRevision,
			ExpectedSnapshotRevision: *request.ExpectedSnapshotRevision,
			Action:                   action,
			SelectedSpaceID:          selectedSpaceID,
			Fields:                   fields,
		})
		if err != nil {
			writePartIndexError(w, err)
			return
		}
		response.WriteData(w, http.StatusOK, detail)
	default:
		response.WriteError(w, http.StatusMethodNotAllowed, "不支持的请求方法")
	}
}

func handlePartIndexRebuild(w http.ResponseWriter, r *http.Request, store partindex.Store, attachmentID, userID string, admin bool) {
	if r.Method != http.MethodPost {
		response.WriteError(w, http.StatusMethodNotAllowed, "不支持的请求方法")
		return
	}
	var err error
	attachmentID, err = partindex.NormalizeUUID(attachmentID)
	if err != nil {
		response.WriteError(w, http.StatusBadRequest, "附件 ID 无效")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	var request struct {
		VersionID                string `json:"versionId"`
		ExpectedSnapshotRevision int64  `json:"expectedSnapshotRevision"`
	}
	if decodeJSON(r, &request) != nil {
		response.WriteError(w, http.StatusBadRequest, "重建参数格式无效")
		return
	}
	versionID, normalizeErr := partindex.NormalizeUUID(request.VersionID)
	if normalizeErr != nil {
		response.WriteError(w, http.StatusBadRequest, "版本 ID 无效")
		return
	}
	result, detail, err := store.Rebuild(r.Context(), attachmentID, userID, admin, partindex.RebuildInput{
		VersionID:                versionID,
		ExpectedSnapshotRevision: request.ExpectedSnapshotRevision,
	})
	if err != nil {
		writePartIndexError(w, err)
		return
	}
	response.WriteData(w, http.StatusOK, map[string]any{"result": result, "detail": detail})
}

func partIndexFilterFromQuery(r *http.Request) partindex.ListFilter {
	return partindex.ListFilter{
		Page:      queryPositiveInt(r, "page", 1),
		PageSize:  queryPositiveInt(r, "page_size", 20),
		Keyword:   strings.TrimSpace(r.URL.Query().Get("keyword")),
		ProjectID: strings.TrimSpace(r.URL.Query().Get("project_id")),
		Material:  strings.TrimSpace(r.URL.Query().Get("material")),
		Designer:  strings.TrimSpace(r.URL.Query().Get("designer")),
		DateFrom:  strings.TrimSpace(r.URL.Query().Get("date_from")),
		DateTo:    strings.TrimSpace(r.URL.Query().Get("date_to")),
		Status:    strings.TrimSpace(r.URL.Query().Get("status")),
	}
}

func queryPositiveInt(r *http.Request, key string, fallback int) int {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return -1
	}
	return value
}

func partIndexEditValues(action string, selectedRaw, fieldsRaw json.RawMessage) (*string, *partindex.Fields, error) {
	switch action {
	case "reset":
		if len(selectedRaw) > 0 || len(fieldsRaw) > 0 {
			return nil, nil, errors.New("恢复识别值不能提交布局或字段")
		}
		return nil, nil, nil
	case "save", "confirm":
		if len(selectedRaw) == 0 || len(fieldsRaw) == 0 {
			return nil, nil, errors.New("保存零件索引必须提交布局和完整字段")
		}
	default:
		return nil, nil, errors.New("零件索引操作无效")
	}

	var selectedSpaceID *string
	if strings.TrimSpace(string(selectedRaw)) != "null" {
		var selected string
		if err := json.Unmarshal(selectedRaw, &selected); err != nil {
			return nil, nil, errors.New("识别空间参数无效")
		}
		selected = strings.TrimSpace(selected)
		if selected == "" {
			return nil, nil, errors.New("识别空间不能为空；如不绑定布局请提交 null")
		}
		selectedSpaceID = &selected
	}

	var rawFields map[string]json.RawMessage
	if err := json.Unmarshal(fieldsRaw, &rawFields); err != nil || len(rawFields) != 12 {
		return nil, nil, errors.New("零件索引字段必须包含全部 12 项")
	}
	expected := map[string]bool{
		"drawingNo": true, "partName": true, "material": true, "designer": true,
		"checker": true, "approver": true, "drawingDateRaw": true, "scale": true,
		"sheetSize": true, "process": true, "standard": true, "company": true,
	}
	for key := range rawFields {
		if !expected[key] {
			return nil, nil, errors.New("零件索引字段包含未知项目")
		}
	}
	for key := range expected {
		rawValue, ok := rawFields[key]
		if !ok || strings.TrimSpace(string(rawValue)) == "null" {
			return nil, nil, errors.New("零件索引字段必须全部为字符串")
		}
		var value string
		if err := json.Unmarshal(rawValue, &value); err != nil {
			return nil, nil, errors.New("零件索引字段必须全部为字符串")
		}
	}
	var fields partindex.Fields
	if err := json.Unmarshal(fieldsRaw, &fields); err != nil {
		return nil, nil, errors.New("零件索引字段必须全部为字符串")
	}
	return selectedSpaceID, &fields, nil
}

func userIsAdmin(roles []string) bool {
	for _, role := range roles {
		if strings.EqualFold(role, "admin") {
			return true
		}
	}
	return false
}

func writePartIndexError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := "零件索引操作失败"
	switch {
	case errors.Is(err, partindex.ErrNotFound):
		status, message = http.StatusNotFound, err.Error()
	case errors.Is(err, partindex.ErrForbidden):
		status, message = http.StatusForbidden, err.Error()
	case errors.Is(err, partindex.ErrConflict):
		status, message = http.StatusConflict, err.Error()
	case errors.Is(err, partindex.ErrSnapshotRequired):
		status, message = http.StatusConflict, err.Error()
	case errors.Is(err, partindex.ErrInvalid):
		status, message = http.StatusBadRequest, err.Error()
	}
	if status == http.StatusInternalServerError {
		log.Printf("[零件索引] 操作失败: %v", err)
	}
	response.WriteError(w, status, message)
}
