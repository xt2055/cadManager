package change

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"cadguanliq/internal/auth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound     = errors.New("变更工单不存在")
	ErrForbidden    = errors.New("无权执行该变更工单操作")
	ErrState        = errors.New("工单当前状态不允许该操作")
	ErrOpenExists   = errors.New("该图纸已存在未结束的变更工单")
	ErrNoArchive    = errors.New("仅已存档的图纸可以发起变更工单")
	ErrNotExecuting = errors.New("变更工单已不在执行中，无法登记本次成果")
	ErrStaleSubmit  = errors.New("工单成果已更新，请刷新后重新验收")
)

// Service 变更工单业务接口，同时作为编辑门禁的工单查询依赖。
type Service interface {
	Create(ctx context.Context, user auth.AuthUser, drawingID string, input CreateInput) (Request, error)
	Get(ctx context.Context, id string) (Request, error)
	List(ctx context.Context, filter ListFilter) ([]Request, error)
	ListByDrawing(ctx context.Context, drawingID string) ([]Request, error)
	Approve(ctx context.Context, user auth.AuthUser, id string, input ApproveInput) (Request, error)
	Reject(ctx context.Context, user auth.AuthUser, id string, input DecisionInput) (Request, error)
	Submit(ctx context.Context, user auth.AuthUser, id string, input SubmitInput) (Request, error)
	Verify(ctx context.Context, user auth.AuthUser, id string, input DecisionInput) (Request, error)
	ReturnForEdit(ctx context.Context, user auth.AuthUser, id string, input DecisionInput) (Request, error)
	Cancel(ctx context.Context, user auth.AuthUser, id string, input DecisionInput) (Request, error)
	// CanEditArchived 报告指定用户是否因持有执行中的工单而可编辑该存档图纸。
	CanEditArchived(ctx context.Context, drawingID, userID string) (bool, string, error)
	// WorkVersion 返回工单当前工作成果版本（附件 ID / 存储键 / 版本 ID）；无工作版本时 found=false。
	WorkVersion(ctx context.Context, requestID string) (attachmentID, storageKey, versionID string, found bool, err error)
	// RecordWorkVersion 把工单最新工作版本登记到工单（仅在工单仍执行中时生效）。
	RecordWorkVersion(ctx context.Context, requestID, versionID string) error
	CompareAndRecordWorkVersion(ctx context.Context, requestID, versionID, expectedVersionID, userID string) (bool, error)
	// EditBaseline 返回工单当前工作版本（无工作版本时回退到工单文件基线版本）的内容哈希。
	EditBaseline(ctx context.Context, requestID string) (sha string, found bool, err error)
	// StillExecuting 报告工单是否仍处于可编辑（executing）状态。
	StillExecuting(ctx context.Context, requestID string) (bool, error)
	// HasActiveEditSession 报告工单是否仍有未关闭的编辑会话（提交/终止前需清空）。
	HasActiveEditSession(ctx context.Context, requestID string) (bool, error)
}

type PGService struct{ pool *pgxpool.Pool }

func NewService(pool *pgxpool.Pool) *PGService { return &PGService{pool: pool} }

func isAdmin(user auth.AuthUser) bool {
	for _, role := range user.Roles {
		if role == "admin" {
			return true
		}
	}
	return false
}

func generateRequestNo() (string, error) {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return fmt.Sprintf("CR%s-%s", time.Now().Format("20060102"), hex.EncodeToString(buf)), nil
}

