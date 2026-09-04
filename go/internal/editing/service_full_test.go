package editing

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"cadguanliq/internal/attachment"
	"cadguanliq/internal/auth"
	"cadguanliq/internal/config"
	"cadguanliq/internal/storage"
	"cadguanliq/internal/versioning"
)

// ---------------------------------------------------------------------------
// 测试基建
// ---------------------------------------------------------------------------

// fakeAttRepo 内存附件仓库（模拟 current 指针）。
type fakeAttRepo struct {
	attachment.Repository
	mu    sync.Mutex
	items map[string]*attachment.Attachment
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

func (repo *fakeAttRepo) SetCurrentVersion(ctx context.Context, sourceStorageKey, currentStorageKey, name, version string, size int64, mimeType, sha256 string) error {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	item, ok := repo.items[sourceStorageKey]
	if !ok {
		return attachment.ErrNotFound
	}
	item.CurrentStorageKey = currentStorageKey
	item.CurrentName = name
	item.Version = version
	item.CurrentSHA256 = sha256
	return nil
}

// fakeSessionRepo 内存编辑会话仓库（模拟唯一活动会话约束与原子关闭）。
type fakeSessionRepo struct {
	Repository
	mu       sync.Mutex
	sessions map[string]*Session
	tickets  map[string]*fakeTicket
}

type fakeTicket struct {
	sessionID string
	userID    string
	expiresAt time.Time
	used      bool
}

func newFakeSessionRepo() *fakeSessionRepo {
	return &fakeSessionRepo{sessions: make(map[string]*Session), tickets: make(map[string]*fakeTicket)}
}

func (repo *fakeSessionRepo) CreateSession(ctx context.Context, session Session) error {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	for _, existing := range repo.sessions {
		if existing.AttachmentID == session.AttachmentID && existing.Status == "active" {
			return ErrFileBusy
		}
	}
	stored := session
	repo.sessions[session.ID] = &stored
	return nil
}

func (repo *fakeSessionRepo) FindActiveByStorageKey(ctx context.Context, storageKey string, now time.Time) (Session, error) {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	var found Session
	has := false
	for _, session := range repo.sessions {
		if session.StorageKey == storageKey && session.Status == "active" {
			if has && found.StartedAt.After(session.StartedAt) {
				continue
			}
			found = *session
			has = true
		}
	}
	if !has {
		return Session{}, ErrSessionNotFound
	}
	return found, nil
}

func (repo *fakeSessionRepo) FindActiveByID(ctx context.Context, sessionID string) (Session, error) {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	session, ok := repo.sessions[sessionID]
	if !ok || session.Status != "active" {
		return Session{}, ErrSessionNotFound
	}
	return *session, nil
}

func (repo *fakeSessionRepo) UpdateWorkStorageKey(ctx context.Context, userID, sessionID, workStorageKey string) error {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	session, ok := repo.sessions[sessionID]
	if !ok || session.Status != "active" || session.UserID != userID {
		return ErrSessionNotFound
	}
	session.WorkStorageKey = workStorageKey
	return nil
}

func (repo *fakeSessionRepo) CreateTicket(ctx context.Context, token string, sessionID, userID string, expiresAt time.Time) error {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	repo.tickets[token] = &fakeTicket{sessionID: sessionID, userID: userID, expiresAt: expiresAt}
	return nil
}

func (repo *fakeSessionRepo) ConsumeTicket(ctx context.Context, token, userID string, now time.Time) (Session, error) {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	ticket, ok := repo.tickets[token]
	if !ok || ticket.used || ticket.userID != userID || now.After(ticket.expiresAt) {
		return Session{}, ErrInvalidTicket
	}
	ticket.used = true
	session, ok := repo.sessions[ticket.sessionID]
	if !ok || session.Status != "active" || session.UserID != userID {
		return Session{}, ErrSessionNotFound
	}
	return *session, nil
}

func (repo *fakeSessionRepo) ListActiveSessions(ctx context.Context, now time.Time, drawingNo string) ([]ActiveSessionInfo, error) {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	list := make([]ActiveSessionInfo, 0)
	for _, session := range repo.sessions {
		if session.Status != "active" {
			continue
		}
		list = append(list, ActiveSessionInfo{
			ID: session.ID, AttachmentID: session.AttachmentID, StorageKey: session.StorageKey,
			WorkStorageKey: session.WorkStorageKey, UserID: session.UserID,
		})
	}
	return list, nil
}

func (repo *fakeSessionRepo) ExpireStale(ctx context.Context, now time.Time) error {
	return nil
}

func (repo *fakeSessionRepo) CleanupTickets(ctx context.Context, now time.Time) error {
	return nil
}

func (repo *fakeSessionRepo) Heartbeat(ctx context.Context, userID, sessionID string, now time.Time) error {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	session, ok := repo.sessions[sessionID]
	if !ok || session.Status != "active" || session.UserID != userID {
		return ErrSessionNotFound
	}
	session.LastSeenAt = now
	return nil
}

func (repo *fakeSessionRepo) Close(ctx context.Context, userID, sessionID string, isAdmin bool, now time.Time) error {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	session, ok := repo.sessions[sessionID]
	if !ok || session.Status != "active" {
		return ErrSessionNotFound
	}
	if !isAdmin && session.UserID != userID {
		return ErrSessionNotFound
	}
	session.Status = "closed"
	closedAt := now
	session.ClosedAt = &closedAt
	return nil
}

// fakeVersioning 版本捕获替身（模拟哈希比对与版本递进，可注入失败）。
type fakeVersioning struct {
	mu           sync.Mutex
	failCapture  bool
	currentHash  string
	nextIndex    int
	captured     [][]byte
	ensureCalls  int
	latestID     string
	latestVer    string
	latestKey    string
	captureDelay time.Duration
}

func (fake *fakeVersioning) CapturePath(ctx context.Context, sourceKey, sourcePath, userID string) (versioning.Version, bool, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.captureDelay > 0 {
		time.Sleep(fake.captureDelay)
	}
	if fake.failCapture {
		return versioning.Version{}, false, errors.New("注入的捕获失败")
	}
	content, err := os.ReadFile(sourcePath)
	if err != nil {
		return versioning.Version{}, false, err
	}
	digest := sha256.Sum256(content)
	hash := hex.EncodeToString(digest[:])
	if fake.currentHash == hash {
		return versioning.Version{ID: fake.latestID, Version: fake.latestVer, StorageKey: fake.latestKey}, false, nil
	}
	fake.nextIndex++
	version := versioning.Version{
		ID:         fmt.Sprintf("ver-%03d", fake.nextIndex),
		Version:    fmt.Sprintf("v1.0-w%03d", fake.nextIndex),
		StorageKey: fmt.Sprintf("drawings/JG-00/history/泵缸/v1.0-w%03d/泵缸.dwg", fake.nextIndex),
		SHA256:     hash,
	}
	fake.captured = append(fake.captured, content)
	fake.currentHash = hash
	fake.latestID = version.ID
	fake.latestVer = version.Version
	fake.latestKey = version.StorageKey
	return version, true, nil
}

func (fake *fakeVersioning) EnsureInitialVersion(ctx context.Context, sourceKey, userID string) error {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.ensureCalls++
	return nil
}

type testEnv struct {
	service     *Service
	storage     *storage.LocalStorage
	sessions    *fakeSessionRepo
	attachments *fakeAttRepo
	versions    *fakeVersioning
	localRoot   string
	storageRoot string
}

// newTestEnv 构造完整编辑服务：LocalStorage 模拟对象存储，本地临时目录模拟 SMB 共享根。
func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	storageRoot := t.TempDir()
	localRoot := t.TempDir()
	objectStorage, err := storage.NewLocalStorage(storageRoot)
	if err != nil {
		t.Fatalf("NewLocalStorage() error = %v", err)
	}
	sessionRepo := newFakeSessionRepo()
	attRepo := newFakeAttRepo()
	versions := &fakeVersioning{}
	smbConfig := config.SMBConfig{
		Enabled:   true,
		Host:      "TESTHOST",
		Share:     "cadshare",
		LocalRoot: localRoot,
		Username:  "cadshare",
		Password:  "secret",
	}
	service := NewService(sessionRepo, attRepo, objectStorage, nil, smbConfig)
	service.SetVersioning(versions)
	return &testEnv{
		service:     service,
		storage:     objectStorage,
		sessions:    sessionRepo,
		attachments: attRepo,
		versions:    versions,
		localRoot:   localRoot,
		storageRoot: storageRoot,
	}
}

