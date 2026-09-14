package editing

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"cadguanliq/internal/attachment"
	"cadguanliq/internal/storage"
)

func TestWaitForCurrentDWGUsesCommittedBlobAfterTemporaryFileCleanup(t *testing.T) {
	objects, err := storage.NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := &Service{storage: objects}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	reads := 0
	key, err := service.waitForCurrentDWG(ctx, "attachment-b", func(ctx context.Context, id string) (attachment.Attachment, error) {
		if id != "attachment-b" {
			t.Fatalf("wrong attachment: %s", id)
		}
		reads++
		if reads == 1 {
			return attachment.Attachment{CurrentName: "blank.exb", CurrentStorageKey: "blobs/shared-template"}, nil
		}
		// 临时转换文件未保留；只有队列已归档的正式内容对象存在。
		if _, err := objects.Put(ctx, "blobs/committed-dwg", bytes.NewBufferString("AC1015-data"), "application/acad"); err != nil {
			t.Fatal(err)
		}
		return attachment.Attachment{CurrentName: "part-b.dwg", CurrentStorageKey: "blobs/committed-dwg"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if key != "blobs/committed-dwg" {
		t.Fatalf("unexpected work source: %s", key)
	}
}

func TestWaitForCurrentDWGDoesNotReturnMissingOrEmptyBlob(t *testing.T) {
	for _, empty := range []bool{false, true} {
		objects, err := storage.NewLocalStorage(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		if empty {
			if _, err := objects.Put(context.Background(), "blobs/pending", bytes.NewReader(nil), "application/acad"); err != nil {
				t.Fatal(err)
			}
		}
		service := &Service{storage: objects}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
		key, err := service.waitForCurrentDWG(ctx, "a", func(context.Context, string) (attachment.Attachment, error) {
			return attachment.Attachment{CurrentName: "part.dwg", CurrentStorageKey: "blobs/pending"}, nil
		})
		cancel()
		if key != "" || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("empty=%v key=%q err=%v", empty, key, err)
		}
	}
}
