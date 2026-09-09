package versioning

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"cadguanliq/internal/attachment"
	"cadguanliq/internal/auth"
	"cadguanliq/internal/storage"
)

// ---------------------------------------------------------------------------
// 测试基建：内存版附件仓库 + 版本仓库（模拟数据库唯一约束与事务语义）
// ---------------------------------------------------------------------------

type fakeAttRepo struct {
	attachment.Repository
	mu             sync.Mutex
	items          map[string]*attachment.Attachment
	setCurrentVoks int
	failSetCurrent bool
}

func newFakeAttRepo(items ...attachment.Attachment) *fakeAttRepo {
	repo := &fakeAttRepo{items: make(map[string]*attachment.Attachment)}
	for i := range items {
		item := items[i]
		repo.items[item.StorageKey] = &item
	}
	return repo
}

func (repo *fakeAttRepo) Find(ctx context.Context, storageKey string) (attachment.Attachment, error) {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	item, ok := repo.items[storageKey]
	if !ok {
		return attachment.Attachment{}, attachment.ErrNotFound
	}
	return *item, nil
}

func (repo *fakeAttRepo) UpdateContent(ctx context.Context, storageKey string, size int64, mimeType, sha256 string) error {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	item, ok := repo.items[storageKey]
	if !ok {
		return attachment.ErrNotFound
	}
	item.Size, item.MimeType, item.SHA256 = size, mimeType, sha256
	return nil
}

func (repo *fakeAttRepo) SetCurrentVersion(ctx context.Context, sourceStorageKey, currentStorageKey, name, version string, size int64, mimeType, sha256 string) error {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	repo.setCurrentVoks++
	if repo.failSetCurrent {
		return errors.New("注入的指针切换失败")
	}
	item, ok := repo.items[sourceStorageKey]
	if !ok {
		return attachment.ErrNotFound
	}
	item.CurrentStorageKey = currentStorageKey
	item.CurrentName = name
	item.Version = version
	item.CurrentSize = size
	item.CurrentMimeType = mimeType
	item.CurrentSHA256 = sha256
	return nil
}

func (repo *fakeAttRepo) mustExist(sourceKey string) error {
	if _, ok := repo.items[sourceKey]; !ok {
		return fmt.Errorf("附件不存在或已删除: %s", sourceKey)
	}
	return nil
}

type fakeVersionRepo struct {
	Repository
	mu           sync.Mutex
	versions     map[string]Version
	byStorageKey map[string]string
	byAttVersion map[string]string
	order        []string
	attachments  *fakeAttRepo
	failOnCreate bool
}

func newFakeVersionRepo(attRepo *fakeAttRepo) *fakeVersionRepo {
	return &fakeVersionRepo{
		versions:     make(map[string]Version),
		byStorageKey: make(map[string]string),
		byAttVersion: make(map[string]string),
		attachments:  attRepo,
	}
}

func (repo *fakeVersionRepo) uniqueConflict() error {
	return errors.New("保存文件版本失败: duplicate key value violates unique constraint")
}

func (repo *fakeVersionRepo) Create(ctx context.Context, input CreateInput, storageKey string) (Version, error) {
	return repo.create(input, storageKey, false)
}

func (repo *fakeVersionRepo) CreateWithPromotion(ctx context.Context, input CreateInput, storageKey string) (Version, error) {
	return repo.create(input, storageKey, true)
}

func (repo *fakeVersionRepo) create(input CreateInput, storageKey string, promote bool) (Version, error) {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	if repo.failOnCreate {
		return Version{}, errors.New("注入的版本写入失败")
	}
	if _, exists := repo.byStorageKey[storageKey]; exists {
		return Version{}, repo.uniqueConflict()
	}
	attVersionKey := input.AttachmentID + "|" + input.Version
	if _, exists := repo.byAttVersion[attVersionKey]; exists {
		return Version{}, repo.uniqueConflict()
	}
	if repo.attachments != nil {
		if err := repo.attachments.mustExist(input.SourceStorageKey); err != nil {
			return Version{}, err
		}
	}
	id := fmt.Sprintf("ver-%03d", len(repo.order)+1)
	version := Version{
		ID: id, AttachmentID: input.AttachmentID, StorageKey: storageKey,
		SourceStorageKey: input.SourceStorageKey, Version: input.Version, VersionKind: input.VersionKind,
		Size: input.Size, MimeType: input.MimeType, SHA256: input.SHA256, CreatedBy: input.CreatedBy,
	}
	if input.ExpiresAt != nil {
		version.ExpiresAt = input.ExpiresAt
	}
	repo.versions[id] = version
	repo.byStorageKey[storageKey] = id
	repo.byAttVersion[attVersionKey] = id
	repo.order = append(repo.order, id)

	if promote && repo.attachments != nil && strings.TrimSpace(input.CurrentName) != "" {
		err := repo.attachments.SetCurrentVersion(context.Background(), input.SourceStorageKey, storageKey, input.CurrentName, input.Version, input.Size, input.MimeType, input.SHA256)
		if err != nil {
			// 模拟事务回滚：版本记录一并撤销
			delete(repo.versions, id)
			delete(repo.byStorageKey, storageKey)
			delete(repo.byAttVersion, attVersionKey)
			repo.order = repo.order[:len(repo.order)-1]
			return Version{}, err
		}
	}
	return version, nil
}