var testCurrentKey = "drawings/JG-00/history/泵缸/v1.0/泵缸.dwg"

func fixtureAttachment() attachment.Attachment {
	return attachment.Attachment{
		ID:                "att-001",
		StorageKey:        "drawings/JG-00/泵缸.exb",
		CurrentStorageKey: testCurrentKey,
		Name:              "泵缸.exb",
		CurrentName:       "泵缸.dwg",
		DrawingNo:         "JG-00",
		MimeType:          "application/octet-stream",
		Version:           "v1.0",
	}
}

var adminUser = auth.AuthUser{ID: "admin-1", Account: "admin", DisplayName: "管理员", Roles: []string{"admin"}}
var designerUser = auth.AuthUser{ID: "user-1", Account: "zhang", DisplayName: "张三", Roles: []string{"designer"}}

// openFixture 准备「对象存储已有 v1.0 版本文件」的附件并打开编辑会话。
func openFixture(t *testing.T, env *testEnv, user auth.AuthUser) OpenResult {
	t.Helper()
	item := fixtureAttachment()
	env.attachments.Items(map[string]attachment.Attachment{item.StorageKey: item})
	if _, err := env.storage.Put(context.Background(), item.StorageKey, bytes.NewReader([]byte("raw-exb")), "application/octet-stream"); err != nil {
		t.Fatalf("Put 原始文件失败: %v", err)
	}
	if _, err := env.storage.Put(context.Background(), testCurrentKey, bytes.NewReader([]byte("v1.0-dwg")), "application/acad"); err != nil {
		t.Fatalf("Put 当前版本失败: %v", err)
	}
	env.versions.seedCurrent("v1.0-dwg")
	result, err := env.service.Open(context.Background(), user, item.StorageKey)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	return result
}

