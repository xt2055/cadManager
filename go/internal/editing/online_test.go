package editing

import (
	"context"
	"errors"
	"strings"
	"testing"

	"cadguanliq/internal/attachment"
	"cadguanliq/internal/drawing"
)

// fakeOnlineDrawing 存档图纸生命周期查询替身。
type fakeOnlineDrawing struct{ status drawing.Status }

func (f fakeOnlineDrawing) FindByNo(ctx context.Context, no string) (drawing.Drawing, error) {
	return drawing.Drawing{ID: "draw-1", No: no, Status: f.status}, nil
}

// fakeOnlineGate 变更工单门禁替身，覆盖在线开启/保存所需的全部查询与登记。
type fakeOnlineGate struct {
	requestID           string
	executing           bool
	workAttID           string
	workVersionID       string
	found               bool
	baseline            string
	recordErr           error
	recorded            []string
	recordedAttachments []string
	conflictOnRecord    bool
}

func (g *fakeOnlineGate) CanEditArchived(ctx context.Context, drawingID, attachmentID, userID string) (bool, string, error) {
	if g.requestID == "" {
		return false, "", nil
	}
	return true, g.requestID, nil
}

func (g *fakeOnlineGate) WorkVersion(ctx context.Context, requestID, attachmentID string) (string, string, bool, error) {
	if g.found && (g.workAttID == "" || g.workAttID == attachmentID) {
		return "history/work.dwg", g.workVersionID, true, nil
	}
	return "", "", false, nil
}

func (g *fakeOnlineGate) EditBaseline(ctx context.Context, requestID, attachmentID string) (string, bool, error) {
	return g.baseline, g.baseline != "", nil
}

func (g *fakeOnlineGate) RecordWorkVersion(ctx context.Context, requestID, attachmentID, versionID string) error {
	g.recorded = append(g.recorded, versionID)
	g.recordedAttachments = append(g.recordedAttachments, attachmentID)
	return g.recordErr
}

func (g *fakeOnlineGate) CompareAndRecordWorkVersion(ctx context.Context, requestID, attachmentID, versionID, expectedVersionID, userID string) (bool, error) {
	if g.conflictOnRecord || !g.executing || expectedVersionID != g.workVersionID {
		return false, nil
	}
	return true, g.RecordWorkVersion(ctx, requestID, attachmentID, versionID)
}

func (g *fakeOnlineGate) StillExecuting(ctx context.Context, requestID string) (bool, error) {
	return g.executing, nil
}

func onlineFixture(t *testing.T, gate *fakeOnlineGate) (*testEnv, attachment.Attachment) {
	t.Helper()
	env := newTestEnv(t)
	item := attachment.Attachment{
		ID:                "att-001",
		StorageKey:        "drawings/JG-00/泵缸.exb",
		CurrentStorageKey: "drawings/JG-00/history/泵缸/v1.0/泵缸.dwg",
		Name:              "泵缸.exb",
		CurrentName:       "泵缸.dwg",
		DrawingNo:         "JG-00",
		Version:           "v1.0",
	}
	env.attachments.Items(map[string]attachment.Attachment{item.StorageKey: item})
	env.service.SetPolicy(fakeOnlineDrawing{status: drawing.StatusArchived}, nil)
	env.service.SetChangeGate(gate)
	return env, item
}

func TestOnlineOpenWithWorkVersionLoadsWorkContent(t *testing.T) {
	env, item := onlineFixture(t, &fakeOnlineGate{
		requestID: "req-1", executing: true, found: true, workAttID: "att-001", workVersionID: "ver-work",
	})
	result, err := env.service.OnlineOpen(context.Background(), designerUser, item.StorageKey)
	if err != nil {
		t.Fatalf("OnlineOpen 失败: %v", err)
	}
	if !result.RequiresTicket || result.ChangeRequestID != "req-1" || result.WorkVersionID != "ver-work" {
		t.Fatalf("OnlineOpen 结果异常: %+v", result)
	}
	if result.LoadURL != "/file-versions/ver-work/source" {
		t.Fatalf("应加载工作版本内容，实际: %s", result.LoadURL)
	}
}

func TestOnlineOpenArchivedWithoutTicketIsRejected(t *testing.T) {
	env, item := onlineFixture(t, &fakeOnlineGate{requestID: ""})
	if _, err := env.service.OnlineOpen(context.Background(), designerUser, item.StorageKey); err == nil {
		t.Fatal("存档且无进行中工单应拒绝在线编辑")
	}
}

func TestOnlineSaveDraftRegistersWorkingVersion(t *testing.T) {
	gate := &fakeOnlineGate{requestID: "req-1", executing: true, baseline: "cafebabe"}
	env, item := onlineFixture(t, gate)
	content := []byte("AC1015fake-dwg-bytes")
	result, err := env.service.OnlineSaveDraft(context.Background(), designerUser, item.StorageKey, "req-1", "", "泵缸.dwg", content)
	if err != nil {
		t.Fatalf("OnlineSaveDraft 失败: %v", err)
	}
	if result.WorkVersionID == "" || len(gate.recorded) != 1 || gate.recorded[0] != result.WorkVersionID {
		t.Fatalf("未把捕获的工作版本登记到工单: result=%+v recorded=%v", result, gate.recorded)
	}
	if len(gate.recordedAttachments) != 1 || gate.recordedAttachments[0] != item.ID {
		t.Fatalf("工作版本登记到了错误附件: %v", gate.recordedAttachments)
	}
	if gate.baseline == "" {
		t.Log("baseline 未透传，检查捕获基准")
	}
}

