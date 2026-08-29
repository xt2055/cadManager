package editing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cadguanliq/internal/attachment"
	"cadguanliq/internal/auth"
	"cadguanliq/internal/config"
	"cadguanliq/internal/converter"
	"cadguanliq/internal/storage"
	"cadguanliq/internal/versioning"
)

var (
	ErrSMBNotConfigured = errors.New("SMB 文件共享未配置")
	ErrSessionNotFound  = errors.New("编辑会话不存在或已过期")
	ErrFileBusy         = errors.New("该文件正在被其他用户编辑")
	ErrInvalidTicket    = errors.New("打开票据无效或已使用")
)

type Service struct {
	attachments attachment.Repository
	storage     storage.ObjectStorage
	converter   *converter.Service
	versions    interface {
		CapturePath(ctx context.Context, sourceKey, sourcePath, userID string) (versioning.Version, bool, error)
	}
	repository Repository
	cfg        config.SMBConfig
}

func NewService(sessionRepository Repository, attachmentRepository attachment.Repository, objectStorage storage.ObjectStorage, convService *converter.Service, smbConfig config.SMBConfig) *Service {
	return &Service{
		attachments: attachmentRepository,
		storage:     objectStorage,
		converter:   convService,
		repository:  sessionRepository,
		cfg:         smbConfig,
	}
}

func (service *Service) SetVersioning(versions interface {
	CapturePath(ctx context.Context, sourceKey, sourcePath, userID string) (versioning.Version, bool, error)
}) {
	service.versions = versions
}

func (service *Service) Open(ctx context.Context, user auth.AuthUser, storageKey string) (OpenResult, error) {
	// 用户名和密码为空时由 Windows 当前登录凭据访问本机 SMB 共享，
	// 避免将 Windows 密码写入项目配置。
	if !service.cfg.Enabled || strings.TrimSpace(service.cfg.Host) == "" || strings.TrimSpace(service.cfg.Share) == "" || strings.TrimSpace(service.cfg.LocalRoot) == "" {
		return OpenResult{}, ErrSMBNotConfigured
	}
	if service.storage == nil {
		return OpenResult{}, errors.New("SMB 文件同步存储未配置")
	}
	if service.repository == nil {
		return OpenResult{}, errors.New("编辑会话数据库未配置")
	}
	if !canEdit(user.Roles) {
		return OpenResult{}, errors.New("当前账号没有 CAD 编辑权限")
	}
	item, err := service.attachments.Find(ctx, storageKey)
	if err != nil {
		return OpenResult{}, err
	}
	if !isCADFile(item.Name) {
		return OpenResult{}, errors.New("当前附件不是可直接编辑的 CAD 文件")
	}

	actualStorageKey := item.StorageKey
	// 本地编辑需求：统一转换为 DWG 后打开
	ext := filepath.Ext(item.StorageKey)
	if ext == "" {
		ext = filepath.Ext(item.Name)
	}
	if strings.EqualFold(ext, ".exb") && service.converter != nil {
		dwgKey, convErr := service.converter.EnsureDwg(ctx, item)
		if convErr != nil {
			return OpenResult{}, fmt.Errorf("将 EXB 转换为本地 DWG 失败: %w", convErr)
		}
		actualStorageKey = dwgKey
		// 同步更新 attachments 表的 current_storage_key 为该 dwgKey
		dwgName := strings.TrimSuffix(item.Name, ext) + ".dwg"
		if reader, info, openErr := service.storage.Open(ctx, dwgKey); openErr == nil {
			_ = reader.Close()
			_ = service.attachments.SetCurrentContent(ctx, item.StorageKey, dwgKey, dwgName, info.Size, "application/acad", info.SHA256)
		}
	} else if item.CurrentStorageKey != "" {
		// 校验当前 key 是否在对象存储中真实存在，若已被物理清理则安全回退到原始 key
		if reader, _, statErr := service.storage.Open(ctx, item.CurrentStorageKey); statErr == nil {
			_ = reader.Close()
			actualStorageKey = item.CurrentStorageKey
		} else {
			log.Printf("[SMB编辑] 当前工作 key 不存在: %s，自动回退到原始 key: %s", item.CurrentStorageKey, item.StorageKey)
			actualStorageKey = item.StorageKey
		}
	}

	now := time.Now().UTC()
	if err := service.repository.ExpireStale(ctx, now); err != nil {
		return OpenResult{}, err
	}
	if _, err := service.repository.FindActiveByStorageKey(ctx, item.StorageKey, now); err != nil && !errors.Is(err, ErrSessionNotFound) {
		return OpenResult{}, err
	} else if err == nil {
		return OpenResult{}, ErrFileBusy
	}
	if err := service.syncToWorkDirectory(ctx, actualStorageKey); err != nil {
		return OpenResult{}, fmt.Errorf("准备 SMB 工作文件失败: %w", err)
	}
	sessionID, err := randomID()
	if err != nil {
		return OpenResult{}, fmt.Errorf("创建编辑会话失败: %w", err)
	}
	openTicket, err := randomID()
	if err != nil {
		return OpenResult{}, fmt.Errorf("创建打开票据失败: %w", err)
	}
	expiresAt := now.Add(60 * time.Second)
	session := Session{
		ID:           sessionID,
		AttachmentID: item.ID,
		StorageKey:   item.StorageKey,
		UserID:       user.ID,
		UserName:     user.DisplayName,
		UNCPath:      service.uncPath(actualStorageKey),
		Status:       "active",
		StartedAt:    now,
		LastSeenAt:   now,
	}
	if err := service.repository.CreateSession(ctx, session); err != nil {
		return OpenResult{}, err
	}
	if err := service.repository.CreateTicket(ctx, openTicket, sessionID, user.ID, expiresAt); err != nil {
		_ = service.repository.Close(ctx, user.ID, sessionID, false, now)
		return OpenResult{}, err
	}
	return OpenResult{
		SessionID: sessionID,
		OpenURL:   "cadguanliq://open?ticket=" + openTicket,
		UNCPath:   session.UNCPath,
		SMBRoot:   service.smbRoot(),
		ExpiresAt: expiresAt,
	}, nil
}

