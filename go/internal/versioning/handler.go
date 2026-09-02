package versioning

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"cadguanliq/internal/auth"
	"cadguanliq/internal/converter"
	"cadguanliq/internal/http/middleware"
	"cadguanliq/internal/response"
)

func Resource(service *Service, converterService *converter.Service) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		user, ok := middleware.UserFromContext(request.Context())
		if !ok {
			response.WriteError(writer, http.StatusUnauthorized, "登录已失效，请重新登录")
			return
		}
		parts := strings.Split(strings.Trim(strings.TrimPrefix(request.URL.Path, "/api/file-versions/"), "/"), "/")
		if len(parts) < 1 || parts[0] == "" {
			response.WriteError(writer, http.StatusNotFound, "文件版本接口不存在")
			return
		}
		versionID, operation := parts[0], ""
		if len(parts) >= 2 {
			operation = parts[1]
		}

		// 历史版本在线浏览源（按版本文件存储键查询）：
		// GET /api/file-versions/source?storageKey=...&versionKey=...
		// 前端历史树节点的 storageKey 即版本文件的存储键，据此精确返回该版本内容，
		// 不修改当前指针、不创建正式版本。
		if operation == "" && versionID == "source" {
			if request.Method != http.MethodGet {
				response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			storageKey := strings.TrimSpace(request.URL.Query().Get("storageKey"))
			if storageKey == "" {
				response.WriteError(writer, http.StatusBadRequest, "storageKey 必填")
				return
			}
			reader, version, err := service.OpenVersionSourceByStorageKey(request.Context(), storageKey)
			if err != nil {
				writeVersionError(writer, err)
				return
			}
			writeVersionSource(writer, request, converterService, reader, version)
			return
		}

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

		// 历史版本在线浏览源（按版本 ID）：GET /api/file-versions/{id}/source
		if operation == "source" {
			if request.Method != http.MethodGet {
				response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			reader, version, err := service.OpenVersionContent(request.Context(), versionID)
			if err != nil {
				writeVersionError(writer, err)
				return
			}
			writeVersionSource(writer, request, converterService, reader, version)
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

// writeVersionSource 以 inline 方式输出版本内容供在线浏览。
// 存量 EXB 版本临时转换为 DWG 后返回，不修改任何正式数据。
func writeVersionSource(writer http.ResponseWriter, request *http.Request, converterService *converter.Service, reader io.ReadCloser, version Version) {
	ctx := request.Context()
	fileName := filepath.Base(version.StorageKey)

	if strings.EqualFold(filepath.Ext(fileName), ".exb") {
		reader.Close()
		if converterService == nil {
			response.WriteError(writer, http.StatusServiceUnavailable, "CAD 转换服务未启动")
			return
		}
		tempDir, tempErr := os.MkdirTemp("", "version-source-")
		if tempErr != nil {
			response.WriteError(writer, http.StatusInternalServerError, "创建临时转换目录失败")
			return
		}
		defer os.RemoveAll(tempDir)
		inputPath := filepath.Join(tempDir, fileName)
		outputPath := filepath.Join(tempDir, strings.TrimSuffix(fileName, filepath.Ext(fileName))+".dwg")
		if err := copyVersionToTempFile(reader, inputPath); err != nil {
			writeVersionError(writer, err)
			return
		}
		if err := converterService.ConvertPathToDwg(ctx, inputPath, outputPath); err != nil {
			response.WriteError(writer, http.StatusUnprocessableEntity, "历史版本转换为 DWG 失败："+err.Error())
			return
		}
		dwgFile, openErr := os.Open(outputPath)
		if openErr != nil {
			response.WriteError(writer, http.StatusInternalServerError, "读取转换结果失败")
			return
		}
		defer dwgFile.Close()
		fileName = strings.TrimSuffix(fileName, filepath.Ext(fileName)) + ".dwg"
		writer.Header().Set("Content-Type", "application/acad")
		writer.Header().Set("Content-Disposition", "inline; filename*=UTF-8''"+url.PathEscape(fileName))
		_, _ = io.Copy(writer, dwgFile)
		return
	}

	defer reader.Close()
	writer.Header().Set("Content-Type", firstNonEmpty(version.MimeType, "application/acad"))
	writer.Header().Set("Content-Disposition", "inline; filename*=UTF-8''"+url.PathEscape(fileName))
	_, _ = io.Copy(writer, reader)
}

// copyVersionToTempFile 把已打开的版本内容流写到本地临时文件（EXB 存量版本转换用）。
func copyVersionToTempFile(reader io.Reader, outputPath string) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return errors.New("创建临时文件失败")
	}
	defer file.Close()
	_, err = io.Copy(file, reader)
	return err
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
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
