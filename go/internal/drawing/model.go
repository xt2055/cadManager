package drawing

import "context"

type Status string

const (
	StatusPublished Status = "published"
	StatusReviewing Status = "reviewing"
	StatusDraft     Status = "draft"
	StatusDisabled  Status = "disabled"
	StatusArchived  Status = "archived"
)

type Drawing struct {
	ID              string            `json:"id"`
	No              string            `json:"no"`
	Name            string            `json:"name"`
	Kind            string            `json:"kind"`
	Project         string            `json:"project"`
	Material        string            `json:"material"`
	Vendor          string            `json:"vendor"`
	Status          Status            `json:"status"`
	Version         string            `json:"ver"`
	BorrowFrom      *string           `json:"borrowFrom,omitempty"`
	Remark          *string           `json:"remark,omitempty"`
	By              string            `json:"by,omitempty"`
	Updated         string            `json:"updated,omitempty"`
	CreatedBy       string            `json:"createdBy,omitempty"`
	CreatedAt       string            `json:"createdAt,omitempty"`
	UpdatedBy       string            `json:"updatedBy,omitempty"`
	UpdatedAt       string            `json:"updatedAt,omitempty"`
	Signers         Signers           `json:"signers"`
	AttributeValues map[string]string `json:"attributeValues"`
	Revision        int64             `json:"revision"`

	// CreatedByID 创建者用户 ID（仅用于服务端权限判断，不下发前端）。
	CreatedByID string `json:"-"`
}

type Signers map[string]string

type Part struct {
	ID                string  `json:"id"`
	RelationID        string  `json:"relationId,omitempty"`
	DrawingID         string  `json:"drawingId"`
	No                string  `json:"no"`
	Name              string  `json:"name"`
	ParentNo          string  `json:"parentNo"`
	Project           string  `json:"project,omitempty"`
	Material          string  `json:"material"`
	Spec              string  `json:"spec"`
	Weight            float64 `json:"weight"`
	SurfaceTreatment  string  `json:"surfaceTreatment"`
	ManufacturingType string  `json:"partType"`
	Quantity          float64 `json:"qty"`
	Status            Status  `json:"status"`
	Version           string  `json:"ver"`
	Vendor            *string `json:"vendor,omitempty"`
	BorrowFrom        *string `json:"borrowFrom,omitempty"`
	Remark            *string `json:"remark,omitempty"`
	CreatedBy         string  `json:"createdBy,omitempty"`
	CreatedAt         string  `json:"createdAt,omitempty"`
	UpdatedBy         string  `json:"updatedBy,omitempty"`
	UpdatedAt         string  `json:"updatedAt,omitempty"`
	Signers           Signers `json:"signers"`
	Revision          int64   `json:"revision"`
	RelationRevision  int64   `json:"relationRevision,omitempty"`
	RelationType      string  `json:"relationType,omitempty"`
	LifecycleStatus   string  `json:"lifecycleStatus,omitempty"`
}

type Relation struct {
	ID               string  `json:"id"`
	DrawingID        string  `json:"drawingId"`
	PartID           string  `json:"partId"`
	ParentRelationID *string `json:"parentRelationId,omitempty"`
	RelationType     string  `json:"relationType"`
	Quantity         float64 `json:"qty"`
	Position         *string `json:"position,omitempty"`
	LineNo           *int    `json:"lineNo,omitempty"`
	Remark           string  `json:"remark"`
	BorrowReason     *string `json:"borrowReason,omitempty"`
	Revision         int64   `json:"revision"`
	Status           string  `json:"status"`
}

type ListFilter struct {
	Page     int
	PageSize int
	Keyword  string
	Status   Status
	Vendor   string
}

type Page[T any] struct {
	List     []T `json:"list"`
	Total    int `json:"total"`
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
}

type CreateDrawingInput struct {
	No         string  `json:"no"`
	Name       string  `json:"name"`
	Project    string  `json:"project"`
	Kind       string  `json:"kind"`
	Material   string  `json:"material"`
	Vendor     string  `json:"vendor"`
	Status     Status  `json:"status"`
	Version    string  `json:"ver"`
	BorrowFrom *string `json:"borrowFrom"`
	Remark     *string `json:"remark"`
	Signers    Signers `json:"signers"`
}

type UpdateDrawingInput struct {
	ExpectedRevision *int64            `json:"expectedRevision"`
	Name             *string           `json:"name"`
	Kind             *string           `json:"kind"`
	Project          *string           `json:"project"`
	Material         *string           `json:"material"`
	Vendor           *string           `json:"vendor"`
	Status           *Status           `json:"status"`
	Version          *string           `json:"ver"`
	BorrowFrom       *string           `json:"borrowFrom"`
	Remark           *string           `json:"remark"`
	Signers          Signers           `json:"signers"`
	AttributeValues  map[string]string `json:"attributeValues"`
}

type CreatePartInput struct {
	No                string  `json:"no"`
	Name              string  `json:"name"`
	ParentNo          string  `json:"parentNo"`
	Material          string  `json:"material"`
	Spec              string  `json:"spec"`
	Weight            float64 `json:"weight"`
	SurfaceTreatment  string  `json:"surfaceTreatment"`
	ManufacturingType string  `json:"partType"`
	Quantity          float64 `json:"qty"`
	Status            Status  `json:"status"`
	Version           string  `json:"ver"`
	Project           string  `json:"project"`
	Vendor            *string `json:"vendor"`
	BorrowFrom        *string `json:"borrowFrom"`
	Remark            *string `json:"remark"`
	Signers           Signers `json:"signers"`
}