// seedCurrent 预置当前版本基线哈希：Open 时同步到 SMB 的工作文件内容与该哈希一致，
// 模拟“上传转换后的 v1.0 已登记”状态，保证无改动关闭不会误生成版本。
func (fake *fakeVersioning) seedCurrent(content string) {
	digest := sha256.Sum256([]byte(content))
	fake.currentHash = hex.EncodeToString(digest[:])
}

// Items 供测试快速注入附件（fakeAttRepo 的测试辅助）。
func (repo *fakeAttRepo) Items(items map[string]attachment.Attachment) {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	for key := range items {
		item := items[key]
		repo.items[key] = &item
	}
}

// ---------------------------------------------------------------------------
// SMB 工作路径安全
// ---------------------------------------------------------------------------

func TestLocalPathValidation(t *testing.T) {
	env := newTestEnv(t)
	valid, err := env.service.localPath("drawings/JG-00/泵缸.dwg")
	if err != nil {
		t.Fatalf("localPath(合法) error = %v", err)
	}
	if !strings.HasPrefix(valid, env.localRoot) {
		t.Fatalf("localPath() = %q; 应位于工作根目录内", valid)
	}

	invalid := []string{"", "../escape.dwg", "drawings/../../escape.dwg", "drawings/JG-00/x:y.dwg", "drawings//x.dwg", "drawings/./x.dwg"}
	for _, key := range invalid {
		if _, err := env.service.localPath(key); err == nil {
			t.Fatalf("localPath(%q) 应拒绝越界/非法路径", key)
		}
	}
}

