# 主题与数据清理

## 目标

- 根据本机时间决定应用启动时的浅色或深色模式。
- 增加经典皮肤，支持经典白和经典暗。
- 移除界面中的演示业务数据，空页面显示明确空状态。

## 已确认上下文

- 主题状态位于 `src/stores/theme.store.ts`，由 `html[data-skin]` 和 `html[data-theme]` 驱动。
- 当前皮肤有 `elegant`、`tech` 两种，样式位于 `src/styles/themes/`。
- 演示数据集中在 `src/stores/demo.store.ts`，但固定业务文案也存在于工作台、通知、审核历史、详情页和弹窗。
- `src/App.vue` 当前启动提示包含 Demo 文案，需要移除或改为非业务提示。

## 实现决策

- 使用 `Date#getHours()`：06:00（含）至 18:00（不含）为 `light`，其他时间为 `dark`。
- 时间仅用于初始化；现有手动明暗切换保留，不在运行中强制覆盖用户选择。
- 新增 `classic-light.css` 与 `classic-dark.css`，经典皮肤使用中性白/灰和传统深灰配色，不使用科技网格和霓虹效果。
- 清空演示数据数组，并为依赖数据的页面补充空状态或安全兜底。

## 验证

- `bun run type-check`：通过。
- `bun run build-only`：通过。
- 固定业务编号、人员、数据库状态、Demo 提示和 CAD 样例文本已完成搜索清理。
