package versioning

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"cadguanliq/internal/attachment"
	"cadguanliq/internal/auth"
	"cadguanliq/internal/storage"
)

type Service struct {
	versions    Repository
	attachments attachment.Repository
	storage     storage.ObjectStorage
	// captureLocks 按附件串行化版本捕获：本人结束编辑与管理员强制关闭可能并发触发
	// 同一附件的 CapturePath，无互斥时会生成相同版本号，失败方清理版本对象时
	// 会误删成功方刚写入的版本文件。单实例部署下进程内互斥即可消除该竞态。
	captureMu   sync.Mutex
	captureLock map[string]*sync.Mutex
}

func NewService(repository Repository, attachments attachment.Repository, objectStorage storage.ObjectStorage) *Service {
	return &Service{versions: repository, attachments: attachments, storage: objectStorage, captureLock: make(map[string]*sync.Mutex)}
}

// lockAttachment 取得该附件的捕获互斥锁，返回解锁函数。
func (service *Service) lockAttachment(sourceKey string) func() {
	service.captureMu.Lock()
	if service.captureLock == nil {
		service.captureLock = make(map[string]*sync.Mutex)
	}
	lock, ok := service.captureLock[sourceKey]
	if !ok {
		lock = &sync.Mutex{}
		service.captureLock[sourceKey] = lock
	}
	service.captureMu.Unlock()
	lock.Lock()
	return lock.Unlock
}

// EnsureInitialVersion 登记上传后生成的 v1.0 初始版本（转换/复制的 DWG，位于版本目录），
// 并将附件当前指针切换到该版本；已有任何版本记录时跳过。
// 调用方负责先通过转换队列把 v1.0 版本文件写入版本目录（EXB/DXF 转换、DWG 复制）。
func (service *Service) EnsureInitialVersion(ctx context.Context, sourceKey, userID string) error {
	unlock := service.lockAttachment(sourceKey)
	defer unlock()

	attachmentItem, err := service.attachments.Find(ctx, sourceKey)
	if err != nil {
		return err
	}
	latest, latestErr := service.versions.LatestByAttachment(ctx, attachmentItem.ID)
	if latestErr == nil && latest.Version != "" {
		return nil
	}
	if latestErr != nil && !errors.Is(latestErr, ErrNotFound) {
		return latestErr
	}

	// v1.0 内容 = 版本目录中的 DWG（EXB/DXF 的转换产物或 DWG 原件副本）。
	// 存量数据没有版本目录文件时兼容引用已有当前文件，避免重复转换。
	folder := filepath.Dir(attachmentItem.StorageKey)
	dwgName := strings.TrimSuffix(attachmentItem.Name, filepath.Ext(attachmentItem.Name)) + ".dwg"
	initialKey := buildVersionStorageKey(folder, dwgName, "v1.0")
	reader, info, err := service.storage.Open(ctx, initialKey)
	if err != nil {
		if fallback := attachmentItem.CurrentStorageKey; fallback != "" && fallback != attachmentItem.StorageKey {
			fallbackReader, fallbackInfo, fallbackErr := service.storage.Open(ctx, fallback)
			if fallbackErr != nil {
				return fmt.Errorf("打开初始版本内容失败: %w", err)
			}
			reader, info = fallbackReader, fallbackInfo
			initialKey = fallback
		} else {
			return fmt.Errorf("打开初始版本内容失败: %w", err)
		}
	}
	sourceHash, err := hashReader(reader)
	closeErr := reader.Close()
	if err != nil {
		return fmt.Errorf("计算初始版本哈希失败: %w", err)
	}
	if closeErr != nil {
		return fmt.Errorf("关闭初始版本内容失败: %w", closeErr)
	}

	created, err := service.versions.CreateWithPromotion(ctx, CreateInput{
		AttachmentID:     attachmentItem.ID,
		SourceStorageKey: attachmentItem.StorageKey,
		Version:          "v1.0",
		VersionKind:      "release",
		Size:             info.Size,
		MimeType:         info.MimeType,
		SHA256:           sourceHash,
		CreatedBy:        userID,
		CurrentName:      filepath.Base(initialKey),
	}, initialKey)
	if err != nil {
		return err
	}
	return service.versions.PromoteInitial(ctx, created.ID)
}

