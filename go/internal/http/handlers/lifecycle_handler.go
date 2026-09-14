package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"regexp"
	"strings"
	"time"

	"cadguanliq/internal/http/middleware"
	"cadguanliq/internal/response"
	"cadguanliq/internal/storage"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Lifecycle records are append-only. Files use a separate storage root so the
// legacy CAD orphan collector cannot remove documentary evidence.
func LifecycleDocuments(pool *pgxpool.Pool, store storage.ObjectStorage) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		fail := func(err error) {
			log.Printf("lifecycle documents: %v", err)
			response.WriteError(w, 500, "资料操作失败，请稍后重试")
		}
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/lifecycle-documents/") {
			id := strings.TrimPrefix(r.URL.Path, "/api/lifecycle-documents/")
			var key, name string
			err := pool.QueryRow(ctx, `SELECT storage_key,file_name FROM lifecycle_documents WHERE id=$1::uuid`, id).Scan(&key, &name)
			if err != nil {
				response.WriteError(w, 404, "资料不存在")
				return
			}
			reader, _, err := store.Open(ctx, key)
			if err != nil {
				fail(err)
				return
			}
			defer reader.Close()
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Cache-Control", "private, no-store")
			w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
			io.Copy(w, reader)
			return
		}
		if r.Method == http.MethodGet {
			drawingID, patentID := r.URL.Query().Get("drawingId"), r.URL.Query().Get("patentId")
			if (drawingID == "") == (patentID == "") {
				response.WriteError(w, 400, "请选择图号或专利")
				return
			}
			rows, err := pool.Query(ctx, `SELECT jsonb_build_object('id',d.id,'title',d.title,'category',d.category,'folderPath',d.folder_path,'description',d.description,'fileName',d.file_name,'size',d.size_bytes,'sha256',d.sha256,'changeRequestId',d.change_request_id,'createdAt',d.created_at,'createdBy',COALESCE(u.display_name,u.account)) FROM lifecycle_documents d JOIN users u ON u.id=d.created_by WHERE (($1<>'' AND d.drawing_id=NULLIF($1,'')::uuid) OR ($2<>'' AND d.patent_id=NULLIF($2,'')::uuid)) AND ($3='' OR EXISTS(SELECT 1 FROM change_submission_documents sd WHERE sd.document_id=d.id AND sd.submission_id=NULLIF($3,'')::uuid)) ORDER BY d.created_at DESC`, drawingID, patentID, r.URL.Query().Get("submissionId"))
			if err != nil {
				fail(err)
				return
			}
			defer rows.Close()
			items := []json.RawMessage{}
			for rows.Next() {
				var raw json.RawMessage
				if err = rows.Scan(&raw); err != nil {
					fail(err)
					return
				}
				items = append(items, raw)
			}
			if err = rows.Err(); err != nil {
				fail(err)
				return
			}
			response.WriteData(w, 200, items)
			return
		}
		if r.Method != http.MethodPost {
			response.WriteError(w, 405, "不支持的操作")
			return
		}
		user, ok := middleware.UserFromContext(ctx)
		if !ok {
			response.WriteError(w, 401, "请登录")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 100<<20)
		if err := r.ParseMultipartForm(8 << 20); err != nil {
			response.WriteError(w, 400, "文件上传失败，单次不得超过 100 MB")
			return
		}
		defer r.MultipartForm.RemoveAll()
		drawingID, patentID, changeID := r.FormValue("drawingId"), r.FormValue("patentId"), r.FormValue("changeRequestId")
		category, title := strings.TrimSpace(r.FormValue("category")), strings.TrimSpace(r.FormValue("title"))
		folder := strings.Trim(strings.TrimSpace(strings.ReplaceAll(r.FormValue("folderPath"), "\\", "/")), "/")
		if len(folder) > 500 || len(strings.Split(folder, "/")) > 10 {
			response.WriteError(w, 400, "目录最多 10 层，路径不超过 500 字节")
			return
		}
		if folder != "" {
			for _, part := range strings.Split(folder, "/") {
				if strings.TrimSpace(part) == "" || part == "." || part == ".." {
					response.WriteError(w, 400, "目录名称不能为空或使用相对路径符号")
					return
				}
			}
		}
		if (drawingID == "") == (patentID == "") || category == "" || title == "" {
			response.WriteError(w, 400, "资料归属、分类和标题不能为空")
			return
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			fail(err)
			return
		}
		defer tx.Rollback(ctx)
		if changeID != "" {
			var status, applicant, executor string
			if err = tx.QueryRow(ctx, `SELECT status,applicant_id::text,executor_id::text FROM change_requests WHERE id=$1::uuid AND drawing_id=$2::uuid FOR UPDATE`, changeID, drawingID).Scan(&status, &applicant, &executor); err != nil {
				response.WriteError(w, 400, "工单不属于当前图号")
				return
			}
			admin := false
			for _, role := range user.Roles {
				if role == "admin" {
					admin = true
				}
			}
			if user.ID != applicant && user.ID != executor && !admin {
				response.WriteError(w, 403, "仅申请人、指定设计员或管理员可追加本次变更材料")
				return
			}
			if status != "pending_approval" && status != "executing" {
				response.WriteError(w, 409, "已提交审核的变更资料已冻结；退回后可追加")
				return
			}
		}
		if patentID != "" {
			var owner string
			if err = tx.QueryRow(ctx, `SELECT responsible_id::text FROM patent_records WHERE id=$1::uuid FOR UPDATE`, patentID).Scan(&owner); err != nil {
				response.WriteError(w, 404, "专利不存在")
				return
			}
			admin := false
			for _, role := range user.Roles {
				if role == "admin" {
					admin = true
				}
			}
			if owner != user.ID && !admin {
				response.WriteError(w, 403, "仅负责人或管理员可追加专利资料")
				return
			}
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			response.WriteError(w, 400, "请选择文件")
			return
		}
		defer file.Close()
		keyBytes := make([]byte, 24)
		if _, err = rand.Read(keyBytes); err != nil {
			fail(err)
			return
		}
		key := hex.EncodeToString(keyBytes)
		info, err := store.Put(ctx, key, file, "application/octet-stream")
		if err != nil {
			fail(err)
			return
		}
		if info.Size == 0 {
			_ = store.Delete(ctx, key)
			response.WriteError(w, 400, "不能将空文件作为归档资料")
			return
		}
		var id string
		err = tx.QueryRow(ctx, `INSERT INTO lifecycle_documents(drawing_id,patent_id,change_request_id,category,title,description,storage_key,file_name,mime_type,size_bytes,sha256,created_by,folder_path) VALUES(NULLIF($1,'')::uuid,NULLIF($2,'')::uuid,NULLIF($3,'')::uuid,$4,$5,$6,$7,$8,$9,$10,$11,$12::uuid,$13) RETURNING id::text`, drawingID, patentID, changeID, category, title, r.FormValue("description"), key, header.Filename, info.MimeType, info.Size, info.SHA256, user.ID, folder).Scan(&id)
		if err != nil {
			_ = store.Delete(ctx, key)
			fail(err)
			return
		}
		// A commit error can be ambiguous; retain the blob for reconciliation instead
		// of risking deletion of an already committed evidence record.
		if err = tx.Commit(ctx); err != nil {
			fail(err)
			return
		}
		response.WriteData(w, 201, map[string]string{"id": id})
	})
}

