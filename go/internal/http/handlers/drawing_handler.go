package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"cadguanliq/internal/auth"
	"cadguanliq/internal/drawing"
	"cadguanliq/internal/http/middleware"
	"cadguanliq/internal/response"
)

func Drawings(repository drawing.Repository) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		user, ok := middleware.UserFromContext(request.Context())
		if !ok {
			response.WriteError(writer, http.StatusUnauthorized, "登录已失效，请重新登录")
			return
		}
		switch request.Method {
		case http.MethodGet:
			filter := drawing.ListFilter{
				Page:     queryInt(request, "page", 1),
				PageSize: queryInt(request, "page_size", 20),
				Keyword:  request.URL.Query().Get("keyword"),
				Status:   drawing.Status(request.URL.Query().Get("status")),
				Vendor:   request.URL.Query().Get("vendor"),
			}
			if filter.Page < 1 {
				filter.Page = 1
			}
			if filter.PageSize < 1 || filter.PageSize > 100 {
				filter.PageSize = 20
			}
			page, err := repository.List(request.Context(), filter)
			if err != nil {
				response.WriteError(writer, http.StatusInternalServerError, "图纸列表读取失败")
				return
			}
			response.WriteData(writer, http.StatusOK, page)
		case http.MethodPost:
			var input drawing.CreateDrawingInput
			if err := decodeJSON(request, &input); err != nil {
				response.WriteError(writer, http.StatusBadRequest, "图纸创建参数格式无效")
				return
			}
			if strings.TrimSpace(input.No) == "" || strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.Project) == "" {
				response.WriteError(writer, http.StatusBadRequest, "图号、名称和项目不能为空")
				return
			}
			if input.Kind != "" && input.Kind != "总图" && input.Kind != "零件图" {
				response.WriteError(writer, http.StatusBadRequest, "图纸类型无效")
				return
			}
			if input.Status != "" && !validStatus(input.Status) {
				response.WriteError(writer, http.StatusBadRequest, "图纸状态无效")
				return
			}
			item, err := repository.Create(request.Context(), input, user.ID)
			if err != nil {
				writeDrawingError(writer, err, "图纸创建失败")
				return
			}
			response.WriteData(writer, http.StatusCreated, item)
		default:
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

