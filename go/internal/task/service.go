package task

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"cadguanliq/internal/auth"
	"cadguanliq/internal/drawing"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Service 是图纸任务的应用服务。
// 指派本身就是任务记录：改派把旧行置为 replaced 并新增 active 行，
// 因此同一图纸同时只有一条有效负责人，而历史指派与改派原因可追溯。
type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// rowSelect 是任务行的统一查询：图纸 + 有效指派 + 图纸文件数 + 当前审核节点进度。
// 三张 LEFT JOIN 都不会放大行数（LATERAL 子查询各自最多返回一行），
// 因此可以安全地和 count / summary 共用同一套 WHERE 条件。
const rowSelect = `
SELECT d.id::text, d.drawing_no, d.name, d.project, d.status, d.version, d.created_at, d.updated_at,
       COALESCE(files.file_count, 0),
       COALESCE(review.node_name, ''), COALESCE(review.done_count, 0), COALESCE(review.total_count, 0),
       COALESCE(t.id::text, ''), COALESCE(t.assignee_id::text, ''),
       COALESCE(assignee.display_name, assignee.account, ''),
       COALESCE(assigner.display_name, assigner.account, ''),
       COALESCE(t.note, ''), COALESCE(to_char(t.due_date, 'YYYY-MM-DD'), ''), t.created_at
FROM drawings d
LEFT JOIN drawing_tasks t ON t.drawing_id = d.id AND t.status = 'active'
LEFT JOIN users assignee ON assignee.id = t.assignee_id
LEFT JOIN users assigner ON assigner.id = t.assigned_by
LEFT JOIN LATERAL (
    SELECT count(*) AS file_count
    FROM attachments a
    WHERE a.deleted_at IS NULL AND (
        a.drawing_id = d.id
        OR EXISTS (SELECT 1 FROM drawing_part_relations r
                   WHERE r.drawing_id = d.id AND r.part_id = a.part_id AND r.status = 'active')
    )
) files ON true
LEFT JOIN LATERAL (
    SELECT
        (SELECT cn.name FROM review_case_nodes cn
          WHERE cn.review_case_id = c.id AND cn.status = 'pending'
          ORDER BY cn.node_order LIMIT 1) AS node_name,
        (SELECT count(*) FROM review_case_nodes cn WHERE cn.review_case_id = c.id AND cn.status = 'pass') AS done_count,
        (SELECT count(*) FROM review_case_nodes cn WHERE cn.review_case_id = c.id) AS total_count
    FROM review_cases c
    WHERE c.drawing_id = d.id AND c.status IN ('pending', 'reviewing')
    ORDER BY c.started_at DESC
    LIMIT 1
) review ON true
`

const activeTaskJoin = `LEFT JOIN drawing_tasks t ON t.drawing_id = d.id AND t.status = 'active'`

// 统计口径与列表共用同一套条件，翻页不会改变统计数字。
const summarySelect = `
SELECT count(*),
       count(t.id),
       count(*) FILTER (WHERE t.id IS NULL),
       count(*) FILTER (WHERE t.id IS NOT NULL AND d.status IN ('draft', 'reviewing')),
       count(*) FILTER (WHERE t.id IS NOT NULL AND d.status IN ('published', 'archived')),
       count(*) FILTER (WHERE t.id IS NOT NULL AND t.due_date IS NOT NULL AND t.due_date < current_date
                              AND d.status IN ('draft', 'reviewing'))
FROM drawings d ` + activeTaskJoin

// conditionBuilder 按加入顺序生成占位符，避免手写 $1/$2 在增删条件时错位。
type conditionBuilder struct {
	clauses []string
	args    []any
}

func (b *conditionBuilder) add(value any, format string) {
	b.args = append(b.args, value)
	b.clauses = append(b.clauses, fmt.Sprintf(format, len(b.args)))
}

func (b *conditionBuilder) where() string {
	if len(b.clauses) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(b.clauses, " AND ")
}

