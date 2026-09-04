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

- [ ] 所有目标页面切换到新模块。
- [ ] 页面不直接访问旧 Store、dataManager 或旧 bulk API。
- [ ] Borrow UI 明确区分“编辑关系”和“分叉并编辑”。
- [ ] 相关类型、单元和页面测试通过。

## 建议提交

```text
refactor(admin-ui): migrate to admin module
refactor(review-ui): migrate review pages
refactor(drawing-ui): migrate drawing library
refactor(drawing-ui): migrate detail tabs
refactor(drawing-ui): migrate drawing creation
```

