package editing

import (
	"bytes"
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
	"sync"
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
	ErrTicketClosed     = errors.New("变更工单已不在执行中，无法开始编辑")
	// ErrWorkVersionConflict 在线保存的工作版本基线已过期：他人已推进该工单成果，需重新加载。
	ErrWorkVersionConflict = errors.New("工作版本已被更新，请重新加载后重试")
	// ErrNotOnlineTicket 该图纸未处于存档变更工单执行中，在线保存不应走工作版本通道。
	ErrNotOnlineTicket = errors.New("该图纸未处于变更工单执行中，请使用常规替换上传")
)

const onlineSaveMaxBytes = 200 << 20

const editTicketTTL = 5 * time.Minute

// DrawingLookup 提供图纸生命周期查询（状态与创建者），用于编辑权限强校验。
type DrawingLookup interface {
	FindByNo(ctx context.Context, no string) (drawing.Drawing, error)
}

// ReviewAssigneeLookup 提供审核中图纸当前活动节点责任人查询。
type ReviewAssigneeLookup interface {
	ActiveCaseAssigneeByDrawingNo(ctx context.Context, drawingNo string) (string, error)
}

// ChangeGate 提供存档图纸的变更工单授权与工作版本隔离能力。
type ChangeGate interface {
	// CanEditArchived 报告用户是否因持有执行中的工单而可编辑该存档图纸附件，返回工单 ID。
	CanEditArchived(ctx context.Context, drawingID, attachmentID, userID string) (bool, string, error)
	// WorkVersion 返回工单中指定附件的当前工作成果；无工作版本时 found=false。
	WorkVersion(ctx context.Context, requestID, attachmentID string) (storageKey, versionID string, found bool, err error)
	// EditBaseline 返回指定附件的当前工作版本（无工作版本时为创建工单时基线）内容哈希。
	EditBaseline(ctx context.Context, requestID, attachmentID string) (sha string, found bool, err error)
	// RecordWorkVersion 把指定附件的最新工作版本登记到工单目标。
	RecordWorkVersion(ctx context.Context, requestID, attachmentID, versionID string) error
	CompareAndRecordWorkVersion(ctx context.Context, requestID, attachmentID, versionID, expectedVersionID, userID string) (bool, error)
	// StillExecuting 报告工单是否仍处于可编辑（executing）状态。
	StillExecuting(ctx context.Context, requestID string) (bool, error)
}

