package dbtest

import (
	"context"
	"strings"
	"testing"
)

// 本文件验证迁移 000041 建立的不可变性约束。这些触发器保证历史证据、提交内容
// 与已签署节点不可被改写，是变更流程可审计的地基；一旦失效，错误数据无法被发现，
// 也无法回滚。

// insertDocument 插入一条归档资料，返回 id。
func insertDocument(t *testing.T, db *DB, fixture Fixture, category, title string) string {
	t.Helper()
	var id string
	err := db.Pool.QueryRow(context.Background(), `INSERT INTO lifecycle_documents(drawing_id,category,title,storage_key,file_name,mime_type,size_bytes,sha256,created_by)
		VALUES($1::uuid,$2,$3,$4,'a.pdf','application/pdf',8,repeat('b',64),$5::uuid) RETURNING id::text`,
		fixture.Drawing, category, title, "docs/"+title, fixture.Author).Scan(&id)
	if err != nil {
		t.Fatalf("插入归档资料失败: %v", err)
	}
	return id
}

func insertPatent(t *testing.T, db *DB, fixture Fixture) string {
	t.Helper()
	var id string
	err := db.Pool.QueryRow(context.Background(), `INSERT INTO patent_records(number,title,responsible_id,reminder_days)
		VALUES('CN-1','测试专利',$1::uuid,90) RETURNING id::text`, fixture.Reviewer).Scan(&id)
	if err != nil {
		t.Fatalf("插入专利失败: %v", err)
	}
	return id
}

// insertChangeRequest 建立一张处于执行中的变更单。
func insertChangeRequest(t *testing.T, db *DB, fixture Fixture, requestNo string) string {
	t.Helper()
	var id string
	err := db.Pool.QueryRow(context.Background(), `INSERT INTO change_requests(request_no,drawing_id,drawing_no,reason,scope,base_drawing_revision,applicant_id,executor_id,status)
		VALUES($1,$2::uuid,'D-1','配合装配调整','part',1,$3::uuid,$3::uuid,'executing') RETURNING id::text`,
		requestNo, fixture.Drawing, fixture.Author).Scan(&id)
	if err != nil {
		t.Fatalf("插入变更单失败: %v", err)
	}
	return id
}

// insertSubmission 建立一轮提交快照。
func insertSubmission(t *testing.T, db *DB, request string, round int) string {
	t.Helper()
	var id string
	err := db.Pool.QueryRow(context.Background(), `INSERT INTO change_request_submissions(request_id,round,actual_changes,status)
		VALUES($1::uuid,$2,'实际变更内容','pending') RETURNING id::text`, request, round).Scan(&id)
	if err != nil {
		t.Fatalf("插入提交轮次失败: %v", err)
	}
	return id
}

// insertFlow 建立一个启用的完整审核流程。
func insertFlow(t *testing.T, db *DB, fixture Fixture) string {
	t.Helper()
	var id string
	err := db.Pool.QueryRow(context.Background(), `INSERT INTO review_flows(name,enabled,created_by) VALUES('完整审核',true,$1::uuid) RETURNING id::text`, fixture.Author).Scan(&id)
	if err != nil {
		t.Fatalf("插入审核流程失败: %v", err)
	}
	return id
}

// TestLifecycleDocumentsAreAppendOnly 归档资料一旦写入即不可修改或删除。
func TestLifecycleDocumentsAreAppendOnly(t *testing.T) {
	db := New(t)
	fixture := db.Seed(t)
	id := insertDocument(t, db, fixture, "设计输入", "原始需求")

	db.MustFail(t, `UPDATE lifecycle_documents SET title='被改写' WHERE id=$1::uuid`, id)
	db.MustFail(t, `DELETE FROM lifecycle_documents WHERE id=$1::uuid`, id)

	// 未篡改的记录必须仍在，证明失败的是触发器而不是外键或权限。
	var title string
	if err := db.Pool.QueryRow(context.Background(), `SELECT title FROM lifecycle_documents WHERE id=$1::uuid`, id).Scan(&title); err != nil {
		t.Fatalf("记录应仍然存在: %v", err)
	}
	if title != "原始需求" {
		t.Fatalf("标题被改写: %s", title)
	}
}

// TestPatentEventsAreAppendOnly 专利事件（含提醒与缴费）不可改写。
func TestPatentEventsAreAppendOnly(t *testing.T) {
	db := New(t)
	fixture := db.Seed(t)
	patent := insertPatent(t, db, fixture)
	db.Exec(t, `INSERT INTO patent_events(patent_id,actor_id,action,detail) VALUES($1::uuid,$2::uuid,'create','{}'::jsonb)`, patent, fixture.Author)

	db.MustFail(t, `UPDATE patent_events SET action='update' WHERE patent_id=$1::uuid`, patent)
	db.MustFail(t, `DELETE FROM patent_events WHERE patent_id=$1::uuid`, patent)
}

