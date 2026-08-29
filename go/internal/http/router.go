package httpapi

import (
	"context"
	"net/http"
	"time"

	"cadguanliq/internal/attachment"
	"cadguanliq/internal/audit"
	"cadguanliq/internal/auth"
	"cadguanliq/internal/config"
	"cadguanliq/internal/converter"
	"cadguanliq/internal/data"
	"cadguanliq/internal/drawing"
	"cadguanliq/internal/editing"
	"cadguanliq/internal/http/handlers"
	"cadguanliq/internal/http/middleware"
	"cadguanliq/internal/review"
	"cadguanliq/internal/storage"
	"cadguanliq/internal/update"
	"cadguanliq/internal/versioning"
	"github.com/jackc/pgx/v5/pgxpool"
)

func NewRouter(cfg config.Config, pool *pgxpool.Pool, authService *auth.Service) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/api/health", handlers.Health(pool, cfg.Database))
	mux.HandleFunc("/api/auth/login", handlers.Login(authService))
	mux.HandleFunc("/api/auth/me", handlers.Me(authService))
	mux.HandleFunc("/api/auth/logout", handlers.Logout(authService))
	protectedUsers := middleware.RequireAuth(authService)
	mux.Handle("/api/users", protectedUsers(handlers.Users(authService)))
	mux.Handle("/api/users/reviewers", protectedUsers(handlers.Reviewers(authService)))
	mux.Handle("/api/users/", protectedUsers(handlers.UserResource(authService)))
	mux.Handle("/api/system/status", protectedUsers(handlers.SystemStatus(pool, cfg, authService)))
	mux.Handle("/api/auth/heartbeat", protectedUsers(handlers.Heartbeat(authService)))
	reviewRepository := review.NewPGRepository(pool)
	mux.Handle("/api/review-flows", protectedUsers(handlers.ReviewFlows(reviewRepository)))
	mux.Handle("/api/review-flows/", protectedUsers(handlers.ReviewFlowResource(reviewRepository)))
	attachmentRepository := attachment.NewPGRepository(pool)
	dataHandler := middleware.RequireAuth(authService)(handlers.DataDocument(data.NewDocumentRepository(pool), attachmentRepository))
	mux.Handle("/api/data/document", dataHandler)
	drawingRepository := drawing.NewPGRepository(pool)
	drawingHandler := middleware.RequireAuth(authService)
	mux.Handle("/api/drawings", drawingHandler(handlers.Drawings(drawingRepository)))
	mux.Handle("/api/drawings/", drawingHandler(handlers.DrawingResource(drawingRepository)))
	mux.Handle("/api/parts/", drawingHandler(handlers.PartResource(drawingRepository)))
	attachmentStorage, storageErr := storage.NewLocalStorage(cfg.StorageRoot)
	if storageErr != nil {
		panic(storageErr)
	}
	convService := converter.NewService(attachmentRepository, attachmentStorage, cfg.Dwg2DxfBin, cfg.CaxaBin)
	convService.Start(context.Background())
	versionRepository := versioning.NewPGRepository(pool)
	versionService := versioning.NewService(versionRepository, attachmentRepository, attachmentStorage)
	versionWatcher := versioning.NewWatcher(attachmentRepository, attachmentStorage, versionService, cfg.SMB.LocalRoot, 10*time.Second)
	versionWatcher.Start(context.Background())
	editingService := editing.NewService(editing.NewPGRepository(pool), attachmentRepository, attachmentStorage, convService, cfg.SMB)
	editingService.SetVersioning(versionService)
	editingService.StartCleanup(context.Background())
	mux.Handle("/api/edit-sessions", drawingHandler(handlers.EditSession(editingService)))
	mux.Handle("/api/edit-sessions/open", drawingHandler(handlers.EditSession(editingService)))
	mux.Handle("/api/edit-tickets/exchange", drawingHandler(handlers.EditTicketExchange(editingService)))
	mux.Handle("/api/edit-sessions/", drawingHandler(handlers.EditSessionResource(editingService)))
	mux.Handle("/api/file-versions", drawingHandler(versioning.List(versionService)))
	mux.Handle("/api/file-versions/", drawingHandler(versioning.Resource(versionService)))

	mux.Handle("/api/attachments", drawingHandler(handlers.UploadAttachment(attachmentRepository, attachmentStorage, convService, versionService, cfg.MaxUploadBytes)))
	mux.Handle("/api/attachments/", drawingHandler(handlers.AttachmentResource(attachmentRepository, attachmentStorage)))
	mux.Handle("/api/bom/export", drawingHandler(handlers.ExportBOM(attachmentRepository, attachmentStorage)))
	mux.Handle("/api/exb/parse", drawingHandler(handlers.ParseEXB(attachmentRepository, attachmentStorage)))
	mux.Handle("/api/exb/identify", drawingHandler(handlers.IdentifyDrawingFile(convService)))
	mux.Handle("/api/exb/material", drawingHandler(handlers.IdentifyDrawingMaterial(convService)))
	mux.Handle("/api/exb/reidentify", drawingHandler(handlers.ReidentifyPart(attachmentRepository)))
	mux.Handle("/api/exb/designer", drawingHandler(handlers.ScanDrawingDesigner(attachmentRepository, attachmentStorage)))
	mux.Handle("/api/exb/preview", drawingHandler(handlers.PreviewEXB(attachmentRepository, attachmentStorage, convService)))
	mux.Handle("/api/exb/convert", drawingHandler(handlers.ConvertToEXB(attachmentRepository, attachmentStorage, convService)))
	mux.Handle("/api/cad/source", drawingHandler(handlers.CADSource(attachmentRepository, attachmentStorage, convService)))
	mux.Handle("/api/cad/convert-dwg", drawingHandler(handlers.ConvertDxfToDwg(convService, cfg.MaxUploadBytes)))
	auditRepository := audit.NewPGRepository(pool)
	mux.Handle("/api/drawing-operation-logs", drawingHandler(handlers.DrawingOperationLogs(auditRepository)))

	// 管理端接口（admin 角色）：日志查看与更新管理
	adminGuard := func(next http.Handler) http.Handler {
		return protectedUsers(middleware.RequireAdmin(next))
	}
	mux.Handle("/api/system/logs", adminGuard(handlers.SystemLogs()))
	mux.Handle("/api/system/logs/files", adminGuard(handlers.SystemLogFiles()))
	mux.Handle("/api/system/logs/download", adminGuard(handlers.SystemLogDownload()))
	updateStore := update.NewStore(pool)
	mux.Handle("/api/system/updates", adminGuard(handlers.UpdateManagement(updateStore, cfg.UpdatesDir)))
	mux.Handle("/api/system/updates/", adminGuard(handlers.UpdateResource(updateStore, cfg.UpdatesDir)))
	mux.HandleFunc("/api/updates/latest", handlers.UpdateLatest(updateStore, cfg))
	mux.Handle("/api/updates/", protectedUsers(handlers.UpdateDownload(updateStore, cfg.UpdatesDir)))

	var handler http.Handler = mux
	handler = middleware.Recovery(handler)
	handler = middleware.Logging(handler)
	handler = middleware.CORS(cfg.AllowedOrigins)(handler)
	return handler
}
