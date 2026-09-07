package drawing

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestUpdatePartRequiresRelationRevision(t *testing.T) {
	revision := int64(1)
	zero := int64(0)
	qty := float64(2)
	remark := "updated"
	relationID := "relation"
	for _, input := range []UpdatePartInput{
		{ExpectedRevision: &revision, RelationID: &relationID, Quantity: &qty},
		{ExpectedRevision: &revision, RelationID: &relationID, Remark: &remark},
		{ExpectedRevision: &revision, ExpectedRelationRevision: &zero, RelationID: &relationID, Quantity: &qty},
	} {
		_, err := (&PGRepository{}).UpdatePart(context.Background(), "part", input, "user")
		if !errors.Is(err, ErrRevisionRequired) {
			t.Fatalf("expected revision guard, got %v", err)
		}
	}
}

func TestBorrowPointerValues(t *testing.T) {
	a, b, c := "A", "A", "B"
	i, j, k := 1, 1, 2
	if !equalStringPtr(&a, &b) || equalStringPtr(&a, &c) || equalStringPtr(nil, &a) || equalStringPtr(&a, nil) || !equalStringPtr(nil, nil) {
		t.Fatal("string pointer comparison must compare nullable values")
	}
	if !equalIntPtr(&i, &j) || equalIntPtr(&i, &k) || equalIntPtr(nil, &i) || equalIntPtr(&i, nil) || !equalIntPtr(nil, nil) {
		t.Fatal("integer pointer comparison must compare nullable values")
	}
}

type snapshotQueryRecorder struct {
	query string
	args  []any
}
type missingSnapshotRow struct{}

func (missingSnapshotRow) Scan(...any) error { return pgx.ErrNoRows }
func (r *snapshotQueryRecorder) QueryRow(_ context.Context, query string, args ...any) pgx.Row {
	r.query, r.args = query, args
	return missingSnapshotRow{}
}

func TestSnapshotUsesExplicitRelationAndRevision(t *testing.T) {
	db := &snapshotQueryRecorder{}
	_, err := findPartSnapshot(context.Background(), db, "part", "borrowed-relation", "draft-revision")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(db.args) != 3 || db.args[1] != "draft-revision" || db.args[2] != "borrowed-relation" {
		t.Fatalf("lost update context: %v", db.args)
	}
	if !strings.Contains(db.query, "COALESCE(NULLIF($2, '')::uuid, p.published_revision_id") || !strings.Contains(db.query, "AND r.id = $3::uuid") {
		t.Fatal("snapshot must prefer the updated revision and constrain the requested relation")
	}
}
