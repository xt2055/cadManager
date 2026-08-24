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

var (
	ErrNotFound = errorString("review flow not found")
	ErrConflict = errorString("review flow conflict")
)

type errorString string

func (e errorString) Error() string { return string(e) }

type Repository interface {
	List(ctx context.Context) ([]Flow, error)
	Find(ctx context.Context, id string) (Flow, error)
	Create(ctx context.Context, input SaveFlowInput, userID string) (Flow, error)
	Update(ctx context.Context, id string, input SaveFlowInput, userID string) (Flow, error)
	SetEnabled(ctx context.Context, id string, enabled bool, userID string) (Flow, error)
}
