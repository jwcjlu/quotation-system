# 仓库协作说明（Agent / 开发者）

## Kratos 工程结构约定

本服务遵循 Kratos 官方推荐的目录与分层，**新增或修改 Go 代码时请按此放置**：

| 区域 | 用途 |
|------|------|
| `api/` | Proto 与生成代码 |
| `cmd/server/` | 入口与 Wire 依赖注入 |
| `internal/biz/` | 领域逻辑、Repository **接口**（`biz/repo.go` 等）、调度与 BOM stdout 等领域代码 |
| `internal/data/` | Repo 实现、数据库（**MySQL 用 GORM**）与外部资源；**仅持久化，不写业务更新逻辑** |
| `internal/service/` | 应用层：实现 API、编排 biz |
| `internal/server/` | HTTP/gRPC 与中间件 |
| `internal/conf/` | 配置结构 |

依赖关系：`server` → `service` → `biz` ← `data`。

**持久化约定：** 操作 MySQL 请使用 **GORM**。状态机、就绪判定、合并调度等 **业务规则放在 `internal/biz`**，不要在 `data` 层实现「带业务决策的更新」。

**详细规则与延伸阅读：** [docs/kratos-project-layout.md](docs/kratos-project-layout.md)（含官方文档链接与 §3 边界说明）。

Cursor 中已通过 `.cursor/rules/kratos-project-layout.mdc` 启用同一条约定。