func (repo *fakeVersionRepo) GetByID(ctx context.Context, versionID string) (Version, error) {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	version, ok := repo.versions[versionID]
	if !ok {
		return Version{}, ErrNotFound
	}
	return version, nil
}

func (repo *fakeVersionRepo) GetByStorageKey(ctx context.Context, storageKey string) (Version, error) {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	id, ok := repo.byStorageKey[storageKey]
	if !ok {
		return Version{}, ErrNotFound
	}
	return repo.versions[id], nil
}

func (repo *fakeVersionRepo) ListByAttachment(ctx context.Context, attachmentID string) ([]Version, error) {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	list := make([]Version, 0)
	for _, id := range repo.order {
		if repo.versions[id].AttachmentID == attachmentID {
			list = append(list, repo.versions[id])
		}
	}
	return list, nil
}

func (repo *fakeVersionRepo) LatestByAttachment(ctx context.Context, attachmentID string) (Version, error) {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	for index := len(repo.order) - 1; index >= 0; index-- {
		version := repo.versions[repo.order[index]]
		if version.AttachmentID == attachmentID {
			return version, nil
		}
	}
	return Version{}, ErrNotFound
}

func (repo *fakeVersionRepo) PromoteInitial(ctx context.Context, versionID string) error {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	version, ok := repo.versions[versionID]
	if !ok {
		return ErrNotFound
	}
	version.VersionKind = "release"
	version.IsPinned = true
	version.IsCurrentRelease = true
	repo.versions[versionID] = version
	return nil
}

func (repo *fakeVersionRepo) Retain(ctx context.Context, versionID, userID string) (Version, error) {
	return repo.GetByID(ctx, versionID)
}

func (repo *fakeVersionRepo) Release(ctx context.Context, versionID, userID string) (Version, error) {
	return repo.GetByID(ctx, versionID)
}

func (repo *fakeVersionRepo) ListExpired(ctx context.Context, now time.Time) ([]Version, error) {
	return nil, nil
}

func (repo *fakeVersionRepo) MarkDeleted(ctx context.Context, versionID string) error {
	return nil
}

func newTestService(t *testing.T, attRepo *fakeAttRepo, versionRepo *fakeVersionRepo) (*Service, *storage.LocalStorage) {
	t.Helper()
	root := t.TempDir()
	objectStorage, err := storage.NewLocalStorage(root)
	if err != nil {
		t.Fatalf("NewLocalStorage() error = %v", err)
	}
	return NewService(versionRepo, attRepo, objectStorage), objectStorage
}

func mustPut(t *testing.T, objectStorage *storage.LocalStorage, key string, content []byte) {
	t.Helper()
	if _, err := objectStorage.Put(context.Background(), key, bytes.NewReader(content), "application/acad"); err != nil {
		t.Fatalf("Put(%s) error = %v", key, err)
	}
}

func mustRead(t *testing.T, objectStorage *storage.LocalStorage, key string) []byte {
	t.Helper()
	reader, _, err := objectStorage.Open(context.Background(), key)
	if err != nil {
		t.Fatalf("Open(%s) error = %v", key, err)
	}
	defer reader.Close()
	var buffer bytes.Buffer
	if _, err := buffer.ReadFrom(reader); err != nil {
		t.Fatalf("读取 %s 失败: %v", key, err)
	}
	return buffer.Bytes()
}

const testSourceKey = "drawings/JG-00/泵缸.exb"

func baseAttachment() attachment.Attachment {
	return attachment.Attachment{
		ID:         "att-001",
		StorageKey: testSourceKey,
		Name:       "泵缸.exb",
		MimeType:   "application/octet-stream",
		Size:       int64(len("raw-exb-content")),
		SHA256:     "",
		Version:    "v1.0",
	}
}

