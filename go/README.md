# 图枢后端

## 当前结构

后端采用模块化单体结构，当前已完成服务骨架：

```text
go/
├─ cmd/server/main.go       # 独立服务入口
├─ internal/app             # 服务组装与启动
├─ internal/config          # 环境变量配置
├─ internal/http             # 路由、处理器和中间件
├─ internal/response        # 统一 JSON 响应
├─ internal/update           # 客户端更新检查逻辑
└─ internal/drawing          # 图纸、结构和零件接口
```

## 启动

在 `go` 目录执行：

```powershell
go run .
```

默认监听 `:8080`，可以通过 `CAD_SERVER_ADDR` 修改。

也可以执行独立服务入口：

```powershell
go run ./cmd/server
```

两种启动方式使用相同的配置和路由。

## 服务配置

开发环境使用 `go/.env` 加载配置。该文件不会提交到 Git，模板见 `go/.env.example`。

复制模板并填写本机密码：

```powershell
Copy-Item .env.example .env
```

```text
CAD_SERVER_ADDR=:8080
CAD_ALLOWED_ORIGINS=http://localhost:5173,http://127.0.0.1:5173
CAD_DB_HOST=127.0.0.1
CAD_DB_PORT=5432
CAD_DB_NAME=cadguanliq
CAD_DB_USER=cadguanliq_app
CAD_DB_PASSWORD=本机开发密码
CAD_DB_SSL_MODE=disable
```

`CAD_ALLOWED_ORIGINS` 使用逗号分隔。未配置时允许开发环境跨域请求。

服务启动时会连接 PostgreSQL 并执行 Ping。数据库连接失败时服务不会启动。

## 图纸接口

除健康检查、认证和兼容文档接口外，当前已实现以下受保护接口：

```text
GET   /api/drawings
POST  /api/drawings
GET   /api/drawings/:drawingId
PATCH /api/drawings/:drawingId

GET   /api/drawings/:drawingId/structure
POST  /api/drawings/:drawingId/parts
GET   /api/parts/:partId
PATCH /api/parts/:partId
```

请求需要携带：

```text
Authorization: Bearer <登录令牌>
```

图纸列表支持：

```text
GET /api/drawings?page=1&page_size=20&keyword=关键词&status=draft&vendor=研发组
```

创建总图示例：

```json
{
  "no": "2000W.02.03d",
  "name": "液压缸总成",
  "project": "液压缸项目",
  "kind": "总图",
  "vendor": "研发组",
  "signers": {
    "设计": "张工",
    "校对": "王工",
    "审核": "李工"
  }
}
```

结构接口使用数据库 UUID 作为资源路径，使用图号作为业务字段。零件的 `parentNo` 必须属于当前总图的结构树，支持多级父子关系。

## 附件接口

当前附件使用本地文件存储，后端已经通过 `ObjectStorage` 接口隔离存储实现，后续可以替换为 MinIO、S3 或其他对象存储。

```text
POST   /api/attachments
GET    /api/attachments/:storageKey
DELETE /api/attachments/:storageKey
```

上传使用 `multipart/form-data`：

```text
file         文件内容，必填
drawingNo    所属总图图号，必填
partNo       所属零件图号，可选
role         assembly / part / material / craft / other，可选
version      文件版本，可选
previewable  是否可预览，可选
```

本地存储路径规则：

```text
CAD_STORAGE_ROOT/<总图图号>(<总图名称>)/<原始文件名>
```

例如：

```text
go/storage/attachments/2000W.02.03d(2000W斜撑油缸)/2000W.02.03d(2000W斜撑油缸).dwg
go/storage/attachments/2000W.02.03d(2000W斜撑油缸)/2000W.02.03d-01(缸体).exb
go/storage/attachments/2000W.02.03d(2000W斜撑油缸)/JG9064-100-63-05(防尘板).exb
```

零件文件仍归档到所属总图目录中，数据库附件元数据通过 `part_id` 区分具体零件。

当前开发配置：

```env
CAD_STORAGE_ROOT=./storage/attachments
CAD_MAX_UPLOAD_MB=100
```

## EXB 标题栏解析接口

解析已上传附件中的 CAXA EXB 标题栏键值，不使用文件名推断，也不执行 OCR：

```text
POST /api/exb/parse
```

请求体：

```json
{
  "storageKey": "2000W.02.03d(2000W斜撑油缸)/2000W.02.03d-01(缸体).exb"
}
```

成功响应中的 `data.titleBlock` 只包含 EXB 标题栏中实际读到的键值，例如：

```json
{
  "单位名称": "泸州市巨力液压有限公司",
  "图纸名称": "缸体",
  "材料名称": "27SiMn组焊件",
  "图纸编号": "2000W.02.03C-01",
  "图纸比例": "1:3"
}
```

后续切换对象存储时，只需要新增 `ObjectStorage` 实现并替换服务组装，不需要修改上传 Handler 和附件数据库接口。

## 业务数据兼容接口

当前前端 JSON/API Provider 仍使用整份文档兼容接口：

```text
GET /api/data/document
PUT /api/data/document
```

该接口的数据保存在 PostgreSQL 的 `data_documents` 表中，后续前端资源接口全部迁移完成后再移除。

## 更新服务配置

更新接口：

```text
GET /api/updates/latest?current_version=0.1.0&platform=windows-x86_64
```

环境变量：

```text
UPDATE_VERSION=0.2.0
UPDATE_NOTES=新增图纸明细表读取功能
UPDATE_PUBLISHED_AT=2026-08-22T12:00:00Z
UPDATE_DOWNLOAD_URL=https://example.com/releases/cad-0.2.0.nsis.zip
UPDATE_MANDATORY=false
```

当前版本大于或等于 `UPDATE_VERSION` 时，接口会返回 `updateAvailable=false`。

当前接口只负责检查版本和返回更新信息，不负责自动下载或安装客户端。自动更新需要后续接入 Tauri Updater 签名包和发布文件。
