package handlers

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"

	"cadguanliq/internal/change"
	"cadguanliq/internal/http/middleware"
	"cadguanliq/internal/response"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// drawingArchivedByNo 查询图号对应图纸是否处于存档保护状态，供各写入入口统一门禁。
func drawingArchivedByNo(ctx context.Context, pool *pgxpool.Pool, drawingNo string) (bool, error) {
	var archived bool
	err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM drawings WHERE drawing_no = $1 AND status = 'archived')`, strings.TrimSpace(drawingNo)).Scan(&archived)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return archived, err
}

type changeCreateRequest struct {
	DrawingID      string   `json:"drawingId"`
	Reason         string   `json:"reason"`
	Scope          string   `json:"scope"`
	Title          string   `json:"title"`
	ExecutorID     string   `json:"executorId"`
	RequireVerify  *bool    `json:"requireVerify"`
	AutoApprove    bool     `json:"autoApprove"`
	ApproveOpinion string   `json:"approveOpinion"`
	WaiveReason    string   `json:"waiveReason"`
	AttachmentIDs  []string `json:"attachmentIds"`
}

type changeApproveRequest struct {
	Opinion       string `json:"opinion"`
	RequireVerify *bool  `json:"requireVerify"`
	WaiveReason   string `json:"waiveReason"`
}

type changeSubmitRequest struct {
	ActualChanges string `json:"actualChanges"`
	Proposed      struct {
		Name     *string `json:"name"`
		Material *string `json:"material"`
		Vendor   *string `json:"vendor"`
		Version  *string `json:"version"`
	} `json:"proposedAttributes"`
}

type changeDecisionRequest struct {
	Opinion      string `json:"opinion"`
	SubmissionID string `json:"submissionId"`
}

// ChangeRequests 处理 /api/change-requests 集合：创建与列表。
func ChangeRequests(service change.Service) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		user, ok := middleware.UserFromContext(request.Context())
		if !ok {
			response.WriteError(writer, http.StatusUnauthorized, "登录已失效，请重新登录")
			return
		}
		switch request.Method {
		case http.MethodGet:
			filter := change.ListFilter{
				DrawingID: request.URL.Query().Get("drawing_id"),
				Status:    change.Status(request.URL.Query().Get("status")),
				OpenOnly:  request.URL.Query().Get("open") == "1",
			}
			if request.URL.Query().Get("mine") == "1" {
				filter.ExecutorID = user.ID
			}
			items, err := service.List(request.Context(), filter)
			if err != nil {
				response.WriteError(writer, http.StatusInternalServerError, "变更工单列表读取失败")
				return
			}
			response.WriteData(writer, http.StatusOK, items)
		case http.MethodPost:
			var payload changeCreateRequest
			if err := decodeJSON(request, &payload); err != nil {
				response.WriteError(writer, http.StatusBadRequest, "变更申请参数格式无效")
				return
			}
			if strings.TrimSpace(payload.DrawingID) == "" {
				response.WriteError(writer, http.StatusBadRequest, "缺少目标图纸")
				return
			}
			item, err := service.Create(request.Context(), user, strings.TrimSpace(payload.DrawingID), change.CreateInput{
				Reason:         payload.Reason,
				Scope:          payload.Scope,
				Title:          payload.Title,
				ExecutorID:     payload.ExecutorID,
				RequireVerify:  payload.RequireVerify,
				AutoApprove:    payload.AutoApprove,
				ApproveOpinion: payload.ApproveOpinion,
				WaiveReason:    payload.WaiveReason,
				AttachmentIDs:  payload.AttachmentIDs,
			})
			if err != nil {
				writeChangeError(writer, err)
				return
			}
			response.WriteData(writer, http.StatusCreated, item)
		default:
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

// ChangeRequestResource 处理 /api/change-requests/{id}[/{action}]。
func ChangeRequestResource(service change.Service) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		user, ok := middleware.UserFromContext(request.Context())
		if !ok {
			response.WriteError(writer, http.StatusUnauthorized, "登录已失效，请重新登录")
			return
		}
		path := strings.Trim(strings.TrimPrefix(request.URL.Path, "/api/change-requests/"), "/")
		segments := strings.Split(path, "/")
		id := segments[0]
		if id == "" {
			response.WriteError(writer, http.StatusNotFound, "变更工单不存在")
			return
		}
		if request.Method == http.MethodGet {
			item, err := service.Get(request.Context(), id)
			if err != nil {
				writeChangeError(writer, err)
				return
			}
			response.WriteData(writer, http.StatusOK, item)
			return
		}
		if len(segments) != 2 || request.Method != http.MethodPost {
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var (
			item change.Request
			err  error
		)
		switch segments[1] {
		case "approve":
			var payload changeApproveRequest
			if err = decodeJSON(request, &payload); err != nil {
				response.WriteError(writer, http.StatusBadRequest, "审批参数格式无效")
				return
			}
			input := change.ApproveInput{Opinion: payload.Opinion, RequireVerify: true, WaiveReason: payload.WaiveReason}
			if payload.RequireVerify != nil {
				input.RequireVerify = *payload.RequireVerify
			}
			item, err = service.Approve(request.Context(), user, id, input)
		case "reject":
			var payload changeDecisionRequest
			if err = decodeJSON(request, &payload); err != nil {
				response.WriteError(writer, http.StatusBadRequest, "驳回参数格式无效")
				return
			}
			item, err = service.Reject(request.Context(), user, id, change.DecisionInput{Opinion: payload.Opinion})
		case "submit":
			var payload changeSubmitRequest
			if err = decodeJSON(request, &payload); err != nil {
				response.WriteError(writer, http.StatusBadRequest, "提交参数格式无效")
				return
			}
			item, err = service.Submit(request.Context(), user, id, change.SubmitInput{
				ActualChanges: payload.ActualChanges,
				Proposed: change.ProposedAttributes{
					Name:     payload.Proposed.Name,
					Material: payload.Proposed.Material,
					Vendor:   payload.Proposed.Vendor,
					Version:  payload.Proposed.Version,
				},
			})
		case "verify":
			var payload changeDecisionRequest
			if err = decodeJSON(request, &payload); err != nil {
				response.WriteError(writer, http.StatusBadRequest, "验收参数格式无效")
				return
			}
			item, err = service.Verify(request.Context(), user, id, change.DecisionInput{Opinion: payload.Opinion, SubmissionID: strings.TrimSpace(payload.SubmissionID)})
		case "return":
			var payload changeDecisionRequest
			if err = decodeJSON(request, &payload); err != nil {
				response.WriteError(writer, http.StatusBadRequest, "退回参数格式无效")
				return
			}
			item, err = service.ReturnForEdit(request.Context(), user, id, change.DecisionInput{Opinion: payload.Opinion})
		case "cancel":
			var payload changeDecisionRequest
			if err = decodeJSON(request, &payload); err != nil {
				response.WriteError(writer, http.StatusBadRequest, "终止参数格式无效")
				return
			}
			item, err = service.Cancel(request.Context(), user, id, change.DecisionInput{Opinion: payload.Opinion})
		default:
			response.WriteError(writer, http.StatusNotFound, "变更工单操作不存在")
			return
		}
		if err != nil {
			log.Printf("[change-request] id=%s action=%s error=%v", id, segments[1], err)
			writeChangeError(writer, err)
			return
		}
		response.WriteData(writer, http.StatusOK, item)
	}
}

func writeChangeError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, change.ErrNotFound):
		response.WriteError(writer, http.StatusNotFound, "变更工单不存在")
	case errors.Is(err, change.ErrForbidden):
		response.WriteError(writer, http.StatusForbidden, err.Error())
	case errors.Is(err, change.ErrState):
		response.WriteError(writer, http.StatusConflict, "工单当前状态不允许该操作")
	case errors.Is(err, change.ErrOpenExists):
		response.WriteError(writer, http.StatusConflict, err.Error())
	case errors.Is(err, change.ErrNoArchive):
		response.WriteError(writer, http.StatusConflict, err.Error())
	case errors.Is(err, change.ErrStaleSubmit):
		response.WriteError(writer, http.StatusConflict, err.Error())
	case errors.Is(err, change.ErrActiveEditSession):
		response.WriteError(writer, http.StatusConflict, err.Error())
	default:
		if message := err.Error(); strings.Contains(message, "不能为空") || strings.Contains(message, "必须") || strings.Contains(message, "无效") {
			response.WriteError(writer, http.StatusBadRequest, message)
			return
		}
		response.WriteError(writer, http.StatusInternalServerError, "变更工单操作失败")
	}
}