// ---------------------------------------------------------------------------
// 版本路径生成
// ---------------------------------------------------------------------------

func TestBuildVersionStorageKey(t *testing.T) {
	tests := []struct {
		name     string
		folder   string
		fileName string
		version  string
		want     string
	}{
		{
			name:     "普通名称",
			folder:   "drawings/JG-00",
			fileName: "泵缸.dwg",
			version:  "v1.0-w001",
			want:     "drawings/JG-00/history/泵缸/v1.0-w001/泵缸.dwg",
		},
		{
			name:     "v1.0 初始版本",
			folder:   "drawings/JG-00",
			fileName: "泵缸.dwg",
			version:  "v1.0",
			want:     "drawings/JG-00/history/泵缸/v1.0/泵缸.dwg",
		},
		{
			name:     "无扩展名默认 DWG",
			folder:   "drawings/JG-00",
			fileName: "泵缸",
			version:  "v1.0-w002",
			want:     "drawings/JG-00/history/泵缸/v1.0-w002/泵缸.dwg",
		},
		{
			name:     "特殊字符文件名 %x4070",
			folder:   "drawings/JG1285-250-180-3255%x4070",
			fileName: "JG1285-250-180-3255%x4070(液压缸).dwg",
			version:  "v1.0-w003",
			want:     "drawings/JG1285-250-180-3255%x4070/history/JG1285-250-180-3255%x4070(液压缸)/v1.0-w003/JG1285-250-180-3255%x4070(液压缸).dwg",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := buildVersionStorageKey(test.folder, test.fileName, test.version)
			if got != test.want {
				t.Fatalf("buildVersionStorageKey() = %q; want %q", got, test.want)
			}
			// 关键约束：每个版本必须位于自己独立的子文件夹内
			segments := strings.Split(got, "/")
			historyIndex := -1
			for index, segment := range segments {
				if segment == "history" {
					historyIndex = index
					break
				}
			}
			if historyIndex < 0 || historyIndex+2 >= len(segments) {
				t.Fatalf("路径缺少 history/文件名/版本号 结构: %q", got)
			}
			if segments[historyIndex+2] != test.version {
				t.Fatalf("版本文件夹 = %q; want %q（每个版本必须独立子文件夹，不堆放）", segments[historyIndex+2], test.version)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 初始版本登记
// ---------------------------------------------------------------------------

func TestEnsureInitialVersionRegistersAndSwitchesPointer(t *testing.T) {
	attRepo := newFakeAttRepo(baseAttachment())
	versionRepo := newFakeVersionRepo(attRepo)
	service, objectStorage := newTestService(t, attRepo, versionRepo)

	initialKey := "drawings/JG-00/history/泵缸/v1.0/泵缸.dwg"
	mustPut(t, objectStorage, initialKey, []byte("v1.0-dwg"))
	mustPut(t, objectStorage, testSourceKey, []byte("raw-exb-content"))

	if err := service.EnsureInitialVersion(context.Background(), testSourceKey, "user-1"); err != nil {
		t.Fatalf("EnsureInitialVersion() error = %v", err)
	}

	item, err := attRepo.Find(context.Background(), testSourceKey)
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	if item.CurrentStorageKey != initialKey {
		t.Fatalf("当前指针 = %q; want %q", item.CurrentStorageKey, initialKey)
	}
	if item.Version != "v1.0" {
		t.Fatalf("版本 = %q; want v1.0", item.Version)
	}
	list, _ := versionRepo.ListByAttachment(context.Background(), "att-001")
	if len(list) != 1 || list[0].Version != "v1.0" || list[0].VersionKind != "release" {
		t.Fatalf("版本记录 = %+v; want 单条 v1.0 release", list)
	}
	// 原始文件未被改动
	if got := mustRead(t, objectStorage, testSourceKey); string(got) != "raw-exb-content" {
		t.Fatalf("原始文件被覆盖: %q", got)
	}
}

func TestEnsureInitialVersionIdempotent(t *testing.T) {
	attRepo := newFakeAttRepo(baseAttachment())
	versionRepo := newFakeVersionRepo(attRepo)
	service, objectStorage := newTestService(t, attRepo, versionRepo)

	mustPut(t, objectStorage, "drawings/JG-00/history/泵缸/v1.0/泵缸.dwg", []byte("v1.0-dwg"))
	mustPut(t, objectStorage, testSourceKey, []byte("raw-exb-content"))

	if err := service.EnsureInitialVersion(context.Background(), testSourceKey, "user-1"); err != nil {
		t.Fatalf("第一次登记失败: %v", err)
	}
	if err := service.EnsureInitialVersion(context.Background(), testSourceKey, "user-1"); err != nil {
		t.Fatalf("重复登记失败: %v", err)
	}
	list, _ := versionRepo.ListByAttachment(context.Background(), "att-001")
	if len(list) != 1 {
		t.Fatalf("重复登记产生 %d 条版本记录; want 1", len(list))
	}
}

func TestEnsureInitialVersionFallsBackToCurrentKey(t *testing.T) {
	item := baseAttachment()
	item.CurrentStorageKey = "drawings/JG-00/泵缸.dwg"
	item.CurrentName = "泵缸.dwg"
	attRepo := newFakeAttRepo(item)
	versionRepo := newFakeVersionRepo(attRepo)
	service, objectStorage := newTestService(t, attRepo, versionRepo)

	// 存量数据：v1.0 版本目录文件不存在，只有旧格式的旁边 DWG
	mustPut(t, objectStorage, "drawings/JG-00/泵缸.dwg", []byte("legacy-dwg"))
	mustPut(t, objectStorage, testSourceKey, []byte("raw-exb-content"))

	if err := service.EnsureInitialVersion(context.Background(), testSourceKey, "user-1"); err != nil {
		t.Fatalf("EnsureInitialVersion() error = %v", err)
	}
	list, _ := versionRepo.ListByAttachment(context.Background(), "att-001")
	if len(list) != 1 || list[0].StorageKey != "drawings/JG-00/泵缸.dwg" {
		t.Fatalf("存量兼容登记 = %+v; want 引用 legacy dwg key", list)
	}
}

func TestEnsureInitialVersionFailsWithoutAnyContent(t *testing.T) {
	attRepo := newFakeAttRepo(baseAttachment())
	versionRepo := newFakeVersionRepo(attRepo)
	service, objectStorage := newTestService(t, attRepo, versionRepo)
	mustPut(t, objectStorage, testSourceKey, []byte("raw-exb-content"))

	if err := service.EnsureInitialVersion(context.Background(), testSourceKey, "user-1"); err == nil {
		t.Fatalf("无任何 DWG 内容时应返回错误")
	}
	list, _ := versionRepo.ListByAttachment(context.Background(), "att-001")
	if len(list) != 0 {
		t.Fatalf("失败时不应登记版本记录: %d", len(list))
	}
}

// ---------------------------------------------------------------------------
// 编辑结束捕获（版本闭环核心）
// ---------------------------------------------------------------------------

func captureFixture(t *testing.T) (*Service, *storage.LocalStorage, *fakeAttRepo, *fakeVersionRepo, string) {
	t.Helper()
	item := baseAttachment()
	item.CurrentStorageKey = "drawings/JG-00/history/泵缸/v1.0/泵缸.dwg"
	item.CurrentName = "泵缸.dwg"
	item.CurrentSHA256 = ""
	attRepo := newFakeAttRepo(item)
	versionRepo := newFakeVersionRepo(attRepo)
	service, objectStorage := newTestService(t, attRepo, versionRepo)

	mustPut(t, objectStorage, testSourceKey, []byte("raw-exb-content"))
	mustPut(t, objectStorage, item.CurrentStorageKey, []byte("v1.0-dwg"))
	// 登记基线版本 v1.0（模拟上传后状态）
	if err := service.EnsureInitialVersion(context.Background(), testSourceKey, "user-1"); err != nil {
		t.Fatalf("EnsureInitialVersion() error = %v", err)
	}
	workPath := filepath.Join(t.TempDir(), "泵缸.dwg")
	return service, objectStorage, attRepo, versionRepo, workPath
}

func TestCapturePathCreatesWorkingVersion(t *testing.T) {
	service, objectStorage, attRepo, _, workPath := captureFixture(t)
	writeFile(t, workPath, []byte("edited-content-1"))

	version, changed, err := service.CapturePath(context.Background(), testSourceKey, workPath, "user-1")
	if err != nil || !changed {
		t.Fatalf("CapturePath() = %v, changed=%v, err=%v", version, changed, err)
	}
	if version.Version != "v1.0-w001" {
		t.Fatalf("版本号 = %q; want v1.0-w001", version.Version)
	}
	wantKey := "drawings/JG-00/history/泵缸/v1.0-w001/泵缸.dwg"
	if version.StorageKey != wantKey {
		t.Fatalf("版本存储键 = %q; want %q", version.StorageKey, wantKey)
	}
	if got := mustRead(t, objectStorage, wantKey); string(got) != "edited-content-1" {
		t.Fatalf("版本对象内容 = %q", got)
	}

	item, _ := attRepo.Find(context.Background(), testSourceKey)
	if item.CurrentStorageKey != wantKey || item.Version != "v1.0-w001" || item.CurrentName != "泵缸.dwg" {
		t.Fatalf("附件指针 = %+v", item)
	}
	// v1.0 与原始文件不被覆盖
	if got := mustRead(t, objectStorage, "drawings/JG-00/history/泵缸/v1.0/泵缸.dwg"); string(got) != "v1.0-dwg" {
		t.Fatalf("v1.0 版本文件被覆盖: %q", got)
	}
	if got := mustRead(t, objectStorage, testSourceKey); string(got) != "raw-exb-content" {
		t.Fatalf("原始文件被覆盖: %q", got)
	}
}

func TestCapturePathUnchangedSkipsVersion(t *testing.T) {
	service, _, _, versionRepo, workPath := captureFixture(t)
	writeFile(t, workPath, []byte("v1.0-dwg"))

	version, changed, err := service.CapturePath(context.Background(), testSourceKey, workPath, "user-1")
	if err != nil {
		t.Fatalf("CapturePath() error = %v", err)
	}
	if changed {
		t.Fatalf("内容未变化不应生成新版本")
	}
	if version.Version != "v1.0" {
		t.Fatalf("返回版本 = %q; want v1.0", version.Version)
	}
	list, _ := versionRepo.ListByAttachment(context.Background(), "att-001")
	if len(list) != 1 {
		t.Fatalf("版本记录数 = %d; want 1", len(list))
	}
}

func TestCapturePathProgression(t *testing.T) {
	service, objectStorage, _, versionRepo, workPath := captureFixture(t)

	expectations := []struct {
		content string
		version string
	}{
		{"edit-1", "v1.0-w001"},
		{"edit-2", "v1.0-w002"},
		{"edit-3", "v1.0-w003"},
	}
	previous := ""
	for _, expect := range expectations {
		writeFile(t, workPath, []byte(expect.content))
		version, changed, err := service.CapturePath(context.Background(), testSourceKey, workPath, "user-1")
		if err != nil || !changed {
			t.Fatalf("CapturePath() = %v, changed=%v, err=%v", version, changed, err)
		}
		if version.Version != expect.version {
			t.Fatalf("版本号 = %q; want %q", version.Version, expect.version)
		}
		if previous != "" {
			// 上一版本文件内容必须原样保留
			oldKey := "drawings/JG-00/history/泵缸/" + previous + "/泵缸.dwg"
			content := previous
			if previous == "v1.0" {
				content = "v1.0-dwg"
			} else {
				content = expectations[0].content
				if previous == "v1.0-w002" {
					content = "edit-2"
				}
			}
			if got := mustRead(t, objectStorage, oldKey); string(got) != content {
				t.Fatalf("旧版本 %s 内容被覆盖: got %q want %q", oldKey, got, content)
			}
		}
		previous = version.Version
	}
	list, _ := versionRepo.ListByAttachment(context.Background(), "att-001")
	if len(list) != 4 {
		t.Fatalf("版本记录数 = %d; want 4 (v1.0 + 3 次编辑)", len(list))
	}
}

func TestCapturePathRollbackOnVersionWriteFailure(t *testing.T) {
	service, _, attRepo, versionRepo, workPath := captureFixture(t)
	versionRepo.failOnCreate = true
	writeFile(t, workPath, []byte("edited-content"))

	if _, _, err := service.CapturePath(context.Background(), testSourceKey, workPath, "user-1"); err == nil {
		t.Fatalf("版本写入失败时应返回错误")
	}
	// 旧指针保持不变
	item, _ := attRepo.Find(context.Background(), testSourceKey)
	if item.CurrentStorageKey != "drawings/JG-00/history/泵缸/v1.0/泵缸.dwg" || item.Version != "v1.0" {
		t.Fatalf("失败后附件指针 = %+v; want 保持 v1.0", item)
	}
	versionRepo.failOnCreate = false
	// 重试成功
	version, changed, err := service.CapturePath(context.Background(), testSourceKey, workPath, "user-1")
	if err != nil || !changed || version.Version != "v1.0-w001" {
		t.Fatalf("重试捕获 = %v, changed=%v, err=%v", version, changed, err)
	}
}

func TestCapturePathConcurrentNoDuplicateVersions(t *testing.T) {
	// 并发关闭场景：本人结束编辑与管理员强制关闭可能同时触发同一附件的捕获。
	// 工作文件写入（模拟 CAD 保存）发生在锁外，捕获被 lockAttachment 串行化。
	root, err := os.MkdirTemp("", "ver-concurrent-")
	if err != nil {
		t.Fatalf("MkdirTemp() error = %v", err)
	}
	// Windows 下实时杀毒/索引服务可能短暂占用新文件导致 RemoveAll 失败，容忍清理错误。
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	objectStorage, err := storage.NewLocalStorage(root)
	if err != nil {
		t.Fatalf("NewLocalStorage() error = %v", err)
	}
	item := baseAttachment()
	item.CurrentStorageKey = "drawings/JG-00/history/泵缸/v1.0/泵缸.dwg"
	item.CurrentName = "泵缸.dwg"
	attRepo := newFakeAttRepo(item)
	versionRepo := newFakeVersionRepo(attRepo)
	service := NewService(versionRepo, attRepo, objectStorage)

	mustPut(t, objectStorage, testSourceKey, []byte("raw-exb-content"))
	mustPut(t, objectStorage, item.CurrentStorageKey, []byte("v1.0-dwg"))
	if err := service.EnsureInitialVersion(context.Background(), testSourceKey, "user-1"); err != nil {
		t.Fatalf("EnsureInitialVersion() error = %v", err)
	}

	workDir := t.TempDir()
	const workers = 8
	var wg sync.WaitGroup
	versionResults := make(chan string, workers)
	for index := 0; index < workers; index++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			// 每个工作文件路径独立，模拟不同会话各自的 SMB 工作副本内容
			workPath := filepath.Join(workDir, fmt.Sprintf("work-%d.dwg", index))
			if writeErr := writeFileE(workPath, []byte(fmt.Sprintf("edited-content-%d", index))); writeErr != nil {
				versionResults <- "write-error"
				return
			}
			version, _, captureErr := service.CapturePath(context.Background(), testSourceKey, workPath, "user-1")
			if captureErr != nil {
				versionResults <- "capture-error"
				return
			}
			versionResults <- version.Version
		}(index)
	}
	wg.Wait()
	close(versionResults)

	seen := make(map[string]int)
	for version := range versionResults {
		seen[version]++
	}
	if seen["capture-error"] > 0 {
		t.Fatalf("并发捕获出现失败: %v", seen)
	}
	if seen["write-error"] > 0 {
		t.Fatalf("并发写工作文件失败: %v", seen)
	}
	for version, count := range seen {
		if version == "v1.0" {
			continue
		}
		if count > 1 {
			t.Fatalf("版本 %q 被重复登记 %d 次", version, count)
		}
	}
	// 版本记录与对象一一对应，无悬空记录
	list, _ := versionRepo.ListByAttachment(context.Background(), "att-001")
	if len(list) != workers+1 {
		t.Fatalf("版本记录数 = %d; want %d", len(list), workers+1)
	}
	for _, version := range list {
		if version.Version != "v1.0" {
			key := "drawings/JG-00/history/泵缸/" + version.Version + "/泵缸.dwg"
			if _, _, err := objectStorage.Open(context.Background(), key); err != nil {
				t.Fatalf("版本 %s 对象缺失: %v", version.Version, err)
			}
		}
	}
	// 当前指针指向最后生成的版本
	finalItem, _ := attRepo.Find(context.Background(), testSourceKey)
	if finalItem.Version == "v1.0" {
		t.Fatalf("并发捕获后指针仍停留在 v1.0")
	}
}

func TestRestoreCreatesNewVersionWithoutOverwrite(t *testing.T) {
	service, objectStorage, attRepo, _, workPath := captureFixture(t)

	// 先生成两个工作版本
	writeFile(t, workPath, []byte("edit-1"))
	if _, _, err := service.CapturePath(context.Background(), testSourceKey, workPath, "user-1"); err != nil {
		t.Fatalf("第一次捕获失败: %v", err)
	}
	writeFile(t, workPath, []byte("edit-2"))
	if _, _, err := service.CapturePath(context.Background(), testSourceKey, workPath, "user-1"); err != nil {
		t.Fatalf("第二次捕获失败: %v", err)
	}

	// 回退到 v1.0（当前是 v1.0-w002）
	list, _ := versionList(service)
	var baseline Version
	for _, version := range list {
		if version.Version == "v1.0" {
			baseline = version
		}
	}
	admin := authAdminUser()
	restored, err := service.Restore(context.Background(), admin, baseline.ID)
	if err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	if restored.Version != "v1.0-w003" {
		t.Fatalf("回退生成版本 = %q; want v1.0-w003（新版本，不篡改历史）", restored.Version)
	}
	wantKey := "drawings/JG-00/history/泵缸/v1.0-w003/泵缸.dwg"
	if got := mustRead(t, objectStorage, wantKey); string(got) != "v1.0-dwg" {
		t.Fatalf("回退版本内容 = %q; want v1.0 原内容", got)
	}
	item, _ := attRepo.Find(context.Background(), testSourceKey)
	if item.CurrentStorageKey != wantKey || item.Version != "v1.0-w003" {
		t.Fatalf("回退后指针 = %q version=%q", item.CurrentStorageKey, item.Version)
	}
	// v1.0-w001 / v1.0-w002 原样保留
	if got := mustRead(t, objectStorage, "drawings/JG-00/history/泵缸/v1.0-w001/泵缸.dwg"); string(got) != "edit-1" {
		t.Fatalf("v1.0-w001 被覆盖: %q", got)
	}
	if got := mustRead(t, objectStorage, "drawings/JG-00/history/泵缸/v1.0-w002/泵缸.dwg"); string(got) != "edit-2" {
		t.Fatalf("v1.0-w002 被覆盖: %q", got)
	}
}

func TestRestoreRejectsNonAdmin(t *testing.T) {
	service, _, _, _, _ := captureFixture(t)
	list, _ := versionList(service)
	if _, err := service.Restore(context.Background(), authNormalUser(), list[0].ID); err == nil {
		t.Fatalf("非管理员回退应被拒绝")
	}
}

// ---------------------------------------------------------------------------
// 版本查询与存储键定位
// ---------------------------------------------------------------------------

func TestListByStorageKeyLazilyRegistersBaseline(t *testing.T) {
	attRepo := newFakeAttRepo(baseAttachment())
	versionRepo := newFakeVersionRepo(attRepo)
	service, objectStorage := newTestService(t, attRepo, versionRepo)
	mustPut(t, objectStorage, "drawings/JG-00/history/泵缸/v1.0/泵缸.dwg", []byte("v1.0-dwg"))
	mustPut(t, objectStorage, testSourceKey, []byte("raw-exb-content"))

	list, err := service.ListByStorageKey(context.Background(), testSourceKey)
	if err != nil {
		t.Fatalf("ListByStorageKey() error = %v", err)
	}
	if len(list) != 1 || list[0].Version != "v1.0" {
		t.Fatalf("懒登记结果 = %+v", list)
	}
}

func TestOpenVersionContentByStorageKey(t *testing.T) {
	service, objectStorage, _, _, _ := captureFixture(t)
	writeFile(t, filepath.Join(t.TempDir(), "work.dwg"), []byte("edit-1"))
	workPath := filepath.Join(t.TempDir(), "work2.dwg")
	writeFile(t, workPath, []byte("edit-1"))
	if _, _, err := service.CapturePath(context.Background(), testSourceKey, workPath, "user-1"); err != nil {
		t.Fatalf("CapturePath() error = %v", err)
	}
	_ = objectStorage
	version, err := service.versions.GetByStorageKey(context.Background(), "drawings/JG-00/history/泵缸/v1.0-w001/泵缸.dwg")
	if err != nil {
		t.Fatalf("GetByStorageKey() error = %v", err)
	}
	if version.Version != "v1.0-w001" {
		t.Fatalf("按存储键定位版本 = %q", version.Version)
	}
}

// ---------------------------------------------------------------------------
// 辅助
// ---------------------------------------------------------------------------

func versionList(service *Service) ([]Version, error) {
	return service.List(context.Background(), "att-001")
}

func authAdminUser() auth.AuthUser {
	return auth.AuthUser{ID: "admin-1", Roles: []string{"admin"}}
}

func authNormalUser() auth.AuthUser {
	return auth.AuthUser{ID: "user-2", Roles: []string{"designer"}}
}

func writeFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := writeFileE(path, content); err != nil {
		t.Fatalf("写工作文件失败: %v", err)
	}
}

func writeFileE(path string, content []byte) error {
	return os.WriteFile(path, content, 0o644)
}

func hexSHA256(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// T2：工单工作版本必须以"上一次工单成果"为比较基准，而非正式当前版本。
// 复现并锁死 A→B→A：正式版 A、工单先存 B、再恢复到 A 时，必须生成反映 A 的新成果版本，
// 否则验收会误判"与正式版相同、无改动"从而错误发布旧的 B。
func TestCaptureWorkingComparesAgainstTicketBaseline(t *testing.T) {
	service, objectStorage, attRepo, _, workPath := captureFixture(t)
	const releaseV1Key = "drawings/JG-00/history/泵缸/v1.0/泵缸.dwg"

	// 第一次：基线为正式版 A（v1.0-dwg），编辑为 B。
	baselineA := hexSHA256([]byte("v1.0-dwg"))
	writeFile(t, workPath, []byte("edited-content-B"))
	ver1, changed1, err := service.CaptureWorking(context.Background(), testSourceKey, workPath, "user-1", baselineA)
	if err != nil || !changed1 {
		t.Fatalf("首次工单捕获 changed=%v err=%v", changed1, err)
	}
	if ver1.Version != "v1.0-w001" {
		t.Fatalf("首轮版本号 = %q; want v1.0-w001", ver1.Version)
	}
	if got := mustRead(t, objectStorage, ver1.StorageKey); string(got) != "edited-content-B" {
		t.Fatalf("首轮成果内容 = %q; want edited-content-B", got)
	}
	// 工作版本不得切换正式指针。
	if item, _ := attRepo.Find(context.Background(), testSourceKey); item.CurrentStorageKey != releaseV1Key {
		t.Fatalf("工单捕获后正式指针被改动: %q", item.CurrentStorageKey)
	}

	// 第二次：以工单上轮成果 B 为基线，把文件恢复到 A。
	baselineB := hexSHA256([]byte("edited-content-B"))
	writeFile(t, workPath, []byte("v1.0-dwg"))
	ver2, changed2, err := service.CaptureWorking(context.Background(), testSourceKey, workPath, "user-1", baselineB)
	if err != nil {
		t.Fatalf("恢复基线时捕获出错: %v", err)
	}
	if !changed2 {
		t.Fatal("A→B→A 必须相对工单基线 B 视为有效改动并生成反映 A 的新成果版本，实际判定为无改动")
	}
	if ver2.Version != "v1.0-w002" {
		t.Fatalf("次轮版本号 = %q; want v1.0-w002", ver2.Version)
	}
	if got := mustRead(t, objectStorage, ver2.StorageKey); string(got) != "v1.0-dwg" {
		t.Fatalf("次轮成果内容 = %q; 期望恢复到 A(v1.0-dwg)", got)
	}
	if item, _ := attRepo.Find(context.Background(), testSourceKey); item.CurrentStorageKey != releaseV1Key {
		t.Fatalf("次轮工单捕获后正式指针被改动: %q", item.CurrentStorageKey)
	}
}

// 工单成果与基线一致时，不重复生成工作版本。
func TestCaptureWorkingNoChangeAgainstTicketBaseline(t *testing.T) {
	service, _, _, versionRepo, workPath := captureFixture(t)
	baselineA := hexSHA256([]byte("v1.0-dwg"))
	writeFile(t, workPath, []byte("v1.0-dwg"))
	_, changed, err := service.CaptureWorking(context.Background(), testSourceKey, workPath, "user-1", baselineA)
	if err != nil {
		t.Fatalf("CaptureWorking error = %v", err)
	}
	if changed {
		t.Fatal("工作文件与工单基线一致，不应生成新版本")
	}
	list, _ := versionRepo.ListByAttachment(context.Background(), "att-001")
	if len(list) != 1 {
		t.Fatalf("版本记录数 = %d; want 1", len(list))
	}
}

func TestCaptureWorkingDoesNotReuseUnrelatedLatestVersion(t *testing.T) {
	service, objectStorage, _, _, workPath := captureFixture(t)
	baseline := hexSHA256([]byte("v1.0-dwg"))
	writeFile(t, workPath, []byte("unregistered-other-content"))
	other, _, err := service.CaptureWorking(context.Background(), testSourceKey, workPath, "user-1", baseline)
	if err != nil {
		t.Fatal(err)
	}
	// A failed registration may leave a newer version that is not the ticket baseline.
	writeFile(t, workPath, []byte("v1.0-dwg"))
	result, _, err := service.CaptureWorking(context.Background(), testSourceKey, workPath, "user-1", baseline)
	if err != nil {
		t.Fatal(err)
	}
	if result.ID == other.ID || string(mustRead(t, objectStorage, result.StorageKey)) != "v1.0-dwg" {
		t.Fatalf("reused unrelated content: %+v", result)
	}
}