// OpenVersionContent 读取版本文件内容，用于下载。
func (service *Service) OpenVersionContent(ctx context.Context, versionID string) (io.ReadCloser, Version, error) {
	version, err := service.versions.GetByID(ctx, versionID)
	if err != nil {
		return nil, Version{}, err
	}
	reader, _, err := service.storage.Open(ctx, version.StorageKey)
	if err != nil {
		return nil, Version{}, fmt.Errorf("读取版本文件失败: %w", err)
	}
	return reader, version, nil
}

// OpenVersionSourceByStorageKey 按版本文件存储键读取版本内容，用于历史版本在线浏览。
// 存储键无版本记录时（旧数据/当前指针未登记）返回 ErrNotFound，由调用方回退当前文件。
func (service *Service) OpenVersionSourceByStorageKey(ctx context.Context, storageKey string) (io.ReadCloser, Version, error) {
	version, err := service.versions.GetByStorageKey(ctx, storageKey)
	if err != nil {
		return nil, Version{}, err
	}
	reader, _, err := service.storage.Open(ctx, version.StorageKey)
	if err != nil {
		return nil, Version{}, fmt.Errorf("读取版本文件失败: %w", err)
	}
	return reader, version, nil
}

// Restore 仅管理员可用：把目标版本内容复制为新工作版本并切换当前指针，
// 历史版本原样保留，当前文件不再被覆盖写入。
func (service *Service) Restore(ctx context.Context, user auth.AuthUser, versionID string) (Version, error) {
	if !isAdminUser(user) {
		return Version{}, errors.New("只有管理员可以回退版本")
	}
	version, err := service.versions.GetByID(ctx, versionID)
	if err != nil {
		return Version{}, err
	}
	if version.DeletedAt != nil {
		return Version{}, ErrNotFound
	}
	unlock := service.lockAttachment(version.SourceStorageKey)
	defer unlock()
	attachmentItem, err := service.attachments.Find(ctx, version.SourceStorageKey)
	if err != nil {
		return Version{}, err
	}

	latest, _ := service.versions.LatestByAttachment(ctx, attachmentItem.ID)
	newVersionNo, err := nextWorkingVersion(latest, attachmentItem.Version)
	if err != nil {
		return Version{}, err
	}

	// 回退版本写入独立版本目录，扩展名跟随目标版本文件，保证格式一致。
	folder := filepath.Dir(attachmentItem.StorageKey)
	baseNoExt := strings.TrimSuffix(filepath.Base(version.StorageKey), filepath.Ext(version.StorageKey))
	activeName := baseNoExt + filepath.Ext(version.StorageKey)
	historyKey := buildVersionStorageKey(folder, activeName, newVersionNo)

	reader, _, err := service.storage.Open(ctx, version.StorageKey)
	if err != nil {
		return Version{}, fmt.Errorf("读取回退目标版本失败: %w", err)
	}
	if _, err := service.storage.Put(ctx, historyKey, reader, version.MimeType); err != nil {
		reader.Close()
		return Version{}, fmt.Errorf("保存回退版本失败: %w", err)
	}
	reader.Close()

	created, err := service.versions.CreateWithPromotion(ctx, CreateInput{
		AttachmentID:     attachmentItem.ID,
		SourceStorageKey: attachmentItem.StorageKey,
		Version:          newVersionNo,
		VersionKind:      "working",
		Size:             version.Size,
		MimeType:         version.MimeType,
		SHA256:           version.SHA256,
		CreatedBy:        user.ID,
		CurrentName:      activeName,
	}, historyKey)
	if err != nil {
		_ = service.storage.Delete(ctx, historyKey)
		return Version{}, err
	}
	return created, nil
}

