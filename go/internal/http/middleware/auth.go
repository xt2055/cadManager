package middleware

import (
	"context"
	"net/http"
	"strings"

	"cadguanliq/internal/auth"
	"cadguanliq/internal/response"
)

type contextKey string

const authUserKey contextKey = "auth-user"

// AuthUserContextKey 导出认证用户的 context 键，供测试与内部装配注入已认证用户。
const AuthUserContextKey = authUserKey

func RequireAuth(service *auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if service == nil {
				response.WriteError(writer, http.StatusUnauthorized, "认证服务未配置")
				return
			}
			token := BearerToken(request.Header.Get("Authorization"))
			user, err := service.CurrentUser(request.Context(), token)
			if err != nil {
				response.WriteError(writer, http.StatusUnauthorized, "登录已失效，请重新登录")
				return
			}
			ctx := context.WithValue(request.Context(), authUserKey, user)
			next.ServeHTTP(writer, request.WithContext(ctx))
		})
	}
}

func UserFromContext(ctx context.Context) (auth.AuthUser, bool) {
	user, ok := ctx.Value(authUserKey).(auth.AuthUser)
	return user, ok
}

// RequireAdmin 在 RequireAuth 之后使用：仅放行 admin 角色用户。
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		user, ok := UserFromContext(request.Context())
		if !ok {
			response.WriteError(writer, http.StatusUnauthorized, "请先登录")
			return
		}
		for _, role := range user.Roles {
			if strings.EqualFold(role, "admin") {
				next.ServeHTTP(writer, request)
				return
			}
		}
		response.WriteError(writer, http.StatusForbidden, "需要管理员权限")
	})
}

func BearerToken(value string) string {
	parts := strings.Fields(value)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}
