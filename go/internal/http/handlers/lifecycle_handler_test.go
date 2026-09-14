package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"sync"
	"testing"

	"cadguanliq/internal/dbtest"
	"cadguanliq/internal/storage"
)

// 本文件验证生命周期资料接口。资料是历史证据，写入后不可修改或删除；
// 因此在写入前必须严格校验归属、目录与权限，否则错误资料会永久留在档案里。

// docUpload 描述一次资料上传。
type docUpload struct {
	fields     map[string]string
	filename   string
	content    []byte
	omitFile   bool
}

// doUpload 执行资料上传并返回响应与存储。
func doUpload(t *testing.T, db *dbtest.DB, objects storage.ObjectStorage, actor patentActor, payload docUpload) (*httptest.ResponseRecorder, storage.ObjectStorage) {
	t.Helper()
	handler := LifecycleDocuments(db.Pool, objects)

	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	for key, value := range payload.fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	if !payload.omitFile {
		name := payload.filename
		if name == "" {
			name = "资料.pdf"
		}
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, name))
		header.Set("Content-Type", "application/pdf")
		part, err := writer.CreatePart(header)
		if err != nil {
			t.Fatal(err)
		}
		content := payload.content
		if content == nil {
			content = []byte("%PDF-1.4 资料内容")
		}
		if _, err = part.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	writer.Close()

	request := httptest.NewRequest(http.MethodPost, "/api/lifecycle-documents", &buffer)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, withActor(request, actor))
	return recorder, objects
}

// TestLifecycleDocumentRequiresExactlyOneOwner 归属必须且只能是图纸或专利之一。
func TestLifecycleDocumentRequiresExactlyOneOwner(t *testing.T) {
	db := dbtest.New(t)
	seed := seedPatentUsers(t, db)
	actor := patentActor{ID: seed.Author}
	patent := createPatent(t, db, seed, patentActor{ID: seed.Owner})
	objects := newTestStorage(t)

	cases := []struct {
		name   string
		fields map[string]string
	}{
		{"都未指定", map[string]string{"category": "设计输入", "title": "资料"}},
		{"同时指定", map[string]string{"drawingId": seed.Drawing, "patentId": patent, "category": "设计输入", "title": "资料"}},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			recorder, _ := doUpload(t, db, objects, actor, docUpload{fields: item.fields})
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("应返回 400，实际 %d %s", recorder.Code, recorder.Body.String())
			}
		})
	}
	count := db.ScanString(t, `SELECT count(*)::text FROM lifecycle_documents`)
	if count != "0" {
		t.Fatalf("非法请求不应写入资料，实际 %s 条", count)
	}
}

// TestLifecycleDocumentValidatesCategoryAndTitle 分类与标题不能为空。
func TestLifecycleDocumentValidatesCategoryAndTitle(t *testing.T) {
	db := dbtest.New(t)
	seed := seedPatentUsers(t, db)
	actor := patentActor{ID: seed.Author}
	objects := newTestStorage(t)

	for _, fields := range []map[string]string{
		{"drawingId": seed.Drawing, "category": "   ", "title": "资料"},
		{"drawingId": seed.Drawing, "category": "设计输入", "title": "  "},
	} {
		recorder, _ := doUpload(t, db, objects, actor, docUpload{fields: fields})
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("应返回 400，实际 %d %s", recorder.Code, recorder.Body.String())
		}
	}
}

