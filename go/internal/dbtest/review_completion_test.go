package dbtest

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"cadguanliq/internal/change"
	"github.com/jackc/pgx/v5"
)

// 本文件验证变更工单完整审核的完成路径。CompleteReview 需要真实事务，无法用 mock
// 覆盖：它依赖行锁、状态前置条件和跨表发布的一致性。

// reviewFixture 描述一次完整审核所需的全部数据。
type reviewFixture struct {
	Fixture
	Request    string
	Submission string
	Flow       string
	CaseID     string
	Version    string
}

// setupReview 建立工单 → 提交轮次 → 审核单的完整链路，工单处于待完整审核状态。
func setupReview(t *testing.T, db *DB) reviewFixture {
	t.Helper()
	fixture := db.Seed(t)
	flow := insertFlow(t, db, fixture)
	// 提交目标指向零件附件，完成审核时会发布零件新修订；发布要求已有基础修订。
	insertBaseRevision(t, db, fixture)
	request := insertChangeRequest(t, db, fixture, "CR-REV")
	submission := insertSubmission(t, db, request, 1)
	db.Exec(t, `UPDATE change_requests SET status='pending_verify',current_submission_id=$2::uuid WHERE id=$1::uuid`, request, submission)

	// 提交目标指向一个工作版本，完成审核时会发布为正式版本。
	attachment, versionID := insertPartAttachment(t, db, fixture, "v1.0")
	db.Exec(t, `INSERT INTO change_request_submission_targets(submission_id,attachment_id,base_attachment_version_id,submitted_attachment_version_id)
		VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid)`, submission, attachment, versionID, versionID)

	var caseID string
	if err := db.Pool.QueryRow(context.Background(), `INSERT INTO review_cases(drawing_id,flow_id,status,initiator_id,change_submission_id,flow_name_snapshot)
		VALUES($1::uuid,$2::uuid,'published',$3::uuid,$4::uuid,'完整审核') RETURNING id::text`,
		fixture.Drawing, flow, fixture.Author, submission).Scan(&caseID); err != nil {
		t.Fatalf("插入审核单失败: %v", err)
	}
	// 全部节点已通过是完成审核的前置条件。
	db.Exec(t, `INSERT INTO review_case_nodes(review_case_id,name,assigned_user_id,assigned_name,status,opinion,required,node_order,signer_role)
		VALUES($1::uuid,'校对复核',$2::uuid,'审核员','pass','同意',true,1,'校对')`, caseID, fixture.Reviewer)

	return reviewFixture{Fixture: fixture, Request: request, Submission: submission, Flow: flow, CaseID: caseID, Version: versionID}
}

// completeReview 在独立事务中调用 CompleteReview，按需提交或回滚。
// 并发场景下若出现锁等待，由 deadline 保证测试快速失败而不是挂起。
func completeReview(t *testing.T, db *DB, submissionID, actorID string, accepted bool, opinion string, commit bool) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("开启事务失败: %v", err)
	}
	defer tx.Rollback(ctx)
	service := change.NewService(db.Pool)
	if err = service.CompleteReview(ctx, tx, submissionID, actorID, accepted, opinion); err != nil {
		return err
	}
	if !commit {
		return nil
	}
	return tx.Commit(ctx)
}

// TestCompleteReviewRejectsStaleSubmission 提交轮次已更新时必须拒绝，防止验收旧成果。
func TestCompleteReviewRejectsStaleSubmission(t *testing.T) {
	db := New(t)
	fixture := setupReview(t, db)
	// 新的一轮提交取代了当前轮次。
	newer := insertSubmission(t, db, fixture.Request, 2)
	db.Exec(t, `UPDATE change_requests SET current_submission_id=$2::uuid WHERE id=$1::uuid`, fixture.Request, newer)

	err := completeReview(t, db, fixture.Submission, fixture.Reviewer, true, "通过", true)
	if !errors.Is(err, change.ErrStaleSubmit) {
		t.Fatalf("应返回 ErrStaleSubmit，实际 %v", err)
	}
}

