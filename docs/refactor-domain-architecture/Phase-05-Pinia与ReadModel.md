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

- [ ] Pinia 不执行复杂业务事务。
- [ ] currentDrawing object reference 消失。
- [ ] Store 不直接调用 dataManager 或 fetch。
- [ ] Read Model 与 Domain Entity 分离。
- [ ] Store 只保存身份、查询结果和 UI 状态。

## 建议提交

```text
refactor(store): add workspace store
refactor(store): add drawing read model store
refactor(store): migrate review state
refactor(store): migrate upload state
refactor(store): migrate admin state
```

