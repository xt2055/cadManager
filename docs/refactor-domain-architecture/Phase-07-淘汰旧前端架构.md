# Phase 7：淘汰旧前端架构

## 目标

在所有页面完成迁移、新模块稳定后，删除第一代前端架构。

## 删除 persist

删除：

```text
persist()
persistedSnapshots
JSON.stringify diff
```

不再通过整表快照持久化业务数据。

## 删除 reloadFromServer

替换为：

```text
targeted refresh
query refresh
resource invalidation
```

命令成功后只刷新受影响的资源，不重载全量业务数据。

## 删除 domain.store

删除：

```text
src/stores/domain.store.ts
useDomainStore
useCadDemoStore
useDemoStore
```

## 删除 dataManager

随着 Repository 全部迁移，删除：

```text
services/data-manager/
```

如仍需要 JSON Debug，保留独立的：

```text
JsonDrawingRepository
JsonAttachmentRepository
```

## 阶段检查

```text
rg "useDomainStore" src
rg "dataManager\." src
rg "persist\(" src
```

业务代码结果全部为 0。

## 当前实施记录

执行日期：2026-09-04

- 桌面布局和侧边栏已切换到 Drawing/Review Store，不再通过旧 Store 初始化或读取导航徽标。
- `STATUS` 展示常量已从旧 Store 提取到 `constants/drawing-status.ts`；无业务页面再导入旧 Store。
- 已删除 `demo.store.ts` 与 `domain.store.ts`；页面只通过 ReadModel Store 读取，尚未迁移的写命令集中在 `drawing-operations.store.ts`。
- 已移除 `reloadFromServer`，命令完成后由页面调用受影响 ReadModel Store 的 `refresh`；操作日志改由 `AuditService` 直接记录。
- 认证 JSON 回退和审核候选人查询已改为通过 `AdminService`，业务服务不再直接导入 dataManager。
- `drawing-operations.store.ts` 已移除 `persist`、快照缓存和 `JSON.stringify` 差异判断；BOM、服务端借用、创建零件和属性字段命令均已切换到原子 Application Service，附件写入统一由附件服务负责。
- 属性、结构和 BOM 仍保留 JSON Debug 的显式兼容适配器，JSON Debug 借用命令明确不支持复制降级；服务端模式不再调用整表写入。`dataManager` 仅保留在组合根中作为基础设施适配器，业务模块不再直接依赖它。
- 本批次 `npm run type-check`、`npm run build` 和 `git diff --check` 已通过。

## 验收标准

- [x] 旧 Store 不再被引用（`domain.store.ts` 已删除，页面无旧 Store 读取引用）。
- [x] dataManager 不再被组合根之外的业务代码使用。
- [x] 没有全量快照持久化。
- [x] 命令完成后使用定向刷新或资源失效。
- [x] 前端类型检查和构建通过；后端 `go test ./...` 通过，前端项目当前无独立 test script。

## 建议提交

```text
refactor(store): remove legacy domain store
refactor(data): remove data manager facade
refactor(state): remove snapshot persistence
```
