package titleblock

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"cadguanliq/internal/config"
	"cadguanliq/internal/data"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func TestValidate(t *testing.T) {
	good := Payload{Spaces: []Space{{ID: "model", TextCount: 2, Fields: []Field{{Key: "designer", Value: "张定"}}}}}
	if Validate(good) != nil {
		t.Fatal("有效记录被拒绝")
	}
	for _, bad := range []Payload{
		{Spaces: []Space{{ID: "model", Fields: []Field{{Key: "password"}}}}},
		{Spaces: []Space{{ID: "model", Fields: []Field{{Key: "designer", Value: strings.Repeat("字", 501)}}}}},
		{Spaces: []Space{{ID: "model", Fields: []Field{{Key: "name"}, {Key: "name"}}}}},
		{Error: "失败", Spaces: []Space{{ID: "model"}}},
	} {
		if Validate(bad) == nil {
			t.Fatal("非法记录被接受")
		}
	}
}

func TestRepositoryDatabase(t *testing.T) {
	if os.Getenv("CAD_TITLEBLOCK_DB_TEST") != "1" {
		t.Skip("设置 CAD_TITLEBLOCK_DB_TEST=1 后在隔离 schema 验证数据库读写")
	}
	_ = godotenv.Load("../../.env")
	ctx := context.Background()
	base, err := data.NewPool(ctx, config.Load().Database)
	if err != nil {
		t.Fatal(err)
	}
	defer base.Close()
	schema := fmt.Sprintf("titleblock_test_%d", time.Now().UnixNano())
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
	_, err = pool.Exec(ctx, `CREATE TABLE users(id uuid PRIMARY KEY);
 CREATE TABLE drawings(id uuid PRIMARY KEY,created_by uuid);
 CREATE TABLE parts(id uuid PRIMARY KEY,created_by uuid);
 CREATE TABLE attachments(id uuid PRIMARY KEY,current_version_id uuid,uploaded_by uuid,drawing_id uuid,part_id uuid,deleted_at timestamptz);
 CREATE TABLE attachment_versions(id uuid PRIMARY KEY,attachment_id uuid REFERENCES attachments(id) ON DELETE CASCADE,original_name text,deleted_at timestamptz,UNIQUE(attachment_id,id));`)
	if err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../../database/migrations/000034_attachment_title_blocks.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	const user = "11111111-1111-4111-8111-111111111111"
	const id = "22222222-2222-4222-8222-222222222222"
	const version = "33333333-3333-4333-8333-333333333333"
	const other = "44444444-4444-4444-8444-444444444444"
	_, err = pool.Exec(ctx, `INSERT INTO users VALUES($1::uuid)`, user)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO attachments(id,current_version_id,uploaded_by) VALUES($1::uuid,$2::uuid,$3::uuid)`, id, version, user)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO attachment_versions(id,attachment_id,original_name) VALUES($1::uuid,$2::uuid,'sample.exb'),($3::uuid,$2::uuid,'sample.dwg')`, version, id, other)
	if err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(pool)
	before, err := repo.Get(ctx, id, user, false)
	if err != nil || !before.CanWrite || before.Payload != nil {
		t.Fatalf("初始读取失败：%+v %v", before, err)
	}
	payload := Payload{Spaces: []Space{{ID: "model", Fields: []Field{{Key: "designer", Value: "张定"}}}}}
	if err = repo.Save(ctx, id, version, other, false, payload); err != ErrForbidden {
		t.Fatalf("越权错误：%v", err)
	}
	if err = repo.Save(ctx, id, other, user, false, payload); err != ErrConflict {
		t.Fatalf("版本校验错误：%v", err)
	}
	if err = repo.Save(ctx, id, version, user, false, payload); err != nil {
		t.Fatal(err)
	}
	first, err := repo.Get(ctx, id, user, false)
	if err != nil || first.Payload.Spaces[0].Fields[0].Value != "张定" {
		t.Fatalf("读回失败：%v", err)
	}
	if err = repo.Save(ctx, id, version, user, false, payload); err != nil {
		t.Fatal(err)
	}
	second, err := repo.Get(ctx, id, user, false)
	if err != nil || first.ExtractedAt != second.ExtractedAt {
		t.Fatalf("重复保存未保持幂等：%v", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE attachments SET current_version_id=$2::uuid WHERE id=$1::uuid`, id, other); err != nil {
		t.Fatal(err)
	}
	changed, err := repo.Get(ctx, id, user, false)
	if err != nil || changed.Payload != nil || !changed.HasPrevious {
		t.Fatalf("旧版本信息未隔离：%+v %v", changed, err)
	}
	if err = repo.Save(ctx, id, version, user, false, payload); err != ErrConflict {
		t.Fatalf("过期保存未拒绝：%v", err)
	}
}