// TestLifecycleDocumentValidatesFolderPath 目录层级与相对路径符号必须受限。
func TestLifecycleDocumentValidatesFolderPath(t *testing.T) {
	db := dbtest.New(t)
	seed := seedPatentUsers(t, db)
	actor := patentActor{ID: seed.Author}
	objects := newTestStorage(t)

	// 超过 10 层。
	deep := strings.TrimSuffix(strings.Repeat("a/", 11), "/")
	// 含相对路径符号。
	cases := []struct {
		name   string
		folder string
	}{
		{"超过十层", deep},
		{"包含上级目录", "a/../b"},
		{"包含当前目录", "a/./b"},
		{"包含空段", "a//b"},
		{"超过 500 字节", strings.Repeat("a", 501)},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			recorder, _ := doUpload(t, db, objects, actor, docUpload{fields: map[string]string{
				"drawingId": seed.Drawing, "category": "设计输入", "title": "资料", "folderPath": item.folder,
			}})
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("应返回 400，实际 %d %s", recorder.Code, recorder.Body.String())
			}
		})
	}
	// 合法的多层目录必须可用，且反斜杠被规范化为正斜杠。
	recorder, _ := doUpload(t, db, objects, actor, docUpload{fields: map[string]string{
		"drawingId": seed.Drawing, "category": "设计输入", "title": "合法目录", "folderPath": `a\b\c`,
	}})
	if recorder.Code != http.StatusCreated {
		t.Fatalf("合法目录应可上传，实际 %d %s", recorder.Code, recorder.Body.String())
	}
	folder := db.ScanString(t, `SELECT folder_path FROM lifecycle_documents WHERE title='合法目录'`)
	if folder != "a/b/c" {
		t.Fatalf("反斜杠应规范化为正斜杠，实际 %q", folder)
	}
}

// TestLifecycleDocumentRejectsEmptyFile 空文件不得作为归档资料。
func TestLifecycleDocumentRejectsEmptyFile(t *testing.T) {
	db := dbtest.New(t)
	seed := seedPatentUsers(t, db)
	actor := patentActor{ID: seed.Author}
	objects := newTestStorage(t)

	recorder, objects := doUpload(t, db, objects, actor, docUpload{
		fields:  map[string]string{"drawingId": seed.Drawing, "category": "设计输入", "title": "空文件"},
		content: []byte{},
	})
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("空文件应返回 400，实际 %d %s", recorder.Code, recorder.Body.String())
	}
	count := db.ScanString(t, `SELECT count(*)::text FROM lifecycle_documents`)
	if count != "0" {
		t.Fatalf("空文件不应写入记录，实际 %s 条", count)
	}
}

// TestLifecycleDocumentRequiresFile 缺少文件字段必须拒绝。
func TestLifecycleDocumentRequiresFile(t *testing.T) {
	db := dbtest.New(t)
	seed := seedPatentUsers(t, db)
	actor := patentActor{ID: seed.Author}
	objects := newTestStorage(t)

	recorder, _ := doUpload(t, db, objects, actor, docUpload{
		fields:   map[string]string{"drawingId": seed.Drawing, "category": "设计输入", "title": "无文件"},
		omitFile: true,
	})
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("缺少文件应返回 400，实际 %d %s", recorder.Code, recorder.Body.String())
	}
}

// TestLifecycleDocumentRequiresLogin 未登录不得上传。
func TestLifecycleDocumentRequiresLogin(t *testing.T) {
	db := dbtest.New(t)
	objects := newTestStorage(t)
	handler := LifecycleDocuments(db.Pool, objects)
	request := httptest.NewRequest(http.MethodPost, "/api/lifecycle-documents", strings.NewReader(""))
	request.Header.Set("Content-Type", "multipart/form-data; boundary=x")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("未登录应返回 401，实际 %d", recorder.Code)
	}
}

