package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"cadguanliq/internal/attachment"
	"cadguanliq/internal/cadtext"
	"cadguanliq/internal/converter"
	"cadguanliq/internal/http/middleware"
	"cadguanliq/internal/response"
	"cadguanliq/internal/storage"
	"cadguanliq/internal/versioning"
	"github.com/jackc/pgx/v5/pgxpool"
)

func UploadAttachment(repository attachment.Repository, objectStorage storage.ObjectStorage, convService *converter.Service, versions *versioning.Service, maxBytes int64) http.HandlerFunc {
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
			// 版本替换会上传到「图号目录/history/历史图号/」之下，同样视为合法归属。
			historyPrefix := filepath.ToSlash(attachmentFolder) + "/history/"
			inHistory := strings.HasPrefix(existingFolder+"/", historyPrefix)
			if existingFolder != filepath.ToSlash(attachmentFolder) && !inHistory && !(role == attachment.RoleCraft && existingFolder == legacyFolder) {
				response.WriteError(writer, http.StatusBadRequest, "附件存储键与所属图号不匹配")
				return
			}
			key = existingKey
		}
		existingItem, existingErr := repository.Find(request.Context(), key)
		if existingErr != nil && !errors.Is(existingErr, attachment.ErrNotFound) {
			writeAttachmentError(writer, existingErr)
			return
		}
		if existingErr == nil && (existingItem.DrawingNo != drawingNo || !samePartNo(existingItem.PartNo, partNo)) {
			response.WriteError(writer, http.StatusConflict, "附件存储键已被其他图纸占用")
			return
		}
		mimeType := request.FormValue("mimeType")
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		var uploadReader io.Reader = file
		if strings.EqualFold(filepathExt(name), ".dxf") {
			content, readErr := io.ReadAll(file)
			if readErr != nil {
				var tooLargeError *http.MaxBytesError
				if errors.As(readErr, &tooLargeError) {
					response.WriteError(writer, http.StatusRequestEntityTooLarge, "附件超过大小限制")
					return
				}
				response.WriteError(writer, http.StatusInternalServerError, "读取 DXF 文件失败")
				return
			}
			if normalized, normalizeErr := cadtext.NormalizeDxfForCaxa(content); normalizeErr != nil {
				response.WriteError(writer, http.StatusUnprocessableEntity, normalizeErr.Error())
				return
			} else {
				uploadReader = bytes.NewReader(normalized)
			}
		}
		object, err := objectStorage.Put(request.Context(), key, uploadReader, mimeType)
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
			if existingErr != nil {
				_ = objectStorage.Delete(request.Context(), key)
			}
			response.WriteError(writer, http.StatusRequestEntityTooLarge, "附件超过大小限制")
			return
		}
		if existingErr == nil {
			if err := repository.UpdateContent(request.Context(), key, object.Size, object.MimeType, object.SHA256); err != nil {
				writeAttachmentError(writer, err)
				return
			}
			item, err := repository.Find(request.Context(), key)
			if err != nil {
				writeAttachmentError(writer, err)
				return
			}
			response.WriteData(writer, http.StatusOK, item)
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
			// 已存在的文件可能只是数据库元数据写入失败，不能因重试失败删除原文件。
			if existingErr != nil {
				_ = objectStorage.Delete(request.Context(), key)
			}
			writeAttachmentError(writer, err)
			return
		}

		// CAD 附件：上传即生成 v1.0 初始版本（EXB/DXF 经 CAXA 转换、DWG 复制进版本目录），
		// 等待完成后登记版本并切换当前指针，保证前端拿到成功响应时 v1.0 已可用；
		// 转换失败则回滚附件记录与原始对象，不返回“上传成功”，避免产生后续查看必然失败的坏状态。
		ext := filepathExt(item.StorageKey)
		if ext == "" {
			ext = filepathExt(item.Name)
		}
		isCAD := strings.EqualFold(ext, ".exb") || strings.EqualFold(ext, ".dwg") || strings.EqualFold(ext, ".dxf")
		if isCAD {
			if convService == nil {
				_ = objectStorage.Delete(request.Context(), key)
				_ = repository.Delete(request.Context(), key, user.ID)
				response.WriteError(writer, http.StatusInternalServerError, "CAD 转换服务未配置，无法生成初始版本")
				return
			}
			if _, convErr := convService.EnsureDwg(request.Context(), item); convErr != nil {
				_ = objectStorage.Delete(request.Context(), key)
				_ = repository.Delete(request.Context(), key, user.ID)
				log.Printf("[版本] 上传后初始版本转换失败，已回滚 storageKey=%s err=%v", item.StorageKey, convErr)
				response.WriteError(writer, http.StatusUnprocessableEntity, "CAD 初始版本转换失败："+convErr.Error())
				return
			}
		}

		// 登记初始版本（v1.0 基线 + 当前指针），保证后续编辑版本可回退。
		if versions != nil {
			if err := versions.EnsureInitialVersion(request.Context(), item.StorageKey, user.ID); err != nil {
				log.Printf("[版本] 登记初始版本失败 storageKey=%s: %v", item.StorageKey, err)
			}
		}

		response.WriteData(writer, http.StatusCreated, item)
	}
}

