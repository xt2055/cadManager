package handlers

import (
	"cadguanliq/internal/response"
	"cadguanliq/internal/storage"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"log"
	"mime"
	"net/http"
	"strings"
)

func LifecycleTree(pool *pgxpool.Pool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			response.WriteError(w, 405, "仅支持读取")
			return
		}
		var result json.RawMessage
		err := pool.QueryRow(r.Context(), `SELECT jsonb_build_object('id',d.id,'drawingNo',d.drawing_no,'name',d.name,'status',d.status,'version',d.version,
   'releases',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',rs.id,'version',rs.version,'source',rs.source,'createdAt',rs.created_at,'snapshot',rs.snapshot) ORDER BY rs.created_at DESC) FROM drawing_release_snapshots rs WHERE rs.drawing_id=d.id),'[]'::jsonb),
   'changes',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',c.id,'requestNo',c.request_no,'reason',c.reason,'scope',c.scope,'status',c.status,'createdAt',c.created_at,
    'terminalFiles',COALESCE((SELECT jsonb_agg(jsonb_build_object('name',a.logical_name,'versionId',t.work_attachment_version_id)) FROM change_request_targets t JOIN attachments a ON a.id=t.attachment_id WHERE t.request_id=c.id AND c.status='cancelled' AND t.work_attachment_version_id IS NOT NULL),'[]'::jsonb),
    'actions',COALESCE((SELECT jsonb_agg(jsonb_build_object('action',a.action,'opinion',a.opinion,'actor',COALESCE(u.display_name,u.account),'createdAt',a.created_at) ORDER BY a.created_at) FROM change_request_actions a LEFT JOIN users u ON u.id=a.actor_id WHERE a.request_id=c.id),'[]'::jsonb),
    'submissions',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',s.id,'round',s.round,'actualChanges',s.actual_changes,'proposedAttributes',s.proposed_attributes,'status',s.status,'createdAt',s.created_at,
     'files',COALESCE((SELECT jsonb_agg(jsonb_build_object('attachmentId',t.attachment_id,'name',a.logical_name,'baseVersionId',t.base_attachment_version_id,'submittedVersionId',COALESCE(t.submitted_attachment_version_id,t.base_attachment_version_id))) FROM change_request_submission_targets t JOIN attachments a ON a.id=t.attachment_id WHERE t.submission_id=s.id),'[]'::jsonb),
     'review', (SELECT jsonb_build_object('id',rc.id,'status',rc.status,'nodes',COALESCE((SELECT jsonb_agg(jsonb_build_object('name',n.name,'assignedName',n.assigned_name,'status',n.status,'opinion',n.opinion,'reviewedAt',n.reviewed_at) ORDER BY n.node_order) FROM review_case_nodes n WHERE n.review_case_id=rc.id),'[]'::jsonb)) FROM review_cases rc WHERE rc.change_submission_id=s.id)) ORDER BY s.round) FROM change_request_submissions s WHERE s.request_id=c.id),'[]'::jsonb)) ORDER BY c.created_at DESC) FROM change_requests c WHERE c.drawing_id=d.id),'[]'::jsonb),
   'reviews',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',rc.id,'status',rc.status,'startedAt',rc.started_at,'nodes',COALESCE((SELECT jsonb_agg(jsonb_build_object('name',n.name,'assignedName',n.assigned_name,'status',n.status,'opinion',n.opinion,'reviewedAt',n.reviewed_at) ORDER BY n.node_order) FROM review_case_nodes n WHERE n.review_case_id=rc.id),'[]'::jsonb)) ORDER BY rc.started_at) FROM review_cases rc WHERE rc.drawing_id=d.id AND rc.change_submission_id IS NULL),'[]'::jsonb)) FROM drawings d WHERE ($1<>'' AND d.id=NULLIF($1,'')::uuid) OR ($2<>'' AND d.drawing_no=$2) LIMIT 1`, r.URL.Query().Get("drawingId"), r.URL.Query().Get("drawingNo")).Scan(&result)
		if err != nil {
			log.Printf("lifecycle tree: %v", err)
			response.WriteError(w, 500, "生命周期记录读取失败")
			return
		}
		response.WriteData(w, 200, result)
	})
}

func LifecycleVersion(pool *pgxpool.Pool, store storage.ObjectStorage) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			response.WriteError(w, 405, "仅支持下载")
			return
		}
		var key, name string
		err := pool.QueryRow(r.Context(), `SELECT b.storage_key,v.original_name FROM attachment_versions v JOIN file_blobs b ON b.id=v.blob_id WHERE v.id=$1::uuid AND (EXISTS(SELECT 1 FROM change_request_submission_targets t WHERE t.submitted_attachment_version_id=v.id OR t.base_attachment_version_id=v.id) OR EXISTS(SELECT 1 FROM change_request_targets t JOIN change_requests cr ON cr.id=t.request_id WHERE cr.status='cancelled' AND t.work_attachment_version_id=v.id) OR EXISTS(SELECT 1 FROM drawing_release_snapshots rs CROSS JOIN LATERAL jsonb_array_elements(rs.snapshot->'files') f WHERE f->>'versionId'=v.id::text))`, strings.TrimPrefix(r.URL.Path, "/api/lifecycle-versions/")).Scan(&key, &name)
		if err != nil {
			response.WriteError(w, 404, "提交快照文件不存在")
			return
		}
		file, _, err := store.Open(r.Context(), key)
		if err != nil {
			response.WriteError(w, 404, "文件暂不可用")
			return
		}
		defer file.Close()
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
		io.Copy(w, file)
	})
}
