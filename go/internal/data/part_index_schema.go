package data

import (
	"context"
	"fmt"
	"strings"

	"cadguanliq/database"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EnsurePartIndexSchema upgrades the title-block/index feature on existing
// installations. Do not replay historical bootstrap migrations here: some
// of them rebuild domain tables. Each feature migration and its ledger entry
// are committed together, with a lock shared by concurrent server starts.
func EnsurePartIndexSchema(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("开始零件索引数据库升级失败: %w", err)
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(734035)`); err != nil {
		return fmt.Errorf("锁定零件索引数据库升级失败: %w", err)
	}
	if _, err = tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return fmt.Errorf("初始化迁移记录失败: %w", err)
	}
	for _, name := range []string{
		"000034_attachment_title_blocks.sql",
		"000035_part_indexes.sql",
		"000036_material_bom_metadata.sql",
	} {
		version := strings.TrimSuffix(name, ".sql")
		var applied bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, version).Scan(&applied); err != nil {
			return fmt.Errorf("读取迁移记录 %s 失败: %w", version, err)
		}
		if applied {
			continue
		}
		content, readErr := database.MigrationFS.ReadFile("migrations/" + name)
		if readErr != nil {
			return fmt.Errorf("读取迁移 %s 失败: %w", version, readErr)
		}
		if _, err = tx.Exec(ctx, string(content)); err != nil {
			return fmt.Errorf("执行迁移 %s 失败: %w", version, err)
		}
		if _, err = tx.Exec(ctx, `INSERT INTO schema_migrations(version) VALUES($1)`, version); err != nil {
			return fmt.Errorf("保存迁移记录 %s 失败: %w", version, err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("提交零件索引数据库升级失败: %w", err)
	}
	return nil
}
