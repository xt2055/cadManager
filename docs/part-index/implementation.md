# CAD 图纸管理系统：零件视图一期实施文档

版本：1.0  
日期：2026-09-09  
仓库：`D:\zt\project\web\cadguanliq`  
目标读者：负责实际编码的模型或开发者  
文档状态：一期代码已完成；部署前仍需执行数据库集成测试和人工验收

---

## 1. 交付目标与范围

### 1.1 交付目标

在现有系统中增加“零件索引”导航。用户能够搜索项目中已有的零件 CAD 附件，查看标题栏信息、修正识别结果、确认信息、查看关联项目，并打开对应附件进行 CAD 预览。

项目和零件索引引用同一份附件及文件版本，不复制文件。

`J1233`、`J1233A`、`J1233B` 保持独立业务编号。不得截断后缀，不得据此前后排序生成版本链，不得自动认定为同一项目。

### 1.2 一期必须实现

1. 后端持久化零件索引和人工修正。
2. 已有标题栏快照保存成功时，同事务同步当前版本索引。
3. 零件列表、服务器分页、关键词搜索。
4. 项目、材料、设计人、图纸日期、处理状态筛选。
5. 零件详情、关联项目列表、CAD 预览入口。
6. 选择标题栏布局、选择候选值、人工编辑、保存草稿、确认。
7. 已有快照批量补建索引。
8. 用户在浏览器中对选中的待提取附件执行串行提取。
9. 文件换版、删除、恢复及借用关系变化后的正确读取。
10. 登录、写入权限、并发冲突和异常处理。
11. 本文规定的自动化测试及人工验收。

### 1.3 一期明确不实现

- 相似度百分比、几何相似搜索、自动认定两个附件是同一零件。
- 自动跨项目零件合并、自动创建或修改 `parts`、`part_revisions`。
- 新的图纸对比算法；现有对比功能保持原样。
- 独立项目主表、项目成员 ACL、审批流程。
- 后端直接解析 DWG、AI、OCR、新的标题栏模板管理系统。
- 一张 CAD 文件拆出多个独立零件索引。
- 旧文件版本的索引浏览页面。
- 从“当前关联项目”推断“历史实际使用版本”。
- 修改 CAD 图内文字、修改文件内容、自动发布零件版本。

### 1.4 范围与原始需求的差别

原始需求包含按项目权限隔离。当前核对到的后端图纸查询没有项目成员过滤，本期继承现有登录后共享读取策略；这是明确记录的需求缺口，不算项目隔离已完成。

如果部署必须满足按项目成员隔离，则需要先完成全系统权限改造，再按同一权限策略开放零件索引。实施模型不得单独发明一套只作用于索引的成员规则。

图幅一期允许人工填写和展示，不自动识别，不提供图幅筛选。设计人、校对人、日期则复用已有提取字段。

---

## 2. 已核对的代码基线

路径均相对仓库根目录。

| 现有文件 | 已有职责 | 本期处理 |
|---|---|---|
| `vue/cad/src/features/drawings/detail-tabs/preview/cad-title-block.ts` | 收集 CAD 实体文字、块属性、空间，提取字段及候选值 | 保留算法，复用结果 |
| `vue/cad/src/services/drawing-title-block.service.ts` | 前端读取 CAD、调用提取、保存快照、缓存 | 扩展请求类型和状态处理 |
| `vue/cad/src/services/title-block-workflow.ts` | 单文件和批量提取工作流 | 保留并扩展版本校验及取消行为 |
| `vue/cad/src/features/drawings/components/detail/SavedDrawingInfo.vue` | 已保存信息展示、重新提取、布局切换 | 增加进入零件索引的入口 |
| `go/internal/titleblock/service.go` | 快照结构、校验、读取和保存 | 增加快照修订号及事务内索引同步 |
| `go/internal/http/handlers/title_block_handler.go` | `/api/cad/title-blocks/{attachmentId}` | 保持原接口兼容 |
| `go/database/migrations/000034_attachment_title_blocks.sql` | 按附件及版本保存标题栏 JSON | 不修改已发布迁移，新增迁移 |
| `go/database/migrations/000022_rebuild_final_domain_model.sql` | 当前项目、零件、附件等基础模型 | 用于理解关系，不重写 |
| `go/internal/drawing/repository.go` | 现有图纸查询 | 参考真实读取策略 |
| `go/internal/attachment/model.go`、`repository.go` | 附件元数据及当前版本读取 | 补充 CAD 源版本校验所需字段 |
| `go/internal/http/handlers/exb_handler.go` | `CADSource` 等 | 为提取增加可选的期望版本校验 |
| `go/internal/http/router.go` | 路由和依赖组装 | 注册新接口 |
| `vue/cad/src/features/drawings/pages/DrawingViewerPage.vue` | CAD 在线预览，支持 `fileId` 查询参数 | 复用，不复制页面 |
| `vue/cad/src/features/drawings/detail-tabs/preview/cad-compare.ts` | 已有图纸实体对比 | 本期不改 |
| `vue/cad/src/router/routes.ts`、`route-names.ts` | 前端路由 | 增加两个页面 |
| `vue/cad/src/layouts/components/DesktopSidebar/SidebarNavigation.vue` | 导航及激活状态 | 增加零件索引 |

现有标题栏组件没有完整的人工字段编辑和确认流程。不要把“查看识别结果”当作“人工确认已实现”。

`DrawingCreatePage.vue` 已将项目号保存为 `project`、总图图号保存为 `no`。因此本期固定使用以下映射，不再让实现者猜测。

---

## 3. 业务身份与查询粒度

### 3.1 名称定义

| 名称 | 存储来源 | 用途 |
|---|---|---|
| 项目内部 ID | `drawings.id` | 项目关联、筛选值 |
| 项目显示编号 | `drawings.project` | 界面显示，例如 J1233 |
| 项目名称 | `drawings.name` | 界面显示 |
| 总图/来源图纸图号 | `drawings.drawing_no` | 现有项目页面路由参数 |
| 正式零件 ID | `parts.id` | 已有零件实体关联 |
| 正式零件图号 | `parts.part_no` | 辅助显示，与标题栏识别值分别显示 |
| 标题栏图号 | 提取字段 `number` | 索引中的 `drawingNo` |
| 附件 ID | `attachments.id` | 索引公开地址、预览精确定位 |
| 文件版本 ID | `attachment_versions.id` | 隔离不同文件内容的索引 |

禁止按图号、项目字符串、文件名或 SHA256 合并两个不同附件。

`drawings.project` 不保证唯一。项目筛选使用 `drawings.id`；下拉标签显示“项目编号 / 项目名称 / 总图图号”，避免同编号条目无法区分。

### 3.2 索引候选附件条件

必须同时满足以下条件，才是本期零件索引的候选附件：

1. `attachments.deleted_at IS NULL`。
2. `current_version_id` 非空，并能连接本附件的未软删除版本。
3. `attachments.file_role = 'part'`。
4. 当前版本 `original_name` 的后缀忽略大小写后属于 `.dwg`、`.dxf`、`.exb`。
5. 附件归属满足以下任意一种：
   - `part_id IS NOT NULL`，且至少存在一条 `status='active'` 的 `drawing_part_relations` 连接到仍存在的 `drawings`。
   - `drawing_id IS NOT NULL`，且连接到仍存在的 `drawings`。

直接挂在总图上的 `file_role='part'` 附件也属于候选；`file_role='assembly'` 的总图不属于候选。

不要自动把 `other` 或 `craft` 分类改为 `part`。错误分类通过原有附件管理功能处理。

`drawings.status='archived'`、`'disabled'` 和 `parts.lifecycle_status` 不是删除标记。本期仍允许查询这些历史记录，状态可在详情辅助显示。

没有任何当前有效项目关系的零件附件，不出现在本期列表和详情中。其历史索引保留在数据库，不伪造“所属项目”。

### 3.3 行数和项目关系