func TestOnlineSaveDraftRejectsNonDWG(t *testing.T) {
	gate := &fakeOnlineGate{requestID: "req-1", executing: true}
	env, item := onlineFixture(t, gate)
	if _, err := env.service.OnlineSaveDraft(context.Background(), designerUser, item.StorageKey, "req-1", "", "x.dwg", []byte("garbage")); err == nil {
		t.Fatal("非 DWG 内容应被拒绝")
	}
	if len(gate.recorded) != 0 {
		t.Fatal("内容非法时不得登记工单成果")
	}
}

func TestOnlineSaveDraftConflictOnStaleBase(t *testing.T) {
	gate := &fakeOnlineGate{requestID: "req-1", executing: true, found: true, workAttID: "att-001", workVersionID: "ver-new"}
	env, item := onlineFixture(t, gate)
	_, err := env.service.OnlineSaveDraft(context.Background(), designerUser, item.StorageKey, "req-1", "ver-old", "泵缸.dwg", []byte("AC1015x"))
	if !errors.Is(err, ErrWorkVersionConflict) {
		t.Fatalf("基线过期应返回工作版本冲突，实际: %v", err)
	}
	if len(gate.recorded) != 0 {
		t.Fatal("冲突时不得登记，避免覆盖他人成果")
	}
}

func TestOnlineSaveDraftTicketClosed(t *testing.T) {
	gate := &fakeOnlineGate{requestID: "req-1", executing: false}
	env, item := onlineFixture(t, gate)
	_, err := env.service.OnlineSaveDraft(context.Background(), designerUser, item.StorageKey, "req-1", "", "泵缸.dwg", []byte("AC1015x"))
	if !errors.Is(err, ErrTicketClosed) {
		t.Fatalf("工单不可写应返回 ErrTicketClosed，实际: %v", err)
	}
}

func TestOnlineSaveDraftKeepsContentWhenRecordFails(t *testing.T) {
	gate := &fakeOnlineGate{requestID: "req-1", executing: true, recordErr: errors.New("db down")}
	env, item := onlineFixture(t, gate)
	_, err := env.service.OnlineSaveDraft(context.Background(), designerUser, item.StorageKey, "req-1", "", "泵缸.dwg", []byte("AC1015x"))
	if err == nil || !strings.Contains(err.Error(), "db down") {
		t.Fatalf("登记失败应返回原始错误: %v", err)
	}
	if len(gate.recorded) != 1 {
		t.Fatal("应尝试登记（版本已生成），失败交由用户重试")
	}
}

func TestOnlineSaveDraftRejectsNonArchived(t *testing.T) {
	env := newTestEnv(t)
	item := attachment.Attachment{ID: "att-001", StorageKey: "drawings/JG-00/泵缸.dwg", Name: "泵缸.dwg", CurrentName: "泵缸.dwg", DrawingNo: "JG-00", Version: "v1.0"}
	env.attachments.Items(map[string]attachment.Attachment{item.StorageKey: item})
	env.service.SetPolicy(fakeOnlineDrawing{status: drawing.StatusDraft}, nil)
	env.service.SetChangeGate(&fakeOnlineGate{requestID: "req-1", executing: true})
	_, err := env.service.OnlineSaveDraft(context.Background(), adminUser, item.StorageKey, "req-1", "", "泵缸.dwg", []byte("AC1015x"))
	if !errors.Is(err, ErrNotOnlineTicket) {
		t.Fatalf("非存档图纸在线保存应被拒绝，实际: %v", err)
	}
}

func TestOnlineSaveDraftConflictDuringCapture(t *testing.T) {
	gate := &fakeOnlineGate{requestID: "req-1", executing: true, conflictOnRecord: true}
	env, item := onlineFixture(t, gate)
	_, err := env.service.OnlineSaveDraft(context.Background(), designerUser, item.StorageKey, "req-1", "", "edit.dwg", []byte("AC1015content"))
	if !errors.Is(err, ErrWorkVersionConflict) || len(gate.recorded) != 0 {
		t.Fatalf("capture-time conflict must not overwrite: err=%v recorded=%v", err, gate.recorded)
	}
}

func TestOnlineSaveDraftRejectsDifferentTicket(t *testing.T) {
	gate := &fakeOnlineGate{requestID: "req-new", executing: true}
	env, item := onlineFixture(t, gate)
	_, err := env.service.OnlineSaveDraft(context.Background(), designerUser, item.StorageKey, "req-old", "", "edit.dwg", []byte("AC1015content"))
	if !errors.Is(err, ErrTicketClosed) || len(gate.recorded) != 0 {
		t.Fatalf("old editor must not save into new ticket: %v", err)
	}
}

func TestOnlineSaveDraftRejectsTruncatedDWGHeader(t *testing.T) {
	for _, content := range []string{"AC10", "AC10xx"} {
		env, item := onlineFixture(t, &fakeOnlineGate{requestID: "req-1", executing: true})
		if _, err := env.service.OnlineSaveDraft(context.Background(), designerUser, item.StorageKey, "req-1", "", "edit.dwg", []byte(content)); err == nil {
			t.Fatalf("accepted invalid header %q", content)
		}
	}
}

func TestOnlineOpenReturnsCurrentAttachmentRevision(t *testing.T) {
	env, item := onlineFixture(t, &fakeOnlineGate{})
	item.Revision = 7
	env.attachments.Items(map[string]attachment.Attachment{item.StorageKey: item})
	env.service.SetPolicy(fakeOnlineDrawing{status: drawing.StatusDraft}, nil)
	result, err := env.service.OnlineOpen(context.Background(), adminUser, item.StorageKey)
	if err != nil || result.RequiresTicket || result.Revision != 7 {
		t.Fatalf("expected current attachment revision: result=%+v err=%v", result, err)
	}
}