type UpdatePartInput struct {
	ExpectedRevision  *int64   `json:"expectedRevision"`
	No                *string  `json:"no"`
	Name              *string  `json:"name"`
	Material          *string  `json:"material"`
	Spec              *string  `json:"spec"`
	Weight            *float64 `json:"weight"`
	SurfaceTreatment  *string  `json:"surfaceTreatment"`
	ManufacturingType *string  `json:"partType"`
	Quantity          *float64 `json:"qty"`
	Status            *Status  `json:"status"`
	Version           *string  `json:"ver"`
	Vendor            *string  `json:"vendor"`
	BorrowFrom        *string  `json:"borrowFrom"`
	Remark            *string  `json:"remark"`
}

type UpdateRelationInput struct {
	ExpectedRevision *int64   `json:"expectedRevision"`
	Qty              *float64 `json:"qty"`
	Remark           *string  `json:"remark"`
	Position         *string  `json:"position"`
	ParentRelationID *string  `json:"parentRelationId"`
}

type BorrowInput struct {
	SourcePartID     string  `json:"sourcePartId"`
	Qty              float64 `json:"qty"`
	Position         *string `json:"position"`
	LineNo           *int    `json:"lineNo"`
	Remark           string  `json:"remark"`
	BorrowReason     string  `json:"borrowReason"`
	ParentRelationID *string `json:"parentRelationId"`
}

type ForkInput struct {
	NewPartNo string `json:"newPartNo"`
	Name      string `json:"name"`
}

type PartRevision struct {
	ID                string  `json:"id"`
	PartID            string  `json:"partId"`
	RevisionNo        int     `json:"revisionNo"`
	Version           string  `json:"ver"`
	RowRevision       int64   `json:"rowRevision"`
	Name              string  `json:"name"`
	Material          string  `json:"material"`
	Spec              string  `json:"spec"`
	Weight            float64 `json:"weight"`
	SurfaceTreatment  string  `json:"surfaceTreatment"`
	PartType          string  `json:"partType"`
	WorkflowStatus    string  `json:"workflowStatus"`
	BasedOnRevisionID *string `json:"basedOnRevisionId,omitempty"`
	CreatedBy         string  `json:"createdBy,omitempty"`
	CreatedAt         string  `json:"createdAt,omitempty"`
	PublishedBy       string  `json:"publishedBy,omitempty"`
	PublishedAt       string  `json:"publishedAt,omitempty"`
}

type CreateRevisionInput struct {
	Name             string  `json:"name"`
	Material         string  `json:"material"`
	Spec             string  `json:"spec"`
	Weight           float64 `json:"weight"`
	SurfaceTreatment string  `json:"surfaceTreatment"`
	PartType         string  `json:"partType"`
	Version          string  `json:"ver"`
}

type UpdateRevisionInput struct {
	ExpectedRowRevision *int64   `json:"expectedRowRevision"`
	Name                *string  `json:"name"`
	Material            *string  `json:"material"`
	Spec                *string  `json:"spec"`
	Weight              *float64 `json:"weight"`
	SurfaceTreatment    *string  `json:"surfaceTreatment"`
	PartType            *string  `json:"partType"`
	Version             *string  `json:"ver"`
}

type BOMItem struct {
	ID                      string  `json:"id,omitempty"`
	ItemNo                  int     `json:"no"`
	PartID                  *string `json:"partId,omitempty"`
	SourceAttachmentVersion *string `json:"sourceAttachmentVersionId,omitempty"`
	Name                    string  `json:"name"`
	Spec                    string  `json:"spec"`
	Quantity                float64 `json:"qty"`
	Weight                  float64 `json:"weight"`
	Remark                  string  `json:"remark"`
}

type BOM struct {
	DrawingID string    `json:"drawingId"`
	Revision  int64     `json:"revision"`
	Items     []BOMItem `json:"items"`
}

type UpdateBOMInput struct {
	ExpectedRevision *int64    `json:"expectedRevision"`
	Items            []BOMItem `json:"items"`
}

type Repository interface {
	List(ctx context.Context, filter ListFilter) (Page[Drawing], error)
	Find(ctx context.Context, id string) (Drawing, error)
	FindByNo(ctx context.Context, no string) (Drawing, error)
	Create(ctx context.Context, input CreateDrawingInput, userID string) (Drawing, error)
	Update(ctx context.Context, id string, input UpdateDrawingInput, userID string) (Drawing, error)
	// SetStatusByNo 受控状态流转（CAS：仅当当前状态等于 from 才更新），用于存档/解除存档。
	SetStatusByNo(ctx context.Context, no string, from, to Status, userID string) (Drawing, error)
	ListParts(ctx context.Context, drawingID string) ([]Part, error)
	FindPart(ctx context.Context, id string) (Part, error)
	CreatePart(ctx context.Context, drawingID string, input CreatePartInput, userID string) (Part, error)
	UpdatePart(ctx context.Context, id string, input UpdatePartInput, userID string) (Part, error)
}
