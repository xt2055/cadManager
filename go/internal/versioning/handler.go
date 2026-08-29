package versioning

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
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

		// 所有用户都可以下载版本文件
		if operation == "content" {
			if request.Method != http.MethodGet {
				response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			reader, version, err := service.OpenVersionContent(request.Context(), versionID)
			if err != nil {
				writeVersionError(writer, err)
				return
			}
			defer reader.Close()
			fileName := filepath.Base(version.StorageKey)
			writer.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(fileName))
			writer.Header().Set("Content-Type", "application/octet-stream")
			_, _ = io.Copy(writer, reader)
			return
		}

		if request.Method != http.MethodPost || (operation != "retain" && operation != "release" && operation != "restore") {
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var item Version
		var err error
		switch operation {
		case "retain":
			item, err = service.Retain(request.Context(), user, versionID)
		case "release":
			item, err = service.Release(request.Context(), user, versionID)
		default:
			item, err = service.Restore(request.Context(), user, versionID)
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
		storageKey := strings.TrimSpace(request.URL.Query().Get("storageKey"))
		if attachmentID == "" && storageKey == "" {
			response.WriteError(writer, http.StatusBadRequest, "缺少附件 ID 或存储键")
			return
		}
		var items []Version
		var err error
		if storageKey != "" {
			items, err = service.ListByStorageKey(request.Context(), storageKey)
		} else {
			items, err = service.List(request.Context(), strings.Trim(attachmentID, "/"))
		}
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
