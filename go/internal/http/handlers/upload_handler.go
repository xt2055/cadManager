package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"cadguanliq/internal/http/middleware"
	"cadguanliq/internal/response"
	"cadguanliq/internal/upload"
	"github.com/jackc/pgx/v5/pgconn"
)

type uploadSessionRequest struct {
	Kind           string          `json:"kind"`
	IdempotencyKey string          `json:"idempotencyKey"`
	Metadata       json.RawMessage `json:"metadata"`
}

type uploadHashCheckRequest struct {
	SHA256   string `json:"sha256"`
	Size     int64  `json:"size"`
	MimeType string `json:"mimeType"`
}

type uploadItemRequest struct {
	ClientRef        string `json:"clientRef"`
	AttachmentID     string `json:"attachmentId"`
	DrawingNo        string `json:"drawingNo"`
	PartNo           string `json:"partNo"`
	Role             string `json:"role"`
	OriginalName     string `json:"originalName"`
	MimeType         string `json:"mimeType"`
	ExpectedRevision *int64 `json:"expectedRevision"`
	SHA256           string `json:"sha256"`
	Size             int64  `json:"size"`
	BlobID           string `json:"blobId"`
}

type uploadChunkInitRequest struct {
	TotalSize int64  `json:"totalSize"`
	ChunkSize int64  `json:"chunkSize"`
	SHA256    string `json:"sha256"`
}

func UploadSessionHashCheck(service *upload.Service) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		user, ok := middleware.UserFromContext(request.Context())
		if !ok {
			response.WriteError(writer, http.StatusUnauthorized, "登录已失效，请重新登录")
			return
		}
		if request.Method != http.MethodPost {
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var input uploadHashCheckRequest
		if err := decodeJSON(request, &input); err != nil {
			response.WriteError(writer, http.StatusBadRequest, "哈希预检参数格式无效")
			return
		}
		result, err := service.HashCheck(request.Context(), user.ID, upload.HashCheckInput{SHA256: input.SHA256, Size: input.Size, MimeType: input.MimeType})
		if err != nil {
			writeUploadError(writer, err)
			return
		}
		response.WriteData(writer, http.StatusOK, result)
	})
}

func UploadSessions(service *upload.Service) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		user, ok := middleware.UserFromContext(request.Context())
		if !ok {
			response.WriteError(writer, http.StatusUnauthorized, "登录已失效，请重新登录")
			return
		}
		if request.Method != http.MethodPost {
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var input uploadSessionRequest
		if err := decodeJSON(request, &input); err != nil {
			response.WriteError(writer, http.StatusBadRequest, "上传会话参数格式无效")
			return
		}
		session, err := service.CreateSession(request.Context(), user.ID, upload.CreateSessionInput{
			Kind: input.Kind, IdempotencyKey: input.IdempotencyKey, Metadata: input.Metadata,
		})
		if err != nil {
			writeUploadError(writer, err)
			return
		}
		response.WriteData(writer, http.StatusCreated, session)
	})
}

// UploadReconciliation exposes a read-only migration audit for administrators.
func UploadReconciliation(service *upload.Service) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, ok := middleware.UserFromContext(request.Context())
		if !ok {
			response.WriteError(writer, http.StatusUnauthorized, "登录已失效，请重新登录")
			return
		}
		if request.Method != http.MethodGet {
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		report, err := service.Reconcile(request.Context())
		if err != nil {
			writeUploadError(writer, err)
			return
		}
		response.WriteData(writer, http.StatusOK, report)
	})
}