type patentInput struct {
	ResponsibleID  string `json:"responsibleId"`
	Number         string `json:"number"`
	Title          string `json:"title"`
	PatentType     string `json:"patentType"`
	Jurisdiction   string `json:"jurisdiction"`
	OwnerName      string `json:"ownerName"`
	DrawingID      string `json:"drawingId"`
	FeeDue         string `json:"feeDue"`
	ExpiresOn      string `json:"expiresOn"`
	DeadlineSource string `json:"deadlineSource"`
	ReminderDays   int    `json:"reminderDays"`
	Notes          string `json:"notes"`
	Revision       int    `json:"revision"`
	ReceiptID      string `json:"receiptId"`
	PaidOn         string `json:"paidOn"`
	Amount         string `json:"amount"`
}

func Patents(pool *pgxpool.Pool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		user, ok := middleware.UserFromContext(ctx)
		if !ok {
			response.WriteError(w, 401, "请登录")
			return
		}
		fail := func(err error) {
			log.Printf("patent records: %v", err)
			response.WriteError(w, 500, "专利记录操作失败，请检查编号是否重复或稍后重试")
		}
		suffix := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/patents"), "/")
		parts := strings.Split(suffix, "/")
		id := parts[0]
		if r.Method == http.MethodGet {
			var rows pgx.Rows
			var err error
			if len(parts) == 2 && parts[1] == "events" {
				rows, err = pool.Query(ctx, `SELECT jsonb_build_object('id',e.id,'action',e.action,'detail',e.detail,'createdAt',e.created_at,'actor',COALESCE(u.display_name,u.account,'系统提醒')) FROM patent_events e LEFT JOIN users u ON u.id=e.actor_id WHERE e.patent_id=$1::uuid ORDER BY e.created_at DESC`, id)
			} else {
				rows, err = pool.Query(ctx, `SELECT jsonb_build_object('id',p.id,'number',p.number,'title',p.title,'patentType',p.patent_type,'jurisdiction',p.jurisdiction,'ownerName',p.owner_name,'responsibleId',p.responsible_id,'responsibleName',COALESCE(u.display_name,u.account),'drawingId',p.drawing_id,'feeDue',p.fee_due,'expiresOn',p.expires_on,'deadlineSource',p.deadline_source,'reminderDays',p.reminder_days,'notes',p.notes,'revision',p.revision,'feeDays',p.fee_due-(now() AT TIME ZONE 'Asia/Shanghai')::date,'expiryDays',p.expires_on-(now() AT TIME ZONE 'Asia/Shanghai')::date) FROM patent_records p JOIN users u ON u.id=p.responsible_id WHERE ($1='' OR p.id=NULLIF($1,'')::uuid) ORDER BY LEAST(p.fee_due,p.expires_on) NULLS LAST,p.created_at DESC`, id)
			}
			if err != nil {
				fail(err)
				return
			}
			defer rows.Close()
			items := []json.RawMessage{}
			for rows.Next() {
				var raw json.RawMessage
				if err = rows.Scan(&raw); err != nil {
					fail(err)
					return
				}
				items = append(items, raw)
			}
			if err = rows.Err(); err != nil {
				fail(err)
				return
			}
			response.WriteData(w, 200, items)
			return
		}
		if r.Method != http.MethodPost && r.Method != http.MethodPut {
			response.WriteError(w, 405, "专利记录不允许删除")
			return
		}
		var input patentInput
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		if err := decodeJSON(r, &input); err != nil {
			response.WriteError(w, 400, "请求格式不正确")
			return
		}
		for _, date := range []string{input.FeeDue, input.ExpiresOn, input.PaidOn} {
			if date != "" {
				if _, err := time.Parse("2006-01-02", date); err != nil {
					response.WriteError(w, 400, "日期格式应为 YYYY-MM-DD")
					return
				}
			}
		}
		payment := len(parts) == 2 && parts[1] == "payment"
		if len(parts) > 1 && !payment {
			response.WriteError(w, 404, "接口不存在")
			return
		}
		if !payment && (strings.TrimSpace(input.Title) == "" || strings.TrimSpace(input.Number) == "" || strings.TrimSpace(input.DeadlineSource) == "" || input.ReminderDays < 1 || input.ReminderDays > 365) {
			response.WriteError(w, 400, "请填写编号、名称、期限依据及 1～365 天提醒提前量")
			return
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			fail(err)
			return
		}
		defer tx.Rollback(ctx)
		action := "create"
		admin := false
		for _, role := range user.Roles {
			if role == "admin" {
				admin = true
			}
		}
		responsibleID := user.ID
		var previous json.RawMessage
		if id != "" {
			var owner string
			var revision int
			err = tx.QueryRow(ctx, `SELECT responsible_id::text,revision,to_jsonb(p) FROM patent_records p WHERE id=$1::uuid FOR UPDATE`, id).Scan(&owner, &revision, &previous)
			if err != nil {
				response.WriteError(w, 404, "专利不存在")
				return
			}
			admin := false
			for _, role := range user.Roles {
				if role == "admin" {
					admin = true
				}
			}
			if owner != user.ID && !admin {
				response.WriteError(w, 403, "仅负责人或管理员可修改")
				return
			}
			if input.Revision != revision {
				response.WriteError(w, 409, "记录已更新，请刷新后重试")
				return
			}
			responsibleID = owner
			action = "update"
		} else if r.Method != http.MethodPost || payment {
			response.WriteError(w, 400, "缺少专利编号")
			return
		}
		if input.ResponsibleID != "" && input.ResponsibleID != responsibleID {
			if !admin {
				response.WriteError(w, 403, "只有管理员可以调整专利负责人")
				return
			}
			responsibleID = input.ResponsibleID
		}
		var validOwner bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1::uuid AND status='active')`, responsibleID).Scan(&validOwner); err != nil || !validOwner {
			response.WriteError(w, 400, "专利负责人不存在或已停用")
			return
		}
		input.ResponsibleID = responsibleID
		if payment {
			if input.ReceiptID == "" || input.PaidOn == "" || strings.TrimSpace(input.Amount) == "" || input.FeeDue == "" || strings.TrimSpace(input.DeadlineSource) == "" {
				response.WriteError(w, 400, "登记缴费需要凭证、缴费日期、金额、下一缴费期限及期限依据")
				return
			}
			input.Amount = strings.Join(strings.Fields(strings.ToUpper(input.Amount)), " ")
			if !regexp.MustCompile(`^[0-9]+(\.[0-9]{1,2})? [A-Z]{3}$`).MatchString(input.Amount) {
				response.WriteError(w, 400, "金额格式为数字和币种，例如 900.00 CNY")
				return
			}
			var valid bool
			err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_documents d JOIN patent_records p ON p.id=d.patent_id WHERE d.id=$1::uuid AND p.id=$2::uuid AND (p.fee_due IS NULL OR $3::date>p.fee_due) AND $4::date <= (now() AT TIME ZONE 'Asia/Shanghai')::date AND NOT EXISTS(SELECT 1 FROM patent_events e WHERE e.patent_id=p.id AND e.action='payment' AND e.detail->'submitted'->>'receiptId'=d.id::text))`, input.ReceiptID, id, input.FeeDue, input.PaidOn).Scan(&valid)
			if err != nil || !valid {
				response.WriteError(w, 400, "凭证须属于当前专利且未重复登记，下一缴费日期须晚于本期，实际缴费日期不能在未来")
				return
			}
			_, err = tx.Exec(ctx, `UPDATE patent_records SET fee_due=$2::date,deadline_source=$3,revision=revision+1,updated_at=now() WHERE id=$1::uuid`, id, input.FeeDue, input.DeadlineSource)
			action = "payment"
		} else if id == "" {
			err = tx.QueryRow(ctx, `INSERT INTO patent_records(number,title,patent_type,jurisdiction,owner_name,responsible_id,drawing_id,fee_due,expires_on,deadline_source,reminder_days,notes) VALUES($1,$2,$3,$4,$5,$6::uuid,NULLIF($7,'')::uuid,NULLIF($8,'')::date,NULLIF($9,'')::date,$10,$11,$12) RETURNING id::text`, strings.TrimSpace(input.Number), strings.TrimSpace(input.Title), input.PatentType, input.Jurisdiction, input.OwnerName, responsibleID, input.DrawingID, input.FeeDue, input.ExpiresOn, input.DeadlineSource, input.ReminderDays, input.Notes).Scan(&id)
		} else {
			_, err = tx.Exec(ctx, `UPDATE patent_records SET number=$2,title=$3,patent_type=$4,jurisdiction=$5,owner_name=$6,drawing_id=NULLIF($7,'')::uuid,fee_due=NULLIF($8,'')::date,expires_on=NULLIF($9,'')::date,deadline_source=$10,reminder_days=$11,notes=$12,responsible_id=$13::uuid,revision=revision+1,updated_at=now() WHERE id=$1::uuid`, id, strings.TrimSpace(input.Number), strings.TrimSpace(input.Title), input.PatentType, input.Jurisdiction, input.OwnerName, input.DrawingID, input.FeeDue, input.ExpiresOn, input.DeadlineSource, input.ReminderDays, input.Notes, responsibleID)
		}
		if err != nil {
			fail(err)
			return
		}
		detail, err := json.Marshal(map[string]any{"before": previous, "submitted": input})
		if err != nil {
			fail(err)
			return
		}
		if _, err = tx.Exec(ctx, `INSERT INTO patent_events(patent_id,actor_id,action,detail) VALUES($1::uuid,$2::uuid,$3,$4::jsonb)`, id, user.ID, action, string(detail)); err != nil {
			fail(err)
			return
		}
		if err = tx.Commit(ctx); err != nil {
			fail(err)
			return
		}
		response.WriteData(w, 200, map[string]string{"id": id, "message": fmt.Sprintf("已保存%s记录", action)})
	})
}
