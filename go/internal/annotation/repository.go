package annotation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

// pgTimeLayout PostgreSQL to_char 模板（纯数字会被 to_char 当字面量，不能用 Go 参考时间格式）。
const pgTimeLayout = "YYYY-MM-DD HH24:MI"

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) Files(ctx context.Context, caseID string) ([]ReviewFile, error) {
	list := []ReviewFile{}
	if !ValidID(caseID) { return list, ErrNotFound }
	rows, err := r.pool.Query(ctx, `SELECT f.attachment_id::text,f.version_id::text,a.logical_name,v.version,
	 COALESCE((SELECT sum(jsonb_array_length(d.content->'marks')) FROM review_annotation_documents d WHERE d.review_case_id=f.review_case_id AND d.attachment_id=f.attachment_id),0)
	 FROM review_annotation_files f JOIN attachments a ON a.id=f.attachment_id JOIN attachment_versions v ON v.id=f.version_id
	 WHERE f.review_case_id=$1::uuid AND lower(a.logical_name) ~ '\.(exb|dwg|dxf)$' ORDER BY a.logical_name`, caseID)
	if err != nil { return list, err }; defer rows.Close()
	for rows.Next() { var item ReviewFile; if err=rows.Scan(&item.AttachmentID,&item.VersionID,&item.Name,&item.Version,&item.MarkCount); err!=nil { return list,err }; list=append(list,item) }
	return list,rows.Err()
}

func (r *Repository) UpdateTemplate(ctx context.Context, t Template, userID string, admin bool) (Template,error) {
	t.Text=strings.TrimSpace(t.Text);t.Category=strings.TrimSpace(t.Category)
	if !ValidID(t.ID) || t.Text=="" || len([]rune(t.Text))>500 || t.Category=="" || len([]rune(t.Category))>40 { return t,errors.New("话术需填写分类和内容，内容最多 500 字") }
	err:=r.pool.QueryRow(ctx,`UPDATE review_annotation_templates SET category=$2,text=$3 WHERE id=$1::uuid AND (owner_id=$4::uuid OR (owner_id IS NULL AND $5)) RETURNING COALESCE(owner_id::text,'')`,t.ID,t.Category,t.Text,userID,admin).Scan(&t.OwnerID)
	if errors.Is(err,pgx.ErrNoRows) { return t,ErrForbidden };return t,err
}

func (r *Repository) Load(ctx context.Context, caseID, attachmentID, userID string) (Workspace, error) {
	w := Workspace{CaseID: caseID, AttachmentID: attachmentID, Documents: []Document{}}
	if !ValidID(caseID) || !ValidID(attachmentID) {
		return w, ErrNotFound
	}
	var status, assigned string
	err := r.pool.QueryRow(ctx, `SELECT f.version_id::text,c.status,COALESCE(n.id::text,''),COALESCE(n.name,''),COALESCE(n.assigned_user_id::text,'')
 FROM review_annotation_files f JOIN review_cases c ON c.id=f.review_case_id
 LEFT JOIN LATERAL (SELECT id,name,assigned_user_id FROM review_case_nodes WHERE review_case_id=c.id AND status='pending' ORDER BY node_order LIMIT 1) n ON true
 WHERE f.review_case_id=$1::uuid AND f.attachment_id=$2::uuid`, caseID, attachmentID).Scan(&w.VersionID, &status, &w.NodeID, &w.NodeName, &assigned)
	if errors.Is(err, pgx.ErrNoRows) {
		return w, ErrNotFound
	}
	if err != nil {
		return w, err
	}
	w.CanEdit = status == "reviewing" && assigned == userID
	rows, err := r.pool.Query(ctx, `SELECT d.id::text,d.node_id::text,n.name,d.author_id::text,COALESCE(u.display_name,u.account),d.revision,to_char(d.updated_at,'YYYY-MM-DD HH24:MI:SS'),d.content
 FROM review_annotation_documents d JOIN review_case_nodes n ON n.id=d.node_id JOIN users u ON u.id=d.author_id
 WHERE d.review_case_id=$1::uuid AND d.attachment_id=$2::uuid ORDER BY n.node_order,d.updated_at`, caseID, attachmentID)
	if err != nil {
		return w, err
	}
	defer rows.Close()
	for rows.Next() {
		var d Document
		var raw []byte
		if err = rows.Scan(&d.ID, &d.NodeID, &d.NodeName, &d.AuthorID, &d.AuthorName, &d.Revision, &d.UpdatedAt, &raw); err != nil {
			return w, err
		}
		if err = json.Unmarshal(raw, &d.Content); err != nil {
			return w, err
		}
		w.Documents = append(w.Documents, d)
	}
	return w, rows.Err()
}