// TestCompleteReviewRejectsWrongStatus 工单不在待完整审核状态时不得发布。
func TestCompleteReviewRejectsWrongStatus(t *testing.T) {
	db := New(t)
	fixture := setupReview(t, db)
	db.Exec(t, `UPDATE change_requests SET status='executing' WHERE id=$1::uuid`, fixture.Request)

	err := completeReview(t, db, fixture.Submission, fixture.Reviewer, true, "通过", true)
	if !errors.Is(err, change.ErrStaleSubmit) {
		t.Fatalf("应返回 ErrStaleSubmit，实际 %v", err)
	}
}

// TestCompleteReviewRequiresAllNodesPassed 节点未全部通过时禁止发布。
func TestCompleteReviewRequiresAllNodesPassed(t *testing.T) {
	db := New(t)
	fixture := setupReview(t, db)
	// 追加一个尚未签署的节点。
	db.Exec(t, `INSERT INTO review_case_nodes(review_case_id,name,assigned_user_id,assigned_name,status,opinion,required,node_order,signer_role)
		VALUES($1::uuid,'专业审核',$2::uuid,'审核员','pending','',true,2,'审核')`, fixture.CaseID, fixture.Reviewer)

	err := completeReview(t, db, fixture.Submission, fixture.Reviewer, true, "通过", true)
	if err == nil || !strings.Contains(err.Error(), "审核节点尚未全部通过") {
		t.Fatalf("应拒绝未全部通过的审核，实际 %v", err)
	}
	// 拒绝后状态必须保持不变，不能留下半完成工单。
	status := db.ScanString(t, `SELECT status FROM change_requests WHERE id=$1::uuid`, fixture.Request)
	if status != "pending_verify" {
		t.Fatalf("失败后状态不应改变，实际 %s", status)
	}
}

// TestCompleteReviewRejectionReturnsToExecuting 退回必须让工单回到执行中并标记轮次已退回。
func TestCompleteReviewRejectionReturnsToExecuting(t *testing.T) {
	db := New(t)
	fixture := setupReview(t, db)

	if err := completeReview(t, db, fixture.Submission, fixture.Reviewer, false, "尺寸标注有误", true); err != nil {
		t.Fatalf("退回操作不应失败: %v", err)
	}
	status := db.ScanString(t, `SELECT status FROM change_requests WHERE id=$1::uuid`, fixture.Request)
	if status != "executing" {
		t.Fatalf("退回后应为 executing，实际 %s", status)
	}
	submissionStatus := db.ScanString(t, `SELECT status FROM change_request_submissions WHERE id=$1::uuid`, fixture.Submission)
	if submissionStatus != "returned" {
		t.Fatalf("轮次应标记为 returned，实际 %s", submissionStatus)
	}
	// 退回必须留下意见供设计员修改。
	opinion := db.ScanString(t, `SELECT string_agg(opinion,'|') FROM change_request_actions WHERE request_id=$1::uuid AND action='return'`, fixture.Request)
	if !strings.Contains(opinion, "尺寸标注有误") {
		t.Fatalf("退回意见未记录: %q", opinion)
	}
	// 退回不得发布文件版本。
	releaseNumbers := db.ScanString(t, `SELECT count(*)::text FROM attachment_versions WHERE release_number IS NOT NULL`)
	if releaseNumbers != "0" {
		t.Fatalf("退回不应发布正式版本，实际 %s 条", releaseNumbers)
	}
}

