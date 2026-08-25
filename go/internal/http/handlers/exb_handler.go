package handlers

import (
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"cadguanliq/internal/attachment"
	"cadguanliq/internal/converter"
	"cadguanliq/internal/exb"
	"cadguanliq/internal/http/middleware"
	"cadguanliq/internal/response"
	"cadguanliq/internal/storage"
)

type exbParseRequest struct {
	StorageKey string `json:"storageKey"`
}

func ParseEXB(repository attachment.Repository, objectStorage storage.ObjectStorage) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if _, ok := middleware.UserFromContext(request.Context()); !ok {
			response.WriteError(writer, http.StatusUnauthorized, "登录已失效，请重新登录")
			return
		}
		if request.Method != http.MethodPost {
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		var input exbParseRequest
		if err := decodeJSON(request, &input); err != nil || strings.TrimSpace(input.StorageKey) == "" {
			response.WriteError(writer, http.StatusBadRequest, "storageKey 必填且请求格式有效")
			return
		}
		if !strings.EqualFold(filepathExt(input.StorageKey), ".exb") {
			response.WriteError(writer, http.StatusBadRequest, "只支持 EXB 文件")
			return
		}
		if _, err := repository.Find(request.Context(), input.StorageKey); err != nil {
			if errors.Is(err, attachment.ErrNotFound) {
				response.WriteError(writer, http.StatusNotFound, "附件不存在")
			} else {
				response.WriteError(writer, http.StatusInternalServerError, "查询附件失败")
			}
			return
		}

		reader, _, err := objectStorage.Open(request.Context(), input.StorageKey)
		if err != nil {
			response.WriteError(writer, http.StatusNotFound, "附件文件不存在")
			return
		}
		defer reader.Close()
		data, err := io.ReadAll(reader)
		if err != nil {
			response.WriteError(writer, http.StatusInternalServerError, "读取 EXB 文件失败")
			return
		}
		result, err := exb.Parse(data)
		if err != nil {
			response.WriteError(writer, http.StatusUnprocessableEntity, "EXB 文件解析失败")
			return
		}
		response.WriteData(writer, http.StatusOK, result)
	}
}

// PreviewEXB 提取并直接输出 EXB/DWG 对应的高清矢量 DXF，若未转换完成则触发高优先级插队并等待
func PreviewEXB(repository attachment.Repository, objectStorage storage.ObjectStorage, convService *converter.Service) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if _, ok := middleware.UserFromContext(request.Context()); !ok {
			response.WriteError(writer, http.StatusUnauthorized, "登录已失效，请重新登录")
			return
		}
		if request.Method != http.MethodGet {
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		storageKey := strings.TrimSpace(request.URL.Query().Get("storageKey"))
		if storageKey != "" && !strings.EqualFold(filepathExt(storageKey), ".exb") && !strings.EqualFold(filepathExt(storageKey), ".dwg") && !strings.EqualFold(filepathExt(storageKey), ".dxf") {
			response.WriteError(writer, http.StatusBadRequest, "只支持 EXB/DWG/DXF 文件")
			return
		}

		var item attachment.Attachment
		var err error
		if storageKey != "" {
			item, err = repository.Find(request.Context(), storageKey)
		} else {
			fileName := strings.TrimSpace(request.URL.Query().Get("fileName"))
			drawingNo := strings.TrimSpace(request.URL.Query().Get("drawingNo"))
			partNo := strings.TrimSpace(request.URL.Query().Get("partNo"))
			if fileName == "" || drawingNo == "" {
				response.WriteError(writer, http.StatusBadRequest, "storageKey 或 drawingNo、fileName 必填")
				return
			}
			item, err = repository.FindByOwnerAndName(request.Context(), drawingNo, partNo, fileName)
			storageKey = item.StorageKey
		}
		if err != nil {
			if errors.Is(err, attachment.ErrNotFound) {
				response.WriteError(writer, http.StatusNotFound, "附件不存在")
			} else {
				response.WriteError(writer, http.StatusInternalServerError, "查询附件失败")
			}
			return
		}

		ext := filepathExt(storageKey)
		if ext == "" {
			ext = filepathExt(item.Name)
		}

		dxfKey := strings.TrimSuffix(storageKey, ext) + ".dxf"
		if strings.EqualFold(ext, ".dxf") {
			dxfKey = storageKey
		}

		// 检查是否已有 DXF（确保大于 0 字节且有效），如果没有或为 0 字节，触发高优先级转换
		needConvert := false
		if reader, info, err := objectStorage.Open(request.Context(), dxfKey); err == nil {
			reader.Close()
			if info.Size == 0 {
				_ = objectStorage.Delete(request.Context(), dxfKey)
				needConvert = true
			}
		} else {
			needConvert = true
		}

		if needConvert {
			if convService != nil && (strings.EqualFold(ext, ".exb") || strings.EqualFold(ext, ".dwg")) {
				doneChan := convService.PushJob(item, converter.PriorityHigh)
				select {
				case convertErr := <-doneChan:
					if convertErr != nil {
						response.WriteError(writer, http.StatusUnprocessableEntity, convertErr.Error())
						return
					}
				case <-time.After(60 * time.Second):
					response.WriteError(writer, http.StatusGatewayTimeout, "CAD 转换 DXF 超时")
					return
				}
			}
		}

		// 读取 DXF
		reader, obj, err := objectStorage.Open(request.Context(), dxfKey)
		if err != nil || obj.Size == 0 {
			if reader != nil {
				reader.Close()
			}
			response.WriteError(writer, http.StatusNotFound, "高清矢量 DXF 文件未就绪或为空")
			return
		}
		defer reader.Close()

		writer.Header().Set("Content-Type", "application/dxf; charset=utf-8")
		writer.Header().Set("Content-Disposition", "inline; filename*=UTF-8''"+filepath.Base(dxfKey))
		writer.Header().Set("Cache-Control", "private, max-age=86400")
		writer.WriteHeader(http.StatusOK)
		_, _ = io.Copy(writer, reader)
		_ = obj
	}
}

func filepathExt(value string) string {
	value = strings.ReplaceAll(value, "\\", "/")
	index := strings.LastIndexByte(value, '.')
	if index < 0 {
		return ""
	}
	return value[index:]
}
