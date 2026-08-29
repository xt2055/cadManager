package handlers

import (
	"fmt"
	"io"
	"net/http"
	"strconv"

	"cadguanliq/internal/logging"
	"cadguanliq/internal/response"
)

// SystemLogs 查询内存缓冲中的最近日志（admin）。
func SystemLogs() http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		lines, _ := strconv.Atoi(request.URL.Query().Get("lines"))
		result := logging.Query(lines, request.URL.Query().Get("keyword"))
		response.WriteData(writer, http.StatusOK, result)
	}
}

// SystemLogFiles 列出日志文件（admin）。
func SystemLogFiles() http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		response.WriteData(writer, http.StatusOK, logging.Files())
	}
}

// SystemLogDownload 下载指定日志文件（admin）。
func SystemLogDownload() http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		name := request.URL.Query().Get("file")
		handle, err := logging.OpenFile(name)
		if err != nil {
			response.WriteError(writer, http.StatusBadRequest, err.Error())
			return
		}
		defer handle.Close()
		writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
		writer.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = io.Copy(writer, handle)
	}
}
