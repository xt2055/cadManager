package handlers

import (
	"cadguanliq/internal/auth"
	"cadguanliq/internal/drawing"
	"cadguanliq/internal/http/middleware"
)

// canCreateDrawing 建档类动作只允许计划员与管理员：
// 创建图纸的权限已从设计人员移交计划员，设计人员改为在任务管理台被指派后编制图纸。
func canCreateDrawing(user auth.AuthUser) bool {
	return user.CanCreateDrawing()
}

// canManageDrawingTasks 任务管理台（指派、改派、取消指派）只允许计划员与管理员。
func canManageDrawingTasks(user auth.AuthUser) bool {
	return user.CanManageTasks()
}

// drawingDecides 判定账号对图纸是否拥有决定控制权。
// 规则正文在 drawing.Drawing.Decides，服务端所有创建人级判定都必须经这里，
// 避免各 handler 自己比较 createdBy 而产生与任务指派不一致的权限。
// middleware 的 HasRole 仅用于保持与既有 handler 相同的角色读取方式。
func drawingDecides(item drawing.Drawing, user auth.AuthUser) bool {
	return item.Decides(user.ID, middleware.HasRole(user, auth.RoleAdmin))
}
