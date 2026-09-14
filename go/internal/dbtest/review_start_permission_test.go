package dbtest

import (
	"context"
	"errors"
	"testing"

	"cadguanliq/internal/review"
)

func TestRegularReviewCannotBypassAssignedChange(t *testing.T) {
	db := New(t)
	fx := db.Seed(t)
	id := insertChangeRequest(t, db, fx, "CR-PERMISSION")
	db.Exec(t, `UPDATE change_requests SET executor_id=$2::uuid WHERE id=$1::uuid`, id, fx.Reviewer)
	repo := review.NewPGRepository(db.Pool)
	for _, actor := range []string{fx.Author, fx.Reviewer} {
		_, err := repo.StartCase(context.Background(), "D-1", actor)
		if !errors.Is(err, review.ErrCaseForbidden) {
			t.Fatalf("普通送审必须拒绝绕过已指派的变更工单，actor=%s err=%v", actor, err)
		}
	}
	if count := db.ScanString(t, `SELECT count(*)::text FROM review_cases`); count != "0" {
		t.Fatalf("越权请求不应创建审核单，实际数量 %s", count)
	}
}
