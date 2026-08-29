//go:build windows

package smb

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"

	"cadguanliq/internal/config"
)

type ShareInfo struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Description string `json:"description"`
}

type Status struct {
	Enabled          bool        `json:"enabled"`
	ServerRunning    bool        `json:"serverRunning"`
	ShareName        string      `json:"shareName"`
	LocalRoot        string      `json:"localRoot"`
	LocalRootExists  bool        `json:"localRootExists"`
	UNCRoot          string      `json:"uncRoot"`
	UNCPathTemplate  string      `json:"uncPathTemplate"`
	ProtocolURL      string      `json:"protocolUrl"`
	ConfiguredExists bool        `json:"configuredShareExists"`
	Shares           []ShareInfo `json:"shares"`
	Error            string      `json:"error,omitempty"`
}

// Ensure checks the Windows SMB server and creates the configured CAD share
// when it does not exist. The process must have administrator privileges.
func Ensure(ctx context.Context, cfg config.SMBConfig) error {
	log.Printf("[SMB] 开始检查：enabled=%t host=%q share=%q localRoot=%q username=%q", cfg.Enabled, cfg.Host, cfg.Share, cfg.LocalRoot, cfg.Username)
	if !cfg.Enabled {
		log.Printf("[SMB] 已关闭：CAD_SMB_ENABLED=false，本次不会创建或检查 SMB 共享")
		return nil
	}
	if strings.TrimSpace(cfg.Share) == "" || strings.TrimSpace(cfg.LocalRoot) == "" {
		log.Printf("[SMB] 失败：配置不完整")
		return fmt.Errorf("SMB 配置不完整：CAD_SMB_SHARE 和 CAD_SMB_LOCAL_ROOT 不能为空")
	}

	root, err := filepath.Abs(cfg.LocalRoot)
	if err != nil {
		log.Printf("[SMB] 失败：解析工作目录失败 err=%v", err)
		return fmt.Errorf("解析 SMB 工作目录失败: %w", err)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		log.Printf("[SMB] 失败：创建工作目录失败 path=%q err=%v", root, err)
		return fmt.Errorf("创建 SMB 工作目录失败: %w", err)
	}
	log.Printf("[SMB] 工作目录已就绪：%q", root)
	if err := ensureServer(ctx); err != nil {
		log.Printf("[SMB] 失败：Windows SMB 服务初始化失败 err=%v", err)
		return err
	}

	exists, output, err := queryShare(ctx, cfg.Share)
	if err != nil {
		log.Printf("[SMB] 失败：检查共享失败 share=%q err=%v", cfg.Share, err)
		return err
	}
	if exists {
		if !sameWindowsPath(output, root) {
			log.Printf("[SMB] 失败：共享已存在但目录不匹配 share=%q expected=%q details=%q", cfg.Share, root, strings.TrimSpace(output))
			return fmt.Errorf("SMB 共享 %q 已存在，但目录不是 %q；请先修正该共享配置", cfg.Share, root)
		}
		log.Printf("[SMB] 成功：共享已存在且目录匹配 \\\\%s\\%s", cfg.Host, cfg.Share)
		return nil
	}
	log.Printf("[SMB] 共享不存在，准备创建：\\\\%s\\%s -> %q", cfg.Host, cfg.Share, root)

	args := []string{"share", cfg.Share + "=" + root}
	if principal := currentPrincipal(ctx); principal != "" {
		args = append(args, "/GRANT:"+principal+",FULL")
	}
	if output, err := run(ctx, "net", args...); err != nil {
		log.Printf("[SMB] 失败：创建共享失败 share=%q err=%v details=%q", cfg.Share, err, strings.TrimSpace(output))
		return fmt.Errorf("创建 SMB 共享 %q 失败（请使用管理员权限启动 Go 后端）: %w: %s", cfg.Share, err, strings.TrimSpace(output))
	}
	log.Printf("[SMB] 成功：共享已创建 \\\\%s\\%s -> %q", cfg.Host, cfg.Share, root)
	return nil
}

func Inspect(ctx context.Context, cfg config.SMBConfig) Status {
	root, _ := filepath.Abs(cfg.LocalRoot)
	status := Status{
		Enabled:         cfg.Enabled,
		ShareName:       cfg.Share,
		LocalRoot:       root,
		LocalRootExists: root != "" && directoryExists(root),
		UNCRoot:         uncRoot(cfg),
		UNCPathTemplate: uncRoot(cfg) + `\\{storageKey}`,
		ProtocolURL:     "cadguanliq://open?ticket={ticket}",
		Shares:          []ShareInfo{},
	}
	if !cfg.Enabled {
		status.Error = "CAD_SMB_ENABLED=false"
		return status
	}
	serverOutput, serverErr := run(ctx, "sc.exe", "query", "LanmanServer")
	status.ServerRunning = serverErr == nil && strings.Contains(strings.ToUpper(serverOutput), "RUNNING")
	shares, shareErr := listShares(ctx)
	status.Shares = shares
	for _, share := range shares {
		if strings.EqualFold(share.Name, cfg.Share) {
			status.ConfiguredExists = sameWindowsPath(share.Path, root)
			if !status.ConfiguredExists {
				status.Error = fmt.Sprintf("共享 %q 的目录不是 %q", cfg.Share, root)
			}
			break
		}
	}
	if serverErr != nil {
		status.Error = "LanmanServer 服务不可用"
	} else if shareErr != nil {
		status.Error = shareErr.Error()
	} else if !status.ConfiguredExists {
		status.Error = fmt.Sprintf("共享 %q 不存在", cfg.Share)
	}
	return status
}

