package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"runtime"
	"strings"
	"time"

	"cadguanliq/internal/auth"
	"cadguanliq/internal/config"
	"cadguanliq/internal/converter"
	"cadguanliq/internal/http/middleware"
	"cadguanliq/internal/response"
	"cadguanliq/internal/smb"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SystemStatusResponse struct {
	Service     ServiceStatusSnapshot  `json:"service"`
	Database    DatabaseStatusSnapshot `json:"database"`
	Storage     StorageStatusSnapshot  `json:"storage"`
	SMB         smb.Status             `json:"smb"`
	CAXA        CAXAStatusSnapshot     `json:"caxa"`
	OnlineUsers OnlineUsersSnapshot    `json:"onlineUsers"`
}

type CAXAStatusSnapshot struct {
	Available bool   `json:"available"`
	Path      string `json:"path,omitempty"`
	Error     string `json:"error,omitempty"`
}

type ServiceStatusSnapshot struct {
	Status    string `json:"status"`
	Version   string `json:"version"`
	Platform  string `json:"platform"`
	CheckedAt string `json:"checkedAt"`
}

type DatabaseStatusSnapshot struct {
	Status        string `json:"status"`
	TotalConns    int32  `json:"totalConns"`
	AcquiredConns int32  `json:"acquiredConns"`
	IdleConns     int32  `json:"idleConns"`
	MaxConns      int32  `json:"maxConns"`
	MinConns      int32  `json:"minConns"`
}

type StorageStatusSnapshot struct {
	Status        string `json:"status"`
	FileCount     int64  `json:"fileCount"`
	UsedBytes     int64  `json:"usedBytes"`
	FormattedUsed string `json:"formattedUsed"`
	FormattedDisk string `json:"formattedDisk"`
	BackupStatus  string `json:"backupStatus"`
	Source        string `json:"source"`
}

type OnlineUsersSnapshot struct {
	Count int64 `json:"count"`
}

func SystemStatus(pool *pgxpool.Pool, cfg config.Config, authService *auth.Service) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		ctx, cancel := context.WithTimeout(request.Context(), 3*time.Second)
		defer cancel()

		dbStatus := "ok"
		var totalConns, acquiredConns, idleConns int32
		if pool == nil {
			dbStatus = "not_configured"
		} else {
			stat := pool.Stat()
			totalConns = stat.TotalConns()
			acquiredConns = stat.AcquiredConns()
			idleConns = stat.IdleConns()
			if err := pool.Ping(ctx); err != nil {
				dbStatus = "error"
			}
		}

		var fileCount int64
		var usedBytes int64
		storageStatus := "not_configured"
		if pool != nil && dbStatus == "ok" {
			if err := pool.QueryRow(ctx, `
				SELECT count(*), COALESCE(sum(v.size_bytes), 0)
				FROM attachments a
				JOIN attachment_versions v ON v.id = a.current_version_id
				WHERE a.deleted_at IS NULL
				  AND v.deleted_at IS NULL`).Scan(&fileCount, &usedBytes); err != nil {
				storageStatus = "error"
			} else {
				storageStatus = "ok"
			}
		}
		diskTotal, diskFree := diskSpace(cfg.StorageRoot)

		var onlineCount int64
		if pool != nil {
			_ = pool.QueryRow(ctx, `
				SELECT count(DISTINCT user_id)
				FROM sessions
				WHERE revoked_at IS NULL AND expires_at > now()
				  AND last_seen_at > now() - interval '90 seconds'`).Scan(&onlineCount)
		}
		overallServiceStatus := "ok"
		if dbStatus == "error" {
			overallServiceStatus = "degraded"
		}

		data := SystemStatusResponse{
			Service: ServiceStatusSnapshot{
				Status:    overallServiceStatus,
				Version:   cfg.UpdateVersion,
				Platform:  runtime.GOOS + "-" + runtime.GOARCH,
				CheckedAt: time.Now().Format(time.RFC3339),
			},
			Database: DatabaseStatusSnapshot{
				Status:        dbStatus,
				TotalConns:    totalConns,
				AcquiredConns: acquiredConns,
				IdleConns:     idleConns,
				MaxConns:      cfg.Database.MaxConns,
				MinConns:      cfg.Database.MinConns,
			},
			Storage: StorageStatusSnapshot{
				Status:        storageStatus,
				FileCount:     fileCount,
				UsedBytes:     usedBytes,
				FormattedUsed: formatBytes(usedBytes),
				FormattedDisk: formatBytes(diskTotal) + " / 剩余 " + formatBytes(diskFree),
				BackupStatus:  "未配置",
				Source:        "database",
			},
			SMB:  smb.Inspect(ctx, cfg.SMB),
			CAXA: caxaStatus(cfg.CaxaBin),
			OnlineUsers: OnlineUsersSnapshot{
				Count: onlineCount,
			},
		}

		response.WriteData(writer, http.StatusOK, data)
	}
}

func caxaStatus(configured string) CAXAStatusSnapshot {
	path, err := converter.ResolveCaxaPath(configured)
	if err != nil {
		return CAXAStatusSnapshot{Error: err.Error()}
	}
	return CAXAStatusSnapshot{Available: true, Path: path}
}

type SMBAccessResponse struct {
	Host     string `json:"host"`
	Share    string `json:"share"`
	UNCRoot  string `json:"uncRoot"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// SMBAccess 下发 SMB 访问信息（需登录）。桌面客户端据此自动写入 Windows 凭据管理器，
// 免去每台电脑手动输入凭据；内网环境且与编辑票据凭据下发一致，不新增暴露面。
func SMBAccess(cfg config.Config) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		smbConfig := cfg.SMB
		uncRoot := ""
		if strings.TrimSpace(smbConfig.Host) != "" && strings.TrimSpace(smbConfig.Share) != "" {
			uncRoot = `\\` + strings.Trim(smbConfig.Host, `\`) + `\` + strings.Trim(smbConfig.Share, `\`)
		}
		response.WriteData(writer, http.StatusOK, SMBAccessResponse{
			Host:     smbConfig.Host,
			Share:    smbConfig.Share,
			UNCRoot:  uncRoot,
			Username: smbConfig.Username,
			Password: smbConfig.Password,
		})
	}
}

func Heartbeat(authService *auth.Service) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if authService == nil {
			response.WriteError(writer, http.StatusServiceUnavailable, "认证服务未配置")
			return
		}
		if err := authService.Heartbeat(request.Context(), middleware.BearerToken(request.Header.Get("Authorization"))); err != nil {
			log.Printf("[认证] 心跳失败: %v", err)
			writeAuthError(writer, err)
			return
		}
		response.WriteData(writer, http.StatusOK, map[string]any{
			"timestamp": time.Now().Unix(),
			"status":    "alive",
		})
	}
}

func formatBytes(bytes int64) string {
	if bytes <= 0 {
		return "0 B"
	}
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	units := []string{"KB", "MB", "GB", "TB"}
	val := float64(bytes)
	unitIndex := -1
	for val >= 1024 && unitIndex < len(units)-1 {
		val /= 1024
		unitIndex++
	}
	if unitIndex < 0 {
		return fmt.Sprintf("%d B", bytes)
	}
	return fmt.Sprintf("%.1f %s", val, units[unitIndex])
}

func diskSpace(root string) (total int64, free int64) {
	// 当前先返回逻辑附件统计，Windows 磁盘容量读取后续通过平台适配实现。
	return 0, 0
}
