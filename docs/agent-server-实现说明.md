# Agent 服务端实现说明（caichip）

与 [分布式采集Agent需求与架构](./分布式采集Agent需求与架构.md)、[分布式采集Agent-API协议](./分布式采集Agent-API协议.md) 对齐：默认 **内存调度**（`agent.dispatch_store: memory`），可选 **MySQL 队列表**（`dispatch_store: mysql`，见 [Agent任务调度-MySQL与Hub改造草图](./Agent任务调度-MySQL与Hub改造草图.md)）。

## 已实现

| 路径 | 说明 |
|------|------|
| `POST /api/v1/agent/task/heartbeat` | 任务心跳 + 长轮询拉取任务；需 `Authorization: Bearer <api_key>` 或 `X-API-Key` |
| `POST /api/v1/agent/script-sync/heartbeat` | 脚本安装心跳；`script_store.enabled` 时按 **全部已发布 `script_id`** 与 `scripts[]` 比对，返回 `sync_actions`（`download`） |
| `POST /api/v1/admin/agent-scripts/packages` | 管理端 multipart 上传（密钥见 `script_admin.api_keys`） |
| `POST /api/v1/admin/agent-scripts/packages/{id}/publish` | 发布为当前版本 |
| `GET /api/v1/admin/agent-scripts/current` | Query: `script_id` |
| `GET /api/v1/admin/agent-scripts/packages` | 分页列表 |
| `GET {script_store.url_prefix}/...` | 同源静态下载（配置 `script_store.root` + `enabled`） |
| `POST /api/v1/agent/task/result` | 任务结果；租约不匹配返回 **409**（`LEASE_EXPIRED`） |
| `internal/biz/agent_hub.go` | 调度、离线重派、`T_offline=max(120,6×interval)`、租约 |
| `internal/pkg/versionutil` | `version.txt` 与任务 `version` 规范化 |

## 配置

`configs/config.yaml` 中 `agent` 段：

- `enabled: true` 才注册路由  
- `api_keys`：至少一个密钥，用于鉴权  
- `long_poll_max_sec`：默认 55（与 API 协议一致）
- `dispatch_store`：`memory` \| `mysql` — `mysql` 时需 MySQL 8+ 并已执行 `docs/schema/agent_dispatch_task_mysql.sql`，且建议已有 `caichip_agent` 表；任务心跳会将 Agent 快照写入库供 match
- `dispatch_lease_extra_sec`：租约 DDL 在任务 `timeout_sec` 基础上附加的秒数

## 验证

```bash
# 单元测试
go test ./internal/biz/... ./internal/service/... ./internal/pkg/versionutil/... -count=1

# 启动服务（需正确 -conf）
go run ./cmd/server -conf configs/config.yaml

# 开发入队（需 agent.dev_enqueue_enabled: true，且与 api_keys 一致）
curl -s -X POST http://127.0.0.1:18080/api/v1/agent/dev/enqueue ^
  -H "Authorization: Bearer change-me-in-production" -H "Content-Type: application/json" ^
  -d "{\"script_id\":\"demo\",\"version\":\"1.0.0\",\"queue\":\"default\"}"

# 最小 Python 心跳 + 若有任务则上报结果
set CAICHIP_API_KEY=change-me-in-production
python scripts/agent_heartbeat_smoke.py http://127.0.0.1:18080
```

**注意**：`dev/enqueue` 仅用于开发验证；**生产** 请设 `dev_enqueue_enabled: false`，任务入队由业务系统写入队列。

## Python Agent 进程

仓库目录 **`agent/`** 为 **独立可运行** 的 Agent（`python -m agent`），见 **`agent/README.md`**（双心跳、长轮询、本地脚本执行、超时杀进程树、同 `script_id` 串行）。

## 未实现（后续）

- 下载鉴权 token、限流、断点续传  
- 数据库持久化任务与 Agent 状态（Hub 仍为内存）  
