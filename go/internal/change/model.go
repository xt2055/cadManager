package change

import "time"

// Status 变更工单状态机：
// pending_approval → executing → pending_verify → completed
// 免验收（require_verify=false）时提交完成直接进入 completed；
// pending_approval 可被 reject；非终态可被 cancel。
type Status string

const (
	StatusPendingApproval Status = "pending_approval"
	StatusExecuting       Status = "executing"
	StatusPendingVerify   Status = "pending_verify"
	StatusCompleted       Status = "completed"
	StatusRejected        Status = "rejected"
	StatusCancelled       Status = "cancelled"
)

// openStatuses 视为未结束、会占用图纸变更名额的状态。
var openStatuses = []Status{StatusPendingApproval, StatusExecuting, StatusPendingVerify}

func (s Status) Open() bool {
	for _, open := range openStatuses {
		if s == open {
			return true
		}
	}
	return false
}

// Action 工单操作类型，用于操作时间线。
type Action string

const (
	ActionCreate      Action = "create"
	ActionApprove     Action = "approve"
	ActionReject      Action = "reject"
	ActionSubmit      Action = "submit"
	ActionVerify      Action = "verify"
	ActionReturn      Action = "return"
	ActionCancel      Action = "cancel"
	ActionWaiveVerify Action = "waive_verify"
)

// Diff 结构化的"修改了什么"，按类别与字段记录前后值。
type Diff struct {
	Kind     string `json:"kind"`
	Field    string `json:"field"`
	OldValue string `json:"oldValue"`
	NewValue string `json:"newValue"`
}

type Target struct {
	AttachmentID string `json:"attachmentId"`
	Name         string `json:"name"`
	FileCategory string `json:"fileCategory"`
	DrawingNo    string `json:"drawingNo"`
	PartNo       string `json:"partNo,omitempty"`
}

// Submission 一次提交完成形成的不可变快照轮次：
// 记录本轮实际修改说明、拟发布属性、基线/成果文件版本与本轮差异，
// 保证验收所见即所发布，退回重提不再与旧轮次混淆。
type Submission struct {
	LegacyHistory bool               `json:"legacyHistory"`
	ID            string             `json:"id"`
	Round         int                `json:"round"`
	ActorName     string             `json:"actorName,omitempty"`
	ActualChanges string             `json:"actualChanges"`
	Proposed      ProposedAttributes `json:"proposedAttributes"`
	Status        string             `json:"status"`
	CreatedAt     time.Time          `json:"createdAt"`
	Diffs         []Diff             `json:"diffs,omitempty"`
}

// ProposedAttributes 变更工单暂存的技术属性草案，验收通过时才落到图纸。
type ProposedAttributes struct {
	Name     *string `json:"name,omitempty"`
	Material *string `json:"material,omitempty"`
	Vendor   *string `json:"vendor,omitempty"`
	Version  *string `json:"version,omitempty"`
}

func (p ProposedAttributes) empty() bool {
	return p.Name == nil && p.Material == nil && p.Vendor == nil && p.Version == nil
}

// Request 变更工单记录（含展示用的关联名称）。
type Request struct {
	ID                string             `json:"id"`
	RequestNo         string             `json:"requestNo"`
	DrawingID         string             `json:"drawingId"`
	DrawingNo         string             `json:"drawingNo"`
	Title             string             `json:"title"`
	Reason            string             `json:"reason"`
	Scope             string             `json:"scope"`
	Status            Status             `json:"status"`
	RequireVerify     bool               `json:"requireVerify"`
	ApplicantID       string             `json:"applicantId"`
	ApplicantName     string             `json:"applicantName,omitempty"`
	ExecutorID        string             `json:"executorId"`
	ExecutorName      string             `json:"executorName,omitempty"`
	ApproverID        string             `json:"approverId,omitempty"`
	ApproverName      string             `json:"approverName,omitempty"`
	VerifierID        string             `json:"verifierId,omitempty"`
	VerifierName      string             `json:"verifierName,omitempty"`
	ApproverOpinion   string             `json:"approverOpinion,omitempty"`
	WaiveVerifyReason string             `json:"waiveVerifyReason,omitempty"`
	DirectAdmin       bool               `json:"directAdminApproval"`
	ApprovedAt        *time.Time         `json:"approvedAt,omitempty"`
	SubmittedAt       *time.Time         `json:"submittedAt,omitempty"`
	CompletedAt       *time.Time         `json:"completedAt,omitempty"`
	CreatedAt         time.Time          `json:"createdAt"`
	ActualChanges     string             `json:"actualChanges"`
	Proposed          ProposedAttributes `json:"proposedAttributes"`
	// CurrentSubmissionID 为当前待验收/最近一次提交的快照轮次；无提交时为空。
	CurrentSubmissionID string         `json:"currentSubmissionId,omitempty"`
	Diffs               []Diff         `json:"diffs,omitempty"`
	Actions             []ActionRecord `json:"actions,omitempty"`
	Submissions         []Submission   `json:"submissions,omitempty"`
	Targets             []Target       `json:"targets,omitempty"`
}

// ActionRecord 工单操作时间线单条记录。
type ActionRecord struct {
	ID        string    `json:"id"`
	Action    Action    `json:"action"`
	Opinion   string    `json:"opinion"`
	ActorName string    `json:"actorName,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

// CreateInput 发起变更申请。
type CreateInput struct {
	Reason         string
	Scope          string
	Title          string
	ExecutorID     string
	RequireVerify  *bool
	AutoApprove    bool     // 管理员直接发起并批准
	ApproveOpinion string   // 管理员直接批准时的审批意见（必填）
	WaiveReason    string   // 管理员直接批准且免验收时的独立原因（必填）
	AttachmentIDs  []string // 本次允许修改的附件清单
}

// ApproveInput 管理员审批通过。
type ApproveInput struct {
	Opinion       string
	RequireVerify bool
	WaiveReason   string
}

// SubmitInput 执行人提交完成。
type SubmitInput struct {
	ActualChanges string
	Proposed      ProposedAttributes
}

// DecisionInput 驳回/退回/终止/验收的通用意见输入。
// SubmissionID 仅验收使用：传入时应与当前待验收轮次一致，防止旧页面验收新一轮成果。
type DecisionInput struct {
	Opinion      string
	SubmissionID string
}

// ListFilter 工单列表过滤。
type ListFilter struct {
	DrawingID  string
	Status     Status
	ExecutorID string
	OpenOnly   bool
}
