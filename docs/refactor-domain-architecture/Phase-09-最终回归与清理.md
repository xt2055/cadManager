# Phase 9：最终回归和清理

## 目标

证明新架构不仅能够编译，而且业务流程、并发规则、桌面编辑和上传链路完整可用。

## 当前回归记录

执行日期：2026-09-04

- `go test ./...` 通过，包含 HTTP 路由、原子命令和附件/版本相关测试。
- 前端 `npm run test`、`npm run type-check` 与 `npm run build` 通过。
- 旧 Store、全量快照持久化、JSON Debug、DataManager、旧 `/api/data/` 路由、bulk write Handler 和 Part clone borrow 扫描均为 0。
- `git diff --check` 通过。
- 需要真实数据库、对象存储、SMB/CAXA 和浏览器部署环境的业务 E2E 保留为部署后手工回归项。

## 后端测试

```text
go test ./...
```

重点覆盖：

- Part number normalize。
- Part owner uniqueness。
- Draft CAS。
- Published immutable。
- Review publish。
- Borrow。
- Fork。
- Fork idempotency。
- Cycle。
- Drawing lock。
- Obsolete。
- BOM revision。
- Attachment version。
- Blob reuse。

## 前端检查

```text
npm run type-check
npm run test
npm run build
```

## Drawing 回归

- 新建。
- 修改。
- 归档。
- 恢复。
- 分叉。

## Part 回归

- 创建。
- Draft 修改。
- Review。
- Published。
- 创建下一版本。
- Obsolete。

## Borrow 回归

- 借用。
- 数量修改。
- 位置修改。
- 源 Part 发布新版。
- 借用方自动看到新版。
- 借用方 Fork。
- Fork 后源 Part 更新不再影响新 Part。

## Structure 回归

- 拖拽。
- parent 修改。
- 环检测。
- 同一 Part 多位置使用。
- 跨 Drawing parent 被拒绝。
- 并发结构修改被正确串行化。

## Attachment 回归

- 上传。
- 分片。
- 断点恢复。
- CAD 转换。
- 转换失败。
- 转换重试。
- replace。
- 版本历史。
- Fork Blob reuse。
- Published Revision 文件历史可重建。

## BOM 回归

- 读取。
- 更新。
- CAS conflict。

## Review 回归

- 提交。
- 驳回。
- 重新修改。
- 再次提交。
- 发布。

## Desktop 回归

- SMB。
- CAXA。
- 编辑 session。
- 保存新版本。
- 票据过期。
- 工作副本命名和真实文件扩展名正确。

## 文档清理

更新：

```text
README
Architecture
Database Schema
API Contract
Deployment
Desktop Compatibility
```

明确：

```text
旧客户端不兼容
数据库可重置
/api/data 已删除
```

## 最终 Definition of Done

以下全部满足：

```text
useDomainStore        = 0
dataManager usage     = 0
persist()             = 0
bulk /api/data writes = 0
Part clone for borrow = 0
Published mutation    = impossible
Borrowed direct edit  = impossible
structure cycle       = impossible
stale revision write  = rejected
Fork retry duplication = impossible
```

并且：

```text
npm type-check ✓
npm build       ✓
frontend tests  ✓ (architecture regression)
go test ./...   ✓
API tests       ✓
business E2E    ✓
```

达到以上标准后，refactor/domain-architecture 才可以合并。

## 建议清理提交

```text
test: complete architecture migration regression
docs: finalize architecture migration
chore: remove obsolete migration artifacts
```
