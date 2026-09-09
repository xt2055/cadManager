# 零件索引一期实施进度

## 基线

- 开始日期：2026-09-09。
- 基线 commit：`ba9c569 docs: add part index implementation specification`。
- 开始时未提交修改：`.zcode/`（未纳入本功能、保持未跟踪）。
- 使用迁移编号：`000035_part_indexes.sql`。
- 数据库测试方式：已有 `CAD_TITLEBLOCK_DB_TEST=1` 隔离 schema 测试；2026-09-09 深入审查已在独立临时 PostgreSQL 实例执行通过。当前没有完整的 `CAD_PARTINDEX_DB_TEST` 测试套件。

## 步骤

- [x] 0 基线
- [x] 1 模型和迁移
- [x] 2 版本绑定
- [x] 3 投影和事务
- [x] 4 查询
- [x] 5 人工编辑
- [x] 6 补建
- [x] 7 前端列表
- [x] 8 详情集成
- [x] 9 批量操作
- [x] 10 自动化回归
- [ ] 11 部署前数据库集成测试与人工验收

## 本次修改

- 新增 `part_indexes` 迁移、投影、查询、人工保存/确认/恢复、单条重建和管理员快照补建。
- 标题栏快照保存后在同一事务同步当前零件附件版本的索引。
- 前端复用已有浏览器 CAD 标题栏提取，增加版本绑定；CAD 源返回 HTTP 409 时不写失败快照。
- 新增 `/part-indexes` 列表、详情与编辑页，含 URL 筛选、请求取消、候选值手工填入、批量串行提取、停止与管理员补建。
- 原项目标题栏区域仅在已知零件图上下文提供“零件索引”入口。
- 审查修复：详情查询统一附件 ID 的文本类型；批量提取后按返回快照重建索引；路由恢复筛选不再重置页码；重建索引会确认未保存修改并锁定编辑控件。
- 深入审查修复：历史标题栏 `null` 集合统一规范化为空数组；保存后换版返回 409；确认必填项正确返回 400；12 个字段拒绝 `null`/非字符串；日期统一限制为 0001-01-01 至 9999-12-31；所有 UUID 入口统一小写规范化。
- 管理员补建失败保留附件 ID 与安全诊断，服务端记录原始异常和失败阶段；前端显示本次失败清单，最多 100 项。

## 验证

- 已执行：`go test ./internal/partindex ./internal/titleblock ./internal/http/handlers`（通过）。
- 已执行：`go test ./...`（通过）。
- 已执行：`node --experimental-strip-types --test scripts/cad-title-block.test.mjs scripts/title-block-workflow.test.mjs scripts/part-index.test.mjs`（28/28 通过）。
- 已执行：`npm test`（通过）。
- 已执行：`npm run test:compare`（23/23 通过）。
- 已执行：`npm run type-check`、`npm run build`（通过）。生产构建仅出现既有的 CAD 依赖外置和大包体提示，未出现构建失败。
- 已执行：`git diff --check`（通过）。
- 已执行：`CAD_TITLEBLOCK_DB_TEST=1 go test ./internal/titleblock -run TestRepositoryDatabase -count=1 -v`（独立临时 PostgreSQL，通过；实例测试后已停止）。
- 深入审查未连接项目数据库，尚未完成完整迁移链及真实 CAD 浏览器验收。详见 [深入审查报告](review-2026-09-09.md)。

## 待处理问题

- 数据库集成测试尚未落地完整 DB01–DB26 覆盖；本轮已在隔离 PostgreSQL 实测部分事务、并发及关系行为，扩展审查探针仍需整理为持久回归测试。
- 深入审查确认的 R1–R6 与补建失败诊断链路已修复；触发条件、修复状态和未完成验收范围见 [深入审查报告](review-2026-09-09.md)。
- 浏览器批量提取依赖页面生命周期；关闭或刷新页面会中断未开始的项目，已成功保存的快照与索引保留。

## 固定边界

- 当前继承登录后共享读取，未实现项目成员 ACL。
- 本期索引不改 CAD、不改正式零件版本。
- 不生成 DWG 副本。
