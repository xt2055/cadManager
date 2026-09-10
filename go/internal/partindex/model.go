package partindex

import (
	"context"
	"errors"
)

var (
	ErrNotFound         = errors.New("零件索引不存在或当前不可用")
	ErrForbidden        = errors.New("没有修改该零件索引的权限")
	ErrConflict         = errors.New("文件、识别结果或索引已变化，请重新加载后核对")
	ErrInvalid          = errors.New("零件索引参数无效")
	ErrSnapshotRequired = errors.New("当前版本没有标题栏快照，请先提取信息")
)

const (
	StatusPending           = "pending"
	StatusFailed            = "failed"
	StatusNeedsConfirmation = "needs_confirmation"
	StatusRecognized        = "recognized"
	StatusEdited            = "edited"
	StatusConfirmed         = "confirmed"
	StatusRecheck           = "recheck"

	ExtractionPending   = "pending"
	ExtractionExtracted = "extracted"
	ExtractionFailed    = "failed"
)

// Fields is deliberately complete. Empty strings are meaningful user input and
// must never be filled with automatic values after manual save.
type Fields struct {
	DrawingNo      string `json:"drawingNo"`
	PartName       string `json:"partName"`
	Material       string `json:"material"`
	Designer       string `json:"designer"`
	Checker        string `json:"checker"`
	Approver       string `json:"approver"`
	DrawingDateRaw string `json:"drawingDateRaw"`
	Scale          string `json:"scale"`
	SheetSize      string `json:"sheetSize"`
	Process        string `json:"process"`
	Standard       string `json:"standard"`
	Company        string `json:"company"`
}

type Project struct {
	DrawingID     string   `json:"drawingId"`
	ProjectCode   string   `json:"projectCode"`
	ProjectName   string   `json:"projectName"`
	DrawingNo     string   `json:"drawingNo"`
	RelationTypes []string `json:"relationTypes"`
}

type Item struct {
	AttachmentID     string    `json:"attachmentId"`
	VersionID        string    `json:"versionId"`
	FileName         string    `json:"fileName"`
	PartID           *string   `json:"partId"`
	RegisteredPartNo string    `json:"registeredPartNo"`
	DrawingNo        string    `json:"drawingNo"`
	PartName         string    `json:"partName"`
	Material         string    `json:"material"`
	Designer         string    `json:"designer"`
	DrawingDateRaw   string    `json:"drawingDateRaw"`
	DrawingDate      *string   `json:"drawingDate"`
	Status           string    `json:"status"`
	ExtractionStatus string    `json:"extractionStatus"`
	CanWrite         bool      `json:"canWrite"`
	ProjectCount     int       `json:"projectCount"`
	Projects         []Project `json:"projects"`
	VersionCreatedAt string    `json:"versionCreatedAt"`
}

type Detail struct {
	Item
	Revision                  int64   `json:"revision"`
	SnapshotRevision          int64   `json:"snapshotRevision"`
	SourceSnapshotRevision    int64   `json:"sourceSnapshotRevision"`
	SelectedSpaceID           *string `json:"selectedSpaceId"`
	SelectionMode             string  `json:"selectionMode"`
	SelectedSpaceMissing      bool    `json:"selectedSpaceMissing"`
	HasManualFields           bool    `json:"hasManualFields"`
	Fields                    Fields  `json:"fields"`
	AutoFields                Fields  `json:"autoFields"`
	ExtractionError           string  `json:"extractionError"`
	ConfirmedBy               *string `json:"confirmedBy"`
	ConfirmedAt               *string `json:"confirmedAt"`
	ConfirmedSnapshotRevision *int64  `json:"confirmedSnapshotRevision"`
	EditedBy                  *string `json:"editedBy"`
	EditedAt                  *string `json:"editedAt"`
	CreatedAt                 *string `json:"createdAt"`
	UpdatedAt                 *string `json:"updatedAt"`
	SourcePayload             any     `json:"sourcePayload"`
	DefaultProjectDrawingNo   string  `json:"defaultProjectDrawingNo"`
}

type Page struct {
	List     []Item `json:"list"`
	Total    int    `json:"total"`
	Page     int    `json:"page"`
	PageSize int    `json:"pageSize"`
}

type ListFilter struct {
	Page      int
	PageSize  int
	Keyword   string
	ProjectID string
	Material  string
	Designer  string
	DateFrom  string
	DateTo    string
	Status    string
}

type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type OptionPage struct {
	List    []Option `json:"list"`
	HasMore bool     `json:"hasMore"`
}

type EditInput struct {
	VersionID                string
	ExpectedRevision         int64
	ExpectedSnapshotRevision int64
	Action                   string
	SelectedSpaceID          *string
	Fields                   *Fields
}

type RebuildInput struct {
	VersionID                string
	ExpectedSnapshotRevision int64
}

type BackfillInput struct {
	AfterAttachmentID string
	Limit             int
}

type BackfillResult struct {
	Scanned    int             `json:"scanned"`
	Created    int             `json:"created"`
	Updated    int             `json:"updated"`
	Unchanged  int             `json:"unchanged"`
	Skipped    int             `json:"skipped"`
	Failed     int             `json:"failed"`
	NextCursor *string         `json:"nextCursor"`
	HasMore    bool            `json:"hasMore"`
	Errors     []BackfillError `json:"errors"`
}

type BackfillError struct {
	AttachmentID string `json:"attachmentId"`
	Message      string `json:"message"`
}

type Store interface {
	List(context.Context, ListFilter, string, bool) (Page, error)
	Detail(context.Context, string, string, bool) (Detail, error)
	Options(context.Context, string, string, int, string, bool) (OptionPage, error)
	Edit(context.Context, string, string, bool, EditInput) (Detail, error)
	Rebuild(context.Context, string, string, bool, RebuildInput) (string, Detail, error)
	Backfill(context.Context, BackfillInput) (BackfillResult, error)
}
