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
	WorkStorageKey string `json:"workStorageKey,omitempty"`
	// ChangeRequestID 非空表示本次编辑属于某张存档变更工单：
	// 结束编辑时只登记工作版本、不切换正式指针，成果待验收后才发布。
	ChangeRequestID string     `json:"changeRequestId,omitempty"`
	UserID          string     `json:"userId"`
	UserName        string     `json:"userName"`
	UNCPath         string     `json:"uncPath"`
	Status          string     `json:"status"`
	StartedAt       time.Time  `json:"startedAt"`
	LastSeenAt      time.Time  `json:"lastSeenAt"`
	ClosedAt        *time.Time `json:"closedAt,omitempty"`
}

type ActiveSessionInfo struct {
	ID             string    `json:"id"`
	AttachmentID   string    `json:"attachmentId"`
	StorageKey     string    `json:"storageKey"`
	WorkStorageKey string    `json:"workStorageKey,omitempty"`
	FileName       string    `json:"fileName"`
	DrawingNo      string    `json:"drawingNo"`
	PartNo         string    `json:"partNo,omitempty"`
	UserID         string    `json:"userId"`
	UserName       string    `json:"userName"`
	UserAccount    string    `json:"userAccount"`
	UNCPath        string    `json:"uncPath"`
	Status         string    `json:"status"`
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
	SessionID         string `json:"sessionId"`
	Changed           bool   `json:"changed"`
	Version           string `json:"version,omitempty"`
	CurrentStorageKey string `json:"currentStorageKey,omitempty"`
	CurrentName       string `json:"currentName,omitempty"`
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

// OnlineOpenResult 浏览器在线编辑的打开结果。
// RequiresTicket 为真表示该附件属于已存档图纸，必须绑定执行中的变更工单，
// 编辑成果只能登记为工单工作版本、验收后才发布；为假时走常规替换保存。
// LoadURL 是相对 API 前缀的内容加载路径：有工作版本时指向工作版本，否则指向正式版当前文件。
type OnlineOpenResult struct {
	Revision        int64  `json:"revision"`
	RequiresTicket  bool   `json:"requiresTicket"`
	ChangeRequestID string `json:"changeRequestId,omitempty"`
	WorkVersionID   string `json:"workVersionId,omitempty"`
	LoadURL         string `json:"loadUrl"`
	FileName        string `json:"fileName"`
}

// OnlineSaveResult 在线保存工单工作版本的结果。WorkVersionID 供下一次保存作为并发基线。
type OnlineSaveResult struct {
	ChangeRequestID string `json:"changeRequestId"`
	WorkVersionID   string `json:"workVersionId"`
	Version         string `json:"version"`
	FileName        string `json:"fileName"`
}

type Repository interface {
	CompleteWorkingSession(ctx context.Context, session Session, versionID string) error
	CreateSession(ctx context.Context, session Session) error
	// CreateSessionWithTicket 为存档变更工单原子地创建会话与打开票据：
	// 在同一事务内锁定工单行、复查其仍为 executing 后才写入，杜绝"提交/终止后又开出可写会话"。
	// token 由调用方预先生成。工单已不可写时返回 ErrTicketClosed。
	CreateSessionWithTicket(ctx context.Context, session Session, token string, expiresAt time.Time, requestID string) error
	CreateTicket(ctx context.Context, token string, sessionID, userID string, expiresAt time.Time) error
	ConsumeTicket(ctx context.Context, token, userID string, now time.Time) (Session, error)
	FindActiveByStorageKey(ctx context.Context, storageKey string, now time.Time) (Session, error)
	FindActiveByID(ctx context.Context, sessionID string) (Session, error)
	UpdateWorkStorageKey(ctx context.Context, userID, sessionID, workStorageKey string) error
	ListActiveSessions(ctx context.Context, now time.Time, drawingNo string) ([]ActiveSessionInfo, error)
	ExpireStale(ctx context.Context, now time.Time) error
	CleanupTickets(ctx context.Context, now time.Time) error
	Heartbeat(ctx context.Context, userID, sessionID string, now time.Time) error
	Close(ctx context.Context, userID, sessionID string, isAdmin bool, now time.Time) error
}