// ---------------------------------------------------------------------------
// SMB 工作文件同步
// ---------------------------------------------------------------------------

func TestSyncToWorkDirectoryCreatesAndReplaces(t *testing.T) {
	env := newTestEnv(t)
	key := "drawings/JG-00/泵缸.dwg"
	if _, err := env.storage.Put(context.Background(), key, bytes.NewReader([]byte("content-1")), "application/acad"); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if err := env.service.syncToWorkDirectory(context.Background(), key, key); err != nil {
		t.Fatalf("syncToWorkDirectory() error = %v", err)
	}
	path, _ := env.service.localPath(key)
	if got, err := os.ReadFile(path); err != nil || string(got) != "content-1" {
		t.Fatalf("工作文件内容 = %q, err=%v; want content-1", got, err)
	}

	if _, err := env.storage.Put(context.Background(), key, bytes.NewReader([]byte("content-2-longer")), "application/acad"); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if err := env.service.syncToWorkDirectory(context.Background(), key, key); err != nil {
		t.Fatalf("再次 syncToWorkDirectory() error = %v", err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "content-2-longer" {
		t.Fatalf("替换后工作文件内容 = %q, err=%v; want content-2-longer", got, err)
	}

	// 无临时文件残留
	entries, _ := os.ReadDir(filepath.Dir(path))
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".cadguanliq.tmp") {
			t.Fatalf("发现临时文件残留: %s", entry.Name())
		}
	}
}

func TestSyncToWorkDirectoryConcurrent(t *testing.T) {
	// 多个用户/流程并发同步同一工作文件：最终文件必须是某一次同步的完整内容，
	// 且不允许临时文件残留（临时文件名唯一化后各写各的，互不破坏）。
	root := t.TempDir()
	objectStorage, err := storage.NewLocalStorage(root)
	if err != nil {
		t.Fatalf("NewLocalStorage() error = %v", err)
	}
	localRoot := t.TempDir()
	smbConfig := config.SMBConfig{Enabled: true, Host: "H", Share: "S", LocalRoot: localRoot}
	service := NewService(newFakeSessionRepo(), newFakeAttRepo(), objectStorage, nil, smbConfig)

	key := "drawings/JG-00/并发.dwg"
	const workers = 12
	contents := make(map[string]bool)
	for index := 0; index < workers; index++ {
		content := fmt.Sprintf("complete-content-%03d-%s", index, strings.Repeat("x", index*97))
		if _, err := objectStorage.Put(context.Background(), key, bytes.NewReader([]byte(content)), "application/acad"); err != nil {
			t.Fatalf("Put() error = %v", err)
		}
		contents[content] = true
	}

	var wg sync.WaitGroup
	errCh := make(chan error, workers)
	for index := 0; index < workers; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := service.syncToWorkDirectory(context.Background(), key, key); err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("并发同步失败: %v", err)
	}

	path, _ := service.localPath(key)
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取并发同步结果失败: %v", err)
	}
	if !contents[string(got)] {
		t.Fatalf("并发同步后文件内容损坏（不是任何一次完整写入）: %q", got)
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".cadguanliq.tmp") {
			t.Fatalf("并发同步后存在临时文件残留: %s", entry.Name())
		}
	}
}

func TestWaitForFileStable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stable.dwg")
	if err := os.WriteFile(path, []byte("stable"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := waitForFileStable(path, 5*time.Second); err != nil {
		t.Fatalf("waitForFileStable(稳定文件) = %v; want nil", err)
	}

	// 持续写入的文件应超时
	changing := filepath.Join(t.TempDir(), "changing.dwg")
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for index := 0; ; index++ {
			_ = os.WriteFile(changing, []byte(fmt.Sprintf("changing-%d", index)), 0o644)
			select {
			case <-stop:
				return
			case <-time.After(50 * time.Millisecond):
			}
		}
	}()
	if err := waitForFileStable(changing, 1200*time.Millisecond); err == nil {
		t.Fatalf("waitForFileStable(持续变化) 应超时")
	}
	close(stop)
	<-done
}