// TestSubmissionContentIsFrozenButStatusFlows 提交内容冻结，但状态必须仍可流转。
//
// 触发器用 to_jsonb 差集判断，容易误伤 status 列，导致流程卡死；这里同时验证两面。
func TestSubmissionContentIsFrozenButStatusFlows(t *testing.T) {
	db := New(t)
	fixture := db.Seed(t)
	request := insertChangeRequest(t, db, fixture, "CR-1")
	submission := insertSubmission(t, db, request, 1)

	// 已提交内容不可覆盖。
	db.MustFail(t, `UPDATE change_request_submissions SET actual_changes='偷偷改内容' WHERE id=$1::uuid`, submission)
	// 轮次永久保留。
	db.MustFail(t, `DELETE FROM change_request_submissions WHERE id=$1::uuid`, submission)
	// 状态流转必须可用，否则审核无法推进。
	db.Exec(t, `UPDATE change_request_submissions SET status='returned' WHERE id=$1::uuid`, submission)
	var status string
	if err := db.Pool.QueryRow(context.Background(), `SELECT status FROM change_request_submissions WHERE id=$1::uuid`, submission).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "returned" {
		t.Fatalf("状态未更新: %s", status)
	}
}

// TestSignedReviewNodesAreLockedButPendingEditable 已签署节点锁定，待签节点可写。
//
// 区分这两者是关键：全锁会卡死流程，不锁则签名可被伪造或推翻。
func TestSignedReviewNodesAreLockedButPendingEditable(t *testing.T) {
	db := New(t)
	fixture := db.Seed(t)
	flow := insertFlow(t, db, fixture)
	var caseID string
	if err := db.Pool.QueryRow(context.Background(), `INSERT INTO review_cases(drawing_id,flow_id,status,initiator_id) VALUES($1::uuid,$2::uuid,'reviewing',$3::uuid) RETURNING id::text`,
		fixture.Drawing, flow, fixture.Author).Scan(&caseID); err != nil {
		t.Fatalf("插入审核单失败: %v", err)
	}
	// 节点名在同一审核单内唯一，两个节点必须使用不同名称。
	makeNode := func(name, status string, order int) string {
		var id string
		if err := db.Pool.QueryRow(context.Background(), `INSERT INTO review_case_nodes(review_case_id,name,assigned_user_id,assigned_name,status,opinion,required,node_order)
			VALUES($1::uuid,$2,$3::uuid,'审核员',$4,'',true,$5) RETURNING id::text`, caseID, name, fixture.Reviewer, status, order).Scan(&id); err != nil {
			t.Fatalf("插入审核节点失败: %v", err)
		}
		return id
	}
	signed := makeNode("校对复核", "pass", 1)
	pending := makeNode("专业审核", "pending", 2)

	// 已签署节点不可删除，也不可改任何字段（含意见与签署人）。
	db.MustFail(t, `DELETE FROM review_case_nodes WHERE id=$1::uuid`, signed)
	db.MustFail(t, `UPDATE review_case_nodes SET opinion='事后改写意见' WHERE id=$1::uuid`, signed)
	db.MustFail(t, `UPDATE review_case_nodes SET status='pending' WHERE id=$1::uuid`, signed)
	db.MustFail(t, `UPDATE review_case_nodes SET assigned_user_id=$2::uuid WHERE id=$1::uuid`, signed, fixture.Author)

	// 待签节点必须可以正常签署。
	db.Exec(t, `UPDATE review_case_nodes SET status='pass',opinion='同意',reviewed_at=now() WHERE id=$1::uuid`, pending)
}

