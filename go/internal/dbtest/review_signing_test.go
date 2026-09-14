package dbtest

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"cadguanliq/internal/change"
	"cadguanliq/internal/review"
)

// 本文件验证审核签署与变更发布之间的接线。
//
// 签署动作与发布必须处在同一事务：审核员点“通过”时，节点签署、案例状态、
// 工单完成与版本发布要么全部生效，要么全部不生效。这是整个变更流程最容易
// 出错的地方——签署已落库但发布失败会丢失变更成果，且工单无法重新提交。

// signFixture 是一次完整签署流程所需的全部上下文。
type signFixture struct {
	reviewFixture
	Repository *review.PGRepository
}

// setupSignedReview 建立带完整发布链路的审核流程：两个节点，签署人分别是
// 校对与审核，全部通过后由 change 服务发布本轮冻结版本。
//
// 注意不能复用 setupReview：那个场景的审核单已处于 published（供 CompleteReview
// 直接收尾），而签署要求审核单仍在 reviewing。
func setupSignedReview(t *testing.T, db *DB) signFixture {
	t.Helper()
	fixture := db.Seed(t)
	flow := insertFlow(t, db, fixture)
	// 发布要求零件已有基础修订。
	insertBaseRevision(t, db, fixture)
	request := insertChangeRequest(t, db, fixture, "CR-SIGN")
	submission := insertSubmission(t, db, request, 1)
	db.Exec(t, `UPDATE change_requests SET status='pending_verify',current_submission_id=$2::uuid WHERE id=$1::uuid`, request, submission)

	attachment, versionID := insertPartAttachment(t, db, fixture, "v1.0")
	db.Exec(t, `INSERT INTO change_request_submission_targets(submission_id,attachment_id,base_attachment_version_id,submitted_attachment_version_id)
		VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid)`, submission, attachment, versionID, versionID)

	var caseID string
	if err := db.Pool.QueryRow(context.Background(), `INSERT INTO review_cases(drawing_id,flow_id,status,initiator_id,change_submission_id,flow_name_snapshot)
		VALUES($1::uuid,$2::uuid,'reviewing',$3::uuid,$4::uuid,'完整审核') RETURNING id::text`,
		fixture.Drawing, flow, fixture.Author, submission).Scan(&caseID); err != nil {
		t.Fatalf("插入审核单失败: %v", err)
	}
	// 第一个节点已通过（由校对签署），第二个节点待签。
	db.Exec(t, `INSERT INTO review_case_nodes(review_case_id,name,assigned_user_id,assigned_name,status,opinion,required,node_order,signer_role)
		VALUES($1::uuid,'校对复核',$2::uuid,'审核员','pass','第一轮同意',true,1,'校对')`, caseID, fixture.Reviewer)
	db.Exec(t, `INSERT INTO review_case_nodes(review_case_id,name,assigned_user_id,assigned_name,status,opinion,required,node_order,signer_role)
		VALUES($1::uuid,'专业审核',$2::uuid,'审核员','pending','',true,2,'审核')`, caseID, fixture.Reviewer)

	repository := review.NewPGRepository(db.Pool)
	repository.SetChangeCompletion(change.NewService(db.Pool).CompleteReview)

	return signFixture{
		reviewFixture: reviewFixture{
			Fixture: fixture, Request: request, Submission: submission, Flow: flow, CaseID: caseID, Version: versionID,
		},
		Repository: repository,
	}
}

// signNode 以指定用户签署节点。
func signNode(t *testing.T, repository *review.PGRepository, caseID, nodeName, action, opinion, userID string) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, err := repository.SubmitNode(ctx, caseID, review.SubmitNodeInput{NodeName: nodeName, Action: action, Opinion: opinion}, userID)
	return err
}