- 默认列表：每个候选附件只出现一行。
- 同一附件被三个项目关联：仍是一行，关联项目数量为三个。
- 同一个零件有两个 CAD 附件：出现两行。
- 两个项目中名称相同、附件不同：出现两行。
- 使用项目筛选时，仅保留至少关联该 `drawings.id` 的附件。
- 不能先把关联表展开后直接分页，否则会产生重复行和错误总数。

项目关系来源：

```text
附件 drawing_id 非空：
    attachments.drawing_id → drawings.id
    关系类型返回 direct

附件 part_id 非空：
    attachments.part_id → drawing_part_relations.part_id
    仅 status = active
    drawing_part_relations.drawing_id → drawings.id
    关系类型返回 owned / borrowed
```

每个项目在 `projects` 数组中出现一次。一个项目存在多条关系时，`relationTypes` 去重；显示顺序为 `direct`、`owned`、`borrowed`。

项目列表按项目编号、总图图号、内部 ID 升序排序。默认跳转项目：先 direct，其次 owned，最后 borrowed；同优先级取上述排序第一项。

借用关系为当前关联关系，界面统一写“关联项目”。不要写“历史使用过这些版本”。

### 3.4 标题栏粒度

一个附件版本最多选一个标题栏空间作为当前索引来源。

有效空间定义：该空间至少有一个字段的 `value` 去除首尾空白后非空，或至少有一个非空候选值。

- 恰好一个有效空间：允许自动选中，但处理状态仍是待确认。
- 多个有效空间：不默认选择第一个；提示用户选择。
- 零个有效空间：允许人工填写。
- 同一空间内多个标题栏导致候选冲突：允许人工选值或填写；不得拆出多个索引。
- 候选值数量大于一且提取 `value` 为空：保持空值，不能自动取第一项。

界面明确说明“一期每个文件维护一条零件信息；含多个零件的文件请核对主零件信息”。

---

## 4. 持久化设计

### 4.1 固定采用的方案

新增 `part_indexes` 表，按 `(attachment_id, version_id)` 唯一存储。

公开 API 使用 `attachmentId`，内部默认定位其当前版本。不要再增加另一套随机索引 ID。

保留历史版本行是为了隔离不同版本的人工修正，并允许当前指针切回旧版本时复用该版本的记录。本期没有历史索引查询接口。

查询必须：

```sql
FROM attachments a
JOIN attachment_versions av
  ON av.attachment_id = a.id
 AND av.id = a.current_version_id
 AND av.deleted_at IS NULL
LEFT JOIN part_indexes pi
  ON pi.attachment_id = a.id
 AND pi.version_id = a.current_version_id
```

列表以候选附件为主体，使用 `LEFT JOIN`。没有索引行时返回虚拟“待提取”行，不能因此漏掉附件。

索引行没有时，公开的 `revision` 为 0，所有识别字段为空。`fileName`、项目关系、附件时间仍从原表读取。

### 4.2 新增迁移

基线最新迁移是 000034。默认新增：

`go/database/migrations/000035_part_indexes.sql`

如果实施时该编号已经被占用，使用下一个未占用编号，并更新本文路径和测试引用。禁止修改 000022、000034 的已发布内容。

迁移主体按以下结构实现：

```sql
ALTER TABLE attachment_title_blocks
    ADD COLUMN revision BIGINT NOT NULL DEFAULT 1
    CHECK (revision > 0);

CREATE TABLE part_indexes (
    attachment_id UUID NOT NULL,
    version_id UUID NOT NULL,
    revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
    source_snapshot_revision BIGINT NOT NULL DEFAULT 0
        CHECK (source_snapshot_revision >= 0),
    extraction_status VARCHAR(20) NOT NULL DEFAULT 'pending'
        CHECK (extraction_status IN ('pending', 'extracted', 'failed')),
    extraction_error TEXT NOT NULL DEFAULT '',
    selected_space_id VARCHAR(128),
    selection_mode VARCHAR(10) NOT NULL DEFAULT 'auto'
        CHECK (selection_mode IN ('auto', 'manual')),
    auto_fields JSONB NOT NULL DEFAULT '{}'::jsonb,
    manual_fields JSONB NOT NULL DEFAULT '{}'::jsonb,
    drawing_no VARCHAR(500) NOT NULL DEFAULT '',
    part_name VARCHAR(500) NOT NULL DEFAULT '',
    material VARCHAR(500) NOT NULL DEFAULT '',
    designer VARCHAR(500) NOT NULL DEFAULT '',
    checker VARCHAR(500) NOT NULL DEFAULT '',
    approver VARCHAR(500) NOT NULL DEFAULT '',
    drawing_date_raw VARCHAR(500) NOT NULL DEFAULT '',
    drawing_date DATE,
    scale VARCHAR(500) NOT NULL DEFAULT '',
    sheet_size VARCHAR(500) NOT NULL DEFAULT '',
    metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    confirmed_by UUID REFERENCES users(id) ON DELETE SET NULL,
    confirmed_at TIMESTAMPTZ,
    confirmed_snapshot_revision BIGINT
        CHECK (confirmed_snapshot_revision >= 0),
    edited_by UUID REFERENCES users(id) ON DELETE SET NULL,
    edited_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (attachment_id, version_id),
    FOREIGN KEY (attachment_id, version_id)
        REFERENCES attachment_versions(attachment_id, id) ON DELETE CASCADE,
    CHECK (jsonb_typeof(auto_fields) = 'object'),
    CHECK (jsonb_typeof(manual_fields) = 'object'),
    CHECK (jsonb_typeof(metadata_json) = 'object'),
    CHECK (
      (confirmed_at IS NULL AND confirmed_snapshot_revision IS NULL)
      OR
      (confirmed_at IS NOT NULL AND confirmed_snapshot_revision IS NOT NULL)
    )
);

CREATE INDEX idx_part_indexes_material ON part_indexes(material);
CREATE INDEX idx_part_indexes_designer ON part_indexes(designer);
CREATE INDEX idx_part_indexes_drawing_date ON part_indexes(drawing_date);
```

不保存 `project_code`、`project_id`、`part_id`、文件路径副本。以上身份通过附件及原关系动态取得，防止归属变化后索引失真。

不为 `drawing_no` 或 `part_name` 建唯一约束。

`confirmed_by` 因用户删除而变空时，`confirmed_at` 仍然有效；不能把“用户 ID 为空”作为未确认依据。

### 4.3 原始值、人工值、生效值

三类值必须分开：

1. 原始证据：`attachment_title_blocks.payload`。
2. 当前选中空间的规范化字段：`part_indexes.auto_fields`。
3. 人工完整表单：`part_indexes.manual_fields`。

`manual_fields = {}` 表示用户尚未保存人工表单。

一旦用户保存草稿或确认，`manual_fields` 必须包含第 6 节规定的全部 12 个字段。空字符串也是人工选择，不能回退到自动值。

生效值规则：

```text
manual_fields 是完整表单 → 所有生效字段使用 manual_fields
manual_fields 是 {}     → 所有生效字段使用 auto_fields
两者没有字段            → 使用空字符串
```

表内独立字段列及 `metadata_json` 是生效值的投影，用于筛选和查询。必须由同一个 Go 函数统一计算，不允许调用方分别拼出不同结果。

原始候选值、来源描述和警告只保留在标题栏快照中，不复制到每个独立字段列。

### 4.4 修订号

区分三个概念：

- `versionId`：文件内容版本。
- `snapshotRevision`：同一文件版本的识别快照修订号；没有快照时为 0。
- `revision`：同一索引行的修订号；没有索引行时为 0。

标题栏同版本重复保存相同 JSON，不增加快照修订号。

索引持久化内容实际变化时才增加索引修订号和 `updated_at`。补建相同投影不得反复改更新时间。

快照变化、人工保存、确认、重置会在有实际变化时增加索引修订号。

---

## 5. 权限规则与现有限制

### 5.1 读取规则

核对基线：

- `router.go` 的图纸、附件、CAD 和标题栏入口使用登录中间件。
- `drawing.Repository.List` 的参数不含当前用户，也没有按项目成员过滤。

