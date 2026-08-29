package handlers

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cadguanliq/internal/logging"
	"cadguanliq/internal/response"
	"cadguanliq/internal/update"
)

const updateUploadMaxBytes = 1024 * 1024 * 1024

func safeUpdateFileName(version string, original string) string {
	base := filepath.Base(original)
	base = strings.Map(func(r rune) rune {
		if r == '\\' || r == '/' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
			return '_'
		}
		return r
	}, base)
	if base == "" || base == "." {
		base = "update.exe"
	}
	return fmt.Sprintf("%s_%s", version, base)
}

// UpdateManagement 更新版本管理（admin）：GET 列表 / POST 上传。
func UpdateManagement(store *update.Store, dir string) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		switch request.Method {
		case http.MethodGet:
			records, err := store.List(request.Context())
			if err != nil {
				response.WriteError(writer, http.StatusInternalServerError, "读取更新列表失败: "+err.Error())
				return
			}
			response.WriteData(writer, http.StatusOK, records)
		case http.MethodPost:
			uploadUpdate(writer, request, store, dir)
		default:
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

func uploadUpdate(writer http.ResponseWriter, request *http.Request, store *update.Store, dir string) {
	request.Body = http.MaxBytesReader(writer, request.Body, updateUploadMaxBytes)
	if err := request.ParseMultipartForm(32 << 20); err != nil {
		response.WriteError(writer, http.StatusBadRequest, "上传内容过大或格式错误（最大 1GB）")
		return
	}
	version := strings.TrimSpace(request.FormValue("version"))
	if version == "" {
		response.WriteError(writer, http.StatusBadRequest, "缺少版本号")
		return
	}
	platform := strings.TrimSpace(request.FormValue("platform"))
	if platform == "" {
		platform = "windows-x86_64"
	}
	file, header, err := request.FormFile("file")
	if err != nil {
		response.WriteError(writer, http.StatusBadRequest, "缺少安装包文件")
		return
	}
	defer file.Close()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		response.WriteError(writer, http.StatusInternalServerError, "创建更新目录失败: "+err.Error())
		return
	}
	fileName := safeUpdateFileName(version, header.Filename)
	targetPath := filepath.Join(dir, fileName)
	target, err := os.Create(targetPath)
	if err != nil {
		response.WriteError(writer, http.StatusInternalServerError, "写入安装包失败: "+err.Error())
		return
	}
	size, copyErr := io.Copy(target, file)
	closeErr := target.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(targetPath)
		response.WriteError(writer, http.StatusInternalServerError, "保存安装包失败")
		return
	}

	record, err := store.Create(request.Context(), update.Record{
		Version:     version,
		Platform:    platform,
		Notes:       request.FormValue("notes"),
		Mandatory:   request.FormValue("mandatory") == "true" || request.FormValue("mandatory") == "1",
		FileName:    fileName,
		SizeBytes:   size,
		PublishedAt: time.Now().Format("2006-01-02 15:04"),
	})
	if err != nil {
		_ = os.Remove(targetPath)
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique") {
			response.WriteError(writer, http.StatusConflict, "该平台下版本号已存在")
			return
		}
		response.WriteError(writer, http.StatusInternalServerError, "保存更新记录失败: "+err.Error())
		return
	}
	downloadURL := fmt.Sprintf("/api/updates/%s/download", record.ID)
	if err := store.SetDownloadURL(request.Context(), record.ID, downloadURL); err != nil {
		logging.Warnf("[更新管理] 回写下载地址失败: %v", err)
	}
	record.DownloadURL = downloadURL
	logging.Infof("[更新管理] 新版本已发布: %s (%s) 大小 %.1f MB", record.Version, record.Platform, float64(size)/1024/1024)
	response.WriteData(writer, http.StatusOK, record)
}

// UpdateResource 单个更新版本（admin）：DELETE。
func UpdateResource(store *update.Store, dir string) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodDelete {
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		id := strings.Trim(strings.TrimPrefix(request.URL.Path, "/api/system/updates/"), "/")
		record, err := store.Get(request.Context(), id)
		if err != nil {
			if errors.Is(err, update.ErrNotFound) {
				response.WriteError(writer, http.StatusNotFound, "更新版本不存在")
				return
			}
			response.WriteError(writer, http.StatusInternalServerError, "读取更新记录失败: "+err.Error())
			return
		}
		if err := store.Delete(request.Context(), id); err != nil {
			response.WriteError(writer, http.StatusInternalServerError, "删除更新记录失败: "+err.Error())
			return
		}
		if record.FileName != "" {
			_ = os.Remove(filepath.Join(dir, record.FileName))
		}
		logging.Infof("[更新管理] 已删除版本 %s", record.Version)
		response.WriteData(writer, http.StatusOK, map[string]bool{"ok": true})
	}
}

// UpdateDownload 下载安装包（登录用户）。
func UpdateDownload(store *update.Store, dir string) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		id := strings.Trim(strings.TrimPrefix(request.URL.Path, "/api/updates/"), "/")
		id = strings.TrimSuffix(id, "/download")
		record, err := store.Get(request.Context(), id)
		if err != nil {
			response.WriteError(writer, http.StatusNotFound, "更新版本不存在")
			return
		}
		targetPath := filepath.Join(dir, record.FileName)
		file, err := os.Open(targetPath)
		if err != nil {
			response.WriteError(writer, http.StatusNotFound, "安装包文件缺失")
			return
		}
		defer file.Close()
		writer.Header().Set("Content-Type", "application/octet-stream")
		writer.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", record.FileName))
		_, _ = io.Copy(writer, file)
	}
}
