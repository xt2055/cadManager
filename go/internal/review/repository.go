package review

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// nodeNameToSignerRole 节点显示名 → 签署角色（与前端映射一致，用于回写图纸签署人员）。
var nodeNameToSignerRole = map[string]string{
	"设计自检":  "设计",
	"校对复核":  "校对",
	"专业审核":  "审核",
	"工艺会签":  "工艺",
	"标准化审查": "标准化",
	"主管批准":  "批准",
}

// pgTimeLayout PostgreSQL to_char 模板（纯数字会被 to_char 当字面量，不能用 Go 参考时间格式）。
const pgTimeLayout = "YYYY-MM-DD HH24:MI"

// goTimeLayout Go time.Format 布局，用于程序内格式化时间字符串。
const goTimeLayout = "2006-01-02 15:04"

type PGRepository struct {
	pool             *pgxpool.Pool
	changeCompletion ChangeCompletion
}

func NewPGRepository(pool *pgxpool.Pool) *PGRepository {
	return &PGRepository{pool: pool}
}

func (repository *PGRepository) List(ctx context.Context) ([]Flow, error) {
	rows, err := repository.pool.Query(ctx, `
		SELECT review_flows.id::text, review_flows.name, review_flows.description, review_flows.enabled, review_flows.created_at, review_flows.updated_at,
		       COALESCE(created_user.display_name, created_user.account, '')
		FROM review_flows
		LEFT JOIN users created_user ON created_user.id = review_flows.created_by
		ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("查询审核流程失败: %w", err)
	}
	defer rows.Close()

	flows := make([]Flow, 0)
	for rows.Next() {
		var item Flow
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&item.ID, &item.Name, &item.Description, &item.Enabled, &createdAt, &updatedAt, &item.CreatedBy); err != nil {
			return nil, fmt.Errorf("读取审核流程失败: %w", err)
		}
		item.CreatedAt = createdAt.Format(time.RFC3339)
		item.UpdatedAt = updatedAt.Format(time.RFC3339)
		item.Nodes, err = repository.loadNodes(ctx, item.ID)
		if err != nil {
			return nil, err
		}
		flows = append(flows, item)
	}
	return flows, rows.Err()
}

func (repository *PGRepository) Create(ctx context.Context, input SaveFlowInput, userID string) (Flow, error) {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return Flow{}, fmt.Errorf("开始创建审核流程事务失败: %w", err)
	}
	defer tx.Rollback(ctx)

	var id string
	err = tx.QueryRow(ctx, `
		INSERT INTO review_flows (name, description, enabled, created_by)
		VALUES ($1, $2, $3, $4::uuid)
		RETURNING id::text`, input.Name, input.Description, input.Enabled, userID).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			return Flow{}, ErrConflict
		}
		return Flow{}, fmt.Errorf("创建审核流程失败: %w", err)
	}
	if err := saveNodes(ctx, tx, id, input.Nodes); err != nil {
		return Flow{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Flow{}, fmt.Errorf("提交审核流程事务失败: %w", err)
	}
	return repository.findByID(ctx, id)
}

func (repository *PGRepository) Find(ctx context.Context, id string) (Flow, error) {
	return repository.findByID(ctx, id)
}

func (repository *PGRepository) Update(ctx context.Context, id string, input SaveFlowInput, userID string) (Flow, error) {
	_ = userID
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return Flow{}, fmt.Errorf("开始修改审核流程事务失败: %w", err)
	}
	defer tx.Rollback(ctx)

	result, err := tx.Exec(ctx, `
		UPDATE review_flows
		SET name = $2, description = $3, enabled = $4, updated_at = now()
		WHERE id = $1::uuid`, id, input.Name, input.Description, input.Enabled)
	if err != nil {
		if isUniqueViolation(err) {
			return Flow{}, ErrConflict
		}
		return Flow{}, fmt.Errorf("修改审核流程失败: %w", err)
	}
	if result.RowsAffected() == 0 {
		return Flow{}, ErrNotFound
	}
	if _, err := tx.Exec(ctx, `DELETE FROM review_flow_nodes WHERE flow_id = $1::uuid`, id); err != nil {
		return Flow{}, fmt.Errorf("清理审核节点失败: %w", err)
	}
	if err := saveNodes(ctx, tx, id, input.Nodes); err != nil {
		return Flow{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Flow{}, fmt.Errorf("提交审核流程事务失败: %w", err)
	}
	return repository.findByID(ctx, id)
}

func (repository *PGRepository) SetEnabled(ctx context.Context, id string, enabled bool, userID string) (Flow, error) {
	_ = userID
	result, err := repository.pool.Exec(ctx, `
		UPDATE review_flows
		SET enabled = $2, updated_at = now()
		WHERE id = $1::uuid`, id, enabled)
	if err != nil {
		return Flow{}, fmt.Errorf("修改审核流程状态失败: %w", err)
	}
	if result.RowsAffected() == 0 {
		return Flow{}, ErrNotFound
	}
	return repository.findByID(ctx, id)
}

func (repository *PGRepository) findByID(ctx context.Context, id string) (Flow, error) {
	var item Flow
	var createdAt, updatedAt time.Time
	err := repository.pool.QueryRow(ctx, `
		SELECT review_flows.id::text, review_flows.name, review_flows.description, review_flows.enabled, review_flows.created_at, review_flows.updated_at,
		       COALESCE(created_user.display_name, created_user.account, '')
		FROM review_flows
		LEFT JOIN users created_user ON created_user.id = review_flows.created_by
		WHERE review_flows.id = $1::uuid`, id).Scan(&item.ID, &item.Name, &item.Description, &item.Enabled, &createdAt, &updatedAt, &item.CreatedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return Flow{}, ErrNotFound
	}
	if err != nil {
		return Flow{}, fmt.Errorf("读取审核流程详情失败: %w", err)
	}
	item.CreatedAt = createdAt.Format(time.RFC3339)
	item.UpdatedAt = updatedAt.Format(time.RFC3339)
	item.Nodes, err = repository.loadNodes(ctx, item.ID)
	return item, err
}

func (repository *PGRepository) loadNodes(ctx context.Context, flowID string) ([]Node, error) {
	rows, err := repository.pool.Query(ctx, `
		SELECT id::text, name, COALESCE(signer_role, ''), COALESCE(candidate_role, 'reviewer'),
		       COALESCE(assigned_user_id::text, ''), assigned_name, required, node_order
		FROM review_flow_nodes
		WHERE flow_id = $1::uuid
		ORDER BY node_order`, flowID)
	if err != nil {
		return nil, fmt.Errorf("查询审核节点失败: %w", err)
	}
	defer rows.Close()
	nodes := make([]Node, 0)
	for rows.Next() {
		var item Node
		if err := rows.Scan(&item.ID, &item.Name, &item.SignerRole, &item.CandidateRole, &item.AssignedUserID, &item.AssignedName, &item.Required, &item.Order); err != nil {
			return nil, fmt.Errorf("读取审核节点失败: %w", err)
		}
		nodes = append(nodes, item)
	}
	return nodes, rows.Err()
}

func saveNodes(ctx context.Context, tx pgx.Tx, flowID string, nodes []Node) error {
	for index, item := range nodes {
		if item.Name == "" {
			return errors.New("审核节点名称不能为空")
		}
		candidateRole := item.CandidateRole
		if candidateRole == "" {
			candidateRole = "reviewer"
		}
		assignedName := item.AssignedName
		if assignedName == "" {
			assignedName = "待定"
		}
		order := item.Order
		if order <= 0 {
			order = index + 1
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO review_flow_nodes (flow_id, name, signer_role, candidate_role, assigned_user_id, assigned_name, required, node_order)
			VALUES ($1::uuid, $2, NULLIF($3, ''), $4, NULLIF($5, '')::uuid, $6, $7, $8)`, flowID, item.Name, item.SignerRole, candidateRole, item.AssignedUserID, assignedName, item.Required, order); err != nil {
			return fmt.Errorf("保存审核节点失败: %w", err)
		}
	}
	return nil
}

