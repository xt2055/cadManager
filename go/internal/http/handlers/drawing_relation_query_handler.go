package handlers

import (
	"net/http"

	"cadguanliq/internal/http/middleware"
	"cadguanliq/internal/response"
	"github.com/jackc/pgx/v5/pgxpool"
)

type borrowProjection struct {
	ID              string `json:"id,omitempty"`
	Direction       string `json:"dir"`
	Project         string `json:"project"`
	Part            string `json:"part"`
	PartNo          string `json:"partNo,omitempty"`
	PartName        string `json:"partName,omitempty"`
	SourceDrawingNo string `json:"sourceDrawingNo,omitempty"`
	TargetDrawingNo string `json:"targetDrawingNo,omitempty"`
	Date            string `json:"date"`
	Status          string `json:"status"`
	User            string `json:"user,omitempty"`
}

// DrawingRelations exposes read-only projections for the final relation model.
// Commands are handled by the drawing and relation atomic APIs.
func DrawingRelations(pool *pgxpool.Pool) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if _, ok := middleware.UserFromContext(request.Context()); !ok {
			response.WriteError(writer, http.StatusUnauthorized, "登录已失效，请重新登录")
			return
		}
		if request.Method != http.MethodGet {
			response.WriteError(writer, http.StatusMethodNotAllowed, "关系查询接口只支持读取")
			return
		}
		resource := relationResource(request.URL.Path)
		switch resource {
		case "branches":
			listBranchesProjection(writer, request, pool)
		case "borrows":
			listFinalBorrows(writer, request, pool)
		default:
			response.WriteError(writer, http.StatusNotFound, "关系查询接口不存在")
		}
	}
}

func listFinalBorrows(writer http.ResponseWriter, request *http.Request, pool *pgxpool.Pool) {
	rows, err := pool.Query(request.Context(), `
		SELECT r.id::text,
		       target.name,
		       p.part_no,
		       COALESCE(pr.name, p.part_no),
		       COALESCE(source.drawing_no, ''),
		       target.drawing_no,
		       COALESCE(r.borrowed_at, r.created_at)::text,
		       CASE WHEN r.status = 'archived' THEN '已归档' ELSE '使用中' END,
		       COALESCE(u.display_name, u.account, '')
		FROM drawing_part_relations r
		JOIN drawings target ON target.id = r.drawing_id
		JOIN parts p ON p.id = r.part_id
		LEFT JOIN part_revisions pr ON pr.id = p.published_revision_id
		LEFT JOIN drawing_part_relations source_relation
		  ON source_relation.part_id = p.id
		 AND source_relation.relation_type = 'owned'
		 AND source_relation.status = 'active'
		LEFT JOIN drawings source ON source.id = source_relation.drawing_id
		LEFT JOIN users u ON u.id = r.borrowed_by
		WHERE r.relation_type = 'borrowed'
		ORDER BY r.created_at DESC`)
	if err != nil {
		response.WriteError(writer, http.StatusInternalServerError, "借用关系读取失败")
		return
	}
	defer rows.Close()
	items := make([]borrowProjection, 0)
	for rows.Next() {
		var item borrowProjection
		if err := rows.Scan(&item.ID, &item.Project, &item.PartNo, &item.PartName, &item.SourceDrawingNo, &item.TargetDrawingNo, &item.Date, &item.Status, &item.User); err != nil {
			response.WriteError(writer, http.StatusInternalServerError, "借用关系读取失败")
			return
		}
		item.Direction = "in"
		item.Part = item.PartNo + " " + item.PartName
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		response.WriteError(writer, http.StatusInternalServerError, "借用关系读取失败")
		return
	}
	response.WriteData(writer, http.StatusOK, items)
}