// TestSignFinalNodePublishesChange 最后一个节点签署通过时立即发布本轮变更。
//
// 这是签署与发布的完整闭环：签署成功后工单必须已完成、版本已发布、签署人已写入。
func TestSignFinalNodePublishesChange(t *testing.T) {
	db := New(t)
	fixture := setupSignedReview(t, db)

	if err := signNode(t, fixture.Repository, fixture.CaseID, "专业审核", "pass", "同意", fixture.Reviewer); err != nil {
		t.Fatalf("签署最后一个节点失败: %v", err)
	}

	// 审核单必须已完成。
	caseStatus := db.ScanString(t, `SELECT status FROM review_cases WHERE id=$1::uuid`, fixture.CaseID)
	if caseStatus != "published" {
		t.Fatalf("审核单应为 published，实际 %s", caseStatus)
	}
	// 工单必须完成并记录验收人。
	requestStatus := db.ScanString(t, `SELECT status FROM change_requests WHERE id=$1::uuid`, fixture.Request)
	if requestStatus != "completed" {
		t.Fatalf("工单应为 completed，实际 %s", requestStatus)
	}
	verifier := db.ScanString(t, `SELECT verifier_id::text FROM change_requests WHERE id=$1::uuid`, fixture.Request)
	if verifier != fixture.Reviewer {
		t.Fatalf("应记录验收人为签署人，实际 %s", verifier)
	}
	// 版本必须已发布。
	release := db.ScanString(t, `SELECT COALESCE(release_number::text,'') FROM attachment_versions WHERE id=$1::uuid`, fixture.Version)
	if release == "" {
		t.Fatal("签署通过后提交版本应发布为正式版本")
	}
	// 节点签署人与案例状态必须同时落库，不能只完成一半。
	signedNodes := db.ScanString(t, `SELECT count(*)::text FROM review_case_nodes WHERE review_case_id=$1::uuid AND status='pass'`, fixture.CaseID)
	if signedNodes != "2" {
		t.Fatalf("两个节点都应已通过，实际 %s", signedNodes)
	}
}

// TestSignEarlyNodeDoesNotPublish 非最后一个节点签署通过时不得发布。
func TestSignEarlyNodeDoesNotPublish(t *testing.T) {
	db := New(t)
	fixture := setupSignedReview(t, db)
	// 再造一个待签节点，使“专业审核”不再是最后一个。
	db.Exec(t, `INSERT INTO review_case_nodes(review_case_id,name,assigned_user_id,assigned_name,status,opinion,required,node_order,signer_role)
		VALUES($1::uuid,'主管批准',$2::uuid,'审核员','pending','',true,3,'批准')`, fixture.CaseID, fixture.Reviewer)

	if err := signNode(t, fixture.Repository, fixture.CaseID, "专业审核", "pass", "同意", fixture.Reviewer); err != nil {
		t.Fatalf("签署失败: %v", err)
	}
	// 仍有待签节点，工单与版本都不能变化。
	requestStatus := db.ScanString(t, `SELECT status FROM change_requests WHERE id=$1::uuid`, fixture.Request)
	if requestStatus != "pending_verify" {
		t.Fatalf("未全部签署时工单应保持 pending_verify，实际 %s", requestStatus)
	}
	release := db.ScanString(t, `SELECT COALESCE(release_number::text,'') FROM attachment_versions WHERE id=$1::uuid`, fixture.Version)
	if release != "" {
		t.Fatal("未全部签署时不应发布版本")
	}
}

