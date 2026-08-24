package storage

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestLocalStoragePutOpenDelete(t *testing.T) {
	local, err := NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	info, err := local.Put(context.Background(), "2000W.02.03d(总图)/part.exb", strings.NewReader("cad data"), "application/octet-stream")
	if err != nil {
		t.Fatal(err)
	}
	if info.Size != 8 || info.SHA256 == "" {
		t.Fatalf("unexpected object info: %+v", info)
	}
	reader, _, err := local.Open(context.Background(), info.Key)
	if err != nil {
		t.Fatal(err)
	}
	content, err := io.ReadAll(reader)
	reader.Close()
	if err != nil || string(content) != "cad data" {
		t.Fatalf("unexpected content: %q, error: %v", content, err)
	}
	if err := local.Delete(context.Background(), info.Key); err != nil {
		t.Fatal(err)
	}
}

func TestLocalStorageRejectsPathTraversal(t *testing.T) {
	local, err := NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := local.Put(context.Background(), "../outside/file.exb", strings.NewReader("bad"), ""); err == nil {
		t.Fatal("expected path traversal to be rejected")
	}
}
