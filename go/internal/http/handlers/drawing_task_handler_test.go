package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"cadguanliq/internal/auth"
	"cadguanliq/internal/task"
)

// fakeTaskService 记录调用方与被调用参数，用来证明「拒绝」发生在触达业务逻辑之前。
type fakeTaskService struct {
	calls         []string
	lastActor     auth.AuthUser
	lastMineUID   string
	lastFilter    task.ListFilter
	assignErr     error
	boardErr      error
	candidatesErr error
}

func (f *fakeTaskService) ListBoard(_ context.Context, filter task.ListFilter) (task.Page, error) {
	f.calls = append(f.calls, "board")
	f.lastFilter = filter
	return task.Page{}, f.boardErr
}

func (f *fakeTaskService) ListMine(_ context.Context, userID string, filter task.ListFilter) (task.Page, error) {
	f.calls = append(f.calls, "mine")
	f.lastMineUID = userID
	f.lastFilter = filter
	return task.Page{List: []task.Row{}, Total: 0, Page: 1, PageSize: 20}, nil
}

func (f *fakeTaskService) Assign(_ context.Context, actor auth.AuthUser, input task.AssignInput) (task.Row, error) {
	f.calls = append(f.calls, "assign")
	f.lastActor = actor
	if f.assignErr != nil {
		return task.Row{}, f.assignErr
	}
	return task.Row{Drawing: task.Drawing{ID: input.DrawingID, No: "D-1", Status: "draft"}}, nil
}

func (f *fakeTaskService) Update(_ context.Context, actor auth.AuthUser, taskID string, input task.UpdateInput) (task.Row, error) {
	f.calls = append(f.calls, "update")
	f.lastActor = actor
	if input.AssigneeID == "missing" {
		return task.Row{}, task.ErrInvalidAssignee
	}
	if taskID == "archived-task" {
		return task.Row{}, task.ErrArchivedDrawing
	}
	return task.Row{Drawing: task.Drawing{ID: "drawing-1", No: "D-1", Status: "draft"}}, nil
}

func (f *fakeTaskService) Cancel(_ context.Context, actor auth.AuthUser, taskID, reason string) error {
	f.calls = append(f.calls, "cancel")
	f.lastActor = actor
	if taskID == "unknown" {
		return task.ErrNotFound
	}
	return nil
}

func (f *fakeTaskService) History(context.Context, string) ([]task.HistoryEntry, error) {
	f.calls = append(f.calls, "history")
	return []task.HistoryEntry{}, nil
}

func (f *fakeTaskService) Candidates(context.Context) ([]task.Candidate, error) {
	f.calls = append(f.calls, "candidates")
	if f.candidatesErr != nil {
		return nil, f.candidatesErr
	}
	return []task.Candidate{{UserID: "u1", Account: "designer", Name: "张工", Roles: []string{"designer"}}}, nil
}

func taskRequest(method, target, body string, roles ...string) *http.Request {
	return authenticatedRequestAs(method, target, body, roles)
}

// 设计人员与审核人员既不能看任务总表，也不能指派、改派、取消。
// 拒绝必须发生在业务逻辑之前：不能出现「先写了一部分再报 403」。
func TestTaskManagementRejectsNonPlannerRoles(t *testing.T) {
	requests := []struct {
		name    string
		handler func(DrawingTaskService) http.Handler
		request *http.Request
	}{
		{name: "看任务总表", handler: DrawingTasks, request: taskRequest(http.MethodGet, "/api/drawing-tasks?scope=board", "", "designer")},
		{name: "新建指派", handler: DrawingTasks, request: taskRequest(http.MethodPost, "/api/drawing-tasks", `{"drawingId":"d1","assigneeId":"u1"}`, "reviewer")},
		{name: "改派", handler: DrawingTaskResource, request: taskRequest(http.MethodPut, "/api/drawing-tasks/t1", `{"assigneeId":"u2"}`, "designer")},
		{name: "取消指派", handler: DrawingTaskResource, request: taskRequest(http.MethodDelete, "/api/drawing-tasks/t1", "", "designer")},
		{name: "候选名单", handler: DrawingTaskCandidates, request: taskRequest(http.MethodGet, "/api/drawing-tasks/candidates", "", "reviewer")},
		{name: "指派历史", handler: DrawingTaskResource, request: taskRequest(http.MethodGet, "/api/drawing-tasks/d1/history", "", "designer")},
	}
	for _, item := range requests {
		t.Run(item.name, func(t *testing.T) {
			service := &fakeTaskService{}
			recorder := httptest.NewRecorder()
			item.handler(service).ServeHTTP(recorder, item.request)
			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d, expected %d", recorder.Code, http.StatusForbidden)
			}
			if len(service.calls) != 0 {
				t.Fatalf("拒绝请求不应触达任务逻辑，实际调用 %v", service.calls)
			}
		})
	}
}

