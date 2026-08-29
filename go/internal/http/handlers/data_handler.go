package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"

	"cadguanliq/internal/attachment"
	"cadguanliq/internal/data"
	"cadguanliq/internal/http/middleware"
	"cadguanliq/internal/response"
)

func DataDocument(repository *data.DocumentRepository, attachmentRepository attachment.Repository) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		switch request.Method {
		case http.MethodGet:
			document, err := repository.Load(request.Context())
			if err != nil {
				response.WriteError(writer, http.StatusInternalServerError, "业务数据读取失败")
				return
			}
			if err := mergeStoredAttachments(request, document, attachmentRepository); err != nil {
				response.WriteError(writer, http.StatusInternalServerError, "附件数据读取失败")
				return
			}
			delete(document, "users")
			response.WriteData(writer, http.StatusOK, document)
		case http.MethodPut:
			user, ok := middleware.UserFromContext(request.Context())
			if !ok {
				response.WriteError(writer, http.StatusUnauthorized, "登录已失效，请重新登录")
				return
			}
			var document map[string]any
			if err := json.NewDecoder(request.Body).Decode(&document); err != nil || document == nil {
				response.WriteError(writer, http.StatusBadRequest, "业务数据文档格式无效")
				return
			}
			if err := repository.Save(request.Context(), document, user.ID); err != nil {
				log.Printf("[数据同步] 业务数据保存失败: %v", err)
				response.WriteError(writer, http.StatusInternalServerError, "业务数据保存失败")
				return
			}
			response.WriteData(writer, http.StatusOK, nil)
		default:
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

func mergeStoredAttachments(request *http.Request, document map[string]any, repository attachment.Repository) error {
	if repository == nil {
		return nil
	}
	drawings, ok := document["drawings"].([]any)
	if !ok {
		return nil
	}
	structure, _ := document["structure"].([]any)
	for _, rawDrawing := range drawings {
		drawing, ok := rawDrawing.(map[string]any)
		if !ok {
			continue
		}
		drawingNo, _ := drawing["no"].(string)
		if strings.TrimSpace(drawingNo) == "" {
			continue
		}
		items, err := repository.ListByDrawing(request.Context(), drawingNo)
		if err != nil {
			return err
		}
		for _, item := range items {
			storedFile := storedAttachmentMap(item)
			if item.PartNo != nil && strings.TrimSpace(*item.PartNo) != "" {
				mergeFileIntoOwner(structure, *item.PartNo, storedFile, item.Role)
				continue
			}
			mergeFileIntoDrawing(drawing, storedFile, item.Role)
		}
	}
	return nil
}

func storedAttachmentMap(item attachment.Attachment) map[string]any {
	name := item.Name
	storageKey := item.StorageKey
	mimeType := item.MimeType
	size := item.Size

	// 如果存在转换好的当前工作 DWG，将其暴露为当前活跃图纸，实现全系统统一读取
	if item.CurrentStorageKey != "" {
		storageKey = item.CurrentStorageKey
	}
	if item.CurrentName != "" {
		name = item.CurrentName
	}
	if item.CurrentMimeType != "" {
		mimeType = item.CurrentMimeType
	}
	if item.CurrentSize > 0 {
		size = item.CurrentSize
	}

	file := map[string]any{
		"id":                item.ID,
		"name":              name,
		"rawName":           item.Name,
		"size":              formatAttachmentSize(size),
		"version":           item.Version,
		"uploadedBy":        item.UploadedBy,
		"uploadedAt":        item.CreatedAt,
		"storageKey":        storageKey,
		"rawStorageKey":     item.StorageKey,
		"currentStorageKey": item.CurrentStorageKey,
		"mimeType":          mimeType,
		"previewable":       item.Previewable,
	}
	if item.Role == attachment.RolePart {
		file["role"] = "part"
	} else if item.Role == attachment.RoleAssembly {
		file["role"] = "assembly"
	} else {
		file["role"] = "other"
	}
	return file
}

func mergeFileIntoDrawing(drawing map[string]any, storedFile map[string]any, role attachment.Role) {
	key := "otherFiles"
	if role == attachment.RoleAssembly {
		key = "files"
	}
	drawing[key] = appendUniqueStoredFile(drawing[key], storedFile)
}

func mergeFileIntoOwner(structure []any, partNo string, storedFile map[string]any, role attachment.Role) {
	for _, rawPart := range structure {
		part, ok := rawPart.(map[string]any)
		if !ok {
			continue
		}
		if no, _ := part["no"].(string); no != partNo {
			continue
		}
		key := "otherFiles"
		if role == attachment.RolePart {
			key = "files"
		}
		part[key] = appendUniqueStoredFile(part[key], storedFile)
		return
	}
}

func appendUniqueStoredFile(value any, storedFile map[string]any) []any {
	files, _ := value.([]any)
	storageKey, _ := storedFile["storageKey"].(string)
	for _, rawFile := range files {
		file, ok := rawFile.(map[string]any)
		if !ok {
			continue
		}
		if currentKey, _ := file["storageKey"].(string); currentKey == storageKey && storageKey != "" {
			return files
		}
	}
	return append(files, storedFile)
}

func formatAttachmentSize(size int64) string {
	if size < 1024 {
		return strconv.FormatInt(size, 10) + " B"
	}
	units := []string{"KB", "MB", "GB"}
	value := float64(size)
	for _, unit := range units {
		value /= 1024
		if value < 1024 || unit == units[len(units)-1] {
			return strconv.FormatFloat(value, 'f', 1, 64) + " " + unit
		}
	}
	return strconv.FormatInt(size, 10) + " B"
}
