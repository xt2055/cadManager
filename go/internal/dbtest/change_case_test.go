package dbtest

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"cadguanliq/internal/review"
	"github.com/jackc/pgx/v5"
)

// 本文件验证 StartChangeCase：变更进入完整审核时冻结节点快照，并与首次审核一致，
// 驳回后重新提交保留已通过节点、从驳回节点继续。

// addFlowNode 向流程添加一个节点。
func addFlowNode(t *testing.T, db *DB, flow, name, signerRole string, assignedUser string, required bool, order int) {
	t.Helper()
	db.Exec(t, `INSERT INTO review_flow_nodes(flow_id,name,signer_role,assigned_user_id,assigned_name,required,node_order)
		VALUES($1::uuid,$2,$3,NULLIF($4,'')::uuid,COALESCE((SELECT display_name FROM users WHERE id=NULLIF($4,'')::uuid),'待定'),$5,$6)`,
		flow, name, signerRole, assignedUser, required, order)
}

// startCase 在独立事务中调用 StartChangeCase。
func startCase(t *testing.T, db *DB, drawingID, submissionID, designerID string, commit bool) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("开启事务失败: %v", err)
	}
	defer tx.Rollback(ctx)
	if err = review.StartChangeCase(ctx, tx, drawingID, submissionID, designerID); err != nil {
		return err
	}
	if !commit {
		return nil
	}
	return tx.Commit(ctx)
}

// changeCaseFixture 准备审核流程与一个待提交的变更轮次。
type changeCaseFixture struct {
	Fixture
	Flow       string
	Request    string
	Submission string
}

func setupChangeCase(t *testing.T, db *DB) changeCaseFixture {
	t.Helper()
	fixture := db.Seed(t)
	flow := insertFlow(t, db, fixture)
	request := insertChangeRequest(t, db, fixture, "CR-CASE")
	submission := insertSubmission(t, db, request, 1)
	db.Exec(t, `UPDATE change_requests SET current_submission_id=$2::uuid WHERE id=$1::uuid`, request, submission)
	return changeCaseFixture{Fixture: fixture, Flow: flow, Request: request, Submission: submission}
}

// TestStartChangeCaseFreezesNodes 冻结流程节点：责任人、顺序与角色必须快照到审核单。
func TestStartChangeCaseFreezesNodes(t *testing.T) {
	db := New(t)
	fx := setupChangeCase(t, db)
	addFlowNode(t, db, fx.Flow, "校对复核", "校对", fx.Reviewer, true, 1)
	addFlowNode(t, db, fx.Flow, "专业审核", "审核", fx.Reviewer, true, 2)

	if err := startCase(t, db, fx.Drawing, fx.Submission, fx.Author, true); err != nil {
		t.Fatalf("冻结审核节点失败: %v", err)
	}

	caseID := db.ScanString(t, `SELECT id::text FROM review_cases WHERE change_submission_id=$1::uuid`, fx.Submission)
	// 节点必须完整复制，且保持流程定义顺序。
	nodes := db.ScanString(t, `SELECT string_agg(name||':'||status,'|' ORDER BY node_order) FROM review_case_nodes WHERE review_case_id=$1::uuid`, caseID)
	if nodes != "校对复核:pending|专业审核:pending" {
		t.Fatalf("节点快照不正确: %s", nodes)
	}
	roles := db.ScanString(t, `SELECT string_agg(signer_role,',' ORDER BY node_order) FROM review_case_nodes WHERE review_case_id=$1::uuid`, caseID)
	if roles != "校对,审核" {
		t.Fatalf("签署角色快照不正确: %s", roles)
	}
	// 流程名必须快照，之后改流程名不影响历史审核单。
	flowName := db.ScanString(t, `SELECT flow_name_snapshot FROM review_cases WHERE id=$1::uuid`, caseID)
	if flowName != "完整审核" {
		t.Fatalf("流程名快照不正确: %s", flowName)
	}
	// 发起动作要留痕，供审计追溯。
	actions := db.ScanString(t, `SELECT count(*)::text FROM review_actions WHERE review_case_id=$1::uuid AND action='start'`, caseID)
	if actions != "1" {
		t.Fatalf("应写入 1 条发起动作，实际 %s", actions)
	}
}