func ensureServer(ctx context.Context) error {
	log.Printf("[SMB] 检查 Windows 服务：LanmanServer")
	output, err := run(ctx, "sc.exe", "query", "LanmanServer")
	if err != nil {
		return fmt.Errorf("检查 Windows SMB 服务 LanmanServer 失败（请使用管理员权限启动 Go 后端）: %w: %s", err, strings.TrimSpace(output))
	}
	if strings.Contains(strings.ToUpper(output), "RUNNING") {
		log.Printf("[SMB] 成功：LanmanServer 正在运行")
		return nil
	}
	log.Printf("[SMB] LanmanServer 未运行，准备设置自动启动并启动服务")
	if output, err = run(ctx, "sc.exe", "config", "LanmanServer", "start=", "auto"); err != nil {
		return fmt.Errorf("设置 Windows SMB 服务自动启动失败（请使用管理员权限启动 Go 后端）: %w: %s", err, strings.TrimSpace(output))
	}
	if output, err = run(ctx, "net", "start", "LanmanServer"); err != nil && !strings.Contains(strings.ToLower(output), "already") {
		return fmt.Errorf("启动 Windows SMB 服务失败（请使用管理员权限启动 Go 后端）: %w: %s", err, strings.TrimSpace(output))
	}
	log.Printf("[SMB] 成功：LanmanServer 已运行")
	return nil
}

func queryShare(ctx context.Context, share string) (bool, string, error) {
	log.Printf("[SMB] 检查共享：%q", share)
	output, err := run(ctx, "net", "share", share)
	if err == nil {
		log.Printf("[SMB] 检查结果：共享 %q 已存在", share)
		return true, output, nil
	}
	if _, ok := err.(*exec.ExitError); ok {
		log.Printf("[SMB] 检查结果：共享 %q 不存在，将自动创建", share)
		return false, output, nil
	}
	return false, output, fmt.Errorf("检查 SMB 共享 %q 失败: %w", share, err)
}

func listShares(ctx context.Context) ([]ShareInfo, error) {
	command := "$ErrorActionPreference='Stop'; ConvertTo-Json -InputObject @(Get-SmbShare | Select-Object Name,Path,Description) -Compress"
	output, err := run(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", command)
	if err != nil {
		return []ShareInfo{}, fmt.Errorf("读取 Windows SMB 共享列表失败: %w: %s", err, strings.TrimSpace(output))
	}
	var raw json.RawMessage
	if err := json.Unmarshal([]byte(output), &raw); err != nil {
		return []ShareInfo{}, fmt.Errorf("解析 Windows SMB 共享列表失败: %w", err)
	}
	var shares []ShareInfo
	if len(raw) > 0 && raw[0] == '{' {
		var share ShareInfo
		if err := json.Unmarshal(raw, &share); err != nil {
			return []ShareInfo{}, fmt.Errorf("解析 Windows SMB 共享对象失败: %w", err)
		}
		shares = []ShareInfo{share}
	} else if err := json.Unmarshal(raw, &shares); err != nil {
		return []ShareInfo{}, fmt.Errorf("解析 Windows SMB 共享数组失败: %w", err)
	}
	return shares, nil
}

func directoryExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func currentPrincipal(ctx context.Context) string {
	if output, err := run(ctx, "whoami"); err == nil {
		if principal := strings.TrimSpace(output); principal != "" {
			return principal
		}
	}
	if current, err := user.Current(); err == nil {
		return strings.TrimSpace(current.Username)
	}
	return ""
}

func sameWindowsPath(output, root string) bool {
	normalizedRoot := strings.ToLower(filepath.Clean(root))
	normalizedRoot = strings.ReplaceAll(normalizedRoot, "/", "\\")
	normalizedOutput := strings.ToLower(strings.ReplaceAll(output, "/", "\\"))
	return strings.Contains(normalizedOutput, normalizedRoot)
}

func uncRoot(cfg config.SMBConfig) string {
	if strings.TrimSpace(cfg.Host) == "" || strings.TrimSpace(cfg.Share) == "" {
		return ""
	}
	return `\\` + strings.Trim(cfg.Host, `\`) + `\` + strings.Trim(cfg.Share, `\`)
}

func run(ctx context.Context, name string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, name, args...)
	output, err := command.CombinedOutput()
	return string(output), err
}
