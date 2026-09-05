package handlers

import (
	"cadguanliq/internal/attachment"
	"cadguanliq/internal/auth"
	"cadguanliq/internal/http/middleware"
	"cadguanliq/internal/storage"
	"context"
	"net/http/httptest"
	"strings"
	"testing"
)

type currentCADRepository struct {
	attachment.Repository
	item attachment.Attachment
}

func (r currentCADRepository) FindByID(context.Context, string) (attachment.Attachment, error) {
	return r.item, nil
}

func TestCADSourceUsesCurrentFormatWithEqualBlobKeys(t *testing.T) {
	store, err := storage.NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Put(context.Background(), "blobs/current", strings.NewReader("AC1027-test"), "application/acad")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"part.dwg", "part.exb"} {
		t.Run(name, func(t *testing.T) {
			repo := currentCADRepository{item: attachment.Attachment{Name: "part.exb", CurrentName: name, StorageKey: "blobs/current", CurrentStorageKey: "blobs/current"}}
			req := httptest.NewRequest("GET", "/api/cad/source?attachmentId=test", nil)
			req = req.WithContext(context.WithValue(req.Context(), middleware.AuthUserContextKey, auth.AuthUser{}))
			out := httptest.NewRecorder()
			CADSource(repo, store, nil)(out, req)
			want := 200
			if name == "part.exb" {
				want = 409
			}
			if out.Code != want {
				t.Fatalf("status=%d body=%s", out.Code, out.Body.String())
			}
			if want == 200 && out.Body.String() != "AC1027-test" {
				t.Fatal("wrong current content")
			}
		})
	}
}
