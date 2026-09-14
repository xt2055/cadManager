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

	"cadguanliq/internal/auth"
	"cadguanliq/internal/dbtest"
	"cadguanliq/internal/http/middleware"
	"cadguanliq/internal/storage"
)

// 本文件验证专利记录与生命周期资料的 HTTP 层：权限、乐观锁、日期与金额校验。
// 这些规则只在请求边界生效，且失败时用户看到的是静默的错误数据而非崩溃，
// 因此需要逐条固定。

// patentActor 描述发起请求的用户。
type patentActor struct {
	ID    string
	Roles []string
}

// withActor 为请求注入登录用户。
func withActor(request *http.Request, actor patentActor) *http.Request {
	return request.WithContext(context.WithValue(request.Context(), middleware.AuthUserContextKey, auth.AuthUser{ID: actor.ID, Roles: actor.Roles}))
}

// patentSeed 建立两个用户（负责人与外部人）与一个管理员。
type patentSeed struct {
	dbtest.Fixture
	Owner     string
	Outsider  string
	Admin     string
}

func seedPatentUsers(t *testing.T, db *dbtest.DB) patentSeed {
	t.Helper()
	fixture := db.Seed(t)
	seed := patentSeed{Fixture: fixture, Owner: fixture.Reviewer}
	// 负责人需要一个独立用户，避免与 fixtures 中角色混淆。
	seed.Owner = db.ScanString(t, `SELECT id::text FROM users WHERE account='reviewer'`)
	seed.Outsider = db.ScanString(t, `INSERT INTO users(account,display_name,password_hash,status) VALUES('outsider','外部人','x','active') RETURNING id::text`)
	seed.Admin = db.ScanString(t, `INSERT INTO users(account,display_name,password_hash,status) VALUES('admin','管理员','x','active') RETURNING id::text`)
	return seed
}

// postJSON 以 JSON 发送请求并返回响应。
func postJSON(t *testing.T, handler http.Handler, actor patentActor, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, withActor(request, actor))
	return recorder
}

// decodeData 解析统一响应体中的 data 字段。
func decodeData(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析响应失败: %v body=%s", err, recorder.Body.String())
	}
	return body.Data
}

// createPatent 建立一个专利并返回其 id。
func createPatent(t *testing.T, db *dbtest.DB, seed patentSeed, owner patentActor) string {
	t.Helper()
	handler := Patents(db.Pool)
	recorder := postJSON(t, handler, owner, http.MethodPost, "/api/patents", map[string]any{
		"number":         "CN-2024-001",
		"title":          "一种测试装置",
		"deadlineSource": "年费通知",
		"reminderDays":   90,
	})
	if recorder.Code != http.StatusOK {
		t.Fatalf("建立专利失败: %d %s", recorder.Code, recorder.Body.String())
	}
	return decodeData(t, recorder)["id"].(string)
}

// TestPatentsRequireLogin 未登录不得读写专利。
func TestPatentsRequireLogin(t *testing.T) {
	db := dbtest.New(t)
	handler := Patents(db.Pool)
	request := httptest.NewRequest(http.MethodGet, "/api/patents", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("未登录应返回 401，实际 %d", recorder.Code)
	}
}

// TestPatentsCreateValidatesRequiredFields 必填字段缺失或提前量越界必须拒绝。
func TestPatentsCreateValidatesRequiredFields(t *testing.T) {
	db := dbtest.New(t)
	seed := seedPatentUsers(t, db)
	owner := patentActor{ID: seed.Owner}
	handler := Patents(db.Pool)

	cases := []struct {
		name string
		body map[string]any
	}{
		{"缺少编号", map[string]any{"title": "标题", "deadlineSource": "依据", "reminderDays": 90}},
		{"缺少名称", map[string]any{"number": "N-1", "deadlineSource": "依据", "reminderDays": 90}},
		{"缺少期限依据", map[string]any{"number": "N-1", "title": "标题", "reminderDays": 90}},
		{"提前量为零", map[string]any{"number": "N-1", "title": "标题", "deadlineSource": "依据", "reminderDays": 0}},
		{"提前量超上限", map[string]any{"number": "N-1", "title": "标题", "deadlineSource": "依据", "reminderDays": 366}},
		{"日期格式错误", map[string]any{"number": "N-1", "title": "标题", "deadlineSource": "依据", "reminderDays": 90, "feeDue": "2024/01/01"}},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			recorder := postJSON(t, handler, owner, http.MethodPost, "/api/patents", item.body)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("应拒绝该请求，实际 %d %s", recorder.Code, recorder.Body.String())
			}
		})
	}
	// 全部非法请求都不应留下记录。
	count := db.ScanString(t, `SELECT count(*)::text FROM patent_records`)
	if count != "0" {
		t.Fatalf("非法请求不应写入记录，实际 %s 条", count)
	}
}

