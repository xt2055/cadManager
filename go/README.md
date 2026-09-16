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

## 角色与建档权

角色枚举为 `admin` / `planner` / `designer` / `reviewer`，一人可兼多角色。判断集中定义在 `internal/auth/roles.go`，不要在 handler 里散写角色字符串。

```text
计划员 planner   创建图纸（三种方式）、指派与改派负责人、查看任务总表
设计人员 designer 在被指派后编制图纸、上传文件、发起审核
审核人员 reviewer 处理待审任务并签署
管理员 admin      全部权限
```

**只有计划员与管理员可以创建图纸。** 这一限制有两个入口必须同时生效，缺一个就等于没做：

```text
POST /api/drawings                              直接建档
POST /api/upload-sessions kind=drawing-create   （创建页三种方式的真实写入口）
```

两者的拒绝口径一致，返回 `403 创建图纸需要计划员或管理员权限；设计人员请在任务管理台等待接受指派`。

## 图纸任务接口

```text
GET    /api/drawing-tasks?scope=mine                         我的任务（任意登录用户）
GET    /api/drawing-tasks?scope=board&page=1&page_size=20     任务总表（计划员/管理员）
       &keyword=&status=&assigned=assigned|unassigned&assignee_id=
POST   /api/drawing-tasks                                     指派负责人
GET    /api/drawing-tasks/candidates                          可被指派人员（含在办数量）
GET    /api/drawing-tasks/{drawingId}/history                 指派历史
PUT    /api/drawing-tasks/{taskId}                            改派或改任务说明
DELETE /api/drawing-tasks/{taskId}?reason=                     取消指派
```

指派请求体：

```json
{ "drawingId": "<drawings.id>", "assigneeId": "<users.id>", "note": "先出总图", "dueDate": "2026-10-01" }
```

改派请求体（`assigneeId` 与当前负责人一致时只更新说明与截止日期，不产生历史行）：

```json
{ "assigneeId": "<users.id>", "note": "", "dueDate": "", "reason": "原负责人休假" }
```

约定：

- 一张图纸同时只有一名有效负责人；重复指派返回 `409 该图纸已有负责人，请使用改派`。
- 被指派人必须在职且具备 `designer` / `planner` / `admin` 身份，否则 `400`（纯审核账号不能被指派，与「变更工单指定修改人」口径一致）。
- 已存档图纸返回 `409`：存档后负责人不再生效，修改须走变更工单指定执行人。
- 任务进度由图纸生命周期推导，不接受人工填写状态：草稿无文件 0%、草稿有文件 30%、审核中 60–90%（按节点完成比例）、生产中与已存档 100%、已停用 0%。
- 指派、改派、取消指派各自写入 `audit_logs`（`action='assign'`）并在同一事务内向当事人写入 `notifications`（`kind='task'`）；回滚不发送。

## 负责人 = 原创建人权限

图纸存在有效负责人时，原创建人的决定控制权归负责人与管理员，创建人不再拥有；没有有效负责人时回落创建人。规则单一实现在两处同构实现，其余代码一律复用：

```text
Go     internal/drawing  →  drawing.Drawing.Decides
SQL    drawing_decision_owner(drawing_uuid, user_uuid)   （事务内路径复用）
```

覆盖的判定点：编辑会话授权（`internal/editing`）、存档、删除图纸文件、3D 模型写入、发起普通送审。**新增涉及创建人权限的代码不要自己比较 `created_by`。**

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
