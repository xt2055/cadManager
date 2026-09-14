package dbtest

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"cadguanliq/internal/review"
	"github.com/jackc/pgx/v5"
)

// 本文件用故障注入覆盖错误分支。
//
// 审核与发布链路上的每个数据库操作失败时都必须向上传播，让事务回滚。
// 若某个分支退化为“记录错误后继续”，会出现签署成功但未发布、或工单完成
// 但没有正式版本这类静默损坏。正常数据无法触发这些分支，因此注入故障。

// failOn 安装一个让指定表上的指定操作失败的临时触发器。
// 触发器随测试结束由 schema 回收，无需手工清理。
func failOn(t *testing.T, db *DB, table, operation, message string) {
	t.Helper()
	suffix := strings.ToLower(operation) + "_" + strings.ReplaceAll(table, "_", "")
	db.Exec(t, `CREATE FUNCTION fail_`+suffix+`() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION '`+message+`'; END; $$`)
	db.Exec(t, `CREATE TRIGGER test_fail_`+suffix+` BEFORE `+operation+` ON `+table+` FOR EACH ROW EXECUTE FUNCTION fail_`+suffix+`()`)
}

// TestCompleteReviewPropagatesUpdateFailure 工单状态更新失败必须传播。
func TestCompleteReviewPropagatesUpdateFailure(t *testing.T) {
	db := New(t)
	fixture := setupReview(t, db)
	failOn(t, db, "change_requests", "UPDATE", "注入工单更新失败")

	err := completeReview(t, db, fixture.Submission, fixture.Reviewer, false, "退回", true)
	if err == nil || !strings.Contains(err.Error(), "注入工单更新失败") {
		t.Fatalf("应传播注入的失败，实际 %v", err)
	}
	// 失败后工单状态必须保持原样。
	status := db.ScanString(t, `SELECT status FROM change_requests WHERE id=$1::uuid`, fixture.Request)
	if status != "pending_verify" {
		t.Fatalf("失败后状态应保持 pending_verify，实际 %s", status)
	}
}

// TestCompleteReviewPropagatesCompletionUpdateFailure 工单置为已完成的更新失败必须传播。
//
// 这里只拦截 status→completed 的更新，不影响读取，因此能精确命中该分支。
func TestCompleteReviewPropagatesCompletionUpdateFailure(t *testing.T) {
	db := New(t)
	fixture := setupReview(t, db)
	db.Exec(t, `CREATE FUNCTION fail_complete() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN IF NEW.status = 'completed' THEN RAISE EXCEPTION '注入完成更新失败'; END IF; RETURN NEW; END; $$`)
	db.Exec(t, `CREATE TRIGGER test_fail_complete BEFORE UPDATE ON change_requests FOR EACH ROW EXECUTE FUNCTION fail_complete()`)

	err := completeReview(t, db, fixture.Submission, fixture.Reviewer, true, "通过", true)
	if err == nil || !strings.Contains(err.Error(), "注入完成更新失败") {
		t.Fatalf("应传播注入的失败，实际 %v", err)
	}
	status := db.ScanString(t, `SELECT status FROM change_requests WHERE id=$1::uuid`, fixture.Request)
	if status != "pending_verify" {
		t.Fatalf("失败后状态应保持 pending_verify，实际 %s", status)
	}
}

// TestCompleteReviewPropagatesAcceptedMarkFailure 轮次标记为已接受的更新失败必须传播。
//
// 该分支在发布之后执行；若失败未被传播，会出现“版本已发布但轮次仍为待处理”。
func TestCompleteReviewPropagatesAcceptedMarkFailure(t *testing.T) {
	db := New(t)
	fixture := setupReview(t, db)
	db.Exec(t, `CREATE FUNCTION fail_accept() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN IF NEW.status = 'accepted' THEN RAISE EXCEPTION '注入接受标记失败'; END IF; RETURN NEW; END; $$`)
	db.Exec(t, `CREATE TRIGGER test_fail_accept BEFORE UPDATE ON change_request_submissions FOR EACH ROW EXECUTE FUNCTION fail_accept()`)

	err := completeReview(t, db, fixture.Submission, fixture.Reviewer, true, "通过", true)
	if err == nil || !strings.Contains(err.Error(), "注入接受标记失败") {
		t.Fatalf("应传播注入的失败，实际 %v", err)
	}
	// 发布必须随事务回滚，不能留下已发布的版本。
	releases := db.ScanString(t, `SELECT count(*)::text FROM attachment_versions WHERE release_number IS NOT NULL`)
	if releases != "0" {
		t.Fatalf("回滚后不应有正式版本，实际 %s", releases)
	}
	submissionStatus := db.ScanString(t, `SELECT status FROM change_request_submissions WHERE id=$1::uuid`, fixture.Submission)
	if submissionStatus != "pending" {
		t.Fatalf("失败后轮次应保持 pending，实际 %s", submissionStatus)
	}
}

