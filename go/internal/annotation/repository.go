package annotation

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

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