// TestStartChangeCaseAssignsDesignerToDesignNode 设计节点责任人必须替换为当前设计员。
//
// 流程定义里设计节点的负责人是固定配置，变更必须由本次提交的设计员签署自检。
func TestStartChangeCaseAssignsDesignerToDesignNode(t *testing.T) {
	db := New(t)
	fx := setupChangeCase(t, db)
	// 流程里设计节点指向另一个用户，验证会被替换为当前设计员。
	addFlowNode(t, db, fx.Flow, "设计自检", "", fx.Reviewer, true, 1)
	addFlowNode(t, db, fx.Flow, "校对复核", "校对", fx.Reviewer, true, 2)

	if err := startCase(t, db, fx.Drawing, fx.Submission, fx.Author, true); err != nil {
		t.Fatalf("冻结审核节点失败: %v", err)
	}
	caseID := db.ScanString(t, `SELECT id::text FROM review_cases WHERE change_submission_id=$1::uuid`, fx.Submission)

	assigned := db.ScanString(t, `SELECT assigned_user_id::text FROM review_case_nodes WHERE review_case_id=$1::uuid AND name='设计自检'`, caseID)
	if assigned != fx.Author {
		t.Fatalf("设计节点应指派给当前设计员 %s，实际 %s", fx.Author, assigned)
	}
	// 责任人显示名按当前用户资料刷新，而不是沿用流程里的固定配置。
	name := db.ScanString(t, `SELECT assigned_name FROM review_case_nodes WHERE review_case_id=$1::uuid AND name='设计自检'`, caseID)
	if name != "设计员" {
		t.Fatalf("设计节点责任人应指向当前设计员，实际 %s", name)
	}
	// 角色为空时按节点名推断。
	role := db.ScanString(t, `SELECT signer_role FROM review_case_nodes WHERE review_case_id=$1::uuid AND name='设计自检'`, caseID)
	if role != "设计" {
		t.Fatalf("设计节点角色应推断为设计，实际 %s", role)
	}
}

// TestStartChangeCaseRejectsRequiredNodeWithoutOwner 必填节点没有责任人时必须拒绝发起。
func TestStartChangeCaseRejectsRequiredNodeWithoutOwner(t *testing.T) {
	db := New(t)
	fx := setupChangeCase(t, db)
	addFlowNode(t, db, fx.Flow, "校对复核", "校对", fx.Reviewer, false, 1)
	addFlowNode(t, db, fx.Flow, "工艺会签", "工艺", "", true, 2)

	err := startCase(t, db, fx.Drawing, fx.Submission, fx.Author, true)
	if err == nil || !strings.Contains(err.Error(), "工艺会签") {
		t.Fatalf("应拒绝没有责任人的必填节点并指明节点名，实际 %v", err)
	}
}

// TestStartChangeCaseSkipsOptionalNodeWithoutOwner 非必填且无责任人的节点不进入审核单。
func TestStartChangeCaseSkipsOptionalNodeWithoutOwner(t *testing.T) {
	db := New(t)
	fx := setupChangeCase(t, db)
	addFlowNode(t, db, fx.Flow, "校对复核", "校对", fx.Reviewer, true, 1)
	addFlowNode(t, db, fx.Flow, "工艺会签", "工艺", "", false, 2)

	if err := startCase(t, db, fx.Drawing, fx.Submission, fx.Author, true); err != nil {
		t.Fatalf("非必填节点缺失不应阻止发起: %v", err)
	}
	caseID := db.ScanString(t, `SELECT id::text FROM review_cases WHERE change_submission_id=$1::uuid`, fx.Submission)
	count := db.ScanString(t, `SELECT count(*)::text FROM review_case_nodes WHERE review_case_id=$1::uuid`, caseID)
	if count != "1" {
		t.Fatalf("应只保留 1 个可执行节点，实际 %s", count)
	}
	exists := db.ScanString(t, `SELECT count(*)::text FROM review_case_nodes WHERE review_case_id=$1::uuid AND name='工艺会签'`, caseID)
	if exists != "0" {
		t.Fatal("无责任人的非必填节点不应写入审核单")
	}
}