本期固定采用“与当前项目页面相同的登录后共享读取”规则。

所有零件索引接口必须经过 `RequireAuth`，包括列表、详情、筛选选项。

无登录返回 401。候选附件不存在、已删除、没有有效项目关系或不属于本期范围，详情返回 404。

禁止新增匿名索引接口，禁止直接暴露 blob 路径给新页面。

### 5.2 写入规则

允许写索引和执行单条补建的人员，与标题栏现有写规则一致：

```text
admin
或 attachments.uploaded_by == 当前用户
或直接归属 drawings.created_by == 当前用户
或直接归属 parts.created_by == 当前用户
```

借用项目的创建人不会仅因借用而获得原附件索引修改权。

只读用户可以查看字段和候选值，不显示保存、确认、重置、提取按钮。后端仍独立校验权限，不能仅依赖隐藏按钮。

批量已有快照补建接口仅 admin 可调用。

已归档项目可修正索引，因为本操作只修改检索信息，不修改正式 CAD 或零件版本。页面必须写清楚这一点。

### 5.3 未来接入项目 ACL 的要求

这是后续需求说明，不在本期新增 ACL 表：

1. 列表基础附件集合先按可读项目过滤。
2. `projects` 数组只返回可读项目。
3. 总数、项目数量、筛选选项也基于同一可读范围。
4. 详情、标题栏原始值、CAD 源、下载入口一致校验。
5. 对无权访问的资源返回 404，避免泄露是否存在。

本期交付报告必须明确写“继承当前共享读取；尚未实现按项目成员隔离”。

---

## 6. 字段映射与校验

### 6.1 固定字段名

`auto_fields`、`manual_fields`、API `fields` 使用下列 12 个键：

| 前端原始 key | 本期 API key | 中文标签 | 独立存储列 |
|---|---|---|---|
| number | drawingNo | 标题栏图号 | drawing_no |
| name | partName | 零件名称 | part_name |
| material | material | 材料 | material |
| designer | designer | 设计 | designer |
| checker | checker | 校对 | checker |
| approver | approver | 批准 | approver |
| date | drawingDateRaw | 图纸日期 | drawing_date_raw |
| scale | scale | 比例 | scale |
| 无 | sheetSize | 图幅 | sheet_size |
| process | process | 工艺签署 | metadata_json.process |
| standard | standard | 标准化 | metadata_json.standard |
| company | company | 单位 | metadata_json.company |

不要把 `checker` 自动改称审核，不要把 `process` 当作热处理工艺要求。

本期不增加原始标题栏 `Payload` 的字段 key；`sheetSize` 自动值固定为空，通过人工表单填写。

### 6.2 规范化

统一在后端实现：

1. 去除首尾 Unicode 空白。
2. 字符串内部连续的空格、制表符、换行替换为一个普通空格。
3. 保留大小写、连字符、斜杠、字母后缀和材料原文。
4. 不把 `45` 改写为 `45钢`；不合并 `40Cr` 和 `42CrMo`。
5. 不把文件名猜测结果填入标题栏图号。
6. 每个字段最长 500 个 Unicode 字符，超长返回 400，不截断人工数据。
7. JSON 中不认识的字段和非字符串值返回 400。
8. 展示空值时用 `—`，数据库和 API 空值仍是 `""`。

图纸日期：

- 原文始终存入 `drawingDateRaw`。
- 仅当规范化后严格符合有效的 `YYYY-MM-DD`，才写入 `drawing_date`。
- `2026-02-30`、`2026/9/9`、`26.9.9` 原文保留，规范日期为 null。
- 非标准日期不阻止确认；界面提示“此日期不会参与日期筛选，建议改为 YYYY-MM-DD”。
- 不使用上传日期代替图纸日期。

### 6.3 保存与确认

- 保存草稿允许图号和名称为空。
- 确认要求 `drawingNo`、`partName` 规范化后都非空。
- 材料、设计人、日期等可空，不因老图缺字段而禁止确认。
- 不检查标题栏图号全库唯一。
- 选中布局后仍允许人工覆盖全部字段。
- 没有可用布局或解析失败时，允许 `selectedSpaceId=null` 并人工确认。

---

## 7. 状态及变更规则

### 7.1 原始提取状态

| 条件 | extractionStatus |
|---|---|
| 当前版本无标题栏快照 | pending |
| 当前快照 payload.error 非空 | failed |
| 当前快照存在且 error 为空，包括零个有效空间 | extracted |

提取完成不代表识别正确，不能自动写确认人或确认时间。

### 7.2 页面处理状态

API `status` 按以下优先级从上到下计算：

1. `confirmed_at` 非空，且 `confirmed_snapshot_revision != source_snapshot_revision`：`recheck`，显示“识别结果已变化，待复核”。
2. `confirmed_at` 非空，且两个快照修订号相等：`confirmed`，显示“已确认”。
3. `manual_fields` 为完整表单：`needs_confirmation`，显示“草稿待确认”。
4. `extraction_status='failed'`：`failed`，显示“提取失败”。
5. `extraction_status='pending'`：`pending`，显示“待提取”。
6. 其他：`needs_confirmation`，显示“待确认”。

只允许上述五种 API 状态。不要再创造 `stale`、`ready`、`done` 等同义状态。

### 7.3 同版本重新提取

- 原始快照变化：更新自动字段和快照修订号。
- 已有完整人工表单：保留人工表单及生效值。
- 已确认且原始快照变化：状态为 `recheck`，保留原确认记录用于展示；用户再次确认后记录新快照修订号。
- 未人工保存过：生效值跟随新自动字段。
- 已手工选择的空间仍存在：继续使用该空间。
- 已手工选择的空间消失：保留选择 ID 和人工表单，返回 `selectedSpaceMissing=true`；自动字段清空。用户下一次保存必须选择现存空间或明确改为 null。
- 自动选择模式：每次按第 3.4 节重新选择，多个有效空间时选中 ID 变为 null。
- 解析失败也保存失败快照；人工表单不丢失。

### 7.4 文件换版

V1 切换到 V2 后：

1. 默认查询只连接 V2 的索引和快照。
2. V2 无索引时显示待提取，字段为空。
3. V1 的人工修正、确认人、确认状态不得复制到 V2。
4. V1 行保留，供以后指针回到 V1 时使用。
5. 正在提交的 V1 提取结果或人工表单，因当前版本已是 V2 返回 409。

只要所有读取都正确连接当前版本，就无需为每个版本更新入口增加清理旧索引的异步任务。本期不加数据库触发器。

### 7.5 删除和关系变化

- 附件软删除：立即从候选集合排除，索引行保留。
- 附件恢复：若重新满足候选条件，自动显示当前版本索引。
- 附件或版本硬删除：按外键级联清理对应索引。
- 项目删除：按现有业务规则处理项目及关系，不额外删除被其他项目继续使用的零件索引。
- 借用关系增加或归档：下一次查询直接反映，不复制、删除共享索引。
- 项目编号或名称修改：下一次查询直接读取新值。
- 附件角色变为 `other`：从索引页面消失，历史索引暂保留。

---

## 8. 后端组织与事务算法

### 8.1 文件组织

新增：

```text
go/internal/partindex/
    model.go
    normalize.go
    projector.go
    repository.go
    service.go
    normalize_test.go
    projector_test.go
    repository_test.go

go/internal/http/handlers/
    part_index_handler.go
    part_index_handler_test.go
```

职责：

- `model.go`：DTO、查询条件、输入、错误。
- `normalize.go`：字段映射、规范化、生效值和状态计算。
- `projector.go`：在调用者提供的事务内同步索引。
- `repository.go`：候选附件基础查询、列表、详情和选项。
- `service.go`：权限、人工编辑、单条重建和批量补建。

依赖方向固定：

```text
titleblock → partindex
handlers   → titleblock / partindex
partindex  不得 import titleblock
```

`partindex` 可以定义自己需要的只读快照解码结构，通过 JSON 解码已有 payload，不必搬迁 titleblock 的公共类型。

