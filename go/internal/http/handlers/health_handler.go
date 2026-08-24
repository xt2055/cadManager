package handlers

import (
	"context"
	"net/http"
	"time"

	"cadguanliq/internal/config"
	"cadguanliq/internal/response"
	"github.com/jackc/pgx/v5/pgxpool"
)

func Health(pool *pgxpool.Pool, cfg config.DatabaseConfig) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		if pool == nil {
			response.WriteData(writer, http.StatusOK, map[string]string{
				"status":   "ok",
				"database": "not_configured",
			})
			return
		}

		healthTimeout := time.Duration(cfg.HealthTimeoutMS) * time.Millisecond
		ctx, cancel := context.WithTimeout(request.Context(), healthTimeout)
		defer cancel()
		if err := pool.Ping(ctx); err != nil {
			response.WriteJSON(writer, http.StatusServiceUnavailable, map[string]any{
				"code":    http.StatusServiceUnavailable,
				"message": "数据库不可用",
				"data": map[string]string{
					"status":   "degraded",
					"database": "error",
				},
			})
			return
		}

		response.WriteData(writer, http.StatusOK, map[string]string{
			"status":   "ok",
			"database": "ok",
		})
	}
}
