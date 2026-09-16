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

// HasRole 判断账号是否具有指定角色（大小写不敏感）。
// 角色名的单一来源在 internal/auth，这里只做转发以保留既有调用点。
func HasRole(user auth.AuthUser, role string) bool {
	return user.HasRole(role)
}

// HasAnyRole 判断账号是否具有任一指定角色。
func HasAnyRole(user auth.AuthUser, roles ...string) bool {
	return user.HasAnyRole(roles...)
}

// RequireAdmin 在 RequireAuth 之后使用：仅放行 admin 角色用户。
func RequireAdmin(next http.Handler) http.Handler {
	return RequireAnyRole("需要管理员权限", "admin")(next)
}

// RequireAnyRole 在 RequireAuth 之后使用：拥有任一指定角色即放行。
// denialMessage 必须写清“为什么不行 + 去哪里解决”，不返回资源是否存在的信息。
func RequireAnyRole(denialMessage string, roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			user, ok := UserFromContext(request.Context())
			if !ok {
				response.WriteError(writer, http.StatusUnauthorized, "请先登录")
				return
			}
			if HasAnyRole(user, roles...) {
				next.ServeHTTP(writer, request)
				return
			}
			response.WriteError(writer, http.StatusForbidden, denialMessage)
		})
	}
}

func BearerToken(value string) string {
	parts := strings.Fields(value)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}
