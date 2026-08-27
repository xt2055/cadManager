package attachment

import "context"

type Role string

const (
	RoleAssembly Role = "assembly"
	RolePart     Role = "part"
	RoleMaterial Role = "material"
	RoleCraft    Role = "craft"
	RoleOther    Role = "other"
)

type Attachment struct {
	ID          string  `json:"id"`
	StorageKey  string  `json:"storageKey"`
	Name        string  `json:"name"`
	DrawingNo   string  `json:"drawingNo"`
	PartNo      *string `json:"partNo,omitempty"`
	Role        Role    `json:"role"`
	Size        int64   `json:"size"`
	MimeType    string  `json:"mimeType"`
	SHA256      string  `json:"sha256,omitempty"`
	Version     string  `json:"version"`
	Previewable bool    `json:"previewable"`
	UploadedBy  string  `json:"uploadedBy,omitempty"`
	CreatedAt   string  `json:"createdAt,omitempty"`
}

type CreateInput struct {
	DrawingNo   string
	PartNo      string
	Role        Role
	Name        string
	MimeType    string
	Version     string
	Previewable bool
}

type Repository interface {
	Create(ctx context.Context, input CreateInput, object StorageObject, userID string) (Attachment, error)
	Find(ctx context.Context, storageKey string) (Attachment, error)
	FindByOwnerAndName(ctx context.Context, drawingNo, partNo, name string) (Attachment, error)
	ListByDrawing(ctx context.Context, drawingNo string) ([]Attachment, error)
	ListAllExb(ctx context.Context) ([]Attachment, error)
	ListAllCad(ctx context.Context) ([]Attachment, error)
	Delete(ctx context.Context, storageKey string, userID string) error
	FolderForDrawing(ctx context.Context, drawingNo string) (string, error)
}

type StorageObject struct {
	Key      string
	Size     int64
	MimeType string
	SHA256   string
}