func (r *Repository) Save(ctx context.Context, in SaveInput, userID string) (Document, error) {
	var out Document
	if !ValidID(in.CaseID) || !ValidID(in.AttachmentID) || !ValidID(in.VersionID) || !ValidID(in.NodeID) || in.Revision < 0 {
		return out, ErrNotFound
	}
	if err := Validate(in.Content); err != nil {
		return out, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	// 与签署共享案例行锁：签署后绝不能保存旧节点批注。
	var status string
	err = tx.QueryRow(ctx, `SELECT status FROM review_cases WHERE id=$1::uuid FOR UPDATE`, in.CaseID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	if err != nil {
		return out, err
	}
	var nodeID, assigned, nodeName string
	err = tx.QueryRow(ctx, `SELECT id::text,COALESCE(assigned_user_id::text,''),name FROM review_case_nodes WHERE review_case_id=$1::uuid AND status='pending' ORDER BY node_order LIMIT 1`, in.CaseID).Scan(&nodeID, &assigned, &nodeName)
	if errors.Is(err, pgx.ErrNoRows) || status != "reviewing" {
		return out, ErrForbidden
	}
	if err != nil {
		return out, err
	}
	if assigned != userID || nodeID != in.NodeID {
		return out, ErrForbidden
	}
	var matches bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM review_annotation_files WHERE review_case_id=$1::uuid AND attachment_id=$2::uuid AND version_id=$3::uuid)`, in.CaseID, in.AttachmentID, in.VersionID).Scan(&matches)
	if err != nil {
		return out, err
	}
	if !matches {
		return out, ErrNotFound
	}
	raw, err := json.Marshal(in.Content)
	if err != nil {
		return out, err
	}
	err = tx.QueryRow(ctx, `INSERT INTO review_annotation_documents(review_case_id,attachment_id,version_id,node_id,author_id,revision,content)
 SELECT $1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::uuid,1,$6::jsonb WHERE $7::bigint=0
 ON CONFLICT(review_case_id,attachment_id,node_id,author_id) DO NOTHING RETURNING id::text,revision,to_char(updated_at,'YYYY-MM-DD HH24:MI:SS')`, in.CaseID, in.AttachmentID, in.VersionID, in.NodeID, userID, raw, in.Revision).Scan(&out.ID, &out.Revision, &out.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) && in.Revision > 0 {
		err = tx.QueryRow(ctx, `UPDATE review_annotation_documents SET content=$6::jsonb,revision=revision+1,updated_at=now()
   WHERE review_case_id=$1::uuid AND attachment_id=$2::uuid AND version_id=$3::uuid AND node_id=$4::uuid AND author_id=$5::uuid AND revision=$7
   RETURNING id::text,revision,to_char(updated_at,'YYYY-MM-DD HH24:MI:SS')`, in.CaseID, in.AttachmentID, in.VersionID, in.NodeID, userID, raw, in.Revision).Scan(&out.ID, &out.Revision, &out.UpdatedAt)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrConflict
	}
	if err != nil {
		return out, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO review_annotation_events(document_id,actor_id,revision,content) VALUES($1::uuid,$2::uuid,$3,$4::jsonb)`, out.ID, userID, out.Revision, raw)
	if err != nil {
		return out, err
	}
	out.NodeID = nodeID
	out.NodeName = nodeName
	out.AuthorID = userID
	out.Content = in.Content
	if err = tx.QueryRow(ctx, `SELECT COALESCE(display_name,account) FROM users WHERE id=$1::uuid`, userID).Scan(&out.AuthorName); err != nil {
		return out, err
	}
	return out, tx.Commit(ctx)
}

