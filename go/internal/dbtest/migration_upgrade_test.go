package dbtest

import (
	"context"
	"testing"
)

// 000041 是生命周期与正式版本发布的地基。除了不可变性约束，它还包含一次性的
// 老数据修正和正式版本发布函数。本文件验证升级路径与发布语义：这些逻辑在全新
// schema 上不会产生可观察效果，必须构造“升级前”的数据再执行迁移。

const migration041 = "migrations/000041_lifecycle_and_patents.sql"

// TestUpgradeReturnsLegacyPendingVerifyToExecuting 升级必须让旧版待验收工单重新可提交。
//
// 旧版工单停留在 pending_verify 且没有审核单，升级后无法进入完整审核流程。
// 迁移把它们退回 executing 并留下退回记录，此处验证该修正真实生效。
func TestUpgradeReturnsLegacyPendingVerifyToExecuting(t *testing.T) {
	db := NewBefore(t, migration041)
	ctx := context.Background()

	// 构造升级前状态：一个 pending_verify 工单，带一轮待处理提交，且没有审核单。
	var author string
	if err := db.Pool.QueryRow(ctx, `INSERT INTO users(account,display_name,password_hash,status) VALUES('legacy','旧设计员','x','active') RETURNING id::text`).Scan(&author); err != nil {
		t.Fatal(err)
	}
	drawing := insertLegacyDrawing(t, db, author)
	var request string
	if err := db.Pool.QueryRow(ctx, `INSERT INTO change_requests(request_no,drawing_id,drawing_no,title,reason,scope,base_drawing_revision,status,require_verify,applicant_id,executor_id)
		VALUES('CR-LEGACY',$1::uuid,'D-L','','历史变更原因','part',1,'pending_verify',false,$2::uuid,$2::uuid) RETURNING id::text`, drawing, author).Scan(&request); err != nil {
		t.Fatal(err)
	}
	var submission string
	if err := db.Pool.QueryRow(ctx, `INSERT INTO change_request_submissions(request_id,round,actual_changes,status)
		VALUES($1::uuid,1,'历史实际变更','pending') RETURNING id::text`, request).Scan(&submission); err != nil {
		t.Fatal(err)
	}
	db.Exec(t, `UPDATE change_requests SET current_submission_id=$2::uuid WHERE id=$1::uuid`, request, submission)

	db.ApplyFrom(t, migration041)

	status := db.ScanString(t, `SELECT status FROM change_requests WHERE id=$1::uuid`, request)
	if status != "executing" {
		t.Fatalf("旧工单应退回 executing 以便重新提交，实际 %s", status)
	}
	requireVerify := db.ScanString(t, `SELECT require_verify::text FROM change_requests WHERE id=$1::uuid`, request)
	if requireVerify != "true" {
		t.Fatalf("升级后必须要求完整审核，实际 require_verify=%s", requireVerify)
	}
	submissionStatus := db.ScanString(t, `SELECT status FROM change_request_submissions WHERE id=$1::uuid`, submission)
	if submissionStatus != "returned" {
		t.Fatalf("旧提交轮次应标记为已退回，实际 %s", submissionStatus)
	}
	// 必须留下可审计的退回记录，否则用户不知道工单为何被退回。
	actions := db.ScanString(t, `SELECT count(*)::text FROM change_request_actions WHERE request_id=$1::uuid AND action='return'`, request)
	if actions != "1" {
		t.Fatalf("应写入一条退回记录，实际 %s 条", actions)
	}
	// 原有轮次必须保留，不能被重建替换。
	round := db.ScanString(t, `SELECT round::text FROM change_request_submissions WHERE id=$1::uuid`, submission)
	if round != "1" {
		t.Fatalf("原轮次应保留，实际 %s", round)
	}
}

// TestUpgradeLeavesInFlightApprovalUntouched 已经在审批中的工单不受升级影响。
//
// 迁移只修正 pending_verify 且无审核单的工单；扩大范围会破坏正在进行的审批。
func TestUpgradeLeavesInFlightApprovalUntouched(t *testing.T) {
	db := NewBefore(t, migration041)
	ctx := context.Background()
	var author string
	if err := db.Pool.QueryRow(ctx, `INSERT INTO users(account,display_name,password_hash,status) VALUES('inflight','审批中','x','active') RETURNING id::text`).Scan(&author); err != nil {
		t.Fatal(err)
	}
	drawing := insertLegacyDrawing(t, db, author)
	var request string
	if err := db.Pool.QueryRow(ctx, `INSERT INTO change_requests(request_no,drawing_id,drawing_no,title,reason,scope,base_drawing_revision,status,applicant_id,executor_id,approver_opinion)
		VALUES('CR-INFLIGHT',$1::uuid,'D-I','','原因','part',1,'pending_approval',$2::uuid,$2::uuid,'已审阅') RETURNING id::text`, drawing, author).Scan(&request); err != nil {
		t.Fatal(err)
	}

	db.ApplyFrom(t, migration041)

	status := db.ScanString(t, `SELECT status FROM change_requests WHERE id=$1::uuid`, request)
	if status != "pending_approval" {
		t.Fatalf("审批中的工单状态不应被改写，实际 %s", status)
	}
	opinion := db.ScanString(t, `SELECT approver_opinion FROM change_requests WHERE id=$1::uuid`, request)
	if opinion != "已审阅" {
		t.Fatalf("审批意见不应被改写，实际 %s", opinion)
	}
}

