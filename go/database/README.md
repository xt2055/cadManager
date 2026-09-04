# 数据库说明

## 开发数据库

当前电脑使用本机 PostgreSQL 开发环境：

```text
地址：127.0.0.1
端口：5432
数据库：cadguanliq
用户：cadguanliq_app
```

`cadguanliq_app` 是业务服务账号，不应使用 PostgreSQL 超级用户运行 Go 后端。

## 文件结构

```text
go/database/
├─ bootstrap.dev.sql
├─ seed.dev.sql
├─ seed_dev.sql
├─ README.md
└─ migrations/
   ├─ 000001_initial_schema.sql
   ├─ 000002_data_document_compat.sql
   ├─ 000003_review_node_roles.sql
   └─ 000004_review_flow_assignments.sql
```

## 初始化方式

`bootstrap.dev.sql` 需要使用有创建角色和数据库权限的管理员账号执行。当前本机管理员账号为 `postgres`。

在 `go` 目录执行：

```powershell
$env:PGPASSWORD = '管理员密码'
psql -h 127.0.0.1 -p 5432 -U postgres -d postgres -v ON_ERROR_STOP=1 -f database/bootstrap.dev.sql
Remove-Item Env:PGPASSWORD
```

脚本会执行：

1. 创建或更新开发业务用户 `cadguanliq_app`。
2. 创建数据库 `cadguanliq`，并将所有者设置为 `cadguanliq_app`。
3. 连接 `cadguanliq`。
4. 按序执行全部迁移，包括最终模型重建迁移。
5. 如需测试数据，再执行 `seed_dev.sql`。

脚本可以重复执行。表、索引、扩展和触发器使用幂等写法；重复执行不会删除已有业务数据。

## 开发管理员

如果需要使用现有前端的开发管理员账号，可以执行：

```powershell
$env:PGPASSWORD = '本机开发密码'
psql -h 127.0.0.1 -p 5432 -U cadguanliq_app -d cadguanliq -v ON_ERROR_STOP=1 -f database/seed.dev.sql
Remove-Item Env:PGPASSWORD
```

脚本创建或恢复以下本地开发账号：

```text
账号：admin
密码：admin
角色：admin
```

该账号只用于本机开发验证，生产环境不得执行 `seed.dev.sql`。

## 连接测试

初始化完成后，可以使用业务账号测试：

```powershell
$env:PGPASSWORD = '5201314520xt'
psql -h 127.0.0.1 -p 5432 -U cadguanliq_app -d cadguanliq -c '\dt'
Remove-Item Env:PGPASSWORD
```

## 本地开发密码

当前 `bootstrap.dev.sql` 中的 `cadguanliq_app` 密码为本机开发密码，仅允许用于本地开发数据库。该密码不得用于测试服务器、生产环境或提交到公开仓库。

如果后续密码发生变化，应同时更新本机角色密码和本地未提交的初始化脚本；生产环境必须通过环境变量、密钥管理服务或部署平台 Secret 注入。

## 最终模型表范围

首版迁移包含以下领域：

```text
用户与角色：users、user_roles、sessions
图纸与结构：drawings、parts、part_revisions、drawing_part_relations、drawing_signers
文件与物料：attachments、attachment_versions、file_blobs、part_revision_attachments、drawing_boms、bom_items
审核流程：review_flows、review_flow_nodes、review_cases、review_case_nodes、review_actions
借用：由 drawing_part_relations.relation_type = 'borrowed' 表示；日志：audit_logs
更新服务：update_manifests
```

## 约定

- 数据库内部关联使用 UUID。
- 图号使用业务字段 `drawing_no` 或 `part_no`，并建立唯一约束。
- 图纸和零件的签署人员独立保存。
- 附件元数据存数据库，文件内容当前由本地 `ObjectStorage` 实现保存。
- 当前本机附件根目录为后端目录下的 `go/storage/attachments`，实际路径为 `go/storage/attachments/<总图图号>(<总图名称>)/<原始文件名>`。
- 迁移文件使用递增编号，已执行的迁移不应直接修改；有变更时新增下一个迁移文件。
- 当前只写入数据库结构，不自动创建管理员业务用户。

## 当前迁移状态

本机开发数据库曾经执行：

```text
000001_initial_schema.sql
000002_data_document_compat.sql
000003_review_node_roles.sql
000004_review_flow_assignments.sql
```

当前开发环境允许通过 000022 重建最终模型。重建后，新的迁移仍只新增递增编号文件；生产环境不得执行开发库重建迁移。
