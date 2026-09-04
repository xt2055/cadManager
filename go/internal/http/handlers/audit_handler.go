package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"cadguanliq/internal/audit"
	"cadguanliq/internal/http/middleware"
	"cadguanliq/internal/response"
)

func DrawingOperationLogs(repository audit.Repository) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		user, ok := middleware.UserFromContext(request.Context())
		if !ok {
			response.WriteError(writer, http.StatusUnauthorized, "登录已失效，请重新登录")
			return
		}
		switch request.Method {
		case http.MethodGet:
			filter := audit.ListFilter{Page: queryInt(request, "page", 1), PageSize: queryInt(request, "page_size", 50), Action: request.URL.Query().Get("action"), DrawingNo: request.URL.Query().Get("drawing_no")}
			if filter.Page < 1 {
				filter.Page = 1
			}
			if filter.PageSize < 1 || filter.PageSize > 100 {
				filter.PageSize = 50
			}
			page, err := repository.List(request.Context(), filter)
			if err != nil {
				response.WriteError(writer, http.StatusInternalServerError, "图纸操作日志读取失败")
				return
			}
			response.WriteData(writer, http.StatusOK, page)
		case http.MethodPost:
			var input audit.CreateInput
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				response.WriteError(writer, http.StatusBadRequest, "图纸操作日志参数格式无效")
				return
			}
			if strings.TrimSpace(input.DrawingNo) == "" || strings.TrimSpace(input.Action) == "" || strings.TrimSpace(input.Summary) == "" {
				response.WriteError(writer, http.StatusBadRequest, "图号、操作类型和操作描述不能为空")
				return
			}
			if !validAuditAction(input.Action) || !validAuditTarget(input.TargetType) {
				response.WriteError(writer, http.StatusBadRequest, "图纸操作类型或目标类型无效")
				return
			}
			if input.Result == "" {
				input.Result = "success"
			}
			item, err := repository.Create(request.Context(), input, user.ID, user.DisplayName, requestRemoteIP(request), request.UserAgent())
			if err != nil {
				response.WriteError(writer, http.StatusInternalServerError, "图纸操作日志保存失败")
				return
			}
			response.WriteData(writer, http.StatusCreated, item)
		default:
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

func AdminOperationLogs(repository audit.AdminRepository) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if _, ok := middleware.UserFromContext(request.Context()); !ok {
			response.WriteError(writer, http.StatusUnauthorized, "登录已失效，请重新登录")
			return
		}
		if request.Method != http.MethodGet {
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		filter := audit.ListFilter{
			Page:       queryInt(request, "page", 1),
			PageSize:   queryInt(request, "page_size", 20),
			Action:     strings.TrimSpace(request.URL.Query().Get("action")),
			DrawingNo:  strings.TrimSpace(request.URL.Query().Get("drawing_no")),
			TargetType: strings.TrimSpace(request.URL.Query().Get("target_type")),
			ActorID:    strings.TrimSpace(request.URL.Query().Get("actor_id")),
			Result:     strings.TrimSpace(request.URL.Query().Get("result")),
			Keyword:    strings.TrimSpace(request.URL.Query().Get("keyword")),
		}
		if value := strings.TrimSpace(request.URL.Query().Get("from")); value != "" {
			parsed, err := time.Parse("2006-01-02", value)
			if err != nil {
				response.WriteError(writer, http.StatusBadRequest, "开始日期格式无效")
				return
			}
			filter.From = parsed.UTC()
		}
		if value := strings.TrimSpace(request.URL.Query().Get("to")); value != "" {
			parsed, err := time.Parse("2006-01-02", value)
			if err != nil {
				response.WriteError(writer, http.StatusBadRequest, "结束日期格式无效")
				return
			}
			filter.To = parsed.UTC().AddDate(0, 0, 1)
		}
		if filter.Page < 1 {
			filter.Page = 1
		}
		if filter.PageSize < 1 || filter.PageSize > 100 {
			filter.PageSize = 20
		}
		if filter.Result != "" && filter.Result != "success" && filter.Result != "failed" {
			response.WriteError(writer, http.StatusBadRequest, "操作结果无效")
			return
		}
		page, err := repository.ListAdmin(request.Context(), filter)
		if err != nil {
			response.WriteError(writer, http.StatusInternalServerError, "管理员操作日志读取失败")
			return
		}
		response.WriteData(writer, http.StatusOK, page)
	}
}

func validAuditAction(action string) bool {
	switch action {
	case "view", "create", "edit", "branch", "upload", "download", "delete", "check", "parse":
		return true
	default:
		return false
	}
}

func validAuditTarget(target string) bool {
	switch target {
	case "drawing", "part", "file", "review", "branch":
		return true
	default:
		return false
	}
}

func requestRemoteIP(request *http.Request) string {
	if forwarded := strings.TrimSpace(strings.Split(request.Header.Get("X-Forwarded-For"), ",")[0]); forwarded != "" {
		return forwarded
	}
	if host := strings.TrimSpace(request.Header.Get("X-Real-IP")); host != "" {
		return host
	}
	host, _, _ := strings.Cut(request.RemoteAddr, ":")
	return host
}
