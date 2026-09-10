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

	handler := AttachmentResource(nil, &findNotFoundRepo{}, objectStorage)
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
	handler := AttachmentResource(nil, repo, objectStorage)
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

func TestAttachmentResourceUpdatesDocumentAuthor(t *testing.T) {
	for _, role := range []attachment.Role{attachment.RoleMaterial, attachment.RoleCraft} {
		t.Run(string(role), func(t *testing.T) {
			repo := &metadataRecordRepo{item: attachment.Attachment{
				ID:                "11111111-1111-4111-8111-111111111111",
				StorageKey:        "blobs/document-source",
				CurrentStorageKey: "blobs/document-source",
				Role:              role,
			}}
			request := httptest.NewRequest(http.MethodPatch, "/api/attachments/blobs/document-source?attachmentId="+repo.item.ID, strings.NewReader(`{"author":" 熊焱林 "}`))
			request = request.WithContext(context.WithValue(request.Context(), middleware.AuthUserContextKey, auth.AuthUser{ID: "test-user", Roles: []string{"user"}}))
			recorder := httptest.NewRecorder()

			AttachmentResource(nil, repo, nil).ServeHTTP(recorder, request)

			if recorder.Code != http.StatusOK {
				t.Fatalf("PATCH status = %d, body=%s; want 200", recorder.Code, recorder.Body.String())
			}
			if repo.author != "熊焱林" {
				t.Fatalf("author = %q; want 熊焱林", repo.author)
			}
		})
	}
}

func TestAttachmentResourceUsesCurrentNameForCurrentBlob(t *testing.T) {
	root := t.TempDir()
	objectStorage, err := storage.NewLocalStorage(root)
	if err != nil {
		t.Fatalf("NewLocalStorage() error = %v", err)
	}
	currentKey := "blobs/4dff4e1de885303ed8b56bb9b0df695d68fb50fd6446be0f74fc914460adc7e7"
	want := []byte("valid-dwg-content")
	if _, putErr := objectStorage.Put(context.Background(), currentKey, bytes.NewReader(want), "application/acad"); putErr != nil {
		t.Fatalf("Put() error = %v", putErr)
	}

	repo := &singleRecordRepo{item: attachment.Attachment{
		StorageKey:        "blobs/original-exb-hash",
		CurrentStorageKey: currentKey,
		Name:              "工程图文档2.exb",
		CurrentName:       "工程图文档2.dwg",
		MimeType:          "application/octet-stream",
		CurrentMimeType:   "application/acad",
	}}
	handler := AttachmentResource(nil, repo, objectStorage)
	request := authenticatedGet("/api/attachments/" + encodeAttachmentKeyPathForTest(currentKey))
	recorder := httptest.NewRecorder()
	handler(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body=%s; want 200", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Body.Bytes(); !bytes.Equal(got, want) {
		t.Fatalf("GET body = %q; want %q", got, want)
	}
	if got := recorder.Header().Get("Content-Disposition"); !strings.Contains(got, url.PathEscape("工程图文档2.dwg")) {
		t.Fatalf("Content-Disposition = %q; want current DWG name", got)
	}
}

type findNotFoundRepo struct {
	attachment.Repository
}

func (repo *findNotFoundRepo) Find(ctx context.Context, storageKey string) (attachment.Attachment, error) {
	return attachment.Attachment{}, attachment.ErrNotFound
}

type metadataRecordRepo struct {
	attachment.Repository
	item   attachment.Attachment
	author string
}

func (repo *metadataRecordRepo) Find(context.Context, string) (attachment.Attachment, error) {
	return repo.item, nil
}

func (repo *metadataRecordRepo) FindByID(context.Context, string) (attachment.Attachment, error) {
	return repo.item, nil
}

func (repo *metadataRecordRepo) DeleteByID(context.Context, string, string) (attachment.Attachment, error) {
	return attachment.Attachment{}, nil
}

func (repo *metadataRecordRepo) UpdateAuthorByID(_ context.Context, _ string, author string) error {
	repo.author = strings.TrimSpace(author)
	return nil
}

func (repo *metadataRecordRepo) HasStorageKeyReference(context.Context, string) (bool, error) {
	return false, nil
}

func (repo *metadataRecordRepo) ReidentifyPartByID(context.Context, string, string, string) (attachment.ReidentifyResult, error) {
	return attachment.ReidentifyResult{}, nil
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