// TestLifecycleDocumentChangeAttachmentPermissions 变更材料仅申请人、设计员或管理员可追加。
func TestLifecycleDocumentChangeAttachmentPermissions(t *testing.T) {
	db := dbtest.New(t)
	seed := seedPatentUsers(t, db)
	objects := newTestStorage(t)
	// 变更单：申请人与执行人都是 Author，Outsider 不参与。
	var request string
	if err := db.Pool.QueryRow(context.Background(), `INSERT INTO change_requests(request_no,drawing_id,drawing_no,reason,scope,base_drawing_revision,applicant_id,executor_id,status)
		VALUES('CR-DOC',$1::uuid,'D-1','原因','part',1,$2::uuid,$2::uuid,'executing') RETURNING id::text`, seed.Drawing, seed.Author).Scan(&request); err != nil {
		t.Fatal(err)
	}
	fields := map[string]string{"drawingId": seed.Drawing, "changeRequestId": request, "category": "变更材料", "title": "变更前图纸"}

	// 外部人必须被拒绝。
	recorder, _ := doUpload(t, db, objects, patentActor{ID: seed.Outsider}, docUpload{fields: fields})
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("无关用户应返回 403，实际 %d %s", recorder.Code, recorder.Body.String())
	}
	// 申请人可以上传。
	recorder, _ = doUpload(t, db, objects, patentActor{ID: seed.Author}, docUpload{fields: fields})
	if recorder.Code != http.StatusCreated {
		t.Fatalf("申请人应可上传，实际 %d %s", recorder.Code, recorder.Body.String())
	}
	// 管理员可以上传。
	recorder, _ = doUpload(t, db, objects, patentActor{ID: seed.Admin, Roles: []string{"admin"}}, docUpload{fields: fields})
	if recorder.Code != http.StatusCreated {
		t.Fatalf("管理员应可上传，实际 %d %s", recorder.Code, recorder.Body.String())
	}
}

// TestLifecycleDocumentRejectsChangeFromAnotherDrawing 工单必须属于当前图号。
func TestLifecycleDocumentRejectsChangeFromAnotherDrawing(t *testing.T) {
	db := dbtest.New(t)
	seed := seedPatentUsers(t, db)
	objects := newTestStorage(t)
	var request string
	if err := db.Pool.QueryRow(context.Background(), `INSERT INTO change_requests(request_no,drawing_id,drawing_no,reason,scope,base_drawing_revision,applicant_id,executor_id,status)
		VALUES('CR-OTHER',$1::uuid,'D-1','原因','part',1,$2::uuid,$2::uuid,'executing') RETURNING id::text`, seed.Drawing, seed.Author).Scan(&request); err != nil {
		t.Fatal(err)
	}
	// 换一张图纸提交同一工单。
	otherDrawing := db.ScanString(t, `INSERT INTO drawings(drawing_no,name,project,created_by,status) VALUES('D-9','另一图纸','P',$1::uuid,'draft') RETURNING id::text`, seed.Author)

	recorder, _ := doUpload(t, db, objects, patentActor{ID: seed.Author}, docUpload{fields: map[string]string{
		"drawingId": otherDrawing, "changeRequestId": request, "category": "变更材料", "title": "错归属",
	}})
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("工单不属于当前图号应返回 400，实际 %d %s", recorder.Code, recorder.Body.String())
	}
}

// TestLifecycleDocumentFrozenAfterSubmission 已提交审核的变更资料被冻结，退回后可追加。
//
// 这是“历史证据”原则的直接体现：审核员看到的材料必须与签署时一致，
// 提交后再追加材料会让审核结论失去依据。
func TestLifecycleDocumentFrozenAfterSubmission(t *testing.T) {
	db := dbtest.New(t)
	seed := seedPatentUsers(t, db)
	objects := newTestStorage(t)
	var request string
	if err := db.Pool.QueryRow(context.Background(), `INSERT INTO change_requests(request_no,drawing_id,drawing_no,reason,scope,base_drawing_revision,applicant_id,executor_id,status)
		VALUES('CR-FROZEN',$1::uuid,'D-1','原因','part',1,$2::uuid,$2::uuid,'executing') RETURNING id::text`, seed.Drawing, seed.Author).Scan(&request); err != nil {
		t.Fatal(err)
	}
	fields := map[string]string{"drawingId": seed.Drawing, "changeRequestId": request, "category": "变更材料", "title": "材料"}
	actor := patentActor{ID: seed.Author}

	// 执行中可追加。
	recorder, _ := doUpload(t, db, objects, actor, docUpload{fields: fields})
	if recorder.Code != http.StatusCreated {
		t.Fatalf("执行中应可追加材料，实际 %d %s", recorder.Code, recorder.Body.String())
	}

	// 进入待完整审核后冻结。
	db.Exec(t, `UPDATE change_requests SET status='pending_verify' WHERE id=$1::uuid`, request)
	recorder, _ = doUpload(t, db, objects, actor, docUpload{fields: fields})
	if recorder.Code != http.StatusConflict {
		t.Fatalf("审核中应返回 409，实际 %d %s", recorder.Code, recorder.Body.String())
	}

	// 退回后恢复可追加。
	db.Exec(t, `UPDATE change_requests SET status='executing' WHERE id=$1::uuid`, request)
	recorder, _ = doUpload(t, db, objects, actor, docUpload{fields: fields})
	if recorder.Code != http.StatusCreated {
		t.Fatalf("退回后应可追加，实际 %d %s", recorder.Code, recorder.Body.String())
	}
}

