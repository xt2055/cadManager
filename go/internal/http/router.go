package httpapi

import (
	"context"
	"net/http"
	"os"
	"path/filepath"

	"cadguanliq/internal/annotation"
	"cadguanliq/internal/attachment"
	"cadguanliq/internal/audit"
	"cadguanliq/internal/auth"
	"cadguanliq/internal/change"
	"cadguanliq/internal/config"
	"cadguanliq/internal/converter"
	"cadguanliq/internal/drawing"
	"cadguanliq/internal/editing"
	"cadguanliq/internal/http/handlers"
	"cadguanliq/internal/http/middleware"
	"cadguanliq/internal/notification"
	"cadguanliq/internal/partindex"
	"cadguanliq/internal/review"
	"cadguanliq/internal/storage"
	"cadguanliq/internal/task"
	"cadguanliq/internal/titleblock"
	"cadguanliq/internal/update"
	"cadguanliq/internal/upload"
	"cadguanliq/internal/versioning"
	"github.com/jackc/pgx/v5/pgxpool"
)

func NewRouter(cfg config.Config, pool *pgxpool.Pool, authService *auth.Service) http.Handler {
	mux := http.NewServeMux()
	notificationHub := notification.NewHub()
	if pool != nil {
		go notificationHub.Run(context.Background(), pool)
	}
	mux.Handle("/api/notifications/ws", notificationHub.Handler(authService, cfg.AllowedOrigins))
	mux.Handle("/api/health", handlers.Health(pool, cfg.Database))
	mux.HandleFunc("/api/auth/login", handlers.Login(authService))
	mux.HandleFunc("/api/auth/me", handlers.Me(authService))
	mux.HandleFunc("/api/auth/logout", handlers.Logout(authService))
	protectedUsers := middleware.RequireAuth(authService)
	mux.Handle("/api/notifications", protectedUsers(handlers.Notifications(pool)))
	mux.Handle("/api/notifications/", protectedUsers(handlers.Notifications(pool)))
	mux.Handle("/api/users", protectedUsers(handlers.Users(authService)))
	mux.Handle("/api/users/reviewers", protectedUsers(handlers.Reviewers(authService)))
	mux.Handle("/api/users/", protectedUsers(handlers.UserResource(authService)))
	mux.Handle("/api/system/status", protectedUsers(handlers.SystemStatus(pool, cfg, authService)))
	mux.Handle("/api/system/smb-access", protectedUsers(handlers.SMBAccess(cfg)))
	mux.Handle("/api/auth/heartbeat", protectedUsers(handlers.Heartbeat(authService)))
	reviewRepository := review.NewPGRepository(pool)
	annotationRepository := annotation.NewRepository(pool)
	mux.Handle("/api/review-annotations", protectedUsers(handlers.AnnotationWorkspace(annotationRepository)))
	mux.Handle("/api/review-annotation-templates", protectedUsers(handlers.AnnotationTemplates(annotationRepository)))
	mux.Handle("/api/review-annotation-files", protectedUsers(handlers.AnnotationFiles(annotationRepository)))
	mux.Handle("/api/review-annotation-history", protectedUsers(handlers.AnnotationHistory(annotationRepository)))
	mux.Handle("/api/review-flows", protectedUsers(handlers.ReviewFlows(reviewRepository)))
	mux.Handle("/api/review-flows/", protectedUsers(handlers.ReviewFlowResource(reviewRepository)))
	mux.Handle("/api/review-cases", protectedUsers(handlers.ReviewCases(reviewRepository)))
	mux.Handle("/api/review-cases/", protectedUsers(handlers.ReviewCaseResource(reviewRepository)))
	changeService := change.NewService(pool)
	reviewRepository.SetChangeCompletion(changeService.CompleteReview)
	mux.Handle("/api/change-requests", protectedUsers(handlers.ChangeRequests(changeService)))
	mux.Handle("/api/change-requests/", protectedUsers(handlers.ChangeRequestResource(changeService)))
	attachmentRepository := attachment.NewPGRepository(pool)
	drawingRepository := drawing.NewPGRepository(pool)
	partIndexRepository := partindex.NewRepository(pool)
	drawingHandler := middleware.RequireAuth(authService)
	mux.Handle("/api/drawings", drawingHandler(handlers.Drawings(drawingRepository)))
	mux.Handle("/api/drawings/", drawingHandler(handlers.DrawingResource(drawingRepository)))
	mux.Handle("/api/parts/", drawingHandler(handlers.PartResource(drawingRepository)))
	mux.Handle("/api/drawing-part-relations/", drawingHandler(handlers.DrawingPartRelationResource(drawingRepository)))
	mux.Handle("/api/part-revisions/", drawingHandler(handlers.PartRevisionResource(drawingRepository)))
	mux.Handle("/api/drawing-attributes", drawingHandler(handlers.DrawingAttributes(pool)))
	mux.Handle("/api/drawing-attributes/", drawingHandler(handlers.DrawingAttributes(pool)))
	mux.Handle("/api/drawing-relations/", drawingHandler(handlers.DrawingRelations(pool)))
	attachmentStorage, storageErr := storage.NewLocalStorage(cfg.StorageRoot)
	if storageErr != nil {
		panic(storageErr)
	}
	documentStorage, documentStorageErr := storage.NewLocalStorage(cfg.StorageRoot + "-documents")
	if documentStorageErr != nil {
		panic(documentStorageErr)
	}
	// 图纸任务：计划员把图纸指派给负责人；负责人由此获得原创建人的控制权。
	// candidates 必须显式注册，否则会被 /api/drawing-tasks/ 前缀路由吞掉。
	taskService := task.NewService(pool)
	mux.Handle("/api/drawing-tasks", protectedUsers(handlers.DrawingTasks(taskService)))
	mux.Handle("/api/drawing-tasks/candidates", protectedUsers(handlers.DrawingTaskCandidates(taskService)))
	mux.Handle("/api/drawing-tasks/", protectedUsers(handlers.DrawingTaskResource(taskService)))
	mux.Handle("/api/lifecycle-documents", protectedUsers(handlers.LifecycleDocuments(pool, documentStorage)))
	mux.Handle("/api/lifecycle-documents/", protectedUsers(handlers.LifecycleDocuments(pool, documentStorage)))
	mux.Handle("/api/patents", protectedUsers(handlers.Patents(pool)))
	mux.Handle("/api/patents/", protectedUsers(handlers.Patents(pool)))
	handlers.StartPatentReminders(context.Background(), pool)
	mux.Handle("/api/lifecycle-tree", protectedUsers(handlers.LifecycleTree(pool)))
	mux.Handle("/api/lifecycle-versions/", protectedUsers(handlers.LifecycleVersion(pool, attachmentStorage)))
	convService := converter.NewService(attachmentRepository, attachmentStorage, cfg.Dwg2DxfBin, cfg.CaxaBin)
	convService.Start(context.Background())
	versionRepository := versioning.NewPGRepository(pool)
	versionService := versioning.NewService(versionRepository, attachmentRepository, attachmentStorage)
	versionService.SetDrawingStatus(attachmentArchivedChecker{pool: pool})
	uploadService := upload.NewService(pool, attachmentStorage, convService, cfg.UploadSessionTTL)
	uploadService.StartConversionQueue(context.Background())
	// 上传清理器将在 Phase 3 迁移到最终附件模型后启用；当前旧服务仍依赖旧附件字段。
	// 版本捕获统一由「结束编辑」显式触发：SMB 工作文件稳定等待 + 哈希比对 + 事务切指针，
	// 不再用后台 watcher 扫描文件变化自动捕获，避免与关闭流程竞争产生重复版本。
	editingService := editing.NewService(editing.NewPGRepository(pool), attachmentRepository, attachmentStorage, convService, cfg.SMB)
	editingService.SetVersioning(versionService)
	editingService.SetPolicy(drawingRepository, reviewRepository)
	editingService.SetChangeGate(changeService)
	editingService.StartCleanup(context.Background())
	mux.Handle("/api/edit-sessions", drawingHandler(handlers.EditSession(editingService)))
	mux.Handle("/api/edit-sessions/open", drawingHandler(handlers.EditSession(editingService)))
	mux.Handle("/api/edit-tickets/exchange", drawingHandler(handlers.EditTicketExchange(editingService)))
	mux.Handle("/api/edit-sessions/", drawingHandler(handlers.EditSessionResource(editingService)))
	mux.Handle("/api/editing/read-only", drawingHandler(handlers.EditReadOnly(editingService)))
	mux.Handle("/api/editing/online-open", drawingHandler(handlers.OnlineEditOpen(editingService)))
	mux.Handle("/api/editing/online-save", drawingHandler(handlers.OnlineEditSave(editingService, cfg.MaxUploadBytes)))
	mux.Handle("/api/file-versions", drawingHandler(versioning.List(versionService)))
	mux.Handle("/api/file-versions/", drawingHandler(versioning.Resource(versionService, convService)))

	mux.Handle("/api/attachments/", drawingHandler(handlers.AttachmentResource(pool, attachmentRepository, attachmentStorage)))
	mux.Handle("/api/attachments", drawingHandler(handlers.Attachments(attachmentRepository)))
	mux.Handle("/api/upload-sessions", drawingHandler(handlers.UploadSessions(uploadService)))
	mux.Handle("/api/upload-sessions/hash-check", drawingHandler(handlers.UploadSessionHashCheck(uploadService)))
	mux.Handle("/api/upload-sessions/", drawingHandler(handlers.UploadSessionResource(uploadService)))
	mux.Handle("/api/bom/export", drawingHandler(handlers.ExportBOM(attachmentRepository, attachmentStorage)))
	mux.Handle("/api/exb/identify", drawingHandler(handlers.IdentifyDrawingFile(convService)))
	mux.Handle("/api/exb/material", drawingHandler(handlers.IdentifyDrawingMaterial(convService)))
	mux.Handle("/api/exb/reidentify", drawingHandler(handlers.ReidentifyPart(attachmentRepository)))
	mux.Handle("/api/exb/designer", drawingHandler(handlers.ScanDrawingDesigner(attachmentRepository, attachmentStorage)))
	mux.Handle("/api/exb/preview", drawingHandler(handlers.PreviewEXB(attachmentRepository, attachmentStorage, convService)))
	mux.Handle("/api/exb/convert", drawingHandler(handlers.ConvertToEXB(attachmentRepository, attachmentStorage, convService)))
	mux.Handle("/api/cad/source", drawingHandler(handlers.CADSource(attachmentRepository, attachmentStorage, convService)))
	mux.Handle("/api/cad/conversions/", drawingHandler(handlers.ConversionStatus(pool)))
	mux.Handle("/api/cad/title-blocks/", drawingHandler(handlers.TitleBlocks(titleblock.NewRepository(pool))))
	mux.Handle("/api/part-indexes", drawingHandler(handlers.PartIndexes(partIndexRepository)))
	mux.Handle("/api/part-indexes/", drawingHandler(handlers.PartIndexes(partIndexRepository)))
	mux.Handle("/api/cad/convert-dwg", drawingHandler(handlers.ConvertDxfToDwg(convService, cfg.MaxUploadBytes)))
	auditRepository := audit.NewPGRepository(pool)
	mux.Handle("/api/drawing-operation-logs", drawingHandler(handlers.DrawingOperationLogs(auditRepository)))
	mux.Handle("/api/drawing-operation-logs/options", drawingHandler(handlers.AuditLogOptions(auditRepository, false)))

	// 管理端接口（admin 角色）：日志查看与更新管理
	adminGuard := func(next http.Handler) http.Handler {
		return protectedUsers(middleware.RequireAdmin(next))
	}
	mux.Handle("/api/admin/audit-logs", adminGuard(handlers.AdminOperationLogs(auditRepository)))
	mux.Handle("/api/admin/notifications", adminGuard(handlers.SendNotifications(pool)))
	mux.Handle("/api/admin/audit-logs/options", adminGuard(handlers.AuditLogOptions(auditRepository, true)))
	mux.Handle("/api/admin/upload-reconciliation", adminGuard(handlers.UploadReconciliation(uploadService)))
	mux.Handle("/api/admin/drawings", adminGuard(handlers.AdminDrawings(pool)))
	mux.Handle("/api/admin/drawings/", adminGuard(handlers.AdminDrawingResource(pool, attachmentStorage, auditRepository)))
	mux.Handle("/api/admin/parts/", adminGuard(handlers.AdminPartResource(pool, attachmentStorage, auditRepository)))
	mux.Handle("/api/admin/attachments/", adminGuard(handlers.AdminAttachmentResource(pool, attachmentStorage, auditRepository)))
	mux.Handle("/api/admin/edit-sessions/", adminGuard(handlers.AdminEditSessionResource(editingService, auditRepository)))
	mux.Handle("/api/admin/cad-conversions", adminGuard(handlers.AdminConversionJobs(pool)))
	mux.Handle("/api/admin/cad-conversions/", adminGuard(handlers.AdminRetryConversionJob(pool)))
	mux.Handle("/api/admin/cad-conversion-logs", adminGuard(handlers.AdminConversionLogs()))
	mux.Handle("/api/admin/part-indexes/backfill", adminGuard(handlers.PartIndexBackfill(partIndexRepository)))
	mux.Handle("/api/system/logs", adminGuard(handlers.SystemLogs()))
	mux.Handle("/api/system/logs/files", adminGuard(handlers.SystemLogFiles()))
	mux.Handle("/api/system/logs/download", adminGuard(handlers.SystemLogDownload()))
	updateStore := update.NewStore(pool)
	mux.Handle("/api/system/updates", adminGuard(handlers.UpdateManagement(updateStore, cfg.UpdatesDir)))
	mux.Handle("/api/system/updates/", adminGuard(handlers.UpdateResource(updateStore, cfg.UpdatesDir)))
	mux.HandleFunc("/api/updates/latest", handlers.UpdateLatest(updateStore, cfg))
	mux.Handle("/api/updates/", protectedUsers(handlers.UpdateDownload(updateStore, cfg.UpdatesDir)))

	// CAD 字体资源（SHX/WOFF，前端 viewer 渲染文字用）：免鉴权，目录锚定 exe 所在目录
	const fontsPrefix = "/cad-data/fonts/"
	if exePath, err := os.Executable(); err == nil {
		fontDir := filepath.Join(filepath.Dir(exePath), "cad-data", "fonts")
		if _, err := os.Stat(fontDir); err == nil {
			mux.Handle(fontsPrefix, http.StripPrefix(fontsPrefix, http.FileServer(http.Dir(fontDir))))
		}
	}

	var handler http.Handler = mux
	handler = middleware.Recovery(handler)
	handler = middleware.Logging(handler)
	handler = middleware.CORS(cfg.AllowedOrigins)(handler)
	return handler
}

// attachmentArchivedChecker 判断附件所属图纸是否处于存档保护状态，
// 供版本发布/回滚入口拦截对存档正式成果的直接改写。
type attachmentArchivedChecker struct{ pool *pgxpool.Pool }

func (c attachmentArchivedChecker) ArchivedByAttachment(ctx context.Context, attachmentID string) (bool, error) {
	var archived bool
	err := c.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM attachments a
			JOIN drawings d ON d.id = a.drawing_id
			WHERE a.id = $1::uuid AND d.status = 'archived'
		)`, attachmentID).Scan(&archived)
	return archived, err
}
