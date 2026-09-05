package converter

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"cadguanliq/internal/attachment"
	"cadguanliq/internal/storage"
)

// ---------------------------------------------------------------------------
// 测试基建
// ---------------------------------------------------------------------------

type fakeCadRepo struct {
	attachment.Repository
	mu   sync.Mutex
	list []attachment.Attachment
}

func (repo *fakeCadRepo) ListAllCad(ctx context.Context) ([]attachment.Attachment, error) {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	list := make([]attachment.Attachment, len(repo.list))
	copy(list, repo.list)
	return list, nil
}

func newConverterTest(t *testing.T) (*Service, *storage.LocalStorage, *fakeCadRepo) {
	t.Helper()
	// Windows 下实时杀毒/索引服务可能短暂占用新建文件导致 t.TempDir 清理失败，
	// 这里用手动目录 + 容忍性清理，避免环境噪声造成误报。
	root, err := os.MkdirTemp("", "converter-test-")
	if err != nil {
		t.Fatalf("MkdirTemp() error = %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	objectStorage, err := storage.NewLocalStorage(root)
	if err != nil {
		t.Fatalf("NewLocalStorage() error = %v", err)
	}
	repo := &fakeCadRepo{}
	return NewService(repo, objectStorage), objectStorage, repo
}

// ---------------------------------------------------------------------------
// v1.0 版本路径
// ---------------------------------------------------------------------------

func TestVersionDwgKey(t *testing.T) {
	tests := []struct {
		name        string
		storageKey  string
		wantVersion string
	}{
		{name: "普通 EXB", storageKey: "drawings/JG-00/泵缸.exb", wantVersion: "drawings/JG-00/history/泵缸/v1.0/泵缸.dwg"},
		{name: "中文与括号", storageKey: "drawings/JG-00/篦冷机(主机).dwg", wantVersion: "drawings/JG-00/history/篦冷机(主机)/v1.0/篦冷机(主机).dwg"},
		{name: "百分号编码名", storageKey: "drawings/JG1285-3255%x4070/液压缸.dwg", wantVersion: "drawings/JG1285-3255%x4070/history/液压缸/v1.0/液压缸.dwg"},
		{name: "零件目录", storageKey: "drawings/JG-00/零件/铜套.dxf", wantVersion: "drawings/JG-00/零件/history/铜套/v1.0/铜套.dwg"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			att := attachment.Attachment{StorageKey: test.storageKey}
			if got := versionDwgKey(att); got != test.wantVersion {
				t.Fatalf("versionDwgKey() = %q; want %q", got, test.wantVersion)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 转换队列产物落位（DWG 复制与幂等）
// ---------------------------------------------------------------------------

func TestProcessOneCopiesDwgIntoVersionFolder(t *testing.T) {
	service, objectStorage, _ := newConverterTest(t)
	att := attachment.Attachment{
		ID:         "att-1",
		StorageKey: "drawings/JG-00/泵缸.dwg",
		Name:       "泵缸.dwg",
	}
	raw := []byte{0x41, 0x43, 0x10, 0x32, 0x00, 0xFF, 0x00, 0x01}
	if _, err := objectStorage.Put(context.Background(), att.StorageKey, bytes.NewReader(raw), "application/acad"); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	if err := service.processOne(context.Background(), att); err != nil {
		t.Fatalf("processOne() error = %v", err)
	}
	versionKey := versionDwgKey(att)
	if got, err := readAll(objectStorage, versionKey); err != nil {
		t.Fatalf("v1.0 版本对象不存在: %v", err)
	} else if !bytes.Equal(got, raw) {
		t.Fatalf("v1.0 版本对象内容与原始不一致")
	}

	// 原始 DWG 保持原样
	if got, err := readAll(objectStorage, att.StorageKey); err != nil || !bytes.Equal(got, raw) {
		t.Fatalf("原始文件被改动: err=%v", err)
	}

	// 幂等：重复执行不再写对象
	if err := service.processOne(context.Background(), att); err != nil {
		t.Fatalf("重复 processOne() error = %v", err)
	}
}

func TestProcessOneSkipsNonCAD(t *testing.T) {
	service, objectStorage, _ := newConverterTest(t)
	att := attachment.Attachment{ID: "att-2", StorageKey: "drawings/JG-00/说明.pdf", Name: "说明.pdf"}
	if err := service.processOne(context.Background(), att); err != nil {
		t.Fatalf("非 CAD 附件应跳过, err = %v", err)
	}
	if _, _, err := objectStorage.Open(context.Background(), versionDwgKey(att)); err == nil {
		t.Fatalf("非 CAD 附件不应产生版本对象")
	}
}

// 说明：EXB/DXF 分支需要真实 CAXA 转换，测试进程不得自动启动本机 CAXA
// （副作用：拉起真实 CAD 程序），此类分支通过 ensureCaxaRunning 门禁保护，
// 由人工在完整环境中回归验证。

// ---------------------------------------------------------------------------
// 启动扫描：缺少 v1.0 的附件进入后台队列
// ---------------------------------------------------------------------------

func TestScanMissingDwgEnqueuesMissingV1(t *testing.T) {
	service, objectStorage, repo := newConverterTest(t)

	missing := attachment.Attachment{ID: "att-m", StorageKey: "drawings/JG-00/缺版本.dwg", Name: "缺版本.dwg"}
	ready := attachment.Attachment{ID: "att-r", StorageKey: "drawings/JG-00/有版本.dwg", Name: "有版本.dwg"}
	nonCad := attachment.Attachment{ID: "att-n", StorageKey: "drawings/JG-00/说明.pdf", Name: "说明.pdf"}
	repo.list = []attachment.Attachment{missing, ready, nonCad}

	raw := []byte("dwg-bytes")
	if _, err := objectStorage.Put(context.Background(), ready.StorageKey, bytes.NewReader(raw), "application/acad"); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if _, err := objectStorage.Put(context.Background(), versionDwgKey(ready), bytes.NewReader(raw), "application/acad"); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if _, err := objectStorage.Put(context.Background(), missing.StorageKey, bytes.NewReader(raw), "application/acad"); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	service.scanMissingDwg(context.Background())

	// 缺失者已入队（inFlight 记录），就绪者与非 CAD 不入队
	if _, loaded := service.inFlight.Load(missing.StorageKey); !loaded {
		t.Fatalf("缺少 v1.0 的附件未被加入转换队列")
	}
	if _, loaded := service.inFlight.Load(ready.StorageKey); loaded {
		t.Fatalf("已有 v1.0 的附件不应重复入队")
	}
	if _, loaded := service.inFlight.Load(nonCad.StorageKey); loaded {
		t.Fatalf("非 CAD 附件不应入队")
	}

	// 队列任务执行后 DWG 复制进版本目录（DWG 分支无需 CAXA）
	job, _ := service.inFlight.Load(missing.StorageKey)
	if typed, ok := job.(*Job); ok {
		if err := service.processOne(context.Background(), typed.Attachment); err != nil {
			t.Fatalf("processOne() error = %v", err)
		}
		if _, _, err := objectStorage.Open(context.Background(), versionDwgKey(missing)); err != nil {
			t.Fatalf("扫描补建后版本对象缺失: %v", err)
		}
	}
}

// ---------------------------------------------------------------------------
// CAXA 路径解析（不依赖真实安装的回归保护）
// ---------------------------------------------------------------------------

func TestCandidatePaths(t *testing.T) {
	relative := "tools/exb2dxf/ok/dwg2dxf.exe"
	paths := candidatePaths(relative)
	if len(paths) == 0 {
		t.Fatalf("candidatePaths() 为空")
	}
	for _, path := range paths {
		if filepath.IsAbs(path) {
			continue
		}
		t.Fatalf("candidatePaths() 返回相对路径: %q", path)
	}
	absolute := candidatePaths(`D:\CAXA\CDRAFT_M.exe`)
	if len(absolute) != 1 || absolute[0] != `D:\CAXA\CDRAFT_M.exe` {
		t.Fatalf("绝对路径候选 = %v", absolute)
	}
}

func TestJobWaiterBroadcast(t *testing.T) {
	job := &Job{}
	waiter := make(chan error, 1)
	job.addWaiter(waiter)
	job.broadcast(nil)
	select {
	case err := <-waiter:
		if err != nil {
			t.Fatalf("broadcast(nil) 传出的错误 = %v", err)
		}
	default:
		t.Fatalf("等待者未收到广播")
	}
}

func TestWaitForCaxaJobFileQuarantinesStaleTask(t *testing.T) {
	jobFile := filepath.Join(t.TempDir(), "caxa_exb_jobs.txt")
	if err := os.WriteFile(jobFile, []byte("old-input|old-output\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	staleAt := time.Now().Add(-caxaJobStaleAfter - time.Second)
	if err := os.Chtimes(jobFile, staleAt, staleAt); err != nil {
		t.Fatalf("Chtimes() error = %v", err)
	}

	if err := waitForCaxaJobFile(jobFile, time.Second); err != nil {
		t.Fatalf("waitForCaxaJobFile() error = %v", err)
	}
	if _, err := os.Stat(jobFile); !os.IsNotExist(err) {
		t.Fatalf("遗留任务文件未被隔离，Stat err = %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(jobFile))
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "caxa_exb_jobs.txt.stale.") {
			return
		}
	}
	t.Fatal("未保留隔离后的遗留任务以供排查")
}

func TestPublishCaxaJobWritesCompleteTask(t *testing.T) {
	jobFile := filepath.Join(t.TempDir(), "caxa_exb_jobs.txt")
	if err := publishCaxaJob(jobFile, `C:\input file.exb`, `C:\output file.dwg`); err != nil {
		t.Fatalf("publishCaxaJob() error = %v", err)
	}
	content, err := os.ReadFile(jobFile)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got, want := string(content), "C:\\input file.exb|C:\\output file.dwg\n"; got != want {
		t.Fatalf("任务内容 = %q; want %q", got, want)
	}
}

// readAll 测试辅助：读取对象全部内容。
func readAll(objectStorage *storage.LocalStorage, key string) ([]byte, error) {
	reader, _, err := objectStorage.Open(context.Background(), key)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	var buffer bytes.Buffer
	if _, err := buffer.ReadFrom(reader); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