// TestLifecycleDocumentPatentPermissions 专利资料仅负责人或管理员可追加。
func TestLifecycleDocumentPatentPermissions(t *testing.T) {
	db := dbtest.New(t)
	seed := seedPatentUsers(t, db)
	objects := newTestStorage(t)
	patent := createPatent(t, db, seed, patentActor{ID: seed.Owner})
	fields := map[string]string{"patentId": patent, "category": "专利证书", "title": "受理通知书"}

	recorder, _ := doUpload(t, db, objects, patentActor{ID: seed.Outsider}, docUpload{fields: fields})
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("非负责人应返回 403，实际 %d %s", recorder.Code, recorder.Body.String())
	}
	recorder, _ = doUpload(t, db, objects, patentActor{ID: seed.Owner}, docUpload{fields: fields})
	if recorder.Code != http.StatusCreated {
		t.Fatalf("负责人应可上传，实际 %d %s", recorder.Code, recorder.Body.String())
	}
	// 不存在的专利。
	recorder, _ = doUpload(t, db, objects, patentActor{ID: seed.Owner}, docUpload{fields: map[string]string{
		"patentId": "00000000-0000-0000-0000-000000000000", "category": "专利证书", "title": "不存在",
	}})
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("不存在的专利应返回 404，实际 %d %s", recorder.Code, recorder.Body.String())
	}
}

// TestLifecycleDocumentListRequiresOneFilter 列表查询必须指定图纸或专利。
func TestLifecycleDocumentListRequiresOneFilter(t *testing.T) {
	db := dbtest.New(t)
	seed := seedPatentUsers(t, db)
	objects := newTestStorage(t)
	handler := LifecycleDocuments(db.Pool, objects)

	// 两个都提供。
	request := httptest.NewRequest(http.MethodGet, "/api/lifecycle-documents?drawingId="+seed.Drawing+"&patentId="+seed.Owner, nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, withActor(request, patentActor{ID: seed.Author}))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("同时指定应返回 400，实际 %d %s", recorder.Code, recorder.Body.String())
	}
	// 都不提供。
	request = httptest.NewRequest(http.MethodGet, "/api/lifecycle-documents", nil)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, withActor(request, patentActor{ID: seed.Author}))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("未指定应返回 400，实际 %d %s", recorder.Code, recorder.Body.String())
	}
}

// TestLifecycleDocumentDownloadRequiresRecord 下载不存在的资料返回 404。
func TestLifecycleDocumentDownloadRequiresRecord(t *testing.T) {
	db := dbtest.New(t)
	seed := seedPatentUsers(t, db)
	objects := newTestStorage(t)
	handler := LifecycleDocuments(db.Pool, objects)

	request := httptest.NewRequest(http.MethodGet, "/api/lifecycle-documents/00000000-0000-0000-0000-000000000000", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, withActor(request, patentActor{ID: seed.Author}))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("不存在应返回 404，实际 %d", recorder.Code)
	}
}

