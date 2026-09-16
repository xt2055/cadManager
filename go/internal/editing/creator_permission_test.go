package editing

import (
	"context"
	"testing"

	"cadguanliq/internal/auth"
	"cadguanliq/internal/drawing"
)

type creatorDrawingLookup struct {
	status    drawing.Status
	assignees []drawing.Assignee
}

func (lookup creatorDrawingLookup) FindByNo(context.Context, string) (drawing.Drawing, error) {
	return drawing.Drawing{ID: "project", CreatedByID: "creator", Status: lookup.status, Assignees: lookup.assignees}, nil
}

// 未指派时控制权归创建人，指派后移交负责人；
// 已存档图纸对所有人只读，必须走变更工单。
func TestCreatorEditPermissionWithoutDesignerRole(t *testing.T) {
	for _, status := range []drawing.Status{drawing.StatusDraft, drawing.StatusReviewing, drawing.StatusPublished, drawing.StatusArchived} {
		for _, id := range []string{"creator", "other"} {
			t.Run(string(status)+"/"+id, func(t *testing.T) {
				service := &Service{drawings: creatorDrawingLookup{status: status}}
				_, err := service.authorizeEdit(context.Background(), auth.AuthUser{ID: id}, "project", "file")
				wantAllowed := id == "creator" && status != drawing.StatusArchived
				if (err == nil) != wantAllowed {
					t.Fatalf("allowed=%v, want %v (error=%v)", err == nil, wantAllowed, err)
				}
			})
		}
	}
}

// 负责人 = 原创建人权限：指派生效后负责人拥有与创建人同等的编辑权，
// 而创建人在未存档前也一并失去控制权（负责人优先）。
func TestAssigneeInheritsCreatorEditPermission(t *testing.T) {
	for _, status := range []drawing.Status{drawing.StatusDraft, drawing.StatusReviewing, drawing.StatusPublished, drawing.StatusArchived} {
		for _, id := range []string{"assignee", "creator", "other"} {
			t.Run(string(status)+"/"+id, func(t *testing.T) {
				service := &Service{drawings: creatorDrawingLookup{
					status:    status,
					assignees: []drawing.Assignee{{UserID: "assignee", Name: "负责人"}},
				}}
				_, err := service.authorizeEdit(context.Background(), auth.AuthUser{ID: id}, "project", "file")
				wantAllowed := id == "assignee" && status != drawing.StatusArchived
				if (err == nil) != wantAllowed {
					t.Fatalf("allowed=%v, want %v (error=%v)", err == nil, wantAllowed, err)
				}
			})
		}
	}
}

// 有负责人时创建人必须拿到「说明原因 + 给出解决入口」的拒绝文案，
// 而不是一句无法行动的「没有权限」。
func TestAssigneeTakesOverEditPermissionWithActionableMessage(t *testing.T) {
	service := &Service{drawings: creatorDrawingLookup{
		status:    drawing.StatusDraft,
		assignees: []drawing.Assignee{{UserID: "assignee"}},
	}}
	_, err := service.authorizeEdit(context.Background(), auth.AuthUser{ID: "creator", Roles: []string{"designer"}}, "project", "file")
	if err == nil {
		t.Fatal("创建人在指派生效后不应再拥有编辑权")
	}
	if message := err.Error(); message == "当前账号没有 CAD 编辑权限" {
		t.Fatalf("拒绝文案必须可行动，实际 %q", message)
	}
}