// TestPatentsCreateRejectsDuplicateNumber 专利编号唯一。
func TestPatentsCreateRejectsDuplicateNumber(t *testing.T) {
	db := dbtest.New(t)
	seed := seedPatentUsers(t, db)
	owner := patentActor{ID: seed.Owner}
	createPatent(t, db, seed, owner)

	handler := Patents(db.Pool)
	recorder := postJSON(t, handler, owner, http.MethodPost, "/api/patents", map[string]any{
		"number": "CN-2024-001", "title": "重复编号", "deadlineSource": "依据", "reminderDays": 90,
	})
	if recorder.Code == http.StatusOK {
		t.Fatal("重复编号必须被拒绝")
	}
}

// TestPatentsUpdateRequiresOwnershipOrAdmin 只有负责人或管理员可以修改。
func TestPatentsUpdateRequiresOwnershipOrAdmin(t *testing.T) {
	db := dbtest.New(t)
	seed := seedPatentUsers(t, db)
	owner := patentActor{ID: seed.Owner}
	patent := createPatent(t, db, seed, owner)
	handler := Patents(db.Pool)

	update := map[string]any{
		"number": "CN-2024-001", "title": "被改标题", "deadlineSource": "年费通知", "reminderDays": 90, "revision": 1,
	}
	// 外部人必须被拒绝。
	recorder := postJSON(t, handler, patentActor{ID: seed.Outsider}, http.MethodPut, "/api/patents/"+patent, update)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("非负责人应返回 403，实际 %d %s", recorder.Code, recorder.Body.String())
	}
	// 管理员可以修改。
	recorder = postJSON(t, handler, patentActor{ID: seed.Admin, Roles: []string{"admin"}}, http.MethodPut, "/api/patents/"+patent, update)
	if recorder.Code != http.StatusOK {
		t.Fatalf("管理员应可修改，实际 %d %s", recorder.Code, recorder.Body.String())
	}
	title := db.ScanString(t, `SELECT title FROM patent_records WHERE id=$1::uuid`, patent)
	if title != "被改标题" {
		t.Fatalf("修改未生效，实际 %s", title)
	}
}

// TestPatentsUpdateOnlyAdminCanChangeOwner 只有管理员可以调整负责人。
func TestPatentsUpdateOnlyAdminCanChangeOwner(t *testing.T) {
	db := dbtest.New(t)
	seed := seedPatentUsers(t, db)
	owner := patentActor{ID: seed.Owner}
	patent := createPatent(t, db, seed, owner)
	handler := Patents(db.Pool)

	body := map[string]any{
		"number": "CN-2024-001", "title": "标题", "deadlineSource": "年费通知", "reminderDays": 90,
		"revision": 1, "responsibleId": seed.Outsider,
	}
	// 负责人自己也不能把负责人转给别人。
	recorder := postJSON(t, handler, owner, http.MethodPut, "/api/patents/"+patent, body)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("非管理员调整负责人应返回 403，实际 %d %s", recorder.Code, recorder.Body.String())
	}
	// 管理员可以调整。
	recorder = postJSON(t, handler, patentActor{ID: seed.Admin, Roles: []string{"admin"}}, http.MethodPut, "/api/patents/"+patent, body)
	if recorder.Code != http.StatusOK {
		t.Fatalf("管理员应可调整负责人，实际 %d %s", recorder.Code, recorder.Body.String())
	}
	responsible := db.ScanString(t, `SELECT responsible_id::text FROM patent_records WHERE id=$1::uuid`, patent)
	if responsible != seed.Outsider {
		t.Fatalf("负责人未更新，实际 %s", responsible)
	}
}