// TestStartChangeCaseRequiresEnabledFlow 没有启用的审核流程时无法发起完整审核。
func TestStartChangeCaseRequiresEnabledFlow(t *testing.T) {
	db := New(t)
	fx := setupChangeCase(t, db)
	// 流程存在但未启用。
	db.Exec(t, `UPDATE review_flows SET enabled=false WHERE id=$1::uuid`, fx.Flow)

	err := startCase(t, db, fx.Drawing, fx.Submission, fx.Author, true)
	if err == nil || !strings.Contains(err.Error(), "完整审核流程") {
		t.Fatalf("应提示缺少启用的完整审核流程，实际 %v", err)
	}
}

// TestStartChangeCasePicksLatestEnabledFlow 多个启用流程时使用最近更新的那个。
func TestStartChangeCasePicksLatestEnabledFlow(t *testing.T) {
	db := New(t)
	fx := setupChangeCase(t, db)
	addFlowNode(t, db, fx.Flow, "旧流程节点", "校对", fx.Reviewer, true, 1)
	// 新建并启用另一个流程，更新时间更晚。
	newFlow := insertFlowNamed(t, db, fx.Fixture, "新审核流程")
	addFlowNode(t, db, newFlow, "新流程节点", "审核", fx.Reviewer, true, 1)

	if err := startCase(t, db, fx.Drawing, fx.Submission, fx.Author, true); err != nil {
		t.Fatalf("发起审核失败: %v", err)
	}
	caseID := db.ScanString(t, `SELECT id::text FROM review_cases WHERE change_submission_id=$1::uuid`, fx.Submission)
	name := db.ScanString(t, `SELECT flow_name_snapshot FROM review_cases WHERE id=$1::uuid`, caseID)
	if name != "新审核流程" {
		t.Fatalf("应使用最近更新的启用流程，实际 %s", name)
	}
}

// TestStartChangeCaseRejectsInactiveAssignee 责任人不处于启用状态时必须拒绝。
func TestStartChangeCaseRejectsInactiveAssignee(t *testing.T) {
	db := New(t)
	fx := setupChangeCase(t, db)
	addFlowNode(t, db, fx.Flow, "校对复核", "校对", fx.Reviewer, true, 1)
	db.Exec(t, `UPDATE users SET status='disabled' WHERE id=$1::uuid`, fx.Reviewer)

	err := startCase(t, db, fx.Drawing, fx.Submission, fx.Author, true)
	if err == nil || !strings.Contains(err.Error(), "责任人不可用") {
		t.Fatalf("应拒绝已停用责任人，实际 %v", err)
	}
}

// TestStartChangeCaseRejectsNodeWithoutExecutable 流程节点全部无责任人时不得建立空审核单。
func TestStartChangeCaseRejectsNodeWithoutExecutable(t *testing.T) {
	db := New(t)
	fx := setupChangeCase(t, db)
	addFlowNode(t, db, fx.Flow, "工艺会签", "工艺", "", false, 1)

	err := startCase(t, db, fx.Drawing, fx.Submission, fx.Author, true)
	if err == nil || !strings.Contains(err.Error(), "没有可执行节点") {
		t.Fatalf("应拒绝建立空审核单，实际 %v", err)
	}
}

