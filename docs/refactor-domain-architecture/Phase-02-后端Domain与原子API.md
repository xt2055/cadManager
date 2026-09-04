# Phase 2：后端 Domain 和原子 API

## 目标

让后端成为最终业务权威，前端核心业务全部通过原子 API 完成。

## Domain

实现：

```text
Part
PartRevision
DrawingPartRelation
```

并集中实现生命周期、借用、Fork、结构和并发规则。

## Part Number Normalizer

后端统一执行：

```text
trim
Unicode normalize
全角 → 半角
大小写规范化
```

前端只做提示，后端是最终权威，不能由前后端各实现一份不一致的算法。

## StructurePolicy

统一负责：

- parent 是否属于同一 Drawing。
- cycle detection。
- owner 唯一性。
- obsolete 校验。

同一 Drawing 的结构写操作使用 drawing-level lock，使“读取结构、检查环、写入关系”保持串行。

## Part Revision API

支持：

```text
创建 Draft Revision
更新 Draft Revision
提交 Review
Reject
Publish
```

Publish 必须单事务完成：

```text
审核通过
  ↓
Revision.workflow_status = published
  ↓
Part.published_revision_id = Revision
  ↓
写 Audit
  ↓
Commit
```

## Borrow API

```http
POST /api/drawings/{drawingId}/borrows
```

负责：

- 验证 source Part。
- 验证 Published Revision 存在。
- 验证 Part active。
- 验证结构不成环。
- 获取 Drawing lock。
- 创建 borrowed relation。
- 写 Audit。

V1 只借用单个 Part，不隐式借用装配子树。

## Relation API

```http
PATCH /api/drawing-part-relations/{id}
```

支持修改：

```text
qty
remark
position
parentRelationId
```

请求必须携带 `expectedRevision`。

## Part Fork API

```http
POST /api/drawing-part-relations/{id}/fork
```

事务流程：

```text
验证 relation = borrowed
  ↓
读取 source PublishedRevision
  ↓
检查 newPartNo
  ↓
创建新 Part
  ↓
创建 Draft PartRevision
  ↓
复用 AttachmentVersion / Blob
  ↓
归档原 borrowed relation
  ↓
创建 owned relation
  ↓
保存 Part lineage 和 Relation lineage
  ↓
写 Audit
  ↓
Commit
```

支持持久化 `idempotencyKey`，重复请求返回第一次结果。

## BOM API

```http
GET /api/drawings/{id}/bom
PUT /api/drawings/{id}/bom
```

请求包含：

```text
expectedRevision
items
```

只更新当前 Drawing 的 BOM。

## Attribute 原子 API

替代旧的：

```text
PUT /api/data/attributes
```

改为独立的：

```text
POST
PATCH
DELETE
```

## 验收标准

新业务不依赖以下接口即可完成核心操作：

```text
/api/data/structure
/api/data/bom
/api/data/borrows
/api/data/branches
```

必须测试：

- revision conflict。
- duplicate part number。
- cycle。
- obsolete borrow。
- fork idempotency。
- publish transaction。

## 建议提交

```text
feat(part): add revision lifecycle
feat(structure): add relation commands and locking
feat(borrow): add atomic borrow command
feat(part): add borrowed part fork transaction
feat(bom): add drawing scoped revisioned api
feat(attribute): add atomic attribute api
```

