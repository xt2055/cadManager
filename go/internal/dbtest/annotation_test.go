package dbtest

import (
	"context"
	"errors"
	"testing"

	"cadguanliq/internal/annotation"
	"cadguanliq/internal/auth"
	"cadguanliq/internal/change"
	"cadguanliq/internal/review"
)


func TestAnnotationsPinVersionAndLockAfterSigning(t *testing.T) {
	db := New(t)
	f := setupSignedReview(t, db)
	ctx := context.Background()
	repo := annotation.NewRepository(db.Pool)
	attachmentID := db.ScanString(t, `SELECT attachment_id::text FROM attachment_versions WHERE id=$1::uuid`, f.Version)
	w, err := repo.Load(ctx, f.CaseID, attachmentID, f.Reviewer)
	if err != nil {
		t.Fatal(err)
	}
	if !w.CanEdit || w.VersionID != f.Version {
		t.Fatalf("bad scope: %+v", w)
	}
	in := annotation.SaveInput{CaseID: f.CaseID, AttachmentID: attachmentID, VersionID: w.VersionID, NodeID: w.NodeID, Content: annotation.Content{SchemaVersion: 1, Marks: []annotation.Mark{{ID: "test", Kind: "check", Layout: "Model", Points: []annotation.Point{{X: 10, Y: 20}}, Color: "#FF6868", Width: 3}}}}
	if _, err = repo.Save(ctx, in, f.Author); !errors.Is(err, annotation.ErrForbidden) {
		t.Fatalf("wrong author: %v", err)
	}
	wrong := in
	wrong.VersionID = f.Drawing
	if _, err = repo.Save(ctx, wrong, f.Reviewer); !errors.Is(err, annotation.ErrNotFound) {
		t.Fatalf("wrong version accepted: %v", err)
	}
	saved, err := repo.Save(ctx, in, f.Reviewer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.Save(ctx, in, f.Reviewer); !errors.Is(err, annotation.ErrConflict) {
		t.Fatalf("duplicate stale save: %v", err)
	}
	in.Revision = saved.Revision
	in.Content.Marks[0].Text = "缺少尺寸"
	saved, err = repo.Save(ctx, in, f.Reviewer)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Revision != 2 {
		t.Fatalf("revision=%d", saved.Revision)
	}
	if got := db.ScanString(t, `SELECT count(*)::text FROM review_annotation_events WHERE document_id=$1::uuid`, saved.ID); got != "2" {
		t.Fatal("missing history")
	}
	if _, err = db.Pool.Exec(ctx, `UPDATE attachment_versions SET deleted_at=now() WHERE id=$1::uuid`, f.Version); err == nil {
		t.Fatal("referenced version was deleted")
	}
	if err = signNode(t, f.Repository, f.CaseID, "专业审核", "rejected", "补充尺寸", f.Reviewer); err != nil {
		t.Fatal(err)
	}
	in.Revision = 2
	if _, err = repo.Save(ctx, in, f.Reviewer); !errors.Is(err, annotation.ErrForbidden) {
		t.Fatalf("signed annotation modified: %v", err)
	}
	w, err = repo.Load(ctx, f.CaseID, attachmentID, f.Author)
	if err != nil {
		t.Fatal(err)
	}
	if w.CanEdit || len(w.Documents) != 1 || w.Documents[0].Content.Marks[0].Text != "缺少尺寸" {
		t.Fatalf("history lost: %+v", w)
	}
}

func TestAnnotationTemplateOwnership(t *testing.T) {
	db := New(t)
	f := db.Seed(t)
	ctx := context.Background()
	repo := annotation.NewRepository(db.Pool)
	if _, err := repo.CreateTemplate(ctx, annotation.Template{Category: "尺寸", Text: "补充尺寸"}, f.Reviewer, false); !errors.Is(err, annotation.ErrForbidden) {
		t.Fatal("non-admin created public template")
	}
	item, err := repo.CreateTemplate(ctx, annotation.Template{OwnerID: f.Author, Category: "尺寸", Text: "补充尺寸"}, f.Reviewer, false)
	if err != nil {
		t.Fatal(err)
	}
	if item.OwnerID != f.Reviewer {
		t.Fatal("spoofed owner")
	}
	if err = repo.DeleteTemplate(ctx, item.ID, f.Author, true); !errors.Is(err, annotation.ErrForbidden) {
		t.Fatal("another user's personal template deleted")
	}
	if err = repo.DeleteTemplate(ctx, item.ID, f.Reviewer, false); err != nil {
		t.Fatal(err)
	}
}

// insertExtraPartAttachment 建立第二份零件附件（文件名不同，用于验证批注的文件归属）。
func insertExtraPartAttachment(t *testing.T, db *DB, fixture Fixture, name string) (attachmentID, versionID string) {
	t.Helper()
	ctx := context.Background()
	if err := db.Pool.QueryRow(ctx, `INSERT INTO attachments(part_id,logical_name,file_role,uploaded_by)
		VALUES($1::uuid,$2,'part',$3::uuid) RETURNING id::text`, fixture.Part, name, fixture.Author).Scan(&attachmentID); err != nil {
		t.Fatalf("插入第二份零件附件失败: %v", err)
	}
	if err := db.Pool.QueryRow(ctx, `INSERT INTO attachment_versions(attachment_id,version,original_name,blob_id,version_kind)
		VALUES($1::uuid,'v1.0',$2,$3::uuid,'working') RETURNING id::text`, attachmentID, name, fixture.Blob).Scan(&versionID); err != nil {
		t.Fatalf("插入第二份零件版本失败: %v", err)
	}
	db.Exec(t, `UPDATE attachments SET current_version_id=$2::uuid WHERE id=$1::uuid`, attachmentID, versionID)
	return attachmentID, versionID
}

// historyFixture 一轮「变更送审 → 审核 → 驳回 → 重新提交」所需的全部上下文。
type historyFixture struct {
	Fixture
	Flow       string
	Request    string
	Service    *change.PGService
	Repository *review.PGRepository
	Attachment string
	Version    string
	Second     string
	SecondVer  string
}

// setupAnnotationHistory 建立「变更送审」的完整上下文：两张图纸的工单目标与审核流程。
// 第一轮审核单由生产入口（变更提交）建立，不在测试里手搓，避免绕过真实链路。
func setupAnnotationHistory(t *testing.T, db *DB) historyFixture {
	t.Helper()
	fixture := db.Seed(t)
	flow := insertFlow(t, db, fixture)
	insertBaseRevision(t, db, fixture)
	request := insertChangeRequest(t, db, fixture, "CR-HIST")
	attachment, version := insertPartAttachment(t, db, fixture, "v1.0")
	second, secondVersion := insertExtraPartAttachment(t, db, fixture, "P-2.dwg")
	// 工单目标：修改时保存的工作版本，提交时被快照成审核批注引用的固定版本。
	for _, target := range [][2]string{{attachment, version}, {second, secondVersion}} {
		db.Exec(t, `INSERT INTO change_request_targets(request_id,attachment_id,base_attachment_version_id,work_attachment_version_id)
			VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid)`, request, target[0], target[1], target[1])
	}
	addFlowNode(t, db, flow, "校对复核", "校对", fixture.Reviewer, true, 1)
	addFlowNode(t, db, flow, "专业审核", "审核", fixture.Reviewer, true, 2)

	service := change.NewService(db.Pool)
	repository := review.NewPGRepository(db.Pool)
	repository.SetChangeCompletion(service.CompleteReview)
	if _, err := service.Submit(context.Background(), auth.AuthUser{ID: fixture.Author}, request, change.SubmitInput{ActualChanges: "第一轮修改"}); err != nil {
		t.Fatalf("第一轮变更提交失败: %v", err)
	}
	return historyFixture{
		Fixture: fixture, Flow: flow, Request: request, Repository: repository, Service: service,
		Attachment: attachment, Version: version, Second: second, SecondVer: secondVersion,
	}
}

// currentRound 返回工单当前提交轮次对应的审核单 ID。
func currentRound(t *testing.T, db *DB, request string) string {
	t.Helper()
	return db.ScanString(t, `SELECT id::text FROM review_cases WHERE change_submission_id=(SELECT current_submission_id FROM change_requests WHERE id=$1::uuid)`, request)
}

// saveHistoryMark 以当前节点责任人身份在某份文件上写入一条文字批注。
func saveHistoryMark(t *testing.T, repo *annotation.Repository, scope annotation.Workspace, attachmentID, userID, text string) {
	t.Helper()
	if _, err := repo.Save(context.Background(), annotation.SaveInput{
		CaseID: scope.CaseID, AttachmentID: attachmentID, VersionID: scope.VersionID, NodeID: scope.NodeID, Revision: 0,
		Content: annotation.Content{SchemaVersion: 1, Marks: []annotation.Mark{{
			ID: "mark-" + text, Kind: "text", Layout: "Model", Points: []annotation.Point{{X: 10, Y: 20}},
			Text: text, Color: "#FF6868", Width: 3,
		}}},
	}, userID); err != nil {
		t.Fatalf("写入批注失败: %v", err)
	}
}

// TestAnnotationHistoryKeepsFilesAndIsolatesRounds 锁定两条契约：
// 驳回轮次的批注永久可查（按审核员 + 按文件归属），而新一轮审核的画布绝不继承上一轮的批注。
func TestAnnotationHistoryKeepsFilesAndIsolatesRounds(t *testing.T) {
	db := New(t)
	fx := setupAnnotationHistory(t, db)
	ctx := context.Background()
	repo := annotation.NewRepository(db.Pool)

	round1 := currentRound(t, db, fx.Request)
	first, err := repo.Load(ctx, round1, fx.Attachment, fx.Reviewer)
	if err != nil || !first.CanEdit {
		t.Fatalf("第一轮应允许当前节点责任人批注: %v / %+v", err, first)
	}
	saveHistoryMark(t, repo, first, fx.Attachment, fx.Reviewer, "缺少尺寸")
	secondScope, err := repo.Load(ctx, round1, fx.Second, fx.Reviewer)
	if err != nil {
		t.Fatalf("读取第二份文件批注失败: %v", err)
	}
	saveHistoryMark(t, repo, secondScope, fx.Second, fx.Reviewer, "材料错误")

	// 驳回：本轮结束，批注从此只读（真归档，不是可改的历史）。
	if err := signNode(t, fx.Repository, round1, "校对复核", "rejected", "尺寸体系不完整", fx.Reviewer); err != nil {
		t.Fatalf("驳回失败: %v", err)
	}
	stale := annotation.SaveInput{
		CaseID: round1, AttachmentID: fx.Attachment, VersionID: fx.Version, NodeID: first.NodeID, Revision: 1,
		Content: annotation.Content{SchemaVersion: 1, Marks: []annotation.Mark{{ID: "late", Kind: "check", Layout: "Model", Points: []annotation.Point{{X: 1, Y: 1}}, Color: "#FF6868", Width: 3}}},
	}
	if _, err := repo.Save(ctx, stale, fx.Reviewer); !errors.Is(err, annotation.ErrForbidden) {
		t.Fatalf("驳回后批注仍被改写: %v", err)
	}

	// 第二轮：修改后重新提交，走生产入口（变更提交），确认审核单真的另起一份。
	if _, err := fx.Service.Submit(ctx, auth.AuthUser{ID: fx.Author}, fx.Request, change.SubmitInput{ActualChanges: "按审核意见修正尺寸"}); err != nil {
		t.Fatalf("第二轮变更提交失败: %v", err)
	}
	round2 := currentRound(t, db, fx.Request)
	if round2 == round1 {
		t.Fatalf("重新提交必须另起审核轮次，不能复用被驳回的审核单")
	}
	// 工作台选定「当前轮次」靠的就是这个不变量：同一图纸同时最多一个进行中的审核单。
	active := db.ScanString(t, `SELECT string_agg(id::text, ',') FROM review_cases WHERE drawing_id=$1::uuid AND status IN ('pending','reviewing')`, fx.Drawing)
	if active != round2 {
		t.Fatalf("进行中的审核单应只有新一轮 %s，实际 %s", round2, active)
	}
	// 列表按时间倒序返回，最新一轮在前，选择轮次时无需依赖分钟级时间戳。
	cases, err := fx.Repository.ListCases(ctx)
	if err != nil {
		t.Fatalf("读取审核案例失败: %v", err)
	}
	if len(cases) != 2 || cases[0].ID != round2 || cases[1].ID != round1 {
		t.Fatalf("审核案例应按时间倒序返回新旧两轮: %+v", cases)
	}

	// 文件快照在审核单创建时就冻结，与「有没有存过批注」无关。
	files, err := repo.Files(ctx, round2)
	if err != nil {
		t.Fatalf("读取新一轮文件失败: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("新一轮应冻结两张图，实际 %d", len(files))
	}
	// 隔离：新一轮是活跃可批注的轮次，但画布必须是空的。
	next, err := repo.Load(ctx, round2, fx.Attachment, fx.Reviewer)
	if err != nil {
		t.Fatalf("读取新一轮批注失败: %v", err)
	}
	if len(next.Documents) != 0 {
		t.Fatalf("新一轮审核不得继承上一轮批注: %+v", next.Documents)
	}
	if !next.CanEdit {
		t.Fatalf("新一轮仍应是可以批注的活跃轮次")
	}

	history, err := repo.History(ctx, "D-1", "")
	if err != nil {
		t.Fatalf("读取批注历史失败: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("应有 2 轮审核历史，实际 %d", len(history))
	}
	if history[0].CaseID != round2 || history[0].Round != 2 || history[0].SubmissionRound != 2 {
		t.Fatalf("最新一轮应为第 2 轮: %+v", history[0])
	}
	if history[1].CaseID != round1 || history[1].Round != 1 || history[1].Status != "rejected" {
		t.Fatalf("第一轮应标记为第 1 轮已驳回: %+v", history[1])
	}
	if history[1].ChangeRequestNo != "CR-HIST" || history[1].SubmissionRound != 1 || history[1].Flow != "完整审核" {
		t.Fatalf("轮次上下文不完整: %+v", history[1])
	}
	if len(history[0].Files) != 2 || len(history[0].Records) != 0 {
		t.Fatalf("新一轮应有文件无边注记录: %+v", history[0])
	}
	if len(history[1].Files) != 2 || len(history[1].Records) != 2 {
		t.Fatalf("驳回轮次应保留两条批注记录: %+v", history[1])
	}

	// 记录必须能定位到具体文件：两张图的意见不能互相串。
	recorded := func(round annotation.HistoryRound, attachmentID string) annotation.HistoryRecord {
		t.Helper()
		for _, item := range round.Records {
			if item.AttachmentID == attachmentID {
				return item
			}
		}
		t.Fatalf("轮次 %s 缺少附件 %s 的批注记录", round.CaseID, attachmentID)
		return annotation.HistoryRecord{}
	}
	recordA := recorded(history[1], fx.Attachment)
	if recordA.FileName != "P-1.dwg" || recordA.VersionID != fx.Version || recordA.AuthorName != "审核员" {
		t.Fatalf("批注记录文件或作者不正确: %+v", recordA)
	}
	if recordA.NodeName != "校对复核" || recordA.NodeStatus != "rejected" || recordA.NodeOpinion != "尺寸体系不完整" {
		t.Fatalf("批注记录节点信息不正确: %+v", recordA)
	}
	if recordA.MarkCount != 1 || len(recordA.Texts) != 1 || recordA.Texts[0].Text != "缺少尺寸" {
		t.Fatalf("批注文字不正确: %+v", recordA)
	}
	recordB := recorded(history[1], fx.Second)
	if recordB.FileName != "P-2.dwg" || len(recordB.Texts) != 1 || recordB.Texts[0].Text != "材料错误" {
		t.Fatalf("第二份文件的批注归属不正确: %+v", recordB)
	}

	// 只按案例查历史时，轮次号仍按整图计算，不能退化成第 1 轮。
	scoped, err := repo.History(ctx, "", round1)
	if err != nil || len(scoped) != 1 || scoped[0].CaseID != round1 || scoped[0].Round != 1 {
		t.Fatalf("按案例查历史不正确: %v / %+v", err, scoped)
	}
	together, err := repo.History(ctx, "D-1", round1)
	if err != nil || len(together) != 1 || together[0].Round != 1 {
		t.Fatalf("图号与案例同时给定时应返回该轮: %v / %+v", err, together)
	}
}

// TestAnnotationHistoryRejectsMismatchedCaseAndDrawing 入口参数错配时必须拒绝，
// 不能把 A 图的历史入口接到 B 图的审核案例上。
func TestAnnotationHistoryRejectsMismatchedCaseAndDrawing(t *testing.T) {
	db := New(t)
	fx := setupAnnotationHistory(t, db)
	ctx := context.Background()
	repo := annotation.NewRepository(db.Pool)

	round1 := currentRound(t, db, fx.Request)
	db.Exec(t, `INSERT INTO drawings(drawing_no,name,project,created_by,status) VALUES('D-2','另一张图','P',$1::uuid,'draft')`, fx.Author)

	if _, err := repo.History(ctx, "D-2", round1); !errors.Is(err, annotation.ErrNotFound) {
		t.Fatalf("图号与案例错配未被拒绝: %v", err)
	}
	if _, err := repo.History(ctx, "", "not-a-uuid"); !errors.Is(err, annotation.ErrNotFound) {
		t.Fatalf("非法案例未被拒绝: %v", err)
	}
	if _, err := repo.History(ctx, "  ", "  "); !errors.Is(err, annotation.ErrNotFound) {
		t.Fatalf("缺少查询条件未被拒绝: %v", err)
	}
	empty, err := repo.History(ctx, "D-999", "")
	if err != nil || len(empty) != 0 {
		t.Fatalf("未知图号应返回空历史: %v / %+v", err, empty)
	}
}