func (b *conditionBuilder) orderBy(clause string, page, pageSize int) string {
	// 排序片段由调用方以常量给出，不含用户输入。
	args := make([]any, 0, len(b.args)+2)
	args = append(args, b.args...)
	args = append(args, pageSize, (page-1)*pageSize)
	return clause + fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)-1, len(args))
}

// buildConditions 把筛选条件翻译成 SQL 片段。
// assigneeID 非空时额外限定只属于某位负责人（首页任务面板与个人筛选共用）。
func buildConditions(filter ListFilter, assigneeID string) *conditionBuilder {
	builder := &conditionBuilder{}
	drawn := strings.TrimSpace(filter.Keyword)
	if drawn != "" {
		builder.add("%"+strings.ToLower(drawn)+"%", `(lower(d.drawing_no) LIKE $%[1]d OR lower(d.name) LIKE $%[1]d OR lower(d.project) LIKE $%[1]d)`)
	}
	if status := strings.TrimSpace(filter.Status); status != "" && validDrawingStatus(status) {
		builder.add(status, `d.status = $%d`)
	}
	switch strings.TrimSpace(filter.Assigned) {
	case "assigned":
		builder.clauses = append(builder.clauses, `t.id IS NOT NULL`)
	case "unassigned":
		builder.clauses = append(builder.clauses, `t.id IS NULL`)
	}
	if id := strings.TrimSpace(assigneeID); id != "" {
		builder.add(id, `t.assignee_id = $%d::uuid`)
	}
	if id := strings.TrimSpace(filter.AssigneeID); id != "" {
		builder.add(id, `t.assignee_id = $%d::uuid`)
	}
	return builder
}

func normalizePaging(filter ListFilter) (int, int) {
	page := filter.Page
	if page < 1 {
		page = 1
	}
	size := filter.PageSize
	if size < 1 || size > 200 {
		size = 20
	}
	return page, size
}

// ListBoard 返回计划视角的图纸总表：所有图纸 + 当前负责人，未指派排在最前，
// 因为计划员打开这个页面最先要处理的就是「还有哪些图没派出去」。
func (service *Service) ListBoard(ctx context.Context, filter ListFilter) (Page, error) {
	if service == nil || service.pool == nil {
		return Page{}, errors.New("任务服务未配置")
	}
	return service.list(ctx, filter, buildConditions(filter, ""))
}

// ListMine 返回「我负责的图纸」，即首页任务面板的数据源。
func (service *Service) ListMine(ctx context.Context, userID string, filter ListFilter) (Page, error) {
	if service == nil || service.pool == nil {
		return Page{}, errors.New("任务服务未配置")
	}
	if strings.TrimSpace(userID) == "" {
		return Page{}, ErrForbidden
	}
	return service.list(ctx, filter, buildConditions(filter, userID))
}