// TestCompleteReviewApprovalPublishesAndCompletes 通过时完成工单、发布版本并记录签署人与审计。
func TestCompleteReviewApprovalPublishesAndCompletes(t *testing.T) {
	db := New(t)
	fixture := setupReview(t, db)

	if err := completeReview(t, db, fixture.Submission, fixture.Reviewer, true, "同意发布", true); err != nil {
		t.Fatalf("通过审核失败: %v", err)
	}

	status := db.ScanString(t, `SELECT status FROM change_requests WHERE id=$1::uuid`, fixture.Request)
	if status != "completed" {
		t.Fatalf("通过后应为 completed，实际 %s", status)
	}
	verifier := db.ScanString(t, `SELECT verifier_id::text FROM change_requests WHERE id=$1::uuid`, fixture.Request)
	if verifier != fixture.Reviewer {
		t.Fatalf("应记录验收人，期望 %s 实际 %s", fixture.Reviewer, verifier)
	}
	for _, column := range []string{"verified_at", "completed_at"} {
		value := db.ScanString(t, `SELECT `+column+`::text FROM change_requests WHERE id=$1::uuid`, fixture.Request)
		if value == "" {
			t.Fatalf("%s 应被写入", column)
		}
	}
	submissionStatus := db.ScanString(t, `SELECT status FROM change_request_submissions WHERE id=$1::uuid`, fixture.Submission)
	if submissionStatus != "accepted" {
		t.Fatalf("轮次应标记为 accepted，实际 %s", submissionStatus)
	}
	// 提交的工作版本必须发布为正式版本。
	releaseNumber := db.ScanString(t, `SELECT COALESCE(release_number::text,'') FROM attachment_versions WHERE id=$1::uuid`, fixture.Version)
	if releaseNumber == "" {
		t.Fatal("提交版本应被发布为正式版本")
	}
	// 正式版本号由发布规则分配，不接受手工填写。
	version := db.ScanString(t, `SELECT version FROM drawings WHERE id=$1::uuid`, fixture.Drawing)
	if !strings.HasPrefix(version, "V") {
		t.Fatalf("图纸正式版本应为 V 开头，实际 %s", version)
	}
	// 签署人必须按节点的 signer_role 写入图纸。
	signer := db.ScanString(t, `SELECT count(*)::text FROM drawing_signers WHERE drawing_id=$1::uuid AND role='校对'`, fixture.Drawing)
	if signer != "1" {
		t.Fatalf("应写入校对签署人，实际 %s 条", signer)
	}
	// 审计与动作日志都要有记录，供追溯。
	audit := db.ScanString(t, `SELECT count(*)::text FROM audit_logs WHERE resource_id=$1::uuid AND action='change_request_verify'`, fixture.Request)
	if audit != "1" {
		t.Fatalf("应写入验收审计，实际 %s 条", audit)
	}
	action := db.ScanString(t, `SELECT count(*)::text FROM change_request_actions WHERE request_id=$1::uuid AND action='verify'`, fixture.Request)
	if action != "1" {
		t.Fatalf("应写入验收动作，实际 %s 条", action)
	}
}

// TestCompleteReviewRollsBackEverythingOnFailure 发布失败时整个事务必须回滚。
//
// 完成审核同时做状态流转、版本发布与签署人写入。任何一步失败都不能留下已完成
// 但未发布的工单，否则会丢失变更成果且无法重新提交。
func TestCompleteReviewRollsBackEverythingOnFailure(t *testing.T) {
	db := New(t)
	fixture := setupReview(t, db)
	// 让在用的当前版本偏离本轮基线，applyCompletion 会因此拒绝发布。
	db.Exec(t, `UPDATE change_requests SET current_submission_id=$2::uuid WHERE id=$1::uuid`, fixture.Request, fixture.Submission)
	db.Exec(t, `UPDATE attachments SET current_version_id=NULL WHERE id=(SELECT attachment_id FROM change_request_submission_targets WHERE submission_id=$1::uuid)`, fixture.Submission)

	err := completeReview(t, db, fixture.Submission, fixture.Reviewer, true, "通过", true)
	if err == nil {
		t.Fatal("基线不一致时应拒绝发布")
	}
	// 事务回滚：状态、轮次、正式版本都必须保持原样。
	status := db.ScanString(t, `SELECT status FROM change_requests WHERE id=$1::uuid`, fixture.Request)
	if status != "pending_verify" {
		t.Fatalf("失败后状态必须回滚为 pending_verify，实际 %s", status)
	}
	submissionStatus := db.ScanString(t, `SELECT status FROM change_request_submissions WHERE id=$1::uuid`, fixture.Submission)
	if submissionStatus != "pending" {
		t.Fatalf("失败后轮次必须保持 pending，实际 %s", submissionStatus)
	}
	releaseNumbers := db.ScanString(t, `SELECT count(*)::text FROM attachment_versions WHERE release_number IS NOT NULL`)
	if releaseNumbers != "0" {
		t.Fatalf("失败后不应留下正式版本，实际 %s 条", releaseNumbers)
	}
	signers := db.ScanString(t, `SELECT count(*)::text FROM drawing_signers WHERE drawing_id=$1::uuid`, fixture.Drawing)
	if signers != "0" {
		t.Fatalf("失败后不应留下签署人，实际 %s 条", signers)
	}
}

