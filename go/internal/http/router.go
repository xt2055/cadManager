package httpapi

import (
	"context"
	"net/http"

	"cadguanliq/internal/attachment"
	"cadguanliq/internal/audit"
	"cadguanliq/internal/auth"
	"cadguanliq/internal/config"
	"cadguanliq/internal/converter"
	"cadguanliq/internal/data"
	"cadguanliq/internal/drawing"
	"cadguanliq/internal/http/handlers"
	"cadguanliq/internal/http/middleware"
	"cadguanliq/internal/review"
	"cadguanliq/internal/storage"
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
	dataHandler := middleware.RequireAuth(authService)(handlers.DataDocument(data.NewDocumentRepository(pool)))
	mux.Handle("/api/data/document", dataHandler)
	drawingRepository := drawing.NewPGRepository(pool)
	drawingHandler := middleware.RequireAuth(authService)
	mux.Handle("/api/drawings", drawingHandler(handlers.Drawings(drawingRepository)))
	mux.Handle("/api/drawings/", drawingHandler(handlers.DrawingResource(drawingRepository)))
	mux.Handle("/api/parts/", drawingHandler(handlers.PartResource(drawingRepository)))
	attachmentRepository := attachment.NewPGRepository(pool)
	attachmentStorage, storageErr := storage.NewLocalStorage(cfg.StorageRoot)
	if storageErr != nil {
		panic(storageErr)
	}
	convService := converter.NewService(attachmentRepository, attachmentStorage, cfg.Dwg2DxfBin, cfg.CaxaBin)
	convService.Start(context.Background())

	mux.Handle("/api/attachments", drawingHandler(handlers.UploadAttachment(attachmentRepository, attachmentStorage, convService, cfg.MaxUploadBytes)))
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
	auditRepository := audit.NewPGRepository(pool)
	mux.Handle("/api/drawing-operation-logs", drawingHandler(handlers.DrawingOperationLogs(auditRepository)))
	mux.HandleFunc("/api/updates/latest", handlers.UpdateLatest(cfg))

	var handler http.Handler = mux
	handler = middleware.Recovery(handler)
	handler = middleware.Logging(handler)
	handler = middleware.CORS(cfg.AllowedOrigins)(handler)
	return handler
}
