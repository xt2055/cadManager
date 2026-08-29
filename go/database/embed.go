// Package database 提供建表迁移脚本的嵌入访问。
package database

import "embed"

// MigrationFS 内嵌全部数据库迁移 SQL（按文件名序号执行）。
//
//go:embed migrations/*.sql
var MigrationFS embed.FS