func (service *Service) Exchange(ctx context.Context, user auth.AuthUser, openTicket string) (ExchangeResult, error) {
	if service.repository == nil {
		return ExchangeResult{}, errors.New("编辑会话数据库未配置")
	}
	now := time.Now().UTC()
	session, err := service.repository.ConsumeTicket(ctx, openTicket, user.ID, now)
	if err != nil {
		return ExchangeResult{}, err
	}
	item, err := service.attachments.Find(ctx, session.StorageKey)
	if err != nil {
		return ExchangeResult{}, err
	}
	caxaPath, err := converter.ResolveCaxaPath("")
	if err != nil {
		return ExchangeResult{}, err
	}

	actualStorageKey := item.StorageKey
	if item.CurrentStorageKey != "" {
		actualStorageKey = item.CurrentStorageKey
	} else if strings.EqualFold(filepath.Ext(item.StorageKey), ".exb") {
		actualStorageKey = strings.TrimSuffix(item.StorageKey, filepath.Ext(item.StorageKey)) + ".dwg"
	}

	fileName := filepath.Base(actualStorageKey)

	return ExchangeResult{
		SessionID:       session.ID,
		SMBRoot:         service.smbRoot(),
		UNCPath:         service.uncPath(actualStorageKey),
		FileName:        fileName,
		CaxaPath:        caxaPath,
		SMBUsername:     service.cfg.Username,
		SMBPassword:     service.cfg.Password,
		HeartbeatSecond: 30,
	}, nil
}

func (service *Service) ListActive(ctx context.Context, user auth.AuthUser, drawingNo string) ([]ActiveSessionInfo, error) {
	if service.repository == nil {
		return nil, errors.New("编辑会话数据库未配置")
	}
	now := time.Now().UTC()
	if err := service.repository.ExpireStale(ctx, now); err != nil {
		return nil, err
	}
	sessions, err := service.repository.ListActiveSessions(ctx, now, drawingNo)
	if err != nil {
		return nil, err
	}
	isAdminUser := isAdmin(user.Roles)
	for i := range sessions {
		actualKey := sessions[i].StorageKey
		if strings.EqualFold(filepath.Ext(actualKey), ".exb") {
			actualKey = strings.TrimSuffix(actualKey, filepath.Ext(actualKey)) + ".dwg"
		}
		sessions[i].UNCPath = service.uncPath(actualKey)
		sessions[i].IsCurrent = sessions[i].UserID == user.ID
		sessions[i].CanClose = sessions[i].IsCurrent || isAdminUser
	}
	return sessions, nil
}

func (service *Service) Heartbeat(ctx context.Context, user auth.AuthUser, sessionID string) error {
	if service.repository == nil {
		return errors.New("编辑会话数据库未配置")
	}
	now := time.Now().UTC()
	if err := service.repository.ExpireStale(ctx, now); err != nil {
		return err
	}
	return service.repository.Heartbeat(ctx, user.ID, sessionID, now)
}