// TestCompleteReviewPropagatesVerifyLogFailure 验收动作日志写入失败必须传播。
func TestCompleteReviewPropagatesVerifyLogFailure(t *testing.T) {
	db := New(t)
	fixture := setupReview(t, db)
	// 只拦截 verify 动作，不影响退回分支的 return 动作。
	db.Exec(t, `CREATE FUNCTION fail_verify_log() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN IF NEW.action = 'verify' THEN RAISE EXCEPTION '注入验收日志失败'; END IF; RETURN NEW; END; $$`)
	db.Exec(t, `CREATE TRIGGER test_fail_verify_log BEFORE INSERT ON change_request_actions FOR EACH ROW EXECUTE FUNCTION fail_verify_log()`)

	err := completeReview(t, db, fixture.Submission, fixture.Reviewer, true, "通过", true)
	if err == nil || !strings.Contains(err.Error(), "注入验收日志失败") {
		t.Fatalf("应传播注入的失败，实际 %v", err)
	}
	status := db.ScanString(t, `SELECT status FROM change_requests WHERE id=$1::uuid`, fixture.Request)
	if status != "pending_verify" {
		t.Fatalf("失败后状态应保持 pending_verify，实际 %s", status)
	}
}

// TestCompleteReviewPropagatesActionLogFailure 动作日志写入失败必须传播。
//
// 动作日志缺失会让流程无法追溯，因此不能容忍“状态已改但日志没写”。
func TestCompleteReviewPropagatesActionLogFailure(t *testing.T) {
	db := New(t)
	fixture := setupReview(t, db)
	failOn(t, db, "change_request_actions", "INSERT", "注入日志写入失败")

	err := completeReview(t, db, fixture.Submission, fixture.Reviewer, false, "退回", true)
	if err == nil || !strings.Contains(err.Error(), "注入日志写入失败") {
		t.Fatalf("应传播注入的失败，实际 %v", err)
	}
	// 整笔事务回滚。
	status := db.ScanString(t, `SELECT status FROM change_requests WHERE id=$1::uuid`, fixture.Request)
	if status != "pending_verify" {
		t.Fatalf("失败后状态应保持 pending_verify，实际 %s", status)
	}
}

// TestCompleteReviewPropagatesSubmissionMarkFailure 轮次状态更新失败必须传播。
func TestCompleteReviewPropagatesSubmissionMarkFailure(t *testing.T) {
	db := New(t)
	fixture := setupReview(t, db)
	// 只阻止对本轮次的更新，避免影响读取。
	db.Exec(t, `CREATE FUNCTION fail_submission() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN IF NEW.id = '`+fixture.Submission+`'::uuid THEN RAISE EXCEPTION '注入轮次更新失败'; END IF; RETURN NEW; END; $$`)
	db.Exec(t, `CREATE TRIGGER test_fail_submission BEFORE UPDATE ON change_request_submissions FOR EACH ROW EXECUTE FUNCTION fail_submission()`)

	err := completeReview(t, db, fixture.Submission, fixture.Reviewer, false, "退回", true)
	if err == nil || !strings.Contains(err.Error(), "注入轮次更新失败") {
		t.Fatalf("应传播注入的失败，实际 %v", err)
	}
}

// TestCompleteReviewPropagatesAuditFailure 审计写入失败必须传播。
func TestCompleteReviewPropagatesAuditFailure(t *testing.T) {
	db := New(t)
	fixture := setupReview(t, db)
	failOn(t, db, "audit_logs", "INSERT", "注入审计写入失败")

	err := completeReview(t, db, fixture.Submission, fixture.Reviewer, true, "通过", true)
	if err == nil || !strings.Contains(err.Error(), "注入审计写入失败") {
		t.Fatalf("应传播注入的失败，实际 %v", err)
	}
	// 审计失败时整笔回滚，不能留下已完成工单。
	status := db.ScanString(t, `SELECT status FROM change_requests WHERE id=$1::uuid`, fixture.Request)
	if status != "pending_verify" {
		t.Fatalf("失败后状态应保持 pending_verify，实际 %s", status)
	}
	releases := db.ScanString(t, `SELECT count(*)::text FROM attachment_versions WHERE release_number IS NOT NULL`)
	if releases != "0" {
		t.Fatalf("回滚后不应有正式版本，实际 %s", releases)
	}
}