func UploadSessionResource(service *upload.Service) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		user, ok := middleware.UserFromContext(request.Context())
		if !ok {
			response.WriteError(writer, http.StatusUnauthorized, "登录已失效，请重新登录")
			return
		}
		path := strings.TrimPrefix(request.URL.Path, "/api/upload-sessions/")
		parts := strings.Split(strings.Trim(path, "/"), "/")
		if len(parts) == 0 || parts[0] == "" {
			response.WriteError(writer, http.StatusNotFound, "上传会话不存在")
			return
		}
		sessionID := parts[0]
		if len(parts) == 1 {
			switch request.Method {
			case http.MethodGet:
				snapshot, err := service.Snapshot(request.Context(), user.ID, sessionID)
				if err != nil {
					writeUploadError(writer, err)
					return
				}
				response.WriteData(writer, http.StatusOK, snapshot)
			case http.MethodDelete:
				if err := service.Cancel(request.Context(), user.ID, sessionID); err != nil {
					writeUploadError(writer, err)
					return
				}
				response.WriteData(writer, http.StatusOK, nil)
			default:
				response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
			}
			return
		}
		if len(parts) == 2 && parts[1] == "items" && request.Method == http.MethodPost {
			var input uploadItemRequest
			if err := decodeJSON(request, &input); err != nil {
				response.WriteError(writer, http.StatusBadRequest, "上传文件项参数格式无效")
				return
			}
			item, err := service.CreateItem(request.Context(), user.ID, sessionID, upload.CreateItemInput{
				ClientRef: input.ClientRef, AttachmentID: input.AttachmentID, DrawingNo: input.DrawingNo,
				PartNo: input.PartNo, Role: input.Role, OriginalName: input.OriginalName,
				MimeType: input.MimeType, ExpectedRevision: input.ExpectedRevision, SHA256: input.SHA256,
				Size: input.Size, BlobID: input.BlobID,
			})
			if err != nil {
				writeUploadError(writer, err)
				return
			}
			response.WriteData(writer, http.StatusCreated, item)
			return
		}
		if len(parts) == 4 && parts[1] == "items" && parts[3] == "chunks" {
			if request.Method != http.MethodGet {
				response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			chunks, err := service.ListChunks(request.Context(), user.ID, sessionID, parts[2])
			if err != nil {
				writeUploadError(writer, err)
				return
			}
			response.WriteData(writer, http.StatusOK, chunks)
			return
		}
		if len(parts) == 5 && parts[1] == "items" && parts[3] == "chunks" {
			itemID, action := parts[2], parts[4]
			switch {
			case request.Method == http.MethodPost && action == "init":
				var input uploadChunkInitRequest
				if err := decodeJSON(request, &input); err != nil {
					response.WriteError(writer, http.StatusBadRequest, "分片初始化参数格式无效")
					return
				}
				result, err := service.InitChunks(request.Context(), user.ID, sessionID, itemID, upload.ChunkManifest{TotalSize: input.TotalSize, ChunkSize: input.ChunkSize, SHA256: input.SHA256})
				if err != nil {
					writeUploadError(writer, err)
					return
				}
				response.WriteData(writer, http.StatusOK, result)
				return
			case request.Method == http.MethodPost && action == "complete":
				item, err := service.CompleteChunks(request.Context(), user.ID, sessionID, itemID)
				if err != nil {
					writeUploadError(writer, err)
					return
				}
				response.WriteData(writer, http.StatusOK, item)
				return
			case request.Method == http.MethodPut:
				partNumber, err := strconv.Atoi(action)
				if err != nil {
					response.WriteError(writer, http.StatusBadRequest, "分片编号无效")
					return
				}
				request.Body = http.MaxBytesReader(writer, request.Body, 32<<20)
				chunk, err := service.UploadChunk(request.Context(), user.ID, sessionID, itemID, partNumber, request.Body, request.Header.Get("X-Chunk-SHA256"))
				if err != nil {
					writeUploadError(writer, err)
					return
				}
				response.WriteData(writer, http.StatusOK, chunk)
				return
			}
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if len(parts) == 2 && parts[1] == "commit" && request.Method == http.MethodPost {
			result, err := service.Commit(request.Context(), user.ID, sessionID)
			if err != nil {
				writeUploadError(writer, err)
				return
			}
			var payload any
			if json.Unmarshal(result, &payload) != nil {
				payload = json.RawMessage(result)
			}
			response.WriteData(writer, http.StatusOK, payload)
			return
		}
		if len(parts) == 3 && parts[1] == "items" {
			itemID := parts[2]
			if request.Method == http.MethodPost && len(parts) == 3 {
				if request.URL.Query().Get("action") == "retry" {
					item, err := service.RetryItem(request.Context(), user.ID, sessionID, itemID)
					if err != nil {
						writeUploadError(writer, err)
						return
					}
					response.WriteData(writer, http.StatusOK, item)
					return
				}
				if request.URL.Query().Get("action") == "retry-conversion" {
					item, err := service.RetryConversion(request.Context(), user.ID, sessionID, itemID)
					if err != nil {
						writeUploadError(writer, err)
						return
					}
					response.WriteData(writer, http.StatusOK, item)
					return
				}
				if err := request.ParseMultipartForm(32 << 20); err != nil {
					response.WriteError(writer, http.StatusBadRequest, "上传文件请求格式无效")
					return
				}
				file, header, err := request.FormFile("file")
				if err != nil {
					response.WriteError(writer, http.StatusBadRequest, "缺少上传文件")
					return
				}
				defer file.Close()
				item, err := service.UploadItem(request.Context(), user.ID, sessionID, itemID, header.Filename, request.FormValue("mimeType"), file)
				if err != nil {
					writeUploadError(writer, err)
					return
				}
				response.WriteData(writer, http.StatusOK, item)
				return
			}
		}
		response.WriteError(writer, http.StatusNotFound, "上传会话接口不存在")
	})
}

func writeUploadError(writer http.ResponseWriter, err error) {
	log.Printf("[上传] 请求失败: %v", err)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "55P03" {
		response.WriteError(writer, http.StatusConflict, "创建图纸正在等待其他数据库事务释放锁，已停止本次提交；文件已保留，请结束数据库中的未提交事务后继续提交")
		return
	}
	if errors.Is(err, context.DeadlineExceeded) {
		response.WriteError(writer, http.StatusGatewayTimeout, "上传提交等待超时，文件已保留，请查询会话状态后继续提交")
		return
	}
	switch {
	case errors.Is(err, upload.ErrNotFound):
		response.WriteError(writer, http.StatusNotFound, err.Error())
	case errors.Is(err, upload.ErrExpired):
		response.WriteError(writer, http.StatusGone, err.Error())
	case errors.Is(err, upload.ErrIncomplete):
		response.WriteError(writer, http.StatusConflict, err.Error())
	case errors.Is(err, upload.ErrConflict):
		response.WriteError(writer, http.StatusConflict, err.Error())
	case errors.Is(err, upload.ErrIdempotencyConflict):
		response.WriteError(writer, http.StatusConflict, err.Error())
	default:
		response.WriteError(writer, http.StatusBadRequest, err.Error())
	}
}
