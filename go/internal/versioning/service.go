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
	"time"

	"cadguanliq/internal/attachment"
	"cadguanliq/internal/auth"
	"cadguanliq/internal/storage"
)

type Service struct {
	versions    Repository
	attachments attachment.Repository
	storage     storage.ObjectStorage
}

func NewService(repository Repository, attachments attachment.Repository, objectStorage storage.ObjectStorage) *Service {
	return &Service{versions: repository, attachments: attachments, storage: objectStorage}
}

// EnsureInitialVersion 在附件上传后登记 v1.0 初始版本（引用当前内容，不复制文件），
// 作为可回退的基线；已有任何版本记录时跳过。
func (service *Service) EnsureInitialVersion(ctx context.Context, sourceKey, userID string) error {
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

	activeKey := attachmentItem.CurrentStorageKey
	if activeKey == "" {
		activeKey = attachmentItem.StorageKey
	}
	reader, info, err := service.storage.Open(ctx, activeKey)
	if err != nil {
		return fmt.Errorf("打开初始版本内容失败: %w", err)
	}
	sourceHash, err := hashReader(reader)
	closeErr := reader.Close()
	if err != nil {
		return fmt.Errorf("计算初始版本哈希失败: %w", err)
	}
	if closeErr != nil {
		return fmt.Errorf("关闭初始版本内容失败: %w", closeErr)
	}

	baseVersion := strings.TrimSpace(attachmentItem.Version)
	if baseVersion == "" {
		baseVersion = "v1.0"
	}
	created, err := service.versions.Create(ctx, CreateInput{
		AttachmentID:     attachmentItem.ID,
		SourceStorageKey: attachmentItem.StorageKey,
		Version:          baseVersion,
		VersionKind:      "release",
		Size:             info.Size,
		MimeType:         info.MimeType,
		SHA256:           sourceHash,
		CreatedBy:        userID,
	}, activeKey)
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

// Restore 仅管理员可用：以目标版本内容生成一个新工作版本并设为当前内容，
// 历史版本原样保留，可再次回退。
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
	attachmentItem, err := service.attachments.Find(ctx, version.SourceStorageKey)
	if err != nil {
		return Version{}, err
	}

	latest, _ := service.versions.LatestByAttachment(ctx, attachmentItem.ID)
	newVersionNo, err := nextWorkingVersion(latest, attachmentItem.Version)
	if err != nil {
		return Version{}, err
	}

	folder := filepath.Dir(attachmentItem.StorageKey)
	baseNoExt := strings.TrimSuffix(filepath.Base(version.StorageKey), filepath.Ext(version.StorageKey))
	historyKey := buildHistoryStorageKey(folder, baseNoExt+filepath.Ext(version.StorageKey), newVersionNo)

	reader, _, err := service.storage.Open(ctx, version.StorageKey)
	if err != nil {
		return Version{}, fmt.Errorf("读取回退目标版本失败: %w", err)
	}
	if _, err := service.storage.Put(ctx, historyKey, reader, version.MimeType); err != nil {
		reader.Close()
		return Version{}, fmt.Errorf("保存回退版本失败: %w", err)
	}
	reader.Close()

	// 回写当前内容；存储键扩展名跟随版本文件，保证格式一致。
	currentKey := attachmentItem.CurrentStorageKey
	if currentKey == "" {
		currentKey = attachmentItem.StorageKey
	}
	currentKey = strings.TrimSuffix(currentKey, filepath.Ext(currentKey)) + filepath.Ext(version.StorageKey)
	if err := service.restoreCurrent(ctx, version.StorageKey, currentKey, version.MimeType); err != nil {
		_ = service.storage.Delete(ctx, historyKey)
		return Version{}, err
	}

	newName := baseNoExt + filepath.Ext(version.StorageKey)
	if err := service.attachments.SetCurrentContent(ctx, attachmentItem.StorageKey, currentKey, newName, version.Size, version.MimeType, version.SHA256); err != nil {
		_ = service.attachments.UpdateContent(ctx, attachmentItem.StorageKey, version.Size, version.MimeType, version.SHA256)
	}

	created, err := service.versions.Create(ctx, CreateInput{
		AttachmentID:     attachmentItem.ID,
		SourceStorageKey: attachmentItem.StorageKey,
		Version:          newVersionNo,
		VersionKind:      "working",
		Size:             version.Size,
		MimeType:         version.MimeType,
		SHA256:           version.SHA256,
		CreatedBy:        user.ID,
	}, historyKey)
	if err != nil {
		_ = service.storage.Delete(ctx, historyKey)
		return Version{}, err
	}
	return created, nil
}

func (service *Service) restoreCurrent(ctx context.Context, versionKey, currentKey, mimeType string) error {
	reader, _, err := service.storage.Open(ctx, versionKey)
	if err != nil {
		return fmt.Errorf("读取回退目标版本失败: %w", err)
	}
	defer reader.Close()
	if _, err := service.storage.Put(ctx, currentKey, reader, mimeType); err != nil {
		return fmt.Errorf("回写当前 CAD 文件失败: %w", err)
	}
	return nil
}

func isAdminUser(user auth.AuthUser) bool {
	for _, role := range user.Roles {
		if role == "admin" {
			return true
		}
	}
	return false
}

