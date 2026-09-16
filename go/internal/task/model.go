// Package task 实现「图纸任务」：计划员把图纸指派给负责人，
// 负责人由此获得原创建人的权限（未存档时对图纸拥有决定控制权）。
//
// 任务不引入人工状态。图纸生命周期（草稿 / 审核中 / 生产中 / 已存档）本身就是
// 权威事实，人工状态会与它相互矛盾，因此进度一律由生命周期、图纸文件与审核节点推导。
package task

import (
	"errors"
	"time"

	"cadguanliq/internal/auth"
	"cadguanliq/internal/drawing"
)

var (
	// ErrForbidden 表示当前账号没有任务管理权限。
	ErrForbidden = errors.New("只有计划员或管理员可以指派图纸负责人")
	// ErrNotFound 表示任务或图纸不存在。
	ErrNotFound = errors.New("任务或图纸不存在")
	// ErrConflict 表示该图纸已有有效负责人，应走改派而不是再次指派。
	ErrConflict = errors.New("该图纸已有负责人，请使用改派")
	// ErrInvalidAssignee 表示被指派人不具备编制图纸的能力或账号已停用。
	ErrInvalidAssignee = errors.New("请选择在职且具有设计、计划或管理员身份的人员作为负责人")
	// ErrArchivedDrawing 表示已存档图纸不允许再指派或改派。
	ErrArchivedDrawing = errors.New("图纸已存档，负责人不再生效；如需修改请走变更工单指定执行人")
	// ErrInvalidInput 表示请求参数不完整。
	ErrInvalidInput = errors.New("任务参数不完整")
)

// Progress 是按图纸生命周期推导的完成度。
// 百分比与阶段同时给出：百分比用于扫视排序，阶段与说明用于回答「下一步做什么」。
type Progress struct {
	Percent int    `json:"percent"`
	Stage   string `json:"stage"`
	Detail  string `json:"detail"`
	// Done 表示这张图纸的编制任务已经交付出去了（生产中或已存档）。
	Done bool `json:"done"`
}

// Drawing 是任务上下文中需要的图纸摘要，不含签署等无关字段。
type Drawing struct {
	ID          string `json:"id"`
	No          string `json:"no"`
	Name        string `json:"name"`
	Project     string `json:"project"`
	Status      string `json:"status"`
	Version     string `json:"version"`
	FileCount   int    `json:"fileCount"`
	ReviewNode  string `json:"reviewNode,omitempty"`
	ReviewDone  int    `json:"reviewDone"`
	ReviewTotal int    `json:"reviewTotal"`
	CreatedAt   string `json:"createdAt,omitempty"`
	UpdatedAt   string `json:"updatedAt,omitempty"`
}

// Assignment 是一次有效指派。改派会生成新的 Assignment，旧记录转为历史。
type Assignment struct {
	TaskID     string `json:"taskId"`
	AssigneeID string `json:"assigneeId"`
	Assignee   string `json:"assignee"`
	AssignedBy string `json:"assignedByName,omitempty"`
	Note       string `json:"note"`
	DueDate    string `json:"dueDate,omitempty"`
	AssignedAt string `json:"assignedAt,omitempty"`
}

// Row 是任务列表/看板的一行：图纸 + 当前指派（可能未指派）。
type Row struct {
	Drawing    Drawing     `json:"drawing"`
	Assignment *Assignment `json:"assignment,omitempty"`
	Progress   Progress    `json:"progress"`
}

// Assigned 表示这一行是否已有有效负责人。
func (row Row) Assigned() bool { return row.Assignment != nil && row.Assignment.AssigneeID != "" }

// HistoryEntry 是改派与取消指派留下的可追溯记录。
type HistoryEntry struct {
	TaskID     string `json:"taskId"`
	AssigneeID string `json:"assigneeId"`
	Assignee   string `json:"assignee"`
	Status     string `json:"status"`
	Note       string `json:"note"`
	DueDate    string `json:"dueDate,omitempty"`
	AssignedBy string `json:"assignedByName,omitempty"`
	AssignedAt string `json:"assignedAt,omitempty"`
	EndedBy    string `json:"endedByName,omitempty"`
	EndedAt    string `json:"endedAt,omitempty"`
	EndReason  string `json:"endReason,omitempty"`
}

// Summary 是当前筛选范围内的计数，用于管理台的统计条。
// 统计与分页解耦：翻页不会改变统计数字。
type Summary struct {
	Total      int `json:"total"`
	Assigned   int `json:"assigned"`
	Unassigned int `json:"unassigned"`
	Active     int `json:"active"`
	Done       int `json:"done"`
	Overdue    int `json:"overdue"`
}

// Page 是任务列表响应。
type Page struct {
	List     []Row   `json:"list"`
	Total    int     `json:"total"`
	Page     int     `json:"page"`
	PageSize int     `json:"pageSize"`
	Summary  Summary `json:"summary"`
}

// ListFilter 是任务列表筛选条件。
type ListFilter struct {
	Page     int
	PageSize int
	Keyword  string
	// Status 按图纸生命周期筛选：draft / reviewing / published / archived / disabled。
	Status string
	// Assigned 取值 all / assigned / unassigned，默认 all。
	Assigned string
	// AssigneeID 只看某位负责人的图纸（计划员用来核对个人负载）。
	AssigneeID string
}

// AssignInput 是新建指派的入参。
type AssignInput struct {
	DrawingID  string `json:"drawingId"`
	AssigneeID string `json:"assigneeId"`
	Note       string `json:"note"`
	DueDate    string `json:"dueDate"`
}

// UpdateInput 是改派或修改任务说明的入参。
// AssigneeID 与当前负责人一致时只更新说明与截止日期，不会产生新的任务历史。
type UpdateInput struct {
	AssigneeID string `json:"assigneeId"`
	Note       string `json:"note"`
	DueDate    string `json:"dueDate"`
	Reason     string `json:"reason"`
}

// Candidate 是可被指派的人员，附带当前在办数量供计划员平衡负载。
type Candidate struct {
	UserID      string   `json:"userId"`
	Account     string   `json:"account"`
	Name        string   `json:"name"`
	Roles       []string `json:"roles"`
	ActiveTasks int      `json:"activeTasks"`
}

// Decides 判定账号对该图纸行是否拥有决定控制权。
// 未指派图纸没有任务行，因此这里只覆盖「已指派」的情况；
// 完整规则（含无负责人时回落创建人）在 drawing.Drawing.Decides。
func (row Row) Decides(userID string, admin bool) bool {
	if admin {
		return true
	}
	if row.Assigned() {
		return row.Assignment != nil && row.Assignment.AssigneeID == userID
	}
	return false
}

// RoleLabels 返回角色的中文标签，供候选人列表直接展示。
func RoleLabels(roles []string) []string {
	labels := make([]string, 0, len(roles))
	for _, role := range roles {
		switch role {
		case auth.RoleAdmin:
			labels = append(labels, "管理员")
		case auth.RolePlanner:
			labels = append(labels, "计划员")
		case auth.RoleDesigner:
			labels = append(labels, "设计人员")
		case auth.RoleReviewer:
			labels = append(labels, "审核人员")
		default:
			labels = append(labels, role)
		}
	}
	return labels
}

// parseTime 把数据库时间戳渲染成 RFC3339，零值返回空串。
func parseTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

// validDrawingStatus 是任务列表允许的图纸状态筛选值。
func validDrawingStatus(status string) bool {
	switch drawing.Status(status) {
	case drawing.StatusDraft, drawing.StatusReviewing, drawing.StatusPublished, drawing.StatusArchived, drawing.StatusDisabled:
		return true
	default:
		return false
	}
}
