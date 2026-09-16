package review

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// turnQuerier 让同一个实现既能跑在业务事务里，也能跑在启动补齐的裸连接池上。
type turnQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// notifyNodeTurn 在待签节点落到具体责任人时写入「轮到你审核」通知。
//
// 判定口径与签署守卫完全一致：当前节点是该案例中 node_order 最小的 pending 节点，
// 且只有 assigned_user_id 非空的节点才能被签署（见 SubmitNode 的责任人守卫），
// 因此未指定责任人的节点不会产生提醒。通知与业务动作共用同一事务，回滚不留痕；
// event_key 以节点 ID 去重，同一节点只提醒一次。
//
// 这里不用数据库触发器，是因为正常启动只补建零件索引相关的历史迁移，
// 新增迁移不会在已安装环境中执行，触发器会静默缺失。
func notifyNodeTurn(ctx context.Context, q turnQuerier, caseID string) error {
	var nodeID, nodeName, recipient string
	err := q.QueryRow(ctx, `
		SELECT id::text, name, COALESCE(assigned_user_id::text, '')
		FROM review_case_nodes
		WHERE review_case_id = $1::uuid AND status = 'pending'
		ORDER BY node_order LIMIT 1`, caseID).Scan(&nodeID, &nodeName, &recipient)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("读取当前审核节点失败: %w", err)
	}
	if recipient == "" {
		return nil
	}

	var status, initiator, drawingID, drawingNo, drawingName string
	err = q.QueryRow(ctx, `
		SELECT c.status, COALESCE(c.initiator_id::text, ''), COALESCE(c.drawing_id::text, ''),
		       COALESCE(d.drawing_no, ''), COALESCE(d.name, '')
		FROM review_cases c LEFT JOIN drawings d ON d.id = c.drawing_id
		WHERE c.id = $1::uuid`, caseID).Scan(&status, &initiator, &drawingID, &drawingNo, &drawingName)
	if err != nil {
		return fmt.Errorf("读取审核案例失败: %w", err)
	}
	if status != "reviewing" {
		return nil
	}

	label := strings.Trim(strings.TrimSpace(drawingNo+" · "+drawingName), " ·")
	if label == "" {
		label = "图纸审核"
	}
	content := label + "\n当前节点「" + nodeName + "」由你签署，请前往审核工作台处理。"
	if _, err = q.Exec(ctx, `
		INSERT INTO notifications(recipient_id, sender_id, kind, title, content, drawing_id, event_key)
		VALUES ($1::uuid, NULLIF($2, '')::uuid, 'review', '轮到你审核', $3, NULLIF($4, '')::uuid, 'review-turn:' || $5)
		ON CONFLICT (recipient_id, event_key) DO NOTHING`,
		recipient, initiator, content, drawingID, nodeID); err != nil {
		return fmt.Errorf("写入待办审核通知失败: %w", err)
	}
	return nil
}

// EnsureTurnNotifications 为所有正在等签的节点补齐提醒，供服务启动时调用。
// 幂等：已提醒过的节点由 (recipient_id, event_key) 唯一约束挡掉，不会重复打扰。
// 覆盖两类场景：本次改动上线前已存在的待办审核，以及责任人离线期间推进的节点。
func EnsureTurnNotifications(ctx context.Context, pool interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	turnQuerier
}) error {
	rows, err := pool.Query(ctx, `
		SELECT DISTINCT c.id::text
		FROM review_cases c
		JOIN review_case_nodes n ON n.review_case_id = c.id
		WHERE c.status = 'reviewing' AND n.status = 'pending' AND n.assigned_user_id IS NOT NULL`)
	if err != nil {
		return fmt.Errorf("查询待签审核案例失败: %w", err)
	}
	var caseIDs []string
	for rows.Next() {
		var caseID string
		if err = rows.Scan(&caseID); err != nil {
			rows.Close()
			return fmt.Errorf("解析待签审核案例失败: %w", err)
		}
		caseIDs = append(caseIDs, caseID)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return fmt.Errorf("读取待签审核案例失败: %w", err)
	}
	for _, caseID := range caseIDs {
		if err = notifyNodeTurn(ctx, pool, caseID); err != nil {
			return err
		}
	}
	return nil
}