// TestCompleteReviewConcurrentApprovalOnlyOneWins 并发审批只允许一个成功。
//
// 两个审核人同时点“通过”时必须串行化：行锁保证只有一个事务能看到 pending_verify，
// 否则会重复发布并重复递增正式版本号。
func TestCompleteReviewConcurrentApprovalOnlyOneWins(t *testing.T) {
	db := New(t)
	fixture := setupReview(t, db)

	const workers = 4
	var wg sync.WaitGroup
	results := make([]error, workers)
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			results[index] = completeReview(t, db, fixture.Submission, fixture.Reviewer, true, "并发通过", true)
		}(i)
	}
	close(start)
	wg.Wait()

	succeeded := 0
	for _, err := range results {
		if err == nil {
			succeeded++
		} else if !errors.Is(err, change.ErrStaleSubmit) {
			t.Fatalf("失败的事务只能因轮次失效而失败，实际 %v", err)
		}
	}
	if succeeded != 1 {
		t.Fatalf("并发审批应只有一个成功，实际 %d 个", succeeded)
	}
	// 只允许发布一份正式版本，且版本号只递增一次。
	releases := db.ScanString(t, `SELECT count(*)::text FROM attachment_versions WHERE release_number IS NOT NULL`)
	if releases != "1" {
		t.Fatalf("应只发布 1 个正式版本，实际 %s", releases)
	}
	audits := db.ScanString(t, `SELECT count(*)::text FROM audit_logs WHERE resource_id=$1::uuid AND action='change_request_verify'`, fixture.Request)
	if audits != "1" {
		t.Fatalf("应只写入 1 条验收审计，实际 %s", audits)
	}
	actions := db.ScanString(t, `SELECT count(*)::text FROM change_request_actions WHERE request_id=$1::uuid AND action='verify'`, fixture.Request)
	if actions != "1" {
		t.Fatalf("应只写入 1 条验收动作，实际 %s", actions)
	}
	version := db.ScanString(t, `SELECT version FROM drawings WHERE id=$1::uuid`, fixture.Drawing)
	if version != "V2" {
		t.Fatalf("正式版本号应只递增一次为 V2，实际 %s", version)
	}
}

// TestCompleteReviewConcurrentMixedDecisionsIsAtomic 并发“通过”与“退回”混合时结果必须自洽。
func TestCompleteReviewConcurrentMixedDecisionsIsAtomic(t *testing.T) {
	db := New(t)
	fixture := setupReview(t, db)

	var wg sync.WaitGroup
	results := make([]error, 2)
	start := make(chan struct{})
	decisions := []bool{true, false}
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			results[index] = completeReview(t, db, fixture.Submission, fixture.Reviewer, decisions[index], "并发决策", true)
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
	// 最终状态必须与成功的决策一致，不能出现通过与非通过混合的中间态。
	status := db.ScanString(t, `SELECT status FROM change_requests WHERE id=$1::uuid`, fixture.Request)
	submissionStatus := db.ScanString(t, `SELECT status FROM change_request_submissions WHERE id=$1::uuid`, fixture.Submission)
	releases := db.ScanString(t, `SELECT count(*)::text FROM attachment_versions WHERE release_number IS NOT NULL`)
	switch status {
	case "completed":
		if submissionStatus != "accepted" || releases != "1" {
			t.Fatalf("通过后轮次应为 accepted 且有 1 个正式版本，实际 %s / %s", submissionStatus, releases)
		}
	case "executing":
		if submissionStatus != "returned" || releases != "0" {
			t.Fatalf("退回后轮次应为 returned 且无正式版本，实际 %s / %s", submissionStatus, releases)
		}
	default:
		t.Fatalf("最终状态不合法: %s", status)
	}
}

