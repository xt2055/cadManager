package dbtest

import (
	"testing"

	"cadguanliq/internal/drawing"
)

// 同一图纸同一时刻只能有一条有效任务。
// 这条约束由部分唯一索引保证，而不是靠应用层先查后写——并发指派下先查后写必然漏。
func TestDrawingTaskAllowsSingleActiveAssignee(t *testing.T) {
	db := New(t)
	fx := db.Seed(t)
	db.AssignDrawingTask(t, fx.Drawing, fx.Author, fx.Planner)
	message := db.MustFail(t, `INSERT INTO drawing_tasks(drawing_id, assignee_id) VALUES($1::uuid, $2::uuid)`, fx.Drawing, fx.Reviewer)
	if message == "" {
		t.Fatal("重复指派必须被数据库拒绝")
	}
}

// 改派保留历史：旧任务转为 replaced，图纸恰好留下一条有效任务。
func TestDrawingTaskReassignmentKeepsHistory(t *testing.T) {
	db := New(t)
	fx := db.Seed(t)
	first := db.AssignDrawingTask(t, fx.Drawing, fx.Author, fx.Planner)
	db.Exec(t, `UPDATE drawing_tasks SET status='replaced', ended_at=now(), end_reason='改派' WHERE id=$1::uuid`, first)
	second := db.AssignDrawingTask(t, fx.Drawing, fx.Reviewer, fx.Planner)

	active := db.ScanString(t, `SELECT id::text FROM drawing_tasks WHERE drawing_id=$1::uuid AND status='active'`, fx.Drawing)
	if active != second {
		t.Fatalf("有效任务 = %s, 期望 %s", active, second)
	}
	if count := db.ScanString(t, `SELECT count(*)::text FROM drawing_tasks WHERE drawing_id=$1::uuid`, fx.Drawing); count != "2" {
		t.Fatalf("历史任务应保留，实际 %s 条", count)
	}
	if reason := db.ScanString(t, `SELECT end_reason FROM drawing_tasks WHERE id=$1::uuid`, first); reason != "改派" {
		t.Fatalf("改派原因 = %q", reason)
	}
}

// 控制权判定与 Go 侧 Drawing.Decides 必须同构：数据库函数是服务端事务内使用的实现，
// Go 方法供非事务路径使用，两者一旦漂移就会出现「页面说能改、保存时报 403」。
func TestDrawingDecisionOwnerMatchesGoDecides(t *testing.T) {
	db := New(t)
	fx := db.Seed(t)

	type scenario struct {
		name         string
		withTask     bool
		assigneeID   string
		userID       string
		admin        bool
		wantDecision bool
	}
	// 管理员场景一律使用现场创建的新账号并单独授予 admin 角色，
	// 否则一次授权会污染后续场景，让「负责人」规则看起来即使失效也能通过。
	scenarios := []scenario{
		{name: "未指派创建人拥有控制权", userID: fx.Author, wantDecision: true},
		{name: "未指派他人没有控制权", userID: fx.Reviewer, wantDecision: false},
		{name: "已指派负责人拥有控制权", withTask: true, assigneeID: fx.Reviewer, userID: fx.Reviewer, wantDecision: true},
		{name: "已指派后创建人失去控制权", withTask: true, assigneeID: fx.Reviewer, userID: fx.Author, wantDecision: false},
		{name: "未指派管理员拥有控制权", admin: true, wantDecision: true},
		{name: "已指派管理员仍拥有控制权", withTask: true, assigneeID: fx.Reviewer, admin: true, wantDecision: true},
	}

	for _, item := range scenarios {
		t.Run(item.name, func(t *testing.T) {
			// 每个场景使用独立图纸，避免场景之间互相污染指派状态。
			drawingID := db.ScanString(t, `INSERT INTO drawings(drawing_no,name,project,created_by,status) VALUES($1,'决策判定图纸','P',$2::uuid,'draft') RETURNING id::text`,
				"D-DECIDE-"+item.name, fx.Author)
			if item.withTask {
				db.AssignDrawingTask(t, drawingID, item.assigneeID, fx.Planner)
			}
			actorID := item.userID
			if item.admin {
				actorID = db.ScanString(t, `INSERT INTO users(account,display_name,password_hash,status) VALUES($1,'临时管理员','x','active') RETURNING id::text`, "admin-"+item.name)
				db.Exec(t, `INSERT INTO user_roles(user_id, role) VALUES($1::uuid,'admin')`, actorID)
			}

			databaseDecision := db.ScanString(t, `SELECT drawing_decision_owner($1::uuid, $2::uuid)::text`, drawingID, actorID) == "true"
			if databaseDecision != item.wantDecision {
				t.Fatalf("drawing_decision_owner = %v, 期望 %v", databaseDecision, item.wantDecision)
			}

			// Go 侧用同一份数据重放规则，断言两个实现给出同一个答案。
			var assignees []drawing.Assignee
			if item.withTask {
				assignees = []drawing.Assignee{{UserID: item.assigneeID}}
			}
			goDecision := drawing.Drawing{ID: drawingID, CreatedByID: fx.Author, Assignees: assignees}.Decides(actorID, item.admin)
			if goDecision != item.wantDecision {
				t.Fatalf("Drawing.Decides = %v, 期望 %v", goDecision, item.wantDecision)
			}
		})
	}
}

