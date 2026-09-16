// Package dbtest 为数据库约束与事务行为提供隔离的集成测试环境。
//
// 约束类需求只能由真实 PostgreSQL 验证：迁移中的触发器和函数决定了历史证据
// 是否可被改写，用 mock 无法证明。启用方式见 Require。
package dbtest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cadguanliq/database"
	"cadguanliq/internal/config"
	"cadguanliq/internal/data"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

// Require 在没有开启集成测试时跳过。避免默认 go test ./... 需要数据库。
func Require(t *testing.T) {
	t.Helper()
	if os.Getenv("CAD_DB_TEST") != "1" {
		t.Skip("设置 CAD_DB_TEST=1 后使用隔离 schema 验证数据库约束")
	}
}

// loadEnv 从当前目录向上查找并加载 .env。
//
// 测试可能从任意包运行，工作目录是各自的包目录，因此不能写死相对路径。
func loadEnv() {
	dir, err := os.Getwd()
	if err != nil {
		return
	}
	for depth := 0; depth < 6; depth++ {
		candidate := filepath.Join(dir, ".env")
		if _, statErr := os.Stat(candidate); statErr == nil {
			_ = godotenv.Load(candidate)
			return
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return
		}
		dir = parent
	}
}

// migratedVersion 是迁移文件中必须已应用的最小版本，用于在 schema 搭建后自检。
const migratedVersion = "migrations/000050_planner_role_and_drawing_tasks.sql"

// DB 是绑定到独立 schema 的测试数据库。
type DB struct {
	Pool   *pgxpool.Pool
	schema string
}

// New 创建一个独立 schema，执行全部迁移，并返回绑定该 schema 的连接池。
// 清理通过 t.Cleanup 自动完成，因此调用方无需关心回收。
func New(t *testing.T) *DB {
	t.Helper()
	Require(t)

	loadEnv()
	ctx := context.Background()

	base, err := data.NewPool(ctx, config.Load().Database)
	if err != nil {
		t.Fatalf("连接测试数据库失败: %v", err)
	}

	// schema 名不能以数字开头，使用固定前缀加纳秒时间戳保证并行测试互不冲突。
	schema := fmt.Sprintf("itest_%d", time.Now().UnixNano())
	if _, err = base.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		base.Close()
		t.Fatalf("创建隔离 schema 失败: %v", err)
	}

	cfg := base.Config()
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		base.Close()
		t.Fatalf("创建 schema 连接池失败: %v", err)
	}

	// 迁移内的 GRANT 指向 public schema，且部分 DDL 不在事务中，因此逐文件执行。
	// 单个 schema 内迁移必须全部成功，否则后续断言没有意义。
	names, err := migrationNames()
	if err != nil {
		base.Close()
		pool.Close()
		t.Fatalf("读取迁移列表失败: %v", err)
	}
	db := &DB{Pool: pool, schema: schema}
	db.applyUpTo(t, names, migratedVersion, true)

	t.Cleanup(func() {
		pool.Close()
		_, _ = base.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		base.Close()
	})

	return db
}

// NewBefore 创建一个 schema 并只执行到 targetMigration 之前（不含该文件），
// 供调用方插入“升级前”的数据后，再用 ApplyFrom 继续执行剩余迁移。
//
// 这用于验证老数据升级路径：迁移中的一次性数据修正逻辑只会在真实旧数据上产生
// 效果，全新 schema 走全量迁移无法复现。
func NewBefore(t *testing.T, targetMigration string) *DB {
	t.Helper()
	Require(t)

	loadEnv()
	ctx := context.Background()
	base, err := data.NewPool(ctx, config.Load().Database)
	if err != nil {
		t.Fatalf("连接测试数据库失败: %v", err)
	}
	schema := fmt.Sprintf("itest_%d", time.Now().UnixNano())
	if _, err = base.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		base.Close()
		t.Fatalf("创建隔离 schema 失败: %v", err)
	}
	cfg := base.Config()
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		base.Close()
		t.Fatalf("创建 schema 连接池失败: %v", err)
	}
	names, err := migrationNames()
	if err != nil {
		base.Close()
		pool.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = base.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		base.Close()
	})

	db := &DB{Pool: pool, schema: schema}
	db.applyUpTo(t, names, targetMigration, false)
	return db
}