// TestCompleteReviewIsIdempotentAfterSuccess 完成后重复调用必须拒绝，避免二次发布。
func TestCompleteReviewIsIdempotentAfterSuccess(t *testing.T) {
	db := New(t)
	fixture := setupReview(t, db)
	if err := completeReview(t, db, fixture.Submission, fixture.Reviewer, true, "第一次通过", true); err != nil {
		t.Fatalf("首次通过失败: %v", err)
	}
	versionBefore := db.ScanString(t, `SELECT version FROM drawings WHERE id=$1::uuid`, fixture.Drawing)

	err := completeReview(t, db, fixture.Submission, fixture.Reviewer, true, "第二次通过", true)
	if !errors.Is(err, change.ErrStaleSubmit) {
		t.Fatalf("重复通过应返回 ErrStaleSubmit，实际 %v", err)
	}
	versionAfter := db.ScanString(t, `SELECT version FROM drawings WHERE id=$1::uuid`, fixture.Drawing)
	if versionBefore != versionAfter {
		t.Fatalf("重复调用不应再次递增版本号: %s -> %s", versionBefore, versionAfter)
	}
	releases := db.ScanString(t, `SELECT count(*)::text FROM attachment_versions WHERE release_number IS NOT NULL`)
	if releases != "1" {
		t.Fatalf("重复调用不应重复发布，实际 %s 条", releases)
	}
}

// TestCompleteReviewReleasesBaselineSnapshotBeforeChange 完成时必须保留变更前基线快照。
func TestCompleteReviewReleasesBaselineSnapshotBeforeChange(t *testing.T) {
	db := New(t)
	fixture := setupReview(t, db)
	if err := completeReview(t, db, fixture.Submission, fixture.Reviewer, true, "通过", true); err != nil {
		t.Fatalf("通过审核失败: %v", err)
	}
	source := db.ScanString(t, `SELECT source FROM drawing_release_snapshots WHERE drawing_id=$1::uuid AND source='变更前基线'`, fixture.Drawing)
	if source != "变更前基线" {
		t.Fatalf("应保留变更前基线快照，实际 %q", source)
	}
}

// TestCompleteReviewKeepsWorkVersionAfterPublish 发布后工作稿必须保留。
//
// 迁移把 prune_attachment_work 改为空实现，工作稿成为历史证据；若被清理，
// 将无法追溯设计过程。
func TestCompleteReviewKeepsWorkVersionAfterPublish(t *testing.T) {
	db := New(t)
	fixture := setupReview(t, db)
	if err := completeReview(t, db, fixture.Submission, fixture.Reviewer, true, "通过", true); err != nil {
		t.Fatalf("通过审核失败: %v", err)
	}
	kept := db.ScanString(t, `SELECT count(*)::text FROM attachment_versions WHERE id=$1::uuid AND deleted_at IS NULL`, fixture.Version)
	if kept != "1" {
		t.Fatal("发布后工作稿应保留为历史证据")
	}
}

// TestStartChangeCaseIsSkippedWhenSubmissionMissing 事务回滚后不应留下审核单。
func TestCompleteReviewTransactionRollsBackCaseCreation(t *testing.T) {
	db := New(t)
	fixture := setupReview(t, db)
	ctx := context.Background()
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// 在同一事务内完成审核后主动回滚，验证不会残留任何副作用。
	service := change.NewService(db.Pool)
	if err = service.CompleteReview(ctx, tx, fixture.Submission, fixture.Reviewer, true, "通过"); err != nil {
		t.Fatalf("事务内完成审核失败: %v", err)
	}
	if err = tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		t.Fatal(err)
	}
	status := db.ScanString(t, `SELECT status FROM change_requests WHERE id=$1::uuid`, fixture.Request)
	if status != "pending_verify" {
		t.Fatalf("回滚后工单应保持 pending_verify，实际 %s", status)
	}
	releases := db.ScanString(t, `SELECT count(*)::text FROM attachment_versions WHERE release_number IS NOT NULL`)
	if releases != "0" {
		t.Fatalf("回滚后不应有正式版本，实际 %s", releases)
	}
}