`SyncCurrentTx(ctx, tx, attachmentID)` 接收调用者提供的 `pgx.Tx`；内部不 Begin、不 Commit，不绕过调用者事务使用连接池写入。

### 8.2 原标题栏保存事务

修改 `titleblock.Repository.Save`，执行顺序：

1. 校验现有 payload 结构和大小。
2. 开事务。
3. 按现有 `attachmentQuery ... FOR UPDATE OF a` 锁定附件。
4. 验证附件存在、未删除、版本未删除、写权限、期望版本。
5. Upsert `attachment_title_blocks`。
6. 首次快照 revision=1；相同 JSON 不更新；不同 JSON 将 revision 加一。
7. 若附件为零件候选类型，调用 `partindex.SyncCurrentTx`。
8. 若为总图、工艺或其他非候选附件，正常保存原快照，跳过索引，不报错。
9. 提交事务。

第 7 步失败时整个事务回滚，不返回“标题栏已保存但索引可能失败”的成功响应。

读取标题栏接口在原字段基础上增加 `snapshotRevision`。无快照返回 0；旧字段名称和含义保持不变。

### 8.3 SyncCurrentTx

调用约定：调用者已锁定附件行。批量补建时也是逐附件开事务并锁附件。

算法：

1. 验证当前候选条件；不符合时返回 skipped，不修改其他数据。
2. 读取当前版本标题栏快照及快照修订号。
3. 读取并锁定同版本索引行（若存在）。
4. 没有快照也没有索引时返回 unchanged，不制造空行。
5. 没有快照但存在人工索引时，保留人工值，提取状态仍 pending。
6. 有快照时按布局选择规则生成 `auto_fields`、状态、错误。
7. 保留 `manual_fields`、编辑人及确认记录。
8. 从生效字段重新计算全部独立列、日期、metadata。
9. 插入或在内容有变化时更新索引；更新时 revision 加一。
10. 返回 `created`、`updated`、`unchanged` 或 `skipped`。

为防止重建清空人工值，所有调用方式必须复用这一算法。

### 8.4 人工保存事务

所有人工写入遵循统一锁顺序：附件 → 快照/索引。不得另一路径反向锁定。

算法：

1. 校验请求语法。
2. 开事务，锁定附件。
3. 检查候选条件及写权限。
4. 检查 `versionId == current_version_id`。
5. 读取当前快照和索引；没有记录分别视为 revision=0。
6. 检查请求 `expectedRevision` 与当前索引 revision 相同。
7. 检查请求 `expectedSnapshotRevision` 与当前快照 revision 相同。
8. 任一不匹配返回 409，不覆盖用户之前的修改。
9. 校验选中空间属于当前快照，或明确为 null。
10. 执行 action，重新生成生效字段和投影。
11. 写入并提交。
12. 返回新详情；详情字段与列表使用同一计算规则。

两人同时第一次编辑同一附件时，附件行锁保证第二人看见已经创建的索引，并返回 409。

动作：

- `save`：保存完整人工表单；`selection_mode='manual'`；清空当前确认时间和确认快照修订号；记录 edited_by/at。
- `confirm`：保存完整人工表单；设置确认人、确认时间、当前快照修订号；记录 edited_by/at。
- `reset`：清空人工表单和确认记录；恢复 auto 模式并重新计算自动空间；记录 edited_by/at。不删除原始快照。

重置只影响当前附件版本。

### 8.5 CAD 源版本绑定

现有前端先读取标题栏快照，再从 `/cad/source` 拉当前 CAD。需要防止两次读取间换版。

固定实现：

1. `attachment.Attachment` 增加 `CurrentVersionID string`，JSON 名 `currentVersionId`。
2. `attachmentSelect` 与 `scanAttachment` 同步读取 `a.current_version_id`。注意所有使用该扫描函数的测试。
3. `CADSource` 增加可选 query `expectedVersionId`。
4. 该参数存在时必须同时提供 `attachmentId`，且两个参数必须是 UUID；否则 400。
5. 查询附件后，若 CurrentVersionID 与 expectedVersionId 不同，返回 409，禁止发送文件。
6. 使用这次附件查询返回的文件对象键读取内容，不再用另一次“当前版本查询”替换源。
7. 原有未传 expectedVersionId 的查看器调用保持兼容。
8. 前端 `parse` 参数改为 `(attachmentId, versionId)`，用首次快照中的 versionId 请求 CAD 源。
9. 最终保存快照仍执行现有当前版本检查。

这不是新增历史版本源接口。版本不匹配时要求重新加载，不静默切换到另一个版本。

---

## 9. HTTP API 合同

### 9.1 通用约定

- 下表是后端完整路径，包含 `/api`。
- 前端 `getApiBaseUrl()` 已提供 API 基地址时，只追加 `/part-indexes` 等后缀，不重复 `/api`。
- 响应使用已有 `response.WriteData` / `WriteError`。
- 成功结构为 `{"data": ...}`；错误保持现有 message 风格。
- UUID 校验后再进入 SQL。
- SQL 使用绑定参数，排序只用固定允许值。
- PATCH 请求体上限 32 KiB。
- 不支持的方法返回 405。

| 方法 | 路径 | 用途 |
|---|---|---|
| GET | `/api/part-indexes` | 列表 |
| GET | `/api/part-indexes/options` | 筛选选项 |
| GET | `/api/part-indexes/{attachmentId}` | 当前版本详情 |
| PATCH | `/api/part-indexes/{attachmentId}` | save / confirm / reset |
| POST | `/api/part-indexes/{attachmentId}/rebuild` | 仅利用快照重建 |
| POST | `/api/admin/part-indexes/backfill` | 管理员批量快照补建 |

路由解析必须先识别 `options`，不能将其当作 UUID。

### 9.2 列表参数

| 参数 | 默认值 | 规则 |
|---|---|---|
| page | 1 | 整数 >=1 |
| page_size | 20 | 整数 1..100 |
| keyword | 空 | 去首尾空白，最多 100 字 |
| project_id | 空 | 单个 drawings.id |
| material | 空 | 规范化后的完整材料值精确匹配 |
| designer | 空 | 规范化后的完整设计人值精确匹配 |
| date_from | 空 | 严格有效 YYYY-MM-DD |
| date_to | 空 | 严格有效 YYYY-MM-DD，包含当日 |
| status | 空 | pending / failed / needs_confirmation / confirmed / recheck |

无效参数返回 400，不静默换成默认值。date_from 大于 date_to 返回 400。

关键词对以下字段做不区分大小写的包含匹配，字段之间 OR：

- 生效标题栏图号。
- 生效零件名称。
- 生效材料。
- 生效设计人。
- 关联项目编号 `drawings.project`。

不同筛选条件之间 AND。日期过滤作用于 `drawing_date`，null 不匹配任何日期区间。

用户输入的 `%`、`_`、反斜杠按普通文字搜索，不能变成 LIKE 通配符。SQL 层统一转义并设置一致的 ESCAPE。

固定排序：当前附件版本创建时间降序、附件 ID 升序。关键词查询不另加模糊排名。

### 9.3 列表响应

```json
{
  "data": {
    "list": [
      {
        "attachmentId": "22222222-2222-4222-8222-222222222222",
        "versionId": "33333333-3333-4333-8333-333333333333",
        "fileName": "J1233-03.dwg",
        "partId": "44444444-4444-4444-8444-444444444444",
        "registeredPartNo": "J1233-03",
        "drawingNo": "J1233-03",
        "partName": "主动轴",
        "material": "40Cr",
        "designer": "张三",
        "drawingDateRaw": "2026-09-09",
        "drawingDate": "2026-09-09",
        "status": "needs_confirmation",
        "extractionStatus": "extracted",
        "canWrite": true,
        "projectCount": 1,
        "projects": [
          {
            "drawingId": "11111111-1111-4111-8111-111111111111",
            "projectCode": "J1233",
            "projectName": "设备项目",
            "drawingNo": "J1233-00",
            "relationTypes": ["owned"]
          }
        ],
        "versionCreatedAt": "2026-09-09T01:00:00Z"
      }
    ],
    "total": 1,
    "page": 1,
    "pageSize": 20
  }
}
```

