package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"cadguanliq/internal/http/middleware"
	"cadguanliq/internal/response"
	"cadguanliq/internal/review"
)

func ReviewFlows(repository review.Repository) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		user, ok := middleware.UserFromContext(request.Context())
		if !ok || !hasAdminRole(user.Roles) {
			response.WriteError(writer, http.StatusForbidden, "只有管理员可以管理审核流程")
			return
		}
		switch request.Method {
		case http.MethodGet:
			items, err := repository.List(request.Context())
			if err != nil {
				log.Printf("review flow list failed: %v", err)
				response.WriteError(writer, http.StatusInternalServerError, "审核流程读取失败")
				return
			}
			response.WriteData(writer, http.StatusOK, items)
		case http.MethodPost:
			var input review.SaveFlowInput
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				response.WriteError(writer, http.StatusBadRequest, "审核流程参数格式无效")
				return
			}
			if strings.TrimSpace(input.Name) == "" {
				response.WriteError(writer, http.StatusBadRequest, "审核流程名称不能为空")
				return
			}
			item, err := repository.Create(request.Context(), input, user.ID)
			if err != nil {
				writeReviewError(writer, err)
				return
			}
			response.WriteData(writer, http.StatusCreated, item)
		default:
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

func ReviewFlowResource(repository review.Repository) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		user, ok := middleware.UserFromContext(request.Context())
		if !ok || !hasAdminRole(user.Roles) {
			response.WriteError(writer, http.StatusForbidden, "只有管理员可以管理审核流程")
			return
		}
		id := strings.Trim(strings.TrimPrefix(request.URL.Path, "/api/review-flows/"), "/")
		if id == "" || strings.Contains(id, "/") {
			response.WriteError(writer, http.StatusNotFound, "审核流程不存在")
			return
		}
		switch request.Method {
		case http.MethodGet:
			item, err := repository.Find(request.Context(), id)
			if err != nil {
				writeReviewError(writer, err)
				return
			}
			response.WriteData(writer, http.StatusOK, item)
		case http.MethodPut, http.MethodPatch:
			var input review.SaveFlowInput
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				response.WriteError(writer, http.StatusBadRequest, "审核流程参数格式无效")
				return
			}
			item, err := repository.Update(request.Context(), id, input, user.ID)
			if err != nil {
				writeReviewError(writer, err)
				return
			}
			response.WriteData(writer, http.StatusOK, item)
		case http.MethodPost:
			if request.URL.Query().Get("action") != "toggle" {
				response.WriteError(writer, http.StatusBadRequest, "不支持的审核流程操作")
				return
			}
			var payload struct {
				Enabled bool `json:"enabled"`
			}
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				response.WriteError(writer, http.StatusBadRequest, "审核流程状态参数无效")
				return
			}
			item, err := repository.SetEnabled(request.Context(), id, payload.Enabled, user.ID)
			if err != nil {
				writeReviewError(writer, err)
				return
			}
			response.WriteData(writer, http.StatusOK, item)
		default:
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

func hasAdminRole(roles []string) bool {
	for _, role := range roles {
		if role == "admin" {
			return true
		}
	}
	return false
}

func writeReviewError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, review.ErrNotFound):
		response.WriteError(writer, http.StatusNotFound, "审核流程不存在")
	case errors.Is(err, review.ErrConflict):
		response.WriteError(writer, http.StatusConflict, "审核流程名称已存在")
	default:
		response.WriteError(writer, http.StatusInternalServerError, "审核流程保存失败")
	}
}
