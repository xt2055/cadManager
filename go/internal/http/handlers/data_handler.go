package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"cadguanliq/internal/attachment"
	"cadguanliq/internal/data"
	"cadguanliq/internal/http/middleware"
	"cadguanliq/internal/response"
)

func DataDocument(repository *data.DocumentRepository, attachmentRepository attachment.Repository) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		switch request.Method {
		case http.MethodGet:
			document, updatedAt, err := repository.Load(request.Context())
			if err != nil {
				response.WriteError(writer, http.StatusInternalServerError, "业务数据读取失败")
				return
			}
			if err := mergeStoredAttachments(request, document, attachmentRepository); err != nil {
				response.WriteError(writer, http.StatusInternalServerError, "附件数据读取失败")
				return
			}
			delete(document, "users")
			if !updatedAt.IsZero() {
				writer.Header().Set("X-Document-UpdatedAt", updatedAt.UTC().Format(time.RFC3339Nano))
			}
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
			var expectedUpdatedAt time.Time
			if expected := strings.TrimSpace(request.Header.Get("X-Expected-UpdatedAt")); expected != "" {
				parsed, parseErr := time.Parse(time.RFC3339Nano, expected)
				if parseErr != nil {
					response.WriteError(writer, http.StatusBadRequest, "并发校验时间戳无效，请刷新页面后重试")
					return
				}
				expectedUpdatedAt = parsed
			}
			savedUpdatedAt, err := repository.Save(request.Context(), document, user.ID, expectedUpdatedAt)
			if err != nil {
				if errors.Is(err, data.ErrDocumentConflict) {
					log.Printf("[数据同步] 拒绝过期保存: %v", err)
					response.WriteError(writer, http.StatusConflict, err.Error())
					return
				}
				log.Printf("[数据同步] 业务数据保存失败: %v", err)
				response.WriteError(writer, http.StatusInternalServerError, "业务数据保存失败")
				return
			}
			if !savedUpdatedAt.IsZero() {
				writer.Header().Set("X-Document-UpdatedAt", savedUpdatedAt.UTC().Format(time.RFC3339Nano))
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
	// 版本替换会把新文件上传到 history 存储键并新建附件记录，旧附件记录保留用于历史下载；
	// 合并文件清单时跳过已被替换（出现在 history 链中）的旧记录，
	// 否则原始文件与转换/替换后的文件会同时出现在项目详情页图纸库。
	replacedKeys := collectReplacedStorageKeys(drawings, structure)
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
			// 工艺文件与备料表有专属页签和数据源（drawing.craftFiles / materialFiles），
			// 不合并进图纸文件清单，否则会以“其他文件”身份重复出现。
			if item.Role == attachment.RoleCraft || item.Role == attachment.RoleMaterial {
				continue
			}
			if replacedKeys[item.StorageKey] || replacedKeys[item.CurrentStorageKey] {
				continue
			}
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

// collectReplacedStorageKeys 收集文档中所有 history 链引用过的存储键；
// 这些键对应的附件记录已被新版本替换，合并清单时应当跳过。
func collectReplacedStorageKeys(drawings, structure []any) map[string]bool {
	replacedKeys := map[string]bool{}
	var collectFiles func(files any)
	collectFiles = func(files any) {
		list, ok := files.([]any)
		if !ok {
			return
		}
		for _, rawFile := range list {
			file, ok := rawFile.(map[string]any)
			if !ok {
				continue
			}
			history, ok := file["history"].([]any)
			if !ok {
				continue
			}
			for _, rawItem := range history {
				item, ok := rawItem.(map[string]any)
				if !ok {
					continue
				}
				if key, _ := item["storageKey"].(string); strings.TrimSpace(key) != "" {
					replacedKeys[key] = true
				}
			}
		}
	}
	for _, rawDrawing := range drawings {
		drawing, ok := rawDrawing.(map[string]any)
		if !ok {
			continue
		}
		collectFiles(drawing["files"])
		collectFiles(drawing["otherFiles"])
	}
	for _, rawPart := range structure {
		part, ok := rawPart.(map[string]any)
		if !ok {
			continue
		}
		collectFiles(part["files"])
		collectFiles(part["otherFiles"])
	}
	return replacedKeys
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
	drawing[key] = upsertStoredFile(drawing[key], storedFile)
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
		part[key] = upsertStoredFile(part[key], storedFile)
		return
	}
}

// upsertStoredFile 把数据库附件合并进前端文件清单。同一附件（原始存储键相同）已存在时
// 原地更新为当前工作形态（如 EXB 经本地编辑转出的 DWG），否则追加；
// 若只按当前存储键判重，"原始 EXB + 当前 DWG"会作为两条记录同时出现在图纸库。
func upsertStoredFile(value any, storedFile map[string]any) []any {
	files, _ := value.([]any)
	currentKey, _ := storedFile["storageKey"].(string)
	rawStorageKey, _ := storedFile["rawStorageKey"].(string)
	for _, rawFile := range files {
		file, ok := rawFile.(map[string]any)
		if !ok {
			continue
		}
		fileKey, _ := file["storageKey"].(string)
		fileRawKey, _ := file["rawStorageKey"].(string)
		fileName, _ := file["name"].(string)
		same := rawStorageKey != "" && (fileKey == rawStorageKey || fileRawKey == rawStorageKey)
		same = same || (currentKey != "" && fileKey == currentKey)
		// 同名兜底：同一清单里出现同名文件（如重复上传产生的多余记录）只展示一条，
		// 避免前端文档条目与数据库记录以“不同存储键的同名文件”身份重复出现。
		if storedName, _ := storedFile["name"].(string); storedName != "" {
			same = same || fileName == storedName
		}
		if !same {
			continue
		}
		if name, _ := storedFile["name"].(string); name != "" {
			file["name"] = name
		}
		if currentKey != "" {
			file["storageKey"] = currentKey
		}
		if rawStorageKey != "" {
			file["rawStorageKey"] = rawStorageKey
		}
		if current, _ := storedFile["currentStorageKey"].(string); current != "" {
			file["currentStorageKey"] = current
		}
		if mimeType, _ := storedFile["mimeType"].(string); mimeType != "" {
			file["mimeType"] = mimeType
		}
		if size, _ := storedFile["size"].(string); size != "" {
			file["size"] = size
		}
		if previewable, ok := storedFile["previewable"].(bool); ok {
			file["previewable"] = previewable
		}
		return files
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
