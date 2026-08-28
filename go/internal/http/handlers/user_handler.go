package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"cadguanliq/internal/auth"
	"cadguanliq/internal/http/middleware"
	"cadguanliq/internal/response"
)

func Users(service *auth.Service) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		user, ok := middleware.UserFromContext(request.Context())
		if !ok || !hasAdminRole(user.Roles) {
			response.WriteError(writer, http.StatusForbidden, "只有管理员可以管理账号")
			return
		}
		if request.Method == http.MethodGet {
			users, err := service.ListUsers(request.Context())
			if err != nil {
				response.WriteError(writer, http.StatusInternalServerError, "读取账号列表失败")
				return
			}
			response.WriteData(writer, http.StatusOK, users)
			return
		}
		if request.Method != http.MethodPost {
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var input auth.UserInput
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			response.WriteError(writer, http.StatusBadRequest, "账号参数格式无效")
			return
		}
		created, err := service.CreateUser(request.Context(), input)
		if err != nil {
			writeUserError(writer, err)
			return
		}
		response.WriteData(writer, http.StatusCreated, created)
	}
}

func UserResource(service *auth.Service) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		user, ok := middleware.UserFromContext(request.Context())
		if !ok || !hasAdminRole(user.Roles) {
			response.WriteError(writer, http.StatusForbidden, "只有管理员可以管理账号")
			return
		}
		userID := strings.Trim(strings.TrimPrefix(request.URL.Path, "/api/users/"), "/")
		if userID == "" || strings.Contains(userID, "/") {
			response.WriteError(writer, http.StatusNotFound, "账号不存在")
			return
		}
		if request.Method != http.MethodPut && request.Method != http.MethodPatch {
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var input auth.UserInput
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			response.WriteError(writer, http.StatusBadRequest, "账号参数格式无效")
			return
		}
		updated, err := service.UpdateUser(request.Context(), userID, input)
		if err != nil {
			writeUserError(writer, err)
			return
		}
		response.WriteData(writer, http.StatusOK, updated)
	}
}

func writeUserError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrInvalidUserInput):
		response.WriteError(writer, http.StatusBadRequest, "账号、姓名、密码或角色无效")
	case errors.Is(err, auth.ErrUserConflict):
		response.WriteError(writer, http.StatusConflict, "登录账号已存在")
	case errors.Is(err, auth.ErrUserNotFound):
		response.WriteError(writer, http.StatusNotFound, "账号不存在")
	case errors.Is(err, auth.ErrLastAdmin):
		response.WriteError(writer, http.StatusConflict, err.Error())
	default:
		response.WriteError(writer, http.StatusInternalServerError, "账号保存失败")
	}
}
