package review

import "context"

type Flow struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	CreatedBy   string `json:"createdBy,omitempty"`
	CreatedAt   string `json:"createdAt,omitempty"`
	UpdatedAt   string `json:"updatedAt,omitempty"`
	Nodes       []Node `json:"nodes"`
}

type Node struct {
	ID             string `json:"id,omitempty"`
	Name           string `json:"name"`
	SignerRole     string `json:"signerRole,omitempty"`
	CandidateRole  string `json:"candidateRole"`
	AssignedUserID string `json:"assignedUserId,omitempty"`
	AssignedName   string `json:"assignedName"`
	Required       bool   `json:"required"`
	Order          int    `json:"order"`
}

type SaveFlowInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	Nodes       []Node `json:"nodes"`
}

// ReviewCase 一次图纸审核流转实例（含顺序节点）。
type ReviewCase struct {
	ID          string     `json:"id"`
	DrawingNo   string     `json:"drawingNo"`
	DrawingName string     `json:"drawingName"`
	FlowName    string     `json:"flow"`
	Status      string     `json:"status"`
	Initiator   string     `json:"initiator"`
	StartedAt   string     `json:"startedAt"`
	CompletedAt string     `json:"completedAt,omitempty"`
	Nodes       []CaseNode `json:"nodes"`
}

// CaseNode 审核案例中的单个签署节点。
type CaseNode struct {
	Name           string `json:"name"`
	AssignedUserID string `json:"assignedUserId,omitempty"`
	AssignedName   string `json:"assignedName"`
	Status         string `json:"status"`
	Opinion        string `json:"opinion"`
	Required       bool   `json:"required"`
	Order          int    `json:"order"`
	ReviewedAt     string `json:"time,omitempty"`
}

// CompletedAction 已办审核归档记录。
type CompletedAction struct {
	ID        string `json:"id"`
	CaseID    string `json:"reviewCaseId"`
	DrawingNo string `json:"no"`
	Name      string `json:"name"`
	NodeName  string `json:"node"`
	Initiator string `json:"by"`
	Reviewer  string `json:"reviewer"`
	Time      string `json:"time"`
	Result    string `json:"result"`
	Opinion   string `json:"opinion"`
	Ver       string `json:"ver"`
}

type StartCaseInput struct {
	DrawingNo string `json:"drawingNo"`
}

type SubmitNodeInput struct {
	NodeName string `json:"nodeName"`
	Action   string `json:"action"`
	Opinion  string `json:"opinion"`
}

var (
	ErrNotFound      = errorString("review flow not found")
	ErrConflict      = errorString("review flow conflict")
	ErrCaseNotFound  = errorString("review case not found")
	ErrCaseConflict  = errorString("review case conflict")
	ErrCaseForbidden = errorString("review forbidden")
)

type errorString string

func (e errorString) Error() string { return string(e) }

type Repository interface {
	List(ctx context.Context) ([]Flow, error)
	Find(ctx context.Context, id string) (Flow, error)
	Create(ctx context.Context, input SaveFlowInput, userID string) (Flow, error)
	Update(ctx context.Context, id string, input SaveFlowInput, userID string) (Flow, error)
	SetEnabled(ctx context.Context, id string, enabled bool, userID string) (Flow, error)
	StartCase(ctx context.Context, drawingNo string, userID string) (ReviewCase, error)
	ListCases(ctx context.Context) ([]ReviewCase, error)
	SubmitNode(ctx context.Context, caseID string, input SubmitNodeInput, userID string) (ReviewCase, error)
	CompletedActions(ctx context.Context) ([]CompletedAction, error)
	// ActiveCaseAssigneeByDrawingNo 返回审核中图纸当前活动节点的责任人用户 ID（无进行中案例返回空串）。
	ActiveCaseAssigneeByDrawingNo(ctx context.Context, drawingNo string) (string, error)
}