// TestPatentsUpdateRejectsMissingOwner 负责人必须是启用中的用户。
func TestPatentsUpdateRejectsMissingOwner(t *testing.T) {
	db := dbtest.New(t)
	seed := seedPatentUsers(t, db)
	owner := patentActor{ID: seed.Owner}
	patent := createPatent(t, db, seed, owner)
	handler := Patents(db.Pool)

	body := map[string]any{
		"number": "CN-2024-001", "title": "标题", "deadlineSource": "年费通知", "reminderDays": 90,
		"revision": 1, "responsibleId": "00000000-0000-0000-0000-000000000000",
	}
	recorder := postJSON(t, handler, patentActor{ID: seed.Admin, Roles: []string{"admin"}}, http.MethodPut, "/api/patents/"+patent, body)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("不存在的负责人应返回 400，实际 %d %s", recorder.Code, recorder.Body.String())
	}
}

// TestPatentsUpdateRejectsStaleRevision 乐观锁：版本不匹配时拒绝，防止覆盖他人修改。
func TestPatentsUpdateRejectsStaleRevision(t *testing.T) {
	db := dbtest.New(t)
	seed := seedPatentUsers(t, db)
	owner := patentActor{ID: seed.Owner}
	patent := createPatent(t, db, seed, owner)
	handler := Patents(db.Pool)

	body := map[string]any{
		"number": "CN-2024-001", "title": "第一次修改", "deadlineSource": "年费通知", "reminderDays": 90, "revision": 1,
	}
	recorder := postJSON(t, handler, owner, http.MethodPut, "/api/patents/"+patent, body)
	if recorder.Code != http.StatusOK {
		t.Fatalf("首次修改应成功，实际 %d %s", recorder.Code, recorder.Body.String())
	}
	// 使用过期版本再次提交必须冲突。
	body["title"] = "第二次修改"
	recorder = postJSON(t, handler, owner, http.MethodPut, "/api/patents/"+patent, body)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("版本过期应返回 409，实际 %d %s", recorder.Code, recorder.Body.String())
	}
	title := db.ScanString(t, `SELECT title FROM patent_records WHERE id=$1::uuid`, patent)
	if title != "第一次修改" {
		t.Fatalf("冲突请求不应生效，实际 %s", title)
	}
}

// TestPatentsDeleteIsRejected 专利记录不允许删除。
func TestPatentsDeleteIsRejected(t *testing.T) {
	db := dbtest.New(t)
	seed := seedPatentUsers(t, db)
	owner := patentActor{ID: seed.Owner}
	patent := createPatent(t, db, seed, owner)
	handler := Patents(db.Pool)

	recorder := postJSON(t, handler, owner, http.MethodDelete, "/api/patents/"+patent, nil)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("删除应返回 405，实际 %d", recorder.Code)
	}
	count := db.ScanString(t, `SELECT count(*)::text FROM patent_records WHERE id=$1::uuid`, patent)
	if count != "1" {
		t.Fatal("专利记录不得被删除")
	}
}

// TestPatentEventsRequireLogin 事件查询需要登录。
func TestPatentEventsRequireLogin(t *testing.T) {
	db := dbtest.New(t)
	handler := Patents(db.Pool)
	request := httptest.NewRequest(http.MethodGet, "/api/patents/00000000-0000-0000-0000-000000000000/events", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("未登录应返回 401，实际 %d", recorder.Code)
	}
}

// TestPatentEventsRecordBeforeAndAfter 每次变更都要留下前后对比，便于追溯。
func TestPatentEventsRecordBeforeAndAfter(t *testing.T) {
	db := dbtest.New(t)
	seed := seedPatentUsers(t, db)
	owner := patentActor{ID: seed.Owner}
	patent := createPatent(t, db, seed, owner)
	handler := Patents(db.Pool)

	body := map[string]any{
		"number": "CN-2024-001", "title": "修改后标题", "deadlineSource": "年费通知", "reminderDays": 60, "revision": 1,
	}
	if recorder := postJSON(t, handler, owner, http.MethodPut, "/api/patents/"+patent, body); recorder.Code != http.StatusOK {
		t.Fatalf("修改失败: %s", recorder.Body.String())
	}

	detail := db.ScanString(t, `SELECT detail::text FROM patent_events WHERE patent_id=$1::uuid AND action='update'`, patent)
	// 事件必须同时保留修改前内容与本次提交，否则无法审计。
	if !strings.Contains(detail, "before") || !strings.Contains(detail, "submitted") {
		t.Fatalf("事件应记录前后对比: %s", detail)
	}
	if !strings.Contains(detail, "修改后标题") {
		t.Fatalf("事件应包含新值: %s", detail)
	}
}

