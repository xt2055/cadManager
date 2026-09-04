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

## 验收标准

- [ ] Store 不处理 chunk/hash。
- [ ] 上传恢复逻辑独立。
- [ ] 转换失败可以重试。
- [ ] Attachment upload 正常。
- [ ] drawing-create 正常。
- [ ] 上传失败、转换失败和会话过期都有明确错误码。

## 建议提交

```text
refactor(upload): extract upload transport
refactor(upload): introduce upload gateway
refactor(upload): isolate recovery state
refactor(attachment): introduce attachment service
refactor(upload): extract drawing create uploader
```