// ---------------------------------------------------------------------------
// 打开编辑会话
// ---------------------------------------------------------------------------

func TestOpenRequiresSMBConfiguration(t *testing.T) {
	env := newTestEnv(t)
	env.service.cfg = config.SMBConfig{}
	item := fixtureAttachment()
	env.attachments.Items(map[string]attachment.Attachment{item.StorageKey: item})
	if _, err := env.service.Open(context.Background(), adminUser, item.StorageKey); !errors.Is(err, ErrSMBNotConfigured) {
		t.Fatalf("Open() error = %v; want ErrSMBNotConfigured", err)
	}
}

func TestOpenCreatesSessionWithWorkKey(t *testing.T) {
	env := newTestEnv(t)
	result := openFixture(t, env, designerUser)

	session, err := env.sessions.FindActiveByID(context.Background(), result.SessionID)
	if err != nil {
		t.Fatalf("会话未创建: %v", err)
	}
	if !strings.HasPrefix(session.WorkStorageKey, "work/"+result.SessionID+"/") || !strings.HasSuffix(session.WorkStorageKey, ".dwg") {
		t.Fatalf("会话工作文件键应使用带 .dwg 扩展名的工作副本，got %q", session.WorkStorageKey)
	}
	if !strings.Contains(session.UNCPath, `\\TESTHOST\cadshare`) {
		t.Fatalf("UNC 路径 = %q", session.UNCPath)
	}
	if result.UNCPath != session.UNCPath {
		t.Fatalf("OpenResult.UNCPath = %q; want %q", result.UNCPath, session.UNCPath)
	}
}

func TestOpenSecondUserBusy(t *testing.T) {
	env := newTestEnv(t)
	openFixture(t, env, designerUser)
	if _, err := env.service.Open(context.Background(), adminUser, "drawings/JG-00/泵缸.exb"); !errors.Is(err, ErrFileBusy) {
		t.Fatalf("第二用户打开 error = %v; want ErrFileBusy", err)
	}
}

func TestOpenReclaimsOwnSession(t *testing.T) {
	env := newTestEnv(t)
	first := openFixture(t, env, designerUser)
	second, err := env.service.Open(context.Background(), designerUser, "drawings/JG-00/泵缸.exb")
	if err != nil {
		t.Fatalf("本人重新打开 error = %v", err)
	}
	if first.SessionID != second.SessionID {
		t.Fatalf("重新打开生成新会话 %q; want 认领原会话 %q", second.SessionID, first.SessionID)
	}
}

func TestResolveWorkKeyUsesExistingCurrentDwgBlob(t *testing.T) {
	env := newTestEnv(t)
	item := attachment.Attachment{
		StorageKey:        "blobs/original-exb-hash",
		CurrentStorageKey: "blobs/current-dwg-hash",
		Name:              "工程图文档2.exb",
		CurrentName:       "工程图文档2.dwg",
	}
	if _, err := env.storage.Put(context.Background(), item.CurrentStorageKey, bytes.NewReader([]byte("valid-dwg")), "application/acad"); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	got, err := env.service.resolveWorkKey(context.Background(), designerUser, item)
	if err != nil {
		t.Fatalf("resolveWorkKey() error = %v", err)
	}
	if got != item.CurrentStorageKey {
		t.Fatalf("工作源键 = %q; want 已存在的当前 DWG %q", got, item.CurrentStorageKey)
	}
}

