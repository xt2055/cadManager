package auth

import "strings"

// 角色名集中定义：职责判断散落在各层的字面量最容易出现拼写漂移，
// 一次写错就会静默放大权限（例如把 "planner" 写成 "planer" 后无人可建档）。
const (
	RoleAdmin    = "admin"
	RolePlanner  = "planner"
	RoleDesigner = "designer"
	RoleReviewer = "reviewer"
)

// HasRole 判断账号是否具有指定角色（大小写不敏感）。
func (user AuthUser) HasRole(role string) bool {
	for _, item := range user.Roles {
		if strings.EqualFold(item, role) {
			return true
		}
	}
	return false
}

// HasAnyRole 判断账号是否具有任一指定角色。
func (user AuthUser) HasAnyRole(roles ...string) bool {
	for _, role := range roles {
		if user.HasRole(role) {
			return true
		}
	}
	return false
}

// CanCreateDrawing 建档权限：只有计划员与管理员可以创建图纸
// （创建新图纸 / 上传老图纸 / 从老图纸分叉三种方式共用同一权限）。
// 创建图纸的职责已从设计人员移交给计划员，设计人员改为在被指派后编制图纸。
func (user AuthUser) CanCreateDrawing() bool {
	return user.HasAnyRole(RoleAdmin, RolePlanner)
}

// CanManageTasks 任务管理台权限：指派、改派、取消指派图纸负责人。
func (user AuthUser) CanManageTasks() bool {
	return user.HasAnyRole(RoleAdmin, RolePlanner)
}

// CanBeDrawingAssignee 是否可被指派为图纸负责人。
// 与「变更工单指定修改人」的既有口径一致：必须真的具备编制图纸的能力，
// 因此纯审核账号不能被指派为负责人。
func (user AuthUser) CanBeDrawingAssignee() bool {
	return user.HasAnyRole(RoleAdmin, RolePlanner, RoleDesigner)
}
