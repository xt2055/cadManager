package middleware

import (
	"log"
	"net/http"

	"cadguanliq/internal/response"
)

func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("panic recovered: %v", recovered)
				response.WriteError(writer, http.StatusInternalServerError, "服务器内部错误")
			}
		}()
		next.ServeHTTP(writer, request)
	})
}
