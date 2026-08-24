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

func BearerToken(value string) string {
	parts := strings.Fields(value)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}
