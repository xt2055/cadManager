# 计划员与图纸任务：实施契约

编写日期：2026-09-17。实现状态以本文末尾的验证记录为准。

本文规定四件事：新增计划员身份、把创建图纸的权限交给计划员、新增任务管理台用于指派负责人、负责人继承原创建人的权限并在他首页显示任务面板。

## 一、已确认的产品决策

| 议题 | 结论 |
|---|---|
| 指派后创建人是否还保有控制权 | **负责人优先**：图纸有有效负责人时控制权归负责人与管理员，创建人不再拥有；无有效负责人时回落创建人（存量图纸零回归） |
| 一张图纸可以有几个负责人 | **单一负责人**；改派即替换，旧任务行保留历史 |
| 任务进度如何维护 | **由图纸生命周期自动推导**（草稿／审核中／生产中／已存档）+ 图纸文件数 + 审核节点进度；不引入人工任务状态 |
| 负责人候选人范围 | 在职且角色含 `designer` / `planner` / `admin`（与变更工单「指定修改人」的既有校验口径一致，并补上计划员） |

## 二、建档权：只有计划员与管理员可以创建图纸

限制必须在**两个后端写入点**同时生效，只藏前端按钮等于没做：

```text
POST /api/drawings                              （直接建档）
POST /api/upload-sessions kind=drawing-create    （创建页三种方式的真实写入口）
```

拒绝口径一致：`403 创建图纸需要计划员或管理员权限；设计人员请在任务管理台等待接受指派`。

前端一致地收敛入口：`/drawings/create` 路由守卫限定 `planner` / `admin`；图纸库与工作台的创建按钮按权限显示，无权限时展示**原因 + 解决入口**（说明需要计划员或管理员，并指向工作台的「我的任务」），不显示点了没反应的按钮。

`designer` 角色保留但权限表去掉 `drawing.create`；设计人员的权限来自**图纸负责人身份**，不再来自角色。已建档图纸内部的编制动作（新增零件图、预览页新建空白图纸、编辑会话）按第三节的控制权规则判定，不按建档权。

## 三、控制权：负责人 = 原创建人权限

判定顺序固定：

```text
管理员                      → 拥有
图纸存在有效负责人          → 只有该负责人拥有（创建人不再拥有）
图纸没有有效负责人          → created_by 拥有
```

规则有且仅有**两份同构实现**，其余代码必须复用：

```text
Go   internal/drawing/model.go   func (Drawing).Decides(userID string, admin bool) bool
SQL  drawing_decision_owner(drawing_uuid, user_uuid)   -- 事务内 FOR UPDATE 路径使用
```

一致性由 `go/internal/dbtest/drawing_task_test.go` 的 `TestDrawingDecisionOwnerMatchesGoDecides` 断言：同一份数据下两个实现必须给出同一答案。

前端对应实现在 `vue/cad/src/modules/drawing/drawing-authority.ts`（`isDrawingDecider` / `isDrawingAssignee` / `isMyTaskDrawing` / `matchesUser`），只用于决定按钮显隐；权威判定在后端。

覆盖的判定点（全部已改用上述规则，不得回退为比较 `created_by`）：

| 判定点 | 位置 |
|---|---|
| 编辑会话授权（草稿／生产／审核中／存档） | `go/internal/editing/service.go` `authorizeEdit` |
| 存档 | `go/internal/http/handlers/drawing_handler.go` `transitionDrawingStatus` |
| 删除图纸文件 | `go/internal/http/handlers/attachment_handler.go` `attachmentDeletionPermission` |
| 3D 模型写入与设为主模型 | `go/internal/attachment/model_management.go` `AuthorizeModelWrite` |
| 发起普通送审 | `go/internal/review/repository.go` `StartCase` |

已存档图纸对所有人只读，仍须走变更工单指定执行人，本规则不放开这一点。

## 四、任务数据模型

`drawing_tasks` 一行即一次指派；**取消与改派不删行**：

```text
status = active      当前有效负责人（部分唯一索引保证一图至多一条）
status = replaced    被改派替换，end_reason 记录原因
status = cancelled   被取消指派，end_reason 记录原因
```