func (service *Service) list(ctx context.Context, filter ListFilter, builder *conditionBuilder) (Page, error) {
	page, pageSize := normalizePaging(filter)
	where := builder.where()

	var summary Summary
	if err := service.pool.QueryRow(ctx, summarySelect+where, builder.args...).Scan(
		&summary.Total, &summary.Assigned, &summary.Unassigned, &summary.Active, &summary.Done, &summary.Overdue,
	); err != nil {
		return Page{}, fmt.Errorf("统计任务失败: %w", err)
	}

	var total int
	if err := service.pool.QueryRow(ctx, `SELECT count(*) FROM drawings d `+activeTaskJoin+where, builder.args...).Scan(&total); err != nil {
		return Page{}, fmt.Errorf("统计图纸失败: %w", err)
	}

	// 未指派优先：计划员的时间花在「还能派给谁」，而不是重排已派的活。
	// 已指派的行按截止日期（最近在前）再按图纸更新时间，逾期的自然浮到上面。
	order := ` ORDER BY (t.id IS NULL) DESC, t.due_date NULLS LAST, d.updated_at DESC, d.drawing_no`
	statement := rowSelect + where + builder.orderBy(order, page, pageSize)
	args := append([]any{}, builder.args...)
	args = append(args, pageSize, (page-1)*pageSize)

	rows, err := service.pool.Query(ctx, statement, args...)
	if err != nil {
		return Page{}, fmt.Errorf("查询任务失败: %w", err)
	}
	defer rows.Close()
	items := make([]Row, 0)
	for rows.Next() {
		item, scanErr := scanRow(rows)
		if scanErr != nil {
			return Page{}, scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return Page{}, fmt.Errorf("读取任务失败: %w", err)
	}
	return Page{List: items, Total: total, Page: page, PageSize: pageSize, Summary: summary}, nil
}

func scanRow(rows pgx.Rows) (Row, error) {
	var item Row
	var createdAt, updatedAt time.Time
	var taskID, assigneeID, assigneeName, assignedByName, note, dueDate string
	var assignedAt *time.Time
	if err := rows.Scan(
		&item.Drawing.ID, &item.Drawing.No, &item.Drawing.Name, &item.Drawing.Project,
		&item.Drawing.Status, &item.Drawing.Version, &createdAt, &updatedAt,
		&item.Drawing.FileCount, &item.Drawing.ReviewNode, &item.Drawing.ReviewDone, &item.Drawing.ReviewTotal,
		&taskID, &assigneeID, &assigneeName, &assignedByName, &note, &dueDate, &assignedAt,
	); err != nil {
		return Row{}, fmt.Errorf("解析任务行失败: %w", err)
	}
	item.Drawing.CreatedAt = parseTime(createdAt)
	item.Drawing.UpdatedAt = parseTime(updatedAt)
	if taskID != "" {
		assignment := &Assignment{
			TaskID:     taskID,
			AssigneeID: assigneeID,
			Assignee:   assigneeName,
			AssignedBy: assignedByName,
			Note:       note,
			DueDate:    dueDate,
		}
		if assignedAt != nil {
			assignment.AssignedAt = parseTime(*assignedAt)
		}
		item.Assignment = assignment
	}
	item.Progress = Derive(item.Drawing.Status, item.Drawing.FileCount, item.Drawing.ReviewNode, item.Drawing.ReviewDone, item.Drawing.ReviewTotal)
	return item, nil
}

// loadRow 重新读取单张图纸的任务行，用于写操作后返回权威结果。
func (service *Service) loadRow(ctx context.Context, drawingID string) (Row, error) {
	row := service.pool.QueryRow(ctx, rowSelect+` WHERE d.id = $1::uuid`, drawingID)
	var item Row
	var createdAt, updatedAt time.Time
	var taskID, assigneeID, assigneeName, assignedByName, note, dueDate string
	var assignedAt *time.Time
	if err := row.Scan(
		&item.Drawing.ID, &item.Drawing.No, &item.Drawing.Name, &item.Drawing.Project,
		&item.Drawing.Status, &item.Drawing.Version, &createdAt, &updatedAt,
		&item.Drawing.FileCount, &item.Drawing.ReviewNode, &item.Drawing.ReviewDone, &item.Drawing.ReviewTotal,
		&taskID, &assigneeID, &assigneeName, &assignedByName, &note, &dueDate, &assignedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Row{}, ErrNotFound
		}
		return Row{}, fmt.Errorf("读取任务失败: %w", err)
	}
	item.Drawing.CreatedAt = parseTime(createdAt)
	item.Drawing.UpdatedAt = parseTime(updatedAt)
	if taskID != "" {
		assignment := &Assignment{
			TaskID:     taskID,
			AssigneeID: assigneeID,
			Assignee:   assigneeName,
			AssignedBy: assignedByName,
			Note:       note,
			DueDate:    dueDate,
		}
		if assignedAt != nil {
			assignment.AssignedAt = parseTime(*assignedAt)
		}
		item.Assignment = assignment
	}
	item.Progress = Derive(item.Drawing.Status, item.Drawing.FileCount, item.Drawing.ReviewNode, item.Drawing.ReviewDone, item.Drawing.ReviewTotal)
	return item, nil
}

