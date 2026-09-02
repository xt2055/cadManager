package editing

import (
	"context"
	"time"
)

type Session struct {
	ID string `json:"id"`
	// AttachmentID 原始附件 ID，用于锁定与查找附件。
	AttachmentID string `json:"attachmentId"`
	// StorageKey 原始附件存储键（可能是 .exb）。
	StorageKey string `json:"storageKey"`
	// WorkStorageKey 实际复制到 SMB 的工作文件（当前版本 DWG）。
	WorkStorageKey string     `json:"workStorageKey,omitempty"`
	UserID         string     `json:"userId"`
	UserName       string     `json:"userName"`
	UNCPath        string     `json:"uncPath"`
	Status         string     `json:"status"`
	StartedAt      time.Time  `json:"startedAt"`
	LastSeenAt     time.Time  `json:"lastSeenAt"`
	ClosedAt       *time.Time `json:"closedAt,omitempty"`
}

type ActiveSessionInfo struct {
	ID             string `json:"id"`
	AttachmentID   string `json:"attachmentId"`
	StorageKey     string `json:"storageKey"`
	WorkStorageKey string `json:"workStorageKey,omitempty"`
	FileName       string `json:"fileName"`
	DrawingNo      string `json:"drawingNo"`
	PartNo         string `json:"partNo,omitempty"`
	UserID         string `json:"userId"`
	UserName       string `json:"userName"`
	UserAccount    string `json:"userAccount"`
	UNCPath        string `json:"uncPath"`
	Status         string `json:"status"`
	StartedAt      time.Time `json:"startedAt"`
	LastSeenAt     time.Time `json:"lastSeenAt"`
	IsCurrent      bool      `json:"isCurrent"`
	CanClose       bool      `json:"canClose"`
	Online         bool      `json:"online"`
}

type OpenResult struct {
	SessionID string    `json:"sessionId"`
	OpenURL   string    `json:"openUrl"`
	UNCPath   string    `json:"uncPath"`
	SMBRoot   string    `json:"smbRoot"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// CloseResult 结束编辑的结果：是否产生新版本及新的当前版本信息，
// 前端据此更新文件版本并从后端刷新业务数据。
type CloseResult struct {
	SessionID        string `json:"sessionId"`
	Changed          bool   `json:"changed"`
	Version          string `json:"version,omitempty"`
	CurrentStorageKey string `json:"currentStorageKey,omitempty"`
	CurrentName      string `json:"currentName,omitempty"`
}

type ExchangeResult struct {
	SessionID       string `json:"sessionId"`
	SMBRoot         string `json:"smbRoot"`
	UNCPath         string `json:"uncPath"`
	FileName        string `json:"fileName"`
	CaxaPath        string `json:"caxaPath"`
	SMBUsername     string `json:"smbUsername"`
	SMBPassword     string `json:"smbPassword"`
	HeartbeatSecond int    `json:"heartbeatIntervalSeconds"`
}

// ReadOnlyOpenResult 只读查看：返回附件下载相对路径与文件名，客户端下载到本机临时目录打开。
type ReadOnlyOpenResult struct {
	DownloadPath string `json:"downloadPath"`
	FileName     string `json:"fileName"`
	CaxaPath     string `json:"caxaPath"`
}

type Repository interface {
	CreateSession(ctx context.Context, session Session) error
	CreateTicket(ctx context.Context, token string, sessionID, userID string, expiresAt time.Time) error
	ConsumeTicket(ctx context.Context, token, userID string, now time.Time) (Session, error)
	FindActiveByStorageKey(ctx context.Context, storageKey string, now time.Time) (Session, error)
	FindActiveByID(ctx context.Context, sessionID string) (Session, error)
	ListActiveSessions(ctx context.Context, now time.Time, drawingNo string) ([]ActiveSessionInfo, error)
	ExpireStale(ctx context.Context, now time.Time) error
	CleanupTickets(ctx context.Context, now time.Time) error
	Heartbeat(ctx context.Context, userID, sessionID string, now time.Time) error
	Close(ctx context.Context, userID, sessionID string, isAdmin bool, now time.Time) error
}
