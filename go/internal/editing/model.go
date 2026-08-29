package editing

import (
	"context"
	"time"
)

type Session struct {
	ID           string     `json:"id"`
	AttachmentID string     `json:"attachmentId"`
	StorageKey   string     `json:"storageKey"`
	UserID       string     `json:"userId"`
	UserName     string     `json:"userName"`
	UNCPath      string     `json:"uncPath"`
	Status       string     `json:"status"`
	StartedAt    time.Time  `json:"startedAt"`
	LastSeenAt   time.Time  `json:"lastSeenAt"`
	ClosedAt     *time.Time `json:"closedAt,omitempty"`
}

type ActiveSessionInfo struct {
	ID           string    `json:"id"`
	AttachmentID string    `json:"attachmentId"`
	StorageKey   string    `json:"storageKey"`
	FileName     string    `json:"fileName"`
	DrawingNo    string    `json:"drawingNo"`
	PartNo       string    `json:"partNo,omitempty"`
	UserID       string    `json:"userId"`
	UserName     string    `json:"userName"`
	UserAccount  string    `json:"userAccount"`
	UNCPath      string    `json:"uncPath"`
	Status       string    `json:"status"`
	StartedAt    time.Time `json:"startedAt"`
	LastSeenAt   time.Time `json:"lastSeenAt"`
	IsCurrent    bool      `json:"isCurrent"`
	CanClose     bool      `json:"canClose"`
}

type OpenResult struct {
	SessionID string    `json:"sessionId"`
	OpenURL   string    `json:"openUrl"`
	UNCPath   string    `json:"uncPath"`
	SMBRoot   string    `json:"smbRoot"`
	ExpiresAt time.Time `json:"expiresAt"`
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

type Repository interface {
	CreateSession(ctx context.Context, session Session) error
	CreateTicket(ctx context.Context, token string, sessionID, userID string, expiresAt time.Time) error
	ConsumeTicket(ctx context.Context, token, userID string, now time.Time) (Session, error)
	FindActiveByStorageKey(ctx context.Context, storageKey string, now time.Time) (Session, error)
	ListActiveSessions(ctx context.Context, now time.Time, drawingNo string) ([]ActiveSessionInfo, error)
	ExpireStale(ctx context.Context, now time.Time) error
	CleanupTickets(ctx context.Context, now time.Time) error
	Heartbeat(ctx context.Context, userID, sessionID string, now time.Time) error
	Close(ctx context.Context, userID, sessionID string, isAdmin bool, now time.Time) error
}
