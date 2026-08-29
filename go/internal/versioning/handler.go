package versioning

import (
	"errors"
	"net/http"
	"strings"

	"cadguanliq/internal/auth"
	"cadguanliq/internal/http/middleware"
	"cadguanliq/internal/response"
)

func Resource(service *Service) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		user, ok := middleware.UserFromContext(request.Context())
		if !ok {
			response.WriteError(writer, http.StatusUnauthorized, "登录已失效，请重新登录")
			return
		}
		parts := strings.Split(strings.Trim(strings.TrimPrefix(request.URL.Path, "/api/file-versions/"), "/"), "/")
		if len(parts) != 2 || parts[0] == "" {
			response.WriteError(writer, http.StatusNotFound, "文件版本接口不存在")
			return
		}
		versionID, operation := parts[0], parts[1]
		if request.Method != http.MethodPost || (operation != "retain" && operation != "release") {
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var item Version
		var err error
		if operation == "retain" {
			item, err = service.Retain(request.Context(), user, versionID)
		} else {
			item, err = service.Release(request.Context(), user, versionID)
		}
		if err != nil {
			writeVersionError(writer, err)
			return
		}
		response.WriteData(writer, http.StatusOK, item)
	}
}

func List(service *Service) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if _, ok := middleware.UserFromContext(request.Context()); !ok {
			response.WriteError(writer, http.StatusUnauthorized, "登录已失效，请重新登录")
			return
		}
		attachmentID := strings.TrimSpace(request.URL.Query().Get("attachmentId"))
		if attachmentID == "" {
			response.WriteError(writer, http.StatusBadRequest, "缺少附件 ID")
			return
		}
		items, err := service.List(request.Context(), strings.Trim(attachmentID, "/"))
		if err != nil {
			writeVersionError(writer, err)
			return
		}
		response.WriteData(writer, http.StatusOK, items)
	}
}

func writeVersionError(writer http.ResponseWriter, err error) {
	if errors.Is(err, auth.ErrInvalidCredentials) {
		response.WriteError(writer, http.StatusUnauthorized, err.Error())
		return
	}
	response.WriteError(writer, http.StatusBadRequest, err.Error())
}
