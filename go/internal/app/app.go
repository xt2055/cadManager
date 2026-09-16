package app

import (
	"context"
	"net/http"
	"os"
	"time"

	"cadguanliq/internal/auth"
	"cadguanliq/internal/config"
	"cadguanliq/internal/data"
	httpapi "cadguanliq/internal/http"
	"cadguanliq/internal/logging"
	"cadguanliq/internal/review"
	"cadguanliq/internal/setup"
	"cadguanliq/internal/smb"
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
	upgradeCtx, cancelUpgrade := context.WithTimeout(context.Background(), 2*time.Minute)
	err = data.EnsurePartIndexSchema(upgradeCtx, pool)
	cancelUpgrade()
	if err != nil {
		return err
	}
	logging.Infof("[数据库] 标题栏和零件索引结构检查完成")
	// 补齐正在等签的审核提醒：责任人离线期间推进的节点、以及本改动上线前已存在的待办。
	remindCtx, cancelRemind := context.WithTimeout(context.Background(), 30*time.Second)
	remindErr := review.EnsureTurnNotifications(remindCtx, pool)
	cancelRemind()
	if remindErr != nil {
		logging.Errorf("[通知] 补期待办审核提醒失败：%v", remindErr)
	} else {
		logging.Infof("[通知] 待办审核提醒补查完成")
	}
	authService := auth.NewService(auth.NewPGRepository(pool))
	logging.Infof("[启动] 服务监听 %s", cfg.Addr)
	return http.ListenAndServe(cfg.Addr, NewHandler(cfg, pool, authService))
}
