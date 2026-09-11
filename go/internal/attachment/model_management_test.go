package attachment

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"cadguanliq/database"
	"cadguanliq/internal/config"
	"cadguanliq/internal/data"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func TestModelManagementDatabase(t *testing.T) {
	if os.Getenv("CAD_MODEL_DB_TEST") != "1" {
		t.Skip("设置 CAD_MODEL_DB_TEST=1 后在隔离 schema 验证模型管理")
	}
	_ = godotenv.Load("../../.env")
	ctx := context.Background()
	base, err := data.NewPool(ctx, config.Load().Database)
	if err != nil {
		t.Fatal(err)
	}
	defer base.Close()
	schema := fmt.Sprintf("model_test_%d", time.Now().UnixNano())
	if _, err = base.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer base.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	cfg := base.Config()
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	_, err = pool.Exec(ctx, `
	CREATE TABLE users(id uuid PRIMARY KEY DEFAULT gen_random_uuid(), account text, display_name text);
	CREATE TABLE user_roles(user_id uuid, role text);
	CREATE TABLE drawings(id uuid PRIMARY KEY DEFAULT gen_random_uuid(), drawing_no text, created_by uuid, status text);
	CREATE TABLE parts(id uuid PRIMARY KEY DEFAULT gen_random_uuid(), part_no text, normalized_part_no text);
	CREATE TABLE drawing_part_relations(drawing_id uuid, part_id uuid, relation_type text, status text, created_at timestamptz DEFAULT now());
	CREATE TABLE attachments(id uuid PRIMARY KEY DEFAULT gen_random_uuid(), drawing_id uuid, part_id uuid, file_role text DEFAULT 'assembly', logical_name text, current_version_id uuid, revision bigint DEFAULT 1, uploaded_by uuid, author text DEFAULT '', created_at timestamptz DEFAULT now(), deleted_at timestamptz);
	CREATE TABLE file_blobs(id uuid PRIMARY KEY DEFAULT gen_random_uuid(), storage_key text, mime_type text, size_bytes bigint, sha256 text);
	CREATE TABLE attachment_versions(id uuid PRIMARY KEY DEFAULT gen_random_uuid(), attachment_id uuid, original_name text, blob_id uuid, version text DEFAULT 'v1.0', mime_type text DEFAULT 'application/octet-stream', size_bytes bigint DEFAULT 8, previewable boolean DEFAULT false);
	CREATE TABLE audit_logs(actor_id uuid, action text, resource_type text, resource_id uuid, summary text);`)
	if err != nil {
		t.Fatal(err)
	}
	migration, err := database.MigrationFS.ReadFile("migrations/000037_model_files.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	var user, outsider, drawing, part, blob string
	for _, entry := range []struct {
		query string
		dest  *string
	}{
		{`INSERT INTO users(account,display_name) VALUES('owner','Owner') RETURNING id::text`, &user},
		{`INSERT INTO users(account,display_name) VALUES('other','Other') RETURNING id::text`, &outsider},
		{`INSERT INTO parts(part_no,normalized_part_no) VALUES('P-1','P-1') RETURNING id::text`, &part},
		{`INSERT INTO file_blobs(storage_key,mime_type,size_bytes,sha256) VALUES('shared-model','application/octet-stream',8,'hash') RETURNING id::text`, &blob},
	} {
		if err := pool.QueryRow(ctx, entry.query).Scan(entry.dest); err != nil {
			t.Fatal(err)
		}
	}
	if err = pool.QueryRow(ctx, `INSERT INTO drawings(drawing_no,created_by,status) VALUES('D-1',$1::uuid,'draft') RETURNING id::text`, user).Scan(&drawing); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO drawing_part_relations VALUES($1::uuid,$2::uuid,'borrowed','active',now())`, drawing, part); err != nil {
		t.Fatal(err)
	}
	makeAttachment := func(name, category string) string {
		var id, version string
		if err := pool.QueryRow(ctx, `INSERT INTO attachments(drawing_id,logical_name,file_category,uploaded_by) VALUES($1::uuid,$2,$3,$4::uuid) RETURNING id::text`, drawing, name, category, user).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `INSERT INTO attachment_versions(attachment_id,original_name,blob_id) VALUES($1::uuid,$2,$3::uuid) RETURNING id::text`, id, name, blob).Scan(&version); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `UPDATE attachments SET current_version_id=$2::uuid WHERE id=$1::uuid`, id, version); err != nil {
			t.Fatal(err)
		}
		return id
	}
	first := makeAttachment("part.Z3PRT", "auto")
	second := makeAttachment("assembly.zip", "model3d")
	other := makeAttachment("notes.zip", "auto")
	repository := NewPGRepository(pool)
	item, err := repository.FindByID(ctx, first)
	if err != nil || item.FileCategory != "model3d" || item.Previewable {
		t.Fatalf("model classification: %+v %v", item, err)
	}
	if _, err = repository.SetPrimaryModel(ctx, first, 1, outsider); !errors.Is(err, ErrModelPermission) {
		t.Fatalf("non-owner allowed: %v", err)
	}
	if _, err = repository.SetPrimaryModel(ctx, other, 1, user); err == nil {
		t.Fatal("ordinary ZIP accepted as primary model")
	}
	if _, err = repository.SetPrimaryModel(ctx, first, 1, user); err != nil {
		t.Fatal(err)
	}
	if _, err = repository.SetPrimaryModel(ctx, first, 1, user); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale revision accepted: %v", err)
	}
	if _, err = repository.SetPrimaryModel(ctx, second, 1, user); err != nil {
		t.Fatal(err)
	}
	item, err = repository.FindByID(ctx, first)
	if err != nil || item.IsPrimaryModel || item.Revision != 3 {
		t.Fatalf("previous primary not cleared: %+v %v", item, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE attachments SET is_primary_model=true WHERE id=$1::uuid`, first); err == nil {
		t.Fatal("multiple primary models allowed by database")
	}
	for _, status := range []string{"reviewing", "archived"} {
		if _, err = pool.Exec(ctx, `UPDATE drawings SET status=$2 WHERE id=$1::uuid`, drawing, status); err != nil {
			t.Fatal(err)
		}
		if _, err = repository.SetPrimaryModel(ctx, first, 3, user); !errors.Is(err, ErrModelPermission) {
			t.Fatalf("%s model modification allowed: %v", status, err)
		}
	}
	if _, err = pool.Exec(ctx, `UPDATE drawings SET status='draft' WHERE id=$1::uuid`, drawing); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err = AuthorizeModelWrite(ctx, tx, "D-1", "P-1", user); !errors.Is(err, ErrModelPermission) {
		t.Fatalf("borrowed part can be changed: %v", err)
	}
}