func (service *Service) Close(ctx context.Context, user auth.AuthUser, sessionID string) error {
	if service.repository == nil {
		return errors.New("编辑会话数据库未配置")
	}

	// 结束编辑时主动从工作区提取最新改动并捕获版本
	if service.versions != nil {
		activeList, err := service.repository.ListActiveSessions(ctx, time.Now().UTC(), "")
		if err == nil {
			for _, s := range activeList {
				if s.ID == sessionID {
					actualKey := s.StorageKey
					ext := filepath.Ext(actualKey)
					if strings.EqualFold(ext, ".exb") {
						actualKey = strings.TrimSuffix(actualKey, ext) + ".dwg"
					}
					if path, pathErr := service.localPath(actualKey); pathErr == nil {
						if _, _, capErr := service.versions.CapturePath(ctx, s.StorageKey, path, user.ID); capErr != nil {
							log.Printf("[编辑关闭] 捕获版本失败 key=%s err=%v", s.StorageKey, capErr)
						} else {
							log.Printf("[编辑关闭] 已生成新版本并回写当前图纸 key=%s", s.StorageKey)
						}
					}
					break
				}
			}
		}
	}

	isAdminUser := isAdmin(user.Roles)
	return service.repository.Close(ctx, user.ID, sessionID, isAdminUser, time.Now().UTC())
}

func (service *Service) StartCleanup(ctx context.Context) {
	if service == nil || service.repository == nil {
		return
	}
	if repository, ok := service.repository.(*PGRepository); ok && (repository == nil || repository.pool == nil) {
		return
	}
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			now := time.Now().UTC()
			if err := service.repository.ExpireStale(ctx, now); err != nil {
				continue
			}
			_ = service.repository.CleanupTickets(ctx, now)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (service *Service) uncPath(storageKey string) string {
	return service.smbRoot() + `\` + strings.ReplaceAll(storageKey, "/", `\`)
}

func (service *Service) syncToWorkDirectory(ctx context.Context, storageKey string) error {
	path, err := service.localPath(storageKey)
	if err != nil {
		return err
	}
	reader, _, err := service.storage.Open(ctx, storageKey)
	if err != nil {
		return fmt.Errorf("读取对象存储文件失败: %w", err)
	}
	defer reader.Close()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建 SMB 工作目录失败: %w", err)
	}
	temporaryPath := path + ".cadguanliq.tmp"
	file, err := os.Create(temporaryPath)
	if err != nil {
		return fmt.Errorf("创建 SMB 工作临时文件失败: %w", err)
	}
	_, copyErr := io.Copy(file, reader)
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(temporaryPath)
		return fmt.Errorf("复制对象存储文件到 SMB 工作目录失败: %w", copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(temporaryPath)
		return fmt.Errorf("关闭 SMB 工作临时文件失败: %w", closeErr)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		_ = os.Remove(temporaryPath)
		return fmt.Errorf("替换 SMB 工作文件失败: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		_ = os.Remove(temporaryPath)
		return fmt.Errorf("落盘 SMB 工作文件失败: %w", err)
	}
	return nil
}

func (service *Service) localPath(storageKey string) (string, error) {
	key := strings.ReplaceAll(storageKey, "\\", "/")
	segments := strings.Split(key, "/")
	if key == "" {
		return "", errors.New("SMB 工作文件路径为空")
	}
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." || strings.ContainsAny(segment, ":\r\n") {
			return "", errors.New("SMB 工作文件路径无效")
		}
	}
	root, err := filepath.Abs(service.cfg.LocalRoot)
	if err != nil {
		return "", fmt.Errorf("解析 SMB 工作目录失败: %w", err)
	}
	path, err := filepath.Abs(filepath.Join(append([]string{root}, segments...)...))
	if err != nil {
		return "", fmt.Errorf("解析 SMB 工作文件路径失败: %w", err)
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("SMB 工作文件路径越界")
	}
	return path, nil
}

func (service *Service) smbRoot() string {
	return `\\` + strings.Trim(service.cfg.Host, `\`) + `\` + strings.Trim(service.cfg.Share, `\`)
}

func canEdit(roles []string) bool {
	for _, role := range roles {
		if role == "admin" || role == "designer" {
			return true
		}
	}
	return false
}

func isAdmin(roles []string) bool {
	for _, role := range roles {
		if role == "admin" {
			return true
		}
	}
	return false
}

func isCADFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".dwg", ".dxf", ".exb":
		return true
	default:
		return false
	}
}

func randomID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(bytes)
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:], nil
}
