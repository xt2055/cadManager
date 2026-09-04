package app

import (
	"context"
	"net/http"
	"os"

	"cadguanliq/internal/attachment"
	"cadguanliq/internal/auth"
	"cadguanliq/internal/config"
	"cadguanliq/internal/data"
	httpapi "cadguanliq/internal/http"
	"cadguanliq/internal/logging"
	"cadguanliq/internal/setup"
	"cadguanliq/internal/smb"
	"cadguanliq/internal/storage"
	"cadguanliq/internal/upload"
	"cadguanliq/internal/versioning"
	"github.com/jackc/pgx/v5/pgxpool"
)

func NewHandler(cfg config.Config, pool *pgxpool.Pool, authService *auth.Service) http.Handler {
	return httpapi.NewRouter(cfg, pool, authService)
}

func Run(cfg config.Config) error {
	if err := logging.Setup(cfg.LogDir); err != nil {
		return err
	}
	// 首次运行（无 .env）进入安装向导，完成后自动重启进入正常模式
	if _, statErr := os.Stat(".env"); os.IsNotExist(statErr) {
		return setup.Run(cfg)
	}
	logging.Infof("[启动] cadguanliq 服务初始化开始，日志目录 %s", cfg.LogDir)
	if err := smb.Ensure(context.Background(), cfg.SMB); err != nil {
		return err
	}
	logging.Infof("[SMB] 启动初始化完成，进入数据库和 HTTP 服务初始化")
	pool, err := data.NewPool(context.Background(), cfg.Database)
	if err != nil {
		return err
	}
	defer pool.Close()
	authService := auth.NewService(auth.NewPGRepository(pool))
	// 增量字段尝试由应用补全（如果连接账号为表属主或有 DDL 权限；若无权限则跳过由管理员迁移脚本维护）
	if _, migErr := pool.Exec(context.Background(), `
		DO $$
		BEGIN
		    BEGIN
		        ALTER TABLE attachments
		            ADD COLUMN IF NOT EXISTS current_storage_key VARCHAR(500),
		            ADD COLUMN IF NOT EXISTS current_name VARCHAR(255),
		            ADD COLUMN IF NOT EXISTS current_mime_type VARCHAR(255),
		            ADD COLUMN IF NOT EXISTS current_size_bytes BIGINT,
		            ADD COLUMN IF NOT EXISTS current_sha256 CHAR(64);
		    EXCEPTION
		        WHEN insufficient_privilege THEN
		            NULL;
		    END;
		END $$;
		UPDATE attachments
		SET current_storage_key = COALESCE(current_storage_key, storage_key),
		    current_name = COALESCE(current_name, original_name),
		    current_mime_type = COALESCE(current_mime_type, mime_type),
		    current_size_bytes = COALESCE(current_size_bytes, size_bytes),
		    current_sha256 = COALESCE(current_sha256, sha256)
		WHERE current_storage_key IS NULL;
	`); migErr != nil {
		logging.Warnf("[DB Migration] 增量数据同步检查略过: %v", migErr)
	} else {
		logging.Infof("[DB Migration] attachments 表增量字段迁移检查完成")
	}
	if _, migErr := pool.Exec(context.Background(), `
			ALTER TABLE update_manifests ADD COLUMN IF NOT EXISTS file_name VARCHAR(255) NOT NULL DEFAULT '';
			ALTER TABLE update_manifests ADD COLUMN IF NOT EXISTS size_bytes BIGINT NOT NULL DEFAULT 0;
		`); migErr != nil {
		logging.Warnf("[DB Migration] update_manifests 增量字段检查略过: %v", migErr)
	}
	if err := ensureUploadSchema(context.Background(), pool); err != nil {
		return err
	}
	logging.Infof("[DB Migration] 上传模块数据库结构检查完成")
	attachmentRepository := attachment.NewPGRepository(pool)
	attachmentStorage, err := storage.NewLocalStorage(cfg.StorageRoot)
	if err != nil {
		return err
	}
	versionService := versioning.NewService(versioning.NewPGRepository(pool), attachmentRepository, attachmentStorage)
	versionService.StartCleanup(context.Background())
	// 将已有附件/版本按既有 SHA-256 回填到内容对象表，避免历史数据永远绕过去重链路。
	legacyUploadService := upload.NewService(pool, attachmentStorage, nil, cfg.UploadSessionTTL)
	if err := legacyUploadService.BackfillLegacyBlobs(context.Background()); err != nil {
		logging.Warnf("[Upload] 历史附件内容对象回填略过: %v", err)
	}

	logging.Infof("[启动] 服务监听 %s", cfg.Addr)
	return http.ListenAndServe(cfg.Addr, NewHandler(cfg, pool, authService))
}
