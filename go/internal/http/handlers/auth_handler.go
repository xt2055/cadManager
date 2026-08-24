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

func Login(service *auth.Service) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var payload auth.LoginRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			response.WriteError(writer, http.StatusBadRequest, "登录请求格式无效")
			return
		}
		if service == nil {
			response.WriteError(writer, http.StatusServiceUnavailable, "认证服务未配置")
			return
		}
		result, err := service.Login(request.Context(), payload)
		if err != nil {
			writeAuthError(writer, err)
			return
		}
		response.WriteData(writer, http.StatusOK, result)
	}
}

func Me(service *auth.Service) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if service == nil {
			response.WriteError(writer, http.StatusServiceUnavailable, "认证服务未配置")
			return
		}
		user, err := service.CurrentUser(request.Context(), middleware.BearerToken(request.Header.Get("Authorization")))
		if err != nil {
			writeAuthError(writer, err)
			return
		}
		response.WriteData(writer, http.StatusOK, map[string]auth.AuthUser{"user": user})
	}
}

func Logout(service *auth.Service) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if service == nil {
			response.WriteError(writer, http.StatusServiceUnavailable, "认证服务未配置")
			return
		}
		if err := service.Logout(request.Context(), middleware.BearerToken(request.Header.Get("Authorization"))); err != nil {
			response.WriteError(writer, http.StatusInternalServerError, "退出登录失败")
			return
		}
		response.WriteData(writer, http.StatusOK, nil)
	}
}

func Reviewers(service *auth.Service) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if service == nil {
			response.WriteError(writer, http.StatusServiceUnavailable, "认证服务未配置")
			return
		}
		role := strings.TrimSpace(request.URL.Query().Get("role"))
		if role == "" {
			role = "reviewer"
		}
		users, err := service.ListActiveUsersByRole(request.Context(), role)
		if err != nil {
			response.WriteError(writer, http.StatusInternalServerError, "读取候选人员失败")
			return
		}
		response.WriteData(writer, http.StatusOK, users)
	}
}

func writeAuthError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrInvalidCredentials):
		response.WriteError(writer, http.StatusUnauthorized, err.Error())
	case errors.Is(err, auth.ErrDisabledUser):
		response.WriteError(writer, http.StatusForbidden, err.Error())
	case errors.Is(err, auth.ErrInvalidToken):
		response.WriteError(writer, http.StatusUnauthorized, "登录已失效，请重新登录")
	default:
		response.WriteError(writer, http.StatusInternalServerError, "认证服务暂时不可用")
	}
}