```sql
-- 唯一性由部分索引保证，而不是应用层先查后写（并发下先查后写必然漏）
CREATE UNIQUE INDEX drawing_tasks_one_active ON drawing_tasks(drawing_id) WHERE status = 'active';
```

## 五、任务进度（推导，不人工填写）

| 图纸状态 | 进度 | 阶段 | 说明 |
|---|---|---|---|
| 草稿，无图纸文件 | 0% | 待上传图纸 | 档案已建立，上传文件后开始编制 |
| 草稿，有图纸文件 | 30% | 编制中 | 完成编制后发起审核 |
| 审核中 | 60–90% | 审核中 | 基准 60%，按已完成节点比例最多加到 90%；缺失节点信息时退回 60% |
| 生产中 | 100% | 生产中 | 审核通过，已投入生产 |
| 已存档 | 100% | 已存档 | 只读保护，修改须走变更工单 |
| 已停用 | 0% | 已停用 | 不再进入编制与审核流程 |

实现见 `go/internal/task/progress.go`（后端唯一来源）与 `vue/cad/src/features/tasks/task.helpers.ts`（前端只做展示）。进度条与百分比不给出人工状态字段，避免与图纸生命周期互相矛盾。

## 六、接口

```text
GET    /api/drawing-tasks?scope=mine                       我的任务（任意登录用户，按调用者本人过滤）
GET    /api/drawing-tasks?scope=board&keyword=&status=&assigned=&page=&page_size=   任务总表（计划员/管理员）
POST   /api/drawing-tasks                                   指派（body: drawingId, assigneeId, note, dueDate）
GET    /api/drawing-tasks/candidates                        可被指派人员 + 在办数量
GET    /api/drawing-tasks/{drawingId}/history               指派历史
PUT    /api/drawing-tasks/{taskId}                          改派或改说明
DELETE /api/drawing-tasks/{taskId}?reason=                  取消指派
```

错误码口径：无任务管理权限 `403`；重复指派 `409 该图纸已有负责人，请使用改派`；被指派人不合格 `400`；已存档图纸 `409`；任务不存在 `404`。

副作用：指派／改派／取消各写入 `audit_logs`（`action='assign'`，`resource_type='drawing'`），并在**同一事务**内向当事人写入 `notifications`（`kind='task'`，`event_key='task:<taskId>'`）。回滚不发送。改派会额外通知原负责人「该图纸已改派给 X，你不再负责」。

## 七、界面

- **任务管理台**（`/tasks`，计划员与管理员）：统计条（图纸总数／待指派／已指派／进行中／已完成／已逾期）+ 筛选（关键词、图纸状态、指派情况）+ 表格（图号、状态、进度、负责人、截止日期、操作）+ 分页。排序为**未指派优先**，其余按截止日期与更新时间：计划员的时间花在「还能派给谁」上。
- 指派对话框：候选人（显示身份与在办数量）、任务说明、截止日期、改派说明、指派历史。
- **首页任务面板**（`MyTaskPanel`）：任何角色只要名下有任务就显示——进行中／已完成计数、逐条进度条与阶段、下一步说明、截止日期与逾期高亮。
- **计划工作台**：工作台新增 planner 视图，指标为待指派／已指派／进行中／已完成，主列表是任务总表（未指派优先），主操作进入任务管理台。
- 四态齐备：加载中、加载失败可重试、无匹配结果、确实没有数据，四者不得合并。
- 任务通知点击进入**图纸预览页**，不跳审核或变更页签（那些通知说的是别的事）。

## 八、实施顺序（每步可运行、可验证）