func (service *Service) Capture(ctx context.Context, sourceKey, userID string) (Version, bool, error) {	attachmentItem, err := service.attachments.Find(ctx, sourceKey)
	if err != nil {
		return Version{}, false, err
	}
	reader, _, err := service.storage.Open(ctx, sourceKey)
	if err != nil {
		return Version{}, false, err
	}
	sourceHash, err := hashReader(reader)
	closeErr := reader.Close()
	if err != nil {
		return Version{}, false, fmt.Errorf("计算 CAD 文件哈希失败: %w", err)
	}
	if closeErr != nil {
		return Version{}, false, fmt.Errorf("关闭 CAD 文件失败: %w", closeErr)
	}
	latest, latestErr := service.versions.LatestByAttachment(ctx, attachmentItem.ID)
	if latestErr == nil && latest.SHA256 == sourceHash {
		return latest, false, nil
	}
	version, err := nextWorkingVersion(latest, attachmentItem.Version)
	if err != nil {
		return Version{}, false, err
	}
	reader, info, err := service.storage.Open(ctx, sourceKey)
	if err != nil {
		return Version{}, false, err
	}
	key := filepath.ToSlash(filepath.Join("versions", attachmentItem.ID, "working", version+"-"+filepath.Base(attachmentItem.Name)))
	object, err := service.storage.Put(ctx, key, reader, info.MimeType)
	closeErr = reader.Close()
	if closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		return Version{}, false, err
	}
	expiresAt := time.Now().UTC().Add(90 * 24 * time.Hour)
	created, err := service.versions.Create(ctx, CreateInput{
		AttachmentID: attachmentItem.ID, SourceStorageKey: sourceKey, Version: version,
		VersionKind: "working", Size: object.Size, MimeType: object.MimeType, SHA256: object.SHA256,
		CreatedBy: userID, ExpiresAt: &expiresAt,
	}, key)
	if err != nil {
		_ = service.storage.Delete(ctx, key)
		return Version{}, false, err
	}
	return created, true, nil
}

func (service *Service) syncPathToStorage(ctx context.Context, sourcePath, storageKey, mimeType string) (storage.ObjectInfo, error) {
	reader, err := os.Open(sourcePath)
	if err != nil {
		return storage.ObjectInfo{}, fmt.Errorf("打开待回写的 SMB 文件失败: %w", err)
	}
	defer reader.Close()
	object, err := service.storage.Put(ctx, storageKey, reader, mimeType)
	if err != nil {
		return storage.ObjectInfo{}, err
	}
	return object, nil
}

func (service *Service) CapturePath(ctx context.Context, sourceKey, sourcePath, userID string) (Version, bool, error) {
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

	// 方案 A：语义化历史版本路径 -> drawings/项目目录/history/图号/图号_版本_时间戳.dwg
	folder := filepath.Dir(attachmentItem.StorageKey)
	activeName := attachmentItem.CurrentName
	if activeName == "" {
		activeName = attachmentItem.Name
	}
	historyKey := buildHistoryStorageKey(folder, activeName, version)

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

	// 先回写当前工作文件并更新附件元数据，全部成功后才登记版本记录；
	// 否则回写失败触发重试时会因时间戳不同产生重复版本。
	currentKey := attachmentItem.CurrentStorageKey
	if currentKey == "" {
		currentKey = strings.TrimSuffix(attachmentItem.StorageKey, filepath.Ext(attachmentItem.StorageKey)) + ".dwg"
	}
	currentObject, err := service.syncPathToStorage(ctx, sourcePath, currentKey, "application/acad")
	if err != nil {
		_ = service.storage.Delete(ctx, historyKey)
		return Version{}, false, fmt.Errorf("回写当前 CAD 文件失败: %w", err)
	}

	dwgName := strings.TrimSuffix(attachmentItem.Name, filepath.Ext(attachmentItem.Name)) + ".dwg"
	if err := service.attachments.SetCurrentContent(ctx, attachmentItem.StorageKey, currentKey, dwgName, currentObject.Size, currentObject.MimeType, currentObject.SHA256); err != nil {
		// 尝试更新通用内容
		_ = service.attachments.UpdateContent(ctx, attachmentItem.StorageKey, currentObject.Size, currentObject.MimeType, currentObject.SHA256)
	}

	expiresAt := time.Now().UTC().Add(90 * 24 * time.Hour)
	created, err := service.versions.Create(ctx, CreateInput{
		AttachmentID:     attachmentItem.ID,
		SourceStorageKey: sourceKey,
		Version:          version,
		VersionKind:      "working",
		Size:             historyObject.Size,
		MimeType:         historyObject.MimeType,
		SHA256:           historyObject.SHA256,
		CreatedBy:        userID,
		ExpiresAt:        &expiresAt,
	}, historyKey)
	if err != nil {
		_ = service.storage.Delete(ctx, historyKey)
		return Version{}, false, err
	}
	return created, true, nil
}

func buildHistoryStorageKey(folder, fileName, version string) string {
	baseNoExt := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	ext := filepath.Ext(fileName)
	if ext == "" {
		ext = ".dwg"
	}
	timeTag := time.Now().Format("20060102_150405")
	histName := fmt.Sprintf("%s_%s_%s%s", baseNoExt, version, timeTag, ext)
	return filepath.ToSlash(filepath.Join(folder, "history", baseNoExt, histName))
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
		if ensureErr := service.EnsureInitialVersion(ctx, attachmentItem.StorageKey, attachmentItem.UploadedBy); ensureErr != nil {
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
