# Phase 1：重建最终数据库模型

## 目标

在开发环境直接建立最终数据库模型。允许 reset database，不做旧数据迁移和旧数据修复。

## `parts`

```text
id
part_no
normalized_part_no UNIQUE
published_revision_id NULL
lifecycle_status: active | obsolete | archived
forked_from_part_id NULL
created_by
created_at
updated_at
```

`part_no` 保存展示值，`normalized_part_no` 由后端统一规范化后参与唯一约束。

## `part_revisions`

```text
id
part_id
revision_no
version
row_revision
name
material
spec
weight
surface_treatment
part_type
workflow_status: draft | reviewing | published | rejected
based_on_revision_id NULL
created_by
created_at
published_by NULL
published_at NULL
```

规则：

- Draft 可编辑，所有修改使用 `row_revision` CAS。
- Reviewing 不可普通编辑，只能审核通过或驳回。
- 驳回回到 Draft，可继续修改同一 Revision。
- Published 永久不可修改。
- 新内容必须创建新的 Revision。

`revision_no`、`version`、`row_revision` 三者语义不能混用。

## `drawing_part_relations`

```text
id
drawing_id
part_id
parent_relation_id NULL
relation_type: owned | borrowed
qty
position NULL
line_no NULL
remark NULL
borrow_reason NULL
borrowed_by NULL
borrowed_at NULL
status: active | archived
revision
forked_from_relation_id NULL
created_by
created_at
updated_by
updated_at
```

约束：

- 一个 Part 最多一个 active owned relation。
- 同一个 Part 可以在同一个 Drawing 中出现多个 usage relation。
- 不建立 `UNIQUE(drawing_id, part_id)`。
- `parent_relation_id` 必须属于同一个 Drawing。
- 结构不能成环。

## 附件版本模型

```text
attachments
attachment_versions
blobs
part_revision_attachments
```

关系：

```text
PartRevision
  ↓
PartRevisionAttachment
  ↓
AttachmentVersion
  ↓
Attachment
  ↓
Blob
```

`part_revision_attachments` 只保存：

```text
part_revision_id
attachment_version_id
role
sort_order
```

不重复保存 `attachment_id`，从模型上消除附件和附件版本错绑的可能。

## BOM

建立 Drawing 级 BOM Revision：

```text
drawing_boms
drawing_id
revision
updated_at
updated_by
```

下面关联 `bom_items`。BOM CAS 独立于 Drawing revision。

## 必须落实的约束

- `normalized_part_no UNIQUE`。
- 一个 Part 一个 active owned relation。
- 父关系属于同一个 Drawing。
- Published Revision 属于当前 Part。
- `published_revision_id` 只能由发布事务推进。
- 同一 Drawing 的结构写操作使用事务级锁并执行 cycle check。

## Seed

重新建立示例：

```text
Drawing
Part
PartRevision
owned relation
borrowed relation
附件
BOM
审核
```

## 验收标准

- [x] 空数据库可执行全部 migration。
- [x] 开发库 reset 后可执行最终 seed。
- [x] DB constraint tests 通过。
- [x] 旧 `structure_parts` 不再作为最终数据模型。

## 本次实施记录

执行日期：2026-09-04

- 新增 `go/database/migrations/000022_rebuild_final_domain_model.sql`，在开发库中一次性重建最终领域模型。
- 更新 `go/database/bootstrap.dev.sql`，启动初始化会包含 000021 和 000022。
- 新增 `go/database/seed_dev.sql`，建立管理员、审核流程、示例图纸、Part、Published Revision、owned/borrowed relation 和 BOM。
- 新增 `go/database/verify_final_schema.sql`，验证最终表、关键约束、Published 不可变、跨图纸父关系拒绝和 owned relation 规则。
- 已在本地开发库完成 reset/rebuild/seed/constraint verification，执行成功。
- 当前本地数据库关键结果：`drawings=2`、`parts=1`、`part_revisions=1`、`drawing_part_relations=2`；`structure_parts`、`file_versions`、`borrow_records` 已不再存在。
- 所有最终模型表已由当前应用数据库用户拥有，应用可继续执行后续迁移。

## 阶段边界

Phase 1 只负责最终 Schema、开发 seed 和数据库约束验证。现有 Go 运行代码仍有部分 Repository 指向旧表和旧附件字段，这是 Phase 2 的迁移内容；在 Phase 2 完成前不能把旧后端程序视为兼容最终 Schema。

## 建议提交

```text
refactor(db): introduce part revision model
refactor(db): introduce drawing part relations
refactor(db): version bom and part attachments
test(db): add domain constraint tests
```
