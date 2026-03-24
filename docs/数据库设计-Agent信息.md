# Agent 信息 · 数据库表设计

与 [分布式采集Agent-API协议](./分布式采集Agent-API协议.md)、`api/agent/v1/agent.proto` 及 `internal/biz/agent_hub.go` 中的 **Agent 元数据** 对齐。当前服务端 `AgentHub` 为 **内存态**；落库后可做 **运维展示、审计、按队列/标签检索、离线分析**。

---

## 1. 实体关系（概念）

```
caichip_agent (1) ──< (N) caichip_agent_tag
caichip_agent (1) ──< (N) caichip_agent_installed_script
```

- **主表**：每个 `agent_id` 一行，存队列、主机名、运行时摘要、最近心跳时间等。
- **标签**：多值，独立表便于 `WHERE tag = ?`。
- **已安装脚本**：与协议 `installed_scripts` / `ScriptRow` 对应，一行一个 `(agent_id, script_id)` 当前快照。

可选：**心跳明细表**（高频写入，仅审计或抽样时使用）。

---

## 2. 表：`caichip_agent`

| 字段 | 类型（示例） | 约束 | 说明 |
|------|----------------|------|------|
| `id` | BIGINT | PK, 自增 | 内部主键（可选；也可用 `agent_id` 单键） |
| `agent_id` | VARCHAR(64) | UNIQUE, NOT NULL | 协议中的 Agent 唯一标识 |
| `queue` | VARCHAR(128) | NOT NULL, 默认 `default` | 认领队列 |
| `hostname` | VARCHAR(256) | NULL | 任务心跳 `hostname` |
| `protocol_version` | VARCHAR(32) | NULL | 如 `1.0` |
| `python_version` | VARCHAR(64) | NULL | `runtime.python_version` |
| `agent_version` | VARCHAR(64) | NULL | `runtime.agent_version`（客户端版本） |
| `last_reported_at` | TIMESTAMPTZ(3) | NULL | 客户端上报的 `reported_at`（解析 RFC3339） |
| `last_task_heartbeat_at` | TIMESTAMPTZ(3) | NULL | 服务端收到 **任务心跳** 的时间 |
| `last_script_sync_heartbeat_at` | TIMESTAMPTZ(3) | NULL | 服务端收到 **脚本同步心跳** 的时间 |
| `created_at` | TIMESTAMPTZ(3) | NOT NULL | 首次见到该 `agent_id` |
| `updated_at` | TIMESTAMPTZ(3) | NOT NULL | 任意字段更新时刷新 |

**索引建议**：`queue`，`last_task_heartbeat_at`（查在线/超时），`updated_at`。

**在线判定**：与配置 `T_offline` 一致，例如 `now() - last_task_heartbeat_at < interval`；不必冗余存 `is_online`，避免双写。

---

## 3. 表：`caichip_agent_tag`

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| `agent_id` | VARCHAR(64) | PK 之一 | 外键 → `caichip_agent.agent_id` |
| `tag` | VARCHAR(256) | PK 之一 | 协议 `tags[]` 单项，原样存储（如 `region=cn`） |

**索引**：`(tag)` 或 `(tag, agent_id)`，便于按标签筛 Agent。

---

## 4. 表：`caichip_agent_installed_script`

与协议 `InstalledScript` / `ScriptRow` 对齐，表示 **该 Agent 当前已上报的脚本状态**（快照，非历史全量）。

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| `agent_id` | VARCHAR(64) | PK 之一 | |
| `script_id` | VARCHAR(128) | PK 之一 | |
| `version` | VARCHAR(64) | NOT NULL | 与 `version.txt` 一致（规范化前可原样存） |
| `env_status` | VARCHAR(32) | NOT NULL | `pending` / `preparing` / `ready` / `failed` 等 |
| `package_sha256` | VARCHAR(64) | NULL | 可选 |
| `message` | TEXT | NULL | 失败原因等（ScriptRow.message） |
| `updated_at` | TIMESTAMPTZ(3) | NOT NULL | 本条上报时间 |

**写入策略**：每次任务心跳或脚本同步心跳后，对该 `agent_id` **先删后插**或 **UPSERT** 全量脚本列表，与内存 `UpdateAgentMeta` 行为一致。

---

## 5. （可选）表：`caichip_agent_heartbeat_log`

仅当需要 **审计或趋势** 时使用；量大，建议 **抽样写入** 或 **按天分区**。

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | BIGSERIAL | PK |
| `agent_id` | VARCHAR(64) | |
| `kind` | VARCHAR(32) | `task` / `script_sync` |
| `occurred_at` | TIMESTAMPTZ(3) | 服务端记录时间 |
| `payload_json` | JSONB / JSON | 可选：原始请求摘要（注意脱敏） |

---

## 6. 与任务/结果的关系

任务派发、租约、结果上报可在 **任务域** 单独建表（如 `caichip_task`、`caichip_task_result`），通过 `agent_id`、`task_id` 与本文档 **Agent 表** 关联；此处不展开。

---

## 7. SQL 脚本位置

见同目录下 **`schema/agent_mysql.sql`**（DDL 以 MySQL 为准）。

---

## 8. 迁移与实现要点

1. **Upsert 主表**：`INSERT ... ON DUPLICATE KEY UPDATE`（MySQL）。
2. **标签**：每次心跳用事务 **删除该 agent 全部 tag 再插入**，保证与协议一致。
3. **脚本行**：同上，全量替换；或 MERGE 逐条。
4. **与 AgentHub**：可先 **双写**（内存 + DB），读调度仍以内存为主；稳定后 **Hub 从 DB 恢复** 或 Redis 缓存。