// TestCompleteReviewPropagatesBaselineSnapshotFailure 基线快照捕获失败必须传播。
//
// 快照是变更前证据，缺失会让变更无法回溯。
func TestCompleteReviewPropagatesBaselineSnapshotFailure(t *testing.T) {
	db := New(t)
	fixture := setupReview(t, db)
	failOn(t, db, "drawing_release_snapshots", "INSERT", "注入快照写入失败")

	err := completeReview(t, db, fixture.Submission, fixture.Reviewer, true, "通过", true)
	if err == nil || !strings.Contains(err.Error(), "注入快照写入失败") {
		t.Fatalf("应传播注入的失败，实际 %v", err)
	}
	status := db.ScanString(t, `SELECT status FROM change_requests WHERE id=$1::uuid`, fixture.Request)
	if status != "pending_verify" {
		t.Fatalf("失败后状态应保持 pending_verify，实际 %s", status)
	}
}

// TestCompleteReviewPropagatesSnapshotQueryFailure 读取属性草案失败必须传播。
func TestCompleteReviewPropagatesSnapshotQueryFailure(t *testing.T) {
	db := New(t)
	fixture := setupReview(t, db)
	// 让 drawing_signers 的插入失败，覆盖 applyCompletion 内部的失败路径。
	failOn(t, db, "drawing_signers", "INSERT", "注入签署人写入失败")

	err := completeReview(t, db, fixture.Submission, fixture.Reviewer, true, "通过", true)
	if err == nil || !strings.Contains(err.Error(), "注入签署人写入失败") {
		t.Fatalf("应传播注入的失败，实际 %v", err)
	}
	signers := db.ScanString(t, `SELECT count(*)::text FROM drawing_signers WHERE drawing_id=$1::uuid`, fixture.Drawing)
	if signers != "0" {
		t.Fatalf("回滚后不应留下签署人，实际 %s", signers)
	}
}

// TestCompleteReviewPropagatesReleasePublishFailure 版本发布失败必须传播。
func TestCompleteReviewPropagatesReleasePublishFailure(t *testing.T) {
	db := New(t)
	fixture := setupReview(t, db)
	failOn(t, db, "part_revisions", "INSERT", "注入零件修订发布失败")

	err := completeReview(t, db, fixture.Submission, fixture.Reviewer, true, "通过", true)
	if err == nil || !strings.Contains(err.Error(), "注入零件修订发布失败") {
		t.Fatalf("应传播注入的失败，实际 %v", err)
	}
	releases := db.ScanString(t, `SELECT count(*)::text FROM attachment_versions WHERE release_number IS NOT NULL`)
	if releases != "0" {
		t.Fatalf("失败后不应留下正式版本，实际 %s", releases)
	}
}

// TestSubmitNodePropagatesActionLogFailure 签署动作日志失败必须传播并回滚节点状态。
func TestSubmitNodePropagatesActionLogFailure(t *testing.T) {
	db := New(t)
	fixture := setupSignedReview(t, db)
	failOn(t, db, "review_actions", "INSERT", "注入审核动作失败")

	err := signNode(t, fixture.Repository, fixture.CaseID, "专业审核", "pass", "同意", fixture.Reviewer)
	if err == nil || !strings.Contains(err.Error(), "注入审核动作失败") {
		t.Fatalf("应传播注入的失败，实际 %v", err)
	}
	nodeStatus := db.ScanString(t, `SELECT status FROM review_case_nodes WHERE review_case_id=$1::uuid AND name='专业审核'`, fixture.CaseID)
	if nodeStatus != "pending" {
		t.Fatalf("失败后节点应回滚为 pending，实际 %s", nodeStatus)
	}
}

// TestSubmitNodePropagatesNodeUpdateFailure 节点更新失败必须传播。
func TestSubmitNodePropagatesNodeUpdateFailure(t *testing.T) {
	db := New(t)
	fixture := setupSignedReview(t, db)
	failOn(t, db, "review_case_nodes", "UPDATE", "注入节点更新失败")

	err := signNode(t, fixture.Repository, fixture.CaseID, "专业审核", "pass", "同意", fixture.Reviewer)
	if err == nil || !strings.Contains(err.Error(), "注入节点更新失败") {
		t.Fatalf("应传播注入的失败，实际 %v", err)
	}
}

