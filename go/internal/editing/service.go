package editing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cadguanliq/internal/attachment"
	"cadguanliq/internal/auth"
	"cadguanliq/internal/config"
	"cadguanliq/internal/converter"
	"cadguanliq/internal/drawing"
	"cadguanliq/internal/storage"
	"cadguanliq/internal/versioning"
)

var (
	ErrSMBNotConfigured = errors.New("SMB 文件共享未配置")
	ErrSessionNotFound  = errors.New("编辑会话不存在或已过期")
	ErrFileBusy         = errors.New("该文件正在被其他用户编辑")
	ErrInvalidTicket    = errors.New("打开票据无效或已使用")
)

// DrawingLookup 提供图纸生命周期查询（状态与创建者），用于编辑权限强校验。
type DrawingLookup interface {
	FindByNo(ctx context.Context, no string) (drawing.Drawing, error)
}

// ReviewAssigneeLookup 提供审核中图纸当前活动节点责任人查询。
type ReviewAssigneeLookup interface {
	ActiveCaseAssigneeByDrawingNo(ctx context.Context, drawingNo string) (string, error)
}

type Service struct {
	attachments attachment.Repository
	storage     storage.ObjectStorage
	converter   *converter.Service
	caxaBin     string
	versions    interface {
		CapturePath(ctx context.Context, sourceKey, sourcePath, userID string) (versioning.Version, bool, error)
	}
	repository Repository
	cfg        config.SMBConfig
	drawings   DrawingLookup
	reviews    ReviewAssigneeLookup
}

func NewService(sessionRepository Repository, attachmentRepository attachment.Repository, objectStorage storage.ObjectStorage, convService *converter.Service, smbConfig config.SMBConfig) *Service {
	var caxaBin string
	if convService != nil {
		caxaBin = convService.CaxaBin()
	}
	return &Service{
		attachments: attachmentRepository,
		storage:     objectStorage,
		converter:   convService,
		caxaBin:     caxaBin,
		repository:  sessionRepository,
		cfg:         smbConfig,
	}
}