// TestSignRejectionReturnsChangeToExecuting 任一节点驳回时工单回到执行中。
func TestSignRejectionReturnsChangeToExecuting(t *testing.T) {
	db := New(t)
	fixture := setupSignedReview(t, db)

	if err := signNode(t, fixture.Repository, fixture.CaseID, "专业审核", "rejected", "尺寸标注有误", fixture.Reviewer); err != nil {
		t.Fatalf("驳回失败: %v", err)
	}
	caseStatus := db.ScanString(t, `SELECT status FROM review_cases WHERE id=$1::uuid`, fixture.CaseID)
	if caseStatus != "rejected" {
		t.Fatalf("审核单应为 rejected，实际 %s", caseStatus)
	}
	requestStatus := db.ScanString(t, `SELECT status FROM change_requests WHERE id=$1::uuid`, fixture.Request)
	if requestStatus != "executing" {
		t.Fatalf("驳回后工单应回到 executing，实际 %s", requestStatus)
	}
	submissionStatus := db.ScanString(t, `SELECT status FROM change_request_submissions WHERE id=$1::uuid`, fixture.Submission)
	if submissionStatus != "returned" {
		t.Fatalf("轮次应为 returned，实际 %s", submissionStatus)
	}
	release := db.ScanString(t, `SELECT COALESCE(release_number::text,'') FROM attachment_versions WHERE id=$1::uuid`, fixture.Version)
	if release != "" {
		t.Fatal("驳回不应发布版本")
	}
}

// TestSignRequiresConfiguredPublisher 未装配发布服务时必须拒绝而不是静默跳过发布。
//
// 若这里退化为“签署成功但不发布”，工单会显示已完成却没有任何正式版本，
// 变更成果静默丢失。
func TestSignRequiresConfiguredPublisher(t *testing.T) {
	db := New(t)
	fixture := setupSignedReview(t, db)
	// 清空发布回调，模拟装配遗漏。
	unconfigured := review.NewPGRepository(db.Pool)

	err := signNode(t, unconfigured, fixture.CaseID, "专业审核", "pass", "同意", fixture.Reviewer)
	if err == nil || !strings.Contains(err.Error(), "发布服务未配置") {
		t.Fatalf("未配置发布服务应报错，实际 %v", err)
	}
	// 失败必须整体回滚：节点不能残留为已签署。
	nodeStatus := db.ScanString(t, `SELECT status FROM review_case_nodes WHERE review_case_id=$1::uuid AND name='专业审核'`, fixture.CaseID)
	if nodeStatus != "pending" {
		t.Fatalf("失败后节点应保持 pending，实际 %s", nodeStatus)
	}
	caseStatus := db.ScanString(t, `SELECT status FROM review_cases WHERE id=$1::uuid`, fixture.CaseID)
	if caseStatus != "reviewing" {
		t.Fatalf("失败后审核单应保持 reviewing，实际 %s", caseStatus)
	}
}

// TestSignRejectsOutOfOrderNode 必须按顺序签署，防止跳过前置节点。
func TestSignRejectsOutOfOrderNode(t *testing.T) {
	db := New(t)
	fixture := setupSignedReview(t, db)
	// 已签署节点受触发器保护、不可改回 pending，因此另建一个含两个待签节点的审核单。
	// 同一图纸只允许一个活跃审核单，先把原审核单置为非活跃并解除轮次占用。
	db.Exec(t, `UPDATE change_requests SET current_submission_id=NULL WHERE id=$1::uuid`, fixture.Request)
	db.Exec(t, `UPDATE review_cases SET status='rejected',completed_at=now() WHERE id=$1::uuid`, fixture.CaseID)
	secondCase := db.ScanString(t, `INSERT INTO review_cases(drawing_id,flow_id,status,initiator_id,flow_name_snapshot)
		VALUES($1::uuid,(SELECT flow_id FROM review_cases WHERE id=$2::uuid),'reviewing',$3::uuid,'完整审核') RETURNING id::text`,
		fixture.Drawing, fixture.CaseID, fixture.Author)
	db.Exec(t, `INSERT INTO review_case_nodes(review_case_id,name,assigned_user_id,assigned_name,status,opinion,required,node_order,signer_role)
		VALUES($1::uuid,'校对复核',$2::uuid,'审核员','pending','',true,1,'校对')`, secondCase, fixture.Reviewer)
	db.Exec(t, `INSERT INTO review_case_nodes(review_case_id,name,assigned_user_id,assigned_name,status,opinion,required,node_order,signer_role)
		VALUES($1::uuid,'专业审核',$2::uuid,'审核员','pending','',true,2,'审核')`, secondCase, fixture.Reviewer)

	err := signNode(t, fixture.Repository, secondCase, "专业审核", "pass", "越序签署", fixture.Reviewer)
	if err == nil || !strings.Contains(err.Error(), "请先处理当前节点") {
		t.Fatalf("应拒绝越序签署并提示当前节点，实际 %v", err)
	}
	// 越序尝试不得改变任何节点状态。
	statuses := db.ScanString(t, `SELECT string_agg(status,',' ORDER BY node_order) FROM review_case_nodes WHERE review_case_id=$1::uuid`, secondCase)
	if statuses != "pending,pending" {
		t.Fatalf("越序尝试不应改变节点状态，实际 %s", statuses)
	}
}