func (r *Repository) Templates(ctx context.Context, userID string) ([]Template, error) {
	list := []Template{}
	rows, err := r.pool.Query(ctx, `SELECT id::text,COALESCE(owner_id::text,''),category,text FROM review_annotation_templates WHERE owner_id IS NULL OR owner_id=$1::uuid ORDER BY owner_id NULLS FIRST,category,created_at`, userID)
	if err != nil {
		return list, err
	}
	defer rows.Close()
	for rows.Next() {
		var t Template
		if err = rows.Scan(&t.ID, &t.OwnerID, &t.Category, &t.Text); err != nil {
			return list, err
		}
		list = append(list, t)
	}
	return list, rows.Err()
}
func (r *Repository) CreateTemplate(ctx context.Context, t Template, userID string, admin bool) (Template, error) {
	t.Text = strings.TrimSpace(t.Text)
	t.Category = strings.TrimSpace(t.Category)
	if t.Text == "" || len([]rune(t.Text)) > 500 || t.Category == "" || len([]rune(t.Category)) > 40 {
		return t, errors.New("话术需填写分类和内容，内容最多 500 字")
	}
	if t.OwnerID == "" {
		if !admin {
			return t, ErrForbidden
		}
	} else {
		t.OwnerID = userID
	}
	err := r.pool.QueryRow(ctx, `INSERT INTO review_annotation_templates(owner_id,category,text) VALUES(NULLIF($1,'')::uuid,$2,$3) RETURNING id::text`, t.OwnerID, t.Category, t.Text).Scan(&t.ID)
	return t, err
}
func (r *Repository) DeleteTemplate(ctx context.Context, id, userID string, admin bool) error {
	if !ValidID(id) {
		return ErrNotFound
	}
	result, err := r.pool.Exec(ctx, `DELETE FROM review_annotation_templates WHERE id=$1::uuid AND (owner_id=$2::uuid OR (owner_id IS NULL AND $3))`, id, userID, admin)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrForbidden
	}
	return nil
}

// History 返回一张图纸（或单个案例）的逐轮批注归档。
//
// 轮次是批注的隔离边界：新一轮审核不会继承上一轮的批注文档，因此历史必须显式按
// review_case 把每一轮、每一位审核员的批注查出来，供「标注历史」回看与只读回放。
// 文件列表与批注记录分开查询：一条 SQL 里同时 LEFT JOIN 两个一对多表会产生
// 文件 × 记录的笛卡尔积，把批注挂到错误的文件上。
func (r *Repository) History(ctx context.Context, drawingNo, caseID string) ([]HistoryRound, error) {
	list := []HistoryRound{}
	if caseID == "" && strings.TrimSpace(drawingNo) == "" {
		return list, ErrNotFound
	}
	if caseID != "" && !ValidID(caseID) {
		return list, ErrNotFound
	}
	drawingNo = strings.TrimSpace(drawingNo)
	if len([]rune(drawingNo)) > 150 {
		return list, ErrNotFound
	}

	// 统一解析成 drawing_id 再查询：图号是业务键，不是数据库 ID。
	var drawingID, resolvedNo string
	if caseID != "" {
		err := r.pool.QueryRow(ctx, `SELECT c.drawing_id::text, d.drawing_no FROM review_cases c JOIN drawings d ON d.id = c.drawing_id WHERE c.id = $1::uuid`, caseID).
			Scan(&drawingID, &resolvedNo)
		if errors.Is(err, pgx.ErrNoRows) {
			return list, ErrNotFound
		}
		if err != nil {
			return list, err
		}
		// 图号与案例必须对得上，否则拿 A 图的历史入口能打开 B 图的批注。
		if drawingNo != "" && drawingNo != resolvedNo {
			return list, ErrNotFound
		}
		drawingNo = resolvedNo
	} else {
		err := r.pool.QueryRow(ctx, `SELECT id::text FROM drawings WHERE drawing_no = $1`, drawingNo).Scan(&drawingID)
		if errors.Is(err, pgx.ErrNoRows) {
			return list, nil
		}
		if err != nil {
			return list, err
		}
	}

	rounds, err := r.historyRounds(ctx, drawingID, drawingNo, caseID)
	if err != nil {
		return list, err
	}
	if len(rounds) == 0 {
		return list, nil
	}
	caseIDs := make([]string, 0, len(rounds))
	for index := range rounds {
		caseIDs = append(caseIDs, rounds[index].CaseID)
	}
	if err := r.fillHistoryFiles(ctx, caseIDs, rounds); err != nil {
		return list, err
	}
	if err := r.fillHistoryRecords(ctx, caseIDs, rounds); err != nil {
		return list, err
	}
	return rounds, nil
}

