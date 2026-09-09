package titleblock

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"cadguanliq/internal/config"
	"cadguanliq/internal/data"
	"cadguanliq/internal/partindex"
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

func TestNormalizePayloadUsesArraysForAllCollections(t *testing.T) {
	payload := normalizePayload(Payload{
		Spaces: []Space{{
			ID:     "model",
			Fields: []Field{{Key: "name"}},
		}},
	})
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err = json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	spaces, ok := decoded["spaces"].([]any)
	if !ok || len(spaces) != 1 {
		t.Fatalf("spaces 未编码为数组：%s", raw)
	}
	space := spaces[0].(map[string]any)
	if _, ok = space["warnings"].([]any); !ok {
		t.Fatalf("warnings 未编码为数组：%s", raw)
	}
	fields := space["fields"].([]any)
	field := fields[0].(map[string]any)
	if _, ok = field["candidates"].([]any); !ok {
		t.Fatalf("candidates 未编码为数组：%s", raw)
	}
}

func TestSnapshotUsesExplicitSnapshotRevisionField(t *testing.T) {
	raw, err := json.Marshal(Snapshot{SnapshotRevision: 7})
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err = json.Unmarshal(raw, &object); err != nil {
		t.Fatal(err)
	}
	if object["snapshotRevision"] != float64(7) {
		t.Fatalf("标题栏快照版本字段不符合 API 契约：%s", raw)
	}
	if _, exists := object["revision"]; exists {
		t.Fatalf("标题栏快照不应使用含义不明确的 revision 字段：%s", raw)
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
 CREATE TABLE drawings(id uuid PRIMARY KEY,created_by uuid,project text,name text,drawing_no text);
 CREATE TABLE parts(id uuid PRIMARY KEY,part_no varchar(150),created_by uuid);
 CREATE TABLE attachments(id uuid PRIMARY KEY,current_version_id uuid,uploaded_by uuid,drawing_id uuid,part_id uuid,file_role text,deleted_at timestamptz);
 CREATE TABLE attachment_versions(id uuid PRIMARY KEY,attachment_id uuid REFERENCES attachments(id) ON DELETE CASCADE,original_name text,created_at timestamptz NOT NULL DEFAULT now(),deleted_at timestamptz,UNIQUE(attachment_id,id));
 CREATE TABLE drawing_part_relations(drawing_id uuid,part_id uuid,relation_type text,status text);`)
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
	// Exercise the real startup upgrade from the old title-block schema.
	if err = data.EnsurePartIndexSchema(ctx, pool); err != nil {
		t.Fatal(err)
	}
	const user = "11111111-1111-4111-8111-111111111111"
	const id = "22222222-2222-4222-8222-222222222222"
	const version = "33333333-3333-4333-8333-333333333333"
	const other = "44444444-4444-4444-8444-444444444444"
	const drawing = "55555555-5555-4555-8555-555555555555"
	_, err = pool.Exec(ctx, `INSERT INTO users VALUES($1::uuid)`, user)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO drawings(id,created_by,project,name,drawing_no) VALUES($1::uuid,$2::uuid,'J1233','测试项目','J1233')`, drawing, user)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO attachments(id,current_version_id,uploaded_by,drawing_id,file_role) VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,'part')`, id, version, user, drawing)
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
	if first.SnapshotRevision != 1 {
		t.Fatalf("首次保存的快照修订号错误：%d", first.SnapshotRevision)
	}
	if err = data.EnsurePartIndexSchema(ctx, pool); err != nil {
		t.Fatalf("重复启动升级失败：%v", err)
	}
	preserved, err := repo.Get(ctx, id, user, false)
	if err != nil || preserved.Payload == nil || preserved.Payload.Spaces[0].Fields[0].Value != "张定" || preserved.SnapshotRevision != first.SnapshotRevision {
		t.Fatalf("重复启动改变了标题栏快照：%+v %v", preserved, err)
	}
	var indexRevision, sourceSnapshotRevision int64
	var extractionStatus string
	if err = pool.QueryRow(ctx, `SELECT revision,source_snapshot_revision,extraction_status
			FROM part_indexes WHERE attachment_id=$1::uuid AND version_id=$2::uuid`, id, version).
		Scan(&indexRevision, &sourceSnapshotRevision, &extractionStatus); err != nil {
		t.Fatalf("标题栏保存后未在同一事务生成零件索引：%v", err)
	}
	if indexRevision != 1 || sourceSnapshotRevision != 1 || extractionStatus != "extracted" {
		t.Fatalf("首次投影状态错误：index=%d snapshot=%d status=%q", indexRevision, sourceSnapshotRevision, extractionStatus)
	}
	indexDetail, err := partindex.NewRepository(pool).Detail(ctx, id, user, false)
	if err != nil || indexDetail.AttachmentID != id || indexDetail.Revision != 1 {
		t.Fatalf("零件索引详情读取失败：%+v %v", indexDetail, err)
	}
	if err = repo.Save(ctx, id, version, user, false, payload); err != nil {
		t.Fatal(err)
	}
	second, err := repo.Get(ctx, id, user, false)
	if err != nil || first.ExtractedAt != second.ExtractedAt || second.SnapshotRevision != first.SnapshotRevision {
		t.Fatalf("重复保存未保持幂等：%v", err)
	}
	if err = pool.QueryRow(ctx, `SELECT revision,source_snapshot_revision
			FROM part_indexes WHERE attachment_id=$1::uuid AND version_id=$2::uuid`, id, version).
		Scan(&indexRevision, &sourceSnapshotRevision); err != nil {
		t.Fatal(err)
	}
	if indexRevision != 1 || sourceSnapshotRevision != 1 {
		t.Fatalf("相同快照不应重复递增索引版本：index=%d snapshot=%d", indexRevision, sourceSnapshotRevision)
	}
	multipleSpaces := Payload{Spaces: []Space{
		{ID: "layout-1", Fields: []Field{{Key: "name", Value: "旧布局零件"}}},
		{ID: "layout-2", Fields: []Field{{Key: "name", Value: "新布局零件"}}},
	}}
	if err = repo.Save(ctx, id, version, user, false, multipleSpaces); err != nil {
		t.Fatalf("保存多布局标题栏失败：%v", err)
	}
	indexRepo := partindex.NewRepository(pool)
	beforeEdit, err := indexRepo.Detail(ctx, id, user, false)
	if err != nil {
		t.Fatalf("读取多布局索引失败：%v", err)
	}
	layoutID := "layout-2"
	manualFields := partindex.Fields{PartName: "人工确认名称"}
	afterEdit, err := indexRepo.Edit(ctx, id, user, false, partindex.EditInput{
		VersionID:                version,
		ExpectedRevision:         beforeEdit.Revision,
		ExpectedSnapshotRevision: beforeEdit.SnapshotRevision,
		Action:                   "save",
		SelectedSpaceID:          &layoutID,
		Fields:                   &manualFields,
	})
	if err != nil {
		t.Fatalf("选择新布局并保存失败：%v", err)
	}
	if afterEdit.SelectedSpaceID == nil || *afterEdit.SelectedSpaceID != layoutID ||
		afterEdit.AutoFields.PartName != "新布局零件" || afterEdit.Fields.PartName != "人工确认名称" {
		t.Fatalf("保存后自动字段未跟随新布局：%+v", afterEdit)
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