`partId` 无值时为 null；`registeredPartNo` 无值时为 `""`；`drawingDate` 无效或缺失时为 null。

列表不返回原始 payload、候选值、文件路径。`projects` 返回当前有权看到的全部去重项目，本期读取范围按第 5 节。

### 9.4 详情响应

详情继承列表行字段，并增加：

```json
{
  "revision": 2,
  "snapshotRevision": 1,
  "sourceSnapshotRevision": 1,
  "selectedSpaceId": "model",
  "selectionMode": "auto",
  "selectedSpaceMissing": false,
  "hasManualFields": false,
  "fields": {
    "drawingNo": "J1233-03",
    "partName": "主动轴",
    "material": "40Cr",
    "designer": "张三",
    "checker": "",
    "approver": "",
    "drawingDateRaw": "2026-09-09",
    "scale": "1:2",
    "sheetSize": "",
    "process": "",
    "standard": "",
    "company": ""
  },
  "autoFields": {},
  "extractionError": "",
  "confirmedBy": null,
  "confirmedAt": null,
  "confirmedSnapshotRevision": null,
  "editedBy": null,
  "editedAt": null,
  "createdAt": "2026-09-09T01:00:00Z",
  "updatedAt": "2026-09-09T01:00:00Z",
  "sourcePayload": {
    "spaces": []
  },
  "defaultProjectDrawingNo": "J1233-00"
}
```

上例为字段结构示意；正式 `autoFields` 必须返回与 `fields` 相同的 12 键完整结构，不能省略空字段。

`sourcePayload` 返回当前标题栏原始 payload，没有快照时为 null。`snapshotRevision` 是当前快照真实值；`sourceSnapshotRevision` 是索引上次同步的值。

无持久化索引时，`createdAt`、`updatedAt`、编辑及确认信息为 null。

标题栏与索引发生异常不同步时，详情仍返回两个修订号；UI 提示重新读取或重建。禁止显示“已确认”且隐瞒未同步；此时对外状态强制为 `recheck`，人工提交仍按实际修订号检查。

### 9.5 人工编辑请求

```json
{
  "versionId": "33333333-3333-4333-8333-333333333333",
  "expectedRevision": 2,
  "expectedSnapshotRevision": 1,
  "action": "confirm",
  "selectedSpaceId": "model",
  "fields": {
    "drawingNo": "J1233-03",
    "partName": "主动轴",
    "material": "42CrMo",
    "designer": "张三",
    "checker": "",
    "approver": "",
    "drawingDateRaw": "2026-09-09",
    "scale": "1:2",
    "sheetSize": "A3",
    "process": "",
    "standard": "",
    "company": ""
  }
}
```

- `action` 为 save / confirm 时，必须存在 selectedSpaceId（字符串或 null）和完整 fields。
- action 为 reset 时，只传 versionId、expectedRevision、expectedSnapshotRevision、action；额外传 fields 或 selectedSpaceId 返回 400。
- revision 必须是 >=0 整数，不能缺省。
- 首次保存无索引时 expectedRevision=0。
- 无快照时 expectedSnapshotRevision=0。
- 成功返回 200 和更新后的详情。

### 9.6 单条重建

请求：

```json
{
  "versionId": "33333333-3333-4333-8333-333333333333",
  "expectedSnapshotRevision": 1
}
```

先校验版本、快照修订号及写权限，再调用同一投影函数。读取最新人工表单，不覆盖它，因此此接口不要求 expectedRevision。

没有当前快照时返回 409：“当前版本没有标题栏快照，请先提取信息”。

成功返回 `{"data":{"result":"created|updated|unchanged","detail":{...}}}`。

这条接口不读取 CAD、不启动转换、不调用 AI。

### 9.7 筛选选项

请求：`GET /api/part-indexes/options?kind=project|material|designer&keyword=...&limit=50`。

固定规则：

- kind 必填。
- keyword 最长 100 字，按包含匹配处理且转义通配符。
- limit 默认 50，范围 1..100。
- 从同一候选附件集合产生选项，不受当前页面其他筛选条件影响。
- project 返回 `{value: drawings.id, label: "项目号 / 名称 / 总图图号"}`。
- material、designer 返回 `{value: 原值, label: 原值}`，去重并排除空值。
- 固定按 label、value 升序，取 limit+1 判断是否还有更多。
- 返回 `{"data":{"list":[...],"hasMore":false}}`。
- UI 提示更多结果时要求继续输入关键词，不一次加载所有选项。

### 9.8 管理员补建

请求：

```json
{
  "afterAttachmentId": null,
  "limit": 50
}
```

afterAttachmentId 为 null 或 UUID；limit 范围 1..100。

扫描当前有快照的候选附件，按 attachment_id 升序，并使用 `a.id > afterAttachmentId` 游标。不要使用会因补建而变化的 updated_at 游标。

逐附件开短事务，允许部分成功，不能一个失败就回滚整页：

```json
{
  "data": {
    "scanned": 50,
    "created": 20,
    "updated": 5,
    "unchanged": 23,
    "skipped": 1,
    "failed": 1,
    "nextCursor": "55555555-5555-4555-8555-555555555555",
    "hasMore": true,
    "errors": [
      {
        "attachmentId": "66666666-6666-4666-8666-666666666666",
        "message": "标题栏快照投影或索引写入失败，请重新提取标题栏后重试"
      }
    ]
  }
}
```

`scanned = created + updated + unchanged + skipped + failed`。

扫描到的最后一个附件 ID 作为 nextCursor，即使它处理失败也推进游标。失败项可用单条重建重试或从头再次补建。

每个失败项必须同时满足：

1. `errors` 返回可定位的 attachmentId 和不包含数据库内部细节的可行动提示。
2. 服务端日志记录 attachmentId、失败阶段（事务开始或快照投影/写入）和原始错误。
3. 前端汇总同一次补建的失败详情；最多展示 100 项，并说明未展开数量，避免长任务造成页面内存或渲染压力。

终页 hasMore=false；空页 nextCursor=null。前端以 hasMore 判断结束。

扫描后附件被删除或换版：该条标记 skipped，不能替旧版本强制写入。

### 9.9 错误码

| HTTP | 场景 | 前端行为 |
|---|---|---|
| 400 | UUID、参数、字段、布局选择无效 | 展示具体校验信息 |
| 401 | 未登录或登录过期 | 走现有登录失效逻辑 |
| 403 | 可查看但无权修改 / 非管理员补建 | 保留页面内容，提示无修改权限 |
| 404 | 非候选、不存在、已删除 | 显示记录不可用，提供返回列表 |
| 409 | 文件换版、索引修订变化、快照变化 | 保留未提交表单，提示重新加载后核对 |
| 500 | 数据库或内部错误 | 展示通用信息，服务端记录错误 |

不得把 409 当作普通解析失败并再保存一个失败快照；版本冲突直接结束本次提取。

---

## 10. 查询实现要求

### 10.1 基础查询复用

在 repository 中集中维护：

1. 当前未删除附件及版本条件。
2. 文件类型及角色条件。
3. 有效项目关系条件。
4. 当前读取权限条件。
5. 当前版本索引连接。

列表、详情、count、选项和补建扫描必须复用相同候选定义，不能各自写近似版本。

项目关联搜索用 EXISTS，不能靠 JOIN 展开行后再碰巧 DISTINCT。

列表总数和分页 ID 应在同一个只读一致性事务内取得，避免同时删除时总数与内容明显不一致。

先查询本页附件，再一次批量读取本页项目关系；不要每个附件执行一次项目查询。

### 10.2 当前版本无索引

无索引、但当前有快照时：