func (s *PGService) Create(ctx context.Context, user auth.AuthUser, drawingID string, input CreateInput) (Request, error) {
	reason := strings.TrimSpace(input.Reason)
	scope := strings.TrimSpace(input.Scope)
	if reason == "" || scope == "" {
		return Request{}, errors.New("变更原因和修改范围不能为空")
	}
	executorID := strings.TrimSpace(input.ExecutorID)
	if executorID == "" {
		executorID = user.ID
	}
	requireVerify := true
	if input.RequireVerify != nil {
		requireVerify = *input.RequireVerify
	}
	if !isAdmin(user) && !requireVerify {
		return Request{}, errors.New("仅管理员可以设置免验收")
	}
	autoApprove := input.AutoApprove && isAdmin(user)
	if autoApprove {
		if strings.TrimSpace(input.ApproveOpinion) == "" {
			return Request{}, errors.New("管理员直接批准必须填写审批意见")
		}
		if !requireVerify && strings.TrimSpace(input.WaiveReason) == "" {
			return Request{}, errors.New("免验收必须单独填写原因，不得复用审批意见")
		}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Request{}, err
	}
	defer tx.Rollback(ctx)

	var drawingNo, drawingName, status string
	var baseRevision int64
	err = tx.QueryRow(ctx, `SELECT drawing_no, name, status, revision FROM drawings WHERE id = $1::uuid FOR UPDATE`, drawingID).Scan(&drawingNo, &drawingName, &status, &baseRevision)
	if errors.Is(err, pgx.ErrNoRows) {
		return Request{}, ErrNotFound
	} else if err != nil {
		return Request{}, err
	}
	if status != "archived" {
		return Request{}, ErrNoArchive
	}

	var baseVersionID *string
	err = tx.QueryRow(ctx, `
		SELECT av.id::text FROM attachments a
		JOIN attachment_versions av ON av.id = a.current_version_id
		WHERE a.drawing_id = $1::uuid AND a.deleted_at IS NULL
		ORDER BY (a.file_role = 'assembly') DESC, av.created_at DESC LIMIT 1`, drawingID).Scan(&baseVersionID)
	if errors.Is(err, pgx.ErrNoRows) {
		baseVersionID = nil
	} else if err != nil {
		return Request{}, fmt.Errorf("查询图纸文件基线失败: %w", err)
	}

	requestNo, err := generateRequestNo()
	if err != nil {
		return Request{}, err
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = drawingName
	}
	initialStatus := StatusPendingApproval
	var approverID any
	direct := autoApprove
	if autoApprove {
		initialStatus = StatusExecuting
		approverID = user.ID
	}

	var id string
	err = tx.QueryRow(ctx, `
		INSERT INTO change_requests (request_no, drawing_id, drawing_no, title, reason, scope,
			base_drawing_revision, base_attachment_version_id, status, require_verify,
			applicant_id, executor_id, approver_id, direct_admin_approval, approved_at, verify_waived_reason)
		VALUES ($1,$2::uuid,$3,$4,$5,$6,$7,$8::uuid,$9,$10,$11::uuid,$12::uuid,$13::uuid,$14,
			CASE WHEN $13::uuid IS NULL THEN NULL ELSE now() END, $15)
		RETURNING id::text`,
		requestNo, drawingID, drawingNo, title, reason, scope, baseRevision, baseVersionID, string(initialStatus), requireVerify,
		user.ID, executorID, approverID, direct, waivedReason(autoApprove, requireVerify, input.WaiveReason)).Scan(&id)
	if err != nil {
		if isOpenRequestConflict(err) {
			return Request{}, ErrOpenExists
		}
		return Request{}, fmt.Errorf("创建变更工单失败: %w", err)
	}

	if err := s.logAction(ctx, tx, id, user.ID, ActionCreate, "发起变更申请"); err != nil {
		return Request{}, err
	}
	if autoApprove {
		opinion := strings.TrimSpace(input.ApproveOpinion)
		if _, err := tx.Exec(ctx, `UPDATE change_requests SET approver_opinion = $2 WHERE id = $1::uuid`, id, opinion); err != nil {
			return Request{}, err
		}
		if err := s.logAction(ctx, tx, id, user.ID, ActionApprove, "管理员直接批准："+opinion); err != nil {
			return Request{}, err
		}
		if !requireVerify {
			waive := strings.TrimSpace(input.WaiveReason)
			if err := s.logAction(ctx, tx, id, user.ID, ActionWaiveVerify, "免验收："+waive); err != nil {
				return Request{}, err
			}
		}
	}
	if err := s.audit(ctx, tx, id, drawingNo, user.ID, "change_request_create", "发起图纸变更申请 "+requestNo); err != nil {
		return Request{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Request{}, err
	}
	return s.Get(ctx, id)
}

func waivedReason(autoApprove, requireVerify bool, opinion string) string {
	if autoApprove && !requireVerify {
		return strings.TrimSpace(opinion)
	}
	return ""
}

func (s *PGService) Approve(ctx context.Context, user auth.AuthUser, id string, input ApproveInput) (Request, error) {
	if !isAdmin(user) {
		return Request{}, ErrForbidden
	}
	opinion := strings.TrimSpace(input.Opinion)
	if opinion == "" {
		return Request{}, errors.New("审批意见不能为空")
	}
	if !input.RequireVerify && strings.TrimSpace(input.WaiveReason) == "" {
		return Request{}, errors.New("关闭验收必须填写免验收原因")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Request{}, err
	}
	defer tx.Rollback(ctx)
	var applicant, status, drawingNo string
	err = tx.QueryRow(ctx, `SELECT applicant_id::text, status, drawing_no FROM change_requests WHERE id = $1::uuid FOR UPDATE`, id).Scan(&applicant, &status, &drawingNo)
	if errors.Is(err, pgx.ErrNoRows) {
		return Request{}, ErrNotFound
	} else if err != nil {
		return Request{}, err
	}
	if status != string(StatusPendingApproval) {
		return Request{}, ErrState
	}
	direct := applicant == user.ID
	tag, err := tx.Exec(ctx, `
		UPDATE change_requests
		SET status = 'executing', approver_id = $2::uuid, approved_at = now(), require_verify = $3,
		    approver_opinion = $4, verify_waived_reason = CASE WHEN $3 THEN '' ELSE $5 END,
		    direct_admin_approval = direct_admin_approval OR $6
		WHERE id = $1::uuid AND status = 'pending_approval'`, id, user.ID, input.RequireVerify, opinion, strings.TrimSpace(input.WaiveReason), direct)
	if err != nil {
		return Request{}, err
	}
	if tag.RowsAffected() == 0 {
		return Request{}, ErrState
	}
	if err := s.logAction(ctx, tx, id, user.ID, ActionApprove, "审批通过："+opinion); err != nil {
		return Request{}, err
	}
	if !input.RequireVerify {
		if err := s.logAction(ctx, tx, id, user.ID, ActionWaiveVerify, "免验收："+strings.TrimSpace(input.WaiveReason)); err != nil {
			return Request{}, err
		}
	}
	if err := s.audit(ctx, tx, id, drawingNo, user.ID, "change_request_approve", "审批通过变更工单"); err != nil {
		return Request{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Request{}, err
	}
	return s.Get(ctx, id)
}

func (s *PGService) Reject(ctx context.Context, user auth.AuthUser, id string, input DecisionInput) (Request, error) {
	if !isAdmin(user) {
		return Request{}, ErrForbidden
	}
	opinion := strings.TrimSpace(input.Opinion)
	if opinion == "" {
		return Request{}, errors.New("驳回必须填写意见")
	}
	return s.transition(ctx, id, user, StatusPendingApproval, StatusRejected, ActionReject, "驳回："+opinion)
}

func (s *PGService) ReturnForEdit(ctx context.Context, user auth.AuthUser, id string, input DecisionInput) (Request, error) {
	if !isAdmin(user) {
		return Request{}, ErrForbidden
	}
	opinion := strings.TrimSpace(input.Opinion)
	if opinion == "" {
		return Request{}, errors.New("退回修改必须填写意见")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Request{}, err
	}
	defer tx.Rollback(ctx)
	var drawingNo, status, currentSubmission string
	err = tx.QueryRow(ctx, `
		SELECT drawing_no, status, COALESCE(current_submission_id::text, '')
		FROM change_requests WHERE id = $1::uuid FOR UPDATE`, id).Scan(&drawingNo, &status, &currentSubmission)
	if errors.Is(err, pgx.ErrNoRows) {
		return Request{}, ErrNotFound
	} else if err != nil {
		return Request{}, err
	}
	if status != string(StatusPendingVerify) {
		return Request{}, ErrState
	}
	tag, err := tx.Exec(ctx, `UPDATE change_requests SET status = 'executing' WHERE id = $1::uuid AND status = 'pending_verify'`, id)
	if err != nil {
		return Request{}, err
	}
	if tag.RowsAffected() == 0 {
		return Request{}, ErrState
	}
	// 保留被退回轮次为历史，不删除其差异；下一轮提交将新建 current_submission。
	if err := s.markSubmission(ctx, tx, currentSubmission, "returned"); err != nil {
		return Request{}, err
	}
	if err := s.logAction(ctx, tx, id, user.ID, ActionReturn, "退回修改："+opinion); err != nil {
		return Request{}, err
	}
	if err := s.audit(ctx, tx, id, drawingNo, user.ID, "change_request_return", "退回修改"); err != nil {
		return Request{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Request{}, err
	}
	return s.Get(ctx, id)
}

func (s *PGService) Cancel(ctx context.Context, user auth.AuthUser, id string, input DecisionInput) (Request, error) {
	opinion := strings.TrimSpace(input.Opinion)
	if opinion == "" {
		return Request{}, errors.New("终止必须填写原因")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Request{}, err
	}
	defer tx.Rollback(ctx)
	var drawingNo, status, applicant string
	err = tx.QueryRow(ctx, `SELECT drawing_no, status, applicant_id::text FROM change_requests WHERE id = $1::uuid FOR UPDATE`, id).Scan(&drawingNo, &status, &applicant)
	if errors.Is(err, pgx.ErrNoRows) {
		return Request{}, ErrNotFound
	} else if err != nil {
		return Request{}, err
	}
	openStatus := Status(status).Open()
	if !openStatus {
		return Request{}, ErrState
	}
	if !isAdmin(user) && applicant != user.ID {
		return Request{}, ErrForbidden
	}
	tag, err := tx.Exec(ctx, `UPDATE change_requests SET status = 'cancelled' WHERE id = $1::uuid AND status = $2`, id, status)
	if err != nil {
		return Request{}, err
	}
	if tag.RowsAffected() == 0 {
		return Request{}, ErrState
	}
	// 冻结：终止在同一事务内撤销该工单尚未使用的打开票据并关闭其活动编辑会话，
	// 终止后旧会话/旧票据不得再登记成果或重新打开。
	if _, err := tx.Exec(ctx, `
		DELETE FROM edit_session_tickets t
		USING edit_sessions s
		WHERE t.session_id = s.id AND s.change_request_id = $1::uuid AND s.status = 'active'`, id); err != nil {
		return Request{}, fmt.Errorf("撤销编辑票据失败: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE edit_sessions SET status = 'closed', closed_at = now(), last_seen_at = now()
		WHERE change_request_id = $1::uuid AND status = 'active'`, id); err != nil {
		return Request{}, fmt.Errorf("关闭编辑会话失败: %w", err)
	}
	// 已提交但被终止的轮次标记为历史，不再作为待验收快照发布。
	var currentSubmission string
	_ = tx.QueryRow(ctx, `SELECT COALESCE(current_submission_id::text, '') FROM change_requests WHERE id = $1::uuid`, id).Scan(&currentSubmission)
	if err := s.markSubmission(ctx, tx, currentSubmission, "returned"); err != nil {
		return Request{}, err
	}
	if err := s.logAction(ctx, tx, id, user.ID, ActionCancel, "终止工单："+opinion); err != nil {
		return Request{}, err
	}
	if err := s.audit(ctx, tx, id, drawingNo, user.ID, "change_request_cancel", "终止变更工单"); err != nil {
		return Request{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Request{}, err
	}
	return s.Get(ctx, id)
}

func (s *PGService) transition(ctx context.Context, id string, user auth.AuthUser, from, to Status, action Action, note string) (Request, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Request{}, err
	}
	defer tx.Rollback(ctx)
	var drawingNo string
	err = tx.QueryRow(ctx, `SELECT drawing_no FROM change_requests WHERE id = $1::uuid FOR UPDATE`, id).Scan(&drawingNo)
	if errors.Is(err, pgx.ErrNoRows) {
		return Request{}, ErrNotFound
	} else if err != nil {
		return Request{}, err
	}
	tag, err := tx.Exec(ctx, `UPDATE change_requests SET status = $2 WHERE id = $1::uuid AND status = $3`, id, string(to), string(from))
	if err != nil {
		return Request{}, err
	}
	if tag.RowsAffected() == 0 {
		return Request{}, ErrState
	}
	if err := s.logAction(ctx, tx, id, user.ID, action, note); err != nil {
		return Request{}, err
	}
	if err := s.audit(ctx, tx, id, drawingNo, user.ID, "change_request_"+string(action), note); err != nil {
		return Request{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Request{}, err
	}
	return s.Get(ctx, id)
}

func (s *PGService) Submit(ctx context.Context, user auth.AuthUser, id string, input SubmitInput) (Request, error) {
	actualChanges := strings.TrimSpace(input.ActualChanges)
	if actualChanges == "" {
		return Request{}, errors.New("提交完成必须填写实际修改说明")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Request{}, err
	}
	defer tx.Rollback(ctx)

	var drawingID, drawingNo, executor, status string
	var requireVerify bool
	var baseVersionID *string
	err = tx.QueryRow(ctx, `
		SELECT drawing_id::text, drawing_no, executor_id::text, status, require_verify, base_attachment_version_id::text
		FROM change_requests WHERE id = $1::uuid FOR UPDATE`, id).
		Scan(&drawingID, &drawingNo, &executor, &status, &requireVerify, &baseVersionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Request{}, ErrNotFound
	} else if err != nil {
		return Request{}, err
	}
	if status != string(StatusExecuting) {
		return Request{}, ErrState
	}
	if user.ID != executor && !isAdmin(user) {
		return Request{}, ErrForbidden
	}
	// 冻结：仍有未关闭的编辑会话时不允许提交，避免提交/验收后继续写入。
	var active bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM edit_sessions WHERE change_request_id = $1::uuid AND status = 'active')`, id).Scan(&active); err != nil {
		return Request{}, err
	}
	if active {
		return Request{}, errors.New("请先结束编辑会话后再提交变更")
	}

	// 存档期正式列未被普通编辑改动，当前列即基线；据此生成本轮属性与文件差异快照。
	var curName, curMaterial, curVendor, curVersion string
	if err := tx.QueryRow(ctx, `SELECT name, material, vendor, version FROM drawings WHERE id = $1::uuid`, drawingID).Scan(&curName, &curMaterial, &curVendor, &curVersion); err != nil {
		return Request{}, err
	}
	// 本轮成果文件版本：工单当前工作版本（无则为空，表示文件未改动）。
	var submittedVersionID *string
	if err := tx.QueryRow(ctx, `SELECT submitted_attachment_version_id::text FROM change_requests WHERE id = $1::uuid`, id).Scan(&submittedVersionID); err != nil {
		return Request{}, err
	}

	proposedJSON, err := json.Marshal(input.Proposed)
	if err != nil {
		return Request{}, err
	}

	// 创建本轮不可变提交快照。
	var round int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(round), 0) + 1 FROM change_request_submissions WHERE request_id = $1::uuid`, id).Scan(&round); err != nil {
		return Request{}, err
	}
	var submissionID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO change_request_submissions (request_id, round, submitted_by, actual_changes, proposed_attributes,
		    base_attachment_version_id, submitted_attachment_version_id, status)
		VALUES ($1::uuid, $2, $3::uuid, $4, $5::jsonb, $6::uuid, $7::uuid, 'pending')
		RETURNING id::text`,
		id, round, user.ID, actualChanges, string(proposedJSON), baseVersionID, submittedVersionID).Scan(&submissionID); err != nil {
		return Request{}, fmt.Errorf("创建提交快照失败: %w", err)
	}

	if err := s.snapshotAttributeDiffs(ctx, tx, id, submissionID, input.Proposed, curName, curMaterial, curVendor, curVersion); err != nil {
		return Request{}, err
	}
	if err := s.snapshotFileDiff(ctx, tx, id, submissionID, baseVersionID, submittedVersionID); err != nil {
		return Request{}, err
	}

	nextStatus := StatusPendingVerify
	if !requireVerify {
		nextStatus = StatusCompleted
	}
	tag, err := tx.Exec(ctx, `
		UPDATE change_requests
		SET status = $2, proposed_attributes = $3::jsonb, actual_changes = $4, submitted_at = now(),
		    current_submission_id = $5::uuid,
		    completed_at = CASE WHEN $2 = 'completed' THEN now() ELSE completed_at END
		WHERE id = $1::uuid AND status = 'executing'`,
		id, string(nextStatus), string(proposedJSON), actualChanges, submissionID)
	if err != nil {
		return Request{}, err
	}
	if tag.RowsAffected() == 0 {
		return Request{}, ErrState
	}
	if err := s.logAction(ctx, tx, id, user.ID, ActionSubmit, fmt.Sprintf("第 %d 轮提交完成：%s", round, actualChanges)); err != nil {
		return Request{}, err
	}
	if !requireVerify {
		if err := s.applyCompletion(ctx, tx, id, drawingID, user.ID); err != nil {
			return Request{}, err
		}
		if err := s.markSubmission(ctx, tx, submissionID, "accepted"); err != nil {
			return Request{}, err
		}
	}
	if err := s.audit(ctx, tx, id, drawingNo, user.ID, "change_request_submit", "提交变更完成"); err != nil {
		return Request{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Request{}, err
	}
	return s.Get(ctx, id)
}

func (s *PGService) snapshotAttributeDiffs(ctx context.Context, tx pgx.Tx, id, submissionID string, proposed ProposedAttributes, curName, curMaterial, curVendor, curVersion string) error {
	if proposed.Name != nil && strings.TrimSpace(*proposed.Name) != "" && strings.TrimSpace(*proposed.Name) != curName {
		if err := s.addDiff(ctx, tx, id, submissionID, "attribute", "名称", curName, strings.TrimSpace(*proposed.Name)); err != nil {
			return err
		}
	}
	if proposed.Material != nil && strings.TrimSpace(*proposed.Material) != curMaterial {
		if err := s.addDiff(ctx, tx, id, submissionID, "material", "材料", curMaterial, strings.TrimSpace(*proposed.Material)); err != nil {
			return err
		}
	}
	if proposed.Vendor != nil && strings.TrimSpace(*proposed.Vendor) != curVendor {
		if err := s.addDiff(ctx, tx, id, submissionID, "attribute", "供应商", curVendor, strings.TrimSpace(*proposed.Vendor)); err != nil {
			return err
		}
	}
	if proposed.Version != nil && strings.TrimSpace(*proposed.Version) != curVersion {
		if err := s.addDiff(ctx, tx, id, submissionID, "attribute", "版本", curVersion, strings.TrimSpace(*proposed.Version)); err != nil {
			return err
		}
	}
	return nil
}

// snapshotFileDiff 记录本轮文件成果相对基线的差异（含版本与哈希前缀）。
func (s *PGService) snapshotFileDiff(ctx context.Context, tx pgx.Tx, id, submissionID string, baseVersionID, submittedVersionID *string) error {
	if submittedVersionID == nil || *submittedVersionID == "" {
		return nil
	}
	if baseVersionID != nil && *baseVersionID == *submittedVersionID {
		return nil
	}
	var subVersion, baseVersion, subHash string
	if err := tx.QueryRow(ctx, `SELECT version, LEFT(COALESCE(b.sha256, ''), 12) FROM attachment_versions av LEFT JOIN file_blobs b ON b.id = av.blob_id WHERE av.id = $1::uuid`, *submittedVersionID).Scan(&subVersion, &subHash); err != nil {
		return fmt.Errorf("读取成果文件版本失败: %w", err)
	}
	if baseVersionID != nil && *baseVersionID != "" {
		if err := tx.QueryRow(ctx, `SELECT version FROM attachment_versions WHERE id = $1::uuid`, *baseVersionID).Scan(&baseVersion); err != nil {
			return fmt.Errorf("读取基线文件版本失败: %w", err)
		}
	}
	return s.addDiff(ctx, tx, id, submissionID, "file", "图纸文件", baseVersion, subVersion+" (sha="+subHash+")")
}

func (s *PGService) Verify(ctx context.Context, user auth.AuthUser, id string, input DecisionInput) (Request, error) {
	if !isAdmin(user) {
		return Request{}, ErrForbidden
	}
	if strings.TrimSpace(input.SubmissionID) == "" {
		return Request{}, ErrStaleSubmit
	}
	opinion := strings.TrimSpace(input.Opinion)
	if opinion == "" {
		return Request{}, errors.New("验收通过必须填写意见")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Request{}, err
	}
	defer tx.Rollback(ctx)
	var drawingID, drawingNo, status string
	var currentSubmission string
	err = tx.QueryRow(ctx, `
		SELECT drawing_id::text, drawing_no, status, COALESCE(current_submission_id::text, '')
		FROM change_requests WHERE id = $1::uuid FOR UPDATE`, id).Scan(&drawingID, &drawingNo, &status, &currentSubmission)
	if errors.Is(err, pgx.ErrNoRows) {
		return Request{}, ErrNotFound
	} else if err != nil {
		return Request{}, err
	}
	if status != string(StatusPendingVerify) {
		return Request{}, ErrState
	}
	// 旧页面防护：验收请求携带的提交轮次必须仍是当前待验收轮次，
	// 否则说明工单已被退回并重新提交，需刷新后重新核对再验收。
	if strings.TrimSpace(input.SubmissionID) == "" || input.SubmissionID != currentSubmission {
		return Request{}, ErrStaleSubmit
	}
	var validSnapshot bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM change_request_submissions WHERE id=$1::uuid AND request_id=$2::uuid AND status='pending' AND NOT legacy_history)`, currentSubmission, id).Scan(&validSnapshot); err != nil {
		return Request{}, err
	}
	if !validSnapshot {
		return Request{}, ErrStaleSubmit
	}
	tag, err := tx.Exec(ctx, `UPDATE change_requests SET status = 'completed', verifier_id = $2::uuid, verified_at = now(), completed_at = now() WHERE id = $1::uuid AND status = 'pending_verify'`, id, user.ID)
	if err != nil {
		return Request{}, err
	}
	if tag.RowsAffected() == 0 {
		return Request{}, ErrState
	}
	if err := s.applyCompletion(ctx, tx, id, drawingID, user.ID); err != nil {
		return Request{}, err
	}
	if err := s.markSubmission(ctx, tx, currentSubmission, "accepted"); err != nil {
		return Request{}, err
	}
	note := "验收通过：" + opinion
	if err := s.logAction(ctx, tx, id, user.ID, ActionVerify, note); err != nil {
		return Request{}, err
	}
	if err := s.audit(ctx, tx, id, drawingNo, user.ID, "change_request_verify", "验收通过并发布变更"); err != nil {
		return Request{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Request{}, err
	}
	return s.Get(ctx, id)
}

// applyCompletion 在工单完成时把"当前提交快照"的属性与文件正式发布生效。
// 发布内容以 current_submission 为准，与验收员所看到的本轮差异严格一致；
// 兼容历史数据：无提交快照时回退到工单顶层字段。
// 必须在事务内、状态已置 completed 之后调用。存档期正式列未被普通编辑改动，故当前列即基线。
func (s *PGService) applyCompletion(ctx context.Context, tx pgx.Tx, id, drawingID, actorID string) error {
	var proposedRaw []byte
	var submittedVersionID *string
	if err := tx.QueryRow(ctx, `
		SELECT sub.proposed_attributes, sub.submitted_attachment_version_id::text
		FROM change_requests cr
		JOIN change_request_submissions sub ON sub.id = cr.current_submission_id AND sub.request_id = cr.id
		WHERE cr.id = $1::uuid`, id).Scan(&proposedRaw, &submittedVersionID); err != nil {
		return err
	}
	var proposed ProposedAttributes
	if len(proposedRaw) > 0 {
		if err := json.Unmarshal(proposedRaw, &proposed); err != nil {
			return fmt.Errorf("解析变更属性草案失败: %w", err)
		}
	}
	var newName, newMaterial, newVendor, newVersion any
	if proposed.Name != nil && strings.TrimSpace(*proposed.Name) != "" {
		newName = strings.TrimSpace(*proposed.Name)
	}
	if proposed.Material != nil {
		newMaterial = strings.TrimSpace(*proposed.Material)
	}
	if proposed.Vendor != nil {
		newVendor = strings.TrimSpace(*proposed.Vendor)
	}
	if proposed.Version != nil {
		newVersion = strings.TrimSpace(*proposed.Version)
	}
	if newName != nil || newMaterial != nil || newVendor != nil || newVersion != nil {
		if _, err := tx.Exec(ctx, `
			UPDATE drawings
			SET name = COALESCE($2, name), material = COALESCE($3, material), vendor = COALESCE($4, vendor), version = COALESCE($5, version),
			    updated_by = $6::uuid, revision = revision + 1
			WHERE id = $1::uuid`, drawingID, newName, newMaterial, newVendor, newVersion, actorID); err != nil {
			return fmt.Errorf("应用变更属性失败: %w", err)
		}
	}
	if submittedVersionID != nil {
		var attachmentID string
		err := tx.QueryRow(ctx, `SELECT attachment_id::text FROM attachment_versions WHERE id = $1::uuid`, *submittedVersionID).Scan(&attachmentID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		} else if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE attachment_versions SET version_kind = 'release' WHERE id = $1::uuid`, *submittedVersionID); err != nil {
			return fmt.Errorf("发布文件版本失败: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE attachments SET current_version_id = $2::uuid, revision = revision + 1 WHERE id = $1::uuid`, attachmentID, *submittedVersionID); err != nil {
			return fmt.Errorf("切换正式版本指针失败: %w", err)
		}
	}
	return nil
}

func (s *PGService) CanEditArchived(ctx context.Context, drawingID, userID string) (bool, string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `
		SELECT id::text FROM change_requests
		WHERE drawing_id = $1::uuid AND status = 'executing' AND executor_id = $2::uuid
		ORDER BY created_at DESC LIMIT 1`, drawingID, userID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	return true, id, nil
}

func (s *PGService) WorkVersion(ctx context.Context, requestID string) (string, string, string, bool, error) {
	var attachmentID, storageKey, versionID string
	err := s.pool.QueryRow(ctx, `
		SELECT av.attachment_id::text, COALESCE(b.storage_key, ''), av.id::text
		FROM change_requests cr
		JOIN attachment_versions av ON av.id = cr.submitted_attachment_version_id
		LEFT JOIN file_blobs b ON b.id = av.blob_id
		WHERE cr.id = $1::uuid AND cr.submitted_attachment_version_id IS NOT NULL`, requestID).
		Scan(&attachmentID, &storageKey, &versionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", "", false, nil
	}
	if err != nil {
		return "", "", "", false, err
	}
	return attachmentID, storageKey, versionID, true, nil
}

// RecordWorkVersion 把工单最新工作版本登记到工单。
// 仅当工单仍处于 executing 且该版本确属本工单图纸的附件时更新成功；
// 更新 0 行时进一步区分"工单已不可写"（返回 ErrNotExecuting）与"版本不匹配/数据异常"（返回错误），
// 绝不把 0 行当作成功。
func (s *PGService) CompareAndRecordWorkVersion(ctx context.Context, requestID, versionID, expectedVersionID, userID string) (bool, error) {
	// 比较和更新必须是同一条 SQL，防止捕获文件期间其他编辑推进成果。
	tag, err := s.pool.Exec(ctx, `
		UPDATE change_requests cr SET submitted_attachment_version_id = $2::uuid
		WHERE cr.id = $1::uuid AND cr.status = 'executing' AND cr.executor_id = $4::uuid
		  AND cr.submitted_attachment_version_id IS NOT DISTINCT FROM NULLIF($3, '')::uuid
		  AND EXISTS (
			SELECT 1 FROM attachment_versions av JOIN attachments a ON a.id = av.attachment_id
			WHERE av.id = $2::uuid AND a.drawing_id = cr.drawing_id
		  )`, requestID, versionID, expectedVersionID, userID)
	if err != nil {
		return false, fmt.Errorf("登记工单工作版本失败: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func (s *PGService) RecordWorkVersion(ctx context.Context, requestID, versionID string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE change_requests cr
		SET submitted_attachment_version_id = $2::uuid
		WHERE cr.id = $1::uuid AND cr.status = 'executing'
		  AND EXISTS (
			SELECT 1 FROM attachment_versions av
			JOIN attachments a ON a.id = av.attachment_id
			WHERE av.id = $2::uuid AND a.drawing_id = cr.drawing_id
		  )`, requestID, versionID)
	if err != nil {
		return fmt.Errorf("登记工单工作版本失败: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return nil
	}
	var status string
	err = s.pool.QueryRow(ctx, `SELECT status FROM change_requests WHERE id = $1::uuid`, requestID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("核对工单状态失败: %w", err)
	}
	if status != string(StatusExecuting) {
		return ErrNotExecuting
	}
	return errors.New("工作版本与工单图纸不匹配，拒绝登记")
}

// EditBaseline 返回工单当前工作版本；无工作版本时回退到工单基线版本（创建工单时的正式当前版本）。
// 结束编辑时以其为比较基准，保证恢复到正式基线（A→B→A）也生成反映 A 的新成果版本。
func (s *PGService) EditBaseline(ctx context.Context, requestID string) (string, bool, error) {
	var sha string
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(b.sha256, '')
		FROM change_requests cr
		LEFT JOIN attachment_versions wv ON wv.id = cr.submitted_attachment_version_id
		LEFT JOIN attachment_versions bv ON bv.id = cr.base_attachment_version_id
		LEFT JOIN file_blobs b ON b.id = COALESCE(wv.blob_id, bv.blob_id)
		WHERE cr.id = $1::uuid`, requestID).Scan(&sha)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("读取工单编辑基线失败: %w", err)
	}
	return sha, sha != "", nil
}

func (s *PGService) StillExecuting(ctx context.Context, requestID string) (bool, error) {
	var status string
	err := s.pool.QueryRow(ctx, `SELECT status FROM change_requests WHERE id = $1::uuid`, requestID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return status == string(StatusExecuting), nil
}

func (s *PGService) HasActiveEditSession(ctx context.Context, requestID string) (bool, error) {
	var active bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM edit_sessions WHERE change_request_id = $1::uuid AND status = 'active')`, requestID).Scan(&active)
	return active, err
}

func (s *PGService) logAction(ctx context.Context, tx pgx.Tx, requestID, actorID string, action Action, opinion string) error {
	_, err := tx.Exec(ctx, `INSERT INTO change_request_actions (request_id, actor_id, action, opinion) VALUES ($1::uuid,$2::uuid,$3,$4)`, requestID, actorID, string(action), opinion)
	return err
}

func (s *PGService) addDiff(ctx context.Context, tx pgx.Tx, requestID, submissionID, kind, field, oldVal, newVal string) error {
	_, err := tx.Exec(ctx, `INSERT INTO change_request_diffs (request_id, submission_id, kind, field, old_value, new_value) VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6)`, requestID, submissionID, kind, field, oldVal, newVal)
	return err
}

// markSubmission 更新某一提交快照轮次的状态（pending / returned / accepted）。
func (s *PGService) markSubmission(ctx context.Context, tx pgx.Tx, submissionID, status string) error {
	if submissionID == "" {
		return nil
	}
	_, err := tx.Exec(ctx, `UPDATE change_request_submissions SET status = $2 WHERE id = $1::uuid`, submissionID, status)
	return err
}

func (s *PGService) audit(ctx context.Context, tx pgx.Tx, requestID, drawingNo, actorID, action, summary string) error {
	metadata, _ := json.Marshal(map[string]any{"drawingNo": drawingNo, "changeRequestId": requestID})
	_, err := tx.Exec(ctx, `INSERT INTO audit_logs (actor_id, action, resource_type, resource_id, summary, metadata) VALUES ($1::uuid,$2,'change_request',$3::uuid,$4,$5::jsonb)`, actorID, action, requestID, summary, metadata)
	return err
}

type scanner interface{ Scan(...any) error }

func (s *PGService) Get(ctx context.Context, id string) (Request, error) {
	item, err := scanRequest(s.pool.QueryRow(ctx, requestSelect+` WHERE cr.id = $1::uuid`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Request{}, ErrNotFound
	}
	if err != nil {
		return Request{}, err
	}
	if item.Diffs, err = s.loadDiffs(ctx, id); err != nil {
		return Request{}, err
	}
	if item.Submissions, err = s.loadSubmissions(ctx, id); err != nil {
		return Request{}, err
	}
	if item.Actions, err = s.loadActions(ctx, id); err != nil {
		return Request{}, err
	}
	return item, nil
}

const requestSelect = `
	SELECT cr.id::text, cr.request_no, cr.drawing_id::text, cr.drawing_no, cr.title, cr.reason, cr.scope, cr.status,
	       cr.require_verify, cr.applicant_id::text, COALESCE(ap.display_name, ''), cr.executor_id::text, COALESCE(ex.display_name, ''),
	       COALESCE(cr.approver_id::text, ''), COALESCE(approver.display_name, ''), COALESCE(cr.verifier_id::text, ''), COALESCE(verifier.display_name, ''),
	       cr.approver_opinion, cr.verify_waived_reason,
	       cr.direct_admin_approval, cr.approved_at, cr.submitted_at, cr.completed_at, cr.created_at, cr.actual_changes, cr.proposed_attributes,
	       COALESCE(cr.current_submission_id::text, '')
	FROM change_requests cr
	LEFT JOIN users ap ON ap.id = cr.applicant_id
	LEFT JOIN users ex ON ex.id = cr.executor_id
	LEFT JOIN users approver ON approver.id = cr.approver_id
	LEFT JOIN users verifier ON verifier.id = cr.verifier_id`

func (s *PGService) List(ctx context.Context, filter ListFilter) ([]Request, error) {
	var conditions []string
	var args []any
	if filter.DrawingID != "" {
		args = append(args, filter.DrawingID)
		conditions = append(conditions, fmt.Sprintf("cr.drawing_id = $%d::uuid", len(args)))
	}
	if filter.Status != "" {
		args = append(args, string(filter.Status))
		conditions = append(conditions, fmt.Sprintf("cr.status = $%d", len(args)))
	}
	if filter.ExecutorID != "" {
		args = append(args, filter.ExecutorID)
		conditions = append(conditions, fmt.Sprintf("cr.executor_id = $%d::uuid", len(args)))
	}
	if filter.OpenOnly {
		conditions = append(conditions, "cr.status IN ('pending_approval','executing','pending_verify')")
	}
	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}
	rows, err := s.pool.Query(ctx, requestSelect+" "+where+" ORDER BY cr.created_at DESC", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Request, 0)
	for rows.Next() {
		item, err := scanRequest(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *PGService) ListByDrawing(ctx context.Context, drawingID string) ([]Request, error) {
	return s.List(ctx, ListFilter{DrawingID: drawingID})
}

func (s *PGService) loadDiffs(ctx context.Context, requestID string) ([]Diff, error) {
	var currentSubmission *string
	err := s.pool.QueryRow(ctx, `SELECT current_submission_id::text FROM change_requests WHERE id = $1::uuid`, requestID).Scan(&currentSubmission)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT kind, field, old_value, new_value
		FROM change_request_diffs
		WHERE request_id = $1::uuid
		  AND ($2::uuid IS NULL AND submission_id IS NULL OR submission_id = $2::uuid)
		  AND NOT EXISTS (SELECT 1 FROM change_request_submissions sub WHERE sub.id = submission_id AND sub.legacy_history)
		ORDER BY created_at`, requestID, currentSubmission)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Diff, 0)
	for rows.Next() {
		var d Diff
		if err := rows.Scan(&d.Kind, &d.Field, &d.OldValue, &d.NewValue); err != nil {
			return nil, err
		}
		items = append(items, d)
	}
	return items, rows.Err()
}