// TestUpgradeBackfillsReleaseSnapshotForArchivedDrawings 归档图纸在升级时补齐发布快照。
func TestUpgradeBackfillsReleaseSnapshotForArchivedDrawings(t *testing.T) {
	db := NewBefore(t, migration041)
	ctx := context.Background()
	var author string
	if err := db.Pool.QueryRow(ctx, `INSERT INTO users(account,display_name,password_hash,status) VALUES('arch','归档员','x','active') RETURNING id::text`).Scan(&author); err != nil {
		t.Fatal(err)
	}
	drawing := insertLegacyDrawing(t, db, author)
	db.Exec(t, `UPDATE drawings SET status='archived' WHERE id=$1::uuid`, drawing)

	db.ApplyFrom(t, migration041)

	source := db.ScanString(t, `SELECT source FROM drawing_release_snapshots WHERE drawing_id=$1::uuid`, drawing)
	if source != "历史在用版本导入快照" {
		t.Fatalf("归档图纸应补齐导入快照，实际来源 %s", source)
	}
}

// TestUpgradeFlowNameSnapshotIsBackfilled 审核单的流程名快照必须从流程表回填。
func TestUpgradeFlowNameSnapshotIsBackfilled(t *testing.T) {
	db := NewBefore(t, migration041)
	ctx := context.Background()
	var author string
	if err := db.Pool.QueryRow(ctx, `INSERT INTO users(account,display_name,password_hash,status) VALUES('snap','快照员','x','active') RETURNING id::text`).Scan(&author); err != nil {
		t.Fatal(err)
	}
	drawing := insertLegacyDrawing(t, db, author)
	var flow string
	if err := db.Pool.QueryRow(ctx, `INSERT INTO review_flows(name,enabled,created_by) VALUES('历史审核流程',true,$1::uuid) RETURNING id::text`, author).Scan(&flow); err != nil {
		t.Fatal(err)
	}
	var caseID string
	if err := db.Pool.QueryRow(ctx, `INSERT INTO review_cases(drawing_id,flow_id,status,initiator_id) VALUES($1::uuid,$2::uuid,'reviewing',$3::uuid) RETURNING id::text`, drawing, flow, author).Scan(&caseID); err != nil {
		t.Fatal(err)
	}

	db.ApplyFrom(t, migration041)

	snapshot := db.ScanString(t, `SELECT flow_name_snapshot FROM review_cases WHERE id=$1::uuid`, caseID)
	if snapshot != "历史审核流程" {
		t.Fatalf("流程名快照应回填为历史审核流程，实际 %q", snapshot)
	}
	// 回填后流程名变更不应影响已存在的审核单。
	db.Exec(t, `UPDATE review_flows SET name='流程已改名' WHERE id=$1::uuid`, flow)
	snapshot = db.ScanString(t, `SELECT flow_name_snapshot FROM review_cases WHERE id=$1::uuid`, caseID)
	if snapshot != "历史审核流程" {
		t.Fatalf("快照不应跟随流程改名，实际 %q", snapshot)
	}
}

// insertPartAttachment 为零件建立附件。attachments 有 drawing_id 与 part_id 互斥
// 约束，因此零件附件必须是独立记录，不能复用图纸附件。
func insertPartAttachment(t *testing.T, db *DB, fixture Fixture, version string) (attachmentID, versionID string) {
	t.Helper()
	ctx := context.Background()
	if err := db.Pool.QueryRow(ctx, `INSERT INTO attachments(part_id,logical_name,file_role,uploaded_by)
		VALUES($1::uuid,'P-1.dwg','part',$2::uuid) RETURNING id::text`, fixture.Part, fixture.Author).Scan(&attachmentID); err != nil {
		t.Fatalf("插入零件附件失败: %v", err)
	}
	if err := db.Pool.QueryRow(ctx, `INSERT INTO attachment_versions(attachment_id,version,original_name,blob_id,version_kind)
		VALUES($1::uuid,$2,'P-1.dwg',$3::uuid,'working') RETURNING id::text`, attachmentID, version, fixture.Blob).Scan(&versionID); err != nil {
		t.Fatalf("插入零件版本失败: %v", err)
	}
	db.Exec(t, `UPDATE attachments SET current_version_id=$2::uuid WHERE id=$1::uuid`, attachmentID, versionID)
	return attachmentID, versionID
}

