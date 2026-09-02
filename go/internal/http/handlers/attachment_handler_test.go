package handlers

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"cadguanliq/internal/attachment"
	"cadguanliq/internal/auth"
	"cadguanliq/internal/http/middleware"
	"cadguanliq/internal/storage"
)

func authenticatedGet(target string) *http.Request {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	user := auth.AuthUser{ID: "test-user", Account: "admin", DisplayName: "Administrator", Roles: []string{"admin"}}
	return request.WithContext(context.WithValue(request.Context(), middleware.AuthUserContextKey, user))
}

func TestAttachmentResourceDownloadsWithoutRecord(t *testing.T) {
	root := t.TempDir()
	objectStorage, err := storage.NewLocalStorage(root)
	if err != nil {
		t.Fatalf("NewLocalStorage() error = %v", err)
	}
	// 与真实场景一致：CAXA 插件转换出的 DWG 直接写入对象存储，没有附件记录。
	dwgKey := "JG6852-70-50-00(篦冷机油缸)总图.dwg"
	want := bytes.Repeat([]byte{0x41, 0x43, 0x10, 0x32, 0x00, 0xFF}, 4096)
	if _, putErr := objectStorage.Put(context.Background(), dwgKey, bytes.NewReader(want), "application/acad"); putErr != nil {
		t.Fatalf("Put() error = %v", putErr)
	}

	handler := AttachmentResource(&findNotFoundRepo{}, objectStorage)
	request := authenticatedGet("/api/attachments/" + encodeAttachmentKeyPathForTest(dwgKey))
	recorder := httptest.NewRecorder()
	handler(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body=%s; want 200", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Body.Bytes(); !bytes.Equal(got, want) {
		t.Fatalf("GET body 大小 = %d, want %d; 内容不一致", len(got), len(want))
	}
}

func TestAttachmentResourceDownloadsWithRecord(t *testing.T) {
	root := t.TempDir()
	objectStorage, err := storage.NewLocalStorage(root)
	if err != nil {
		t.Fatalf("NewLocalStorage() error = %v", err)
	}
	dwgKey := "drawings/JG-00/铜套.dwg"
	want := bytes.Repeat([]byte{0x10, 0x32, 0x00, 0xFF}, 2048)
	if _, putErr := objectStorage.Put(context.Background(), dwgKey, bytes.NewReader(want), "application/acad"); putErr != nil {
		t.Fatalf("Put() error = %v", putErr)
	}

	repo := &singleRecordRepo{item: attachment.Attachment{
		StorageKey: dwgKey,
		Name:       "铜套.dwg",
		MimeType:   "application/acad",
	}}
	handler := AttachmentResource(repo, objectStorage)
	request := authenticatedGet("/api/attachments/" + encodeAttachmentKeyPathForTest(dwgKey))
	recorder := httptest.NewRecorder()
	handler(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body=%s; want 200", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Body.Bytes(); !bytes.Equal(got, want) {
		t.Fatalf("GET body 大小 = %d, want %d; 内容不一致", len(got), len(want))
	}
}

type findNotFoundRepo struct {
	attachment.Repository
}

func (repo *findNotFoundRepo) Find(ctx context.Context, storageKey string) (attachment.Attachment, error) {
	return attachment.Attachment{}, attachment.ErrNotFound
}

type singleRecordRepo struct {
	attachment.Repository
	item attachment.Attachment
}

func (repo *singleRecordRepo) Find(ctx context.Context, storageKey string) (attachment.Attachment, error) {
	return repo.item, nil
}

func encodeAttachmentKeyPathForTest(key string) string {
	segments := strings.Split(strings.ReplaceAll(key, "\\", "/"), "/")
	for index, segment := range segments {
		segments[index] = url.PathEscape(segment)
	}
	return strings.Join(segments, "/")
}