// ApplyFrom 执行 targetMigration 及其之后的全部迁移。
// 之前序号的迁移必须已经执行过（通常由 NewBefore 完成）。
func (d *DB) ApplyFrom(t *testing.T, targetMigration string) {
	t.Helper()
	names, err := migrationNames()
	if err != nil {
		t.Fatal(err)
	}
	started := false
	for _, name := range names {
		if name == targetMigration {
			started = true
		}
		if !started {
			continue
		}
		content, readErr := database.MigrationFS.ReadFile(name)
		if readErr != nil {
			t.Fatalf("读取迁移 %s 失败: %v", name, readErr)
		}
		if _, execErr := d.Pool.Exec(context.Background(), stripMetaCommands(string(content))); execErr != nil {
			t.Fatalf("执行迁移 %s 失败: %v", name, execErr)
		}
	}
	if !started {
		t.Fatalf("未找到目标迁移 %s", targetMigration)
	}
}

// applyUpTo 按序执行迁移。include 为真时包含 target 本身，否则停在 target 之前。
func (d *DB) applyUpTo(t *testing.T, names []string, target string, include bool) {
	t.Helper()
	seen := false
	for _, name := range names {
		if name == target {
			seen = true
			if !include {
				return
			}
		}
		content, err := database.MigrationFS.ReadFile(name)
		if err != nil {
			t.Fatalf("读取迁移 %s 失败: %v", name, err)
		}
		if _, err = d.Pool.Exec(context.Background(), stripMetaCommands(string(content))); err != nil {
			t.Fatalf("执行迁移 %s 失败: %v", name, err)
		}
	}
	if !seen {
		t.Fatalf("未找到目标迁移 %s", target)
	}
}

// Schema 返回当前测试使用的 schema 名，便于诊断。
func (d *DB) Schema() string { return d.schema }

// stripMetaCommands 移除 psql 反斜杠元命令。
//
// 迁移文件以 "\set ON_ERROR_STOP on" 开头，该行是 psql 客户端指令而非 SQL，
// 通过驱动执行会导致语法错误。ON_ERROR_STOP 的语义由本函数调用方保证：
// 任一语句出错即终止测试。
func stripMetaCommands(sql string) string {
	if !strings.Contains(sql, "\\") {
		return sql
	}
	var builder strings.Builder
	builder.Grow(len(sql))
	for _, line := range strings.Split(sql, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "\\") {
			// 保留换行以维持错误信息中的行号。
			builder.WriteString("\n")
			continue
		}
		builder.WriteString(line)
		builder.WriteString("\n")
	}
	return builder.String()
}

// migrationNames 返回按序排列的迁移文件名。文件名前缀序号即执行顺序，
// 与 embed 的字典序一致。
func migrationNames() ([]string, error) {
	entries, err := database.MigrationFS.ReadDir("migrations")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		names = append(names, "migrations/"+entry.Name())
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("没有找到迁移文件")
	}
	for _, name := range names {
		if name == migratedVersion {
			return names, nil
		}
	}
	return nil, fmt.Errorf("缺少目标迁移 %s", migratedVersion)
}

// Fixture 提供测试数据骨架：两个用户、一个图纸、一个零件。
// 多数约束测试都需要这些外键目标。
type Fixture struct {
	Author     string
	Reviewer   string
	Planner    string
	Drawing    string
	Part       string
	Attachment string
	Blob       string
}

