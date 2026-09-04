# Phase 8：删除旧 Go API

## 目标

在前端已经完全使用原子 API 后，删除旧数据模型和全量写入接口。

本次明确旧 Tauri 客户端不兼容，前后端同步升级，因此不保留旧 API 兼容窗口。

## 删除接口

```text
PUT /api/data/structure
PUT /api/data/bom
PUT /api/data/borrows
PUT /api/data/branches
PUT /api/data/attributes
```

## 删除相关实现

```text
domain_module_handler
legacy repository
legacy DTO
```

以及只为旧接口存在的全量数组写入逻辑。

## 搜索检查

```text
rg "/api/data/" .
rg "/data/" src
```

确认不存在业务调用、旧接口注册和旧 Repository 依赖。

## 当前实施记录

执行日期：2026-09-04

- 前端服务端模式的属性、结构创建、BOM、借用和附件查询已切换到最终资源接口；JSON Debug 仍由独立本地仓储承载。
- `/api/data/` 路由和 `domain_module_handler` 已删除，分支、借用和附件查询分别迁移到只读投影 Handler。
- 旧整表 PUT 写入实现、DTO 和辅助事务代码已删除；旧路由 404 回归测试已加入。
- `go test ./...`、前端 `npm run type-check`、`npm run build` 和 `git diff --check` 均通过。

## 阶段完成标准

- [x] 前端服务端模式只能使用 Atomic API/最终资源查询；JSON Debug 使用独立本地仓储。
- [x] 后端不存在 legacy bulk write handler。
- [x] 不存在全库数组替换接口。
- [x] 旧 Tauri 客户端明确不在兼容范围内。
- [x] API 路由测试确认旧接口返回不存在。

## 建议提交

```text
refactor(api): remove legacy bulk data endpoints
```