func TestOpenFallsBackToRawKeyWhenCurrentMissing(t *testing.T) {
	env := newTestEnv(t)
	item := fixtureAttachment()
	env.attachments.Items(map[string]attachment.Attachment{item.StorageKey: item})
	// 原始文件存在，当前版本对象被物理清理
	if _, err := env.storage.Put(context.Background(), item.StorageKey, bytes.NewReader([]byte("raw-exb")), "application/octet-stream"); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	// EXB + converter==nil：resolveWorkKey 走当前指针分支，缺失时回退原始 key
	result, err := env.service.Open(context.Background(), designerUser, item.StorageKey)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	session, _ := env.sessions.FindActiveByID(context.Background(), result.SessionID)
	wantWorkKey, err := editWorkStorageKey(result.SessionID, item.StorageKey, item)
	if err != nil {
		t.Fatalf("editWorkStorageKey() error = %v", err)
	}
	if session.WorkStorageKey != wantWorkKey {
		t.Fatalf("回退工作文件键 = %q; want %q", session.WorkStorageKey, wantWorkKey)
	}
	if filepath.Ext(session.WorkStorageKey) != ".exb" {
		t.Fatalf("回退原始 EXB 时工作副本必须保留 .exb 扩展名，got %q", session.WorkStorageKey)
	}
}

func TestOpenRejectsNonCADAttachment(t *testing.T) {
	env := newTestEnv(t)
	item := fixtureAttachment()
	item.Name = "说明.pdf"
	env.attachments.Items(map[string]attachment.Attachment{item.StorageKey: item})
	if _, err := env.service.Open(context.Background(), adminUser, item.StorageKey); err == nil || !strings.Contains(err.Error(), "CAD") {
		t.Fatalf("非 CAD 附件应被拒绝, err = %v", err)
	}
}

// ---------------------------------------------------------------------------
// 结束编辑（版本闭环核心）
// ---------------------------------------------------------------------------

func TestCloseWithChangeCreatesVersion(t *testing.T) {
	env := newTestEnv(t)
	result := openFixture(t, env, designerUser)

	// 模拟 CAD 保存：修改 SMB 工作文件
	workKey, err := editWorkStorageKey(result.SessionID, testCurrentKey, fixtureAttachment())
	if err != nil {
		t.Fatalf("editWorkStorageKey() error = %v", err)
	}
	workPath, err := env.service.localPath(workKey)
	if err != nil {
		t.Fatalf("localPath() error = %v", err)
	}
	if err := os.WriteFile(workPath, []byte("edited-cad-content"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	closeResult, err := env.service.Close(context.Background(), designerUser, result.SessionID)
	if err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if !closeResult.Changed {
		t.Fatalf("有改动应生成新版本")
	}
	if closeResult.Version != "v1.0-w001" || closeResult.SessionID != result.SessionID {
		t.Fatalf("CloseResult = %+v", closeResult)
	}
	if closeResult.CurrentStorageKey == "" || closeResult.CurrentName == "" {
		t.Fatalf("CloseResult 缺少当前版本信息: %+v", closeResult)
	}
	if _, err := env.sessions.FindActiveByID(context.Background(), result.SessionID); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("结束编辑后会话应关闭, err = %v", err)
	}
	// 工作文件被清理
	if _, statErr := os.Stat(workPath); !os.IsNotExist(statErr) {
		t.Fatalf("工作文件应被清理, stat = %v", statErr)
	}
	if len(env.versions.captured) != 1 {
		t.Fatalf("捕获次数 = %d; want 1", len(env.versions.captured))
	}
}

func TestCloseWithoutChangeSkipsVersion(t *testing.T) {
	env := newTestEnv(t)
	result := openFixture(t, env, designerUser)

	closeResult, err := env.service.Close(context.Background(), designerUser, result.SessionID)
	if err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if closeResult.Changed {
		t.Fatalf("无改动不应生成新版本: %+v", closeResult)
	}
	if len(env.versions.captured) != 0 {
		t.Fatalf("无改动不应捕获内容: %d", len(env.versions.captured))
	}
	if _, err := env.sessions.FindActiveByID(context.Background(), result.SessionID); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("会话应关闭, err = %v", err)
	}
}

