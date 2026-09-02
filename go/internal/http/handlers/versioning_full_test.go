package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"cadguanliq/internal/attachment"
	"cadguanliq/internal/auth"
	"cadguanliq/internal/config"
	"cadguanliq/internal/editing"
	"cadguanliq/internal/http/middleware"
	"cadguanliq/internal/storage"
	"cadguanliq/internal/versioning"
)

// tolerantTempDir 创建测试目录并容忍清理失败（Windows 杀毒/索引服务可能短暂占用新文件）。
func tolerantTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "handlers-test-")
	if err != nil {
		t.Fatalf("MkdirTemp() error = %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// ---------------------------------------------------------------------------
// versioning fake：内存版本仓库 + 附件仓库
// ---------------------------------------------------------------------------

type fakeVersionRepo struct {
	versioning.Repository
	mu           sync.Mutex
	versions     map[string]versioning.Version
	byStorageKey map[string]string
	byAttVersion map[string]string
}

func newFakeVersionRepo() *fakeVersionRepo {
	return &fakeVersionRepo{
		versions:     make(map[string]versioning.Version),
		byStorageKey: make(map[string]string),
		byAttVersion: make(map[string]string),
	}
}

func (repo *fakeVersionRepo) Create(ctx context.Context, input versioning.CreateInput, storageKey string) (versioning.Version, error) {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	if _, exists := repo.byStorageKey[storageKey]; exists {
		return versioning.Version{}, errors.New("duplicate storage key")
	}
	id := fmt.Sprintf("ver-%03d", len(repo.versions)+1)
	version := versioning.Version{
		ID: id, AttachmentID: input.AttachmentID, StorageKey: storageKey,
		SourceStorageKey: input.SourceStorageKey, Version: input.Version, VersionKind: input.VersionKind,
		Size: input.Size, MimeType: input.MimeType, SHA256: input.SHA256, CreatedBy: input.CreatedBy,
	}
	repo.versions[id] = version
	repo.byStorageKey[storageKey] = id
	repo.byAttVersion[input.AttachmentID+"|"+input.Version] = id
	return version, nil
}

func (repo *fakeVersionRepo) CreateWithPromotion(ctx context.Context, input versioning.CreateInput, storageKey string) (versioning.Version, error) {
	return repo.Create(ctx, input, storageKey)
}

func (repo *fakeVersionRepo) GetByID(ctx context.Context, versionID string) (versioning.Version, error) {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	version, ok := repo.versions[versionID]
	if !ok {
		return versioning.Version{}, versioning.ErrNotFound
	}
	return version, nil
}

func (repo *fakeVersionRepo) GetByStorageKey(ctx context.Context, storageKey string) (versioning.Version, error) {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	id, ok := repo.byStorageKey[storageKey]
	if !ok {
		return versioning.Version{}, versioning.ErrNotFound
	}
	return repo.versions[id], nil
}

func (repo *fakeVersionRepo) ListByAttachment(ctx context.Context, attachmentID string) ([]versioning.Version, error) {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	list := make([]versioning.Version, 0)
	for _, version := range repo.versions {
		if version.AttachmentID == attachmentID {
			list = append(list, version)
		}
	}
	return list, nil
}

func (repo *fakeVersionRepo) LatestByAttachment(ctx context.Context, attachmentID string) (versioning.Version, error) {
	list, err := repo.ListByAttachment(ctx, attachmentID)
	if err != nil || len(list) == 0 {
		return versioning.Version{}, versioning.ErrNotFound
	}
	return list[len(list)-1], nil
}

func (repo *fakeVersionRepo) PromoteInitial(ctx context.Context, versionID string) error { return nil }
func (repo *fakeVersionRepo) Retain(ctx context.Context, versionID, userID string) (versioning.Version, error) {
	return repo.GetByID(ctx, versionID)
}
func (repo *fakeVersionRepo) Release(ctx context.Context, versionID, userID string) (versioning.Version, error) {
	return repo.GetByID(ctx, versionID)
}
func (repo *fakeVersionRepo) ListExpired(ctx context.Context, now time.Time) ([]versioning.Version, error) {
	return nil, nil
}
func (repo *fakeVersionRepo) MarkDeleted(ctx context.Context, versionID string) error { return nil }

type fakeAttForVersion struct {
	attachment.Repository
	item attachment.Attachment
}

func (repo *fakeAttForVersion) Find(ctx context.Context, storageKey string) (attachment.Attachment, error) {
	if repo.item.StorageKey == "" {
		return attachment.Attachment{}, attachment.ErrNotFound
	}
	return repo.item, nil
}

func authenticatedVersionGet(target string) *http.Request {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	user := auth.AuthUser{ID: "user-1", Account: "zhang", DisplayName: "张三", Roles: []string{"designer"}}
	return request.WithContext(context.WithValue(request.Context(), middleware.AuthUserContextKey, user))
}

// ---------------------------------------------------------------------------
// 版本源接口（历史版本在线浏览）
// ---------------------------------------------------------------------------

func versionSourceFixture(t *testing.T) (*storage.LocalStorage, *versioning.Service, versioning.Version) {
	t.Helper()
	root, err := storage.NewLocalStorage(tolerantTempDir(t))
	if err != nil {
		t.Fatalf("NewLocalStorage() error = %v", err)
	}
	repo := newFakeVersionRepo()
	attRepo := &fakeAttForVersion{item: attachment.Attachment{
		ID: "att-001", StorageKey: "drawings/JG-00/泵缸.exb", Name: "泵缸.exb",
	}}
	service := versioning.NewService(repo, attRepo, root)

	dwgKey := "drawings/JG-00/history/泵缸/v1.0-w001/泵缸.dwg"
	if _, err := root.Put(context.Background(), dwgKey, bytes.NewReader([]byte("version-dwg-bytes")), "application/acad"); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	created, err := repo.Create(context.Background(), versioning.CreateInput{
		AttachmentID:     "att-001",
		SourceStorageKey: "drawings/JG-00/泵缸.exb",
		Version:          "v1.0-w001",
		VersionKind:      "working",
	}, dwgKey)
	if err != nil {
		t.Fatalf("创建测试版本失败: %v", err)
	}
	return root, service, created
}

func TestVersionSourceByID(t *testing.T) {
	_, service, created := versionSourceFixture(t)
	handler := versioning.Resource(service, nil)

	request := authenticatedVersionGet("/api/file-versions/" + created.ID + "/source")
	recorder := httptest.NewRecorder()
	handler(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("source status = %d body=%s; want 200", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Body.String(); got != "version-dwg-bytes" {
		t.Fatalf("source 内容 = %q", got)
	}
	if disposition := recorder.Header().Get("Content-Disposition"); !strings.HasPrefix(disposition, "inline") {
		t.Fatalf("在线浏览应 inline 输出, got %q", disposition)
	}
}

func TestVersionSourceByStorageKey(t *testing.T) {
	_, service, _ := versionSourceFixture(t)
	handler := versioning.Resource(service, nil)

	dwgKey := "drawings/JG-00/history/泵缸/v1.0-w001/泵缸.dwg"
	request := authenticatedVersionGet("/api/file-versions/source?storageKey=" + encodeAttachmentKeyPathForTest(dwgKey))
	recorder := httptest.NewRecorder()
	handler(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("source status = %d body=%s; want 200", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Body.String(); got != "version-dwg-bytes" {
		t.Fatalf("source 内容 = %q", got)
	}
}

func TestVersionSourceByStorageKeyMissing(t *testing.T) {
	_, service, _ := versionSourceFixture(t)
	handler := versioning.Resource(service, nil)

	request := authenticatedVersionGet("/api/file-versions/source?storageKey=drawings/JG-00/不存在.dwg")
	recorder := httptest.NewRecorder()
	handler(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("未知存储键 status = %d; want 400", recorder.Code)
	}
}

func TestVersionSourceRequiresStorageKey(t *testing.T) {
	_, service, _ := versionSourceFixture(t)
	handler := versioning.Resource(service, nil)

	request := authenticatedVersionGet("/api/file-versions/source")
	recorder := httptest.NewRecorder()
	handler(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("缺少 storageKey status = %d; want 400", recorder.Code)
	}
}

func TestVersionListByStorageKey(t *testing.T) {
	_, service, _ := versionSourceFixture(t)
	handler := versioning.List(service)

	dwgKey := "drawings/JG-00/history/泵缸/v1.0-w001/泵缸.dwg"
	request := authenticatedVersionGet("/api/file-versions?storageKey=" + encodeAttachmentKeyPathForTest(dwgKey))
	recorder := httptest.NewRecorder()
	handler(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("list status = %d body=%s; want 200", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Data []versioning.Version `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("解析版本列表失败: %v body=%s", err, recorder.Body.String())
	}
	if len(payload.Data) != 1 || payload.Data[0].Version != "v1.0-w001" {
		t.Fatalf("版本列表 = %+v", payload.Data)
	}
}

// ---------------------------------------------------------------------------
// 上传 CAD 转换失败回滚
// ---------------------------------------------------------------------------

type rollbackTrackingRepo struct {
	attachment.Repository
	mu           sync.Mutex
	createCalled bool
	deleteCalled bool
	createdItem  attachment.Attachment
}

func (repo *rollbackTrackingRepo) FolderForDrawing(ctx context.Context, drawingNo string) (string, error) {
	return "JG-00(测试项目)", nil
}

func (repo *rollbackTrackingRepo) Find(ctx context.Context, storageKey string) (attachment.Attachment, error) {
	return attachment.Attachment{}, attachment.ErrNotFound
}

func (repo *rollbackTrackingRepo) Create(ctx context.Context, input attachment.CreateInput, object attachment.StorageObject, userID string) (attachment.Attachment, error) {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	repo.createCalled = true
	repo.createdItem = attachment.Attachment{
		ID:         "att-new",
		StorageKey: object.Key,
		Name:       input.Name,
		MimeType:   input.MimeType,
		Size:       object.Size,
	}
	return repo.createdItem, nil
}

func (repo *rollbackTrackingRepo) Delete(ctx context.Context, storageKey string, userID string) error {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	repo.deleteCalled = true
	return nil
}

func uploadRequest(t *testing.T, fileName, content string) *http.Request {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	filePart, _ := writer.CreateFormFile("file", fileName)
	_, _ = filePart.Write([]byte(content))
	_ = writer.WriteField("drawingNo", "JG-00")
	_ = writer.WriteField("role", "assembly")
	writer.Close()

	request := httptest.NewRequest(http.MethodPost, "/api/attachments", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	user := auth.AuthUser{ID: "user-1", Account: "zhang", DisplayName: "张三", Roles: []string{"designer"}}
	return request.WithContext(context.WithValue(request.Context(), middleware.AuthUserContextKey, user))
}

func TestUploadAttachmentCADRollsBackWhenConverterMissing(t *testing.T) {
	objectStorage, err := storage.NewLocalStorage(tolerantTempDir(t))
	if err != nil {
		t.Fatalf("NewLocalStorage() error = %v", err)
	}
	repo := &rollbackTrackingRepo{}

	request := uploadRequest(t, "泵缸.exb", "fake-exb-content")
	handler := UploadAttachment(repo, objectStorage, nil, nil, 0)

	recorder := httptest.NewRecorder()
	handler(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("转换服务缺失 status = %d body=%s; want 500", recorder.Code, recorder.Body.String())
	}
	repo.mu.Lock()
	defer repo.mu.Unlock()
	if !repo.createCalled || !repo.deleteCalled {
		t.Fatalf("转换失败必须回滚: create=%v delete=%v", repo.createCalled, repo.deleteCalled)
	}
	// 原始对象也必须删除，不残留半成品
	if _, _, openErr := objectStorage.Open(context.Background(), repo.createdItem.StorageKey); openErr == nil {
		t.Fatalf("回滚后对象仍存在: %s", repo.createdItem.StorageKey)
	}
}

func TestUploadAttachmentNonCADSkipsConversion(t *testing.T) {
	objectStorage, err := storage.NewLocalStorage(tolerantTempDir(t))
	if err != nil {
		t.Fatalf("NewLocalStorage() error = %v", err)
	}
	repo := &rollbackTrackingRepo{}

	request := uploadRequest(t, "说明书.pdf", "pdf-bytes")
	handler := UploadAttachment(repo, objectStorage, nil, nil, 0)

	recorder := httptest.NewRecorder()
	handler(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("非 CAD 上传 status = %d body=%s; want 201", recorder.Code, recorder.Body.String())
	}
	repo.mu.Lock()
	defer repo.mu.Unlock()
	if repo.deleteCalled {
		t.Fatalf("非 CAD 上传不应触发回滚删除")
	}
	if _, _, openErr := objectStorage.Open(context.Background(), repo.createdItem.StorageKey); openErr != nil {
		t.Fatalf("非 CAD 上传对象应保留: %v", openErr)
	}
}

// ---------------------------------------------------------------------------
// 编辑会话关闭接口契约
// ---------------------------------------------------------------------------

func TestEditSessionCloseWithoutDatabase(t *testing.T) {
	// 服务未装配数据库时关闭应返回明确错误，而不是静默成功。
	service := editing.NewService(nil, nil, nil, nil, config.SMBConfig{})
	handler := EditSessionResource(service)

	request := authenticatedGet("/api/edit-sessions/abc/close")
	request.Method = http.MethodPost
	recorder := httptest.NewRecorder()
	handler(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("close status = %d body=%s; want 400", recorder.Code, recorder.Body.String())
	}
}