// Assign 新建指派。图纸已有有效负责人时返回 ErrConflict，
// 由调用方引导到改派，而不是静默覆盖（覆盖会让上一任负责人不知道活已经不在自己手上）。
func (service *Service) Assign(ctx context.Context, actor auth.AuthUser, input AssignInput) (Row, error) {
	if !actor.CanManageTasks() {
		return Row{}, ErrForbidden
	}
	drawingID := strings.TrimSpace(input.DrawingID)
	assigneeID := strings.TrimSpace(input.AssigneeID)
	if drawingID == "" || assigneeID == "" {
		return Row{}, ErrInvalidInput
	}
	dueDate, err := normalizeDueDate(input.DueDate)
	if err != nil {
		return Row{}, err
	}

	tx, err := service.pool.Begin(ctx)
	if err != nil {
		return Row{}, err
	}
	defer tx.Rollback(ctx)

	// 锁定图纸行：并发指派必须串行，否则两条 active 任务会撞上部分唯一索引，
	// 用户看到的将是数据库约束错误而不是「已有负责人」的业务提示。
	status, drawingNo, drawingName, err := lockDrawing(ctx, tx, drawingID)
	if err != nil {
		return Row{}, err
	}
	if drawing.Status(status) == drawing.StatusArchived {
		return Row{}, ErrArchivedDrawing
	}
	assigneeName, err := loadAssignee(ctx, tx, assigneeID)
	if err != nil {
		return Row{}, err
	}
	var existing string
	err = tx.QueryRow(ctx, `SELECT id::text FROM drawing_tasks WHERE drawing_id = $1::uuid AND status = 'active'`, drawingID).Scan(&existing)
	if err == nil {
		return Row{}, ErrConflict
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Row{}, fmt.Errorf("检查现有任务失败: %w", err)
	}

	taskID, err := insertTask(ctx, tx, drawingID, assigneeID, actor.ID, input.Note, dueDate)
	if err != nil {
		return Row{}, err
	}
	if err := notifyTask(ctx, tx, taskID, assigneeID, actor.ID, drawingID, drawingNo, drawingName, "你被指派为图纸负责人", assignmentContent(drawingNo, drawingName, input.Note, dueDate)); err != nil {
		return Row{}, err
	}
	if err := writeAudit(ctx, tx, actor.ID, drawingID, drawingNo, "assign", fmt.Sprintf("指派图纸负责人：%s", assigneeName)); err != nil {
		return Row{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Row{}, err
	}
	return service.loadRow(ctx, drawingID)
}

// Update 处理改派与说明维护：换了人就结束旧任务并新建任务（保留历史），
// 没换人就只更新说明与截止日期（不制造无意义的历史行）。
func (service *Service) Update(ctx context.Context, actor auth.AuthUser, taskID string, input UpdateInput) (Row, error) {
	if !actor.CanManageTasks() {
		return Row{}, ErrForbidden
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return Row{}, ErrNotFound
	}
	dueDate, err := normalizeDueDate(input.DueDate)
	if err != nil {
		return Row{}, err
	}

	tx, err := service.pool.Begin(ctx)
	if err != nil {
		return Row{}, err
	}
	defer tx.Rollback(ctx)

	var drawingID, currentAssignee, drawingNo, drawingName string
	err = tx.QueryRow(ctx, `
		SELECT t.drawing_id::text, t.assignee_id::text, d.drawing_no, d.name
		FROM drawing_tasks t JOIN drawings d ON d.id = t.drawing_id
		WHERE t.id = $1::uuid AND t.status = 'active' FOR UPDATE OF t`, taskID).
		Scan(&drawingID, &currentAssignee, &drawingNo, &drawingName)
	if errors.Is(err, pgx.ErrNoRows) {
		return Row{}, ErrNotFound
	}
	if err != nil {
		return Row{}, fmt.Errorf("读取任务失败: %w", err)
	}

	nextAssigneeID := strings.TrimSpace(input.AssigneeID)
	if nextAssigneeID == "" {
		nextAssigneeID = currentAssignee
	}

	if nextAssigneeID == currentAssignee {
		_, err = tx.Exec(ctx, `UPDATE drawing_tasks SET note = $2, due_date = NULLIF($3, '')::date WHERE id = $1::uuid`,
			taskID, strings.TrimSpace(input.Note), dueDate)
		if err != nil {
			return Row{}, fmt.Errorf("更新任务说明失败: %w", err)
		}
		if err := writeAudit(ctx, tx, actor.ID, drawingID, drawingNo, "assign", "更新图纸任务说明"); err != nil {
			return Row{}, err
		}
	} else {
		nextAssigneeName, err := loadAssignee(ctx, tx, nextAssigneeID)
		if err != nil {
			return Row{}, err
		}
		var previousName string
		if err := tx.QueryRow(ctx, `SELECT COALESCE(display_name, account, '') FROM users WHERE id = $1::uuid`, currentAssignee).Scan(&previousName); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return Row{}, fmt.Errorf("读取原负责人失败: %w", err)
		}
		reason := strings.TrimSpace(input.Reason)
		if reason == "" {
			reason = fmt.Sprintf("改派给 %s", nextAssigneeName)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE drawing_tasks SET status = 'replaced', ended_at = now(), ended_by = $2::uuid, end_reason = $3
			WHERE id = $1::uuid`, taskID, actor.ID, reason); err != nil {
			return Row{}, fmt.Errorf("结束原任务失败: %w", err)
		}
		newTaskID, err := insertTask(ctx, tx, drawingID, nextAssigneeID, actor.ID, input.Note, dueDate)
		if err != nil {
			return Row{}, err
		}
		content := assignmentContent(drawingNo, drawingName, input.Note, dueDate) + "\n改派说明：" + reason
		if err := notifyTask(ctx, tx, newTaskID, nextAssigneeID, actor.ID, drawingID, drawingNo, drawingName, "你被指派为图纸负责人", content); err != nil {
			return Row{}, err
		}
		if err := notifyTask(ctx, tx, "reassign:"+taskID, currentAssignee, actor.ID, drawingID, drawingNo, drawingName, "图纸任务已改派", fmt.Sprintf("%s · %s\n该图纸已改派给 %s，你不再负责这张图纸。\n改派说明：%s", drawingNo, drawingName, nextAssigneeName, reason)); err != nil {
			return Row{}, err
		}
		if err := writeAudit(ctx, tx, actor.ID, drawingID, drawingNo, "assign", fmt.Sprintf("改派图纸负责人：%s → %s（%s）", previousName, nextAssigneeName, reason)); err != nil {
			return Row{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Row{}, err
	}
	return service.loadRow(ctx, drawingID)
}

// Cancel 取消指派：图纸回到「未指派」状态，控制权回落给创建人。
// 已存档图纸允许取消，因为存档后负责人本来就不再生效，取消只是把管理台清理干净。
func (service *Service) Cancel(ctx context.Context, actor auth.AuthUser, taskID, reason string) error {
	if !actor.CanManageTasks() {
		return ErrForbidden
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return ErrNotFound
	}
	tx, err := service.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var drawingID, assigneeID, drawingNo, drawingName string
	err = tx.QueryRow(ctx, `
		SELECT t.drawing_id::text, t.assignee_id::text, d.drawing_no, d.name
		FROM drawing_tasks t JOIN drawings d ON d.id = t.drawing_id
		WHERE t.id = $1::uuid AND t.status = 'active' FOR UPDATE OF t`, taskID).
		Scan(&drawingID, &assigneeID, &drawingNo, &drawingName)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("读取任务失败: %w", err)
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "计划员取消指派"
	}
	if _, err := tx.Exec(ctx, `
		UPDATE drawing_tasks SET status = 'cancelled', ended_at = now(), ended_by = $2::uuid, end_reason = $3
		WHERE id = $1::uuid`, taskID, actor.ID, reason); err != nil {
		return fmt.Errorf("取消指派失败: %w", err)
	}
	if err := notifyTask(ctx, tx, "cancel:"+taskID, assigneeID, actor.ID, drawingID, drawingNo, drawingName, "图纸任务已取消", fmt.Sprintf("%s · %s\n计划员取消了你在这张图纸上的任务。\n原因：%s", drawingNo, drawingName, reason)); err != nil {
		return err
	}
	if err := writeAudit(ctx, tx, actor.ID, drawingID, drawingNo, "assign", "取消图纸负责人指派："+reason); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// History 返回一张图纸的指派历史（含改派与取消），供管理台追溯「为什么换人」。
func (service *Service) History(ctx context.Context, drawingID string) ([]HistoryEntry, error) {
	drawingID = strings.TrimSpace(drawingID)
	if drawingID == "" {
		return nil, ErrNotFound
	}
	rows, err := service.pool.Query(ctx, `
		SELECT t.id::text, t.assignee_id::text, COALESCE(assignee.display_name, assignee.account, ''),
		       t.status, t.note, COALESCE(to_char(t.due_date, 'YYYY-MM-DD'), ''),
		       COALESCE(assigner.display_name, assigner.account, ''), t.created_at,
		       COALESCE(ender.display_name, ender.account, ''), t.ended_at, t.end_reason
		FROM drawing_tasks t
		LEFT JOIN users assignee ON assignee.id = t.assignee_id
		LEFT JOIN users assigner ON assigner.id = t.assigned_by
		LEFT JOIN users ender ON ender.id = t.ended_by
		WHERE t.drawing_id = $1::uuid
		ORDER BY t.created_at DESC`, drawingID)
	if err != nil {
		return nil, fmt.Errorf("读取指派历史失败: %w", err)
	}
	defer rows.Close()
	items := make([]HistoryEntry, 0)
	for rows.Next() {
		var item HistoryEntry
		var createdAt time.Time
		var endedAt *time.Time
		if err := rows.Scan(&item.TaskID, &item.AssigneeID, &item.Assignee, &item.Status, &item.Note, &item.DueDate,
			&item.AssignedBy, &createdAt, &item.EndedBy, &endedAt, &item.EndReason); err != nil {
			return nil, fmt.Errorf("解析指派历史失败: %w", err)
		}
		item.AssignedAt = parseTime(createdAt)
		if endedAt != nil {
			item.EndedAt = parseTime(*endedAt)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// Candidates 返回可被指派的人员。
// 名单按「能否真的编制图纸」收敛，并带上在办数量，让计划员能把活分匀。
func (service *Service) Candidates(ctx context.Context) ([]Candidate, error) {
	rows, err := service.pool.Query(ctx, `
		SELECT u.id::text, u.account, u.display_name,
		       COALESCE(array_agg(ur.role ORDER BY ur.role) FILTER (WHERE ur.role IS NOT NULL), '{}'),
		       COALESCE((SELECT count(*) FROM drawing_tasks t WHERE t.assignee_id = u.id AND t.status = 'active'), 0)
		FROM users u
		JOIN user_roles candidate ON candidate.user_id = u.id AND candidate.role IN ('admin', 'planner', 'designer')
		LEFT JOIN user_roles ur ON ur.user_id = u.id
		WHERE u.status = 'active'
		GROUP BY u.id
		ORDER BY u.display_name, u.account`)
	if err != nil {
		return nil, fmt.Errorf("读取候选人失败: %w", err)
	}
	defer rows.Close()
	items := make([]Candidate, 0)
	for rows.Next() {
		var item Candidate
		if err := rows.Scan(&item.UserID, &item.Account, &item.Name, &item.Roles, &item.ActiveTasks); err != nil {
			return nil, fmt.Errorf("解析候选人失败: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func lockDrawing(ctx context.Context, tx pgx.Tx, drawingID string) (status, drawingNo, drawingName string, err error) {
	err = tx.QueryRow(ctx, `SELECT status, drawing_no, name FROM drawings WHERE id = $1::uuid FOR UPDATE`, drawingID).
		Scan(&status, &drawingNo, &drawingName)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", "", ErrNotFound
	}
	if err != nil {
		return "", "", "", fmt.Errorf("读取图纸失败: %w", err)
	}
	return status, drawingNo, drawingName, nil
}

// loadAssignee 校验被指派人确实可以负责图纸：账号在职，且具备设计、计划或管理员身份。
func loadAssignee(ctx context.Context, tx pgx.Tx, userID string) (string, error) {
	var name string
	var canAssign bool
	err := tx.QueryRow(ctx, `
		SELECT COALESCE(u.display_name, u.account, ''), u.status = 'active' AND EXISTS (
			SELECT 1 FROM user_roles r WHERE r.user_id = u.id AND r.role IN ('admin', 'planner', 'designer'))
		FROM users u WHERE u.id = $1::uuid`, userID).Scan(&name, &canAssign)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrInvalidAssignee
	}
	if err != nil {
		return "", fmt.Errorf("读取被指派人失败: %w", err)
	}
	if !canAssign {
		return "", ErrInvalidAssignee
	}
	return name, nil
}

func insertTask(ctx context.Context, tx pgx.Tx, drawingID, assigneeID, assignedBy, note, dueDate string) (string, error) {
	var taskID string
	err := tx.QueryRow(ctx, `
		INSERT INTO drawing_tasks (drawing_id, assignee_id, assigned_by, note, due_date)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, NULLIF($5, '')::date)
		RETURNING id::text`, drawingID, assigneeID, assignedBy, strings.TrimSpace(note), dueDate).Scan(&taskID)
	if err != nil {
		return "", fmt.Errorf("创建图纸任务失败: %w", err)
	}
	return taskID, nil
}

// notifyTask 在业务事务内写入通知：回滚时不发送，event_key 保证同一次任务只提醒一次。
func notifyTask(ctx context.Context, tx pgx.Tx, eventKey, recipientID, senderID, drawingID, drawingNo, drawingName, title, content string) error {
	if strings.TrimSpace(recipientID) == "" {
		return nil
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO notifications (recipient_id, sender_id, kind, title, content, drawing_id, event_key)
		VALUES ($1::uuid, $2::uuid, 'task', $3, $4, $5::uuid, $6)
		ON CONFLICT (recipient_id, event_key) DO NOTHING`,
		recipientID, senderID, title, content, drawingID, "task:"+eventKey)
	if err != nil {
		return fmt.Errorf("发送任务通知失败: %w", err)
	}
	return nil
}