func isAdminUser(user auth.AuthUser) bool {
	for _, role := range user.Roles {
		if role == "admin" {
			return true
		}
	}
	return false
}

func (service *Service) CapturePath(ctx context.Context, sourceKey, sourcePath, userID string) (Version, bool, error) {
	// 同一附件的捕获全程互斥：版本号计算、对象写入、事务登记必须串行，
	// 否则并发方会生成相同版本号并在失败清理时误删对方的版本对象。
	unlock := service.lockAttachment(sourceKey)
	defer unlock()

	attachmentItem, err := service.attachments.Find(ctx, sourceKey)
	if err != nil {
		return Version{}, false, err
	}
	reader, err := os.Open(sourcePath)
	if err != nil {
		return Version{}, false, fmt.Errorf("打开 SMB 工作文件失败: %w", err)
	}
	sourceHash, err := hashReader(reader)
	closeErr := reader.Close()
	if err != nil {
		return Version{}, false, fmt.Errorf("计算 SMB 工作文件哈希失败: %w", err)
	}
	if closeErr != nil {
		return Version{}, false, fmt.Errorf("关闭 SMB 工作文件失败: %w", closeErr)
	}

	// 比较当前活跃哈希，未改动则跳过
	currentHash := attachmentItem.CurrentSHA256
	if currentHash == "" {
		currentHash = attachmentItem.SHA256
	}
	if currentHash == sourceHash {
		latest, latestErr := service.versions.LatestByAttachment(ctx, attachmentItem.ID)
		if latestErr == nil {
			return latest, false, nil
		}
	}

	latest, _ := service.versions.LatestByAttachment(ctx, attachmentItem.ID)
	version, err := nextWorkingVersion(latest, attachmentItem.Version)
	if err != nil {
		return Version{}, false, err
	}

	// 语义化版本路径：每个版本一个独立子目录 -> 目录/history/文件名/版本号/文件名.dwg
	// 工作文件必为 DWG（EXB 已在打开编辑时转换），版本文件名与当前文件保持一致。
	folder := filepath.Dir(attachmentItem.StorageKey)
	activeName := attachmentItem.CurrentName
	if activeName == "" {
		activeName = attachmentItem.Name
	}
	if strings.EqualFold(filepath.Ext(activeName), ".exb") {
		activeName = strings.TrimSuffix(activeName, filepath.Ext(activeName)) + ".dwg"
	}
	historyKey := buildVersionStorageKey(folder, activeName, version)

	reader, err = os.Open(sourcePath)
	if err != nil {
		return Version{}, false, fmt.Errorf("重新打开 SMB 工作文件失败: %w", err)
	}
	historyObject, err := service.storage.Put(ctx, historyKey, reader, "application/acad")
	closeErr = reader.Close()
	if closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		_ = service.storage.Delete(ctx, historyKey)
		return Version{}, false, fmt.Errorf("保存 SMB 历史归档版本失败: %w", err)
	}

	// 事务内登记版本并切换当前指针；不覆盖任何已有文件。
	// 失败时删除刚写入的版本对象，旧当前版本保持不变，编辑会话保留等待重试。
	expiresAt := time.Now().UTC().Add(90 * 24 * time.Hour)
	created, err := service.versions.CreateWithPromotion(ctx, CreateInput{
		AttachmentID:     attachmentItem.ID,
		SourceStorageKey: sourceKey,
		Version:          version,
		VersionKind:      "working",
		Size:             historyObject.Size,
		MimeType:         historyObject.MimeType,
		SHA256:           historyObject.SHA256,
		CreatedBy:        userID,
		ExpiresAt:        &expiresAt,
		CurrentName:      activeName,
	}, historyKey)
	if err != nil {
		_ = service.storage.Delete(ctx, historyKey)
		return Version{}, false, err
	}
	return created, true, nil
}

