package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PGRepository struct {
	pool *pgxpool.Pool
}

func NewPGRepository(pool *pgxpool.Pool) *PGRepository {
	return &PGRepository{pool: pool}
}

func (repository *PGRepository) Create(ctx context.Context, input CreateInput, actorID, actorName, ipAddress, userAgent string) (Log, error) {
	metadata, err := json.Marshal(map[string]any{
		"drawingNo":   input.DrawingNo,
		"drawingName": input.DrawingName,
		"targetType":  input.TargetType,
		"result":      input.Result,
		"detail":      input.Detail,
	})
	if err != nil {
		return Log{}, fmt.Errorf("编码图纸操作日志详情失败: %w", err)
	}

	var id string
	var createdAt time.Time
	var ip any
	if parsed := net.ParseIP(ipAddress); parsed != nil {
		ip = parsed
	}
	err = repository.pool.QueryRow(ctx, `
		INSERT INTO audit_logs (actor_id, action, resource_type, summary, metadata, ip_address, user_agent)
		VALUES ($1::uuid, $2, $3, $4, $5::jsonb, $6, $7)
		RETURNING id::text, created_at`, actorID, input.Action, input.TargetType, input.Summary, metadata, ip, userAgent).Scan(&id, &createdAt)
	if err != nil {
		return Log{}, fmt.Errorf("保存图纸操作日志失败: %w", err)
	}
	return Log{
		ID: id, DrawingNo: input.DrawingNo, DrawingName: input.DrawingName, TargetType: input.TargetType,
		UserID: actorID, User: actorName, Action: input.Action, Summary: input.Summary,
		OccurredAt: createdAt.Format(time.RFC3339Nano), Time: createdAt.Format("2006-01-02 15:04:05"),
		Result: input.Result, Detail: input.Detail,
	}, nil
}

func (repository *PGRepository) List(ctx context.Context, filter ListFilter) (Page, error) {
	where := `WHERE metadata ? 'drawingNo' AND ($1 = '' OR action = $1) AND ($2 = '' OR metadata->>'drawingNo' = $2)`
	var total int
	if err := repository.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs `+where, filter.Action, filter.DrawingNo).Scan(&total); err != nil {
		return Page{}, fmt.Errorf("统计图纸操作日志失败: %w", err)
	}
	offset := (filter.Page - 1) * filter.PageSize
	rows, err := repository.pool.Query(ctx, `
		SELECT a.id::text, COALESCE(a.metadata->>'drawingNo', ''), COALESCE(a.metadata->>'drawingName', ''),
		       a.resource_type, COALESCE(a.actor_id::text, ''), COALESCE(u.display_name, u.account, '未知用户'),
		       a.action, a.summary, a.created_at, COALESCE(a.metadata->>'result', 'success'),
		       COALESCE(a.metadata->'detail', '{}'::jsonb)
		FROM audit_logs a
		LEFT JOIN users u ON u.id = a.actor_id
		`+where+` ORDER BY a.created_at DESC LIMIT $3 OFFSET $4`, filter.Action, filter.DrawingNo, filter.PageSize, offset)
	if err != nil {
		return Page{}, fmt.Errorf("查询图纸操作日志失败: %w", err)
	}
	defer rows.Close()
	items := make([]Log, 0)
	for rows.Next() {
		var item Log
		var createdAt time.Time
		var detail []byte
		if err := rows.Scan(&item.ID, &item.DrawingNo, &item.DrawingName, &item.TargetType, &item.UserID, &item.User, &item.Action, &item.Summary, &createdAt, &item.Result, &detail); err != nil {
			return Page{}, fmt.Errorf("读取图纸操作日志失败: %w", err)
		}
		item.OccurredAt = createdAt.Format(time.RFC3339Nano)
		item.Time = createdAt.Format("2006-01-02 15:04:05")
		if err := json.Unmarshal(detail, &item.Detail); err != nil {
			return Page{}, fmt.Errorf("解析图纸操作日志详情失败: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return Page{}, fmt.Errorf("读取图纸操作日志失败: %w", err)
	}
	return Page{List: items, Total: total, Page: filter.Page, PageSize: filter.PageSize}, nil
}