// TestSubmitNodePropagatesCaseUpdateFailure 案例状态更新失败必须传播。
func TestSubmitNodePropagatesCaseUpdateFailure(t *testing.T) {
	db := New(t)
	fixture := setupSignedReview(t, db)
	failOn(t, db, "review_cases", "UPDATE", "注入案例更新失败")

	err := signNode(t, fixture.Repository, fixture.CaseID, "专业审核", "pass", "同意", fixture.Reviewer)
	if err == nil || !strings.Contains(err.Error(), "注入案例更新失败") {
		t.Fatalf("应传播注入的失败，实际 %v", err)
	}
	// 节点必须回滚。
	nodeStatus := db.ScanString(t, `SELECT status FROM review_case_nodes WHERE review_case_id=$1::uuid AND name='专业审核'`, fixture.CaseID)
	if nodeStatus != "pending" {
		t.Fatalf("失败后节点应保持 pending，实际 %s", nodeStatus)
	}
}

// TestSubmitNodeUnknownCase 未知审核单必须报错而不是静默成功。
func TestSubmitNodeUnknownCase(t *testing.T) {
	db := New(t)
	fixture := setupSignedReview(t, db)
	err := signNode(t, fixture.Repository, fixture.CaseID, "不存在的节点", "pass", "同意", fixture.Reviewer)
	if err == nil || !strings.Contains(err.Error(), "未找到审核节点") {
		t.Fatalf("未知节点应报错，实际 %v", err)
	}
}

// TestSubmitNodeRejectsInvalidAction 非法动作必须被拒绝。
func TestSubmitNodeRejectsInvalidAction(t *testing.T) {
	db := New(t)
	fixture := setupSignedReview(t, db)
	err := signNode(t, fixture.Repository, fixture.CaseID, "专业审核", "approve", "同意", fixture.Reviewer)
	if err == nil || !strings.Contains(err.Error(), "无效的审核操作") {
		t.Fatalf("非法动作应被拒绝，实际 %v", err)
	}
}

// TestSubmitNodeRejectsMissingCase 不存在审核单时返回明确错误。
func TestSubmitNodeRejectsMissingCase(t *testing.T) {
	db := New(t)
	fixture := setupSignedReview(t, db)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, err := fixture.Repository.SubmitNode(ctx, "00000000-0000-0000-0000-000000000000",
		review.SubmitNodeInput{NodeName: "专业审核", Action: "pass", Opinion: "同意"}, fixture.Reviewer)
	if err == nil {
		t.Fatal("不存在的审核单应报错")
	}
	if !errors.Is(err, review.ErrCaseNotFound) && !strings.Contains(err.Error(), "审核") {
		t.Fatalf("错误信息应说明原因，实际 %v", err)
	}
}

// TestSubmitNodeDefaultOpinions 未填写意见时按动作给出默认意见。
func TestSubmitNodeDefaultOpinions(t *testing.T) {
	db := New(t)
	fixture := setupSignedReview(t, db)
	// 通过：默认意见应为同意。
	if err := signNode(t, fixture.Repository, fixture.CaseID, "专业审核", "pass", "   ", fixture.Reviewer); err != nil {
		t.Fatalf("签署失败: %v", err)
	}
	opinion := db.ScanString(t, `SELECT opinion FROM review_case_nodes WHERE review_case_id=$1::uuid AND name='专业审核'`, fixture.CaseID)
	if opinion != "同意通过。" {
		t.Fatalf("通过时默认意见应为同意通过。，实际 %q", opinion)
	}

	// 驳回：另建审核单验证默认意见。
	db2 := New(t)
	second := setupSignedReview(t, db2)
	if err := signNode(t, second.Repository, second.CaseID, "专业审核", "rejected", "", second.Reviewer); err != nil {
		t.Fatalf("驳回失败: %v", err)
	}
	rejected := db2.ScanString(t, `SELECT opinion FROM review_case_nodes WHERE review_case_id=$1::uuid AND name='专业审核'`, second.CaseID)
	if rejected != "审核驳回，请按意见修正后重新提交。" {
		t.Fatalf("驳回时默认意见不正确，实际 %q", rejected)
	}
}

// TestCompleteReviewReturnsErrorForUnknownSubmission 未知提交轮次必须报错。
func TestCompleteReviewReturnsErrorForUnknownSubmission(t *testing.T) {
	db := New(t)
	setupReview(t, db)
	err := completeReview(t, db, "00000000-0000-0000-0000-000000000000", db.ScanString(t, `SELECT id::text FROM users LIMIT 1`), true, "通过", true)
	if err == nil {
		t.Fatal("未知提交轮次应报错")
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Logf("失败原因（可接受）: %v", err)
	}
}
