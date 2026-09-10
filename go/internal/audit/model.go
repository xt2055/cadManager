package audit

import (
	"context"
	"time"
)

type Log struct {
	ID          string         `json:"id"`
	DrawingNo   string         `json:"drawingNo"`
	DrawingName string         `json:"drawingName"`
	TargetType  string         `json:"targetType"`
	UserID      string         `json:"userId"`
	User        string         `json:"user"`
	UserAccount string         `json:"userAccount,omitempty"`
	Action      string         `json:"act"`
	Summary     string         `json:"txt"`
	OccurredAt  string         `json:"occurredAt"`
	Time        string         `json:"time"`
	Result      string         `json:"result"`
	IPAddress   string         `json:"ipAddress,omitempty"`
	UserAgent   string         `json:"userAgent,omitempty"`
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
	Page       int
	PageSize   int
	Action     string
	DrawingNo  string
	TargetType string
	ActorID    string
	Actor      string
	Result     string
	Keyword    string
	From       time.Time
	To         time.Time
}

type Page struct {
	List     []Log `json:"list"`
	Total    int   `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"pageSize"`
}

type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type OptionPage struct {
	List []Option `json:"list"`
}

type OptionFilter struct {
	Kind      string
	Keyword   string
	Limit     int
	AdminOnly bool
}

type Repository interface {
	Create(ctx context.Context, input CreateInput, actorID, actorName, ipAddress, userAgent string) (Log, error)
	List(ctx context.Context, filter ListFilter) (Page, error)
}

type AdminRepository interface {
	ListAdmin(ctx context.Context, filter ListFilter) (Page, error)
}

type OptionRepository interface {
	Options(ctx context.Context, filter OptionFilter) (OptionPage, error)
}