func AttachmentResource(pool *pgxpool.Pool, repository attachment.Repository, objectStorage storage.ObjectStorage) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		user, ok := middleware.UserFromContext(request.Context())
		if !ok {
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
		if err != nil && !errors.Is(err, attachment.ErrNotFound) {
			writeAttachmentError(writer, err)
			return
		}
		// 转换产物（如 EXB 转出的 DWG）没有附件记录；GET 时允许按对象存储直接下载，
		// 否则只读查看会因查无记录而 404。删除等变更操作仍要求记录存在。
		recordMissing := err != nil
		switch request.Method {
		case http.MethodGet:
			reader, object, openErr := objectStorage.Open(request.Context(), key)
			if openErr != nil {
				response.WriteError(writer, http.StatusNotFound, "附件文件不存在")
				return
			}
			defer reader.Close()
			fileName := filepath.Base(key)
			mimeType := firstNonEmpty(object.MimeType, "application/octet-stream")
			if !recordMissing {
				fileName = item.Name
				mimeType = firstNonEmpty(item.MimeType, object.MimeType, "application/octet-stream")
				// currentStorageKey 可能是 blobs/<hash>，对象键本身没有扩展名。
				// 如果请求的是当前对象，必须使用 currentName；否则会把 DWG 内容
				// 以原始 EXB 文件名返回，CAXA 会报“数据错误”。
				if key == item.CurrentStorageKey && strings.TrimSpace(item.CurrentName) != "" {
					fileName = item.CurrentName
					mimeType = firstNonEmpty(item.CurrentMimeType, item.MimeType, object.MimeType, "application/octet-stream")
				}
			}
			isDxf := strings.EqualFold(filepathExt(key), ".dxf") || (!recordMissing && strings.EqualFold(filepathExt(item.Name), ".dxf"))
			if isDxf {
				content, readErr := io.ReadAll(reader)
				if readErr != nil {
					response.WriteError(writer, http.StatusInternalServerError, "读取 DXF 文件失败")
					return
				}
				normalized, normalizeErr := cadtext.NormalizeDxfForCaxa(content)
				if normalizeErr != nil {
					response.WriteError(writer, http.StatusUnprocessableEntity, normalizeErr.Error())
					return
				}
				content = normalized
				writer.Header().Set("Content-Type", "application/dxf")
				writer.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s", url.PathEscape(fileName)))
				writer.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
				_, _ = writer.Write(content)
				return
			}
			writer.Header().Set("Content-Type", mimeType)
			writer.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s", url.PathEscape(fileName)))
			writer.Header().Set("Content-Length", fmt.Sprintf("%d", object.Size))
			_, _ = io.Copy(writer, reader)
		case http.MethodPatch:
			identity, ok := repository.(attachment.IdentityRepository)
			if !ok {
				response.WriteError(writer, http.StatusNotImplemented, "附件元数据更新尚未配置")
				return
			}
			attachmentID := strings.TrimSpace(request.URL.Query().Get("attachmentId"))
			if attachmentID == "" {
				response.WriteError(writer, http.StatusBadRequest, "更新附件元数据必须提供 attachmentId")
				return
			}
			var input struct {
				Author           string `json:"author"`
				PrimaryModel     *bool  `json:"primaryModel"`
				ExpectedRevision int64  `json:"expectedRevision"`
			}
			decoder := json.NewDecoder(request.Body)
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&input); err != nil || len([]rune(strings.TrimSpace(input.Author))) > 100 {
				response.WriteError(writer, http.StatusBadRequest, "附件元数据格式无效")
				return
			}
			current, err := identity.FindByID(request.Context(), attachmentID)
			if err != nil {
				writeAttachmentError(writer, err)
				return
			}
			if current.StorageKey != key && current.CurrentStorageKey != key {
				response.WriteError(writer, http.StatusNotFound, "附件不存在")
				return
			}
			if input.PrimaryModel != nil {
				if !*input.PrimaryModel || input.ExpectedRevision < 1 || input.Author != "" {
					response.WriteError(writer, http.StatusBadRequest, "主模型参数无效")
					return
				}
				models, ok := repository.(interface {
					SetPrimaryModel(context.Context, string, int64, string) (attachment.Attachment, error)
				})
				if !ok {
					response.WriteError(writer, http.StatusNotImplemented, "主模型管理尚未配置")
					return
				}
				updated, err := models.SetPrimaryModel(request.Context(), attachmentID, input.ExpectedRevision, user.ID)
				if errors.Is(err, attachment.ErrModelPermission) {
					response.WriteError(writer, http.StatusForbidden, err.Error())
					return
				}
				if err != nil {
					writeAttachmentError(writer, err)
					return
				}
				response.WriteData(writer, http.StatusOK, updated)
				return
			}
			if current.Role != attachment.RoleMaterial && current.Role != attachment.RoleCraft {
				response.WriteError(writer, http.StatusBadRequest, "只有备料表和工艺文件支持更新编制人")

				return
			}
			if err := identity.UpdateAuthorByID(request.Context(), attachmentID, input.Author); err != nil {
				writeAttachmentError(writer, err)
				return
			}
			current.Author = strings.TrimSpace(input.Author)
			response.WriteData(writer, http.StatusOK, current)
		case http.MethodDelete:
			identity, hasIdentity := repository.(attachment.IdentityRepository)
			if !hasIdentity {
				response.WriteError(writer, http.StatusNotImplemented, "附件删除尚未配置")
				return
			}
			attachmentID := strings.TrimSpace(request.URL.Query().Get("attachmentId"))
			if attachmentID == "" {
				response.WriteError(writer, http.StatusBadRequest, "删除附件必须提供 attachmentId")
				return
			}
			item, findErr := identity.FindByID(request.Context(), attachmentID)
			if findErr != nil {
				writeAttachmentError(writer, findErr)
				return
			}
			if item.StorageKey != key && item.CurrentStorageKey != key {
				response.WriteError(writer, http.StatusNotFound, "附件不存在")
				return
			}
			ownerID, archived, permissionErr := attachmentDeletionPermission(request.Context(), pool, attachmentID)
			if permissionErr != nil {
				response.WriteError(writer, http.StatusInternalServerError, "图纸文件删除权限读取失败")
				return
			}
			if statusCode, message := attachmentDeletionDecision(ownerID, archived, user.ID, hasAdminRole(user.Roles)); statusCode != http.StatusOK {
				response.WriteError(writer, statusCode, message)
				return
			}
			var deleted attachment.Attachment
			deleted, err = identity.DeleteByID(request.Context(), attachmentID, "")

			if err != nil {
				writeAttachmentError(writer, err)
				return
			}
			// Blob 是可共享内容对象；只有没有任何活动附件引用时才物理删除。
			keys := []string{key}
			if deleted.StorageKey != "" {
				keys = append(keys, deleted.StorageKey, deleted.CurrentStorageKey)
			}
			seen := map[string]struct{}{}
			for _, objectKey := range keys {
				if objectKey == "" {
					continue
				}
				if _, ok := seen[objectKey]; ok {
					continue
				}
				seen[objectKey] = struct{}{}
				if hasIdentity {
					referenced, refErr := identity.HasStorageKeyReference(request.Context(), objectKey)
					if refErr != nil {
						response.WriteError(writer, http.StatusInternalServerError, "附件引用检查失败")
						return
					}
					if referenced {
						continue
					}
				}
				if deleteErr := objectStorage.Delete(request.Context(), objectKey); deleteErr != nil {
					response.WriteError(writer, http.StatusInternalServerError, "附件文件删除失败")
					return
				}
			}
			response.WriteData(writer, http.StatusOK, nil)
		default:
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

func attachmentDeletionDecision(ownerID string, archived bool, userID string, admin bool) (int, string) {
	if archived {
		return http.StatusConflict, "图纸已存档，不能删除图纸文件"
	}
	if !admin && ownerID != userID {
		return http.StatusForbidden, "只有图纸创建者或管理员可以删除图纸文件"
	}
	return http.StatusOK, ""
}

func attachmentDeletionPermission(ctx context.Context, pool *pgxpool.Pool, attachmentID string) (string, bool, error) {
	if pool == nil {
		return "", false, errors.New("数据库连接未配置")
	}
	var ownerID, status string
	err := pool.QueryRow(ctx, `
		SELECT COALESCE(d.created_by::text, parent.created_by::text, ''), COALESCE(d.status::text, parent.status::text, '')
		FROM attachments a
		LEFT JOIN drawings d ON d.id = a.drawing_id
		LEFT JOIN parts p ON p.id = a.part_id
		LEFT JOIN drawing_part_relations owner_relation ON owner_relation.part_id = p.id AND owner_relation.relation_type = 'owned' AND owner_relation.status = 'active'
		LEFT JOIN drawings parent ON parent.id = owner_relation.drawing_id
		WHERE a.id = $1::uuid AND a.deleted_at IS NULL
		ORDER BY owner_relation.created_at NULLS FIRST
		LIMIT 1`, attachmentID).Scan(&ownerID, &status)
	if err != nil {
		return "", false, err
	}
	return ownerID, status == "archived", nil
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
	log.Printf("attachment request failed: %v", err)
	switch {
	case errors.Is(err, attachment.ErrNotFound):
		response.WriteError(writer, http.StatusNotFound, "附件不存在")
	case errors.Is(err, attachment.ErrConflict):
		response.WriteError(writer, http.StatusConflict, "附件已存在")
	case strings.Contains(err.Error(), "SQLSTATE 23505") || strings.Contains(err.Error(), "重复键违反唯一约束"):
		response.WriteError(writer, http.StatusConflict, "附件存储键已存在")
	default:
		response.WriteError(writer, http.StatusInternalServerError, "附件处理失败")
	}
}

func samePartNo(left *string, right string) bool {
	if left == nil {
		return strings.TrimSpace(right) == ""
	}
	return strings.TrimSpace(*left) == strings.TrimSpace(right)
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
