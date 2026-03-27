# Kratos 项目结构约定（caichip 及后续服务）

本文整理自官方说明：[项目结构 | Kratos](https://go-kratos.dev/zh-cn/docs/intro/layout/)，作为本仓库及后续基于 Kratos 的微服务 **目录与分层约定**。新建服务或拆模块时按此对齐；与官方 `kratos new` 生成的 [kratos-layout](https://github.com/go-kratos/kratos-layout) 一致。

---

## 1. 顶层目录

| 路径 | 职责 |
|------|------|
| `api/` | **对外协议**：`.proto` 及生成的 `.pb.go`、`_grpc.pb.go`、`_http.pb.go`、swagger 等；按服务/版本分子目录（如 `api/<svc>/v1/`）。 |
| `cmd/<service>/` | **进程入口**：`main.go`、`wire.go`、`wire_gen.go`（依赖注入）。 |
| `configs/` | 本地/样例配置（如 `config.yaml`）。 |
| `internal/` | **不对外 import** 的实现代码（Go `internal` 语义），业务主体在此。 |
| `third_party/` | API 依赖的第三方 proto（`google/api`、`validate` 等）。 |
| `Makefile` / `Dockerfile` / `go.mod` | 构建、镜像、依赖管理。 |

---

## 2. `internal/` 分层（DDD 风格）

| 包 | 职责 | 约定 |
|----|------|------|
| **`internal/biz`** | 业务规则与用例组装，接近 **领域层**；**Repository 接口在此定义**（依赖倒置），不直接依赖具体存储。调度工厂、Agent stdout 报价解析等领域逻辑也在此（见 `repo.go`、`scheduler_factory.go`、`task_stdout_quotes.go`）。 |
| **`internal/data`** | **数据访问**：DB、缓存、RPC 客户端等；实现 `biz` 中定义的 repo 接口；命名偏业务含义，不等同于纯 DAO。 |
| **`internal/service`** | **应用层**：实现 `api` 定义的服务；**DTO ↔ 领域对象** 转换、编排多个 `biz`；**不写复杂业务规则**（下沉到 `biz`）。 |
| **`internal/server`** | **传输层**：HTTP/gRPC Server 实例、路由注册、中间件挂载。 |
| **`internal/conf`** | 服务内部配置结构（常用 proto 定义并生成 `conf.pb.go`）。 |

依赖方向（自上而下调用）：**`server` → `service` → `biz` ← `data`（实现接口）**。

---

## 3. 持久化（MySQL）与 `data` 层边界

### 3.1 MySQL 访问方式

- 本工程约定：对 **MySQL** 的增删改查 **统一使用 [GORM](https://gorm.io/)**（`gorm.io/gorm`）。
- **新增** 数据访问代码时优先 GORM；若存在历史 `database/sql` 片段，**逐步迁移**或至少在**新模块**中不再扩大裸 SQL 使用面（除非 GORM 无法表达的极少数场景，需在 PR 中说明）。

### 3.2 `data` 层禁止承载「业务更新逻辑」

- **`internal/data`**：**只做持久化与资源访问**——按 `biz` 定义的 repo 接口执行保存、查询、删除、事务提交等；可做 **与存储直接相关的** 条件过滤、分页参数、乐观锁字段更新。
- **不得**在 `data` 层编写：**状态机推导**（如 BOM 搜索任务何时 `succeeded`）、**就绪判定**、**调度合并**、**跨聚合根的业务规则**、**需读多表再「决策写回」的用例**。这些属于 **`internal/biz`**（由用例函数编排，可多次调用 repo）。
- **`internal/service`**：编排 API 与 `biz`，**不**把上述规则下沉到 `data`；也避免在 `service` 里堆领域规则（应进 `biz`）。

---

## 4. 代码放置规则（简表）

| 内容 | 放在 |
|------|------|
| `.proto`、生成代码 | `api/` |
| `main`、Wire 装配 | `cmd/<name>/` |
| 领域实体、业务规则、repo **接口** | `internal/biz/` |
| GORM/MySQL、缓存、外部 API 封装 | `internal/data/`（仅持久化，见 §3） |
| 实现 proto Service | `internal/service/` |
| `NewHTTPServer` / `NewGRPCServer`、注册 handler | `internal/server/` |

---

## 5. 工具链约定

- **依赖注入**：使用 [Wire](https://github.com/google/wire)，在 `cmd/.../wire.go` 维护 provider，`wire_gen.go` 为生成物。  
- **新建项目**：`kratos new <project-name>` 基于官方 layout 脚手架。  
- **与本仓库**：**新增模块优先落在对应包**，避免在 `service` 堆业务；**业务规则在 `biz`**，`data` 仅持久化（见 §3）。

---

## 6. 延伸阅读（官方推荐）

- [Go 工程化 - Project Layout 最佳实践](https://go-kratos.dev/zh-cn/docs/intro/layout/) 同页「推荐阅读」中的外链  
- Kratos 文档：[Wire 依赖注入](https://go-kratos.dev/zh-cn/docs/guide/wire/)、[Protobuf 规范](https://go-kratos.dev/zh-cn/docs/guide/api-protobuf/)

---

## 7. 修订记录

| 日期 | 说明 |
|------|------|
| 2026-03-27 | 据 Kratos 官方「项目结构」页整理为仓库约定 |
| 2026-03-27 | 纳入项目：根目录 `AGENTS.md`、`.cursor/rules/kratos-project-layout.mdc`（`alwaysApply: true`） |
| 2026-03-27 | §3：MySQL 统一 GORM；`data` 层不做业务更新逻辑（规则在 `biz`） |
