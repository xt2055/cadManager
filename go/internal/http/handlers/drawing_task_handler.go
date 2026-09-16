package handlers

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"

	"cadguanliq/internal/auth"
	"cadguanliq/internal/http/middleware"
	"cadguanliq/internal/response"
	"cadguanliq/internal/task"
)

// DrawingTaskService 是任务处理器依赖的最小接口。
// 用接口而不是具体类型，是为了让权限与参数校验能用假实现覆盖，
// 不需要为「设计人员不能指派」这类规则准备数据库。
type DrawingTaskService interface {
	ListBoard(ctx context.Context, filter task.ListFilter) (task.Page, error)
	ListMine(ctx context.Context, userID string, filter task.ListFilter) (task.Page, error)
	Assign(ctx context.Context, actor auth.AuthUser, input task.AssignInput) (task.Row, error)
	Update(ctx context.Context, actor auth.AuthUser, taskID string, input task.UpdateInput) (task.Row, error)
	Cancel(ctx context.Context, actor auth.AuthUser, taskID, reason string) error
	History(ctx context.Context, drawingID string) ([]task.HistoryEntry, error)
	Candidates(ctx context.Context) ([]task.Candidate, error)
}

// DrawingTasks 处理任务列表。scope=mine 是任何登录用户都能看的「我负责的图纸」，
// scope=board 是计划员的任务管理台。
func DrawingTasks(service DrawingTaskService) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		user, ok := middleware.UserFromContext(request.Context())
		if !ok {
			response.WriteError(writer, http.StatusUnauthorized, "登录已失效，请重新登录")
			return
		}
		if service == nil {
			response.WriteError(writer, http.StatusServiceUnavailable, "任务服务未配置")
			return
		}
		switch request.Method {
		case http.MethodGet:
			filter := task.ListFilter{
				Page:       queryInt(request, "page", 1),
				PageSize:   queryInt(request, "page_size", 20),
				Keyword:    request.URL.Query().Get("keyword"),
				Status:     request.URL.Query().Get("status"),
				Assigned:   request.URL.Query().Get("assigned"),
				AssigneeID: request.URL.Query().Get("assignee_id"),
			}
			scope := strings.TrimSpace(request.URL.Query().Get("scope"))
			if scope == "mine" {
				page, err := service.ListMine(request.Context(), user.ID, filter)
				if err != nil {
					writeTaskError(writer, err, "读取我的任务失败")
					return
				}
				response.WriteData(writer, http.StatusOK, page)
				return
			}
			if !canManageDrawingTasks(user) {
				response.WriteError(writer, http.StatusForbidden, "查看任务总表需要计划员或管理员权限；如需查看自己的任务请使用 scope=mine")
				return
			}
			page, err := service.ListBoard(request.Context(), filter)
			if err != nil {
				writeTaskError(writer, err, "读取任务总表失败")
				return
			}
			response.WriteData(writer, http.StatusOK, page)
		case http.MethodPost:
			if !canManageDrawingTasks(user) {
				response.WriteError(writer, http.StatusForbidden, task.ErrForbidden.Error())
				return
			}
			var input task.AssignInput
			if err := decodeJSON(request, &input); err != nil {
				response.WriteError(writer, http.StatusBadRequest, "指派参数格式无效")
				return
			}
			item, err := service.Assign(request.Context(), user, input)
			if err != nil {
				writeTaskError(writer, err, "指派图纸负责人失败")
				return
			}
			response.WriteData(writer, http.StatusCreated, item)
		default:
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
		}
	})
}

// DrawingTaskCandidates 返回可被指派的人员名单（计划员与管理员）。
func DrawingTaskCandidates(service DrawingTaskService) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		user, ok := middleware.UserFromContext(request.Context())
		if !ok {
			response.WriteError(writer, http.StatusUnauthorized, "登录已失效，请重新登录")
			return
		}
		if request.Method != http.MethodGet {
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !canManageDrawingTasks(user) {
			response.WriteError(writer, http.StatusForbidden, task.ErrForbidden.Error())
			return
		}
		items, err := service.Candidates(request.Context())
		if err != nil {
			writeTaskError(writer, err, "读取负责人候选名单失败")
			return
		}
		response.WriteData(writer, http.StatusOK, items)
	})
}

