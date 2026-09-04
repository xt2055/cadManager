# Phase 4：建立前端业务 Modules

## 目标

建立与后端领域边界一致的前端核心模块，业务代码不再直接依赖 Vue、Pinia、Tauri 或 dataManager。

## 目录

```text
src/modules/
├─ drawing/
├─ attachment/
├─ review/
├─ editing/
├─ versioning/
├─ attribute/
├─ audit/
└─ admin/
```

## Shared

建立：

```text
shared/errors/
shared/types/
shared/infrastructure/
```

统一业务错误：

```text
RevisionConflictError
PartNoConflictError
BorrowedPartRequiresForkError
PartObsoleteError
StructureCycleError
UploadNotReadyError
```

## Composition Root

建立：

```text
src/app/container.ts
```

统一创建 repositories、gateways、services。页面不得自行 new XxxService。

## Drawing Domain

实现纯规则对象：

```text
Drawing
Part
PartRevision
DrawingPartRelation
DrawingNo
```

Domain 不依赖 Vue、Pinia、Tauri。

## Drawing Services

DrawingCommandService：

```text
createDrawing
createPart
updatePartDraft
borrowPart
forkBorrowedPart
updateRelation
replaceBom
archive
unarchive
```

DrawingQueryService：

```text
listDrawings
getDrawing
getStructure
getPart
getBom
```

## Repository

建立：

```text
DrawingRepository
ApiDrawingRepository
```

如保留 JSON Debug，实现独立的：

```text
JsonDrawingRepository
JsonAttachmentRepository
```

不得再通过 dataManager 绕行。

## Review

建立：

```text
ReviewCase
ReviewNode
ReviewService
ReviewQueryService
ReviewRepository
```

后端仍为审核状态权威。

## Editing

建立：

```text
EditingService
EditingGateway
TauriEditingGateway
SmbCredentialGateway
CaxaGateway
```

桌面、SMB、CAXA 细节全部留在 Gateway。

## 其他模块

依次抽离：

```text
Versioning
Attribute
Audit
Admin
```

## 验收标准

- [ ] 新业务代码不依赖 domain.store。
- [ ] Module Domain 不依赖 Vue/Pinia/Tauri。
- [ ] Repository 不经过 dataManager。
- [ ] JSON provider 如保留则按 Repository 独立实现。

## 当前实施记录

执行日期：2026-09-04

- 已建立 `src/modules/upload/UploadGateway` 和 `ApiUploadGateway`。
- 已建立 `BrowserUploadRecoveryStore`，统一适配现有 IndexedDB 恢复实现。
- 已建立 `src/app/container.ts` 作为上传基础设施 composition root。
- `domain.store.ts` 的分片上传、上传进度和恢复文件访问已改为通过容器能力；完整 Drawing/Attachment Service 抽取仍待本阶段后续工作。

## 建议提交

```text
refactor(core): add application container
refactor(drawing): introduce domain model
refactor(drawing): add command and query services
refactor(review): extract review module
refactor(editing): isolate desktop gateways
refactor(versioning): extract version module
refactor(attribute): extract attribute module
refactor(audit): extract audit module
refactor(admin): extract admin module
```
