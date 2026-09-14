package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"cadguanliq/internal/dbtest"
)

// 本文件验证生命周期树与历史版本下载。
//
// 版本下载的鉴权不是基于角色，而是基于“该版本是否已被引用为历史证据”。
// 因此必须验证：被引用的版本可下载，未被引用的版本一律 404，
// 否则任何附件版本都会变成可通过猜测 UUID 读取的公开资源。

// TestLifecycleTreeRequiresKnownDrawing 生命周期树必须能按图号或 ID 命中。
func TestLifecycleTreeRequiresKnownDrawing(t *testing.T) {
	db := dbtest.New(t)
	seed := seedPatentUsers(t, db)
	handler := LifecycleTree(db.Pool)
	actor := patentActor{ID: seed.Author}

	// 按 drawingId 查询。
	request := httptest.NewRequest(http.MethodGet, "/api/lifecycle-tree?drawingId="+seed.Drawing, nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, withActor(request, actor))
	if recorder.Code != http.StatusOK {
		t.Fatalf("按 ID 查询应返回 200，实际 %d %s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data["drawingNo"] != "D-1" {
		t.Fatalf("应返回对应图纸，实际 %v", body.Data["drawingNo"])
	}
	// 未建立发布快照时 releases 必须为空数组而不是 null，避免前端报错。
	if body.Data["releases"] == nil {
		t.Fatal("releases 不应为 null")
	}

	// 按图号查询。
	request = httptest.NewRequest(http.MethodGet, "/api/lifecycle-tree?drawingNo=D-1", nil)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, withActor(request, actor))
	if recorder.Code != http.StatusOK {
		t.Fatalf("按图号查询应返回 200，实际 %d", recorder.Code)
	}
}

// TestLifecycleTreeUnknownDrawingReturnsError 未知图纸必须报错而不是返回空对象。
func TestLifecycleTreeUnknownDrawingReturnsError(t *testing.T) {
	db := dbtest.New(t)
	seed := seedPatentUsers(t, db)
	handler := LifecycleTree(db.Pool)

	request := httptest.NewRequest(http.MethodGet, "/api/lifecycle-tree?drawingNo=NOT-EXIST", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, withActor(request, patentActor{ID: seed.Author}))
	if recorder.Code == http.StatusOK {
		t.Fatalf("未知图纸不应返回 200: %s", recorder.Body.String())
	}
}

// TestLifecycleTreeRejectsNonGet 只允许读取。
func TestLifecycleTreeRejectsNonGet(t *testing.T) {
	db := dbtest.New(t)
	seed := seedPatentUsers(t, db)
	handler := LifecycleTree(db.Pool)
	request := httptest.NewRequest(http.MethodPost, "/api/lifecycle-tree", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, withActor(request, patentActor{ID: seed.Author}))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("非 GET 应返回 405，实际 %d", recorder.Code)
	}
}

