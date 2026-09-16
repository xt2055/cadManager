package dbtest

import (
	"context"
	"errors"
	"testing"

	"cadguanliq/internal/annotation"
)

func TestAnnotationsPinVersionAndLockAfterSigning(t *testing.T) {
	db := New(t)
	f := setupSignedReview(t, db)
	ctx := context.Background()
	repo := annotation.NewRepository(db.Pool)
	attachmentID := db.ScanString(t, `SELECT attachment_id::text FROM attachment_versions WHERE id=$1::uuid`, f.Version)
	w, err := repo.Load(ctx, f.CaseID, attachmentID, f.Reviewer)
	if err != nil {
		t.Fatal(err)
	}
	if !w.CanEdit || w.VersionID != f.Version {
		t.Fatalf("bad scope: %+v", w)
	}
	in := annotation.SaveInput{CaseID: f.CaseID, AttachmentID: attachmentID, VersionID: w.VersionID, NodeID: w.NodeID, Content: annotation.Content{SchemaVersion: 1, Marks: []annotation.Mark{{ID: "test", Kind: "check", Layout: "Model", Points: []annotation.Point{{X: 10, Y: 20}}, Color: "#FF6868", Width: 3}}}}
	if _, err = repo.Save(ctx, in, f.Author); !errors.Is(err, annotation.ErrForbidden) {
		t.Fatalf("wrong author: %v", err)
	}
	wrong := in
	wrong.VersionID = f.Drawing
	if _, err = repo.Save(ctx, wrong, f.Reviewer); !errors.Is(err, annotation.ErrNotFound) {
		t.Fatalf("wrong version accepted: %v", err)
	}
	saved, err := repo.Save(ctx, in, f.Reviewer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.Save(ctx, in, f.Reviewer); !errors.Is(err, annotation.ErrConflict) {
		t.Fatalf("duplicate stale save: %v", err)
	}
	in.Revision = saved.Revision
	in.Content.Marks[0].Text = "缺少尺寸"
	saved, err = repo.Save(ctx, in, f.Reviewer)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Revision != 2 {
		t.Fatalf("revision=%d", saved.Revision)
	}
	if got := db.ScanString(t, `SELECT count(*)::text FROM review_annotation_events WHERE document_id=$1::uuid`, saved.ID); got != "2" {
		t.Fatal("missing history")
	}
	if _, err = db.Pool.Exec(ctx, `UPDATE attachment_versions SET deleted_at=now() WHERE id=$1::uuid`, f.Version); err == nil {
		t.Fatal("referenced version was deleted")
	}
	if err = signNode(t, f.Repository, f.CaseID, "专业审核", "rejected", "补充尺寸", f.Reviewer); err != nil {
		t.Fatal(err)
	}
	in.Revision = 2
	if _, err = repo.Save(ctx, in, f.Reviewer); !errors.Is(err, annotation.ErrForbidden) {
		t.Fatalf("signed annotation modified: %v", err)
	}
	w, err = repo.Load(ctx, f.CaseID, attachmentID, f.Author)
	if err != nil {
		t.Fatal(err)
	}
	if w.CanEdit || len(w.Documents) != 1 || w.Documents[0].Content.Marks[0].Text != "缺少尺寸" {
		t.Fatalf("history lost: %+v", w)
	}
}

func TestAnnotationTemplateOwnership(t *testing.T) {
	db := New(t)
	f := db.Seed(t)
	ctx := context.Background()
	repo := annotation.NewRepository(db.Pool)
	if _, err := repo.CreateTemplate(ctx, annotation.Template{Category: "尺寸", Text: "补充尺寸"}, f.Reviewer, false); !errors.Is(err, annotation.ErrForbidden) {
		t.Fatal("non-admin created public template")
	}
	item, err := repo.CreateTemplate(ctx, annotation.Template{OwnerID: f.Author, Category: "尺寸", Text: "补充尺寸"}, f.Reviewer, false)
	if err != nil {
		t.Fatal(err)
	}
	if item.OwnerID != f.Reviewer {
		t.Fatal("spoofed owner")
	}
	if err = repo.DeleteTemplate(ctx, item.ID, f.Author, true); !errors.Is(err, annotation.ErrForbidden) {
		t.Fatal("another user's personal template deleted")
	}
	if err = repo.DeleteTemplate(ctx, item.ID, f.Reviewer, false); err != nil {
		t.Fatal(err)
	}
}
