package dbtest

import (
	"context"
	"errors"
	"strings"
	"testing"

	"cadguanliq/internal/versioning"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestUpgradeAllowsLongWorkingVersionLabels(t *testing.T) {
	const migration = "migrations/000043_attachment_version_label_length.sql"
	db := NewBefore(t, migration)
	f := db.Seed(t)
	ctx := context.Background()
	repo := versioning.NewPGRepository(db.Pool)
	base := strings.Repeat("版", 50)
	input := versioning.CreateInput{
		AttachmentID: f.Attachment, Version: base, VersionKind: "working",
		CurrentName: "D-1.dwg", MimeType: "application/acad", Size: 8,
		SHA256: strings.Repeat("a", 64), CreatedBy: f.Author,
	}
	original, err := repo.CreateWithPromotion(ctx, input, "blobs/seed")
	if err != nil {
		t.Fatal(err)
	}
	input.Version = base + "-w001"
	_, err = repo.CreateWithPromotion(ctx, input, "blobs/seed")
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "22001" {
		t.Fatalf("升级前应复现版本标签超长，实际 %v", err)
	}

	db.ApplyFrom(t, migration)
	// 模拟迁移完成后服务重新建立连接，丢弃旧字段类型的预编译语句缓存。
	db.Pool.Reset()
	working, err := repo.CreateWithPromotion(ctx, input, "blobs/seed")
	if err != nil || working.Version != input.Version {
		t.Fatalf("升级后应完整保存常规编辑版本，实际 %q, %v", working.Version, err)
	}
	input.Version = base + "-w002"
	if work, err := repo.Create(ctx, input, "blobs/seed"); err != nil || work.Version != input.Version {
		t.Fatalf("升级后应完整保存工单工作版本，实际 %q, %v", work.Version, err)
	}
	if got := db.ScanString(t, `SELECT current_version_id::text FROM attachments WHERE id=$1::uuid`, f.Attachment); got != working.ID {
		t.Fatalf("工单工作版不应改变当前指针，实际 %s", got)
	}
	if got, err := repo.GetByID(ctx, original.ID); err != nil || got.Version != base {
		t.Fatalf("升级应保留原始标签，实际 %q, %v", got.Version, err)
	}
}
