# Phase 0：冻结设计与建立基线

## 目标

在修改数据库和架构前，固定当前系统能力和新系统必须保持的用户可见行为，避免把既有问题误判为重构回归。

## 领域规则

- `Part` = 全局唯一工程零件身份。
- `PartRevision` = 零件工程内容版本。
- `DrawingPartRelation` = Drawing 对 Part 的使用实例。
- `Borrowed` = 动态引用源 Part 最新 Published Revision。
- Borrowed Part 不允许直接修改，修改 Borrowed Part 必须 Fork。
- Fork 后生成新的 Part，与源 Part 独立演化。
- `qty`、`parent`、`position`、`remark` 属于 Relation。
- Published 永久不可修改。
- Reviewing 禁止普通编辑。
- Draft 使用 `row_revision` CAS。
- Obsolete 阻止后续项目发布。
- 结构禁止形成环。

## V1 范围

V1 只支持：

```text
Borrow = 单 Part 动态引用
```

暂不实现：

- 完整 Assembly Structure Revision。
- 借用装配子树。
- 通用 BOM 图模型。

## 基线检查

前端：

```text
npm install
npm run type-check
npm run build
npm run test
```

后端：

```text
go test ./...
```

记录每项命令的日期、结果和失败原因。已有失败必须单独登记，不能直接归因于重构。

## 本次基线记录

执行日期：2026-09-04

| 检查项 | 结果 | 说明 |
|---|---|---|
| `go test ./...` | 通过 | 后端全部现有测试通过 |
| `npm run type-check` | 通过 | Vue/TypeScript 类型检查通过 |
| `npm run build` | 通过 | 前端生产构建通过；存在 Vite 动态 URL 和大 chunk 警告 |
| `npm run test` | 未执行 | `package.json` 没有 `test` script，前端测试命令当前不存在 |

当前构建警告不作为 Phase 0 阻塞项，但在最终回归时需要重新确认没有新增警告。

## 现有测试覆盖盘点

当前后端已经存在并通过测试的行为基线包括：

- 上传服务的安全文件名、SHA-256 规范化、TTL 辅助逻辑。
- 版本创建、恢复、并发、回滚和存储键处理。
- CAD 转换、缺失转换产物扫描和非 CAD 文件处理。
- 编辑会话、工作副本、并发关闭和管理员强制关闭。
- 附件下载、当前 Blob 文件名和无记录文件处理。
- EXB 图号解析、回退图号和标题栏处理。
- 存储路径安全、路由、认证、审核日志和系统初始化。

以下内容不在当前旧架构中提供可独立注入的业务接口，因此不在 Phase 0 伪造单元基线：

- Borrow 的数据库集成行为。
- BOM 的数据库集成行为。
- Fork Drawing 的上传事务行为。
- 前端页面行为基线。

上述范围已登记到 Phase 2/9：新原子 API 或数据库集成测试建立后，必须补充真实事务、回滚和并发验证。

本阶段已新增轻量 HTTP 行为基线：

- 未认证访问图纸接口返回 401。
- 合法用户创建图纸返回 201，并传递当前用户 ID。
- 图纸修改缺少 expected revision 返回 428。
- 结构读取返回当前零件列表。
- 零件 revision 冲突返回 409。
- 仓储错误不会被静默当作成功。
- 发起审核会 trim 图纸图号并传递当前用户 ID。
- 发起审核缺少图号返回 400。
- 审核提交缺少节点名称返回 400。
- 已办审核记录可以正常读取。

测试文件：

```text
go/internal/http/handlers/drawing_handler_test.go
go/internal/http/handlers/review_case_handler_test.go
```

这些缺口记录为后续补测项，不影响先冻结领域规则，但在 Phase 0 完成前必须明确测试范围和优先级。

## Characterization Tests

至少覆盖当前用户可见行为：

- 创建 Drawing。
- 创建和更新 Part。
- 上传附件。
- 材料文件。
- BOM。
- 审核。
- 分叉 Drawing。
- 借用 Part。
- 归档与恢复。
- 版本。
- 编辑流程。

测试关注业务行为，不要求保留旧内部实现。

## 验收标准

- [x] 架构规则文档冻结。
- [x] 当前前后端构建结果已记录。
- [x] 当前可观察核心接口已有测试基线，未具备独立接口的旧事务范围已登记。
- [x] 不再讨论大的领域模型方向。

## 阶段完成记录

2026-09-04：Phase 0 完成。

完成内容：

- 领域规则和 V1 范围冻结。
- 前端类型检查、生产构建完成基线记录。
- Go 全量测试通过。
- 图纸、零件、审核 HTTP Characterization Tests 已建立并通过。
- 旧架构中无法独立注入的 Borrow、BOM、Fork Drawing 事务范围已明确登记，不阻塞进入数据库和原子 API 阶段。

## 建议提交

```text
test: establish legacy behavior baseline
docs: freeze domain architecture rules
```