// TestReviewCaseChangeSubmissionIsUnique 一个提交轮次只能对应一个审核单，防止重复发布。
func TestReviewCaseChangeSubmissionIsUnique(t *testing.T) {
	db := New(t)
	fixture := db.Seed(t)
	flow := insertFlow(t, db, fixture)
	request := insertChangeRequest(t, db, fixture, "CR-UQ")
	submission := insertSubmission(t, db, request, 1)
	db.Exec(t, `INSERT INTO review_cases(drawing_id,flow_id,status,initiator_id,change_submission_id) VALUES($1::uuid,$2::uuid,'reviewing',$3::uuid,$4::uuid)`,
		fixture.Drawing, flow, fixture.Author, submission)

	// 第二次插入必须被唯一索引拒绝：否则同一轮次可能被发布两次。
	db.MustFail(t, `INSERT INTO review_cases(drawing_id,flow_id,status,initiator_id,change_submission_id) VALUES($1::uuid,$2::uuid,'reviewing',$3::uuid,$4::uuid)`,
		fixture.Drawing, flow, fixture.Author, submission)

	// 唯一性是针对提交轮次本身的全局约束：同一轮次不能出现在任何第二张审核单上，
	// 即使换了图纸也不行。
	var second string
	if err := db.Pool.QueryRow(context.Background(), `INSERT INTO drawings(drawing_no,name,project,created_by,status) VALUES('D-2','另一图纸','P',$1::uuid,'draft') RETURNING id::text`, fixture.Author).Scan(&second); err != nil {
		t.Fatal(err)
	}
	db.MustFail(t, `INSERT INTO review_cases(drawing_id,flow_id,status,initiator_id,change_submission_id) VALUES($1::uuid,$2::uuid,'reviewing',$3::uuid,$4::uuid)`,
		second, flow, fixture.Author, submission)

	// 不关联提交轮次（NULL）的审核单不受该唯一约束限制，可以并存。
	db.Exec(t, `INSERT INTO review_cases(drawing_id,flow_id,status,initiator_id) VALUES($1::uuid,$2::uuid,'reviewing',$3::uuid)`, second, flow, fixture.Author)
}

// TestDrawingReleaseSnapshotIsImmutableAndIdempotent 发布快照不可改写，重复捕获不覆盖。
func TestDrawingReleaseSnapshotIsImmutableAndIdempotent(t *testing.T) {
	db := New(t)
	fixture := db.Seed(t)
	db.Exec(t, `SELECT capture_drawing_release($1::uuid,'正式发布')`, fixture.Drawing)
	// 同一版本重复捕获必须忽略，不能覆盖既有快照。
	db.Exec(t, `SELECT capture_drawing_release($1::uuid,'第二次来源')`, fixture.Drawing)

	var count int
	var source string
	if err := db.Pool.QueryRow(context.Background(), `SELECT count(*),(SELECT source FROM drawing_release_snapshots WHERE drawing_id=$1::uuid LIMIT 1) FROM drawing_release_snapshots WHERE drawing_id=$1::uuid`, fixture.Drawing).Scan(&count, &source); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("快照应只保留一份，实际 %d", count)
	}
	if source != "正式发布" {
		t.Fatalf("快照来源被覆盖: %s", source)
	}

	snapshotID := db.ScanString(t, `SELECT id::text FROM drawing_release_snapshots WHERE drawing_id=$1::uuid`, fixture.Drawing)
	db.MustFail(t, `UPDATE drawing_release_snapshots SET source='改写' WHERE id=$1::uuid`, snapshotID)
	db.MustFail(t, `DELETE FROM drawing_release_snapshots WHERE id=$1::uuid`, snapshotID)

	// 快照必须包含图纸与文件结构，否则审计信息不完整。
	detail := db.ScanString(t, `SELECT snapshot::text FROM drawing_release_snapshots WHERE id=$1::uuid`, snapshotID)
	for _, key := range []string{"drawing", "files", "structure", "signers", "bom"} {
		if !strings.Contains(detail, `"`+key+`"`) {
			t.Fatalf("快照缺少字段 %s: %s", key, detail)
		}
	}
}

// TestSubmissionDocumentLinkIsImmutable 提交与资料的关联同样是历史证据。
func TestSubmissionDocumentLinkIsImmutable(t *testing.T) {
	db := New(t)
	fixture := db.Seed(t)
	document := insertDocument(t, db, fixture, "变更材料", "变更前图纸")
	request := insertChangeRequest(t, db, fixture, "CR-DOC")
	submission := insertSubmission(t, db, request, 1)
	db.Exec(t, `INSERT INTO change_submission_documents(submission_id,document_id) VALUES($1::uuid,$2::uuid)`, submission, document)
	db.MustFail(t, `DELETE FROM change_submission_documents WHERE submission_id=$1::uuid`, submission)
	db.MustFail(t, `UPDATE change_submission_documents SET document_id=(SELECT id FROM lifecycle_documents LIMIT 1) WHERE submission_id=$1::uuid`, submission)
}