- 原始快照可以在详情返回。
- 列表生效字段仍为空。
- `extractionStatus` 根据快照计算。
- `status` 为 failed 或 needs_confirmation。
- 显示“索引尚未建立”，有写权限者可点击重建。
- 不在 GET 请求里偷偷写数据库。

部署后应先做快照补建，降低这种状态的数量。

### 10.3 搜索性能

一期使用数据库包含匹配，不引入新的搜索服务或扩展依赖。

普通字段索引仅用于相应精确筛选，不能宣称它自动加速任意包含搜索。

验收记录真实数据量、查询耗时和测试环境。列表最多返回 100 条；不允许前端下载全部记录后分页。

若实际数据量导致查询不达标，记录慢 SQL 和执行计划再优化，不自行切换数据库。

---

## 11. 前端模块与页面

### 11.1 新增文件

```text
vue/cad/src/modules/part-index/
    part-index.types.ts
    part-index-service.ts
    index.ts

vue/cad/src/features/part-index/
    pages/PartIndexListPage.vue
    pages/PartIndexDetailPage.vue
    components/PartIndexFilters.vue
    components/PartIndexEditor.vue
    components/PartIndexProjects.vue
    components/PartIndexBatchProgress.vue
```

所有新 API 请求集中在 part-index-service.ts。组件不得直接拼 fetch。

标题栏重新提取继续调用已有 drawing-title-block.service.ts。

复用现有 API 基地址、登录 token 和 UI 样式。不引入新的 UI 框架、HTTP 包或独立存储层。

状态优先使用页面级 ref/computed。不要将全部索引复制进 drawing.store，不为本功能建立全量离线缓存。

### 11.2 固定路由

```text
path: /part-indexes
name: part-index-list

path: /part-indexes/:attachmentId
name: part-index-detail
```

在现有 DesktopLayout 的认证 children 中注册。

主导航在“图纸库”后增加“零件索引”。两个路由都激活该导航项。原导航名称保持不变。

详情页标题：“零件索引详情”；字段空时显示文件名，不显示假零件名称。

### 11.3 列表

列固定为：

1. 零件名称。
2. 标题栏图号。
3. 文件名。
4. 关联项目。
5. 材料。
6. 设计人。
7. 图纸日期。
8. 处理状态。
9. 操作：查看。

关联项目显示第一项和“另 N 个”；展开可见完整 projects。不同附件即使名称相同也保持独立行。

工具栏：

- 关键词输入。
- 项目远程选择。
- 材料远程选择。
- 设计人远程选择。
- 日期起止。
- 处理状态选择。
- 重置筛选。
- 对当前页所选可写条目执行“提取所选”。
- admin 显示“补建已有快照索引”。

关键词输入防抖 300ms，修改任何筛选时 page 重置为 1。

筛选和页码写入 route.query，刷新页面和从详情返回时恢复条件。

请求防竞态：AbortController 或递增请求序号，旧响应不得覆盖新筛选结果。加载失败保留筛选，不显示假“无结果”。

无数据时区分：

- 首次无候选附件：“暂无零件 CAD 附件”。
- 有筛选无匹配：“没有找到匹配的零件”。
- 请求失败：错误信息和重试按钮。

### 11.4 详情

从上到下：

1. 返回列表。
2. 零件名称、标题栏图号、文件名、状态。
3. 原始 CAD 预览按钮。
4. 生效字段。
5. 标题栏布局与识别来源、候选值、警告。
6. 人工编辑表单及动作。
7. 关联项目表。
8. 最近人工编辑和确认信息。

固定提示：

> 此处修改仅影响零件索引，不修改 CAD 文件或正式零件资料。

已提取未确认的页面显示：

> 当前信息由系统提取，尚未人工确认。

多有效空间时显示空选择和明确提示。不要默认取第一个。

### 11.5 编辑表单行为

- 初次加载表单使用详情 fields。
- 候选值只通过用户点击填入表单，不自动选中。
- 切换布局时，用该布局规范化后的字段填充表单，sheetSize 保留当前人工值。
- 如果当前表单已经修改，切换布局前确认是否放弃未保存修改。
- 布局切换先只影响本地表单；用户点击保存或确认后才持久化。
- `selectedSpaceId=null` 选项显示“人工填写，不绑定布局”。
- 展示“保存草稿”“确认信息”“恢复识别值”三个按钮。
- “恢复识别值”执行 reset，点击前确认会清除当前版本全部人工字段和确认状态。
- 保存期间禁止重复提交及重新提取。
- 有未保存编辑时重新提取前提示先保存或放弃。
- 409 不自动重试、不自动覆盖：保留本地表单，并提供“重新加载并核对”。
- 重新加载前提醒用户本地未提交内容将被替换，取消则继续保留。
- 无写权限时表单只读，原始候选和来源仍可查看。

### 11.6 原项目入口

SavedDrawingInfo.vue 增加一个可选 prop：

```ts
partIndexEnabled?: boolean
```

默认 false。调用方只在选中文件满足零件附件类型时传 true。

组件使用当前选中附件 ID 生成“零件索引”链接。总图标题栏展示不能无条件显示此链接。

若调用组件只有文件名而无角色信息，在上层拥有 FileView/附件角色的组件判断，不能从文件名猜角色。

不在该旧组件中复制一整套编辑表单，人工编辑统一进入新详情页。

### 11.7 预览及项目跳转

项目链接：

```ts
router.push({
  name: RouteName.DrawingPreview,
  params: { drawingId: project.drawingNo }
})
```

CAD 预览链接：

```ts
router.push({
  name: RouteName.DrawingViewer,
  params: { drawingId: detail.defaultProjectDrawingNo },
  query: { fileId: detail.attachmentId }
})
```

必须使用命名路由传参数，让路由器处理图号中的 `/`、空格等字符，不手工拼未经编码的路径。

已有 DrawingViewerPage 会优先通过 fileId 定位附件。必须验收借用零件和直接挂在项目上的零件附件。

如果精确 fileId 不存在，显示附件不可用，禁止回退为总图文件。

打开预览后显示的是打开时的当前文件。如果打开前发生换版，应显示新版本并清晰标注当前版本；不得承诺固定打开旧详情版本。本期不增加历史预览参数。

---

## 12. 自动提取及历史补齐

### 12.1 新上传

保留现有项目创建后等待转换就绪、再调用 extractCreationTitleBlocks 的流程。

前端保存标题栏后，后端同步索引。不要另外再调用一次“创建零件索引”接口。

其他上传入口只要已调用同一提取服务，就自动获得相同行为。没有调用的入口，本期允许附件先显示待提取；不要宣称关闭浏览器后仍会自动完成前端解析。

### 12.2 浏览器批量提取

“提取所选”只处理当前页用户勾选的候选附件，最多 100 个。

选择规则：

- canWrite=false 的行不能勾选用于提取。
- 默认只允许 extractionStatus 为 pending 或 failed 的行进入此批次。
- 已提取记录要重提取，进入详情操作，不混入批次强制覆盖。

执行规则：

1. 按当前列表顺序串行执行，不并行开启多个 CAD 数据库。
2. 每条调用已有 extractAndSaveTitleBlock；pending 使用 force=false，failed 允许重试。
3. 后端转换未就绪时，该条记录失败原因，继续下一条；不在此处启动新的转换逻辑。
4. 一条失败不影响后续。
5. 显示总数、完成数、成功数、失败数和失败列表。
6. 点击停止：正在处理的一条允许结束，结束后不开始下一条。
7. 批次结束刷新当前列表。
8. 关闭或刷新浏览器会中断未完成任务；已成功保存的数据仍保留。
9. 再次打开通过服务器状态继续选择 pending/failed，不建立仅存在 localStorage 的假任务进度。

前端工作流遇到 HTTP 409 时直接记录版本冲突，不生成和保存 `payload.error`。

普通解析错误可按已有逻辑保存失败快照。API service 应保留 HTTP status，不能只抛丢失状态的 Error。

### 12.3 已有快照补建

admin 在列表点击“补建已有快照索引”：

