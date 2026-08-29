//go:build !windows

package smb

import (
	"context"
	"errors"
	"log"

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

func Ensure(_ context.Context, cfg config.SMBConfig) error {
	log.Printf("[SMB] 检查：当前操作系统不是 Windows，enabled=%t", cfg.Enabled)
	if cfg.Enabled {
		log.Printf("[SMB] 失败：当前操作系统不支持自动创建 Windows SMB 共享")
		return errors.New("当前操作系统不支持自动创建 Windows SMB 共享")
	}
	log.Printf("[SMB] 已关闭：非 Windows 平台且 SMB 未启用")
	return nil
}

func Inspect(_ context.Context, cfg config.SMBConfig) Status {
	return Status{
		Enabled:         cfg.Enabled,
		ShareName:       cfg.Share,
		LocalRoot:       cfg.LocalRoot,
		UNCRoot:         "",
		UNCPathTemplate: "",
		ProtocolURL:     "cadguanliq://open?ticket={ticket}",
		Shares:          []ShareInfo{},
		Error:           "当前操作系统不是 Windows",
	}
}