// TestPatentPaymentValidation 缴费登记是最容易出错的一条路径，逐项验证。
func TestPatentPaymentValidation(t *testing.T) {
	db := dbtest.New(t)
	seed := seedPatentUsers(t, db)
	owner := patentActor{ID: seed.Owner}
	patent := createPatent(t, db, seed, owner)
	handler := Patents(db.Pool)
	// 设置本期缴费期限，后续校验“下一期必须晚于本期”。
	db.Exec(t, `UPDATE patent_records SET fee_due='2026-01-01' WHERE id=$1::uuid`, patent)

	// 缴费同样受乐观锁保护，必须带上当前 revision。
	revision := 1
	fmt.Sscanf(db.ScanString(t, `SELECT revision::text FROM patent_records WHERE id=$1::uuid`, patent), "%d", &revision)
	base := map[string]any{
		"feeDue":         "2027-01-01",
		"deadlineSource": "年费通知",
		"paidOn":         "2025-12-01",
		"amount":         "900.00 CNY",
		"revision":       revision,
	}
	withExtra := func(extra map[string]any) map[string]any {
		body := map[string]any{}
		for key, value := range base {
			body[key] = value
		}
		for key, value := range extra {
			body[key] = value
		}
		return body
	}

	// 缺少凭证。
	if recorder := postJSON(t, handler, owner, http.MethodPost, "/api/patents/"+patent+"/payment", withExtra(nil)); recorder.Code != http.StatusBadRequest {
		t.Fatalf("缺少凭证应返回 400，实际 %d %s", recorder.Code, recorder.Body.String())
	}
	// 凭证不存在或不属于本专利。
	if recorder := postJSON(t, handler, owner, http.MethodPost, "/api/patents/"+patent+"/payment", withExtra(map[string]any{"receiptId": "00000000-0000-0000-0000-000000000000"})); recorder.Code != http.StatusBadRequest {
		t.Fatalf("凭证不属于本专利应返回 400，实际 %d %s", recorder.Code, recorder.Body.String())
	}
	// 金额格式错误。
	receipt := uploadLifecycleDocument(t, db, seed, owner, "专利证书", "缴费凭证")
	if recorder := postJSON(t, handler, owner, http.MethodPost, "/api/patents/"+patent+"/payment", withExtra(map[string]any{"receiptId": receipt, "amount": "900"})); recorder.Code != http.StatusBadRequest {
		t.Fatalf("金额缺少币种应返回 400，实际 %d %s", recorder.Code, recorder.Body.String())
	}
	// 实际缴费日期不能在未来。
	future := withExtra(map[string]any{"receiptId": receipt, "amount": "900.00 CNY", "paidOn": "2099-01-01"})
	if recorder := postJSON(t, handler, owner, http.MethodPost, "/api/patents/"+patent+"/payment", future); recorder.Code != http.StatusBadRequest {
		t.Fatalf("未来缴费日期应返回 400，实际 %d %s", recorder.Code, recorder.Body.String())
	}
	// 下一缴费期限必须晚于本期。
	earlier := withExtra(map[string]any{"receiptId": receipt, "feeDue": "2025-06-01"})
	if recorder := postJSON(t, handler, owner, http.MethodPost, "/api/patents/"+patent+"/payment", earlier); recorder.Code != http.StatusBadRequest {
		t.Fatalf("下一期早于本期应返回 400，实际 %d %s", recorder.Code, recorder.Body.String())
	}

	// 合法缴费必须成功并推进期限。
	valid := withExtra(map[string]any{"receiptId": receipt})
	if recorder := postJSON(t, handler, owner, http.MethodPost, "/api/patents/"+patent+"/payment", valid); recorder.Code != http.StatusOK {
		t.Fatalf("合法缴费应成功，实际 %d %s", recorder.Code, recorder.Body.String())
	}
	feeDue := db.ScanString(t, `SELECT fee_due::text FROM patent_records WHERE id=$1::uuid`, patent)
	if !strings.HasPrefix(feeDue, "2027-01-01") {
		t.Fatalf("缴费后期限应推进到 2027-01-01，实际 %s", feeDue)
	}
	// 同一凭证不得重复登记。首次缴费已递增 revision，因此必须用新 revision 才能
	// 越过乐观锁、真正命中凭证唯一性校验。
	nextRevision := 1
	fmt.Sscanf(db.ScanString(t, `SELECT revision::text FROM patent_records WHERE id=$1::uuid`, patent), "%d", &nextRevision)
	repeat := withExtra(map[string]any{"receiptId": receipt, "revision": nextRevision})
	if recorder := postJSON(t, handler, owner, http.MethodPost, "/api/patents/"+patent+"/payment", repeat); recorder.Code != http.StatusBadRequest {
		t.Fatalf("重复登记应返回 400，实际 %d %s", recorder.Code, recorder.Body.String())
	}
	// 重复登记不得再次推进期限。
	feeDue = db.ScanString(t, `SELECT fee_due::text FROM patent_records WHERE id=$1::uuid`, patent)
	if !strings.HasPrefix(feeDue, "2027-01-01") {
		t.Fatalf("重复登记不应改变期限，实际 %s", feeDue)
	}
}

