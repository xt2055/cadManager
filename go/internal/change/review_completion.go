package change

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
)

func (s *PGService) CompleteReview(ctx context.Context, tx pgx.Tx, submissionID, actorID string, accepted bool, opinion string) error {
	var id, drawingID, drawingNo, status, current string
	err := tx.QueryRow(ctx, `SELECT cr.id::text,cr.drawing_id::text,cr.drawing_no,cr.status,COALESCE(cr.current_submission_id::text,'') FROM change_requests cr JOIN change_request_submissions sub ON sub.request_id=cr.id WHERE sub.id=$1::uuid FOR UPDATE OF cr`, submissionID).Scan(&id, &drawingID, &drawingNo, &status, &current)
	if err != nil {
		return err
	}
	if status != "pending_verify" || current != submissionID {
		return ErrStaleSubmit
	}
	if !accepted {
		if _, err = tx.Exec(ctx, `UPDATE change_requests SET status='executing' WHERE id=$1::uuid`, id); err != nil {
			return err
		}
		if err = s.markSubmission(ctx, tx, submissionID, "returned"); err != nil {
			return err
		}
		return s.logAction(ctx, tx, id, actorID, ActionReturn, "完整审核退回："+opinion)
	}
	var approved bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM review_cases c WHERE c.change_submission_id=$1::uuid AND c.status='published' AND NOT EXISTS(SELECT 1 FROM review_case_nodes n WHERE n.review_case_id=c.id AND n.status<>'pass'))`, submissionID).Scan(&approved); err != nil {
		return err
	}
	if !approved {
		return fmt.Errorf("审核节点尚未全部通过")
	}
	if _, err = tx.Exec(ctx, `UPDATE change_requests SET status='completed',verifier_id=$2::uuid,verified_at=now(),completed_at=now() WHERE id=$1::uuid`, id, actorID); err != nil {
		return err
	}
	if err = s.applyCompletion(ctx, tx, id, drawingID, actorID); err != nil {
		return err
	}
	if err = s.markSubmission(ctx, tx, submissionID, "accepted"); err != nil {
		return err
	}
	if err = s.logAction(ctx, tx, id, actorID, ActionVerify, "完整审核通过，发布本轮冻结版本："+opinion); err != nil {
		return err
	}
	return s.audit(ctx, tx, id, drawingNo, actorID, "change_request_verify", "完整审核通过并发布变更")
}