type Service struct {
	attachments attachment.Repository
	storage     storage.ObjectStorage
	converter   *converter.Service
	caxaBin     string
	versions    interface {
		CapturePath(ctx context.Context, sourceKey, sourcePath, userID string) (versioning.Version, bool, error)
		CaptureWorking(ctx context.Context, sourceKey, sourcePath, userID, baselineSHA string) (versioning.Version, bool, error)
		EnsureInitialVersion(ctx context.Context, sourceKey, userID string) error
	}
	repository Repository
	cfg        config.SMBConfig
	drawings   DrawingLookup
	reviews    ReviewAssigneeLookup
	changes    ChangeGate
	// syncLocks 按存储键串行化工作文件同步：Remove+Rename 的替换序列在 Windows 上
	// 不允许并发执行（目标被其他 rename 占用时失败），多用户同时打开同一文件时必须互斥。
	syncMu   sync.Mutex
	syncLock map[string]*sync.Mutex
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

// SetChangeGate 装配存档图纸变更工单门禁。
func (service *Service) SetChangeGate(gate ChangeGate) {
	service.changes = gate
}

// SetVersioning 装配版本捕获服务。
func (service *Service) SetVersioning(versions interface {
	CapturePath(ctx context.Context, sourceKey, sourcePath, userID string) (versioning.Version, bool, error)
	CaptureWorking(ctx context.Context, sourceKey, sourcePath, userID, baselineSHA string) (versioning.Version, bool, error)
	EnsureInitialVersion(ctx context.Context, sourceKey, userID string) error
}) {
	service.versions = versions
}

// resolveWorkKey 解析实际用于本地编辑的工作文件（当前版本 DWG）：
// EXB 先经转换队列生成 v1.0 版本文件，其余附件优先使用当前指针。
// 返回工作文件键，并保证 v1.0 初始版本已登记、当前指针已切换。
func (service *Service) resolveWorkKey(ctx context.Context, user auth.AuthUser, item attachment.Attachment, changeRequestID string) (string, error) {
	// 存档变更工单：若该工单已登记工作成果版本，则基于它继续编辑，
	// 不回到正式版本，保证未验收成果与正式版隔离。
	if changeRequestID != "" && service.changes != nil {
		storageKey, _, found, gateErr := service.changes.WorkVersion(ctx, changeRequestID, item.ID)
		if gateErr != nil {
			return "", fmt.Errorf("读取工单工作版本失败: %w", gateErr)
		}
		if found && storageKey != "" {
			if reader, _, openErr := service.storage.Open(ctx, storageKey); openErr == nil {
				_ = reader.Close()
				return storageKey, nil
			} else {
				return "", fmt.Errorf("读取工单工作文件失败，禁止回退正式版: %w", openErr)
			}
		}
		if found {
			return "", errors.New("工单工作版本存储键缺失")
		}
	}
	ext := filepath.Ext(item.StorageKey)
	if ext == "" {
		ext = filepath.Ext(item.Name)
	}
	// EXB 附件提交后会保留原始文件，同时把可编辑的 DWG 放到 current_blob_id。
	// currentStorageKey 可能是 blobs/<hash>，不能靠它自身的扩展名判断格式；
	// currentName 才是当前对象的真实文件名。已有有效 DWG 时禁止再次读取/转换原始 EXB。
	if item.CurrentStorageKey != "" && strings.EqualFold(filepath.Ext(item.CurrentName), ".dwg") {
		if reader, _, statErr := service.storage.Open(ctx, item.CurrentStorageKey); statErr == nil {
			_ = reader.Close()
			return item.CurrentStorageKey, nil
		}
		log.Printf("[编辑会话] 当前 DWG 对象不存在: %s，继续处理原始文件", item.CurrentStorageKey)
	}
	if strings.EqualFold(ext, ".exb") && service.converter != nil {
		// 持久化上传队列拥有转换、归档和指针切换。EnsureDwg 返回的是
		// 随后会被队列清理的临时路径，不能直接作为 SMB 编辑源。
		if finder, ok := service.attachments.(interface {
			FindByID(context.Context, string) (attachment.Attachment, error)
		}); ok {
			readyCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()
			return service.waitForCurrentDWG(readyCtx, item.ID, finder.FindByID)
		}
		dwgKey, convErr := service.converter.EnsureDwg(ctx, item)
		if convErr != nil {
			return "", fmt.Errorf("将 EXB 转换为本地 DWG 失败: %w", convErr)
		}
		// 登记初始版本 v1.0（版本记录 + 当前指针切换）；失败时兜底更新指针，
		// 保证本次编辑打开的仍是转换后的 DWG，不让版本登记问题阻塞编辑。
		if service.versions != nil {
			if ensureErr := service.versions.EnsureInitialVersion(ctx, item.StorageKey, user.ID); ensureErr != nil {
				log.Printf("[编辑会话] 登记初始版本失败 storageKey=%s: %v", item.StorageKey, ensureErr)
			}
		}
		if fresh, findErr := service.attachments.Find(ctx, item.StorageKey); findErr != nil || fresh.CurrentStorageKey != dwgKey {
			dwgName := strings.TrimSuffix(item.Name, ext) + ".dwg"
			if reader, info, openErr := service.storage.Open(ctx, dwgKey); openErr == nil {
				_ = reader.Close()
				_ = service.attachments.SetCurrentVersion(ctx, item.StorageKey, dwgKey, dwgName, "v1.0", info.Size, "application/acad", info.SHA256)
			}
		}
		return dwgKey, nil
	}

	// 非 EXB：优先使用当前版本指针；当前文件已被物理清理时安全回退到原始 key。
	if item.CurrentStorageKey != "" && item.CurrentStorageKey != item.StorageKey {
		if reader, _, statErr := service.storage.Open(ctx, item.CurrentStorageKey); statErr == nil {
			_ = reader.Close()
			return item.CurrentStorageKey, nil
		}
		log.Printf("[编辑会话] 当前工作 key 不存在: %s，自动回退到原始 key: %s", item.CurrentStorageKey, item.StorageKey)
	}
	return item.StorageKey, nil
}

// 仅接受附件当前指针指向且实际存在的 DWG；不消费临时转换产物。
func (service *Service) waitForCurrentDWG(ctx context.Context, attachmentID string, find func(context.Context, string) (attachment.Attachment, error)) (string, error) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		item, err := find(ctx, attachmentID)
		if err != nil {
			return "", fmt.Errorf("读取图纸转换状态失败: %w", err)
		}
		if strings.EqualFold(filepath.Ext(item.CurrentName), ".dwg") && item.CurrentStorageKey != "" {
			reader, info, openErr := service.storage.Open(ctx, item.CurrentStorageKey)
			if openErr == nil {
				_ = reader.Close()
				if info.Size > 0 {
					return item.CurrentStorageKey, nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("图纸转换尚未就绪，请稍后从文件列表重试本地编辑（无需重新创建图纸）: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

// editWorkStorageKey 是 SMB 工作副本的逻辑文件名，不是内容对象键。
// file_blobs 使用 blobs/<hash> 存储时没有扩展名，不能直接交给 CAXA；
// 工作副本必须保留 CAD 文件扩展名，供 CAXA 按 DWG/EXB/DXF 正确识别。
func editWorkStorageKey(sessionID, sourceStorageKey string, item attachment.Attachment) (string, error) {
	// 当前版本实际可用时使用 currentName（通常是 EXB 转换后的 DWG）。
	// 当前版本已丢失而回退原文件时，必须保持原文件扩展名，不能把 EXB 内容伪装成 DWG。
	name := strings.TrimSpace(item.Name)
	fallbackRawEXB := sourceStorageKey == item.StorageKey && strings.EqualFold(filepath.Ext(item.Name), ".exb") && strings.EqualFold(filepath.Ext(sourceStorageKey), ".exb")
	if strings.TrimSpace(item.CurrentName) != "" && !fallbackRawEXB {
		name = strings.TrimSpace(item.CurrentName)
	}
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	if name == "" || name == "." {
		return "", errors.New("附件缺少真实文件名，无法创建 CAXA 工作副本")
	}
	if filepath.Ext(name) == "" {
		ext := filepath.Ext(item.Name)
		if ext == "" {
			ext = filepath.Ext(sourceStorageKey)
		}
		if ext == "" {
			return "", fmt.Errorf("附件真实文件名缺少扩展名: %q", name)
		}
		name += ext
	}
	return "work/" + sessionID + "/" + name, nil
}

func (service *Service) Open(ctx context.Context, user auth.AuthUser, storageKey string, attachmentIDs ...string) (OpenResult, error) {
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
	var item attachment.Attachment
	var err error
	if len(attachmentIDs) > 0 && attachmentIDs[0] != "" {
		finder, ok := service.attachments.(interface {
			FindByID(context.Context, string) (attachment.Attachment, error)
		})
		if !ok {
			return OpenResult{}, errors.New("附件身份查询未配置")
		}
		item, err = finder.FindByID(ctx, attachmentIDs[0])
	} else {
		item, err = service.attachments.Find(ctx, storageKey)
	}
	if err != nil {
		return OpenResult{}, err
	}
	if !isCADFile(item.Name) {
		return OpenResult{}, errors.New("当前附件不是可直接编辑的 CAD 文件")
	}
	// 图纸生命周期 × 身份统一授权：角色门禁并入状态矩阵——
	// 审核中当前节点责任人可编辑（即使无 designer 角色）；存档仅管理员经解除存档后编辑；
	// 草稿/生产仅创建者或管理员可编辑。其他用户一律走「本地查看（只读）」。
	changeRequestID, err := service.authorizeEdit(ctx, user, item.DrawingNo, item.ID)
	if err != nil {
		return OpenResult{}, err
	}

	sourceStorageKey, err := service.resolveWorkKey(ctx, user, item, changeRequestID)
	if err != nil {
		return OpenResult{}, err
	}
	// 转换等待期间附件的当前键和文件名会改变，工作副本必须使用刷新后的元数据。
	if finder, ok := service.attachments.(interface {
		FindByID(context.Context, string) (attachment.Attachment, error)
	}); ok {
		item, err = finder.FindByID(ctx, item.ID)
		if err != nil {
			return OpenResult{}, err
		}
	}

	now := time.Now().UTC()
	if err := service.repository.ExpireStale(ctx, now); err != nil {
		return OpenResult{}, err
	}
	var existing Session
	var findErr error
	if finder, ok := service.repository.(interface {
		FindActiveByAttachmentID(context.Context, string, time.Time) (Session, error)
	}); ok {
		existing, findErr = finder.FindActiveByAttachmentID(ctx, item.ID, now)
	} else {
		existing, findErr = service.repository.FindActiveByStorageKey(ctx, item.StorageKey, now)
		if findErr == nil && existing.AttachmentID != item.ID {
			findErr = ErrSessionNotFound
		}
	}
	if findErr != nil && !errors.Is(findErr, ErrSessionNotFound) {
		return OpenResult{}, findErr
	}
	if findErr == nil {
		// 会话归属校验：活动会话只能被"同一工单"复用，禁止把旧工单（或无工单）的会话
		// 直接接管到新工单，避免旧工单未提交的文件与操作来源混入本次变更。
		if existing.ChangeRequestID != changeRequestID {
			return OpenResult{}, errors.New("该文件存在其它变更工单或历史编辑会话，请先结束它后再开始本次编辑")
		}
		// 同一文件已有活动会话：本人则认领恢复（重新同步工作文件并签发打开票据），
		// 其他人则提示占用。退出软件后重新打开即可继续编辑，无需重新占用。
		if existing.UserID != user.ID {
			return OpenResult{}, ErrFileBusy
		}
		workStorageKey := existing.WorkStorageKey
		if filepath.Ext(workStorageKey) == "" {
			workStorageKey, err = editWorkStorageKey(existing.ID, sourceStorageKey, item)
			if err != nil {
				return OpenResult{}, err
			}
			if err := service.repository.UpdateWorkStorageKey(ctx, user.ID, existing.ID, workStorageKey); err != nil {
				return OpenResult{}, err
			}
		}
		// 重新认领（呼出 CAD）不得覆盖工作文件：用户可能已保存但尚未结束编辑，
		// 工作文件里是未归档的最新修改；仅当工作文件丢失时才重新同步。
		if path, pathErr := service.localPath(workStorageKey); pathErr != nil {
			return OpenResult{}, pathErr
		} else if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
			if err := service.syncToWorkDirectory(ctx, sourceStorageKey, workStorageKey); err != nil {
				return OpenResult{}, fmt.Errorf("准备 SMB 工作文件失败: %w", err)
			}
		}
		openTicket, ticketErr := randomID()
		if ticketErr != nil {
			return OpenResult{}, fmt.Errorf("创建打开票据失败: %w", ticketErr)
		}
		expiresAt := now.Add(editTicketTTL)
		if err := service.repository.CreateTicket(ctx, openTicket, existing.ID, user.ID, expiresAt); err != nil {
			return OpenResult{}, err
		}
		if err := service.repository.Heartbeat(ctx, user.ID, existing.ID, now); err != nil {
			return OpenResult{}, err
		}
		return OpenResult{
			SessionID: existing.ID,
			OpenURL:   "cadguanliq://open?ticket=" + openTicket,
			UNCPath:   service.uncPath(workStorageKey),
			SMBRoot:   service.smbRoot(),
			ExpiresAt: expiresAt,
		}, nil
	}
	sessionID, err := randomID()
	if err != nil {
		return OpenResult{}, fmt.Errorf("创建编辑会话失败: %w", err)
	}
	workStorageKey, err := editWorkStorageKey(sessionID, sourceStorageKey, item)
	if err != nil {
		return OpenResult{}, err
	}
	if err := service.syncToWorkDirectory(ctx, sourceStorageKey, workStorageKey); err != nil {
		return OpenResult{}, fmt.Errorf("准备 SMB 工作文件失败: %w", err)
	}
	openTicket, err := randomID()
	if err != nil {
		return OpenResult{}, fmt.Errorf("创建打开票据失败: %w", err)
	}
	expiresAt := now.Add(editTicketTTL)
	session := Session{
		ID:              sessionID,
		AttachmentID:    item.ID,
		StorageKey:      item.StorageKey,
		WorkStorageKey:  workStorageKey,
		ChangeRequestID: changeRequestID,
		UserID:          user.ID,
		UserName:        user.DisplayName,
		UNCPath:         service.uncPath(workStorageKey),
		Status:          "active",
		StartedAt:       now,
		LastSeenAt:      now,
	}
	if changeRequestID != "" {
		// 存档工单：在同一事务内锁定工单行、复查其仍为 executing 后再写入会话与票据，
		// 与提交/终止互斥，杜绝"提交或终止后又开出新的可写会话"的时间窗口。
		if err := service.repository.CreateSessionWithTicket(ctx, session, openTicket, expiresAt, changeRequestID); err != nil {
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

// authorizeEdit 图纸生命周期 × 身份的编辑授权矩阵（后端强校验）。
// 返回的 changeRequestID 非空表示这次编辑受某张执行中的存档变更工单授权，
// 编辑成果须登记为工作版本、验收后才发布，不得直接切换正式指针。
func (service *Service) authorizeEdit(ctx context.Context, user auth.AuthUser, drawingNo, attachmentID string) (string, error) {
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
			return "", nil
		}
		return "", errors.New("当前账号没有 CAD 编辑权限")
	}

	item, err := service.drawings.FindByNo(ctx, drawingNo)
	if errors.Is(err, drawing.ErrNotFound) {
		return "", errors.New("未找到图纸信息，无法发起编辑")
	}
	if err != nil {
		return "", fmt.Errorf("读取图纸状态失败: %w", err)
	}

	if item.Status != drawing.StatusArchived && (admin || item.CreatedByID == user.ID) {
		return "", nil
	}
	switch item.Status {
	case drawing.StatusReviewing:
		if admin {
			return "", nil
		}
		if service.reviews == nil {
			return "", errors.New("审核节点查询未配置，无法确认编辑权限")
		}
		assignee, assigneeErr := service.reviews.ActiveCaseAssigneeByDrawingNo(ctx, drawingNo)
		if assigneeErr != nil {
			return "", fmt.Errorf("读取审核节点责任人失败: %w", assigneeErr)
		}
		if assignee != "" && assignee == user.ID {
			return "", nil
		}
		if designer {
			return "", errors.New("审核中的图纸仅当前节点责任人可编辑，其他用户请使用「本地查看（只读）」")
		}
		return "", errors.New("审核中的图纸仅当前节点责任人可编辑，当前账号不是该节点责任人")
	case drawing.StatusArchived:
		// 存档图纸默认只读；仅当存在执行中的变更工单且当前用户是指定执行人时放行编辑，
		// 并把工单 ID 透传给会话，用于把工作成果隔离在未验收版本上。
		if service.changes != nil {
			allowed, requestID, gateErr := service.changes.CanEditArchived(ctx, item.ID, attachmentID, user.ID)
			if gateErr != nil {
				return "", fmt.Errorf("查询变更工单授权失败: %w", gateErr)
			}
			if allowed {
				return requestID, nil
			}
		}
		return "", errors.New("图纸已存档，需通过变更工单审批并由指定执行人修改")
	default:
		if admin {
			return "", nil
		}
		if item.CreatedByID == user.ID {
			return "", nil
		}
		if designer {
			return "", errors.New("仅创建者或管理员可以编辑图纸，其他用户请使用「本地查看（只读）」")
		}
		return "", errors.New("当前账号没有 CAD 编辑权限")
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

	actualStorageKey, err := service.resolveWorkKey(ctx, user, item, "")
	if err != nil {
		return ReadOnlyOpenResult{}, err
	}
	// 只读打开同样仅提供可选提示；客户端负责在本机寻找 CAXA 或按文件关联启动。
	caxaPath, caxaErr := converter.ResolveCaxaPath(service.caxaBin)
	if caxaErr != nil {
		log.Printf("[编辑会话] 只读打开未找到服务器 CAXA，交由客户端启动: %v", caxaErr)
		caxaPath = ""
	}
	fileName := item.Name
	if actualStorageKey == item.CurrentStorageKey && strings.TrimSpace(item.CurrentName) != "" {
		fileName = item.CurrentName
	}
	return ReadOnlyOpenResult{
		// 相对 API 前缀的路径（客户端 apiBase 已含 /api，直接拼接）。
		// 存储键可能含 %、括号、中文（如 3255%x4070），必须按路径段转义，否则 %x4 被当作 URL 转义序列。
		DownloadPath: "/attachments/" + encodeAttachmentKeyPath(actualStorageKey),
		FileName:     filepath.Base(fileName),
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

func (service *Service) Exchange(ctx context.Context, user auth.AuthUser, openTicket string) (ExchangeResult, error) {
	if service.repository == nil {
		return ExchangeResult{}, errors.New("编辑会话数据库未配置")
	}
	now := time.Now().UTC()
	session, err := service.repository.ConsumeTicket(ctx, openTicket, user.ID, now)
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

	// 优先使用会话记录的工作文件键（当前版本 DWG）；旧会话无记录时回退附件当前指针。
	actualStorageKey := session.WorkStorageKey
	if actualStorageKey == "" {
		item, findErr := service.attachments.Find(ctx, session.StorageKey)
		if findErr != nil {
			return ExchangeResult{}, findErr
		}
		if item.CurrentStorageKey != "" {
			actualStorageKey = item.CurrentStorageKey
		} else {
			actualStorageKey = item.StorageKey
		}
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

// OnlineOpen 为浏览器在线编辑解析可编辑内容与工单归属。
// 存档图纸必须绑定执行中的变更工单：编辑成果只能登记为工单工作版本，验收后才发布，
// 正式版本在此之前不受影响。非存档图纸 RequiresTicket=false，前端沿用常规替换保存。
// LoadURL 指向当前应加载的内容：已有工作版本时加载工作版本，否则加载正式版当前文件。
func (service *Service) OnlineOpen(ctx context.Context, user auth.AuthUser, storageKey string) (OnlineOpenResult, error) {
	if service.attachments == nil {
		return OnlineOpenResult{}, errors.New("附件服务未配置")
	}
	storageKey = strings.TrimSpace(storageKey)
	if storageKey == "" {
		return OnlineOpenResult{}, errors.New("缺少 CAD 文件存储键")
	}
	item, err := service.attachments.Find(ctx, storageKey)
	if err != nil {
		return OnlineOpenResult{}, err
	}
	if !isCADFile(item.Name) {
		return OnlineOpenResult{}, errors.New("当前附件不是可编辑的 CAD 文件")
	}
	releasePath := "/cad/source?storageKey=" + url.QueryEscape(item.StorageKey)
	fileName := filepath.Base(firstNonEmpty(item.CurrentName, item.Name))
	changeRequestID, err := service.authorizeEdit(ctx, user, item.DrawingNo, item.ID)
	if err != nil {
		return OnlineOpenResult{}, err
	}
	if changeRequestID == "" {
		log.Printf("[Online Edit] opened attachment=%s revision=%d user=%s", item.ID, item.Revision, user.ID)
		return OnlineOpenResult{Revision: item.Revision, LoadURL: releasePath, FileName: fileName}, nil
	}
	workKey, workVersionID, found, gateErr := service.changes.WorkVersion(ctx, changeRequestID, item.ID)
	if gateErr != nil {
		return OnlineOpenResult{}, fmt.Errorf("读取工单工作版本失败: %w", gateErr)
	}
	if found {
		fileName = filepath.Base(workKey)
		return OnlineOpenResult{RequiresTicket: true, ChangeRequestID: changeRequestID, WorkVersionID: workVersionID, LoadURL: "/file-versions/" + workVersionID + "/source", FileName: fileName}, nil
	}
	return OnlineOpenResult{RequiresTicket: true, ChangeRequestID: changeRequestID, LoadURL: releasePath, FileName: fileName}, nil
}

// OnlineSaveDraft 把浏览器在线编辑产出的 DWG 登记为对应变更目标的工作版本：
// 以该附件上一次成果（或创建工单时基线）为比较基准捕获新版本，绝不切换正式当前指针。
// baseWorkVersionID 为编辑器加载时的该附件工作版本；与当前值不一致时拒绝覆盖。
// 捕获或登记失败时不删除已生成内容、不动正式版本，用户可安全重试。
func (service *Service) OnlineSaveDraft(ctx context.Context, user auth.AuthUser, storageKey, requestID, baseWorkVersionID, fileName string, content []byte) (OnlineSaveResult, error) {
	if service.attachments == nil || service.versions == nil {
		return OnlineSaveResult{}, errors.New("在线编辑保存服务未配置")
	}
	storageKey = strings.TrimSpace(storageKey)
	if storageKey == "" {
		return OnlineSaveResult{}, errors.New("缺少 CAD 文件存储键")
	}
	if len(content) == 0 {
		return OnlineSaveResult{}, errors.New("在线编辑内容为空，未保存")
	}
	if len(content) < 6 || !bytes.HasPrefix(content, []byte("AC10")) || content[4] < '0' || content[4] > '9' || content[5] < '0' || content[5] > '9' {
		return OnlineSaveResult{}, errors.New("转换结果不是有效的 DWG 文件，未保存")
	}
	item, err := service.attachments.Find(ctx, storageKey)
	if err != nil {
		return OnlineSaveResult{}, err
	}
	if !isCADFile(item.Name) {
		return OnlineSaveResult{}, errors.New("当前附件不是可编辑的 CAD 文件")
	}
	changeRequestID, err := service.authorizeEdit(ctx, user, item.DrawingNo, item.ID)
	if err != nil {
		return OnlineSaveResult{}, err
	}
	if changeRequestID == "" || service.changes == nil {
		return OnlineSaveResult{}, ErrNotOnlineTicket
	}
	if strings.TrimSpace(requestID) != changeRequestID {
		return OnlineSaveResult{}, ErrTicketClosed
	}
	executing, gateErr := service.changes.StillExecuting(ctx, changeRequestID)
	if gateErr != nil {
		return OnlineSaveResult{}, gateErr
	}
	if !executing {
		return OnlineSaveResult{}, ErrTicketClosed
	}
	_, currentWorkID, found, gateErr := service.changes.WorkVersion(ctx, changeRequestID, item.ID)
	if gateErr != nil {
		return OnlineSaveResult{}, fmt.Errorf("读取工单工作版本失败: %w", gateErr)
	}
	expected := strings.TrimSpace(baseWorkVersionID)
	if found {
		if expected != currentWorkID {
			return OnlineSaveResult{}, ErrWorkVersionConflict
		}
	} else if expected != "" {
		return OnlineSaveResult{}, ErrWorkVersionConflict
	}
	baselineSHA := ""
	if sha, ok, baseErr := service.changes.EditBaseline(ctx, changeRequestID, item.ID); baseErr != nil {
		return OnlineSaveResult{}, baseErr
	} else if ok {
		baselineSHA = sha
	}
	tempDir, err := os.MkdirTemp("", "online-save-")
	if err != nil {
		return OnlineSaveResult{}, fmt.Errorf("创建临时目录失败: %w", err)
	}
	defer os.RemoveAll(tempDir)
	tempPath := filepath.Join(tempDir, "edit.dwg")
	if err := os.WriteFile(tempPath, content, 0o600); err != nil {
		return OnlineSaveResult{}, fmt.Errorf("写入临时编辑文件失败: %w", err)
	}
	version, _, capErr := service.versions.CaptureWorking(ctx, item.StorageKey, tempPath, user.ID, baselineSHA)
	if capErr != nil {
		return OnlineSaveResult{}, fmt.Errorf("保存编辑版本失败，请重试: %w", capErr)
	}
	recorded, regErr := service.changes.CompareAndRecordWorkVersion(ctx, changeRequestID, item.ID, version.ID, expected, user.ID)
	if regErr != nil {
		return OnlineSaveResult{}, regErr
	}
	if !recorded {
		return OnlineSaveResult{}, ErrWorkVersionConflict
	}
	return OnlineSaveResult{ChangeRequestID: changeRequestID, WorkVersionID: version.ID, Version: version.Version, FileName: filepath.Base(firstNonEmpty(fileName, item.CurrentName, item.Name))}, nil
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
		// UNC 路径优先使用会话记录的工作文件键；旧会话无记录时按旧规则推算。
		actualKey := sessions[i].WorkStorageKey
		if actualKey == "" {
			actualKey = sessions[i].StorageKey
			if strings.EqualFold(filepath.Ext(actualKey), ".exb") {
				actualKey = strings.TrimSuffix(actualKey, filepath.Ext(actualKey)) + ".dwg"
			}
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

// Close 结束编辑：等待 SMB 工作文件写入稳定后捕获新版本（版本文件写入版本目录、
// 事务内切换当前指针），全部成功后才关闭会话。
// 捕获失败时保留编辑会话并返回错误，用户重试不会丢失工作内容；
// 无改动时直接关闭会话，不产生重复版本。
func (service *Service) Close(ctx context.Context, user auth.AuthUser, sessionID string) (CloseResult, error) {
	if service.repository == nil {
		return CloseResult{}, errors.New("编辑会话数据库未配置")
	}
	now := time.Now().UTC()
	if err := service.repository.ExpireStale(ctx, now); err != nil {
		return CloseResult{}, err
	}
	session, err := service.repository.FindActiveByID(ctx, sessionID)
	if err != nil {
		return CloseResult{}, err
	}
	isAdminUser := isAdmin(user.Roles)
	if session.UserID != user.ID && !isAdminUser {
		return CloseResult{}, errors.New("只能结束自己的编辑会话")
	}

	result := CloseResult{SessionID: sessionID}
	forceByAdmin := session.UserID != user.ID

	workKey := session.WorkStorageKey
	if workKey == "" {
		workKey = session.StorageKey
		if strings.EqualFold(filepath.Ext(workKey), ".exb") {
			workKey = strings.TrimSuffix(workKey, filepath.Ext(workKey)) + ".dwg"
		}
	}
	// 工单不可写时返回冲突，保留工作文件，不把放弃修改当作保存成功。
	working := session.ChangeRequestID != ""
	var workingVersionID string
	if working && service.changes == nil {
		return CloseResult{}, errors.New("工单编辑门禁未配置")
	}
	executing := true
	var baselineSHA string
	if working && service.changes != nil {
		ok, gateErr := service.changes.StillExecuting(ctx, session.ChangeRequestID)
		if gateErr != nil {
			return CloseResult{}, gateErr
		}
		executing = ok
		if sha, found, baseErr := service.changes.EditBaseline(ctx, session.ChangeRequestID, session.AttachmentID); baseErr != nil {
			return CloseResult{}, baseErr
		} else if found {
			baselineSHA = sha
		}
	}
	if path, pathErr := service.localPath(workKey); pathErr == nil {
		if working && !executing {
			return CloseResult{}, ErrTicketClosed
		} else {
			// CAD 编辑器保存是异步落盘的：若用户保存后立即结束会话，
			// 工作文件可能仍在写入，必须等待文件稳定后再捕获，否则会归档旧内容或损坏内容。
			if waitErr := waitForFileStable(path, 15*time.Second); waitErr != nil {
				if working {
					return CloseResult{}, waitErr
				}
				log.Printf("[编辑关闭] 等待工作文件稳定失败 key=%s err=%v", workKey, waitErr)
			}
			if version, changed, capErr := service.captureWithRetry(ctx, session.StorageKey, path, user.ID, working, baselineSHA, session.AttachmentID); capErr != nil {
				if forceByAdmin && !working {
					// 管理员强制关闭他人会话：尽力归档但不阻塞锁释放，避免死锁文件。
					log.Printf("[编辑关闭] 管理员强制关闭，捕获版本失败仍将关闭会话 key=%s err=%v", session.StorageKey, capErr)
				} else if _, stillActiveErr := service.repository.FindActiveByID(ctx, sessionID); errors.Is(stillActiveErr, ErrSessionNotFound) {
					// 会话已被并发关闭（如管理员强关导致工作文件被清理），
					// 此时不再误报“保存失败”，与 repository.Close 的语义对齐。
					return CloseResult{}, ErrSessionNotFound
				} else {
					// 捕获失败：保留编辑会话，旧版本不受影响，用户可重试。
					return CloseResult{}, fmt.Errorf("保存编辑版本失败，请重试: %w", capErr)
				}
			} else {
				result.Changed = changed
				if changed {
					result.Version = version.Version
					if working {
						// 捕获文件后，在下方事务内登记成果并关闭会话。
						workingVersionID = version.ID
					} else {
						result.CurrentStorageKey = version.StorageKey
						result.CurrentName = filepath.Base(version.StorageKey)
						log.Printf("[编辑关闭] 已生成新版本 %s key=%s", version.Version, version.StorageKey)
					}
				} else {
					log.Printf("[编辑关闭] 工作文件无改动，未生成新版本 key=%s", session.StorageKey)
				}
			}
		}
	} else {
		if working {
			return CloseResult{}, pathErr
		}
		log.Printf("[编辑关闭] 解析工作文件路径失败 key=%s err=%v", workKey, pathErr)
	}

	var closeErr error
	if working {
		closeErr = service.repository.CompleteWorkingSession(ctx, session, workingVersionID)
	} else {
		closeErr = service.repository.Close(ctx, user.ID, sessionID, isAdminUser, time.Now().UTC())
	}
	if closeErr != nil {
		return CloseResult{}, closeErr
	}

	// 版本已成功归档，清理 SMB 工作文件（清理失败不影响关闭结果）。
	if path, pathErr := service.localPath(workKey); pathErr == nil {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			log.Printf("[编辑关闭] 清理 SMB 工作文件失败 path=%s err=%v", path, err)
		}
	}
	return result, nil
}

// captureWithRetry 捕获工作文件版本；文件可能被 CAD 进程短暂占用，失败后小间隔重试。
// 工作文件已不存在（被并发关闭清理等）属于永久性错误，直接失败不重试。
// working=true 时走不切换正式指针的工作版本捕获（存档变更工单），
// baselineSHA 为该工单上一次成果（或基线）的哈希，用于正确判断相对工单成果的变化。
func (service *Service) captureWithRetry(ctx context.Context, sourceKey, sourcePath, userID string, working bool, baselineSHA string, attachmentIDs ...string) (versioning.Version, bool, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return versioning.Version{}, false, ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}
		var version versioning.Version
		var created bool
		var err error
		if capture, ok := service.versions.(interface {
			CaptureAttachment(context.Context, string, string, string, bool, string) (versioning.Version, bool, error)
		}); ok && len(attachmentIDs) > 0 && attachmentIDs[0] != "" {
			version, created, err = capture.CaptureAttachment(ctx, attachmentIDs[0], sourcePath, userID, !working, baselineSHA)
		} else if working {
			version, created, err = service.versions.CaptureWorking(ctx, sourceKey, sourcePath, userID, baselineSHA)
		} else {
			version, created, err = service.versions.CapturePath(ctx, sourceKey, sourcePath, userID)
		}
		if err == nil {
			return version, created, nil
		}
		if errors.Is(err, os.ErrNotExist) {
			return versioning.Version{}, false, err
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

// lockWorkFile 取得该存储键的工作文件同步互斥锁，返回解锁函数。
func (service *Service) lockWorkFile(storageKey string) func() {
	service.syncMu.Lock()
	if service.syncLock == nil {
		service.syncLock = make(map[string]*sync.Mutex)
	}
	lock, ok := service.syncLock[storageKey]
	if !ok {
		lock = &sync.Mutex{}
		service.syncLock[storageKey] = lock
	}
	service.syncMu.Unlock()
	lock.Lock()
	return lock.Unlock
}

func (service *Service) syncToWorkDirectory(ctx context.Context, sourceStorageKey, workStorageKey string) error {
	unlock := service.lockWorkFile(workStorageKey)
	defer unlock()
	path, err := service.localPath(workStorageKey)
	if err != nil {
		return err
	}
	reader, _, err := service.storage.Open(ctx, sourceStorageKey)
	if err != nil {
		return fmt.Errorf("读取对象存储文件失败: %w", err)
	}
	defer reader.Close()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建 SMB 工作目录失败: %w", err)
	}
	// 临时文件名带纳秒时间戳：并发同步同一文件时各写各的临时文件，
	// 避免互相覆盖产生损坏内容；最终以一次完整的原子替换落盘。
	temporaryPath := fmt.Sprintf("%s.%d.cadguanliq.tmp", path, time.Now().UnixNano())
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

// firstNonEmpty 返回第一个非空白字符串。
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
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
