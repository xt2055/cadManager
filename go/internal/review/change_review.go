package review

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
)

// ChangeCompletion runs inside the signing transaction. Publication and approval
// must either both commit or both roll back.
type ChangeCompletion func(context.Context, pgx.Tx, string, string, bool, string) error

func (r *PGRepository) SetChangeCompletion(fn ChangeCompletion) { r.changeCompletion = fn }

// StartChangeCase freezes the configured review nodes for a change submission.
// It mirrors the initial review flow: nodes already approved in the latest
// rejected round of the same change request are carried over, so resubmitting
// after a rejection resumes from the rejected node while every other node must
// be signed again. Fresh submissions with no rejected history start from scratch,
// and no signature from an earlier version can approve an unsigned node.
func StartChangeCase(ctx context.Context, tx pgx.Tx, drawingID, submissionID, designerID string) error {
	var flowID, flowName string
	if err := tx.QueryRow(ctx, `SELECT id::text,name FROM review_flows WHERE enabled ORDER BY updated_at DESC LIMIT 1 FOR SHARE`).Scan(&flowID, &flowName); err != nil {
		return fmt.Errorf("请先配置并启用完整审核流程: %w", err)
	}

	// 本轮提交所属工单，用于定位同一工单最近一次被驳回的审核单。
	var requestID string
	if err := tx.QueryRow(ctx, `SELECT request_id::text FROM change_request_submissions WHERE id=$1::uuid`, submissionID).Scan(&requestID); err != nil {
		return fmt.Errorf("变更提交轮次不存在: %w", err)
	}
	var prevCaseID string
	err := tx.QueryRow(ctx, `
		SELECT rc.id::text FROM review_cases rc
		JOIN change_request_submissions s ON s.id = rc.change_submission_id
		WHERE s.request_id = $1::uuid AND rc.status = 'rejected'
		ORDER BY rc.started_at DESC LIMIT 1`, requestID).Scan(&prevCaseID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("读取历史变更审核失败: %w", err)
	}
	// 上一轮已通过节点（续审时保留签署记录）。
	prevPass := make(map[string]CaseNode)
	if prevCaseID != "" {
		rows, err := tx.Query(ctx, `
			SELECT name, COALESCE(signer_role, ''), COALESCE(assigned_user_id::text, ''), assigned_name, opinion,
			       COALESCE(to_char(reviewed_at, $2), '')
			FROM review_case_nodes WHERE review_case_id = $1::uuid AND status = 'pass'`, prevCaseID, pgTimeLayout)
		if err != nil {
			return fmt.Errorf("读取历史已通过节点失败: %w", err)
		}
		for rows.Next() {
			var node CaseNode
			if err := rows.Scan(&node.Name, &node.SignerRole, &node.AssignedUserID, &node.AssignedName, &node.Opinion, &node.ReviewedAt); err != nil {
				rows.Close()
				return fmt.Errorf("解析历史已通过节点失败: %w", err)
			}
			node.Status = "pass"
			prevPass[node.Name] = node
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
	}

	rows, err := tx.Query(ctx, `SELECT name,signer_role,COALESCE(assigned_user_id::text,''),assigned_name,required,node_order FROM review_flow_nodes WHERE flow_id=$1::uuid ORDER BY node_order`, flowID)
	if err != nil {
		return err
	}
	var nodes []CaseNode
	for rows.Next() {
		var n CaseNode
		var role string
		if err = rows.Scan(&n.Name, &role, &n.AssignedUserID, &n.AssignedName, &n.Required, &n.Order); err != nil {
			rows.Close()
			return err
		}
		if role == "" {
			role = nodeNameToSignerRole[n.Name]
		}
		n.SignerRole = role
		// 已通过节点保留签署历史，顺序与必填性跟随最新配置。
		if prev, ok := prevPass[n.Name]; ok {
			prev.Order = n.Order
			prev.Required = n.Required
			nodes = append(nodes, prev)
			continue
		}
		if role == "设计" || n.Name == "设计自检" {
			n.AssignedUserID = designerID
			n.AssignedName = "变更设计员"
		}
		if n.AssignedUserID == "" {
			if n.Required {
				rows.Close()
				return fmt.Errorf("审核节点「%s」尚未指定责任人", n.Name)
			}
			continue
		}
		n.Status = "pending"
		nodes = append(nodes, n)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(nodes) == 0 {
		return fmt.Errorf("审核流程没有可执行节点")
	}
	// 仅校验待签节点的责任人可用；已通过节点是历史签署记录，不因账号停用而失效。
	for i := range nodes {
		if nodes[i].Status == "pass" {
			continue
		}
		if err = tx.QueryRow(ctx, `SELECT COALESCE(display_name,account) FROM users WHERE id=$1::uuid AND status='active'`, nodes[i].AssignedUserID).Scan(&nodes[i].AssignedName); err != nil {
			return fmt.Errorf("审核节点「%s」责任人不可用", nodes[i].Name)
		}
	}
	var caseID string
	if err = tx.QueryRow(ctx, `INSERT INTO review_cases(drawing_id,flow_id,status,initiator_id,change_submission_id,flow_name_snapshot) VALUES($1::uuid,$2::uuid,'reviewing',$3::uuid,$4::uuid,$5) RETURNING id::text`, drawingID, flowID, designerID, submissionID, flowName).Scan(&caseID); err != nil {
		return err
	}
	for _, n := range nodes {
		if _, err = tx.Exec(ctx, `INSERT INTO review_case_nodes(review_case_id,name,assigned_user_id,assigned_name,status,opinion,required,node_order,signer_role) VALUES($1::uuid,$2,$3::uuid,$4,$5,$6,$7,$8,$9)`, caseID, n.Name, n.AssignedUserID, n.AssignedName, n.Status, n.Opinion, n.Required, n.Order, n.SignerRole); err != nil {
			return err
		}
	}
	opinion := "变更提交：全部节点重新审核，正式在用版本保持不变"
	if prevCaseID != "" {
		opinion = "变更提交：延续上一轮已通过节点，从驳回节点继续审核，正式在用版本保持不变"
	}
	_, err = tx.Exec(ctx, `INSERT INTO review_actions(review_case_id,actor_id,action,opinion) VALUES($1::uuid,$2::uuid,'start',$3)`, caseID, designerID, opinion)
	return err
}