// DrawingTaskResource 处理 /api/drawing-tasks/{taskId} 的改派、说明维护与取消，
// 以及 /api/drawing-tasks/{drawingId}/history 的指派历史。
func DrawingTaskResource(service DrawingTaskService) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		user, ok := middleware.UserFromContext(request.Context())
		if !ok {
			response.WriteError(writer, http.StatusUnauthorized, "登录已失效，请重新登录")
			return
		}
		if service == nil {
			response.WriteError(writer, http.StatusServiceUnavailable, "任务服务未配置")
			return
		}
		path := strings.Trim(strings.TrimPrefix(request.URL.Path, "/api/drawing-tasks/"), "/")
		parts := strings.Split(path, "/")
		if len(parts) == 0 || parts[0] == "" {
			response.WriteError(writer, http.StatusNotFound, "任务不存在")
			return
		}
		identifier := parts[0]
		if len(parts) == 2 && parts[1] == "history" {
			if request.Method != http.MethodGet {
				response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			if !canManageDrawingTasks(user) {
				response.WriteError(writer, http.StatusForbidden, task.ErrForbidden.Error())
				return
			}
			items, err := service.History(request.Context(), identifier)
			if err != nil {
				writeTaskError(writer, err, "读取指派历史失败")
				return
			}
			response.WriteData(writer, http.StatusOK, items)
			return
		}
		if len(parts) != 1 {
			response.WriteError(writer, http.StatusNotFound, "任务接口不存在")
			return
		}
		if !canManageDrawingTasks(user) {
			response.WriteError(writer, http.StatusForbidden, task.ErrForbidden.Error())
			return
		}
		switch request.Method {
		case http.MethodPut:
			var input task.UpdateInput
			if err := decodeJSON(request, &input); err != nil {
				response.WriteError(writer, http.StatusBadRequest, "改派参数格式无效")
				return
			}
			item, err := service.Update(request.Context(), user, identifier, input)
			if err != nil {
				writeTaskError(writer, err, "改派图纸负责人失败")
				return
			}
			response.WriteData(writer, http.StatusOK, item)
		case http.MethodDelete:
			if err := service.Cancel(request.Context(), user, identifier, request.URL.Query().Get("reason")); err != nil {
				writeTaskError(writer, err, "取消指派失败")
				return
			}
			response.WriteData(writer, http.StatusOK, nil)
		default:
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
		}
	})
}

// writeTaskError 把领域错误映射成状态码。
// 所有业务错误都带可操作的下一步，不返回「操作失败」这类无法行动的信息。
func writeTaskError(writer http.ResponseWriter, err error, fallback string) {
	log.Printf("drawing task request failed: %v", err)
	switch {
	case errors.Is(err, task.ErrForbidden):
		response.WriteError(writer, http.StatusForbidden, err.Error())
	case errors.Is(err, task.ErrNotFound):
		response.WriteError(writer, http.StatusNotFound, err.Error())
	case errors.Is(err, task.ErrConflict):
		response.WriteError(writer, http.StatusConflict, err.Error())
	case errors.Is(err, task.ErrInvalidAssignee):
		response.WriteError(writer, http.StatusBadRequest, err.Error())
	case errors.Is(err, task.ErrArchivedDrawing):
		response.WriteError(writer, http.StatusConflict, err.Error())
	case errors.Is(err, task.ErrInvalidInput):
		response.WriteError(writer, http.StatusBadRequest, err.Error())
	default:
		if message := err.Error(); strings.Contains(message, "格式无效") || strings.Contains(message, "不完整") {
			response.WriteError(writer, http.StatusBadRequest, message)
			return
		}
		response.WriteError(writer, http.StatusInternalServerError, fallback)
	}
}