1. 从 afterAttachmentId=null 开始。
2. 每批 limit=50。
3. 使用响应 nextCursor 请求下一页，直到 hasMore=false 或用户停止。
4. 汇总 created、updated、unchanged、skipped、failed。
5. 不调用 CAD 引擎。
6. 再执行一次应主要返回 unchanged，不清空人工值。
7. 有失败时展示附件 ID 与安全诊断；完整原始错误只在管理员可访问的服务端日志中查看。

升级部署流程中先完成这一步，再组织缺快照附件的浏览器提取。

---

## 13. 必须覆盖的测试

### 13.1 后端纯逻辑测试

至少覆盖：

1. 11 个原始字段准确映射到 12 键表单，sheetSize 为空。
2. checker、approver、process 标签语义不混用。
3. 空白规范化不破坏 `J1233A`、`40Cr` 和带 `/` 的图号。
4. Unicode 长度边界 500/501。
5. 候选值多个且 value 为空时不取第一项。
6. 零个、一个、多个有效空间的选择。
7. 人工字段中的空字符串确实覆盖自动非空值。
8. 无效日期保留原文、规范日期为 null。
9. 五个处理状态优先级，包括人工确认无快照。
10. 已确认后快照变化进入 recheck。
11. 手工选择空间消失的标记及字段保留。
12. 搜索 `%`、`_`、反斜杠的文字转义。

### 13.2 数据库集成测试

新测试使用显式开关 `CAD_PARTINDEX_DB_TEST=1`，连接方式复用已有 titleblock 集成测试。

每次使用独立随机测试 schema。创建测试表和执行真实新增迁移；结束只删除自己创建的 schema。名称只能由固定前缀和程序生成的数字组成，禁止从用户输入拼接。

现有 titleblock 数据库测试使用最小表结构。新增索引同步需要 file_role、关联表等字段，必须同步更新该测试的最小 schema 和 fixture，不能禁用原测试。

至少覆盖：

| 编号 | 场景 | 预期 |
|---|---|---|
| DB01 | 首次保存零件标题栏 | 快照和索引同事务创建 |
| DB02 | 模拟投影写入失败 | 快照写入也回滚 |
| DB03 | 相同快照重复保存/重建 | 不重复行，不无故增加 revision |
| DB04 | 总图附件保存标题栏 | 原功能成功，不创建零件索引 |
| DB05 | 同一附件借用到两个项目 | 列表一行、项目两个、total=1 |
| DB06 | 同名不同附件 | 列表两行 |
| DB07 | V1 确认后切 V2 | V2 不显示 V1 生效字段或确认状态 |
| DB08 | V1 旧结果晚到 | 409，不覆盖 V2 |
| DB09 | 两人相同 revision 编辑 | 一人成功，另一人 409 |
| DB10 | 编辑期间快照变化 | expectedSnapshotRevision 冲突 |
| DB11 | 人工值后同版本重提取 | 人工值保留，状态 recheck |
| DB12 | reset | 恢复自动值，清除确认 |
| DB13 | 软删除/恢复 | 消失/重新出现当前版本记录 |
| DB14 | 删除一个借用关系 | 另一项目及索引仍正常 |
| DB15 | 硬删除版本 | 只级联删除对应版本索引 |
| DB16 | 普通只读用户尝试写 | 403，数据库不变化 |
| DB17 | 借用项目创建人但非附件写入者 | 403 |
| DB18 | 无快照人工确认 | revision 从 0 创建，snapshotRevision=0 |
| DB19 | 已确认无快照后第一次提取 | 人工值保留，进入 recheck |
| DB20 | 项目重命名/改编号 | 索引查询立即反映新值 |
| DB21 | 关系展开及分页 | 无重复、总数正确、页面排序稳定 |
| DB22 | 部分补建失败 | 其他条成功，游标继续推进；返回失败附件和安全诊断，服务端日志含原始错误与阶段 |
| DB23 | 筛选选项 | 排除非候选/删除附件，精确去重 |
| DB24 | 从 V2 切回 V1 | 可复用 V1 索引，不覆盖为 V2 信息 |
| DB25 | 没有索引行 | 列表仍显示候选附件 |
| DB26 | 一个项目内多个相同 part 关系 | projects 去重、relationTypes 合并 |

### 13.3 HTTP 测试

- 所有新入口未登录 401。
- 非 admin 不能批量补建。
- options 不被解析为附件 UUID。
- 无效 UUID、超长 keyword、无效分页和日期返回 400。
- PATCH 缺任意必要键、未知键、fields 非字符串、请求过大返回 400。
- 选中不存在空间返回 400。
- 版本/修订冲突 409。
- 补建响应保留逐附件失败详情；游标 UUID 无论输入大小写均按规范化值传入存储层。
- 非候选附件 404。
- 正常列表、详情、保存、确认、重置和重建结构符合合同。
- CADSource 期望版本不匹配返回 409 且没有文件字节。
- CADSource 不传新增参数的旧调用仍正常。

### 13.4 前端测试

沿用项目已有 Node 测试方式，新建：

```text
vue/cad/scripts/part-index.test.mjs
```

纯函数放入可直接导入的 `.ts` 文件，避免为了测试安装新框架。

测试请求参数生成、12 键表单、状态标签、空值、候选填入、分页重置、批量停止和失败继续。

扩展 `title-block-workflow.test.mjs`：

- parse 收到首次 load 的 versionId。
- 409 不保存失败快照。
- 普通解析错误仍保存失败快照。
- 相同附件批次去重仍正常。

UI 交互通过第 14 节人工验收。不能用只检查文件字符串包含某个按钮名的测试代替行为验证。

---

## 14. 人工验收数据与操作

在隔离开发环境建立以下数据，不写进生产：

| 数据 | 设置 |
|---|---|
| 项目 A | project=J1233，drawing_no=J1233-00 |
| 项目 B | project=J1233A，drawing_no=J1233A-00 |
| 零件 P1 | 主动轴，附件 F1，被 A owned、B borrowed |
| 零件 P2 | 也叫主动轴，附件 F2，只关联 B |
| 文件 F1/V1 | 图号 J1233-03，材料 40Cr |
| 文件 F1/V2 | 图号 J1233-03，材料 42CrMo |
| 文件 F3 | 无识别快照的零件 DWG |
| 文件 F4 | 多有效布局且有候选冲突 |
| 文件 F5 | 总图 assembly 附件 |
| 用户 U1 | F1 的合法写入者 |
| 用户 U2 | 普通已登录用户，非 F1 的写入者 |

逐项检查：

1. 导航可进入列表，刷新和直接输入详情 URL 可用。
2. 搜索“轴”可找到主动轴；J1233A 不被改写为 J1233 的版本。
3. F1 只一行，但有两个关联项目；F2 是另一行。
4. F5 不出现在零件索引列表，但原标题栏功能仍可用。
5. F3 显示待提取，可以人工填写或执行提取。
6. F4 需要选择布局，不默认取第一项；候选可以点选和修改。
7. 把 F1 材料人工改为其他值后重提取，人工值不会被覆盖。
8. 未确认、已确认、待复核的提示准确。
9. F1 切换到 V2，详情和列表不展示 V1 的已确认材料。
10. 编辑过程中另一人修改或换版，提交得到冲突提示，本地输入保留。
11. 用 F1 的“查看图纸”打开的是 F1，不是项目总图。
12. 带 `/` 的总图图号能跳转。
13. U2 能按当前共享读取策略查看，但不能修改、确认、重建。
14. 刷新、返回列表保留搜索条件和页码。
15. 批量提取一条失败后下一条继续，停止后不再开始新任务。
16. 管理员重复补建不会丢失人工字段或增加重复记录。
17. 删除 B 的借用关系，A 的 F1 仍正常。
18. 项目改编号，列表和筛选标签立即使用新值。
19. 检查数据库及文件目录：索引操作没有新增 DWG 文件、blob 或正式零件版本。
20. 记录实际列表总量、默认查询和关键词查询耗时。

F1/V2 中出现 42CrMo 是测试数据，不要求修改真实 CAD 文件制造该值；可使用隔离 fixture 或已准备好的测试图纸。

