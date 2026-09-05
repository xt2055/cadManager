package versioning

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"cadguanliq/internal/attachment"
	"cadguanliq/internal/storage"
)

type Watcher struct {
	attachments attachment.Repository
	storage     storage.ObjectStorage
	versions    *Service
	localRoot   string
	interval    time.Duration
	mu          sync.Mutex
	lastSeen    map[string]fileState
}

type fileState struct {
	size    int64
	modTime time.Time
}

func NewWatcher(attachments attachment.Repository, objectStorage storage.ObjectStorage, versions *Service, localRoot string, interval time.Duration) *Watcher {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	return &Watcher{
		attachments: attachments,
		storage:     objectStorage,
		versions:    versions,
		localRoot:   localRoot,
		interval:    interval,
		lastSeen:    make(map[string]fileState),
	}
}

func (watcher *Watcher) Start(ctx context.Context) {
	if watcher == nil || watcher.versions == nil || watcher.attachments == nil || watcher.storage == nil || strings.TrimSpace(watcher.localRoot) == "" {
		return
	}
	go watcher.loop(ctx)
}

func (watcher *Watcher) loop(ctx context.Context) {
	ticker := time.NewTicker(watcher.interval)
	defer ticker.Stop()
	for {
		if err := watcher.scan(ctx); err != nil {
			log.Printf("CAD 工作文件扫描失败: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (watcher *Watcher) scan(ctx context.Context) error {
	items, err := watcher.attachments.ListAllCad(ctx)
	if err != nil {
		return err
	}
	for _, item := range items {
		if !isWatchedCAD(item.Name) {
			continue
		}
		// 如果是 EXB 文件，可能以转化后的 DWG 形式在 SMB 工作区编辑，同时检查原始 key 与 dwg key
		storageKeys := []string{item.StorageKey}
		ext := filepath.Ext(item.StorageKey)
		if ext == "" {
			ext = filepath.Ext(item.Name)
		}
		if strings.EqualFold(ext, ".exb") {
			dwgKey := strings.TrimSuffix(item.StorageKey, ext) + ".dwg"
			storageKeys = append(storageKeys, dwgKey)
		}

		for _, actualKey := range storageKeys {
			path, pathErr := watcher.localPath(actualKey)
			if pathErr != nil {
				continue
			}
			stat, statErr := os.Stat(path)
			if statErr != nil || !stat.Mode().IsRegular() || stat.Size() == 0 {
				continue
			}
			state := fileState{size: stat.Size(), modTime: stat.ModTime()}
			watcher.mu.Lock()
			previous, known := watcher.lastSeen[actualKey]
			watcher.lastSeen[actualKey] = state
			watcher.mu.Unlock()
			if !known || (previous.size == state.size && previous.modTime.Equal(state.modTime)) {
				continue
			}
			// 连续两次扫描内容和修改时间均稳定后再生成版本。
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(2 * time.Second):
			}
			latestStat, latestErr := os.Stat(path)
			if latestErr != nil || latestStat.Size() != state.size || !latestStat.ModTime().Equal(state.modTime) {
				continue
			}
			if _, changed, captureErr := watcher.versions.CapturePath(ctx, actualKey, path, item.UploadedByID); captureErr != nil {
				log.Printf("捕获 CAD 工作版本失败 storageKey=%s: %v", actualKey, captureErr)
			} else if changed {
				log.Printf("已捕获 CAD 工作版本 storageKey=%s", actualKey)
			}
		}
	}
	return nil
}

func (watcher *Watcher) localPath(storageKey string) (string, error) {
	key := strings.ReplaceAll(storageKey, "\\", "/")
	segments := strings.Split(key, "/")
	if key == "" {
		return "", fmt.Errorf("SMB 工作文件路径为空")
	}
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." || strings.ContainsAny(segment, ":\r\n") {
			return "", fmt.Errorf("SMB 工作文件路径无效")
		}
	}
	root, err := filepath.Abs(watcher.localRoot)
	if err != nil {
		return "", err
	}
	path, err := filepath.Abs(filepath.Join(append([]string{root}, segments...)...))
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("SMB 工作文件路径越界")
	}
	return path, nil
}

func isWatchedCAD(name string) bool {
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, ".dwl") || strings.HasSuffix(lower, ".dwl2") || strings.HasSuffix(lower, ".bak") || strings.HasSuffix(lower, ".tmp") || strings.HasPrefix(filepath.Base(lower), "~$") {
		return false
	}
	return strings.HasSuffix(lower, ".dwg") || strings.HasSuffix(lower, ".dxf") || strings.HasSuffix(lower, ".exb")
}
