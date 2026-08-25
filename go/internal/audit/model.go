package audit

import "context"

type Log struct {
	ID          string         `json:"id"`
	DrawingNo   string         `json:"drawingNo"`
	DrawingName string         `json:"drawingName"`
	TargetType  string         `json:"targetType"`
	UserID      string         `json:"userId"`
	User        string         `json:"user"`
	Action      string         `json:"act"`
	Summary     string         `json:"txt"`
	OccurredAt  string         `json:"occurredAt"`
	Time        string         `json:"time"`
	Result      string         `json:"result"`
	Detail      map[string]any `json:"detail,omitempty"`
}

type CreateInput struct {
	DrawingNo   string         `json:"drawingNo"`
	DrawingName string         `json:"drawingName"`
	TargetType  string         `json:"targetType"`
	Action      string         `json:"act"`
	Summary     string         `json:"txt"`
	Result      string         `json:"result"`
	Detail      map[string]any `json:"detail"`
}

type ListFilter struct {
	Page      int
	PageSize  int
	Action    string
	DrawingNo string
}

type Page struct {
	List     []Log `json:"list"`
	Total    int   `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"pageSize"`
}

type Repository interface {
	Create(ctx context.Context, input CreateInput, actorID, actorName, ipAddress, userAgent string) (Log, error)
	List(ctx context.Context, filter ListFilter) (Page, error)
}
