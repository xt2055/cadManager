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

## 验收标准

- [ ] 旧 Store 不再被引用。
- [ ] dataManager 不再被业务代码使用。
- [ ] 没有全量快照持久化。
- [ ] 命令完成后使用定向刷新或资源失效。
- [ ] 前端类型检查、单元测试和构建通过。

## 建议提交

```text
refactor(store): remove legacy domain store
refactor(data): remove data manager facade
refactor(state): remove snapshot persistence
```

