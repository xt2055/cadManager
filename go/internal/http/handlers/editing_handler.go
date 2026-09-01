package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"cadguanliq/internal/editing"
	"cadguanliq/internal/http/middleware"
	"cadguanliq/internal/response"
)

func EditSession(service *editing.Service) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		user, ok := middleware.UserFromContext(request.Context())
		if !ok {
			response.WriteError(writer, http.StatusUnauthorized, "登录已失效，请重新登录")
			return
		}
		switch request.Method {
		case http.MethodGet:
			drawingNo := strings.TrimSpace(request.URL.Query().Get("drawingNo"))
			list, err := service.ListActive(request.Context(), user, drawingNo)
			if err != nil {
				writeEditingError(writer, err)
				return
			}
			response.WriteData(writer, http.StatusOK, list)
		case http.MethodPost:
			var input struct {
				StorageKey string `json:"storageKey"`
			}
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil || strings.TrimSpace(input.StorageKey) == "" {
				response.WriteError(writer, http.StatusBadRequest, "缺少 CAD 文件存储键")
				return
			}
			result, err := service.Open(request.Context(), user, strings.TrimSpace(input.StorageKey))
			if err != nil {
				log.Printf("[编辑会话] 创建失败 user=%s storageKey=%q err=%v", user.ID, strings.TrimSpace(input.StorageKey), err)
				writeEditingError(writer, err)
				return
			}
			log.Printf("[编辑会话] 创建成功 user=%s sessionId=%s storageKey=%q", user.ID, result.SessionID, input.StorageKey)
			response.WriteData(writer, http.StatusCreated, result)
		default:
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

func EditTicketExchange(service *editing.Service) http.HandlerFunc {
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
		var input struct {
			Ticket string `json:"ticket"`
		}
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil || strings.TrimSpace(input.Ticket) == "" {
			response.WriteError(writer, http.StatusBadRequest, "缺少打开票据")
			return
		}
		result, err := service.Exchange(request.Context(), user, strings.TrimSpace(input.Ticket))
		if err != nil {
			writeEditingError(writer, err)
			return
		}
		response.WriteData(writer, http.StatusOK, result)
	}
}

func EditSessionResource(service *editing.Service) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		user, ok := middleware.UserFromContext(request.Context())
		if !ok {
			response.WriteError(writer, http.StatusUnauthorized, "登录已失效，请重新登录")
			return
		}
		sessionID := strings.Trim(strings.TrimPrefix(request.URL.Path, "/api/edit-sessions/"), "/")
		if sessionID == "" {
			response.WriteError(writer, http.StatusNotFound, "编辑会话不存在")
			return
		}
		var err error
		switch request.Method {
		case http.MethodPost:
			if strings.HasSuffix(request.URL.Path, "/heartbeat") {
				sessionID = strings.TrimSuffix(sessionID, "/heartbeat")
				err = service.Heartbeat(request.Context(), user, sessionID)
			} else if strings.HasSuffix(request.URL.Path, "/close") {
				sessionID = strings.TrimSuffix(sessionID, "/close")
				err = service.Close(request.Context(), user, sessionID)
			} else {
				response.WriteError(writer, http.StatusNotFound, "编辑会话操作不存在")
				return
			}
		default:
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if err != nil {
			writeEditingError(writer, err)
			return
		}
		response.WriteData(writer, http.StatusOK, nil)
	}
}

func EditReadOnly(service *editing.Service) http.HandlerFunc {
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
		var input struct {
			StorageKey string `json:"storageKey"`
		}
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil || strings.TrimSpace(input.StorageKey) == "" {
			response.WriteError(writer, http.StatusBadRequest, "缺少 CAD 文件存储键")
			return
		}
		result, err := service.ReadOnlyOpen(request.Context(), user, strings.TrimSpace(input.StorageKey))
		if err != nil {
			writeEditingError(writer, err)
			return
		}
		response.WriteData(writer, http.StatusOK, result)
	}
}

func writeEditingError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, editing.ErrSMBNotConfigured):
		response.WriteError(writer, http.StatusServiceUnavailable, "SMB 文件共享尚未配置")
	case errors.Is(err, editing.ErrFileBusy):
		response.WriteError(writer, http.StatusConflict, "该文件正在被其他用户编辑")
	case errors.Is(err, editing.ErrInvalidTicket), errors.Is(err, editing.ErrSessionNotFound):
		response.WriteError(writer, http.StatusGone, "编辑票据或会话已失效，请重新打开")
	default:
		response.WriteError(writer, http.StatusBadRequest, err.Error())
	}
}