// TestLifecycleDocumentUnsupportedMethod 不支持的方法返回 405。
func TestLifecycleDocumentUnsupportedMethod(t *testing.T) {
	db := dbtest.New(t)
	seed := seedPatentUsers(t, db)
	objects := newTestStorage(t)
	handler := LifecycleDocuments(db.Pool, objects)

	request := httptest.NewRequest(http.MethodDelete, "/api/lifecycle-documents", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, withActor(request, patentActor{ID: seed.Author}))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("删除应返回 405，实际 %d", recorder.Code)
	}
}

// TestLifecycleDocumentListFiltersBySubmission 可按提交轮次筛选材料，供审核页展示。
func TestLifecycleDocumentListFiltersBySubmission(t *testing.T) {
	db := dbtest.New(t)
	seed := seedPatentUsers(t, db)
	objects := newTestStorage(t)
	// 建立工单与提交轮次。
	var request, submission string
	if err := db.Pool.QueryRow(context.Background(), `INSERT INTO change_requests(request_no,drawing_id,drawing_no,reason,scope,base_drawing_revision,applicant_id,executor_id,status)
		VALUES('CR-LIST',$1::uuid,'D-1','原因','part',1,$2::uuid,$2::uuid,'executing') RETURNING id::text`, seed.Drawing, seed.Author).Scan(&request); err != nil {
		t.Fatal(err)
	}
	if err := db.Pool.QueryRow(context.Background(), `INSERT INTO change_request_submissions(request_id,round,actual_changes,status) VALUES($1::uuid,1,'内容','pending') RETURNING id::text`, request).Scan(&submission); err != nil {
		t.Fatal(err)
	}
	// 两份图纸资料，其中一份关联到该轮次。
	fields := map[string]string{"drawingId": seed.Drawing, "category": "设计输入", "title": "通用资料"}
	recorder, _ := doUpload(t, db, objects, patentActor{ID: seed.Author}, docUpload{fields: fields})
	if recorder.Code != http.StatusCreated {
		t.Fatalf("上传失败: %s", recorder.Body.String())
	}
	linked, _ := doUpload(t, db, objects, patentActor{ID: seed.Author}, docUpload{fields: map[string]string{
		"drawingId": seed.Drawing, "category": "变更材料", "title": "本轮材料",
	}})
	linkedID := decodeData(t, linked)["id"].(string)
	db.Exec(t, `INSERT INTO change_submission_documents(submission_id,document_id) VALUES($1::uuid,$2::uuid)`, submission, linkedID)

	handler := LifecycleDocuments(db.Pool, objects)
	request2 := httptest.NewRequest(http.MethodGet, "/api/lifecycle-documents?drawingId="+seed.Drawing+"&submissionId="+submission, nil)
	result := httptest.NewRecorder()
	handler.ServeHTTP(result, withActor(request2, patentActor{ID: seed.Author}))
	if result.Code != http.StatusOK {
		t.Fatalf("列表应返回 200，实际 %d %s", result.Code, result.Body.String())
	}
	var body struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(result.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) != 1 {
		t.Fatalf("按轮次筛选应只返回 1 份材料，实际 %d", len(body.Data))
	}
	if body.Data[0]["title"] != "本轮材料" {
		t.Fatalf("筛选结果不正确: %v", body.Data[0]["title"])
	}
}