// loadSubmissions 按轮次返回历次提交快照及其差异，供待验收与历史查看使用。
func (s *PGService) loadSubmissions(ctx context.Context, requestID string) ([]Submission, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT sub.id::text, sub.round, COALESCE(u.display_name, ''), sub.actual_changes, sub.status, sub.created_at, sub.proposed_attributes, sub.legacy_history
		FROM change_request_submissions sub
		LEFT JOIN users u ON u.id = sub.submitted_by
		WHERE sub.request_id = $1::uuid
		ORDER BY sub.round`, requestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Submission, 0)
	index := map[string]int{}
	for rows.Next() {
		var sub Submission
		var proposedRaw []byte
		if err := rows.Scan(&sub.ID, &sub.Round, &sub.ActorName, &sub.ActualChanges, &sub.Status, &sub.CreatedAt, &proposedRaw, &sub.LegacyHistory); err != nil {
			return nil, err
		}
		if len(proposedRaw) > 0 {
			_ = json.Unmarshal(proposedRaw, &sub.Proposed)
		}
		sub.Diffs = make([]Diff, 0)
		index[sub.ID] = len(items)
		items = append(items, sub)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	diffRows, err := s.pool.Query(ctx, `
		SELECT submission_id::text, kind, field, old_value, new_value
		FROM change_request_diffs
		WHERE request_id = $1::uuid AND submission_id IS NOT NULL
		ORDER BY created_at`, requestID)
	if err != nil {
		return nil, err
	}
	defer diffRows.Close()
	for diffRows.Next() {
		var submissionID string
		var d Diff
		if err := diffRows.Scan(&submissionID, &d.Kind, &d.Field, &d.OldValue, &d.NewValue); err != nil {
			return nil, err
		}
		if pos, ok := index[submissionID]; ok {
			items[pos].Diffs = append(items[pos].Diffs, d)
		}
	}
	return items, diffRows.Err()
}

func (s *PGService) loadActions(ctx context.Context, requestID string) ([]ActionRecord, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT a.id::text, a.action, a.opinion, COALESCE(u.display_name, ''), a.created_at
		FROM change_request_actions a LEFT JOIN users u ON u.id = a.actor_id
		WHERE a.request_id = $1::uuid ORDER BY a.created_at`, requestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]ActionRecord, 0)
	for rows.Next() {
		var rec ActionRecord
		var action string
		if err := rows.Scan(&rec.ID, &action, &rec.Opinion, &rec.ActorName, &rec.CreatedAt); err != nil {
			return nil, err
		}
		rec.Action = Action(action)
		items = append(items, rec)
	}
	return items, rows.Err()
}

func scanRequest(row scanner) (Request, error) {
	var item Request
	var status string
	var proposedRaw []byte
	err := row.Scan(&item.ID, &item.RequestNo, &item.DrawingID, &item.DrawingNo, &item.Title, &item.Reason, &item.Scope, &status,
		&item.RequireVerify, &item.ApplicantID, &item.ApplicantName, &item.ExecutorID, &item.ExecutorName,
		&item.ApproverID, &item.ApproverName, &item.VerifierID, &item.VerifierName,
		&item.ApproverOpinion, &item.WaiveVerifyReason, &item.DirectAdmin,
		&item.ApprovedAt, &item.SubmittedAt, &item.CompletedAt, &item.CreatedAt, &item.ActualChanges, &proposedRaw, &item.CurrentSubmissionID)
	if err != nil {
		return Request{}, err
	}
	item.Status = Status(status)
	if len(proposedRaw) > 0 {
		_ = json.Unmarshal(proposedRaw, &item.Proposed)
	}
	return item, nil
}

func isUnique(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// isOpenRequestConflict 精确识别"同一图纸已有未结束工单"的部分唯一索引冲突，
// 避免把工单号唯一键冲突等误判为重复申请。
func isOpenRequestConflict(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "uq_change_requests_open_drawing"
}