// TestPatentPaymentRequiresOwnership 非负责人不得登记缴费。
func TestPatentPaymentRequiresOwnership(t *testing.T) {
	db := dbtest.New(t)
	seed := seedPatentUsers(t, db)
	owner := patentActor{ID: seed.Owner}
	patent := createPatent(t, db, seed, owner)
	handler := Patents(db.Pool)
	db.Exec(t, `UPDATE patent_records SET fee_due='2026-01-01' WHERE id=$1::uuid`, patent)
	receipt := uploadLifecycleDocument(t, db, seed, owner, "专利证书", "缴费凭证")

	// 缴费受乐观锁保护，必须传当前 revision；并发场景下传同一个 revision 才能形成真实竞争。
	revision := 1
	fmt.Sscanf(db.ScanString(t, `SELECT revision::text FROM patent_records WHERE id=$1::uuid`, patent), "%d", &revision)
	body := map[string]any{"receiptId": receipt, "feeDue": "2027-01-01", "deadlineSource": "年费通知", "paidOn": "2025-12-01", "amount": "900.00 CNY", "revision": revision}
	recorder := postJSON(t, handler, patentActor{ID: seed.Outsider}, http.MethodPost, "/api/patents/"+patent+"/payment", body)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("非负责人应返回 403，实际 %d %s", recorder.Code, recorder.Body.String())
	}
}

// TestPatentPaymentConcurrentOnlyOneSucceeds 并发登记同一凭证只允许一次成功。
//
// 重复登记会让期限被推进两次并产生虚假缴费记录。
func TestPatentPaymentConcurrentOnlyOneSucceeds(t *testing.T) {
	db := dbtest.New(t)
	seed := seedPatentUsers(t, db)
	owner := patentActor{ID: seed.Owner}
	patent := createPatent(t, db, seed, owner)
	handler := Patents(db.Pool)
	db.Exec(t, `UPDATE patent_records SET fee_due='2026-01-01' WHERE id=$1::uuid`, patent)
	receipt := uploadLifecycleDocument(t, db, seed, owner, "专利证书", "缴费凭证")

	// 缴费受乐观锁保护，必须传当前 revision；并发场景下传同一个 revision 才能形成真实竞争。
	revision := 1
	fmt.Sscanf(db.ScanString(t, `SELECT revision::text FROM patent_records WHERE id=$1::uuid`, patent), "%d", &revision)
	body := map[string]any{"receiptId": receipt, "feeDue": "2027-01-01", "deadlineSource": "年费通知", "paidOn": "2025-12-01", "amount": "900.00 CNY", "revision": revision}
	const workers = 4
	var wg sync.WaitGroup
	codes := make([]int, workers)
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			codes[index] = postJSON(t, handler, owner, http.MethodPost, "/api/patents/"+patent+"/payment", body).Code
		}(i)
	}
	close(start)
	wg.Wait()

	succeeded := 0
	for _, code := range codes {
		if code == http.StatusOK {
			succeeded++
		}
	}
	if succeeded != 1 {
		t.Fatalf("并发缴费应只有一次成功，实际 %d 次（状态码 %v）", succeeded, codes)
	}
	// 只允许一条缴费事件，且期限只推进一次。
	payments := db.ScanString(t, `SELECT count(*)::text FROM patent_events WHERE patent_id=$1::uuid AND action='payment'`, patent)
	if payments != "1" {
		t.Fatalf("应只记录 1 条缴费事件，实际 %s", payments)
	}
}