---

## 15. 实施顺序与每步完成条件

必须按步骤执行。每步完成后更新 `docs/part-index/progress.md`。

### 步骤 0：基线记录

操作：

1. 阅读根 AGENTS.md。
2. 执行 git status，记录已有修改。
3. 阅读本文第 2 节列出的相关文件，不扫描无关目录。
4. 确认最新迁移编号、依赖安装情况、数据库测试连接方式。
5. 创建 progress.md，抄入下方任务清单。

完成条件：有实际基线记录，没有改动业务代码。

### 步骤 1：模型和迁移

操作：

1. 新增迁移。
2. 新增 partindex DTO、错误类型。
3. 实现 12 键映射、规范化、日期、生效值、状态和空间选择纯函数。
4. 写纯逻辑测试。

完成条件：纯逻辑测试通过；迁移在隔离 schema 执行成功。

### 步骤 2：版本绑定

操作：

1. 扩展附件 CurrentVersionID 查询和扫描。
2. CADSource 支持 expectedVersionId。
3. 前端 parse 接收 versionId。
4. 为 API 错误保留 status，409 不保存错误快照。
5. 更新相关原测试。

完成条件：源文件版本不匹配被拒绝；原预览无新增参数仍可用。

### 步骤 3：索引投影与标题栏事务

操作：

1. 实现候选判断和 SyncCurrentTx。
2. titleblock 增加 snapshotRevision。
3. 原 Save 同事务同步索引。
4. 更新原 titleblock 集成测试 fixture。
5. 验证事务回滚、幂等、非零件跳过。

完成条件：保存零件快照生成索引，总图快照不受影响。

### 步骤 4：查询 API

操作：

1. 实现列表、count、详情、项目关系批量查询。
2. 实现 options。
3. 实现统一过滤、排序、分页。
4. 注册路由。
5. 完成读取和参数相关 HTTP/DB 测试。

完成条件：通过真实 API 可查询到正确、不重复的记录。

### 步骤 5：人工编辑 API

操作：

1. 实现 save、confirm、reset。
2. 完成两个修订号及 versionId 校验。
3. 实现读写权限差异。
4. 完成并发、人工值保留、状态转换测试。

完成条件：两人编辑不会覆盖；新版本不会继承旧确认。

### 步骤 6：补建 API

操作：

1. 单条 rebuild。
2. admin 游标 backfill。
3. 验证部分失败、跳过变化记录、幂等和人工值保留。

完成条件：已有快照能补齐，补建不会启动 CAD 解析。

### 步骤 7：前端列表

操作：

1. 新增 types 和 service。
2. 新增路由与侧栏。
3. 实现列表、分页、筛选、URL 状态、防竞态和错误态。
4. 接入真实 API。

完成条件：所有列表条件可用，刷新恢复，无前端全量分页。

### 步骤 8：详情及原页面集成

操作：

1. 生效字段、原始候选、空间选择、编辑表单。
2. 接入保存、确认、重置及冲突处理。
3. 关联项目跳转和精确 fileId 预览。
4. SavedDrawingInfo 增加受控入口。

完成条件：一次完整“提取→修正→确认→搜索→预览”真实链路通过。

### 步骤 9：批量操作

操作：

1. 当前页选择、串行提取、停止和进度。
2. 管理员补建 UI。
3. 完成前端批次行为测试。

完成条件：失败继续、停止有效、刷新后状态以服务器为准。

### 步骤 10：回归与交付

操作：

1. 执行第 16 节命令。
2. 完成第 14 节人工验收。
3. 查看 git diff，确认没有修改正式零件、文件复制及无关重构。
4. 补全 progress.md。
5. 提交实施报告，注明未满足的原始需求和未执行验证。

完成条件：所有必做任务有结果；剩余问题真实列出，不能用“基本完成”掩盖失败。

### progress.md 模板

```markdown
# 零件索引一期实施进度

## 基线
- 开始日期：
- 基线 commit：
- 开始时未提交修改：
- 使用迁移编号：
- 数据库测试方式：

## 步骤
- [ ] 0 基线
- [ ] 1 模型和迁移
- [ ] 2 版本绑定
- [ ] 3 投影和事务
- [ ] 4 查询
- [ ] 5 人工编辑
- [ ] 6 补建
- [ ] 7 前端列表
- [ ] 8 详情集成
- [ ] 9 批量操作
- [ ] 10 回归交付

## 本次修改
- 文件：
- 行为：

## 验证
- 命令：
- 结果：通过 / 失败 / 未执行
- 未执行原因：

## 待处理问题
- 问题、影响、下一步：

## 固定边界
- 当前继承登录后共享读取，未实现项目成员 ACL。
- 本期索引不改 CAD、不改正式零件版本。
```

---

## 16. 验证命令与交付格式

以下命令由实施模型在完成对应代码后执行。本次文档编写不代表这些新增测试已经存在或执行。

后端，在 `go` 目录：

```powershell
go test ./internal/partindex ./internal/titleblock ./internal/http/handlers
go test ./...
```

数据库测试，在具备隔离 schema 创建权限的测试数据库上：

```powershell
$env:CAD_PARTINDEX_DB_TEST = "1"
$env:CAD_TITLEBLOCK_DB_TEST = "1"
go test ./internal/partindex ./internal/titleblock -count=1
Remove-Item Env:CAD_PARTINDEX_DB_TEST
Remove-Item Env:CAD_TITLEBLOCK_DB_TEST
```

若环境变量执行前已存在，运行后恢复原值，不能无条件删除别人的配置。

前端，在 `vue/cad` 目录：

```powershell
npm run type-check
npm test
node --experimental-strip-types --test scripts/cad-title-block.test.mjs scripts/title-block-workflow.test.mjs scripts/part-index.test.mjs
npm run test:compare
npm run build
```

不强制升级 Node、Go、CAD 依赖或 lock 文件。如果当前环境无法运行，记录实际版本、命令和错误，不把环境修复扩大成依赖全面升级。

提交说明至少包含：

1. 已完成的功能和页面。
2. 新迁移文件及执行要求。
3. 修改的主要文件。
4. 自动化测试命令与真实结果。
5. 人工验收结果。
6. 未执行的测试和原因。
7. 已知边界：共享读取、多零件单文件限制、前端提取受浏览器生命周期影响。
8. 是否产生 DWG 副本：应为否。
9. 是否修改正式 parts/part_revisions：应为否。
10. 如有偏离本文，逐条列出原因、最终行为及对应测试。

---

## 17. 常见错误禁止清单

- 不要因为后端 EXB identify 只猜文件名，就重写前端已存在的标题栏提取。
- 不要把项目号 `J1233A` 拆成项目 J1233 和版本 A。
- 不要把当前所有 CAD 附件都当成零件图。
- 不要把“同名”当成“同一零件”。
- 不要把一个共享附件按项目复制成多条当前列表记录。
- 不要按项目编号字符串反查并猜测附件归属。
- 不要把标题栏图号修正写入 `parts.part_no`。
- 不要把索引材料修正写入 `part_revisions.material`。
- 不要直接覆盖 CAD 标题栏文字。
- 不要将第一候选、第一布局静默认定为正确结果。
- 不要用 `value || autoValue` 处理人工空字符串。
- 不要在文件换版后沿用旧版本“已确认”标记。
- 不要在 GET 中补建索引。
- 不要在标题栏事务提交后再另起一个无保证的写索引请求。
- 不要从前端上传人显示名判断权限，后端必须用用户 UUID。
- 不要只过滤列表而让详情或候选值匿名可读。
- 不要把重建索引写成后台打开 DWG 的接口。
- 不要将浏览器批次进度描述为可靠后台任务。
- 不要用客户端传入的 storageKey 当作附件身份。
- 不要修改已发布迁移或清空数据库来适配新表。
- 不要为通过测试而删掉原有 titleblock 测试。
- 不要在没有运行数据库测试时声称版本一致性和事务已经验证。