| 步骤 | 内容 | 验证 |
|---|---|---|
| 1 | 迁移 000050 + 角色接入（后端角色常量、前端类型与权限表、账号管理角色选项）+ 建档权双写入口收敛 | 后端测试 + designer 建档被 403 |
| 2 | 任务域后端（模型、进度推导、服务、handler、路由、通知与审计） | `internal/task` 单测、handler 权限矩阵、dbtest 约束 |
| 3 | 控制权接入（编辑／存档／删除文件／3D 模型／送审）+ 前端 `isDrawingDecider` 统一 | 编辑权限矩阵、SQL 与 Go 判定一致性断言 |
| 4 | 任务管理台（服务、store、helpers、页面、路由、侧边栏） | 前端任务测试 + 手工指派链路 |
| 5 | 首页任务面板 + 计划工作台 + 通知跳转映射 | dashboard 测试 |
| 6 | 文档与回归（本文、PRODUCT.md、go/README.md、go/database/README.md） | 全量自动化回归 |

## 九、验证记录

**已执行并通过**

```text
go build ./...                                        通过
go vet ./...                                          通过
go test ./...                                         通过（dbtest 默认跳过）
go test ./internal/task/...                            通过（进度推导表驱动用例）
go test ./internal/http/handlers/...                   通过（含建档权与任务接口权限矩阵）
go test ./internal/editing/...                         通过（含负责人继承创建人权限矩阵）
CAD_DB_TEST=1 go test ./internal/dbtest/ -run 'TestDrawingTask|TestDrawingDecisionOwner|TestCancelledAssignment|TestPlannerRole|TestTaskNotification' -p 1 -parallel 1
                                                       通过（唯一负责人约束、改派留痕、取消后控制权回落、
                                                       SQL 与 Go 判定一致性、planner 角色约束、task 通知类型）
npx vue-tsc --build --force                            通过
node --experimental-strip-types --test scripts/dashboard.test.mjs scripts/drawing-task.test.mjs scripts/account-role-editor.test.mjs
                                                       通过
node scripts/architecture-check.mjs（npm test）         通过
```

**未执行 / 未通过，不得宣称完成**

- `CAD_DB_TEST=1 go test ./internal/dbtest/...` **全量并发运行未通过**：本机连接池被打满，出现 3 个 `connect: context deadline exceeded`（环境问题，非断言失败）。受影响用例已用 `-p 1 -parallel 1` 单独跑通，但全量绿未取得。
- 两个既有失败与本次改动无关，改动前即存在：
  - `scripts/ui-ux-workflows.test.mjs` 的「审核意见」用例缺少 harness 桩 `@/services/review-annotation.service`。
  - `scripts/attachment-create.test.mjs` 触发 Node 22.20 strip-only 模式不支持 TypeScript 参数属性，且该文件未登记在 `package.json` 的 scripts 中。
- **浏览器与 CAXA／SMB 业务 E2E 未执行**：需现场验证。待验证清单见下节。

## 十、待现场验证清单

1. 管理员创建计划员账号 → 计划员用三种方式之一创建图纸 → 任务管理台指派给设计人员（含任务说明与截止日期）。
2. 设计人员首页出现「我的任务」面板与推导进度；可编辑、新增零件图、发起审核。
3. 同一图纸计划员**不能**再编辑或存档（负责人优先），管理员仍可；改派后控制权随之转移，历史任务行与改派原因保留，原负责人收到改派通知。
4. 取消指派后控制权回落创建人。
5. 已存档图纸：负责人不再生效，仅管理员或变更工单指定执行人可改。
6. 设计人员直接访问 `/drawings/create` 被守卫送回工作台，直接 `POST /api/drawings` 返回 403。
7. 任务通知点击进入图纸预览页。

## 十一、给后续实施者的约束

1. 阅读仓库根目录 `AGENTS.md`，**禁止启动子代理**。
2. 先执行 `git status --short`，保留已有未提交修改。
3. 涉及创建人级权限的新代码，**必须**复用 `Drawing.Decides` 或 `drawing_decision_owner`，不得自行比较 `created_by`。
4. 任务记录不得用 `DELETE` 清理，改派与取消一律通过 `status` + `end_reason` 表达。
5. 新增角色必须同时改 `user_roles` 的 CHECK 约束与 `internal/auth/roles.go`，并补前端权限表。
6. 测试未执行时必须写「未执行」，不得写「全部通过」。
