package upload

import (
	"context"
	"strings"
	"testing"

	"cadguanliq/internal/dbtest"
	"cadguanliq/internal/storage"
)

func TestCommitCADConversionPreservesHistory(t *testing.T) {
	for _, formal := range []bool{false, true} {
		name := "original"
		if formal {
			name = "formal"
		}
		t.Run(name, func(t *testing.T) {
			db, service, job, source, converted := seedConversion(t)
			if formal {
				db.Exec(t, `UPDATE attachment_versions SET release_number=1, version='V1', version_kind='release' WHERE id=$1::uuid`, source)
			}
			before := db.ScanString(t, `SELECT row_to_json(v)::text FROM attachment_versions v WHERE id=$1::uuid`, source)
			object := storage.ObjectInfo{Size: 9, SHA256: strings.Repeat("b", 64), MimeType: "application/acad"}
			// A failure after inserting the new version must roll back the pointer too.
			invalid := object
			invalid.SHA256 = strings.Repeat("b", 65)
			if err := service.commitCADConversion(context.Background(), job, converted, "blobs/converted", invalid); err == nil {
				t.Fatal("expected metadata persistence failure")
			}
			if got := db.ScanString(t, `SELECT current_version_id::text FROM attachments WHERE id=$1::uuid`, job.AttachmentID); got != source {
				t.Fatal("failed transaction moved the current pointer")
			}
			for i := 0; i < 2; i++ {
				if err := service.commitCADConversion(context.Background(), job, converted, "blobs/converted", object); err != nil {
					t.Fatal(err)
				}
			}
			if after := db.ScanString(t, `SELECT row_to_json(v)::text FROM attachment_versions v WHERE id=$1::uuid`, source); after != before {
				t.Fatal("conversion mutated the historical source")
			}
			if got := db.ScanString(t, `SELECT original_version_id::text FROM attachments WHERE id=$1::uuid`, job.AttachmentID); got != source {
				t.Fatal("original pointer changed")
			}
			if got := db.ScanString(t, `SELECT v.blob_id::text || ':' || v.original_name || ':' || v.version_kind FROM attachments a JOIN attachment_versions v ON v.id=a.current_version_id WHERE a.id=$1::uuid`, job.AttachmentID); got != converted+":part.dwg:working" {
				t.Fatalf("unexpected current file: %s", got)
			}
			if got := db.ScanString(t, `SELECT count(*)::text FROM attachment_versions WHERE attachment_id=$1::uuid`, job.AttachmentID); got != "2" {
				t.Fatalf("retry created duplicate versions: %s", got)
			}
			if got := db.ScanString(t, `SELECT processed_blob_id::text FROM upload_session_items WHERE id=$1::uuid`, job.ItemID); got != converted {
				t.Fatal("upload result was not saved")
			}
			if got := db.ScanString(t, `SELECT count(*)::text FROM cad_conversion_jobs WHERE id=$1::uuid`, job.ID); got != "0" {
				t.Fatal("completed job remains queued")
			}
			db.MustFail(t, `UPDATE attachment_versions SET original_name='overwritten.dwg' WHERE id=$1::uuid`, source)
		})
	}
}

func TestCommitCADConversionDoesNotReplaceNewerUpload(t *testing.T) {
	db, service, job, _, converted := seedConversion(t)
	newVersion := db.ScanString(t, `INSERT INTO attachment_versions(attachment_id,version,blob_id,original_name) VALUES($1::uuid,'new-upload',$2::uuid,'new.exb') RETURNING id::text`, job.AttachmentID, converted)
	db.Exec(t, `UPDATE attachments SET current_version_id=$2::uuid WHERE id=$1::uuid`, job.AttachmentID, newVersion)
	if err := service.commitCADConversion(context.Background(), job, converted, "blobs/converted", storage.ObjectInfo{Size: 9}); err != nil {
		t.Fatal(err)
	}
	if got := db.ScanString(t, `SELECT current_version_id::text FROM attachments WHERE id=$1::uuid`, job.AttachmentID); got != newVersion {
		t.Fatal("stale job replaced newer upload")
	}
}

func TestMigrationRetriesOnlyHistoricalConversionFailures(t *testing.T) {
	db, _, job, _, _ := seedConversion(t)
	for _, tc := range []struct{ status, message, want string }{
		{"failed", "错误: 历史版本内容不可覆盖或隐藏 (SQLSTATE P0001)", "retry"},
		{"cancelled", "历史版本内容不可覆盖或隐藏", "cancelled"},
		{"failed", "转换器不可用", "failed"},
	} {
		db.Exec(t, `UPDATE cad_conversion_jobs SET status=$2,last_error=$3,attempts=5 WHERE id=$1::uuid`, job.ID, tc.status, tc.message)
		db.ApplyFrom(t, "migrations/000042_retry_historical_cad_conversions.sql")
		if got := db.ScanString(t, `SELECT status FROM cad_conversion_jobs WHERE id=$1::uuid`, job.ID); got != tc.want {
			t.Fatalf("%s / %s: got %s, want %s", tc.status, tc.message, got, tc.want)
		}
	}
}

func seedConversion(t *testing.T) (*dbtest.DB, *Service, cadConversionJob, string, string) {
	t.Helper()
	db := dbtest.New(t)
	f := db.Seed(t)
	source := db.ScanString(t, `INSERT INTO attachment_versions(attachment_id,version,blob_id,original_name,created_by) VALUES($1::uuid,'v1.0',$2::uuid,'part.exb',$3::uuid) RETURNING id::text`, f.Attachment, f.Blob, f.Author)
	db.Exec(t, `UPDATE attachments SET current_version_id=$2::uuid,original_version_id=$2::uuid WHERE id=$1::uuid`, f.Attachment, source)
	converted := db.ScanString(t, `INSERT INTO file_blobs(storage_key,mime_type,size_bytes,sha256) VALUES('blobs/converted','application/acad',9,repeat('b',64)) RETURNING id::text`)
	session := db.ScanString(t, `INSERT INTO upload_sessions(user_id,kind,idempotency_key,expires_at,absolute_expires_at) VALUES($1::uuid,'attachment','conversion',now()+interval '1 day',now()+interval '7 days') RETURNING id::text`, f.Author)
	item := db.ScanString(t, `INSERT INTO upload_session_items(session_id,client_ref,attachment_id,original_name,status) VALUES($1::uuid,'conversion',$2::uuid,'part.exb','committed') RETURNING id::text`, session, f.Attachment)
	id := db.ScanString(t, `INSERT INTO cad_conversion_jobs(upload_item_id,attachment_id,source_blob_id,source_storage_key,source_name,source_size_bytes,source_sha256,status) VALUES($1::uuid,$2::uuid,$3::uuid,'blobs/seed','part.exb',8,repeat('a',64),'processing') RETURNING id::text`, item, f.Attachment, f.Blob)
	return db, &Service{pool: db.Pool}, cadConversionJob{ID: id, ItemID: item, AttachmentID: f.Attachment, SourceBlobID: f.Blob, SourceName: "part.exb"}, source, converted
}