// buildVersionStorageKey 生成版本文件的独立目录路径：
// {folder}/history/{文件名去后缀}/{版本号}/{文件名去后缀}{ext}
// 每个版本一个子文件夹，文件名保持干净（不带版本号、不带时间戳），
// 同版本重试不会因时间戳漂移产生重复版本，路径本身即含版本语义。
func buildVersionStorageKey(folder, fileName, version string) string {
	baseNoExt := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	ext := filepath.Ext(fileName)
	if ext == "" {
		ext = ".dwg"
	}
	return filepath.ToSlash(filepath.Join(folder, "history", baseNoExt, version, baseNoExt+ext))
}

func (service *Service) List(ctx context.Context, attachmentID string) ([]Version, error) {
	return service.versions.ListByAttachment(ctx, attachmentID)
}

// ListByStorageKey 按附件存储键（原始 key 或当前 key 均可）列出版本。
// 存量附件可能没有初始版本记录，查询时懒登记一次 v1.0 基线。
func (service *Service) ListByStorageKey(ctx context.Context, storageKey string) ([]Version, error) {
	attachmentItem, err := service.attachments.Find(ctx, storageKey)
	if err != nil {
		return nil, err
	}
	_, latestErr := service.versions.LatestByAttachment(ctx, attachmentItem.ID)
	if errors.Is(latestErr, ErrNotFound) {
		if ensureErr := service.EnsureInitialVersion(ctx, attachmentItem.StorageKey, attachmentItem.UploadedByID); ensureErr != nil {
			log.Printf("[版本] 懒登记初始版本失败 storageKey=%s: %v", attachmentItem.StorageKey, ensureErr)
		}
	} else if latestErr != nil {
		return nil, latestErr
	}
	return service.versions.ListByAttachment(ctx, attachmentItem.ID)
}

func (service *Service) Retain(ctx context.Context, user auth.AuthUser, versionID string) (Version, error) {
	if !canManageVersion(user) {
		return Version{}, errors.New("当前账号没有保留文件版本的权限")
	}
	return service.versions.Retain(ctx, versionID, user.ID)
}

func (service *Service) Release(ctx context.Context, user auth.AuthUser, versionID string) (Version, error) {
	if !canManageVersion(user) {
		return Version{}, errors.New("当前账号没有发布文件版本的权限")
	}
	return service.versions.Release(ctx, versionID, user.ID)
}

func (service *Service) Cleanup(ctx context.Context) error {
	items, err := service.versions.ListExpired(ctx, time.Now().UTC())
	if err != nil {
		return err
	}
	for _, item := range items {
		if err := service.storage.Delete(ctx, item.StorageKey); err != nil {
			continue
		}
		if err := service.versions.MarkDeleted(ctx, item.ID); err != nil {
			return fmt.Errorf("标记过期版本 %s 失败: %w", item.ID, err)
		}
	}
	return nil
}

func (service *Service) StartCleanup(ctx context.Context) {
	if service == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			if err := service.Cleanup(ctx); err != nil {
				log.Printf("清理过期 CAD 工作版本失败: %v", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func nextWorkingVersion(latest Version, base string) (string, error) {
	base = strings.TrimSpace(base)
	if latest.Version == "" || latest.VersionKind == "release" {
		return base + "-w001", nil
	}
	separator := strings.LastIndex(latest.Version, "-w")
	if separator < 0 || len(latest.Version)-separator-2 != 3 {
		return "", errors.New("文件版本号无效")
	}
	number, err := strconv.Atoi(latest.Version[separator+2:])
	if err != nil || number >= 999 {
		return "", errors.New("文件版本号无效")
	}
	return latest.Version[:separator+2] + fmt.Sprintf("%03d", number+1), nil
}

func hashReader(reader io.Reader) (string, error) {
	hash := sha256.New()
	if _, err := io.Copy(hash, reader); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func canManageVersion(user auth.AuthUser) bool {
	for _, role := range user.Roles {
		if role == "admin" || role == "designer" {
			return true
		}
	}
	return false
}