// TestStartChangeCaseResumesFromRejectedNode 变更驳回后重新提交，保留已通过节点，
// 从驳回节点继续，与首次审核流程保持一致。
//
// 已通过节点的签署记录延续到新轮次；被驳回及其后的节点必须重新签署，
// 旧轮次的记录原样保留作为历史证据。
func TestStartChangeCaseResumesFromRejectedNode(t *testing.T) {
	db := New(t)
	fx := setupChangeCase(t, db)
	addFlowNode(t, db, fx.Flow, "校对复核", "校对", fx.Reviewer, true, 1)
	addFlowNode(t, db, fx.Flow, "专业审核", "审核", fx.Reviewer, true, 2)

	// 第一轮：节点 1 通过，节点 2 驳回。
	if err := startCase(t, db, fx.Drawing, fx.Submission, fx.Author, true); err != nil {
		t.Fatalf("第一轮发起失败: %v", err)
	}
	firstCase := db.ScanString(t, `SELECT id::text FROM review_cases WHERE change_submission_id=$1::uuid`, fx.Submission)
	db.Exec(t, `UPDATE review_case_nodes SET status='pass',opinion='第一轮同意',reviewed_at=now() WHERE review_case_id=$1::uuid AND node_order=1`, firstCase)
	db.Exec(t, `UPDATE review_case_nodes SET status='rejected',opinion='第一轮驳回',reviewed_at=now() WHERE review_case_id=$1::uuid AND node_order=2`, firstCase)
	db.Exec(t, `UPDATE review_cases SET status='rejected',completed_at=now() WHERE id=$1::uuid`, firstCase)

	// 第二轮：退回后重新提交，产生新的提交轮次与审核单。
	second := insertSubmission(t, db, fx.Request, 2)
	db.Exec(t, `UPDATE change_requests SET current_submission_id=$2::uuid WHERE id=$1::uuid`, fx.Request, second)
	if err := startCase(t, db, fx.Drawing, second, fx.Author, true); err != nil {
		t.Fatalf("第二轮发起失败: %v", err)
	}
	secondCase := db.ScanString(t, `SELECT id::text FROM review_cases WHERE change_submission_id=$1::uuid`, second)
	if secondCase == firstCase {
		t.Fatal("第二轮必须建立新的审核单，不能复用旧审核单")
	}
	// 已通过节点保留签署记录。
	passStatus := db.ScanString(t, `SELECT status FROM review_case_nodes WHERE review_case_id=$1::uuid AND node_order=1`, secondCase)
	if passStatus != "pass" {
		t.Fatalf("已通过节点必须延续，实际 %s", passStatus)
	}
	passOpinion := db.ScanString(t, `SELECT opinion FROM review_case_nodes WHERE review_case_id=$1::uuid AND node_order=1`, secondCase)
	if passOpinion != "第一轮同意" {
		t.Fatalf("已通过节点应保留签署意见，实际 %q", passOpinion)
	}
	// 被驳回节点重新待签。
	rejectedStatus := db.ScanString(t, `SELECT status FROM review_case_nodes WHERE review_case_id=$1::uuid AND node_order=2`, secondCase)
	if rejectedStatus != "pending" {
		t.Fatalf("驳回节点必须重新待签，实际 %s", rejectedStatus)
	}
	// 上一轮的签名记录必须原样保留，作为历史证据。
	oldStatus := db.ScanString(t, `SELECT string_agg(status,',' ORDER BY node_order) FROM review_case_nodes WHERE review_case_id=$1::uuid`, firstCase)
	if oldStatus != "pass,rejected" {
		t.Fatalf("历史审核单签名不应被改写，实际 %s", oldStatus)
	}
}

