# Phase 3：重构上传和附件基础设施

## 目标

先拆分复杂但相对独立的 Upload / Attachment 模块，建立可恢复、可重试、可转换的上传基础。

## UploadTransport

只负责：

```text
hash
upload
chunk
resume
retry
```

不得知道：

```text
Drawing
Part
Material
Craft
```

## Upload 状态机

```text
pending
uploading
uploaded
processing
ready
failed
committed
```

失败必须记录 failureStage：

```text
hash
upload
conversion
validation
```

uploaded 不等于 ready。

## UploadGateway

建立：

```text
UploadGateway
ApiUploadGateway
```

把 API 细节隔离在 Infrastructure 层。

## Upload Recovery

建立独立的 UploadRecoveryStore，负责：

- localStorage / IndexedDB。
- 会话恢复。
- 分片进度恢复。
- 失败文件恢复。

Application 层不得依赖具体存储技术。

## AttachmentService

负责：

```text
upload
replace
delete
download
```

借用件不能直接替换工程内容，必须先 Fork。

## CAD Conversion

转换失败时：

```text
status = failed
failureStage = conversion
```

支持仅重试转换，尽可能复用已经上传的 Blob，不重新上传文件内容。

## DrawingCreateUploader

对现有 drawing-create session 做独立封装，并保留：

```text
commitDrawingCreateTx()
```

作为创建 Drawing 的事务入口。

## 当前实施结果

后端基础设施已切换到最终模型：

- [x] `file_blobs` 是唯一内容对象，附件只通过 `attachment_versions.blob_id` 引用。
- [x] 普通附件创建、替换和删除不再写旧 `attachments` 文件字段或 `file_versions`。
- [x] `drawing-create` 在一个事务中写入 Drawing、Part、Revision、Relation、附件版本、BOM、借用、分支和审计。
- [x] 附件替换使用 `attachments.revision` CAS；冲突返回 `ErrConflict`。
- [x] 哈希预检、秒传、分片初始化/上传/查询/合并和最终 SHA-256 校验保留在上传会话服务。
- [x] CAD 转换失败记录为失败项，重试时复用原始上传内容对象，不要求重新上传。
- [x] 清理任务使用 `FOR UPDATE SKIP LOCKED` 抢占，并支持 processing 超时恢复和失败重试。
- [x] 编辑器读取当前版本真实文件名；Blob 键无扩展名不会直接交给 CAXA。
- [x] 管理后台和编辑会话读取已切换到最终附件、版本和零件关系表。

仍属于前端后续阶段的工作：

- [x] 已将底层上传和恢复能力抽成 `UploadGateway` / `UploadRecoveryStore`。
- [x] 待上传文件已通过恢复存储持久化到 IndexedDB；分片快照由服务端会话查询恢复。
- [ ] 将完整的 Drawing/Attachment 上传业务编排从 `domain.store.ts` 移到 Application Service。
- [ ] 将失败文件列表、逐文件重试、转换重试和会话过期交互完整接入页面。

## 验收标准

- [x] Store 不再直接处理 chunk/hash，统一通过 `ApiUploadGateway`。
- [x] 上传恢复文件通过 `BrowserUploadRecoveryStore` 适配 IndexedDB。
- [x] 转换失败可以重试。
- [x] Attachment upload 正常。
- [x] drawing-create 正常。
- [x] 上传失败、转换失败和会话过期都有明确错误码。

## 建议提交

```text
refactor(upload): extract upload transport
refactor(upload): introduce upload gateway
refactor(upload): isolate recovery state
refactor(attachment): introduce attachment service
refactor(upload): extract drawing create uploader
```

## 本次实施记录

执行日期：2026-09-04

- 上传提交、附件仓储和版本仓储已迁移到 `attachments + attachment_versions + file_blobs`。
- 删除运行时旧上传 schema 修复逻辑，最终 schema 只由 migration 建立。
- 管理后台图纸、零件、附件、版本和编辑会话查询已改用最终模型。
- 修复 Blob 键无扩展名导致 CAXA 工作副本命名错误的问题。
- `go test ./...` 通过；`go build` 通过。
- 前端新增 `ApiUploadGateway`、`BrowserUploadRecoveryStore` 和 `appContainer`；上传进度、分片续传和恢复文件不再由 Store 直接访问底层 API/IndexedDB。