func assignmentContent(drawingNo, drawingName, note, dueDate string) string {
	lines := []string{fmt.Sprintf("%s · %s", drawingNo, drawingName)}
	if strings.TrimSpace(note) != "" {
		lines = append(lines, "任务说明："+strings.TrimSpace(note))
	}
	if dueDate != "" {
		lines = append(lines, "截止日期："+dueDate)
	}
	lines = append(lines, "你已获得这张图纸的负责人权限，未存档时可直接编制、上传文件并发起审核。")
	return strings.Join(lines, "\n")
}

// writeAudit 记录指派类操作。审计与任务写入同一事务，失败则整体回滚，
// 避免出现「有任务但没有操作记录」的不可解释状态。
func writeAudit(ctx context.Context, tx pgx.Tx, actorID, drawingID, drawingNo, action, summary string) error {
	metadata, err := json.Marshal(map[string]any{"drawingNo": drawingNo})
	if err != nil {
		return fmt.Errorf("生成审计元数据失败: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO audit_logs (actor_id, action, resource_type, resource_id, summary, metadata)
		VALUES ($1::uuid, $2, 'drawing', $3::uuid, $4, $5::jsonb)`, actorID, action, drawingID, summary, metadata)
	if err != nil {
		return fmt.Errorf("记录任务操作失败: %w", err)
	}
	return nil
}

// normalizeDueDate 只接受 YYYY-MM-DD，空值表示不设截止日期。
// 拒绝其他格式是为了避免「2026/3/1」被数据库按不同区域设置解释成不同日期。
func normalizeDueDate(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil
	}
	if _, err := time.Parse("2006-01-02", trimmed); err != nil {
		return "", errors.New("截止日期格式无效，请使用 YYYY-MM-DD")
	}
	return trimmed, nil
}
