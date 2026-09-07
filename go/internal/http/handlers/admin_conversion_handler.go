package handlers

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"cadguanliq/internal/response"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ConversionJobItem struct {
	ID             string     `json:"id"`
	UploadItemID   string     `json:"uploadItemId"`
	AttachmentID   *string    `json:"attachmentId"`
	DrawingID      *string    `json:"drawingId"`
	DrawingNo      *string    `json:"drawingNo"`
	DrawingName    *string    `json:"drawingName"`
	SourceName     string     `json:"sourceName"`
	SourceSize     int64      `json:"sourceSize"`
	SourceMimeType string     `json:"sourceMimeType"`
	Status         string     `json:"status"`
	Attempts       int        `json:"attempts"`
	NextAttemptAt  time.Time  `json:"nextAttemptAt"`
	LeaseUntil     *time.Time `json:"leaseUntil"`
	LastError      *string    `json:"lastError"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

type ConversionStats struct {
	Total      int `json:"total"`
	Pending    int `json:"pending"`
	Processing int `json:"processing"`
	Retry      int `json:"retry"`
	Backoff    int `json:"backoff"`
	Failed     int `json:"failed"`
}

type ConversionListResponse struct {
	Items      []ConversionJobItem `json:"items"`
	Total      int                 `json:"total"`
	Page       int                 `json:"page"`
	PageSize   int                 `json:"pageSize"`
	Stats      ConversionStats     `json:"stats"`
	CaxaStatus string              `json:"caxaStatus"`
}

// AdminConversionJobs 获取转换任务列表及汇总统计
func AdminConversionJobs(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			response.WriteError(w, http.StatusMethodNotAllowed, "方法不支持")
			return
		}

		ctx := r.Context()
		query := r.URL.Query()

		page, _ := strconv.Atoi(query.Get("page"))
		if page < 1 {
			page = 1
		}
		pageSize, _ := strconv.Atoi(query.Get("pageSize"))
		if pageSize < 1 || pageSize > 100 {
			pageSize = 20
		}
		offset := (page - 1) * pageSize

		statusFilter := strings.TrimSpace(query.Get("status"))
		keyword := strings.TrimSpace(query.Get("keyword"))

		// 统计汇总
		statsQuery := `
			SELECT 
				COUNT(*) as total,
				COUNT(*) FILTER (WHERE status = 'pending') as pending,
				COUNT(*) FILTER (WHERE status = 'processing') as processing,
				COUNT(*) FILTER (WHERE status = 'retry') as retry,
				COUNT(*) FILTER (WHERE status = 'backoff') as backoff,
				COUNT(*) FILTER (WHERE status = 'failed') as failed
			FROM cad_conversion_jobs`

		var stats ConversionStats
		if err := pool.QueryRow(ctx, statsQuery).Scan(
			&stats.Total,
			&stats.Pending,
			&stats.Processing,
			&stats.Retry,
			&stats.Backoff,
			&stats.Failed,
		); err != nil {
			response.WriteError(w, http.StatusInternalServerError, "获取转换任务统计失败: "+err.Error())
			return
		}

		// 构建筛选条件
		var conds []string
		var args []any
		argIdx := 1

		if statusFilter != "" && statusFilter != "all" {
			conds = append(conds, fmt.Sprintf("j.status = $%d", argIdx))
			args = append(args, statusFilter)
			argIdx++
		}

		if keyword != "" {
			conds = append(conds, fmt.Sprintf("(j.source_name ILIKE $%d OR d.drawing_no ILIKE $%d OR d.name ILIKE $%d)", argIdx, argIdx, argIdx))
			args = append(args, "%"+keyword+"%")
			argIdx++
		}

		whereClause := ""
		if len(conds) > 0 {
			whereClause = "WHERE " + strings.Join(conds, " AND ")
		}

		// 查符合条件的记录总数
		countSQL := fmt.Sprintf(`
			SELECT COUNT(*) 
			FROM cad_conversion_jobs j
			LEFT JOIN attachments a ON a.id = j.attachment_id
			LEFT JOIN drawings d ON d.id = a.drawing_id
			%s`, whereClause)

		var filteredTotal int
		if err := pool.QueryRow(ctx, countSQL, args...).Scan(&filteredTotal); err != nil {
			response.WriteError(w, http.StatusInternalServerError, "查询转换任务数量失败: "+err.Error())
			return
		}

		// 列表查询
		listSQL := fmt.Sprintf(`
			SELECT 
				j.id::text,
				j.upload_item_id::text,
				j.attachment_id::text,
				d.id::text,
				d.drawing_no,
				d.name,
				j.source_name,
				j.source_size_bytes,
				j.source_mime_type,
				j.status,
				j.attempts,
				j.next_attempt_at,
				j.lease_until,
				j.last_error,
				j.created_at,
				j.updated_at
			FROM cad_conversion_jobs j
			LEFT JOIN attachments a ON a.id = j.attachment_id
			LEFT JOIN drawings d ON d.id = a.drawing_id
			%s
			ORDER BY 
				CASE j.status 
					WHEN 'processing' THEN 1 
					WHEN 'pending' THEN 2 
					WHEN 'retry' THEN 3 
					WHEN 'backoff' THEN 4 
					WHEN 'failed' THEN 5 
					ELSE 6 
				END,
				j.updated_at DESC
			LIMIT $%d OFFSET $%d`, whereClause, argIdx, argIdx+1)

		args = append(args, pageSize, offset)

		rows, err := pool.Query(ctx, listSQL, args...)
		if err != nil {
			response.WriteError(w, http.StatusInternalServerError, "查询转换任务列表失败: "+err.Error())
			return
		}
		defer rows.Close()

		items := make([]ConversionJobItem, 0, pageSize)
		for rows.Next() {
			var item ConversionJobItem
			var attID, dID, dNo, dName, lastErr *string
			if err := rows.Scan(
				&item.ID,
				&item.UploadItemID,
				&attID,
				&dID,
				&dNo,
				&dName,
				&item.SourceName,
				&item.SourceSize,
				&item.SourceMimeType,
				&item.Status,
				&item.Attempts,
				&item.NextAttemptAt,
				&item.LeaseUntil,
				&lastErr,
				&item.CreatedAt,
				&item.UpdatedAt,
			); err != nil {
				response.WriteError(w, http.StatusInternalServerError, "读取转换任务失败: "+err.Error())
				return
			}
			item.AttachmentID = attID
			item.DrawingID = dID
			item.DrawingNo = dNo
			item.DrawingName = dName
			item.LastError = lastErr
			items = append(items, item)
		}

		caxaState := "idle"
		if stats.Processing > 0 {
			caxaState = "busy"
		}

		response.WriteData(w, http.StatusOK, ConversionListResponse{
			Items:      items,
			Total:      filteredTotal,
			Page:       page,
			PageSize:   pageSize,
			Stats:      stats,
			CaxaStatus: caxaState,
		})
	}
}

// AdminRetryConversionJob 管理员重试单条或全部失败的转换任务
func AdminRetryConversionJob(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			response.WriteError(w, http.StatusMethodNotAllowed, "方法不支持")
			return
		}

		ctx := r.Context()
		id := strings.TrimPrefix(r.URL.Path, "/api/admin/cad-conversions/")
		id = strings.TrimSuffix(id, "/retry")

		if id == "all-failed" || id == "" {
			// 批量重试所有 failed/backoff/retry 的任务
			result, err := pool.Exec(ctx, `
				UPDATE cad_conversion_jobs
				SET status = 'pending',
				    attempts = 0,
				    next_attempt_at = now(),
				    lease_until = NULL,
				    last_error = NULL,
				    updated_at = now()
				WHERE status IN ('failed', 'backoff', 'retry')`)
			if err != nil {
				response.WriteError(w, http.StatusInternalServerError, "批量重试失败: "+err.Error())
				return
			}
			response.WriteData(w, http.StatusOK, map[string]any{
				"retriedCount": result.RowsAffected(),
				"message":      fmt.Sprintf("已成功将 %d 个失败任务重置为排队中", result.RowsAffected()),
			})
			return
		}

		// 重试单条任务
		result, err := pool.Exec(ctx, `
			UPDATE cad_conversion_jobs
			SET status = 'pending',
			    attempts = 0,
			    next_attempt_at = now(),
			    lease_until = NULL,
			    last_error = NULL,
			    updated_at = now()
			WHERE id = $1::uuid`, id)
		if err != nil {
			response.WriteError(w, http.StatusInternalServerError, "重试任务入队失败: "+err.Error())
			return
		}

		if result.RowsAffected() == 0 {
			response.WriteError(w, http.StatusNotFound, "未找到该转换任务")
			return
		}

		response.WriteData(w, http.StatusOK, map[string]any{
			"id":      id,
			"message": "已将转换任务重置为排队状态",
		})
	}
}

// AdminConversionLogs 获取 CAXA 插件执行转换的实时日志 (取末尾 N 行)
func AdminConversionLogs() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			response.WriteError(w, http.StatusMethodNotAllowed, "方法不支持")
			return
		}

		limit := 300
		if lStr := r.URL.Query().Get("limit"); lStr != "" {
			if l, err := strconv.Atoi(lStr); err == nil && l > 0 && l <= 1000 {
				limit = l
			}
		}

		logPath := filepath.Join(os.TempDir(), "caxa_worker_log.txt")
		file, err := os.Open(logPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				response.WriteData(w, http.StatusOK, map[string]any{
					"lines":   []string{"[系统] CAXA 转换日志文件暂未生成"},
					"path":    logPath,
					"modTime": nil,
				})
				return
			}
			response.WriteError(w, http.StatusInternalServerError, "无法打开 CAXA 日志文件: "+err.Error())
			return
		}
		defer file.Close()

		fi, err := file.Stat()
		var modTime *time.Time
		if err == nil {
			mt := fi.ModTime()
			modTime = &mt
		}

		// 读取全部行并保留最后 limit 行
		scanner := bufio.NewScanner(file)
		var lines []string
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		if err := scanner.Err(); err != nil && err != io.EOF {
			// 文件可能正在被写入，忽略部分读取错误
		}

		if len(lines) > limit {
			lines = lines[len(lines)-limit:]
		}

		response.WriteData(w, http.StatusOK, map[string]any{
			"lines":   lines,
			"path":    logPath,
			"modTime": modTime,
			"total":   len(lines),
		})
	}
}