// 计划员可以完整走完指派、改派、取消。
func TestTaskManagementAllowsPlanner(t *testing.T) {
	service := &fakeTaskService{}

	recorder := httptest.NewRecorder()
	DrawingTasks(service).ServeHTTP(recorder, taskRequest(http.MethodPost, "/api/drawing-tasks", `{"drawingId":"d1","assigneeId":"u1","note":"优先","dueDate":"2026-10-01"}`, "planner"))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("指派 status = %d, expected %d", recorder.Code, http.StatusCreated)
	}
	if service.lastActor.ID != "user-1" {
		t.Fatalf("指派应带上操作人身份，实际 %q", service.lastActor.ID)
	}

	recorder = httptest.NewRecorder()
	DrawingTaskResource(service).ServeHTTP(recorder, taskRequest(http.MethodPut, "/api/drawing-tasks/t1", `{"assigneeId":"u2","reason":"原负责人休假"}`, "planner"))
	if recorder.Code != http.StatusOK {
		t.Fatalf("改派 status = %d, expected %d", recorder.Code, http.StatusOK)
	}

	recorder = httptest.NewRecorder()
	DrawingTaskResource(service).ServeHTTP(recorder, taskRequest(http.MethodDelete, "/api/drawing-tasks/t1?reason=任务取消", "", "planner"))
	if recorder.Code != http.StatusOK {
		t.Fatalf("取消 status = %d, expected %d", recorder.Code, http.StatusOK)
	}
}

// scope=mine 对所有登录用户开放，但只按调用者本人过滤：
// 首页任务面板靠它取数，若漏传本人 ID 就会把别人的任务显示成「我的」。
func TestMyTasksScopeUsesCallerIdentity(t *testing.T) {
	service := &fakeTaskService{}
	recorder := httptest.NewRecorder()
	DrawingTasks(service).ServeHTTP(recorder, taskRequest(http.MethodGet, "/api/drawing-tasks?scope=mine&page=2&page_size=5", "", "designer"))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, expected %d", recorder.Code, http.StatusOK)
	}
	if service.lastMineUID != "user-1" {
		t.Fatalf("本人 ID = %q, expected user-1", service.lastMineUID)
	}
	if service.lastFilter.Page != 2 || service.lastFilter.PageSize != 5 {
		t.Fatalf("分页参数未透传: %+v", service.lastFilter)
	}
}

// 看板只对计划员与管理员开放。
func TestBoardScopeAllowsPlannerAndAdmin(t *testing.T) {
	for _, role := range []string{"planner", "admin"} {
		t.Run(role, func(t *testing.T) {
			service := &fakeTaskService{}
			recorder := httptest.NewRecorder()
			DrawingTasks(service).ServeHTTP(recorder, taskRequest(http.MethodGet, "/api/drawing-tasks?scope=board&keyword=JG&status=draft&assigned=unassigned", "", role))
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, expected %d", recorder.Code, http.StatusOK)
			}
			if service.lastFilter.Keyword != "JG" || service.lastFilter.Status != "draft" || service.lastFilter.Assigned != "unassigned" {
				t.Fatalf("筛选条件未透传: %+v", service.lastFilter)
			}
		})
	}
}

// 领域错误必须映射成可行动的状态码与文案。
func TestTaskErrorsMapToActionableStatus(t *testing.T) {
	tests := []struct {
		name    string
		handler func(DrawingTaskService) http.Handler
		request *http.Request
		want    int
	}{
		{
			name:    "重复指派返回冲突并提示改派",
			handler: DrawingTasks,
			request: taskRequest(http.MethodPost, "/api/drawing-tasks", `{"drawingId":"d1","assigneeId":"u1"}`, "planner"),
			want:    http.StatusConflict,
		},
		{
			name:    "被指派人不可用返回 400",
			handler: DrawingTaskResource,
			request: taskRequest(http.MethodPut, "/api/drawing-tasks/t1", `{"assigneeId":"missing"}`, "planner"),
			want:    http.StatusBadRequest,
		},
		{
			name:    "已存档图纸返回冲突",
			handler: DrawingTaskResource,
			request: taskRequest(http.MethodPut, "/api/drawing-tasks/archived-task", `{"assigneeId":"u2"}`, "planner"),
			want:    http.StatusConflict,
		},
		{
			name:    "任务不存在返回 404",
			handler: DrawingTaskResource,
			request: taskRequest(http.MethodDelete, "/api/drawing-tasks/unknown", "", "planner"),
			want:    http.StatusNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &fakeTaskService{assignErr: task.ErrConflict}
			recorder := httptest.NewRecorder()
			tt.handler(service).ServeHTTP(recorder, tt.request)
			if recorder.Code != tt.want {
				t.Fatalf("status = %d, expected %d (body=%s)", recorder.Code, tt.want, recorder.Body.String())
			}
			envelope := responseEnvelope(t, recorder)
			message, _ := envelope["message"].(string)
			if message == "" {
				t.Fatal("错误响应必须带可读文案")
			}
		})
	}
}

// 未登录不得读取任务（scope=mine 同样需要登录）。
func TestTaskEndpointsRequireAuthentication(t *testing.T) {
	service := &fakeTaskService{}
	for _, target := range []string{"/api/drawing-tasks?scope=mine", "/api/drawing-tasks?scope=board"} {
		request := httptest.NewRequest(http.MethodGet, target, nil)
		recorder := httptest.NewRecorder()
		DrawingTasks(service).ServeHTTP(recorder, request)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("%s status = %d, expected %d", target, recorder.Code, http.StatusUnauthorized)
		}
	}
}

// 参数格式错误必须是 400，而不是 500。
func TestTaskAssignRejectsMalformedPayload(t *testing.T) {
	service := &fakeTaskService{}
	recorder := httptest.NewRecorder()
	DrawingTasks(service).ServeHTTP(recorder, taskRequest(http.MethodPost, "/api/drawing-tasks", `{"drawingId":`, "planner"))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, expected %d", recorder.Code, http.StatusBadRequest)
	}
}