// TestLifecycleTreeIncludesSubmissionsAndReviews 树必须包含提交轮次与审核节点，
// 否则生命周期页无法还原完整过程。
func TestLifecycleTreeIncludesSubmissionsAndReviews(t *testing.T) {
	db := dbtest.New(t)
	seed := seedPatentUsers(t, db)
	// 建立工单、提交轮次、审核单与节点。
	var request, submission, flow, caseID string
	if err := db.Pool.QueryRow(context.Background(), `INSERT INTO change_requests(request_no,drawing_id,drawing_no,reason,scope,base_drawing_revision,applicant_id,executor_id,status)
		VALUES('CR-TREE',$1::uuid,'D-1','生命周期原因','part',1,$2::uuid,$2::uuid,'pending_verify') RETURNING id::text`, seed.Drawing, seed.Author).Scan(&request); err != nil {
		t.Fatal(err)
	}
	if err := db.Pool.QueryRow(context.Background(), `INSERT INTO change_request_submissions(request_id,round,actual_changes,status) VALUES($1::uuid,1,'本轮实际变更','pending') RETURNING id::text`, request).Scan(&submission); err != nil {
		t.Fatal(err)
	}
	if err := db.Pool.QueryRow(context.Background(), `INSERT INTO review_flows(name,enabled,created_by) VALUES('完整审核',true,$1::uuid) RETURNING id::text`, seed.Author).Scan(&flow); err != nil {
		t.Fatal(err)
	}
	if err := db.Pool.QueryRow(context.Background(), `INSERT INTO review_cases(drawing_id,flow_id,status,initiator_id,change_submission_id,flow_name_snapshot)
		VALUES($1::uuid,$2::uuid,'reviewing',$3::uuid,$4::uuid,'完整审核') RETURNING id::text`, seed.Drawing, flow, seed.Author, submission).Scan(&caseID); err != nil {
		t.Fatal(err)
	}
	db.Exec(t, `INSERT INTO review_case_nodes(review_case_id,name,assigned_user_id,assigned_name,status,opinion,required,node_order,signer_role)
		VALUES($1::uuid,'校对复核',$2::uuid,'审核员','pass','同意',true,1,'校对')`, caseID, seed.Reviewer)
	db.Exec(t, `INSERT INTO change_request_actions(request_id,actor_id,action,opinion) VALUES($1::uuid,$2::uuid,'submit','提交本轮')`, request, seed.Author)

	handler := LifecycleTree(db.Pool)
	httpRequest := httptest.NewRequest(http.MethodGet, "/api/lifecycle-tree?drawingId="+seed.Drawing, nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, withActor(httpRequest, patentActor{ID: seed.Author}))
	if recorder.Code != http.StatusOK {
		t.Fatalf("查询应返回 200，实际 %d %s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Data struct {
			Changes []struct {
				RequestNo   string `json:"requestNo"`
				Reason      string `json:"reason"`
				Actions     []any  `json:"actions"`
				Submissions []struct {
					Round  int `json:"round"`
					Review struct {
						ID    string `json:"id"`
						Nodes []any  `json:"nodes"`
					} `json:"review"`
				} `json:"submissions"`
			} `json:"changes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析失败: %v body=%s", err, recorder.Body.String())
	}
	if len(body.Data.Changes) != 1 {
		t.Fatalf("应返回 1 条变更，实际 %d", len(body.Data.Changes))
	}
	change := body.Data.Changes[0]
	if change.Reason != "生命周期原因" {
		t.Fatalf("变更原因不正确: %s", change.Reason)
	}
	if len(change.Actions) != 1 {
		t.Fatalf("应返回 1 条动作记录，实际 %d", len(change.Actions))
	}
	if len(change.Submissions) != 1 {
		t.Fatalf("应返回 1 轮提交，实际 %d", len(change.Submissions))
	}
	if change.Submissions[0].Review.ID == "" {
		t.Fatal("提交轮次应关联审核单")
	}
	if len(change.Submissions[0].Review.Nodes) != 1 {
		t.Fatalf("审核单应包含 1 个节点，实际 %d", len(change.Submissions[0].Review.Nodes))
	}
}

// TestLifecycleVersionDownloadRequiresKnownVersion 未被引用的版本一律不可下载。
func TestLifecycleVersionDownloadRequiresKnownVersion(t *testing.T) {
	db := dbtest.New(t)
	seed := seedPatentUsers(t, db)
	objects := newTestStorage(t)
	handler := LifecycleVersion(db.Pool, objects)

	request := httptest.NewRequest(http.MethodGet, "/api/lifecycle-versions/00000000-0000-0000-0000-000000000000", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, withActor(request, patentActor{ID: seed.Author}))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("未知版本应返回 404，实际 %d", recorder.Code)
	}
}

// TestLifecycleVersionDownloadRequiresReference 只有被历史记录引用的版本可以下载。
//
// 普通工作版本不应通过该接口暴露，否则任何版本 UUID 都能被读成文件。
func TestLifecycleVersionDownloadRequiresReference(t *testing.T) {
	db := dbtest.New(t)
	seed := seedPatentUsers(t, db)
	objects := newTestStorage(t)
	handler := LifecycleVersion(db.Pool, objects)

	// 建立一个普通工作版本，未被任何历史记录引用。
	var version string
	if err := db.Pool.QueryRow(context.Background(), `INSERT INTO attachment_versions(attachment_id,version,original_name,blob_id,version_kind)
		VALUES($1::uuid,'v1.0','D-1.dwg',$2::uuid,'working') RETURNING id::text`, seed.Attachment, seed.Blob).Scan(&version); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/lifecycle-versions/"+version, nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, withActor(request, patentActor{ID: seed.Author}))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("未被引用的版本应返回 404，实际 %d", recorder.Code)
	}
}

// TestLifecycleVersionDownloadWorksForReferencedVersion 被提交轮次引用的版本可以下载。
func TestLifecycleVersionDownloadWorksForReferencedVersion(t *testing.T) {
	db := dbtest.New(t)
	seed := seedPatentUsers(t, db)
	objects := newTestStorage(t)
	// 上传一个真实对象，供下载读取。
	content := []byte("AC1015-historical-dwg")
	key := "blobs/historical"
	if err := putObject(t, objects, key, content); err != nil {
		t.Fatal(err)
	}
	var blob string
	if err := db.Pool.QueryRow(context.Background(), `INSERT INTO file_blobs(storage_key,mime_type,size_bytes,sha256) VALUES($1,'application/acad',$2,repeat('e',64)) RETURNING id::text`, key, len(content)).Scan(&blob); err != nil {
		t.Fatal(err)
	}
	var version string
	if err := db.Pool.QueryRow(context.Background(), `INSERT INTO attachment_versions(attachment_id,version,original_name,blob_id,version_kind)
		VALUES($1::uuid,'v1.0','历史版本.dwg',$2::uuid,'working') RETURNING id::text`, seed.Attachment, blob).Scan(&version); err != nil {
		t.Fatal(err)
	}
	// 把它登记为某轮提交的基线版本，即成为历史证据。
	var request, submission string
	if err := db.Pool.QueryRow(context.Background(), `INSERT INTO change_requests(request_no,drawing_id,drawing_no,reason,scope,base_drawing_revision,applicant_id,executor_id,status)
		VALUES('CR-VER',$1::uuid,'D-1','原因','part',1,$2::uuid,$2::uuid,'executing') RETURNING id::text`, seed.Drawing, seed.Author).Scan(&request); err != nil {
		t.Fatal(err)
	}
	if err := db.Pool.QueryRow(context.Background(), `INSERT INTO change_request_submissions(request_id,round,actual_changes,status) VALUES($1::uuid,1,'内容','pending') RETURNING id::text`, request).Scan(&submission); err != nil {
		t.Fatal(err)
	}
	db.Exec(t, `INSERT INTO change_request_submission_targets(submission_id,attachment_id,base_attachment_version_id,submitted_attachment_version_id)
		VALUES($1::uuid,$2::uuid,$3::uuid,$3::uuid)`, submission, seed.Attachment, version)

	handler := LifecycleVersion(db.Pool, objects)
	httpRequest := httptest.NewRequest(http.MethodGet, "/api/lifecycle-versions/"+version, nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, withActor(httpRequest, patentActor{ID: seed.Author}))
	if recorder.Code != http.StatusOK {
		t.Fatalf("被引用的版本应可下载，实际 %d %s", recorder.Code, recorder.Body.String())
	}
	if recorder.Body.String() != string(content) {
		t.Fatalf("下载内容不一致: %q", recorder.Body.String())
	}
	// 必须禁止内容嗅探并强制下载，避免历史文件被浏览器当作活动内容执行。
	if recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("应设置 X-Content-Type-Options: nosniff")
	}
	if recorder.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("应禁止缓存历史文件，实际 %q", recorder.Header().Get("Cache-Control"))
	}
	if disposition := recorder.Header().Get("Content-Disposition"); disposition == "" || disposition[:10] != "attachment" {
		t.Fatalf("应作为附件下载，实际 %q", disposition)
	}
}

// TestLifecycleVersionDownloadRejectsNonGet 只允许下载。
func TestLifecycleVersionDownloadRejectsNonGet(t *testing.T) {
	db := dbtest.New(t)
	seed := seedPatentUsers(t, db)
	objects := newTestStorage(t)
	handler := LifecycleVersion(db.Pool, objects)
	request := httptest.NewRequest(http.MethodDelete, "/api/lifecycle-versions/00000000-0000-0000-0000-000000000000", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, withActor(request, patentActor{ID: seed.Author}))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("非 GET 应返回 405，实际 %d", recorder.Code)
	}
}

// TestLifecycleDocumentDownloadServesContent 已归档资料可以下载并带上防护头。
func TestLifecycleDocumentDownloadServesContent(t *testing.T) {
	db := dbtest.New(t)
	seed := seedPatentUsers(t, db)
	objects := newTestStorage(t)
	actor := patentActor{ID: seed.Author}

	uploaded, _ := doUpload(t, db, objects, actor, docUpload{fields: map[string]string{
		"drawingId": seed.Drawing, "category": "设计输入", "title": "可下载资料",
	}, content: []byte("PDF-资料正文")})
	if uploaded.Code != http.StatusCreated {
		t.Fatalf("上传失败: %s", uploaded.Body.String())
	}
	id := decodeData(t, uploaded)["id"].(string)

	handler := LifecycleDocuments(db.Pool, objects)
	request := httptest.NewRequest(http.MethodGet, "/api/lifecycle-documents/"+id, nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, withActor(request, actor))
	if recorder.Code != http.StatusOK {
		t.Fatalf("下载应返回 200，实际 %d %s", recorder.Code, recorder.Body.String())
	}
	if recorder.Body.String() != "PDF-资料正文" {
		t.Fatalf("下载内容不一致: %q", recorder.Body.String())
	}
	if recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("应设置 nosniff")
	}
}
