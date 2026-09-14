package review

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
)

// ChangeCompletion runs inside the signing transaction. Publication and approval
// must either both commit or both roll back.
type ChangeCompletion func(context.Context, pgx.Tx, string, string, bool, string) error

func (r *PGRepository) SetChangeCompletion(fn ChangeCompletion) { r.changeCompletion = fn }

// StartChangeCase freezes a fresh copy of every configured review node. No
// signature from an earlier version or rejected round can approve this version.
func StartChangeCase(ctx context.Context, tx pgx.Tx, drawingID, submissionID, designerID string) error {
	var flowID, flowName string
	if err := tx.QueryRow(ctx, `SELECT id::text,name FROM review_flows WHERE enabled ORDER BY updated_at DESC LIMIT 1 FOR SHARE`).Scan(&flowID, &flowName); err != nil {
		return fmt.Errorf("请先配置并启用完整审核流程: %w", err)
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
	for i := range nodes {
		if err = tx.QueryRow(ctx, `SELECT COALESCE(display_name,account) FROM users WHERE id=$1::uuid AND status='active'`, nodes[i].AssignedUserID).Scan(&nodes[i].AssignedName); err != nil {
			return fmt.Errorf("审核节点「%s」责任人不可用", nodes[i].Name)
		}
	}
	var caseID string
	if err = tx.QueryRow(ctx, `INSERT INTO review_cases(drawing_id,flow_id,status,initiator_id,change_submission_id,flow_name_snapshot) VALUES($1::uuid,$2::uuid,'reviewing',$3::uuid,$4::uuid,$5) RETURNING id::text`, drawingID, flowID, designerID, submissionID, flowName).Scan(&caseID); err != nil {
		return err
	}
	for _, n := range nodes {
		if _, err = tx.Exec(ctx, `INSERT INTO review_case_nodes(review_case_id,name,assigned_user_id,assigned_name,status,opinion,required,node_order,signer_role) VALUES($1::uuid,$2,$3::uuid,$4,'pending','',$5,$6,$7)`, caseID, n.Name, n.AssignedUserID, n.AssignedName, n.Required, n.Order, n.SignerRole); err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO review_actions(review_case_id,actor_id,action,opinion) VALUES($1::uuid,$2::uuid,'start','变更提交：全部节点重新审核，正式在用版本保持不变')`, caseID, designerID)
	return err
}
