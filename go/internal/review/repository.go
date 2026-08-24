package review

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PGRepository struct {
	pool *pgxpool.Pool
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
	return strings.Contains(err.Error(), "duplicate key value") || strings.Contains(err.Error(), "unique constraint")
}
