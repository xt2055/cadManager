package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"cadguanliq/internal/http/middleware"
	"cadguanliq/internal/response"
	"cadguanliq/internal/review"
)

// ReviewCases 审核案例集合接口：GET 列表 / POST 发起审核。
func ReviewCases(repository review.Repository) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		user, ok := middleware.UserFromContext(request.Context())
		if !ok {
			response.WriteError(writer, http.StatusUnauthorized, "请先登录")
			return
		}
		switch request.Method {
		case http.MethodGet:
			items, err := repository.ListCases(request.Context())
			if err != nil {
				writeCaseError(writer, err)
				return
			}
			response.WriteData(writer, http.StatusOK, items)
		case http.MethodPost:
			var input review.StartCaseInput
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				response.WriteError(writer, http.StatusBadRequest, "发起审核参数格式无效")
				return
			}
			if strings.TrimSpace(input.DrawingNo) == "" {
				response.WriteError(writer, http.StatusBadRequest, "图纸图号不能为空")
				return
			}
			item, err := repository.StartCase(request.Context(), strings.TrimSpace(input.DrawingNo), user.ID)
			if err != nil {
				writeCaseError(writer, err)
				return
			}
			response.WriteData(writer, http.StatusCreated, item)
		default:
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

// ReviewCaseResource 审核案例子资源：/api/review-cases/completed 与 /api/review-cases/{id}/submit。
func ReviewCaseResource(repository review.Repository) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		user, ok := middleware.UserFromContext(request.Context())
		if !ok {
			response.WriteError(writer, http.StatusUnauthorized, "请先登录")
			return
		}
		path := strings.Trim(strings.TrimPrefix(request.URL.Path, "/api/review-cases/"), "/")

		if path == "completed" {
			if request.Method != http.MethodGet {
				response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			items, err := repository.CompletedActions(request.Context())
			if err != nil {
				writeCaseError(writer, err)
				return
			}
			response.WriteData(writer, http.StatusOK, items)
			return
		}

		if strings.HasSuffix(path, "/submit") {
			if request.Method != http.MethodPost {
				response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			caseID := strings.TrimSuffix(path, "/submit")
			if caseID == "" || strings.Contains(caseID, "/") {
				response.WriteError(writer, http.StatusNotFound, "审核案例不存在")
				return
			}
			var input review.SubmitNodeInput
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				response.WriteError(writer, http.StatusBadRequest, "签署参数格式无效")
				return
			}
			if strings.TrimSpace(input.NodeName) == "" {
				response.WriteError(writer, http.StatusBadRequest, "审核节点名称不能为空")
				return
			}
			item, err := repository.SubmitNode(request.Context(), caseID, input, user.ID)
			if err != nil {
				writeCaseError(writer, err)
				return
			}
			response.WriteData(writer, http.StatusOK, item)
			return
		}

		response.WriteError(writer, http.StatusNotFound, "审核案例接口不存在")
	}
}

func writeCaseError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, review.ErrCaseNotFound):
		response.WriteError(writer, http.StatusNotFound, "审核案例不存在")
	case errors.Is(err, review.ErrCaseConflict):
		response.WriteError(writer, http.StatusConflict, "该图纸已有进行中的审核流程")
	case errors.Is(err, review.ErrCaseForbidden):
		response.WriteError(writer, http.StatusForbidden, "无权执行该审核操作")
	default:
		// 业务校验错误（顺序、责任人、配置缺失等）直接返回具体原因。
		response.WriteError(writer, http.StatusBadRequest, err.Error())
	}
}
