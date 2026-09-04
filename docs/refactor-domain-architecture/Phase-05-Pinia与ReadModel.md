# Phase 5：建立新的 Pinia / Read Model

## 目标

把 Pinia 降级为 Presentation State 和 Read Model Cache，不再承载复杂业务事务。

## workspace.store

只保存：

```text
selection
treeOpen
selectedStructureIndex
UI preference
```

选择使用判别联合：

```ts
type WorkspaceSelection =
  | { type: 'drawing'; id: DrawingId }
  | { type: 'part'; id: PartId }
```

不保存完整对象引用。

## drawing.store

保存：

```text
DrawingSummaryView
PartView
StructureNodeView
```

只提供：

```text
load
refresh
invalidate
```

不包含复杂事务。

## 其他 Store

- review.store：当前审核、待审核、轮询和加载状态。
- upload.store：进度、会话、失败项和恢复状态。
- admin.store：管理员页面状态。

## Read Model Mapper

建立：

```text
DTO
 ↓
DrawingReadModelMapper
 ↓
StructureNodeView
```

借用件由查询层映射：

```text
borrowed
sourcePartId
sourceDrawing
publishedVersion
```

前端不自行拼接借用来源或 Published 版本。

## 验收标准

- [x] 新增 Pinia Store 不执行领域事务，写操作统一委托 Application Service/Coordinator。
- [x] 新增 `workspace.store` 不保存 `currentDrawing` 对象，只保存判别联合身份。
- [x] 新增 Store 不直接调用 `dataManager` 或 `fetch`。
- [x] `DrawingReadModelMapper` 将查询结果映射为独立的 Summary/Part/Structure View。
- [x] 新增 Store 只保存身份、查询结果、上传会话和 UI/加载状态。

迁移期说明：旧 `domain.store` 仍保留 `currentDrawing` 和历史兼容写入路径，供 Phase 6 页面迁移期间使用；它不属于本阶段新增 Store 边界，待 Phase 6/7 收口。

## 当前实施记录

执行日期：2026-09-04

- 已建立 `workspace.store`，统一保存 drawing/part 判别联合选择、树展开状态和结构索引。
- 已建立 `drawing.store`，通过 `DrawingQueryService` 加载查询快照，并缓存独立 Read Model。
- 已建立 `DrawingReadModelMapper`，映射图纸摘要、零件视图、结构树视图和借用来源展示字段。
- 已建立 `review.store`，集中保存审核案例、已办记录、流程列表、加载状态和轮询生命周期。
- 已建立 `upload.store`，集中保存上传会话、逐文件进度、失败项和刷新恢复状态。
- 已建立 `admin.store`，集中保存账号、系统日志、更新包和管理员图纸列表查询结果。
- `npm run type-check`、`npm run build`、`git diff --check` 均通过。

## 建议提交

```text
refactor(store): add workspace store
refactor(store): add drawing read model store
refactor(store): migrate review state
refactor(store): migrate upload state
refactor(store): migrate admin state
```