// publishTarget 把零件附件登记为某个变更单当前轮次的提交目标。
func publishTarget(t *testing.T, db *DB, fixture Fixture, requestNo, version string) {
	t.Helper()
	attachment, versionID := insertPartAttachment(t, db, fixture, version)
	request := insertChangeRequest(t, db, fixture, requestNo)
	submission := insertSubmission(t, db, request, 1)
	db.Exec(t, `UPDATE change_requests SET current_submission_id=$2::uuid WHERE id=$1::uuid`, request, submission)
	db.Exec(t, `INSERT INTO change_request_submission_targets(submission_id,attachment_id,base_attachment_version_id,submitted_attachment_version_id)
		VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid)`, submission, attachment, versionID, versionID)
}

// insertBaseRevision 建立零件的初始正式修订，作为发布基线。
func insertBaseRevision(t *testing.T, db *DB, fixture Fixture) string {
	t.Helper()
	var id string
	if err := db.Pool.QueryRow(context.Background(), `INSERT INTO part_revisions(part_id,revision_no,version,name,material,spec,weight,surface_treatment,part_type,workflow_status,created_by)
		VALUES($1::uuid,1,'V1','零件甲','45钢','φ10',1.5,'发黑','自制件','published',$2::uuid) RETURNING id::text`, fixture.Part, fixture.Author).Scan(&id); err != nil {
		t.Fatalf("插入基础修订失败: %v", err)
	}
	db.Exec(t, `UPDATE parts SET published_revision_id=$2::uuid WHERE id=$1::uuid`, fixture.Part, id)
	return id
}

// TestPublishChangedPartRevisionsCreatesNextRevision 发布变更零件时递增修订号并切换正式修订。
func TestPublishChangedPartRevisionsCreatesNextRevision(t *testing.T) {
	db := New(t)
	fixture := db.Seed(t)
	baseRevision := insertBaseRevision(t, db, fixture)
	publishTarget(t, db, fixture, "CR-PUB", "v1.0")

	request := db.ScanString(t, `SELECT id::text FROM change_requests WHERE request_no='CR-PUB'`)
	db.Exec(t, `SELECT publish_changed_part_revisions($1::uuid,$2::uuid)`, request, fixture.Author)

	// 必须产生下一个修订号（2），并成为零件当前正式修订。
	nextNo := db.ScanString(t, `SELECT revision_no::text FROM part_revisions WHERE part_id=$1::uuid ORDER BY revision_no DESC LIMIT 1`, fixture.Part)
	if nextNo != "2" {
		t.Fatalf("应发布修订 2，实际 %s", nextNo)
	}
	published := db.ScanString(t, `SELECT pr.revision_no::text FROM parts p JOIN part_revisions pr ON pr.id=p.published_revision_id WHERE p.id=$1::uuid`, fixture.Part)
	if published != "2" {
		t.Fatalf("published_revision_id 应指向新修订，实际 %s", published)
	}
	workflow := db.ScanString(t, `SELECT workflow_status FROM part_revisions WHERE part_id=$1::uuid ORDER BY revision_no DESC LIMIT 1`, fixture.Part)
	if workflow != "published" {
		t.Fatalf("新修订应为已发布状态，实际 %s", workflow)
	}
	// 新修订必须记录来源，便于追溯改自哪一版。
	basedOn := db.ScanString(t, `SELECT based_on_revision_id::text FROM part_revisions WHERE part_id=$1::uuid ORDER BY revision_no DESC LIMIT 1`, fixture.Part)
	if basedOn != baseRevision {
		t.Fatalf("新修订应基于原正式修订，期望 %s 实际 %s", baseRevision, basedOn)
	}
	// 附件必须继承到新修订，否则正式版本会丢失文件。
	inherited := db.ScanString(t, `SELECT count(*)::text FROM part_revision_attachments pra JOIN part_revisions pr ON pr.id=pra.part_revision_id
		WHERE pr.part_id=$1::uuid AND pr.revision_no=2`, fixture.Part)
	if inherited != "1" {
		t.Fatalf("新修订应继承 1 个附件，实际 %s", inherited)
	}
}

