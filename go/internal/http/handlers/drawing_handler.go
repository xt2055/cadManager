package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

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
		resourcePath := strings.TrimPrefix(request.URL.Path, "/api/drawings/")
		parts := strings.Split(strings.Trim(resourcePath, "/"), "/")
		if len(parts) == 0 || parts[0] == "" {
			response.WriteError(writer, http.StatusNotFound, "图纸不存在")
			return
		}
		id := parts[0]
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

func PartResource(repository drawing.Repository) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		user, ok := middleware.UserFromContext(request.Context())
		if !ok {
			response.WriteError(writer, http.StatusUnauthorized, "登录已失效，请重新登录")
			return
		}
		id := strings.TrimPrefix(request.URL.Path, "/api/parts/")
		if id == "" || strings.Contains(id, "/") {
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
	default:
		response.WriteError(writer, http.StatusInternalServerError, fallback)
	}
}

func validStatus(status drawing.Status) bool {
	switch status {
	case drawing.StatusPublished, drawing.StatusReviewing, drawing.StatusDraft, drawing.StatusHidden, drawing.StatusDisabled:
		return true
	default:
		return false
	}
}
