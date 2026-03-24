# Agent 任务调度 — MySQL 表结构与 `AgentHub` 改造草图

目标：**多台 server 进程共用同一待派发队列**，调度事实以 **MySQL** 为准，替代当前纯内存的 `pending` / `assign`。

**实现状态（2026-03）**：配置 `agent.dispatch_store: mysql` 且已建表后，`internal/service` 使用 `dbTaskScheduler` + `DispatchTaskRepo` / `AgentRegistryRepo`；`memory` 仍为纯 `AgentHub`。BOM `EnsureTasksForSession`（MySQL）在 `dispatch_store=mysql` 时会往 `caichip_dispatch_task` 幂等入队（`bom_platform_script.script_id`，任务版本暂固定 `1.0.0`）。详见 `docs/superpowers/plans/2026-03-24-agent-dispatch-mysql-implementation.md`。

---

## 1. 表结构（已实现 DDL）

见 **`docs/schema/agent_dispatch_task_mysql.sql`**，主表 **`caichip_dispatch_task`**。

### 1.1 状态机（建议）

| `state`     | 含义 |
|------------|------|
| `pending`  | 可被选入认领；`lease_id` 为空 |
| `leased`   | 已发给某 Agent；`lease_id`、`leased_to_agent_id`、`leased_at` 必填 |
| `finished` | 已收到有效结果或业务判终态 |
| `cancelled`| 管理/业务取消，不再派发 |

迁移：`pending` → `leased`（认领）→ `finished`（结果 OK）；重派：`leased` / 超时回收 → `pending` 且 `attempt+1`，**新 `lease_id`**。

### 1.2 多节点原子认领（MySQL 8）

在事务中：

1. `SELECT id, ... FROM caichip_dispatch_task WHERE queue = ? AND state = 'pending' ORDER BY id ASC LIMIT :k FOR UPDATE SKIP LOCKED;`
2. 对每一行（或少于 `max` 条）在 **应用层** 用现有 **`matchLocked` 等价逻辑**（队列 + tags + `script_id`/`version` + `env_status=ready`）过滤。
3. 命中的行执行：
   `UPDATE caichip_dispatch_task SET state='leased', lease_id=?, leased_to_agent_id=?, leased_at=NOW(3), lease_deadline_at=..., attempt=..., updated_at=NOW(3) WHERE id=? AND state='pending';`
   检查 `(RowsAffected==1)` 防止并发双认领。
4. 未命中则 **不更新**，锁在事务结束释放；可继续取下一条 SKIP LOCKED 候选，直到凑满 `max` 或没有候选。

**版本比对**：与现 Hub 一致，建议仍调用 `versionutil.Equal`（应用层），表中存 **任务要求版本**；Agent 侧版本来自 `caichip_agent_installed_script` 或心跳快照表。若调度进程 **只信 DB 中的 Agent 安装表**，可将 `match` 部分写成 `JOIN caichip_agent_installed_script`，JSON `required_tags` 与 tag 表联动，复杂度更高，可二期再做。

### 1.3 租约回收（重派触发）

两类条件（与现语义对齐，可合并进一次扫描）：

1. **Agent 离线**：`leased_to_agent_id` 在 `caichip_agent.last_task_heartbeat_at`（或内存心跳表若尚未落库）已超过 `T_offline`。
2. **Lease 超时**：`lease_deadline_at < NOW(3)` 或 `leased_at` + 配置的 `lease_ttl_sec` / `timeout_sec` 已满。

处理：

```sql
UPDATE caichip_dispatch_task
SET state = 'pending',
    lease_id = NULL,
    leased_to_agent_id = NULL,
    leased_at = NULL,
    lease_deadline_at = NULL,
    attempt = attempt + 1,
    updated_at = NOW(3)
WHERE state = 'leased' AND (... 离线或 lease 过期 ...);
```

之后在下一轮 `Pull` 中可被 **任意节点** 再次认领。

### 1.4 结果上报

`SubmitTaskResult` 对应事务：

```sql
UPDATE caichip_dispatch_task
SET state = 'finished',
    finished_at = NOW(3),
    result_status = ?,
    updated_at = NOW(3)
WHERE task_id = ? AND state = 'leased' AND lease_id = ?;
```

- `RowsAffected == 0`：若行已是 `finished` → 幂等成功；若 `lease_id` 不匹配 → **409 / ErrLeaseReassigned**。

可选：校验 `leased_to_agent_id = ?` 与请求 `agent_id` 一致（加强审计）。

---

## 2. `AgentHub` 改造草图（包与职责）

### 2.1 分层

