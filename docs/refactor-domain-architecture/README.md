# CAD 系统架构重构任务

本目录将架构重构拆分为可独立验收的阶段。每个阶段完成后单独验证、提交，并更新 [当前任务.md](./当前任务.md)。

阶段顺序：

```text
Phase 0 设计冻结与基线
Phase 1 最终数据库模型
Phase 2 后端 Domain 与原子 API
Phase 3 上传与附件基础设施
Phase 4 前端业务 Modules
Phase 5 Pinia 与 Read Model
Phase 6 Vue 页面迁移
Phase 7 淘汰旧前端架构
Phase 8 删除旧 Go API
Phase 9 最终回归与清理
```

原则：

```text
新增新架构
  ↓
验证
  ↓
迁移调用方
  ↓
验证
  ↓
旧代码失去使用者
  ↓
删除旧代码
```

不要先删除旧代码再让项目整体进入不可运行状态。每个阶段都应尽量保持可运行、可验证、可回滚。

