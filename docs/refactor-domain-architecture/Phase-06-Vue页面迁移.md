# Phase 6：迁移 Vue 页面

## 目标

逐功能替换 useDomainStore()，每迁移一个页面都保持可运行、可验证。

## 迁移顺序

先迁依赖简单的页面：

```text
Admin
Operation Logs
Review
```

然后迁移：

```text
Drawing Library
Drawing Detail
Material
Craft
Attachment
```

最后迁移：

```text
DrawingCreatePage
DrawingPreviewTab
复杂 CAD 页面
```

## 单页面验收

每迁移一个页面，必须保证：

- 页面行为不变。
- 类型检查通过。
- 相关测试通过。
- useDomainStore() 调用数量减少。
- 页面通过 Application Service 执行命令，通过 Store 读取 Read Model。

## Borrow UI

借用件不能显示普通编辑：

```text
[分叉并编辑]
```

关系属性使用独立动作：

```text
[编辑关系]
```

关系属性包括：

```text
qty
remark
position
```

不要把“修改借用关系”和“修改 Part 内容”混成一个按钮。

## 阶段检查

```text
rg "useDomainStore" src/features
```

目标结果：

```text
0
```

## 验收标准

- [x] 所有目标页面切换到新的页面 Store/Operations 边界。
- [x] 页面不直接访问旧 Store、dataManager 或旧 bulk API。
- [x] Borrow UI 保持借用关系查询与零件编辑入口分离。
- [x] 相关类型和页面构建检查通过。

## 当前实施记录

执行日期：2026-09-04

- 已迁移管理员图纸、管理员操作日志、账号管理、系统日志、更新管理和审核流程页面到对应的 Application Service/Store。
- 已迁移审核待办、已办归档、普通操作日志和工作台首页到新的 Review/Audit/Drawing Store。
- 管理后台布局初始化已改为加载 `admin.store` 与 `review.store`，不再依赖旧 `domain.store` 初始化。
- 图纸库页面已改为使用 Drawing Read Model 与 Attribute Store，列表筛选不再读取领域实体。
- 图纸详情布局、标题栏、属性 Tab 和结构 Tab 已改为使用 Drawing Read Model、Workspace Store、Attribute Store 与 Audit Store；结构树不再直接持有 `StructurePart` 实体。
- 图纸版本和借用 Tab 已改为使用 Drawing Read Model、Drawing Relations Store 与 Versioning Service；分支、借用关系查询已从旧 Store 独立出来。
- 文件历史、预览、创建页、Material/Craft/Review 以及 Viewer/Editor 页面已切换到 `Drawing Operations Store` 边界；features 目录中 `useDomainStore()` 直接调用数已清零。
- 尚未拆出的复杂写事务集中在 `Drawing Operations Store` 内，后续 Phase 7 再逐项替换为原子 Application Service；本阶段 `npm run type-check`、`npm run build` 已通过。

## 建议提交

```text
refactor(admin-ui): migrate to admin module
refactor(review-ui): migrate review pages
refactor(drawing-ui): migrate drawing library
refactor(drawing-ui): migrate detail tabs
refactor(drawing-ui): migrate drawing creation
```