// TestLifecycleDocumentRequiresExactlyOneOwner 资料必须且只能归属图纸或专利之一。
func TestLifecycleDocumentRequiresExactlyOneOwner(t *testing.T) {
	db := New(t)
	fixture := db.Seed(t)
	patent := insertPatent(t, db, fixture)

	// 两者都为空。
	db.MustFail(t, `INSERT INTO lifecycle_documents(category,title,storage_key,file_name,mime_type,size_bytes,sha256,created_by)
		VALUES('分类','标题','k1','a.pdf','application/pdf',8,repeat('c',64),$1::uuid)`, fixture.Author)
	// 两者同时给出。
	db.MustFail(t, `INSERT INTO lifecycle_documents(drawing_id,patent_id,category,title,storage_key,file_name,mime_type,size_bytes,sha256,created_by)
		VALUES($1::uuid,$2::uuid,'分类','标题','k2','a.pdf','application/pdf',8,repeat('c',64),$3::uuid)`, fixture.Drawing, patent, fixture.Author)
	// 单独归属专利可用。
	db.Exec(t, `INSERT INTO lifecycle_documents(patent_id,category,title,storage_key,file_name,mime_type,size_bytes,sha256,created_by)
		VALUES($1::uuid,'专利证书','受理通知书','k3','a.pdf','application/pdf',8,repeat('c',64),$2::uuid)`, patent, fixture.Author)
}

// TestHistoryGuardProtectsReleasedVersions 正式版本、提交快照与原始版本永久保留。
func TestHistoryGuardProtectsReleasedVersions(t *testing.T) {
	db := New(t)
	fixture := db.Seed(t)
	var version string
	if err := db.Pool.QueryRow(context.Background(), `INSERT INTO attachment_versions(attachment_id,version,original_name,blob_id,release_number)
		VALUES($1::uuid,'v1.0','D-1.dwg',$2::uuid,1) RETURNING id::text`, fixture.Attachment, fixture.Blob).Scan(&version); err != nil {
		t.Fatalf("插入正式版本失败: %v", err)
	}
	db.Exec(t, `UPDATE attachments SET current_version_id=$2::uuid WHERE id=$1::uuid`, fixture.Attachment, version)

	// 保护只针对“内容变更”，因此必须用不同的 blob 才能验证触发器真的生效；
	// 写入相同值会被 IS DISTINCT FROM 判定为未变更而放行。
	var otherBlob string
	if err := db.Pool.QueryRow(context.Background(), `INSERT INTO file_blobs(storage_key,mime_type,size_bytes,sha256) VALUES('blobs/other','application/octet-stream',8,repeat('d',64)) RETURNING id::text`).Scan(&otherBlob); err != nil {
		t.Fatal(err)
	}
	db.MustFail(t, `UPDATE attachment_versions SET blob_id=$2::uuid WHERE id=$1::uuid`, version, otherBlob)
	db.MustFail(t, `UPDATE attachment_versions SET deleted_at=now() WHERE id=$1::uuid`, version)
	db.MustFail(t, `UPDATE attachment_versions SET original_name='改名.dwg' WHERE id=$1::uuid`, version)
	// 正式版本不可删除。
	db.MustFail(t, `DELETE FROM attachment_versions WHERE id=$1::uuid`, version)
	// 未触及受保护列的元数据更新必须仍允许，否则版本管理无法运转。
	db.Exec(t, `UPDATE attachment_versions SET version='v2.0' WHERE id=$1::uuid`, version)

	// 有正式版本的图纸不可删除，也不可被隐藏。
	db.MustFail(t, `DELETE FROM attachments WHERE id=$1::uuid`, fixture.Attachment)
	db.MustFail(t, `UPDATE attachments SET deleted_at=now() WHERE id=$1::uuid`, fixture.Attachment)
}

// TestPruneAttachmentWorkKeepsWorkingDrafts 工作稿是历史证据，不再被发布动作清除。
//
// 迁移把该函数替换为空实现属于行为反转；若未生效，历史工作稿会被静默清理。
func TestPruneAttachmentWorkKeepsWorkingDrafts(t *testing.T) {
	db := New(t)
	fixture := db.Seed(t)
	var version string
	if err := db.Pool.QueryRow(context.Background(), `INSERT INTO attachment_versions(attachment_id,version,original_name,blob_id,version_kind)
		VALUES($1::uuid,'v1.0','D-1-work.dwg',$2::uuid,'working') RETURNING id::text`, fixture.Attachment, fixture.Blob).Scan(&version); err != nil {
		t.Fatalf("插入工作稿失败: %v", err)
	}
	db.Exec(t, `SELECT prune_attachment_work($1::uuid)`, fixture.Attachment)

	var exists bool
	if err := db.Pool.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM attachment_versions WHERE id=$1::uuid AND deleted_at IS NULL)`, version).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("工作稿被清除了，prune_attachment_work 必须为空操作")
	}
}
