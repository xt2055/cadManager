package handlers

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"cadguanliq/internal/attachment"
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

func filepathExt(value string) string {
	value = strings.ReplaceAll(value, "\\", "/")
	index := strings.LastIndexByte(value, '.')
	if index < 0 {
		return ""
	}
	return value[index:]
}
