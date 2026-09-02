package versioning

import (
	"context"
	"time"
)

type Version struct {
	ID               string     `json:"id"`
	AttachmentID     string     `json:"attachmentId"`
	StorageKey       string     `json:"storageKey"`
	SourceStorageKey string     `json:"sourceStorageKey"`
	Version          string     `json:"version"`
	VersionKind      string     `json:"versionKind"`
	Size             int64      `json:"size"`
	MimeType         string     `json:"mimeType"`
	SHA256           string     `json:"sha256"`
	CreatedBy        string     `json:"createdBy,omitempty"`
	CreatedByName    string     `json:"createdByName,omitempty"`
	CreatedAt        time.Time  `json:"createdAt"`
	ExpiresAt        *time.Time `json:"expiresAt,omitempty"`
	IsPinned         bool       `json:"isPinned"`
	PinnedBy         string     `json:"pinnedBy,omitempty"`
	PinnedAt         *time.Time `json:"pinnedAt,omitempty"`
	ReleasedBy       string     `json:"releasedBy,omitempty"`
	ReleasedAt       *time.Time `json:"releasedAt,omitempty"`
	IsCurrentRelease bool       `json:"isCurrentRelease"`
	DeletedAt        *time.Time `json:"deletedAt,omitempty"`
}

type CreateInput struct {
	AttachmentID     string
	SourceStorageKey string
	Version          string
	VersionKind      string
	Size             int64
	MimeType         string
	SHA256           string
	CreatedBy        string
	ExpiresAt        *time.Time
	// CurrentName 版本登记成功后写入 attachments.current_name 的当前文件名；
	// 为空时跳过指针切换（仅登记版本）。
	CurrentName string
}

type Repository interface {
	Create(ctx context.Context, input CreateInput, storageKey string) (Version, error)
	// CreateWithPromotion 在同一数据库事务内插入版本记录并切换附件当前版本指针，
	// 保证「版本存在」与「当前指针指向该版本」原子生效。
	CreateWithPromotion(ctx context.Context, input CreateInput, storageKey string) (Version, error)
	GetByID(ctx context.Context, versionID string) (Version, error)
	GetByStorageKey(ctx context.Context, storageKey string) (Version, error)
	ListByAttachment(ctx context.Context, attachmentID string) ([]Version, error)
	LatestByAttachment(ctx context.Context, attachmentID string) (Version, error)
	PromoteInitial(ctx context.Context, versionID string) error
	Retain(ctx context.Context, versionID, userID string) (Version, error)
	Release(ctx context.Context, versionID, userID string) (Version, error)
	ListExpired(ctx context.Context, now time.Time) ([]Version, error)
	MarkDeleted(ctx context.Context, versionID string) error
}