// SetPolicy 装配图纸生命周期与审核责任人查询（编辑权限强校验）。
func (service *Service) SetPolicy(drawings DrawingLookup, reviews ReviewAssigneeLookup) {
	service.drawings = drawings
	service.reviews = reviews
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
	item, err := service.attachments.Find(ctx, storageKey)
	if err != nil {
		return OpenResult{}, err
	}
	if !isCADFile(item.Name) {
		return OpenResult{}, errors.New("当前附件不是可直接编辑的 CAD 文件")
	}
	// 图纸生命周期 × 身份统一授权：角色门禁并入状态矩阵——
	// 审核中当前节点责任人可编辑（即使无 designer 角色）；存档仅管理员经解除存档后编辑；
	// 草稿/生产仅创建者或管理员可编辑。其他用户一律走「本地查看（只读）」。
	if err := service.authorizeEdit(ctx, user, item.DrawingNo); err != nil {
		return OpenResult{}, err
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
	existing, findErr := service.repository.FindActiveByStorageKey(ctx, item.StorageKey, now)
	if findErr != nil && !errors.Is(findErr, ErrSessionNotFound) {
		return OpenResult{}, findErr
	}
	if findErr == nil {
		// 同一文件已有活动会话：本人则认领恢复（重新同步工作文件并签发打开票据），
		// 其他人则提示占用。退出软件后重新打开即可继续编辑，无需重新占用。
		if existing.UserID != user.ID {
			return OpenResult{}, ErrFileBusy
		}
		if err := service.syncToWorkDirectory(ctx, actualStorageKey); err != nil {
			return OpenResult{}, fmt.Errorf("准备 SMB 工作文件失败: %w", err)
		}
		openTicket, ticketErr := randomID()
		if ticketErr != nil {
			return OpenResult{}, fmt.Errorf("创建打开票据失败: %w", ticketErr)
		}
		expiresAt := now.Add(60 * time.Second)
		if err := service.repository.CreateTicket(ctx, openTicket, existing.ID, user.ID, expiresAt); err != nil {
			return OpenResult{}, err
		}
		if err := service.repository.Heartbeat(ctx, user.ID, existing.ID, now); err != nil {
			return OpenResult{}, err
		}
		return OpenResult{
			SessionID: existing.ID,
			OpenURL:   "cadguanliq://open?ticket=" + openTicket,
			UNCPath:   service.uncPath(actualStorageKey),
			SMBRoot:   service.smbRoot(),
			ExpiresAt: expiresAt,
		}, nil
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

// authorizeEdit 图纸生命周期 × 身份的编辑授权矩阵（后端强校验，角色门禁并入矩阵）：
// 草稿/生产 → 创建者或管理员；审核中 → 当前节点责任人或管理员（审核人员即使只有 reviewer 角色也可签改）；
// 存档 → 一律拒绝（管理员须先解除存档）。
func (service *Service) authorizeEdit(ctx context.Context, user auth.AuthUser, drawingNo string) error {
	admin := isAdmin(user.Roles)
	designer := false
	for _, role := range user.Roles {
		if role == "designer" {
			designer = true
			break
		}
	}
	drawingNo = strings.TrimSpace(drawingNo)

	// 未装配策略（非图纸附件等）：退回旧的角色门禁。
	if service.drawings == nil || drawingNo == "" {
		if admin || designer {
			return nil
		}
		return errors.New("当前账号没有 CAD 编辑权限")
	}

	item, err := service.drawings.FindByNo(ctx, drawingNo)
	if errors.Is(err, drawing.ErrNotFound) {
		return errors.New("未找到图纸信息，无法发起编辑")
	}
	if err != nil {
		return fmt.Errorf("读取图纸状态失败: %w", err)
	}

	switch item.Status {
	case drawing.StatusReviewing:
		if admin {
			return nil
		}
		assignee, assigneeErr := service.reviews.ActiveCaseAssigneeByDrawingNo(ctx, drawingNo)
		if assigneeErr != nil {
			return fmt.Errorf("读取审核节点责任人失败: %w", assigneeErr)
		}
		if assignee != "" && assignee == user.ID {
			return nil
		}
		if designer {
			return errors.New("审核中的图纸仅当前节点责任人可编辑，其他用户请使用「本地查看（只读）」")
		}
		return errors.New("审核中的图纸仅当前节点责任人可编辑，当前账号不是该节点责任人")
	case drawing.StatusArchived:
		return errors.New("图纸已存档，处于只读保护中；如需修改请联系管理员解除存档")
	default:
		if admin {
			return nil
		}
		if designer && item.CreatedByID == user.ID {
			return nil
		}
		if designer {
			return errors.New("仅创建者或管理员可以编辑图纸，其他用户请使用「本地查看（只读）」")
		}
		return errors.New("当前账号没有 CAD 编辑权限")
	}
}

// ReadOnlyOpen 只读查看：不建会话、不占文件锁、不捕获版本。
// 解析当前可编辑格式（EXB 自动转换为 DWG）后返回附件下载相对路径，
// 客户端下载到本机临时目录打开，退出后由客户端销毁临时文件。
func (service *Service) ReadOnlyOpen(ctx context.Context, user auth.AuthUser, storageKey string) (ReadOnlyOpenResult, error) {
	if service.storage == nil {
		return ReadOnlyOpenResult{}, errors.New("文件存储未配置")
	}
	item, err := service.attachments.Find(ctx, storageKey)
	if err != nil {
		return ReadOnlyOpenResult{}, err
	}
	if !isCADFile(item.Name) {
		return ReadOnlyOpenResult{}, errors.New("当前附件不是可打开的 CAD 文件")
	}

	actualStorageKey := item.StorageKey
	ext := filepath.Ext(item.StorageKey)
	if ext == "" {
		ext = filepath.Ext(item.Name)
	}
	if strings.EqualFold(ext, ".exb") && service.converter != nil {
		dwgKey, convErr := service.converter.EnsureDwg(ctx, item)
		if convErr != nil {
			return ReadOnlyOpenResult{}, fmt.Errorf("将 EXB 转换为 DWG 失败: %w", convErr)
		}
		actualStorageKey = dwgKey
		dwgName := strings.TrimSuffix(item.Name, ext) + ".dwg"
		if reader, info, openErr := service.storage.Open(ctx, dwgKey); openErr == nil {
			_ = reader.Close()
			_ = service.attachments.SetCurrentContent(ctx, item.StorageKey, dwgKey, dwgName, info.Size, "application/acad", info.SHA256)
		}
	} else if item.CurrentStorageKey != "" {
		if reader, _, statErr := service.storage.Open(ctx, item.CurrentStorageKey); statErr == nil {
			_ = reader.Close()
			actualStorageKey = item.CurrentStorageKey
		} else {
			actualStorageKey = item.StorageKey
		}
	}
	// 只读打开同样仅提供可选提示；客户端负责在本机寻找 CAXA 或按文件关联启动。
	caxaPath, caxaErr := converter.ResolveCaxaPath(service.caxaBin)
	if caxaErr != nil {
		log.Printf("[编辑会话] 只读打开未找到服务器 CAXA，交由客户端启动: %v", caxaErr)
		caxaPath = ""
	}
	return ReadOnlyOpenResult{
		// 相对 API 前缀的路径（客户端 apiBase 已含 /api，直接拼接）。
		// 存储键可能含 %、括号、中文（如 3255%x4070），必须按路径段转义，否则 %x4 被当作 URL 转义序列。
		DownloadPath: "/attachments/" + encodeAttachmentKeyPath(actualStorageKey),
		FileName:     filepath.Base(actualStorageKey),
		CaxaPath:     caxaPath,
	}, nil
}

// encodeAttachmentKeyPath 按路径段转义存储键，保证含 %、空格、中文等字符的 key 能原样到达服务端。
func encodeAttachmentKeyPath(key string) string {
	segments := strings.Split(strings.ReplaceAll(key, "\\", "/"), "/")
	for index, segment := range segments {
		segments[index] = url.PathEscape(segment)
	}
	return strings.Join(segments, "/")
}

func (service *Service) Exchange(ctx context.Context, user auth.AuthUser, openTicket string) (ExchangeResult, error) {	if service.repository == nil {
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
	// CAXA 由客户端在本机查找或按文件关联启动，与服务端互不干扰；
	// 这里仅按 .env 的 CAD_CAXA_BIN 提供可选提示，找不到时返回空路径，不阻塞交换。
	caxaPath, caxaErr := converter.ResolveCaxaPath(service.caxaBin)
	if caxaErr != nil {
		log.Printf("[编辑会话] 服务器未找到 CAXA，交由客户端按文件关联启动: %v", caxaErr)
		caxaPath = ""
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

	// 结束编辑时主动从工作区提取最新改动并捕获版本。
	// CAD 编辑器保存是异步落盘的：若用户保存后立即结束会话，
	// 工作文件可能仍在写入，必须等待文件稳定后再捕获，否则会回写旧内容或损坏内容。
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
						if waitErr := waitForFileStable(path, 15*time.Second); waitErr != nil {
							log.Printf("[编辑关闭] 等待工作文件稳定失败 key=%s err=%v", s.StorageKey, waitErr)
						}
						if _, changed, capErr := service.captureWithRetry(ctx, s.StorageKey, path, user.ID); capErr != nil {
							log.Printf("[编辑关闭] 捕获版本失败 key=%s err=%v", s.StorageKey, capErr)
						} else if changed {
							log.Printf("[编辑关闭] 已生成新版本并回写当前图纸 key=%s", s.StorageKey)
						} else {
							log.Printf("[编辑关闭] 工作文件无改动，未生成新版本 key=%s", s.StorageKey)
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

// captureWithRetry 捕获工作文件版本；文件可能被 CAD 进程短暂占用，失败后小间隔重试。
func (service *Service) captureWithRetry(ctx context.Context, sourceKey, sourcePath, userID string) (versioning.Version, bool, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return versioning.Version{}, false, ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}
		version, created, err := service.versions.CapturePath(ctx, sourceKey, sourcePath, userID)
		if err == nil {
			return version, created, nil
		}
		lastErr = err
		log.Printf("[编辑关闭] 捕获版本第 %d 次失败 key=%s: %v", attempt+1, sourceKey, err)
	}
	return versioning.Version{}, false, lastErr
}

// waitForFileStable 轮询文件大小与修改时间，连续两次采样一致视为写入完成。
func waitForFileStable(path string, timeout time.Duration) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(700 * time.Millisecond)
	defer ticker.Stop()

	var lastSize int64
	var lastMod time.Time
	first := true
	for {
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("查看工作文件状态失败: %w", err)
		}
		if !first && info.Size() == lastSize && info.ModTime().Equal(lastMod) {
			return nil
		}
		lastSize = info.Size()
		lastMod = info.ModTime()
		first = false
		select {
		case <-deadline.C:
			return errors.New("等待工作文件写入稳定超时")
		case <-ticker.C:
		}
	}
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
