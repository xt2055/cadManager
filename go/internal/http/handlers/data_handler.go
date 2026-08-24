package handlers

import (
	"encoding/json"
	"net/http"

	"cadguanliq/internal/data"
	"cadguanliq/internal/http/middleware"
	"cadguanliq/internal/response"
)

func DataDocument(repository *data.DocumentRepository) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		switch request.Method {
		case http.MethodGet:
			document, err := repository.Load(request.Context())
			if err != nil {
				response.WriteError(writer, http.StatusInternalServerError, "业务数据读取失败")
				return
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
			if err := repository.Save(request.Context(), document, user.ID); err != nil {
				response.WriteError(writer, http.StatusInternalServerError, "业务数据保存失败")
				return
			}
			response.WriteData(writer, http.StatusOK, nil)
		default:
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}