func isUniqueViolation(err error) bool {
	// 数据库 locale 可能是中文（错误消息非英文），必须按 SQLSTATE 判断。
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}

// StartCase 为总图发起审核（存在被驳回案例时从驳回节点续审）。
func (repository *PGRepository) StartCase(ctx context.Context, drawingNo string, userID string) (ReviewCase, error) {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return ReviewCase{}, fmt.Errorf("开始发起审核事务失败: %w", err)
	}
	defer tx.Rollback(ctx)

	var drawingID, drawingName, drawingStatus string
	err = tx.QueryRow(ctx, `SELECT id::text, name, status FROM drawings WHERE drawing_no = $1`, drawingNo).Scan(&drawingID, &drawingName, &drawingStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReviewCase{}, fmt.Errorf("未找到待审核对象：%s", drawingNo)
	}
	if err != nil {
		return ReviewCase{}, fmt.Errorf("读取待审核图纸失败: %w", err)
	}
	// 开放的变更工单只能由执行人通过变更提交接口送审，禁止普通入口绕过工单。
	var hasOpenChange bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM change_requests WHERE drawing_id = $1::uuid
		AND status IN ('pending_approval', 'executing', 'pending_verify')
	)`, drawingID).Scan(&hasOpenChange); err != nil {
		return ReviewCase{}, fmt.Errorf("检查变更工单失败: %w", err)
	}
	if hasOpenChange {
		return ReviewCase{}, fmt.Errorf("存在未结束的变更工单，请由指定修改人通过变更工单发起审核: %w", ErrCaseForbidden)
	}
	if drawingStatus == "archived" {
		return ReviewCase{}, fmt.Errorf("在用图纸必须通过变更工单提交新版本审核")
	}
	// 进行中的案例唯一：无论图纸状态字段是否被污染，先查活动案例给出友好冲突提示。
	var activeCaseID string
	err = tx.QueryRow(ctx, `
		SELECT id::text FROM review_cases
		WHERE drawing_id = $1::uuid AND status IN ('pending', 'reviewing')`, drawingID).Scan(&activeCaseID)
	if err == nil {
		return ReviewCase{}, ErrCaseConflict
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ReviewCase{}, fmt.Errorf("检查进行中审核案例失败: %w", err)
	}

	var userName string
	if err := tx.QueryRow(ctx, `SELECT COALESCE(display_name, account, '') FROM users WHERE id = $1::uuid`, userID).Scan(&userName); err != nil {
		return ReviewCase{}, fmt.Errorf("读取发起人信息失败: %w", err)
	}

	var prevCaseID string
	err = tx.QueryRow(ctx, `
		SELECT id::text FROM review_cases
		WHERE drawing_id = $1::uuid AND status = 'rejected'
		ORDER BY started_at DESC LIMIT 1`, drawingID).Scan(&prevCaseID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return ReviewCase{}, fmt.Errorf("读取历史审核案例失败: %w", err)
	}

	var flowID, flowName string
	err = tx.QueryRow(ctx, `SELECT id::text, name FROM review_flows WHERE enabled = true ORDER BY updated_at DESC LIMIT 1 FOR SHARE`).Scan(&flowID, &flowName)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReviewCase{}, fmt.Errorf("未配置启用的审核流程，请联系管理员")
	}
	if err != nil {
		return ReviewCase{}, fmt.Errorf("读取审核流程失败: %w", err)
	}

	// 审核节点始终与后台流程配置同步：配置删除的节点不再进入案例，
	// 非必须且未指定责任人的节点跳过，已通过节点保留签署历史。
	type flowNodeDef struct {
		name           string
		signerRole     string
		assignedUserID string
		assignedName   string
		required       bool
		order          int
	}
	flowNodes := make([]flowNodeDef, 0)
	{
		rows, err := tx.Query(ctx, `
			SELECT name, signer_role, COALESCE(assigned_user_id::text, ''), assigned_name, required, node_order
			FROM review_flow_nodes WHERE flow_id = $1::uuid ORDER BY node_order`, flowID)
		if err != nil {
			return ReviewCase{}, fmt.Errorf("读取审核节点失败: %w", err)
		}
		for rows.Next() {
			var item flowNodeDef
			if err := rows.Scan(&item.name, &item.signerRole, &item.assignedUserID, &item.assignedName, &item.required, &item.order); err != nil {
				rows.Close()
				return ReviewCase{}, fmt.Errorf("解析审核节点失败: %w", err)
			}
			flowNodes = append(flowNodes, item)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return ReviewCase{}, fmt.Errorf("遍历审核节点失败: %w", err)
		}
		rows.Close()
	}
	if len(flowNodes) == 0 {
		return ReviewCase{}, fmt.Errorf("审核流程没有配置任何节点")
	}

	// 上一案例节点（续审时保留已通过记录）。
	prevByName := make(map[string]CaseNode)
	if prevCaseID != "" {
		rows, err := tx.Query(ctx, `
			SELECT name, COALESCE(assigned_user_id::text, ''), assigned_name, status, opinion, required, node_order,
			       COALESCE(to_char(reviewed_at, $2), '')
			FROM review_case_nodes WHERE review_case_id = $1::uuid ORDER BY node_order`, prevCaseID, pgTimeLayout)
		if err != nil {
			return ReviewCase{}, fmt.Errorf("读取历史审核节点失败: %w", err)
		}
		for rows.Next() {
			var node CaseNode
			if err := rows.Scan(&node.Name, &node.AssignedUserID, &node.AssignedName, &node.Status, &node.Opinion, &node.Required, &node.Order, &node.ReviewedAt); err != nil {
				rows.Close()
				return ReviewCase{}, fmt.Errorf("解析历史审核节点失败: %w", err)
			}
			prevByName[node.Name] = node
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return ReviewCase{}, fmt.Errorf("遍历历史审核节点失败: %w", err)
		}
		rows.Close()
	}

	nodes := make([]CaseNode, 0)
	for _, flowItem := range flowNodes {
		signerRole := flowItem.signerRole
		if signerRole == "" {
			signerRole = nodeNameToSignerRole[flowItem.name]
		}
		assignedUserID := flowItem.assignedUserID
		assignedName := flowItem.assignedName
		if signerRole == "设计" || flowItem.name == "设计自检" {
			assignedUserID = userID
			assignedName = userName
		}

		// 已通过节点保留签署历史（签署人/意见/时间不变），顺序与必填性跟随最新配置。
		if prev, ok := prevByName[flowItem.name]; ok && prev.Status == "pass" {
			prev.Order = flowItem.order
			prev.Required = flowItem.required
			nodes = append(nodes, prev)
			continue
		}

		// 未通过节点（含被驳回）一律按当前配置重建为待签；非必须且无责任人直接跳过。
		if assignedUserID == "" {
			if flowItem.required {
				return ReviewCase{}, fmt.Errorf("审核节点「%s」未指定责任人，请在「审核流程管理」中配置", flowItem.name)
			}
			continue
		}
		nodes = append(nodes, CaseNode{
			Name:           flowItem.name,
			AssignedUserID: assignedUserID,
			AssignedName:   assignedName,
			Status:         "pending",
			Required:       flowItem.required,
			Order:          flowItem.order,
		})
	}
	if len(nodes) == 0 {
		return ReviewCase{}, fmt.Errorf("审核流程没有可执行的审核节点")
	}
	// 设计自检直接通过（新进入案例时）。
	for index := range nodes {
		if nodes[index].Name == "设计自检" || nodeNameToSignerRole[nodes[index].Name] == "设计" {
			if nodes[index].Status != "pass" {
				nodes[index].Status = "pass"
				nodes[index].Opinion = "设计完成并自检通过，发起审核流转。"
				nodes[index].ReviewedAt = time.Now().Format(goTimeLayout)
			}
			break
		}
	}

	var caseID string
	err = tx.QueryRow(ctx, `
		INSERT INTO review_cases (drawing_id, flow_id, status, initiator_id,flow_name_snapshot)
		VALUES ($1::uuid, $2::uuid, 'reviewing', $3::uuid,$4)
		RETURNING id::text`, drawingID, flowID, userID, flowName).Scan(&caseID)
	if err != nil {
		if isUniqueViolation(err) {
			return ReviewCase{}, ErrCaseConflict
		}
		return ReviewCase{}, fmt.Errorf("创建审核案例失败: %w", err)
	}
	for _, node := range nodes {
		if _, err := tx.Exec(ctx, `
			INSERT INTO review_case_nodes (review_case_id, name, assigned_user_id, assigned_name, status, opinion, required, node_order, reviewed_at)
			VALUES ($1::uuid, $2, NULLIF($3, '')::uuid, $4, $5, $6, $7, $8, NULLIF($9, '')::timestamptz)`,
			caseID, node.Name, node.AssignedUserID, node.AssignedName, node.Status, node.Opinion, node.Required, node.Order, node.ReviewedAt); err != nil {
			return ReviewCase{}, fmt.Errorf("创建审核节点失败: %w", err)
		}
		// 同步图纸签署人员，保持签署栏与节点责任人一致。
		if role := nodeNameToSignerRole[node.Name]; role != "" {
			if _, err := tx.Exec(ctx, `DELETE FROM drawing_signers WHERE drawing_id = $1::uuid AND role = $2`, drawingID, role); err != nil {
				return ReviewCase{}, fmt.Errorf("清理图纸签署人员失败: %w", err)
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO drawing_signers (drawing_id, role, signer_name) VALUES ($1::uuid, $2, $3)`,
				drawingID, role, node.AssignedName); err != nil {
				return ReviewCase{}, fmt.Errorf("同步图纸签署人员失败: %w", err)
			}
		}
	}
	// 节点已就位：通知第一位责任人审核。
	if err := notifyNodeTurn(ctx, tx, caseID); err != nil {
		return ReviewCase{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO review_actions (review_case_id, actor_id, action, opinion)
		VALUES ($1::uuid, $2::uuid, 'start', $3)`, caseID, userID, "发起审核流转"); err != nil {
		return ReviewCase{}, fmt.Errorf("记录审核动作失败: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE drawings SET status = 'reviewing' WHERE id = $1::uuid`, drawingID); err != nil {
		return ReviewCase{}, fmt.Errorf("更新图纸状态失败: %w", err)
	}
	var pendingCount int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM review_case_nodes WHERE review_case_id=$1::uuid AND status<>'pass'`, caseID).Scan(&pendingCount); err != nil {
		return ReviewCase{}, err
	}
	if pendingCount == 0 {
		if _, err := tx.Exec(ctx, `UPDATE review_cases SET status='published',completed_at=now() WHERE id=$1::uuid`, caseID); err != nil {
			return ReviewCase{}, err
		}
		if _, err := tx.Exec(ctx, `UPDATE drawings SET status='archived',updated_by=$2::uuid WHERE id=$1::uuid`, drawingID, userID); err != nil {
			return ReviewCase{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return ReviewCase{}, fmt.Errorf("提交发起审核事务失败: %w", err)
	}
	return repository.caseByID(ctx, caseID)
}

// ListCases 返回全部审核案例（含节点），供前端工作台与详情页使用。
func (repository *PGRepository) ListCases(ctx context.Context) ([]ReviewCase, error) {
	rows, err := repository.pool.Query(ctx, `
		SELECT c.id::text, d.drawing_no, d.name, COALESCE(NULLIF(c.flow_name_snapshot,''),f.name, ''), c.status,
		       COALESCE(u.display_name, u.account, ''), to_char(c.started_at, $1),
		       COALESCE(to_char(c.completed_at, $1), ''), COALESCE(c.change_submission_id::text,'')
		FROM review_cases c
		JOIN drawings d ON d.id = c.drawing_id
		LEFT JOIN review_flows f ON f.id = c.flow_id
		LEFT JOIN users u ON u.id = c.initiator_id
		ORDER BY c.started_at DESC`, pgTimeLayout)
	if err != nil {
		return nil, fmt.Errorf("查询审核案例失败: %w", err)
	}
	defer rows.Close()

	cases := make([]ReviewCase, 0)
	for rows.Next() {
		var item ReviewCase
		if err := rows.Scan(&item.ID, &item.DrawingNo, &item.DrawingName, &item.FlowName, &item.Status, &item.Initiator, &item.StartedAt, &item.CompletedAt, &item.ChangeSubmissionID); err != nil {
			return nil, fmt.Errorf("读取审核案例失败: %w", err)
		}
		cases = append(cases, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历审核案例失败: %w", err)
	}
	for index := range cases {
		cases[index].Nodes, err = repository.caseNodes(ctx, cases[index].ID)
		if err != nil {
			return nil, err
		}
	}
	return cases, nil
}

// SubmitNode 签署审核节点：强制顺序与责任人校验。
func (repository *PGRepository) SubmitNode(ctx context.Context, caseID string, input SubmitNodeInput, userID string) (ReviewCase, error) {
	action := input.Action
	if action != "pass" && action != "rejected" {
		return ReviewCase{}, fmt.Errorf("无效的审核操作")
	}

	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return ReviewCase{}, fmt.Errorf("开始签署事务失败: %w", err)
	}
	defer tx.Rollback(ctx)

	// Lock the work order before its review, matching cancellation and resubmission.
	if _, err := tx.Exec(ctx, `SELECT cr.id FROM change_requests cr JOIN change_request_submissions s ON s.request_id=cr.id JOIN review_cases c ON c.change_submission_id=s.id WHERE c.id=$1::uuid FOR UPDATE OF cr`, caseID); err != nil {
		return ReviewCase{}, err
	}
	var caseStatus, drawingID, changeSubmissionID string
	err = tx.QueryRow(ctx, `
		SELECT c.status, COALESCE(c.drawing_id::text, ''), COALESCE(c.change_submission_id::text,'')
		FROM review_cases c WHERE c.id = $1::uuid FOR UPDATE`, caseID).Scan(&caseStatus, &drawingID, &changeSubmissionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReviewCase{}, ErrCaseNotFound
	}
	if err != nil {
		return ReviewCase{}, fmt.Errorf("读取审核案例失败: %w", err)
	}
	if caseStatus != "reviewing" {
		return ReviewCase{}, fmt.Errorf("审核流程已结束，无法继续签署")
	}

	var nodeID, nodeStatus, assignedUserID, assignedName string
	var nodeOrder int
	err = tx.QueryRow(ctx, `
		SELECT id::text, status, COALESCE(assigned_user_id::text, ''), assigned_name, node_order
		FROM review_case_nodes WHERE review_case_id = $1::uuid AND name = $2`, caseID, input.NodeName).
		Scan(&nodeID, &nodeStatus, &assignedUserID, &assignedName, &nodeOrder)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReviewCase{}, fmt.Errorf("未找到审核节点：%s", input.NodeName)
	}
	if err != nil {
		return ReviewCase{}, fmt.Errorf("读取审核节点失败: %w", err)
	}
	if nodeStatus != "pending" {
		return ReviewCase{}, fmt.Errorf("审核节点「%s」当前不是待处理状态，无法签署", input.NodeName)
	}
	// 顺序守卫：只允许处理顺序最靠前的待处理节点。
	var minOrder int
	var minName string
	err = tx.QueryRow(ctx, `
		SELECT node_order, name FROM review_case_nodes
		WHERE review_case_id = $1::uuid AND status = 'pending'
		ORDER BY node_order LIMIT 1`, caseID).Scan(&minOrder, &minName)
	if err != nil {
		return ReviewCase{}, fmt.Errorf("读取当前活动节点失败: %w", err)
	}
	if minOrder != nodeOrder {
		return ReviewCase{}, fmt.Errorf("审核须按顺序流转：请先处理当前节点「%s」", minName)
	}
	// 责任人守卫：仅节点责任人可签署。
	if assignedUserID == "" || assignedUserID != userID {
		return ReviewCase{}, fmt.Errorf("节点「%s」由「%s」负责，当前登录人无权签署", input.NodeName, assignedName)
	}

	opinion := input.Opinion
	if strings.TrimSpace(opinion) == "" {
		if action == "pass" {
			opinion = "同意通过。"
		} else {
			opinion = "审核驳回，请按意见修正后重新提交。"
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE review_case_nodes SET status = $1, opinion = $2, reviewed_at = now()
		WHERE id = $3::uuid`, action, opinion, nodeID); err != nil {
		return ReviewCase{}, fmt.Errorf("更新审核节点失败: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO review_actions (review_case_id, node_id, actor_id, action, opinion)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5)`,
		caseID, nodeID, userID, map[string]string{"pass": "pass", "rejected": "reject"}[action], opinion); err != nil {
		return ReviewCase{}, fmt.Errorf("记录审核动作失败: %w", err)
	}

	if action == "rejected" {
		if _, err := tx.Exec(ctx, `UPDATE review_cases SET status = 'rejected', completed_at = now() WHERE id = $1::uuid`, caseID); err != nil {
			return ReviewCase{}, fmt.Errorf("更新审核案例状态失败: %w", err)
		}
		if changeSubmissionID != "" {
			if repository.changeCompletion == nil {
				return ReviewCase{}, fmt.Errorf("变更审核发布服务未配置")
			}
			if err := repository.changeCompletion(ctx, tx, changeSubmissionID, userID, false, opinion); err != nil {
				return ReviewCase{}, err
			}
		} else if drawingID != "" {
			if _, err := tx.Exec(ctx, `UPDATE drawings SET status = 'draft' WHERE id = $1::uuid`, drawingID); err != nil {
				return ReviewCase{}, fmt.Errorf("更新图纸状态失败: %w", err)
			}
		}
	} else {
		var requiredPending int
		if err := tx.QueryRow(ctx, `
			SELECT COUNT(*) FROM review_case_nodes
			WHERE review_case_id = $1::uuid AND status <> 'pass'`, caseID).Scan(&requiredPending); err != nil {
			return ReviewCase{}, fmt.Errorf("检查审核进度失败: %w", err)
		}
		if requiredPending == 0 {
			if _, err := tx.Exec(ctx, `UPDATE review_cases SET status = 'published', completed_at = now() WHERE id = $1::uuid`, caseID); err != nil {
				return ReviewCase{}, fmt.Errorf("更新审核案例状态失败: %w", err)
			}
			if changeSubmissionID != "" {
				if repository.changeCompletion == nil {
					return ReviewCase{}, fmt.Errorf("变更审核发布服务未配置")
				}
				if err := repository.changeCompletion(ctx, tx, changeSubmissionID, userID, true, opinion); err != nil {
					return ReviewCase{}, err
				}
			} else if drawingID != "" {
				if _, err := tx.Exec(ctx, `UPDATE drawings SET status = 'archived',updated_by=$2::uuid WHERE id = $1::uuid`, drawingID, userID); err != nil {
					return ReviewCase{}, fmt.Errorf("更新图纸状态失败: %w", err)
				}
			}
		} else {
			// 本轮未结束：待签节点推进到下一个责任人，通知新的当前责任人。
			if err := notifyNodeTurn(ctx, tx, caseID); err != nil {
				return ReviewCase{}, err
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return ReviewCase{}, fmt.Errorf("提交签署事务失败: %w", err)
	}
	return repository.caseByID(ctx, caseID)
}

// ActiveCaseAssigneeByDrawingNo 查询审核中案例当前活动节点（node_order 最小的 pending 节点）的责任人用户 ID。
func (repository *PGRepository) ActiveCaseAssigneeByDrawingNo(ctx context.Context, drawingNo string) (string, error) {
	var assignee string
	err := repository.pool.QueryRow(ctx, `
		SELECT COALESCE(n.assigned_user_id::text, '')
		FROM review_cases c
		JOIN drawings d ON d.id = c.drawing_id
		JOIN review_case_nodes n ON n.review_case_id = c.id
		WHERE d.drawing_no = $1 AND c.status = 'reviewing' AND n.status = 'pending'
		ORDER BY n.node_order
		LIMIT 1`, drawingNo).Scan(&assignee)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("查询审核节点责任人失败: %w", err)
	}
	return assignee, nil
}

// CompletedActions 返回已办审核归档（通过/驳回动作）。
func (repository *PGRepository) CompletedActions(ctx context.Context) ([]CompletedAction, error) {
	rows, err := repository.pool.Query(ctx, `
		SELECT a.id::text, c.id::text, d.drawing_no, d.name, COALESCE(n.name, ''),
		       COALESCE(initiator.display_name, initiator.account, ''),
		       COALESCE(actor.display_name, actor.account, ''),
		       to_char(a.created_at, $1), a.action, a.opinion, d.version
		FROM review_actions a
		JOIN review_cases c ON c.id = a.review_case_id
		JOIN drawings d ON d.id = c.drawing_id
		LEFT JOIN review_case_nodes n ON n.id = a.node_id
		LEFT JOIN users initiator ON initiator.id = c.initiator_id
		LEFT JOIN users actor ON actor.id = a.actor_id
		WHERE a.action IN ('pass', 'reject')
		ORDER BY a.created_at DESC`, pgTimeLayout)
	if err != nil {
		return nil, fmt.Errorf("查询已办审核失败: %w", err)
	}
	defer rows.Close()

	items := make([]CompletedAction, 0)
	for rows.Next() {
		var item CompletedAction
		var action string
		if err := rows.Scan(&item.ID, &item.CaseID, &item.DrawingNo, &item.Name, &item.NodeName, &item.Initiator, &item.Reviewer, &item.Time, &action, &item.Opinion, &item.Ver); err != nil {
			return nil, fmt.Errorf("读取已办审核失败: %w", err)
		}
		if action == "reject" {
			item.Result = "rejected"
		} else {
			item.Result = "pass"
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (repository *PGRepository) caseByID(ctx context.Context, caseID string) (ReviewCase, error) {
	var item ReviewCase
	err := repository.pool.QueryRow(ctx, `
		SELECT c.id::text, d.drawing_no, d.name, COALESCE(NULLIF(c.flow_name_snapshot,''),f.name, ''), c.status,
		       COALESCE(u.display_name, u.account, ''), to_char(c.started_at, $1),
		       COALESCE(to_char(c.completed_at, $1), ''), COALESCE(c.change_submission_id::text,'')
		FROM review_cases c
		JOIN drawings d ON d.id = c.drawing_id
		LEFT JOIN review_flows f ON f.id = c.flow_id
		LEFT JOIN users u ON u.id = c.initiator_id
		WHERE c.id = $2::uuid`, pgTimeLayout, caseID).
		Scan(&item.ID, &item.DrawingNo, &item.DrawingName, &item.FlowName, &item.Status, &item.Initiator, &item.StartedAt, &item.CompletedAt, &item.ChangeSubmissionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReviewCase{}, ErrCaseNotFound
	}
	if err != nil {
		return ReviewCase{}, fmt.Errorf("读取审核案例详情失败: %w", err)
	}
	item.Nodes, err = repository.caseNodes(ctx, caseID)
	return item, err
}

func (repository *PGRepository) caseNodes(ctx context.Context, caseID string) ([]CaseNode, error) {
	rows, err := repository.pool.Query(ctx, `
		SELECT name, COALESCE(assigned_user_id::text, ''), assigned_name, status, opinion, required, node_order,
		       COALESCE(to_char(reviewed_at, $2), '')
		FROM review_case_nodes WHERE review_case_id = $1::uuid ORDER BY node_order`, caseID, pgTimeLayout)
	if err != nil {
		return nil, fmt.Errorf("查询审核节点失败: %w", err)
	}
	defer rows.Close()
	nodes := make([]CaseNode, 0)
	for rows.Next() {
		var node CaseNode
		if err := rows.Scan(&node.Name, &node.AssignedUserID, &node.AssignedName, &node.Status, &node.Opinion, &node.Required, &node.Order, &node.ReviewedAt); err != nil {
			return nil, fmt.Errorf("读取审核节点失败: %w", err)
		}
		nodes = append(nodes, node)
	}
	return nodes, rows.Err()
}
