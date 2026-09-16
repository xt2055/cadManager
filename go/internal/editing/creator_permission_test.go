package editing

import (
	"context"
	"testing"

	"cadguanliq/internal/auth"
	"cadguanliq/internal/drawing"
)

type creatorDrawingLookup struct{ status drawing.Status }

func (lookup creatorDrawingLookup) FindByNo(context.Context, string) (drawing.Drawing, error) {
	return drawing.Drawing{ID: "project", CreatedByID: "creator", Status: lookup.status}, nil
}

func TestCreatorEditPermissionWithoutDesignerRole(t *testing.T) {
	for _, status := range []drawing.Status{drawing.StatusDraft, drawing.StatusReviewing, drawing.StatusPublished, drawing.StatusArchived} {
		for _, id := range []string{"creator", "other"} {
			t.Run(string(status)+"/"+id, func(t *testing.T) {
				service := &Service{drawings: creatorDrawingLookup{status: status}}
				_, err := service.authorizeEdit(context.Background(), auth.AuthUser{ID: id}, "project", "file")
				wantAllowed := id == "creator" && status != drawing.StatusArchived
				if (err == nil) != wantAllowed {
					t.Fatalf("allowed=%v, want %v (error=%v)", err == nil, wantAllowed, err)
				}
			})
		}
	}
}
