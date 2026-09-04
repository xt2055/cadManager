package handlers

import (
	"net/http"
	"strings"

	"cadguanliq/internal/attachment"
	"cadguanliq/internal/http/middleware"
	"cadguanliq/internal/response"
	"github.com/jackc/pgx/v5/pgxpool"
)

type branchProjection struct {
	ID              string `json:"id,omitempty"`
	SourceDrawingNo string `json:"sourceDrawingNo,omitempty"`
	TargetDrawingNo string `json:"targetDrawingNo,omitempty"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	Status          string `json:"status"`
}

func listBranchesProjection(writer http.ResponseWriter, request *http.Request, pool *pgxpool.Pool) {
	rows, err := pool.Query(request.Context(), `
		SELECT b.id::text, COALESCE(source.drawing_no, ''), COALESCE(target.drawing_no, ''),
		       b.name, b.description, b.status
		FROM drawing_branches b
		LEFT JOIN drawings source ON source.id = b.source_drawing_id
		LEFT JOIN drawings target ON target.id = b.target_drawing_id
		ORDER BY b.created_at DESC`)
	if err != nil {
		response.WriteError(writer, http.StatusInternalServerError, "分支关系读取失败")
		return
	}
	defer rows.Close()
	items := make([]branchProjection, 0)
	for rows.Next() {
		var item branchProjection
		if err := rows.Scan(&item.ID, &item.SourceDrawingNo, &item.TargetDrawingNo, &item.Name, &item.Description, &item.Status); err != nil {
			response.WriteError(writer, http.StatusInternalServerError, "分支关系读取失败")
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		response.WriteError(writer, http.StatusInternalServerError, "分支关系读取失败")
		return
	}
	response.WriteData(writer, http.StatusOK, items)
}

func Attachments(repository attachment.Repository) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if _, ok := middleware.UserFromContext(request.Context()); !ok {
			response.WriteError(writer, http.StatusUnauthorized, "登录已失效，请重新登录")
			return
		}
		if request.Method != http.MethodGet {
			response.WriteError(writer, http.StatusMethodNotAllowed, "附件查询接口只支持读取")
			return
		}
		items, err := repository.ListAll(request.Context())
		if err != nil {
			response.WriteError(writer, http.StatusInternalServerError, "附件列表读取失败")
			return
		}
		response.WriteData(writer, http.StatusOK, items)
	}
}

func relationResource(path string) string {
	return strings.Trim(strings.TrimPrefix(path, "/api/drawing-relations/"), "/")
}
