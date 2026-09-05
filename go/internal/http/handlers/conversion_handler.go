package handlers

import (
	"cadguanliq/internal/http/middleware"
	"cadguanliq/internal/response"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"strings"
)

// ConversionStatus reports the current attachment version, not an expired upload session.
func ConversionStatus(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/cad/conversions/")
		user, ok := middleware.UserFromContext(r.Context())
		if !ok {
			response.WriteError(w, 401, "请先登录")
			return
		}
		if r.Method == http.MethodPost {
			admin := false
			for _, role := range user.Roles {
				if role == "admin" {
					admin = true
				}
			}
			result, err := pool.Exec(r.Context(), `UPDATE cad_conversion_jobs j SET status='pending', attempts=0, next_attempt_at=now(), lease_until=NULL, last_error=NULL, updated_at=now() FROM attachments a, attachment_versions v WHERE j.attachment_id=a.id AND a.current_version_id=v.id AND v.blob_id=j.source_blob_id AND a.id=$1::uuid AND a.deleted_at IS NULL AND (a.uploaded_by=$2::uuid OR $3) AND j.status IN ('failed','retry','backoff')`, id, user.ID, admin)
			if err != nil {
				response.WriteError(w, 500, "转换重试入队失败")
				return
			}
			if result.RowsAffected() == 0 {
				response.WriteError(w, 409, "没有可重试任务，或没有重试权限")
				return
			}
		} else if r.Method != http.MethodGet {
			response.WriteError(w, 405, "method not allowed")
			return
		}
		var state, detail string
		var attempts int
		err := pool.QueryRow(r.Context(), `SELECT CASE WHEN lower(v.original_name) LIKE '%.dwg' OR lower(v.original_name) LIKE '%.dxf' THEN 'ready' ELSE COALESCE(j.status,'missing') END, COALESCE(j.last_error,''), COALESCE(j.attempts,0) FROM attachments a JOIN attachment_versions v ON v.id=a.current_version_id LEFT JOIN LATERAL (SELECT status,last_error,attempts FROM cad_conversion_jobs WHERE attachment_id=a.id AND source_blob_id=v.blob_id AND status<>'cancelled' ORDER BY created_at DESC LIMIT 1) j ON true WHERE a.id=$1::uuid AND a.deleted_at IS NULL`, id).Scan(&state, &detail, &attempts)
		if err != nil {
			response.WriteError(w, 404, "附件转换状态不可用")
			return
		}
		response.WriteData(w, 200, map[string]any{"status": state, "error": detail, "attempts": attempts})
	}
}