```
internal/data/
  dispatch_task_repo.go   // SQL：enqueue、claimBatch、submitResult、reclaimStaleLeases、（可选）agent 心跳写库

internal/biz/
  agent_hub.go            // 保留类型 QueuedTask / TaskMessage / match 逻辑；或抽 match 为独立函数
  agent_hub_db.go         // 新：AgentHubDB 组合 Repo + 轻量进程内缓存（可选）
```

- **`AgentHub`（内存）**：可保留为 **测试桩** 或 **无 DB 模式**；生产 Wire 注入 **`AgentHubDB`** 实现相同接口（见下）。
- **接口**：建议抽象 `TaskScheduler`：`Enqueue`、`PullForAgent`、`SubmitResult`、`TouchHeartbeat`、`UpdateAgentMeta`、`WaitForLongPoll`，由两种实现（memory / db）挂接 `AgentService`。

### 2.2 与原方法映射

| 现 `AgentHub` | DB 化后 |
|---------------|---------|
| `EnqueueTask` | `INSERT caichip_dispatch_task`（`state=pending`，填 `task_id/script_id/version/queue/...`） |
| `PullTasksForAgent` | 短事务：`reclaimStale...` + SKIP LOCKED 循环 + `match` + `UPDATE` 批量 leased |
| `SubmitTaskResult` | 上节 SQL；删除内存 `assign` |
| `TouchTaskHeartbeat` | 更新 **`caichip_agent.last_task_heartbeat_at`**（若已接 DB）；再触发/并入 **lease 回收** |
| `UpdateAgentMeta` | **`UPSERT caichip_agent` + `caichip_agent_tag` + `caichip_agent_installed_script`**，与现 schema 对齐，供 `match` 或运维查询 |
| `reassignStaleLocked` | 改为 **`reclaimStaleLeases` SQL**（按全局 lease 时限 + agent 离线） |
| `lastTaskHB` / `meta` map | **以 DB 为准**；或 **进程内 LRU 只读缓存**（如 1–3s TTL）降低 Pull 压力 |

### 2.3 `running_tasks`（方案 A）

行为保持：**在应用层过滤** — 从 DB 认领候选在 `match` 之后，再剔除 `running_tasks` 中已占用 `task_id` / `script_id` 的派发（与现逻辑一致）。**库仍以 `leased` 行为准**；若 Agent 漏报 `running_tasks` 但库中已是 `leased`，以库为准避免重复派发。

### 2.4 长轮询 `WaitForLongPoll`

不变：定时调用 `PullTasksForAgent`；仅 **Pull 内部** 从内存遍历改为 **DB 认领**。注意 **连接/事务** 要短，避免长事务占锁。

### 2.5 Wire / 配置

- `bootstrap` 增加例如 `agent.dispatch_store: memory|mysql`（或检测 `data.database` 非空则启用 DB）。
- `wire.go`：`NewAgentHub` → 条件注入 `NewDispatchTaskRepo` + `NewAgentSchedulerDB`。

### 2.6 测试

- **集成测试**：Docker MySQL 或 `sqlmock` 模拟 SKIP LOCKED + 双协程抢同一行，断言仅一个 `leased`。
- **兼容**：原 `agent_hub_test.go` 保留内存路径；新增 `agent_hub_db_test.go`。

---

## 3. 与 BOM 的衔接

- `bom_search_task.caichip_task_id` **等于** `caichip_dispatch_task.task_id` 时，可在管理端 **JOIN** 查看排队/执行状态。
- BOM 创建搜索任务时：生成 `task_id` → **插入 `caichip_dispatch_task`（pending）** → 更新 BOM 行上的 `caichip_task_id`（顺序可在一个业务事务内完成）。

---

## 4. 迁移风险提示

1. **首次上线**：需把「仅在内存中的队列」排空后再切，或接受 **短暂双写**（入队写 DB + 内存，读逐步切 DB）。
2. **`FOR UPDATE SKIP LOCKED`** 要求 **MySQL 8+**；与 `docs/schema/agent_mysql.sql` 前提一致。
3. **索引**：认领路径依赖 `idx_dispatch_claim (queue, state, id)`；监控慢查询与锁等待。

---

## 5. 文档与脚本索引

| 资源 | 路径 |
|------|------|
| DDL | `docs/schema/agent_dispatch_task_mysql.sql` |
| API 语义 | `docs/分布式采集Agent-API协议.md` |
| Agent 元数据表 | `docs/schema/agent_mysql.sql` |

本文档为 **实现前草图**；落地时可根据是否启用 `caichip_dispatch_task_lease_log`、是否加外键等再收敛一版迁移清单。