func TestCloseCaptureFailureKeepsSession(t *testing.T) {
	env := newTestEnv(t)
	result := openFixture(t, env, designerUser)

	workKey, err := editWorkStorageKey(result.SessionID, testCurrentKey, fixtureAttachment())
	if err != nil {
		t.Fatalf("editWorkStorageKey() error = %v", err)
	}
	workPath, _ := env.service.localPath(workKey)
	if err := os.WriteFile(workPath, []byte("unsaved-work"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	env.versions.failCapture = true

	if _, err := env.service.Close(context.Background(), designerUser, result.SessionID); err == nil {
		t.Fatalf("捕获失败应返回错误")
	}
	// 会话保留，工作内容不丢失
	session, findErr := env.sessions.FindActiveByID(context.Background(), result.SessionID)
	if findErr != nil {
		t.Fatalf("捕获失败后会话必须保留, err = %v", findErr)
	}
	wantWorkKey, err := editWorkStorageKey(result.SessionID, testCurrentKey, fixtureAttachment())
	if err != nil {
		t.Fatalf("editWorkStorageKey() error = %v", err)
	}
	if session.WorkStorageKey != wantWorkKey {
		t.Fatalf("会话状态被破坏: %+v", session)
	}

	// 修复后重试成功
	env.versions.failCapture = false
	closeResult, err := env.service.Close(context.Background(), designerUser, result.SessionID)
	if err != nil {
		t.Fatalf("重试 Close() error = %v", err)
	}
	if !closeResult.Changed {
		t.Fatalf("重试应生成新版本")
	}
}

func TestCloseConcurrentOnlyOneWins(t *testing.T) {
	// 并发结束编辑（本人 + 管理员同时点击）：只有一个 Close 成功，
	// 版本最多生成一个，不会出现重复版本或 panic。
	root := t.TempDir()
	objectStorage, err := storage.NewLocalStorage(root)
	if err != nil {
		t.Fatalf("NewLocalStorage() error = %v", err)
	}
	localRoot := t.TempDir()
	sessionRepo := newFakeSessionRepo()
	item := fixtureAttachment()
	attRepo := newFakeAttRepo(item)
	versions := &fakeVersioning{captureDelay: 30 * time.Millisecond}
	versions.seedCurrent("v1.0-dwg")
	smbConfig := config.SMBConfig{Enabled: true, Host: "H", Share: "S", LocalRoot: localRoot}
	service := NewService(sessionRepo, attRepo, objectStorage, nil, smbConfig)
	service.SetVersioning(versions)

	if _, err := objectStorage.Put(context.Background(), item.StorageKey, bytes.NewReader([]byte("raw-exb")), "application/octet-stream"); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if _, err := objectStorage.Put(context.Background(), testCurrentKey, bytes.NewReader([]byte("v1.0-dwg")), "application/acad"); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	opened, err := service.Open(context.Background(), designerUser, item.StorageKey)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	workKey, err := editWorkStorageKey(opened.SessionID, testCurrentKey, item)
	if err != nil {
		t.Fatalf("editWorkStorageKey() error = %v", err)
	}
	workPath, _ := service.localPath(workKey)
	if err := os.WriteFile(workPath, []byte("concurrent-edit"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	const closers = 5
	var wg sync.WaitGroup
	outcomes := make(chan error, closers)
	for index := 0; index < closers; index++ {
		wg.Add(1)
		user := designerUser
		if index%2 == 1 {
			user = adminUser
		}
		go func(user auth.AuthUser) {
			defer wg.Done()
			_, err := service.Close(context.Background(), user, opened.SessionID)
			outcomes <- err
		}(user)
	}
	wg.Wait()
	close(outcomes)

	successes := 0
	for err := range outcomes {
		if err == nil {
			successes++
			continue
		}
		if !errors.Is(err, ErrSessionNotFound) {
			t.Fatalf("并发关闭出现意外错误: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("并发关闭成功次数 = %d; want 1", successes)
	}
	if len(versions.captured) > 1 {
		t.Fatalf("并发关闭生成 %d 个版本; want ≤ 1", len(versions.captured))
	}
}

func TestCloseAdminForcesThroughCaptureFailure(t *testing.T) {
	env := newTestEnv(t)
	result := openFixture(t, env, designerUser)
	env.versions.failCapture = true

	// 管理员强制关闭他人会话：捕获失败不阻塞锁释放（尽力归档），会话必须关闭。
	if _, err := env.service.Close(context.Background(), adminUser, result.SessionID); err != nil {
		t.Fatalf("管理员强制关闭 error = %v", err)
	}
	if _, err := env.sessions.FindActiveByID(context.Background(), result.SessionID); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("强制关闭后会话应关闭, err = %v", err)
	}
}

func TestCloseRejectsStranger(t *testing.T) {
	env := newTestEnv(t)
	result := openFixture(t, env, designerUser)
	stranger := auth.AuthUser{ID: "user-9", Account: "li", DisplayName: "李四", Roles: []string{"designer"}}
	if _, err := env.service.Close(context.Background(), stranger, result.SessionID); err == nil {
		t.Fatalf("无关用户关闭他人会话应被拒绝")
	}
}

// ---------------------------------------------------------------------------
// 票据交换与只读打开
// ---------------------------------------------------------------------------

func TestExchangeUsesWorkStorageKey(t *testing.T) {
	env := newTestEnv(t)
	result := openFixture(t, env, designerUser)
	// 从 OpenURL 中提取票据
	const marker = "ticket="
	index := strings.Index(result.OpenURL, marker)
	if index < 0 {
		t.Fatalf("OpenURL 缺少票据: %q", result.OpenURL)
	}
	ticket := result.OpenURL[index+len(marker):]
	exchanged, err := env.service.Exchange(context.Background(), designerUser, ticket)
	if err != nil {
		t.Fatalf("Exchange() error = %v", err)
	}
	if exchanged.UNCPath != result.UNCPath {
		t.Fatalf("Exchange UNC = %q; want %q", exchanged.UNCPath, result.UNCPath)
	}
	if !strings.HasSuffix(exchanged.FileName, ".dwg") {
		t.Fatalf("工作文件名 = %q; want DWG", exchanged.FileName)
	}
}

func TestExchangeInvalidTicket(t *testing.T) {
	env := newTestEnv(t)
	if _, err := env.service.Exchange(context.Background(), designerUser, "bad-ticket"); !errors.Is(err, ErrInvalidTicket) {
		t.Fatalf("Exchange(无效票据) error = %v; want ErrInvalidTicket", err)
	}
}

func TestReadOnlyOpenReturnsDownloadPath(t *testing.T) {
	env := newTestEnv(t)
	item := fixtureAttachment()
	env.attachments.Items(map[string]attachment.Attachment{item.StorageKey: item})
	if _, err := env.storage.Put(context.Background(), item.StorageKey, bytes.NewReader([]byte("raw-exb")), "application/octet-stream"); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if _, err := env.storage.Put(context.Background(), testCurrentKey, bytes.NewReader([]byte("v1.0-dwg")), "application/acad"); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	result, err := env.service.ReadOnlyOpen(context.Background(), designerUser, item.StorageKey)
	if err != nil {
		t.Fatalf("ReadOnlyOpen() error = %v", err)
	}
	if !strings.HasPrefix(result.DownloadPath, "/attachments/") {
		t.Fatalf("DownloadPath = %q", result.DownloadPath)
	}
	if result.FileName != "泵缸.dwg" {
		t.Fatalf("FileName = %q; want 泵缸.dwg", result.FileName)
	}
	decoded, err := urlPathUnescapeAll(result.DownloadPath)
	if err != nil {
		t.Fatalf("路径解码失败: %v", err)
	}
	if !strings.HasSuffix(decoded, testCurrentKey) {
		t.Fatalf("DownloadPath 解码后 = %q; want 以 %q 结尾", decoded, testCurrentKey)
	}
}

func urlPathUnescapeAll(path string) (string, error) {
	segments := strings.Split(strings.TrimPrefix(path, "/attachments/"), "/")
	for index, segment := range segments {
		decoded, err := url.PathUnescape(segment)
		if err != nil {
			return "", err
		}
		segments[index] = decoded
	}
	return strings.Join(segments, "/"), nil
}
