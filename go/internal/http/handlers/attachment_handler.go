package handlers

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"cadguanliq/internal/attachment"
	"cadguanliq/internal/converter"
	"cadguanliq/internal/http/middleware"
	"cadguanliq/internal/response"
	"cadguanliq/internal/storage"
)

func UploadAttachment(repository attachment.Repository, objectStorage storage.ObjectStorage, convService *converter.Service, maxBytes int64) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		user, ok := middleware.UserFromContext(request.Context())
		if !ok {
			response.WriteError(writer, http.StatusUnauthorized, "登录已失效，请重新登录")
			return
		}
		if request.Method != http.MethodPost {
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if maxBytes > 0 {
			request.Body = http.MaxBytesReader(writer, request.Body, maxBytes+1)
		}
		if err := request.ParseMultipartForm(32 << 20); err != nil {
			response.WriteError(writer, http.StatusBadRequest, "附件上传请求格式无效")
			return
		}
		file, header, err := request.FormFile("file")
		if err != nil {
			response.WriteError(writer, http.StatusBadRequest, "缺少附件文件")
			return
		}
		defer file.Close()
		drawingNo := strings.TrimSpace(request.FormValue("drawingNo"))
		partNo := strings.TrimSpace(request.FormValue("partNo"))
		if drawingNo == "" {
			response.WriteError(writer, http.StatusBadRequest, "缺少所属图号")
			return
		}
		name := safePathSegment(header.Filename)
		if name == "" {
			response.WriteError(writer, http.StatusBadRequest, "附件文件名无效")
			return
		}
		role := attachment.Role(request.FormValue("role"))
		if role == "" {
			if partNo != "" {
				role = attachment.RolePart
			} else {
				role = attachment.RoleOther
			}
		}
		if !validAttachmentRole(role) {
			response.WriteError(writer, http.StatusBadRequest, "附件类型无效")
			return
		}
		folder, err := repository.FolderForDrawing(request.Context(), drawingNo)
		if err != nil {
			writeAttachmentError(writer, err)
			return
		}
		folder = safePathSegment(folder)
		attachmentFolder := folder
		if role == attachment.RoleCraft {
			attachmentFolder = filepath.ToSlash(filepath.Join(folder, "工艺文件"))
		}
		key := filepath.ToSlash(filepath.Join(attachmentFolder, name))
		if existingKey := strings.TrimSpace(request.FormValue("storageKey")); existingKey != "" {
			existingFolder := filepath.ToSlash(filepath.Dir(existingKey))
			legacyFolder := filepath.ToSlash(folder)
			if existingFolder != filepath.ToSlash(attachmentFolder) && !(role == attachment.RoleCraft && existingFolder == legacyFolder) {
				response.WriteError(writer, http.StatusBadRequest, "附件存储键与所属图号不匹配")
				return
			}
			key = existingKey
		}
		mimeType := request.FormValue("mimeType")
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		object, err := objectStorage.Put(request.Context(), key, file, mimeType)
		if err != nil {
			var tooLargeError *http.MaxBytesError
			if errors.As(err, &tooLargeError) {
				response.WriteError(writer, http.StatusRequestEntityTooLarge, "附件超过大小限制")
				return
			}
			response.WriteError(writer, http.StatusInternalServerError, "附件保存失败")
			return
		}
		if maxBytes > 0 && object.Size > maxBytes {
			_ = objectStorage.Delete(request.Context(), key)
			response.WriteError(writer, http.StatusRequestEntityTooLarge, "附件超过大小限制")
			return
		}
		item, err := repository.Create(request.Context(), attachment.CreateInput{
			DrawingNo:   drawingNo,
			PartNo:      partNo,
			Role:        role,
			Name:        name,
			MimeType:    mimeType,
			Version:     request.FormValue("version"),
			Previewable: request.FormValue("previewable") != "false",
		}, attachment.StorageObject{Key: object.Key, Size: object.Size, MimeType: object.MimeType, SHA256: object.SHA256}, user.ID)
		if err != nil {
			_ = objectStorage.Delete(request.Context(), key)
			writeAttachmentError(writer, err)
			return
		}

		// 如果上传的是 EXB/DWG 文件，立即加入普通优先级转换队列
		if convService != nil && (strings.EqualFold(filepathExt(name), ".exb") || strings.EqualFold(filepathExt(key), ".exb") || strings.EqualFold(filepathExt(name), ".dwg") || strings.EqualFold(filepathExt(key), ".dwg")) {
			convService.PushJob(item, converter.PriorityNormal)
		}

		response.WriteData(writer, http.StatusCreated, item)
	}
}

func AttachmentResource(repository attachment.Repository, objectStorage storage.ObjectStorage) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if _, ok := middleware.UserFromContext(request.Context()); !ok {
			response.WriteError(writer, http.StatusUnauthorized, "登录已失效，请重新登录")
			return
		}
		key := strings.TrimPrefix(request.URL.Path, "/api/attachments/")
		key = strings.Trim(key, "/")
		if key == "" {
			response.WriteError(writer, http.StatusNotFound, "附件不存在")
			return
		}
		item, err := repository.Find(request.Context(), key)
		if err != nil {
			writeAttachmentError(writer, err)
			return
		}
		switch request.Method {
		case http.MethodGet:
			reader, object, err := objectStorage.Open(request.Context(), key)
			if err != nil {
				response.WriteError(writer, http.StatusNotFound, "附件文件不存在")
				return
			}
			defer reader.Close()
			writer.Header().Set("Content-Type", firstNonEmpty(item.MimeType, object.MimeType, "application/octet-stream"))
			writer.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s", url.PathEscape(item.Name)))
			writer.Header().Set("Content-Length", fmt.Sprintf("%d", object.Size))
			_, _ = io.Copy(writer, reader)
		case http.MethodDelete:
			if err := repository.Delete(request.Context(), key, ""); err != nil {
				writeAttachmentError(writer, err)
				return
			}
			if err := objectStorage.Delete(request.Context(), key); err != nil {
				response.WriteError(writer, http.StatusInternalServerError, "附件文件删除失败")
				return
			}
			response.WriteData(writer, http.StatusOK, nil)
		default:
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

func validAttachmentRole(role attachment.Role) bool {
	switch role {
	case attachment.RoleAssembly, attachment.RolePart, attachment.RoleMaterial, attachment.RoleCraft, attachment.RoleOther:
		return true
	default:
		return false
	}
}

func writeAttachmentError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, attachment.ErrNotFound):
		response.WriteError(writer, http.StatusNotFound, "附件不存在")
	case errors.Is(err, attachment.ErrConflict):
		response.WriteError(writer, http.StatusConflict, "附件已存在")
	default:
		response.WriteError(writer, http.StatusInternalServerError, "附件处理失败")
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func safePathSegment(value string) string {
	value = strings.ReplaceAll(value, "\\", "/")
	value = filepath.Base(value)
	value = strings.TrimSpace(value)
	var builder strings.Builder
	for _, character := range value {
		if character < 32 || strings.ContainsRune(`<>:"/\\|?*`, character) {
			builder.WriteRune('_')
			continue
		}
		builder.WriteRune(character)
	}
	result := strings.Trim(builder.String(), ". ")
	if result == "." || result == ".." {
		return ""
	}
	return result
}