// TestSignRejectsNonAssignee 仅节点责任人可以签署。
func TestSignRejectsNonAssignee(t *testing.T) {
	db := New(t)
	fixture := setupSignedReview(t, db)

	err := signNode(t, fixture.Repository, fixture.CaseID, "专业审核", "pass", "越权签署", fixture.Author)
	if err == nil || !strings.Contains(err.Error(), "无权签署") {
		t.Fatalf("非责任人应被拒绝，实际 %v", err)
	}
	// 越权尝试不得留下任何签署痕迹。
	status := db.ScanString(t, `SELECT status FROM review_case_nodes WHERE review_case_id=$1::uuid AND name='专业审核'`, fixture.CaseID)
	if status != "pending" {
		t.Fatalf("越权签署不应改变节点状态，实际 %s", status)
	}
}

// TestSignRejectsAlreadySignedNode 已签署的节点不能再次签署。
func TestSignRejectsAlreadySignedNode(t *testing.T) {
	db := New(t)
	fixture := setupSignedReview(t, db)
	// fixture 中“校对复核”已处于 pass，重复签署必须被拒绝。
	err := signNode(t, fixture.Repository, fixture.CaseID, "校对复核", "pass", "重复签署", fixture.Reviewer)
	if err == nil || !strings.Contains(err.Error(), "不是待处理状态") {
		t.Fatalf("已签署节点应拒绝再次签署，实际 %v", err)
	}
	// 原签署意见不得被覆盖。
	opinion := db.ScanString(t, `SELECT opinion FROM review_case_nodes WHERE review_case_id=$1::uuid AND name='校对复核'`, fixture.CaseID)
	if opinion != "第一轮同意" {
		t.Fatalf("原签署意见不应被改写，实际 %q", opinion)
	}
}

// TestSignRejectsEndedCase 已结束的审核单不能继续签署。
func TestSignRejectsEndedCase(t *testing.T) {
	db := New(t)
	fixture := setupSignedReview(t, db)
	// 驳回结束该轮审核。
	if err := signNode(t, fixture.Repository, fixture.CaseID, "专业审核", "rejected", "驳回", fixture.Reviewer); err != nil {
		t.Fatalf("驳回失败: %v", err)
	}
	err := signNode(t, fixture.Repository, fixture.CaseID, "专业审核", "pass", "重签", fixture.Reviewer)
	if err == nil || !strings.Contains(err.Error(), "已结束") {
		t.Fatalf("已结束的审核单应拒绝签署，实际 %v", err)
	}
}