// AssignDrawingTask 直接写入一条有效指派，用于验证「负责人 = 原创建人权限」的规则。
// 走 SQL 而不是服务层，是为了让约束类断言不依赖上层实现。
func (d *DB) AssignDrawingTask(t *testing.T, drawingID, assigneeID, assignedBy string) string {
	t.Helper()
	var id string
	if err := d.Pool.QueryRow(context.Background(), `
		INSERT INTO drawing_tasks(drawing_id, assignee_id, assigned_by, note)
		VALUES ($1::uuid, $2::uuid, NULLIF($3,'')::uuid, '测试指派')
		RETURNING id::text`, drawingID, assigneeID, assignedBy).Scan(&id); err != nil {
		t.Fatalf("插入图纸任务失败: %v", err)
	}
	return id
}

// Seed 插入满足外键约束的最小数据集，返回各自 UUID。
func (d *DB) Seed(t *testing.T) Fixture {
	t.Helper()
	ctx := context.Background()
	var f Fixture

	exec := func(dest *string, query string, args ...any) {
		t.Helper()
		if err := d.Pool.QueryRow(ctx, query, args...).Scan(dest); err != nil {
			t.Fatalf("插入测试数据失败 (%s): %v", query, err)
		}
	}

	for _, item := range []struct {
		dest  *string
		query string
		args  []any
	}{
		{&f.Author, `INSERT INTO users(account,display_name,password_hash,status) VALUES('author','设计员','x','active') RETURNING id::text`, nil},
		{&f.Reviewer, `INSERT INTO users(account,display_name,password_hash,status) VALUES('reviewer','审核员','x','active') RETURNING id::text`, nil},
		{&f.Planner, `INSERT INTO users(account,display_name,password_hash,status) VALUES('planner','计划员','x','active') RETURNING id::text`, nil},
		{&f.Part, `INSERT INTO parts(part_no,normalized_part_no,created_by) VALUES('P-1','P-1',(SELECT id FROM users WHERE account='author')) RETURNING id::text`, nil},
		{&f.Blob, `INSERT INTO file_blobs(storage_key,mime_type,size_bytes,sha256) VALUES('blobs/seed','application/octet-stream',8,repeat('a',64)) RETURNING id::text`, nil},
	} {
		exec(item.dest, item.query, item.args...)
	}

	exec(&f.Drawing, `INSERT INTO drawings(drawing_no,name,project,created_by,status) VALUES('D-1','测试图纸','P','`+f.Author+`'::uuid,'draft') RETURNING id::text`)
	exec(&f.Attachment, `INSERT INTO attachments(drawing_id,logical_name,file_role,uploaded_by) VALUES('`+f.Drawing+`'::uuid,'D-1.dwg','part','`+f.Author+`'::uuid) RETURNING id::text`)
	return f
}

// Exec 执行语句并要求成功，失败时终止测试。
func (d *DB) Exec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := d.Pool.Exec(context.Background(), query, args...); err != nil {
		t.Fatalf("执行失败: %v\nSQL: %s", err, query)
	}
}

// QueryRow 扫描单行，失败时终止测试。
func (d *DB) QueryRow(t *testing.T, query string, args ...any) pgx.Row {
	t.Helper()
	return d.Pool.QueryRow(context.Background(), query, args...)
}

// ScanString 执行查询并把结果读入 string，失败时终止测试。
func (d *DB) ScanString(t *testing.T, query string, args ...any) string {
	t.Helper()
	var value string
	if err := d.Pool.QueryRow(context.Background(), query, args...).Scan(&value); err != nil {
		t.Fatalf("查询失败: %v\nSQL: %s", err, query)
	}
	return value
}

// MustFail 断言语句被数据库拒绝（触发器或约束生效），并返回错误信息。
func (d *DB) MustFail(t *testing.T, query string, args ...any) string {
	t.Helper()
	_, err := d.Pool.Exec(context.Background(), query, args...)
	if err == nil {
		t.Fatalf("期望数据库拒绝该操作，但执行成功\nSQL: %s", query)
	}
	return err.Error()
}