// TestStartChangeCaseConcurrentOnlyOneCasePerSubmission 并发发起时同一轮次只能建立一个审核单。
func TestStartChangeCaseConcurrentOnlyOneCasePerSubmission(t *testing.T) {
	db := New(t)
	fx := setupChangeCase(t, db)
	addFlowNode(t, db, fx.Flow, "校对复核", "校对", fx.Reviewer, true, 1)

	const workers = 4
	var wg sync.WaitGroup
	results := make([]error, workers)
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			results[index] = startCase(t, db, fx.Drawing, fx.Submission, fx.Author, true)
		}(i)
	}
	close(start)
	wg.Wait()

	succeeded := 0
	for _, err := range results {
		if err == nil {
			succeeded++
		}
	}
	if succeeded != 1 {
		t.Fatalf("并发发起应只有一个成功，实际 %d 个（错误: %v）", succeeded, results)
	}
	// 唯一索引必须挡住重复审核单，否则同一轮次会被发布多次。
	cases := db.ScanString(t, `SELECT count(*)::text FROM review_cases WHERE change_submission_id=$1::uuid`, fx.Submission)
	if cases != "1" {
		t.Fatalf("同一轮次应只有 1 个审核单，实际 %s", cases)
	}
}

// TestStartChangeCaseRollsBackOnFailure 发起失败时不得留下部分数据。
//
// 这类失败最危险的是“审核单已建但节点没建全”：审核单占用提交轮次的唯一索引，
// 留下半成品会永久堵死该轮次的重新发起。
//
// 流程节点表对 name 和 node_order 都有唯一约束，与审核节点表一一对应，因此正常
// 数据无法构造节点插入失败。这里用一个临时触发器强制节点写入失败，模拟该阶段
// 出现异常（如磁盘或约束变更），验证审核单不会残留。
func TestStartChangeCaseRollsBackOnFailure(t *testing.T) {
	db := New(t)
	fx := setupChangeCase(t, db)
	addFlowNode(t, db, fx.Flow, "校对复核", "校对", fx.Reviewer, true, 1)
	db.Exec(t, `CREATE FUNCTION reject_node_insert() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION '模拟节点写入失败'; END; $$`)
	db.Exec(t, `CREATE TRIGGER test_reject_node BEFORE INSERT ON review_case_nodes FOR EACH ROW EXECUTE FUNCTION reject_node_insert()`)

	err := startCase(t, db, fx.Drawing, fx.Submission, fx.Author, true)
	if err == nil {
		t.Fatal("节点写入失败应导致整体回滚")
	}
	// 失败后不得留下审核单，否则该轮次再也无法发起（唯一索引占用）。
	cases := db.ScanString(t, `SELECT count(*)::text FROM review_cases WHERE change_submission_id=$1::uuid`, fx.Submission)
	if cases != "0" {
		t.Fatalf("失败后不应留下审核单，实际 %s", cases)
	}

	// 移除故障注入后，该轮次必须仍能正常发起。
	db.Exec(t, `DROP TRIGGER test_reject_node ON review_case_nodes`)
	if err = startCase(t, db, fx.Drawing, fx.Submission, fx.Author, true); err != nil {
		t.Fatalf("回滚后应能重新发起: %v", err)
	}
}

// insertFlowNamed 建立一个指定名称的启用流程，并保证其 updated_at 晚于已有流程。
func insertFlowNamed(t *testing.T, db *DB, fixture Fixture, name string) string {
	t.Helper()
	// 显式设置 updated_at，避免同毫秒创建导致排序不确定。
	var id string
	if err := db.Pool.QueryRow(context.Background(), `INSERT INTO review_flows(name,enabled,created_by,updated_at)
		VALUES($1,true,$2::uuid,now()+interval '1 second') RETURNING id::text`, name, fixture.Author).Scan(&id); err != nil {
		t.Fatalf("插入审核流程失败: %v", err)
	}
	return id
}