func DrawingResource(repository drawing.Repository) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		user, ok := middleware.UserFromContext(request.Context())
		if !ok {
			response.WriteError(writer, http.StatusUnauthorized, "登录已失效，请重新登录")
			return
		}
		// 图纸号可能含「/」（如 JG9055e-50/32-00），必须基于未解码的路径分段，
		// 否则 %2F 解码后被拆段导致 404。
		resourcePath := strings.TrimPrefix(request.URL.EscapedPath(), "/api/drawings/")
		parts := strings.Split(strings.Trim(resourcePath, "/"), "/")
		if len(parts) == 0 || parts[0] == "" {
			response.WriteError(writer, http.StatusNotFound, "图纸不存在")
			return
		}
		id, err := url.PathUnescape(parts[0])
		if err != nil {
			response.WriteError(writer, http.StatusBadRequest, "图纸编号无效")
			return
		}
		if len(parts) == 2 && parts[1] == "bom" {
			atomic, ok := repository.(drawing.AtomicRepository)
			if !ok {
				response.WriteError(writer, http.StatusNotImplemented, "BOM 命令尚未配置")
				return
			}
			if request.Method == http.MethodGet {
				item, err := atomic.GetBOM(request.Context(), id)
				if err != nil {
					writeAtomicDrawingError(writer, err, "BOM 读取失败")
					return
				}
				response.WriteData(writer, http.StatusOK, item)
				return
			}
			if request.Method == http.MethodPut {
				var input drawing.UpdateBOMInput
				if err := decodeJSON(request, &input); err != nil {
					response.WriteError(writer, http.StatusBadRequest, "BOM 参数格式无效")
					return
				}
				item, err := atomic.ReplaceBOM(request.Context(), id, input, user.ID)
				if err != nil {
					writeAtomicDrawingError(writer, err, "BOM 修改失败")
					return
				}
				response.WriteData(writer, http.StatusOK, item)
				return
			}
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if len(parts) == 2 && parts[1] == "borrows" && request.Method == http.MethodPost {
			atomic, ok := repository.(drawing.AtomicRepository)
			if !ok {
				response.WriteError(writer, http.StatusNotImplemented, "借用命令尚未配置")
				return
			}
			var input drawing.BorrowInput
			if err := decodeJSON(request, &input); err != nil {
				response.WriteError(writer, http.StatusBadRequest, "借用零件参数格式无效")
				return
			}
			item, err := atomic.Borrow(request.Context(), id, input, user.ID)
			if err != nil {
				writeAtomicDrawingError(writer, err, "借用零件失败")
				return
			}
			response.WriteData(writer, http.StatusCreated, item)
			return
		}
		if len(parts) == 2 && parts[1] == "structure" && request.Method == http.MethodGet {
			items, err := repository.ListParts(request.Context(), id)
			if err != nil {
				writeDrawingError(writer, err, "结构树读取失败")
				return
			}
			response.WriteData(writer, http.StatusOK, items)
			return
		}
		if len(parts) == 2 && parts[1] == "parts" && request.Method == http.MethodPost {
			var input drawing.CreatePartInput
			if err := decodeJSON(request, &input); err != nil {
				response.WriteError(writer, http.StatusBadRequest, "零件创建参数格式无效")
				return
			}
			if input.Status != "" && !validStatus(input.Status) {
				response.WriteError(writer, http.StatusBadRequest, "图纸状态无效")
				return
			}
			if strings.TrimSpace(input.No) == "" || strings.TrimSpace(input.Name) == "" {
				response.WriteError(writer, http.StatusBadRequest, "零件图号和名称不能为空")
				return
			}
			item, err := repository.CreatePart(request.Context(), id, input, user.ID)
			if err != nil {
				writeDrawingError(writer, err, "零件创建失败")
				return
			}
			response.WriteData(writer, http.StatusCreated, item)
			return
		}
		if len(parts) == 2 && (parts[1] == "archive" || parts[1] == "unarchive") && request.Method == http.MethodPost {
			item, err := transitionDrawingStatus(request.Context(), repository, user, id, parts[1])
			if err != nil {
				writeTransitionError(writer, err)
				return
			}
			response.WriteData(writer, http.StatusOK, item)
			return
		}
		if len(parts) != 1 {
			response.WriteError(writer, http.StatusNotFound, "图纸接口不存在")
			return
		}
		switch request.Method {
		case http.MethodGet:
			item, err := repository.Find(request.Context(), id)
			if err != nil {
				writeDrawingError(writer, err, "图纸读取失败")
				return
			}
			response.WriteData(writer, http.StatusOK, item)
		case http.MethodPatch:
			var input drawing.UpdateDrawingInput
			if err := decodeJSON(request, &input); err != nil {
				response.WriteError(writer, http.StatusBadRequest, "图纸修改参数格式无效")
				return
			}
			if input.Status != nil && !validStatus(*input.Status) {
				response.WriteError(writer, http.StatusBadRequest, "图纸状态无效")
				return
			}
			if input.Kind != nil && *input.Kind != "总图" && *input.Kind != "零件图" {
				response.WriteError(writer, http.StatusBadRequest, "图纸类型无效")
				return
			}
			if input.ExpectedRevision == nil || *input.ExpectedRevision < 1 {
				response.WriteError(writer, http.StatusPreconditionRequired, "修改图纸必须提供当前 revision")
				return
			}
			item, err := repository.Update(request.Context(), id, input, user.ID)
			if err != nil {
				writeDrawingError(writer, err, "图纸修改失败")
				return
			}
			response.WriteData(writer, http.StatusOK, item)
		default:
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

// DrawingPartRelationResource 提供结构关系自己的原子写接口，不再通过整表 PUT。
func DrawingPartRelationResource(repository drawing.AtomicRepository) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		user, ok := middleware.UserFromContext(request.Context())
		if !ok {
			response.WriteError(writer, http.StatusUnauthorized, "登录已失效，请重新登录")
			return
		}
		resourcePath := strings.Trim(strings.TrimPrefix(request.URL.Path, "/api/drawing-part-relations/"), "/")
		resourceParts := strings.Split(resourcePath, "/")
		id := resourceParts[0]
		action := ""
		if len(resourceParts) == 2 {
			action = resourceParts[1]
		}
		if id == "" || len(resourceParts) > 2 {
			response.WriteError(writer, http.StatusNotFound, "结构关系不存在")
			return
		}
		switch request.Method {
		case http.MethodPatch:
			var input drawing.UpdateRelationInput
			if err := decodeJSON(request, &input); err != nil {
				response.WriteError(writer, http.StatusBadRequest, "结构关系修改参数格式无效")
				return
			}
			item, err := repository.UpdateRelation(request.Context(), id, input, user.ID)
			if err != nil {
				writeAtomicDrawingError(writer, err, "结构关系修改失败")
				return
			}
			response.WriteData(writer, http.StatusOK, item)
		case http.MethodPost:
			if action != "fork" {
				response.WriteError(writer, http.StatusNotFound, "结构关系接口不存在")
				return
			}
			var input drawing.ForkInput
			if err := decodeJSON(request, &input); err != nil {
				response.WriteError(writer, http.StatusBadRequest, "Fork 参数格式无效")
				return
			}
			item, err := repository.ForkBorrowedPart(request.Context(), id, input, user.ID, request.Header.Get("Idempotency-Key"))
			if err != nil {
				writeAtomicDrawingError(writer, err, "Fork 零件失败")
				return
			}
			response.WriteData(writer, http.StatusCreated, item)
		default:
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

func PartResource(repository drawing.Repository) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		user, ok := middleware.UserFromContext(request.Context())
		if !ok {
			response.WriteError(writer, http.StatusUnauthorized, "登录已失效，请重新登录")
			return
		}
		resourcePath := strings.Trim(strings.TrimPrefix(request.URL.Path, "/api/parts/"), "/")
		resourceParts := strings.Split(resourcePath, "/")
		id := resourceParts[0]
		if len(resourceParts) == 2 && resourceParts[1] == "revisions" && request.Method == http.MethodPost {
			atomic, ok := repository.(drawing.AtomicRepository)
			if !ok {
				response.WriteError(writer, http.StatusNotImplemented, "零件版本命令尚未配置")
				return
			}
			var input drawing.CreateRevisionInput
			if err := decodeJSON(request, &input); err != nil {
				response.WriteError(writer, http.StatusBadRequest, "零件版本参数格式无效")
				return
			}
			item, err := atomic.CreateDraftRevision(request.Context(), id, input, user.ID)
			if err != nil {
				writeAtomicDrawingError(writer, err, "创建零件版本失败")
				return
			}
			response.WriteData(writer, http.StatusCreated, item)
			return
		}
		if id == "" || len(resourceParts) != 1 {
			response.WriteError(writer, http.StatusNotFound, "零件不存在")
			return
		}
		switch request.Method {
		case http.MethodGet:
			item, err := repository.FindPart(request.Context(), id)
			if err != nil {
				writeDrawingError(writer, err, "零件读取失败")
				return
			}
			response.WriteData(writer, http.StatusOK, item)
		case http.MethodPatch:
			var input drawing.UpdatePartInput
			if err := decodeJSON(request, &input); err != nil {
				response.WriteError(writer, http.StatusBadRequest, "零件修改参数格式无效")
				return
			}
			if input.ExpectedRevision == nil || *input.ExpectedRevision < 1 {
				response.WriteError(writer, http.StatusPreconditionRequired, "修改零件必须提供当前 revision")
				return
			}
			if input.Status != nil && !validStatus(*input.Status) {
				response.WriteError(writer, http.StatusBadRequest, "零件图状态无效")
				return
			}
			item, err := repository.UpdatePart(request.Context(), id, input, user.ID)
			if err != nil {
				writeDrawingError(writer, err, "零件修改失败")
				return
			}
			response.WriteData(writer, http.StatusOK, item)
		default:
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

func PartRevisionResource(repository drawing.AtomicRepository) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		user, ok := middleware.UserFromContext(request.Context())
		if !ok {
			response.WriteError(writer, http.StatusUnauthorized, "登录已失效，请重新登录")
			return
		}
		resourcePath := strings.Trim(strings.TrimPrefix(request.URL.Path, "/api/part-revisions/"), "/")
		parts := strings.Split(resourcePath, "/")
		id := parts[0]
		if id == "" || len(parts) > 2 {
			response.WriteError(writer, http.StatusNotFound, "零件版本不存在")
			return
		}
		if request.Method == http.MethodGet && len(parts) == 1 {
			item, err := repository.GetRevision(request.Context(), id)
			if err != nil {
				writeAtomicDrawingError(writer, err, "零件版本读取失败")
				return
			}
			response.WriteData(writer, http.StatusOK, item)
			return
		}
		if request.Method == http.MethodPatch && len(parts) == 1 {
			var input drawing.UpdateRevisionInput
			if err := decodeJSON(request, &input); err != nil {
				response.WriteError(writer, http.StatusBadRequest, "零件版本修改参数格式无效")
				return
			}
			item, err := repository.UpdateDraftRevision(request.Context(), id, input, user.ID)
			if err != nil {
				writeAtomicDrawingError(writer, err, "零件版本修改失败")
				return
			}
			response.WriteData(writer, http.StatusOK, item)
			return
		}
		if request.Method == http.MethodPost && len(parts) == 2 && (parts[1] == "submit" || parts[1] == "reject" || parts[1] == "publish") {
			item, err := repository.TransitionRevision(request.Context(), id, parts[1], user.ID)
			if err != nil {
				writeAtomicDrawingError(writer, err, "零件版本流转失败")
				return
			}
			response.WriteData(writer, http.StatusOK, item)
			return
		}
		response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func decodeJSON(request *http.Request, target any) error {
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func queryInt(request *http.Request, key string, fallback int) int {
	value, err := strconv.Atoi(request.URL.Query().Get(key))
	if err != nil {
		return fallback
	}
	return value
}

func writeDrawingError(writer http.ResponseWriter, err error, fallback string) {
	log.Printf("drawing request failed: %v", err)
	switch {
	case errors.Is(err, drawing.ErrNotFound):
		response.WriteError(writer, http.StatusNotFound, "图纸或零件不存在")
	case errors.Is(err, drawing.ErrConflict):
		response.WriteError(writer, http.StatusConflict, "图号已存在或资源冲突")
	case errors.Is(err, drawing.ErrRevisionConflict):
		response.WriteError(writer, http.StatusConflict, "资源已被其他用户修改，请刷新后重新编辑")
	case errors.Is(err, drawing.ErrRevisionRequired):
		response.WriteError(writer, http.StatusPreconditionRequired, "修改请求缺少当前 revision")
	default:
		response.WriteError(writer, http.StatusInternalServerError, fallback)
	}
}

func writeAtomicDrawingError(writer http.ResponseWriter, err error, fallback string) {
	log.Printf("atomic drawing request failed: %v", err)
	switch {
	case errors.Is(err, drawing.ErrNotFound):
		response.WriteError(writer, http.StatusNotFound, "图纸、零件或结构关系不存在")
	case errors.Is(err, drawing.ErrConflict):
		response.WriteError(writer, http.StatusConflict, "图号已存在或资源冲突")
	case errors.Is(err, drawing.ErrRevisionConflict):
		response.WriteError(writer, http.StatusConflict, "结构关系已被其他用户修改，请刷新后重试")
	case errors.Is(err, drawing.ErrRevisionRequired):
		response.WriteError(writer, http.StatusPreconditionRequired, "请求必须提供当前 revision")
	case errors.Is(err, drawing.ErrIdempotencyConflict):
		response.WriteError(writer, http.StatusConflict, "Idempotency-Key conflict")
	default:
		if strings.Contains(err.Error(), "不能") || strings.Contains(err.Error(), "必须") || strings.Contains(err.Error(), "无效") {
			response.WriteError(writer, http.StatusBadRequest, err.Error())
			return
		}
		response.WriteError(writer, http.StatusInternalServerError, fallback)
	}
}

func validStatus(status drawing.Status) bool {
	switch status {
	case drawing.StatusPublished, drawing.StatusReviewing, drawing.StatusDraft, drawing.StatusDisabled, drawing.StatusArchived:
		return true
	default:
		return false
	}
}

// transitionDrawingStatus 存档（生产→存档，创建者/管理员）与解除存档（存档→生产，仅管理员）。
func transitionDrawingStatus(ctx context.Context, repository drawing.Repository, user auth.AuthUser, key, action string) (drawing.Drawing, error) {
	// key 可能是 uuid 或图纸号；Find 按 uuid 查询，非 uuid 输入会报 22P02 而非
	// ErrNotFound，因此任何错误都回退到按图纸号查询，两者都未命中才算不存在。
	target, err := repository.Find(ctx, key)
	if err != nil {
		target, err = repository.FindByNo(ctx, key)
	}
	if err != nil {
		return drawing.Drawing{}, err
	}
	if action == "archive" {
		if target.CreatedByID != user.ID && !hasAdminRole(user.Roles) {
			return drawing.Drawing{}, fmt.Errorf("仅创建者或管理员可以存档图纸")
		}
		item, err := repository.SetStatusByNo(ctx, target.No, drawing.StatusPublished, drawing.StatusArchived, user.ID)
		if errors.Is(err, drawing.ErrInvalidTransition) {
			return drawing.Drawing{}, fmt.Errorf("仅「生产中」的图纸可以存档（草稿需先完成审核，审核中请等待签署完成）")
		}
		return item, err
	}
	if !hasAdminRole(user.Roles) {
		return drawing.Drawing{}, fmt.Errorf("解除存档需要管理员操作，请联系管理员")
	}
	item, err := repository.SetStatusByNo(ctx, target.No, drawing.StatusArchived, drawing.StatusPublished, user.ID)
	if errors.Is(err, drawing.ErrInvalidTransition) {
		return drawing.Drawing{}, fmt.Errorf("该图纸当前不在存档状态")
	}
	return item, err
}

func writeTransitionError(writer http.ResponseWriter, err error) {
	log.Printf("drawing status transition failed: %v", err)
	switch {
	case errors.Is(err, drawing.ErrNotFound):
		response.WriteError(writer, http.StatusNotFound, "图纸不存在")
	case errors.Is(err, drawing.ErrInvalidTransition):
		response.WriteError(writer, http.StatusConflict, "图纸当前状态不允许该操作")
	default:
		if msg := err.Error(); !strings.Contains(msg, "资源") && !strings.HasPrefix(msg, "drawing") {
			response.WriteError(writer, http.StatusForbidden, err.Error())
			return
		}
		response.WriteError(writer, http.StatusInternalServerError, "图纸状态更新失败")
	}
}

// hasAdminRole 复用 review_handler.go 中的同名包内 helper。