// historyRounds 先在本图纸的全部案例上算轮次号，再按 caseId 过滤：
// 若先 WHERE 再算窗口函数，任何一轮都会被算成「第 1 轮」。
func (r *Repository) historyRounds(ctx context.Context, drawingID, drawingNo, caseID string) ([]HistoryRound, error) {
	rows, err := r.pool.Query(ctx, `
		WITH ranked AS (
			SELECT c.id, c.status, c.started_at, c.completed_at, c.change_submission_id, c.flow_id,
			       c.initiator_id, c.flow_name_snapshot,
			       row_number() OVER (ORDER BY c.started_at, c.id)::int AS round_no
			FROM review_cases c
			WHERE c.drawing_id = $1::uuid
		)
		SELECT r.id::text, r.round_no, r.status,
		       COALESCE(NULLIF(r.flow_name_snapshot, ''), f.name, ''),
		       COALESCE(iu.display_name, iu.account, ''),
		       COALESCE(to_char(r.started_at, $2), ''), COALESCE(to_char(r.completed_at, $2), ''),
		       COALESCE(r.change_submission_id::text, ''), COALESCE(cr.request_no, ''), COALESCE(s.round, 0)::int
		FROM ranked r
		LEFT JOIN review_flows f ON f.id = r.flow_id
		LEFT JOIN users iu ON iu.id = r.initiator_id
		LEFT JOIN change_request_submissions s ON s.id = r.change_submission_id
		LEFT JOIN change_requests cr ON cr.id = s.request_id
		WHERE ($3 = '' OR r.id = $3::uuid)
		ORDER BY r.started_at DESC, r.id DESC`, drawingID, pgTimeLayout, caseID)
	if err != nil {
		return nil, fmt.Errorf("查询审核轮次失败: %w", err)
	}
	defer rows.Close()

	list := make([]HistoryRound, 0)
	for rows.Next() {
		item := HistoryRound{DrawingNo: drawingNo, Files: []HistoryFile{}, Records: []HistoryRecord{}}
		if err := rows.Scan(&item.CaseID, &item.Round, &item.Status, &item.Flow, &item.Initiator, &item.StartedAt,
			&item.CompletedAt, &item.ChangeSubmissionID, &item.ChangeRequestNo, &item.SubmissionRound); err != nil {
			return nil, fmt.Errorf("读取审核轮次失败: %w", err)
		}
		list = append(list, item)
	}
	return list, rows.Err()
}

