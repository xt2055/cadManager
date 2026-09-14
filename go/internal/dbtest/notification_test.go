package dbtest

import (
	"context"
	"strings"
	"testing"
)

func TestNotificationChangeActions(t *testing.T) {
	db := New(t)
	fx := db.Seed(t)
	db.Exec(t, `INSERT INTO user_roles(user_id,role) VALUES($1::uuid,'admin')`, fx.Reviewer)
	request := insertChangeRequest(t, db, fx, "CR-NOTIFY")
	for _, action := range []string{"create", "approve", "reject", "submit", "verify", "return", "cancel"} {
		id := db.ScanString(t, `INSERT INTO change_request_actions(request_id,actor_id,action,opinion) VALUES($1::uuid,$2::uuid,$3,'测试意见') RETURNING id::text`, request, fx.Author, action)
		var recipient string
		if action == "create" || action == "submit" {
			recipient = fx.Reviewer
		} else {
			recipient = fx.Author
		}
		if got := db.ScanString(t, `SELECT count(*)::text FROM notifications WHERE event_key=$1 AND recipient_id=$2::uuid`, "change:"+id, recipient); got != "1" {
			t.Fatalf("%s missing notification: %s", action, got)
		}
	}
	before := db.ScanString(t, `SELECT count(*)::text FROM notifications`)
	ctx := context.Background()
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO change_request_actions(request_id,actor_id,action) VALUES($1::uuid,$2::uuid,'create')`, request, fx.Author); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if after := db.ScanString(t, `SELECT count(*)::text FROM notifications`); before != after {
		t.Fatal("rolled back action delivered a notification")
	}
}

func TestNotificationReviewResults(t *testing.T) {
	for _, status := range []string{"published", "rejected"} {
		t.Run(status, func(t *testing.T) {
			db := New(t)
			fx := db.Seed(t)
			id := db.ScanString(t, `INSERT INTO review_cases(drawing_id,initiator_id,status) VALUES($1::uuid,$2::uuid,'reviewing') RETURNING id::text`, fx.Drawing, fx.Author)
			action := "reject"
			if status == "published" { action = "pass" }
			db.Exec(t, `INSERT INTO review_actions(review_case_id,actor_id,action,opinion) VALUES($1::uuid,$2::uuid,$3,'尺寸需要复核')`, id, fx.Reviewer, action)
			db.Exec(t, `UPDATE review_cases SET status=$2 WHERE id=$1::uuid`, id, status)
			db.Exec(t, `UPDATE review_cases SET status=$2 WHERE id=$1::uuid`, id, status)
			if got := db.ScanString(t, `SELECT count(*)::text FROM notifications WHERE recipient_id=$1::uuid`, fx.Author); got != "1" {
				t.Fatalf("review result duplicated or missing: %s", got)
			}
			if content := db.ScanString(t, `SELECT content FROM notifications`); !strings.Contains(content, "尺寸需要复核") {
				t.Fatalf("opinion missing: %s", content)
			}
		})
	}
}

func TestNotificationChangeSigning(t *testing.T) {
	for _, action := range []string{"pass", "rejected"} {
		t.Run(action, func(t *testing.T) {
			db := New(t)
			fx := setupSignedReview(t, db)
			if err := signNode(t, fx.Repository, fx.CaseID, "专业审核", action, "审核意见", fx.Reviewer); err != nil {
				t.Fatal(err)
			}
			if got := db.ScanString(t, `SELECT count(*)::text FROM notifications WHERE recipient_id=$1::uuid`, fx.Author); got != "1" {
				t.Fatalf("change result should notify once: %s", got)
			}
			if kind := db.ScanString(t, `SELECT kind FROM notifications LIMIT 1`); kind != "change" {
				t.Fatalf("wrong result kind: %s", kind)
			}
		})
	}
}