// TestPublishChangedPartRevisionsRequiresBaseline 缺少基础修订时发布必须失败而不是写入错误数据。
func TestPublishChangedPartRevisionsRequiresBaseline(t *testing.T) {
	db := New(t)
	fixture := db.Seed(t)
	// 不建立任何 part_revisions，直接发布。
	publishTarget(t, db, fixture, "CR-NOBASE", "v1.0")
	request := db.ScanString(t, `SELECT id::text FROM change_requests WHERE request_no='CR-NOBASE'`)

	db.MustFail(t, `SELECT publish_changed_part_revisions($1::uuid,$2::uuid)`, request, fixture.Author)

	// 失败后不应留下半成品修订。
	count := db.ScanString(t, `SELECT count(*)::text FROM part_revisions WHERE part_id=$1::uuid`, fixture.Part)
	if count != "0" {
		t.Fatalf("发布失败不应写入修订，实际 %s 条", count)
	}
}

// TestPublishChangedPartRevisionsIsNotIdempotentByItself 该函数每次调用都会发布新修订，
// 不具幂等性；重复发布只能靠审核状态阻止。此处固定该行为，防止被误认为可安全重试。
func TestPublishChangedPartRevisionsIsNotIdempotentByItself(t *testing.T) {
	db := New(t)
	fixture := db.Seed(t)
	insertBaseRevision(t, db, fixture)
	publishTarget(t, db, fixture, "CR-TWICE", "v1.0")
	request := db.ScanString(t, `SELECT id::text FROM change_requests WHERE request_no='CR-TWICE'`)

	db.Exec(t, `SELECT publish_changed_part_revisions($1::uuid,$2::uuid)`, request, fixture.Author)
	db.Exec(t, `SELECT publish_changed_part_revisions($1::uuid,$2::uuid)`, request, fixture.Author)

	maxNo := db.ScanString(t, `SELECT max(revision_no)::text FROM part_revisions WHERE part_id=$1::uuid`, fixture.Part)
	if maxNo != "3" {
		t.Fatalf("函数本身会重复发布，期望修订号到 3，实际 %s", maxNo)
	}
}

// TestArchiveCapturesReleaseSnapshot 归档动作自动捕获发布快照。
func TestArchiveCapturesReleaseSnapshot(t *testing.T) {
	db := New(t)
	fixture := db.Seed(t)
	// 非归档状态不应产生快照。
	db.Exec(t, `UPDATE drawings SET status='published' WHERE id=$1::uuid`, fixture.Drawing)
	count := db.ScanString(t, `SELECT count(*)::text FROM drawing_release_snapshots WHERE drawing_id=$1::uuid`, fixture.Drawing)
	if count != "0" {
		t.Fatalf("未归档不应产生发布快照，实际 %s 条", count)
	}
	db.Exec(t, `UPDATE drawings SET version='v2.0' WHERE id=$1::uuid`, fixture.Drawing)
	count = db.ScanString(t, `SELECT count(*)::text FROM drawing_release_snapshots WHERE drawing_id=$1::uuid`, fixture.Drawing)
	if count != "0" {
		t.Fatalf("仅改版本未归档不应产生快照，实际 %s 条", count)
	}

	// 归档时自动捕获，且快照版本必须等于归档时的图纸版本。
	db.Exec(t, `UPDATE drawings SET status='archived' WHERE id=$1::uuid`, fixture.Drawing)
	current := db.ScanString(t, `SELECT version FROM drawings WHERE id=$1::uuid`, fixture.Drawing)
	source := db.ScanString(t, `SELECT source FROM drawing_release_snapshots WHERE drawing_id=$1::uuid`, fixture.Drawing)
	if source != "正式发布" {
		t.Fatalf("归档应自动捕获快照，实际来源 %q", source)
	}
	version := db.ScanString(t, `SELECT version FROM drawing_release_snapshots WHERE drawing_id=$1::uuid`, fixture.Drawing)
	if version != current {
		t.Fatalf("快照版本应等于归档时版本 %q，实际 %q", current, version)
	}
}

// insertLegacyDrawing 在只跑到 000041 之前的 schema 上建立图纸。
// 该阶段的 drawings 表结构与最终结构一致，因此可直接复用。
func insertLegacyDrawing(t *testing.T, db *DB, author string) string {
	t.Helper()
	var id string
	if err := db.Pool.QueryRow(context.Background(), `INSERT INTO drawings(drawing_no,name,project,created_by,status) VALUES('D-L','历史图纸','P',$1::uuid,'draft') RETURNING id::text`, author).Scan(&id); err != nil {
		t.Fatalf("插入历史图纸失败: %v", err)
	}
	return id
}
