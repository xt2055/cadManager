package upload

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"cadguanliq/internal/attachment"
	"cadguanliq/internal/storage"
	"github.com/jackc/pgx/v5"
)

const (
	conversionLease      = 5 * time.Minute
	conversionRetryLimit = 5
)

type cadConversionJob struct {
	ID               string
	ItemID           string
	AttachmentID     string
	SourceBlobID     string
	SourceStorageKey string
	SourceName       string
	SourceMimeType   string
	SourceSize       int64
	SourceSHA256     string
	Attempts         int
}

// StartConversionQueue runs the durable queue. A job remains in PostgreSQL
// until the converted DWG is stored and the attachment current pointer moves.
func (service *Service) StartConversionQueue(ctx context.Context) {
	if service == nil || service.pool == nil || service.storage == nil || service.converter == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			if err := service.processNextCADConversion(ctx); err != nil {
				log.Printf("[CAD Converter] 异步转换队列处理失败: %v", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (service *Service) enqueueCADConversion(ctx context.Context, itemID, blobID, blobKey, name string, object storage.ObjectInfo) error {
	if service.converter == nil {
		return errors.New("CAD 转换服务未配置")
	}
	_, err := service.pool.Exec(ctx, `
		INSERT INTO cad_conversion_jobs (upload_item_id, source_blob_id, source_storage_key, source_name, source_mime_type, source_size_bytes, source_sha256)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7)
		ON CONFLICT (upload_item_id) DO UPDATE SET
		 source_blob_id = EXCLUDED.source_blob_id, source_storage_key = EXCLUDED.source_storage_key,
		 source_name = EXCLUDED.source_name, source_mime_type = EXCLUDED.source_mime_type,
		 source_size_bytes = EXCLUDED.source_size_bytes, source_sha256 = EXCLUDED.source_sha256,
		 updated_at = now()
		WHERE cad_conversion_jobs.attachment_id IS NULL`, itemID, blobID, blobKey, name, object.MimeType, object.Size, object.SHA256)
	if err != nil {
		return fmt.Errorf("写入转换队列失败: %w", err)
	}
	return nil
}

func bindCADConversionJobTx(ctx context.Context, tx pgx.Tx, itemID, attachmentID string) error {
	// A newly uploaded replacement supersedes any unclaimed older task for this
	// logical attachment. A leased task is checked against source_blob_id before
	// it is allowed to switch the current version.
	if _, err := tx.Exec(ctx, `UPDATE cad_conversion_jobs SET status = 'cancelled', updated_at = now() WHERE attachment_id = $1::uuid AND upload_item_id <> $2::uuid AND status IN ('pending', 'retry')`, attachmentID, itemID); err != nil {
		return fmt.Errorf("取消旧转换任务失败: %w", err)
	}
	result, err := tx.Exec(ctx, `UPDATE cad_conversion_jobs SET attachment_id = $2::uuid, updated_at = now() WHERE upload_item_id = $1::uuid`, itemID, attachmentID)
	if err != nil {
		return fmt.Errorf("关联转换任务失败: %w", err)
	}
	if result.RowsAffected() == 0 {
		return errors.New("CAD 文件缺少转换队列任务")
	}
	return nil
}

func (service *Service) processNextCADConversion(ctx context.Context) error {
	job, err := service.claimCADConversion(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}

	if err := service.convertQueuedCAD(ctx, job); err != nil {
		return service.retryCADConversion(ctx, job, err)
	}
	return nil
}

func (service *Service) claimCADConversion(ctx context.Context) (cadConversionJob, error) {
	tx, err := service.pool.Begin(ctx)
	if err != nil {
		return cadConversionJob{}, err
	}
	defer tx.Rollback(ctx)
	var job cadConversionJob
	err = tx.QueryRow(ctx, `
		SELECT id::text, upload_item_id::text, attachment_id::text, source_blob_id::text,
		       source_storage_key, source_name, source_mime_type, source_size_bytes, source_sha256, attempts
		FROM cad_conversion_jobs
		WHERE attachment_id IS NOT NULL
		  AND ((status IN ('pending', 'retry', 'backoff') AND next_attempt_at <= now())
		       OR (status = 'processing' AND lease_until <= now()))
		ORDER BY created_at
		FOR UPDATE SKIP LOCKED
		LIMIT 1`).Scan(&job.ID, &job.ItemID, &job.AttachmentID, &job.SourceBlobID,
		&job.SourceStorageKey, &job.SourceName, &job.SourceMimeType, &job.SourceSize, &job.SourceSHA256, &job.Attempts)
	if err != nil {
		return cadConversionJob{}, err
	}
	job.Attempts++
	if _, err := tx.Exec(ctx, `UPDATE cad_conversion_jobs SET status = 'processing', attempts = $2, lease_until = now() + $3::interval, last_error = NULL, updated_at = now() WHERE id = $1::uuid`, job.ID, job.Attempts, intervalText(conversionLease)); err != nil {
		return cadConversionJob{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return cadConversionJob{}, err
	}
	return job, nil
}

func (service *Service) convertQueuedCAD(ctx context.Context, job cadConversionJob) error {
	// Never let an old queued job overwrite a newer current attachment version.
	var currentSource bool
	if err := service.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM attachments a JOIN attachment_versions v ON v.id = a.current_version_id WHERE a.id = $1::uuid AND v.blob_id = $2::uuid AND a.deleted_at IS NULL)`, job.AttachmentID, job.SourceBlobID).Scan(&currentSource); err != nil {
		return err
	}
	if !currentSource {
		_, err := service.pool.Exec(ctx, `DELETE FROM cad_conversion_jobs WHERE id = $1::uuid`, job.ID)
		return err
	}

	convertedKey, err := service.converter.EnsureDwg(ctx, attachment.Attachment{
		StorageKey:        job.SourceStorageKey,
		CurrentStorageKey: job.SourceStorageKey,
		Name:              job.SourceName,
		CurrentName:       job.SourceName,
		Size:              job.SourceSize,
		MimeType:          job.SourceMimeType,
		SHA256:            job.SourceSHA256,
		CurrentSHA256:     job.SourceSHA256,
	})
	if err != nil {
		return err
	}
	convertedObject, err := service.openObjectInfo(ctx, convertedKey)
	if err != nil {
		return err
	}
	processedBlobID, processedKey, err := service.ensureBlob(ctx, convertedKey, convertedObject)
	if err != nil {
		return err
	}
	if err := service.commitCADConversion(ctx, job, processedBlobID, processedKey, convertedObject); err != nil {
		return fmt.Errorf("保存 CAD 转换结果失败（文件已转换）: %w", err)
	}
	if convertedKey != processedKey {
		if err := service.storage.Delete(ctx, convertedKey); err != nil {
			service.scheduleCleanup(ctx, convertedKey, "cad-conversion-staging")
		}
	}
	log.Printf("[CAD Converter] 异步转换完成并切换当前版本: %s", job.SourceName)
	return nil
}

func (service *Service) commitCADConversion(ctx context.Context, job cadConversionJob, processedBlobID, processedKey string, convertedObject storage.ObjectInfo) error {
	tx, err := service.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var versionID, currentBlobID string
	if err := tx.QueryRow(ctx, `SELECT a.current_version_id::text, v.blob_id::text FROM attachments a JOIN attachment_versions v ON v.id=a.current_version_id WHERE a.id=$1::uuid AND a.deleted_at IS NULL FOR UPDATE OF a, v`, job.AttachmentID).Scan(&versionID, &currentBlobID); err != nil {
		return err
	}
	if currentBlobID != job.SourceBlobID {
		if _, err := tx.Exec(ctx, `DELETE FROM cad_conversion_jobs WHERE id=$1::uuid`, job.ID); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	// The source may already be the original, a release or a submitted snapshot.
	// Keep it immutable and register the converted file as a separate working
	// version. The attachment lock and source check above prevent stale jobs from
	// replacing a newer upload; insertion and queue completion commit together.
	var convertedVersionID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO attachment_versions (attachment_id, version, blob_id, original_name, mime_type, size_bytes, previewable, version_kind, created_by)
		SELECT attachment_id, '_converted_' || $5::text, $2::uuid, $3, 'application/acad', $4, true, 'working', created_by
		FROM attachment_versions WHERE id=$1::uuid
		RETURNING id::text`, versionID, processedBlobID, processedCADName(job.SourceName), convertedObject.Size, job.ID).Scan(&convertedVersionID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE attachments SET current_version_id=$2::uuid, revision=revision+1 WHERE id=$1::uuid`, job.AttachmentID, convertedVersionID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE upload_session_items SET processed_object_key = $2, processed_blob_id = $3::uuid, processed_size_bytes = $4, processed_sha256 = $5, processed_mime_type = $6, updated_at = now() WHERE id = $1::uuid`, job.ItemID, processedKey, processedBlobID, convertedObject.Size, convertedObject.SHA256, firstNonEmpty(convertedObject.MimeType, "application/acad")); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM cad_conversion_jobs WHERE id=$1::uuid`, job.ID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (service *Service) retryCADConversion(ctx context.Context, job cadConversionJob, cause error) error {
	status := "retry"
	delay := time.Duration(job.Attempts) * 15 * time.Second
	if job.Attempts >= conversionRetryLimit {
		// 快速重试用尽后保留任务，等待修复后手动重试。
		status = "failed"
	}
	_, err := service.pool.Exec(ctx, `UPDATE cad_conversion_jobs SET status = $2, next_attempt_at = now() + $3::interval, lease_until = NULL, last_error = $4, updated_at = now() WHERE id = $1::uuid`, job.ID, status, intervalText(delay), strings.TrimSpace(cause.Error()))
	if err != nil {
		return fmt.Errorf("记录 CAD 转换失败状态失败: %w", err)
	}
	if status == "failed" {
		log.Printf("[CAD Converter] 自动重试用尽，保留任务等待手动重试: %s: %v", job.SourceName, cause)
	} else {
		log.Printf("[CAD Converter] 异步转换失败，将重试（%d/%d）: %s: %v", job.Attempts, conversionRetryLimit, job.SourceName, cause)
	}
	return nil
}
