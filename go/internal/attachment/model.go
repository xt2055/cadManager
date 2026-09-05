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
	ID                string  `json:"id"`
	StorageKey        string  `json:"storageKey"`
	CurrentStorageKey string  `json:"currentStorageKey,omitempty"`
	Name              string  `json:"name"`
	CurrentName       string  `json:"currentName,omitempty"`
	CurrentMimeType   string  `json:"currentMimeType,omitempty"`
	CurrentSize       int64   `json:"currentSize,omitempty"`
	CurrentSHA256     string  `json:"currentSha256,omitempty"`
	DrawingNo         string  `json:"drawingNo"`
	PartNo            *string `json:"partNo,omitempty"`
	Role              Role    `json:"role"`
	Size              int64   `json:"size"`
	MimeType          string  `json:"mimeType"`
	SHA256            string  `json:"sha256,omitempty"`
	Version           string  `json:"version"`
	Previewable       bool    `json:"previewable"`
	// UploadedBy 显示名（列表/详情展示用）；UploadedByID 是 users.id 原始 UUID，
	// 供版本登记等需要真实用户 ID 的内部链路使用，两者互不混用。
	UploadedBy        string  `json:"uploadedBy,omitempty"`
	UploadedByID      string  `json:"-"`
	CreatedAt         string  `json:"createdAt,omitempty"`
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
	UpdateContent(ctx context.Context, storageKey string, size int64, mimeType, sha256 string) error
	// SetCurrentVersion 将附件当前内容切换到新版本文件并同步版本号；
	// 版本文件永远写入 history 版本目录，原始 storage_key 不被覆盖。
	SetCurrentVersion(ctx context.Context, sourceStorageKey, currentStorageKey, name, version string, size int64, mimeType, sha256 string) error
	ListByDrawing(ctx context.Context, drawingNo string) ([]Attachment, error)
	ListAll(ctx context.Context) ([]Attachment, error)
	ListAllExb(ctx context.Context) ([]Attachment, error)
	ListAllCad(ctx context.Context) ([]Attachment, error)
	Delete(ctx context.Context, storageKey string, userID string) error
	ReassignPart(ctx context.Context, storageKey, partID string) error
	ReidentifyPart(ctx context.Context, storageKey, partNo, userID string) (ReidentifyResult, error)
	FolderForDrawing(ctx context.Context, drawingNo string) (string, error)
}

// IdentityRepository exposes operations that must target one logical
// attachment. A storage key identifies content and may be shared by many
// attachments after blob de-duplication.
type IdentityRepository interface {
	FindByID(ctx context.Context, attachmentID string) (Attachment, error)
	DeleteByID(ctx context.Context, attachmentID string, userID string) (Attachment, error)
	HasStorageKeyReference(ctx context.Context, storageKey string) (bool, error)
	ReidentifyPartByID(ctx context.Context, attachmentID, partNo, userID string) (ReidentifyResult, error)
}

type StorageObject struct {
	Key      string
	Size     int64
	MimeType string
	SHA256   string
}

type ReidentifyResult struct {
	StorageKey string `json:"storageKey"`
	DrawingNo  string `json:"drawingNo"`
	OldPartNo  string `json:"oldPartNo"`
	PartNo     string `json:"partNo"`
}