// fillHistoryFiles 补齐每轮冻结的文件版本快照；与「有没有批注」无关，全新一轮也有文件。
func (r *Repository) fillHistoryFiles(ctx context.Context, caseIDs []string, rounds []HistoryRound) error {
	rows, err := r.pool.Query(ctx, `
		SELECT f.review_case_id::text, f.attachment_id::text, f.version_id::text, a.logical_name, v.version
		FROM review_annotation_files f
		JOIN attachments a ON a.id = f.attachment_id
		JOIN attachment_versions v ON v.id = f.version_id
		WHERE f.review_case_id = ANY($1::uuid[])
		ORDER BY a.logical_name`, caseIDs)
	if err != nil {
		return fmt.Errorf("查询审核轮次文件失败: %w", err)
	}
	defer rows.Close()

	byCase := make(map[string]*HistoryRound, len(rounds))
	for index := range rounds {
		byCase[rounds[index].CaseID] = &rounds[index]
	}
	for rows.Next() {
		var caseID string
		var item HistoryFile
		if err := rows.Scan(&caseID, &item.AttachmentID, &item.VersionID, &item.Name, &item.Version); err != nil {
			return fmt.Errorf("读取审核轮次文件失败: %w", err)
		}
		if target, ok := byCase[caseID]; ok {
			target.Files = append(target.Files, item)
		}
	}
	return rows.Err()
}

// fillHistoryRecords 补齐每轮的批注记录；记录直接带上文件与版本，
// 一轮多文件时才能说清「这条意见属于哪张图」。只取文字意见，不返回笔迹点。
func (r *Repository) fillHistoryRecords(ctx context.Context, caseIDs []string, rounds []HistoryRound) error {
	rows, err := r.pool.Query(ctx, `
		SELECT d.review_case_id::text, d.attachment_id::text, f.version_id::text, a.logical_name, v.version,
		       d.id::text, d.node_id::text, COALESCE(n.name, ''), COALESCE(n.signer_role, ''), COALESCE(n.status, ''),
		       COALESCE(n.opinion, ''), d.author_id::text, COALESCE(u.display_name, u.account, '未知用户'),
		       d.revision, COALESCE(to_char(d.updated_at, 'YYYY-MM-DD HH24:MI:SS'), ''),
		       COALESCE(jsonb_array_length(d.content->'marks'), 0)::int,
		       COALESCE((SELECT jsonb_agg(jsonb_build_object('kind', m->>'kind', 'text', m->>'text'))
		                 FROM jsonb_array_elements(d.content->'marks') m
		                 WHERE COALESCE(m->>'text', '') <> ''), '[]'::jsonb)
		FROM review_annotation_documents d
		JOIN review_annotation_files f ON f.review_case_id = d.review_case_id AND f.attachment_id = d.attachment_id
		JOIN attachments a ON a.id = d.attachment_id
		LEFT JOIN attachment_versions v ON v.id = f.version_id
		LEFT JOIN review_case_nodes n ON n.id = d.node_id
		LEFT JOIN users u ON u.id = d.author_id
		WHERE d.review_case_id = ANY($1::uuid[])
		ORDER BY n.node_order, a.logical_name, u.display_name`, caseIDs)
	if err != nil {
		return fmt.Errorf("查询审核批注记录失败: %w", err)
	}
	defer rows.Close()

	byCase := make(map[string]*HistoryRound, len(rounds))
	for index := range rounds {
		byCase[rounds[index].CaseID] = &rounds[index]
	}
	for rows.Next() {
		var caseID string
		var raw []byte
		item := HistoryRecord{Texts: []HistoryText{}}
		if err := rows.Scan(&caseID, &item.AttachmentID, &item.VersionID, &item.FileName, &item.FileVersion,
			&item.DocumentID, &item.NodeID, &item.NodeName, &item.SignerRole, &item.NodeStatus, &item.NodeOpinion,
			&item.AuthorID, &item.AuthorName, &item.Revision, &item.UpdatedAt, &item.MarkCount, &raw); err != nil {
			return fmt.Errorf("读取审核批注记录失败: %w", err)
		}
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &item.Texts); err != nil {
				return fmt.Errorf("解析审核批注文字失败: %w", err)
			}
		}
		if target, ok := byCase[caseID]; ok {
			target.Records = append(target.Records, item)
		}
	}
	return rows.Err()
}