// TestPatentsListIncludesDayCounts 列表必须返回剩余天数，供前端计算紧迫度。
func TestPatentsListIncludesDayCounts(t *testing.T) {
	db := dbtest.New(t)
	seed := seedPatentUsers(t, db)
	owner := patentActor{ID: seed.Owner}
	patent := createPatent(t, db, seed, owner)
	// 设置一个未来期限，剩余天数应为正。
	db.Exec(t, `UPDATE patent_records SET expires_on=(now() AT TIME ZONE 'Asia/Shanghai')::date+30 WHERE id=$1::uuid`, patent)

	handler := Patents(db.Pool)
	request := httptest.NewRequest(http.MethodGet, "/api/patents", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, withActor(request, owner))
	if recorder.Code != http.StatusOK {
		t.Fatalf("列表应返回 200，实际 %d %s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) != 1 {
		t.Fatalf("应返回 1 条记录，实际 %d", len(body.Data))
	}
	days, ok := body.Data[0]["expiryDays"].(float64)
	if !ok {
		t.Fatalf("应返回 expiryDays 字段: %v", body.Data[0])
	}
	if days != 30 {
		t.Fatalf("剩余天数应为 30，实际 %v", days)
	}
}

// TestPatentsUnknownPathReturnsNotFound 未知子路径必须 404，避免被当作专利 ID 处理。
func TestPatentsUnknownPathReturnsNotFound(t *testing.T) {
	db := dbtest.New(t)
	seed := seedPatentUsers(t, db)
	handler := Patents(db.Pool)
	recorder := postJSON(t, handler, patentActor{ID: seed.Owner}, http.MethodPost, "/api/patents/abc/unknown", map[string]any{})
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("未知子路径应返回 404，实际 %d %s", recorder.Code, recorder.Body.String())
	}
}

// newTestStorage 为测试提供隔离的本地对象存储。
func newTestStorage(t *testing.T) storage.ObjectStorage {
	t.Helper()
	objects, err := storage.NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatalf("创建测试存储失败: %v", err)
	}
	return objects
}

// putObject 写入一个测试对象。
func putObject(t *testing.T, objects storage.ObjectStorage, key string, content []byte) error {
	t.Helper()
	_, err := objects.Put(context.Background(), key, bytes.NewReader(content), "application/octet-stream")
	return err
}

// uploadLifecycleDocument 通过 lifecycle 资料接口上传一份凭证，返回资料 id。
func uploadLifecycleDocument(t *testing.T, db *dbtest.DB, seed patentSeed, actor patentActor, category, title string) string {
	t.Helper()
	// 专利资料需要 patentId，使用当前唯一专利。
	patentID := db.ScanString(t, `SELECT id::text FROM patent_records ORDER BY created_at LIMIT 1`)
	return uploadDocumentForPatent(t, db, actor, patentID, category, title)
}

// uploadDocumentForPatent 上传归属于指定专利的资料。
func uploadDocumentForPatent(t *testing.T, db *dbtest.DB, actor patentActor, patentID, category, title string) string {
	t.Helper()
	objects := newTestStorage(t)
	handler := LifecycleDocuments(db.Pool, objects)

	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	for key, value := range map[string]string{
		"patentId": patentID, "category": category, "title": title,
	} {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s.pdf"`, title))
	header.Set("Content-Type", "application/pdf")
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = part.Write([]byte("%PDF-1.4 凭证内容")); err != nil {
		t.Fatal(err)
	}
	writer.Close()

	request := httptest.NewRequest(http.MethodPost, "/api/lifecycle-documents", &buffer)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, withActor(request, actor))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("上传资料失败: %d %s", recorder.Code, recorder.Body.String())
	}
	return decodeData(t, recorder)["id"].(string)
}