// 取消指派后控制权必须回落给创建人，否则图纸会卡在「没有人能改」的状态。
func TestCancelledAssignmentReturnsControlToCreator(t *testing.T) {
	db := New(t)
	fx := db.Seed(t)
	taskID := db.AssignDrawingTask(t, fx.Drawing, fx.Reviewer, fx.Planner)

	if allowed := db.ScanString(t, `SELECT drawing_decision_owner($1::uuid, $2::uuid)::text`, fx.Drawing, fx.Author); allowed != "false" {
		t.Fatal("指派生效期间创建人不应拥有控制权")
	}
	db.Exec(t, `UPDATE drawing_tasks SET status='cancelled', ended_at=now(), end_reason='取消测试' WHERE id=$1::uuid`, taskID)
	if allowed := db.ScanString(t, `SELECT drawing_decision_owner($1::uuid, $2::uuid)::text`, fx.Drawing, fx.Author); allowed != "true" {
		t.Fatal("取消指派后控制权必须回落给创建人")
	}
	if allowed := db.ScanString(t, `SELECT drawing_decision_owner($1::uuid, $2::uuid)::text`, fx.Drawing, fx.Reviewer); allowed != "false" {
		t.Fatal("取消指派后被取消人不应再拥有控制权")
	}
}

// 计划员身份必须能写进 user_roles：角色 CHECK 约束漏改会让建档账号建不出来。
func TestPlannerRoleAcceptedByConstraint(t *testing.T) {
	db := New(t)
	fx := db.Seed(t)
	db.Exec(t, `INSERT INTO user_roles(user_id, role) VALUES($1::uuid,'planner')`, fx.Planner)
	if role := db.ScanString(t, `SELECT role FROM user_roles WHERE user_id=$1::uuid`, fx.Planner); role != "planner" {
		t.Fatalf("role = %q", role)
	}
	db.MustFail(t, `INSERT INTO user_roles(user_id, role) VALUES($1::uuid,'architect')`, fx.Planner)
}

// 任务通知使用独立 kind，避免与变更、审核通知混在一起导致点击跳错页。
func TestTaskNotificationKindAccepted(t *testing.T) {
	db := New(t)
	fx := db.Seed(t)
	db.Exec(t, `INSERT INTO notifications(recipient_id, sender_id, kind, title, content, drawing_id, event_key)
		VALUES($1::uuid, $2::uuid, 'task', '你被指派为图纸负责人', 'D-1 · 测试图纸', $3::uuid, 'task:test')`,
		fx.Reviewer, fx.Planner, fx.Drawing)
	db.MustFail(t, `INSERT INTO notifications(recipient_id, kind, title, event_key)
		VALUES($1::uuid, 'unknown-kind', '标题', 'event')`, fx.Reviewer)
}
