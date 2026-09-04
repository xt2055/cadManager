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

## 阶段完成标准

- [ ] 前端只能使用 Atomic API。
- [ ] 后端不存在 legacy bulk write handler。
- [ ] 不存在全库数组替换接口。
- [ ] 旧 Tauri 客户端明确不在兼容范围内。
- [ ] API 路由测试确认旧接口返回不存在或不支持。

## 建议提交

```text
refactor(api): remove legacy bulk data endpoints
```