// TestStartChangeCaseRequiresSubmissionBinding 审核单必须绑定提交轮次，否则无法与发布关联。
func TestStartChangeCaseRequiresSubmissionBinding(t *testing.T) {
	db := New(t)
	fx := setupChangeCase(t, db)
	addFlowNode(t, db, fx.Flow, "校对复核", "校对", fx.Reviewer, true, 1)
	if err := startCase(t, db, fx.Drawing, fx.Submission, fx.Author, true); err != nil {
		t.Fatalf("发起审核失败: %v", err)
	}
	bound := db.ScanString(t, `SELECT change_submission_id::text FROM review_cases WHERE change_submission_id=$1::uuid`, fx.Submission)
	if bound != fx.Submission {
		t.Fatalf("审核单必须绑定提交轮次，实际 %s", bound)
	}
}

// TestReviewCaseSubmissionBindingIsNotProtected documents a defense-in-depth gap.
//
// review_cases 上没有不可变性触发器，因此 change_submission_id 可以被置空；
// 置空后同一提交轮次能再建一个审核单，绕过 review_cases_change_submission_uq
// 的唯一保护。当前应用代码没有任何路径这样做（只有查询和按轮次改状态），
// 所以这是纵深防御缺口而非可利用漏洞。本用例固定该行为，一旦补上约束即会失败，
// 提醒同步更新。
func TestReviewCaseSubmissionBindingIsNotProtected(t *testing.T) {
	db := New(t)
	fx := setupChangeCase(t, db)
	addFlowNode(t, db, fx.Flow, "校对复核", "校对", fx.Reviewer, true, 1)
	if err := startCase(t, db, fx.Drawing, fx.Submission, fx.Author, true); err != nil {
		t.Fatalf("发起审核失败: %v", err)
	}
	caseID := db.ScanString(t, `SELECT id::text FROM review_cases WHERE change_submission_id=$1::uuid`, fx.Submission)

	// 当前未受保护：置空成功。
	db.Exec(t, `UPDATE review_cases SET change_submission_id=NULL WHERE id=$1::uuid`, caseID)
	// 第一条审核单置为非活跃状态，解除 uq_review_cases_active_drawing 的占用。
	db.Exec(t, `UPDATE review_cases SET status='published' WHERE id=$1::uuid`, caseID)
	// 置空后同一轮次可以再建审核单，change_submission_id 的唯一索引不再拦截。
	db.Exec(t, `INSERT INTO review_cases(drawing_id,flow_id,status,initiator_id,change_submission_id,flow_name_snapshot)
		VALUES($1::uuid,$2::uuid,'reviewing',$3::uuid,$4::uuid,'完整审核')`, fx.Drawing, fx.Flow, fx.Author, fx.Submission)

	total := db.ScanString(t, `SELECT count(*)::text FROM review_cases WHERE change_submission_id=$1::uuid`, fx.Submission)
	if total != "1" {
		t.Fatalf("同一轮次应能被重新绑定一次，实际 %s 条", total)
	}
	total = db.ScanString(t, `SELECT count(*)::text FROM review_cases WHERE drawing_id=$1::uuid`, fx.Drawing)
	if total != "2" {
		t.Fatalf("存在两条审核单时该缺口成立，实际 %s 条", total)
	}
}

// TestStartCaseRejectsUnknownSubmission 提交轮次不存在时必须失败。
func TestStartCaseRejectsUnknownSubmission(t *testing.T) {
	db := New(t)
	fx := setupChangeCase(t, db)
	addFlowNode(t, db, fx.Flow, "校对复核", "校对", fx.Reviewer, true, 1)
	err := startCase(t, db, fx.Drawing, "00000000-0000-0000-0000-000000000000", fx.Author, true)
	if err == nil {
		t.Fatal("不存在的提交轮次必须导致失败")
	}
	if !errors.Is(err, pgx.ErrNoRows) && !strings.Contains(err.Error(), "review_cases_change_submission_uq") {
		// 外键或唯一索引拒绝均可接受；此处只要求不静默成功。
		t.Logf("失败原因: %v", err)
	}
}
