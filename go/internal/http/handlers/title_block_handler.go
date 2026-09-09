package handlers

import (
	"errors"
	"log"
	"net/http"
	"regexp"
	"strings"

	"cadguanliq/internal/http/middleware"
	"cadguanliq/internal/response"
	"cadguanliq/internal/titleblock"
)

var titleBlockID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func TitleBlocks(store titleblock.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := middleware.UserFromContext(r.Context())
		if !ok {
			response.WriteError(w, http.StatusUnauthorized, "登录已失效，请重新登录")
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/api/cad/title-blocks/")
		if !titleBlockID.MatchString(id) {
			response.WriteError(w, http.StatusBadRequest, "附件 ID 无效")
			return
		}
		id = strings.ToLower(id)
		admin := false
		for _, role := range user.Roles {
			if strings.EqualFold(role, "admin") {
				admin = true
			}
		}
		var err error
		switch r.Method {
		case http.MethodGet:
			var data titleblock.Snapshot
			data, err = store.Get(r.Context(), id, user.ID, admin)
			if err == nil {
				response.WriteData(w, http.StatusOK, data)
				return
			}
		case http.MethodPut:
			r.Body = http.MaxBytesReader(w, r.Body, 256<<10)
			var input struct {
				VersionID string             `json:"versionId"`
				Payload   titleblock.Payload `json:"payload"`
			}
			if decodeJSON(r, &input) != nil || !titleBlockID.MatchString(input.VersionID) || titleblock.Validate(input.Payload) != nil {
				response.WriteError(w, http.StatusBadRequest, "标题栏提取数据无效或超过限制")
				return
			}
			input.VersionID = strings.ToLower(input.VersionID)
			err = store.Save(r.Context(), id, input.VersionID, user.ID, admin, input.Payload)
			if err == nil {
				response.WriteData(w, http.StatusOK, map[string]bool{"saved": true})
				return
			}
		default:
			response.WriteError(w, http.StatusMethodNotAllowed, "不支持的请求方法")
			return
		}
		status := http.StatusInternalServerError
		message := "标题栏信息读写失败"
		switch {
		case errors.Is(err, titleblock.ErrNotFound):
			status = http.StatusNotFound
			message = err.Error()
		case errors.Is(err, titleblock.ErrForbidden):
			status = http.StatusForbidden
			message = err.Error()
		case errors.Is(err, titleblock.ErrConflict):
			status = http.StatusConflict
			message = err.Error()
		case errors.Is(err, titleblock.ErrInvalid):
			status = http.StatusBadRequest
			message = err.Error()
		}
		if status == http.StatusInternalServerError {
			log.Printf("[标题栏] %s attachment=%s: %v", r.Method, id, err)
		}
		response.WriteError(w, status, message)
	}
}