// TestSignIncompleteNodesRollsBackOnPublishFailure 发布失败时签署必须一并回滚。
//
// 这是“签署与发布同事务”的核心验证：若签署先落库而发布失败，
// 工单会卡在 pending_verify 且节点已全通过，既不能重签也不能发布。
func TestSignIncompleteNodesRollsBackOnPublishFailure(t *testing.T) {
	db := New(t)
	fixture := setupSignedReview(t, db)
	// 让在用版本偏离本轮基线，applyCompletion 会拒绝发布。
	db.Exec(t, `UPDATE attachments SET current_version_id=NULL WHERE id=(SELECT attachment_id FROM change_request_submission_targets WHERE submission_id=$1::uuid)`, fixture.Submission)

	err := signNode(t, fixture.Repository, fixture.CaseID, "专业审核", "pass", "同意", fixture.Reviewer)
	if err == nil {
		t.Fatal("基线不一致时发布应失败")
	}
	// 全部回滚：节点、案例、工单都不能变化。
	nodeStatus := db.ScanString(t, `SELECT status FROM review_case_nodes WHERE review_case_id=$1::uuid AND name='专业审核'`, fixture.CaseID)
	if nodeStatus != "pending" {
		t.Fatalf("发布失败后节点应回滚为 pending，实际 %s", nodeStatus)
	}
	caseStatus := db.ScanString(t, `SELECT status FROM review_cases WHERE id=$1::uuid`, fixture.CaseID)
	if caseStatus != "reviewing" {
		t.Fatalf("发布失败后审核单应回滚为 reviewing，实际 %s", caseStatus)
	}
	requestStatus := db.ScanString(t, `SELECT status FROM change_requests WHERE id=$1::uuid`, fixture.Request)
	if requestStatus != "pending_verify" {
		t.Fatalf("发布失败后工单应保持 pending_verify，实际 %s", requestStatus)
	}
}

// TestSignConcurrentSameNodeOnlyOneWins 同一节点被并发签署时只有一个生效。
func TestSignConcurrentSameNodeOnlyOneWins(t *testing.T) {
	db := New(t)
	fixture := setupSignedReview(t, db)

	const workers = 4
	var wg sync.WaitGroup
	results := make([]error, workers)
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			results[index] = signNode(t, fixture.Repository, fixture.CaseID, "专业审核", "pass", "并发通过", fixture.Reviewer)
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
		t.Fatalf("并发签署同一节点应只有一个成功，实际 %d 个（错误: %v）", succeeded, results)
	}
	// 只允许发布一次，版本号只推进一次。
	releases := db.ScanString(t, `SELECT count(*)::text FROM attachment_versions WHERE release_number IS NOT NULL`)
	if releases != "1" {
		t.Fatalf("应只发布 1 个正式版本，实际 %s", releases)
	}
	actions := db.ScanString(t, `SELECT count(*)::text FROM review_actions WHERE review_case_id=$1::uuid AND action='pass'`, fixture.CaseID)
	if actions != "1" {
		t.Fatalf("应只记录 1 条通过动作，实际 %s", actions)
	}
}

// TestSignConcurrentPassAndRejectIsConsistent 并发通过与驳回混合时结果必须自洽。
func TestSignConcurrentPassAndRejectIsConsistent(t *testing.T) {
	db := New(t)
	fixture := setupSignedReview(t, db)

	var wg sync.WaitGroup
	results := make([]error, 2)
	actions := []string{"pass", "rejected"}
	start := make(chan struct{})
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			results[index] = signNode(t, fixture.Repository, fixture.CaseID, "专业审核", actions[index], "并发决策", fixture.Reviewer)
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
		t.Fatalf("并发决策应只有一个成功，实际 %d 个", succeeded)
	}
	// 最终状态必须与成功的决策一致。
	caseStatus := db.ScanString(t, `SELECT status FROM review_cases WHERE id=$1::uuid`, fixture.CaseID)
	requestStatus := db.ScanString(t, `SELECT status FROM change_requests WHERE id=$1::uuid`, fixture.Request)
	releases := db.ScanString(t, `SELECT count(*)::text FROM attachment_versions WHERE release_number IS NOT NULL`)
	switch caseStatus {
	case "published":
		if requestStatus != "completed" || releases != "1" {
			t.Fatalf("通过后应为 completed 且有 1 个正式版本，实际 %s / %s", requestStatus, releases)
		}
	case "rejected":
		if requestStatus != "executing" || releases != "0" {
			t.Fatalf("驳回后应为 executing 且无正式版本，实际 %s / %s", requestStatus, releases)
		}
	default:
		t.Fatalf("最终审核单状态不合法: %s", caseStatus)
	}
}