// TestLifecycleDocumentConcurrentUploadsAllPersisted 并发上传必须全部成功且互不覆盖。
//
// 资料是历史证据，任何一份丢失都不可恢复。
func TestLifecycleDocumentConcurrentUploadsAllPersisted(t *testing.T) {
	db := dbtest.New(t)
	seed := seedPatentUsers(t, db)
	objects := newTestStorage(t)
	actor := patentActor{ID: seed.Author}

	const workers = 6
	var wg sync.WaitGroup
	codes := make([]int, workers)
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			recorder, _ := doUpload(t, db, objects, actor, docUpload{fields: map[string]string{
				"drawingId": seed.Drawing, "category": "设计输入", "title": fmt.Sprintf("并发资料-%d", index),
			}})
			codes[index] = recorder.Code
		}(i)
	}
	close(start)
	wg.Wait()

	for index, code := range codes {
		if code != http.StatusCreated {
			t.Fatalf("并发上传 %d 应成功，实际 %d", index, code)
		}
	}
	count := db.ScanString(t, `SELECT count(*)::text FROM lifecycle_documents WHERE drawing_id=$1::uuid`, seed.Drawing)
	if count != fmt.Sprintf("%d", workers) {
		t.Fatalf("应保存 %d 份资料，实际 %s", workers, count)
	}
	// 每份资料必须有独立的存储键，否则会互相覆盖。
	distinct := db.ScanString(t, `SELECT count(DISTINCT storage_key)::text FROM lifecycle_documents WHERE drawing_id=$1::uuid`, seed.Drawing)
	if distinct != fmt.Sprintf("%d", workers) {
		t.Fatalf("存储键必须互不相同，实际 %s 个", distinct)
	}
}

// TestLifecycleDocumentDetailRequiresDesignerForChange 变更材料的申请人校验在事务内完成，
// 确保并发下不会因状态变化而写入不该写入的材料。
func TestLifecycleDocumentConcurrentUploadDuringStatusChange(t *testing.T) {
	db := dbtest.New(t)
	seed := seedPatentUsers(t, db)
	objects := newTestStorage(t)
	var request string
	if err := db.Pool.QueryRow(context.Background(), `INSERT INTO change_requests(request_no,drawing_id,drawing_no,reason,scope,base_drawing_revision,applicant_id,executor_id,status)
		VALUES('CR-RACE',$1::uuid,'D-1','原因','part',1,$2::uuid,$2::uuid,'executing') RETURNING id::text`, seed.Drawing, seed.Author).Scan(&request); err != nil {
		t.Fatal(err)
	}
	fields := map[string]string{"drawingId": seed.Drawing, "changeRequestId": request, "category": "变更材料", "title": "竞争材料"}
	actor := patentActor{ID: seed.Author}

	// 与上传并发地把工单切换到已提交状态。
	var wg sync.WaitGroup
	start := make(chan struct{})
	var uploadCode int
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		recorder, _ := doUpload(t, db, objects, actor, docUpload{fields: fields})
		uploadCode = recorder.Code
	}()
	go func() {
		defer wg.Done()
		<-start
		_, _ = db.Pool.Exec(context.Background(), `UPDATE change_requests SET status='pending_verify' WHERE id=$1::uuid`, request)
	}()
	close(start)
	wg.Wait()

	// 允许两种结果：抢在状态切换前写入（201），或已被冻结（409）。
	// 不允许的是“状态已冻结却仍然写入成功”。
	if uploadCode != http.StatusCreated && uploadCode != http.StatusConflict {
		t.Fatalf("并发结果只能是 201 或 409，实际 %d", uploadCode)
	}
	if uploadCode == http.StatusConflict {
		count := db.ScanString(t, `SELECT count(*)::text FROM lifecycle_documents WHERE change_request_id=$1::uuid`, request)
		if count != "0" {
			t.Fatalf("被拒绝时不应写入资料，实际 %s 条", count)
		}
	}
}

// TestPatentsRejectMalformedJSON 专利接口对非法 JSON 返回 400。
func TestPatentsRejectMalformedJSON(t *testing.T) {
	db := dbtest.New(t)
	seed := seedPatentUsers(t, db)
	handler := Patents(db.Pool)
	request := httptest.NewRequest(http.MethodPost, "/api/patents", strings.NewReader("{不是 JSON"))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, withActor(request, patentActor{ID: seed.Owner}))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("非法 JSON 应返回 400，实际 %d %s", recorder.Code, recorder.Body.String())
	}
}
