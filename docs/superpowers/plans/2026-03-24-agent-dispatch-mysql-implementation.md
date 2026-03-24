# Agent 任务调度 MySQL 持久化与 AgentHub 改造 Implementation Plan

> **For agentic workers:** 实现本计划时优先使用 [@c:\Users\Admin\.cursor\skills\superpowers\skills\subagent-driven-development\SKILL.md](../../superpowers/subagent-driven-development.md)（推荐）或 executing-plans，**按 Task 顺序**完成；步骤使用 `- [ ]` 勾选跟踪。
>
> **建议：** 在独立 git worktree 中实施（见 [@c:\Users\Admin\.cursor\skills\superpowers\skills\using-git-worktrees\SKILL.md](../../superpowers/using-git-worktrees.md)）。

**Goal:** 将 Agent 任务队列与 **`task_id`↔`agent_id` 租约绑定** 从纯内存 `AgentHub` 迁到 **MySQL `caichip_dispatch_task`**，使 **多实例 server 共用同一待派发队列**；协议行为（长轮询、`running_tasks`、`lease_id`、409）与 [分布式采集Agent-API协议.md](../../分布式采集Agent-API协议.md) 及 [Agent任务调度-MySQL与Hub改造草图.md](../../Agent任务调度-MySQL与Hub改造草图.md) 一致。

**Architecture:** **调度真相源在 DB**：`pending` 行经 `FOR UPDATE SKIP LOCKED` 认领转 `leased`；`SubmitTaskResult` 通过 `task_id+lease_id` 乐观更新写 `finished`；**离线/租约过期** 由 `ReclaimStaleLeases` 将行打回 `pending` 并 `attempt++`。**应用层**保留现 `matchLocked` 语义（队列 / tags / script 就绪与版本）；**match 输入**逐步从「内存 meta」改为「DB：`caichip_agent` + `caichip_agent_tag` + `caichip_agent_installed_script`」或短 TTL 缓存。**兼容路径**：配置 `memory` 时仍用当前 `AgentHub`（测试与无 DB 环境）。

**Tech Stack:** Go、`database/sql`、MySQL 8+（`SKIP LOCKED`）、Wire、Kratos、现有 `internal/biz/agent_hub.go` 类型与 `versionutil`。

**Spec / 设计输入:** [docs/Agent任务调度-MySQL与Hub改造草图.md](../../Agent任务调度-MySQL与Hub改造草图.md) · DDL：[docs/schema/agent_dispatch_task_mysql.sql](../../schema/agent_dispatch_task_mysql.sql) · API：[docs/分布式采集Agent-API协议.md](../../分布式采集Agent-API协议.md)

---

## 文件结构（创建 / 修改一览）

| 路径 | 职责 |
|------|------|
| **已有** `docs/schema/agent_dispatch_task_mysql.sql` | 确认与实现一致；按需补迁移备注 |
| **Modify** `internal/data/dispatch_task_repo.go` | `EnqueuePending`、`ClaimCandidates`、`LeaseRow`、`FinishLeased`、`ReclaimStaleLeases`；MySQL 专用 SQL（与 `data.dbDriver` 分支时再抽 PG） |
| **Create** `internal/data/dispatch_task_repo_test.go`（或 `_integration_test.go`） | `TEST_MYSQL_DSN` 门禁；SKIP LOCKED 双 goroutine 抢同一 `pending` 仅一条变 `leased` |
| **Create** `internal/data/agent_registry_repo.go`（可选拆分） | 任务心跳时 `UPSERT caichip_agent`、`REPLACE` tags、`UPSERT` `installed_script`；供 match 读或仅写 |
| **Modify** `internal/conf/conf.proto` / `conf.pb.go` | `agent.dispatch_store`: `memory` \| `mysql`（或布尔 `use_dispatch_db`）；可选 `lease_ttl_sec` |
| **Modify** `internal/biz/` | 抽离 `MatchQueuedTask(agentMeta, *QueuedTask) bool` 供内存与 DB 路径共用；**Create** `task_scheduler.go` 定义 `TaskScheduler` 接口 |
| **Create** `internal/biz/agent_scheduler_db.go` | 组合 `DispatchTaskRepo` + agent 快照加载；实现 `PullTasksForAgent` / `SubmitTaskResult` / `Enqueue` / `WaitForLongPoll` 委托 |
| **Modify** `internal/biz/agent_hub.go` | 让内存 Hub 实现同一 `TaskScheduler` 接口（adapter 方法） |
| **Modify** `internal/service/agent.go` | 按配置选择 `TaskScheduler`；`TaskHeartbeat`/`TaskResult`/`DevEnqueue` 走接口；`dispatchRepo` 已注入，改为与 scheduler 一致 |
| **Modify** `cmd/server/wire.go` | 构造 `biz.TaskScheduler`：若仅运行时分支，可 `NewAgentService(hub, dispatchRepo, ..., scheduler)` 由 **单一工厂** 在 `service` 内根据 `bc` 选实现，避免 Wire 条件编译 |
| **Modify** `internal/service/bom_service.go`（或 BOM enqueue 调用点） | 创建 BOM 搜索任务时 **INSERT `caichip_dispatch_task`** + 写 `caichip_task_id`（与草图 §3 一致）— 可列为 **Phase 2** |
| **Modify** `docs/Agent任务调度-MySQL与Hub改造草图.md` | 标注「已实现」与配置开关 |

---

### Task 1: 迁移与运维准备

**Files:**
- `docs/schema/agent_dispatch_task_mysql.sql`
- 运维 runbook（可本文件 §「上线检查」或 `docs/agent-dispatch-migration.md` 短页）

- [ ] **Step 1:** 在 **目标 MySQL 8+** 执行 DDL，确认 `SHOW CREATE TABLE caichip_dispatch_task` 与索引 `idx_dispatch_claim` 存在。
- [ ] **Step 2:** 记录 **回滚**（`DROP TABLE` 仅限未接生产流量环境）；生产用 feature flag 先关 `dispatch_store=mysql`。
- [ ] **Step 3:** Commit（若仅有运维说明）

```bash
git add docs/schema/agent_dispatch_task_mysql.sql docs/agent-dispatch-migration.md  # 若新建
git commit -m "docs(agent): dispatch task table migration notes"
```

---

### Task 2: 配置开关

**Files:**
- `api/conf/v1/conf.proto`（若项目以 proto 为源）
- `internal/conf/`
- `configs/config.yaml`

- [ ] **Step 1:** 增加 `Agent.DispatchStore`（string，默认 `memory`）或 `UseDispatchMysql bool`；可选 `DispatchLeaseTTLSeconds`。
- [ ] **Step 2:** 运行 `make api` 或等价 `protoc` 重生 `conf.pb.go`。
- [ ] **Step 3:** `go build ./...`。
- [ ] **Step 4:** Commit  

```bash
git commit -m "feat(conf): agent dispatch_store for mysql scheduler"
```

---

### Task 3: `DispatchTaskRepo` 数据访问（TDD）

**Files:**
- `internal/data/dispatch_task_repo.go`
- `internal/data/dispatch_task_repo_test.go`

- [ ] **Step 1:** 定义与表对齐的 `DispatchTaskRow` / 入参结构；**ErrLeaseMismatch** 等哨兵错误与现有 `biz.ErrLeaseReassigned` 映射在 service 层。
- [ ] **Step 2:** **失败测试**：`EnqueuePending` 插入后 `state=pending`；`FinishLeased` 在 `lease` 正确时变 `finished`，错误 `lease` 时 `RowsAffected==0`。
- [ ] **Step 3:** 实现 `EnqueuePending(ctx, *biz.QueuedTask)`（`required_tags` JSON、`params_json` 可选）。
- [ ] **Step 4:** 实现事务内 **`ClaimPendingIDs(queue, limit)`** → `FOR UPDATE SKIP LOCKED`；再 **`TryLease(id, agentID, leaseID, deadline)`** `WHERE id=? AND state='pending'`。
- [ ] **Step 5:** 实现 `ReclaimStaleLeases(ctx, offlineBefore time.Time, now time.Time)`：子查询 `leased` 且（`lease_deadline_at < now` **或** `leased_to_agent_id` 关联 `caichip_agent.last_task_heartbeat_at < offlineBefore`）；首期若无 agent 表数据，可先只做 `lease_deadline_at`。
- [ ] **Step 6:** 并发测试：两 goroutine 同时 claim 同一队列，断言全局仅一行转入 `leased`。
- [ ] **Step 7:** Commit  

```bash
git commit -m "feat(data): DispatchTaskRepo enqueue claim finish reclaim"
```

---

### Task 4: 抽取 `match` 与 DB 调度候选过滤

**Files:**
- `internal/biz/agent_hub.go`
- `internal/biz/match_task.go`（新建，若需减耦合）

- [ ] **Step 1:** 将 `matchLocked` 提炼为 **`MatchTaskForAgent(meta agentMeta, t *QueuedTask) bool`**（或接受接口），内存 Hub 改为调用它。
- [ ] **Step 2:** 定义从 DB 加载 `agentMeta` 的函数：`LoadAgentMeta(ctx, db, agentID)` 读 `caichip_agent` + tag + installed_script（首期可只在 `data` 包实现，`biz` 调 `repo`）。
- [ ] **Step 3:** 将 `DispatchTaskRow` 转为「可 match 的 `QueuedTask`」或直接把 row 字段传入 `Match...`。
- [ ] **Step 4:** `go test ./internal/biz/...`。
- [ ] **Step 5:** Commit  

```bash
git commit -m "refactor(biz): extract MatchTaskForAgent for db scheduler"
```

---

### Task 5: Agent 心跳元数据落库（match 数据源）

**Files:**
- `internal/data/agent_registry_repo.go`（或 `agent_heartbeat_repo.go`）
- `internal/service/agent.go`

- [ ] **Step 1:** 实现 `UpsertAgentHeartbeat(ctx, agentID, queue, hostname, reportedAt, scripts, tags)`：与 [docs/schema/agent_mysql.sql](../../schema/agent_mysql.sql) 一致；注意 **DELETE+INSERT tags** 或等价同步。
- [ ] **Step 2:** 在 `TaskHeartbeat` 中，当 `dispatch_store=mysql` 时 **先/后** 写库（与 `TouchTaskHeartbeat` 顺序在计划中固定，避免与 reclaim 死锁）；内存模式不写。
- [ ] **Step 3:** Commit  

```bash
git commit -m "feat(data): persist agent task heartbeat meta for mysql dispatch"
```

---

### Task 6: `TaskScheduler` 接口与 `AgentSchedulerDB`

**Files:**
- `internal/biz/task_scheduler.go`
- `internal/biz/agent_scheduler_db.go`
- `internal/biz/agent_hub.go`（adapter）

- [ ] **Step 1:** 定义接口方法：`EnqueueTask`、`PullTasksForAgent`、`SubmitTaskResult`、`TouchTaskHeartbeat`、`UpdateAgentMeta`、`WaitForLongPoll`（签名与现 Hub 对齐）。
- [ ] **Step 2:** `AgentHub` 实现接口（薄包装现有方法）。
- [ ] **Step 3:** `AgentSchedulerDB`：内嵌 `*DispatchTaskRepo`、config、`LoadAgentMeta`；`Pull` = `ReclaimStale` + SKIP LOCKED 循环 + `Match` + `TryLease` + 拼 `TaskMessage`；**应用层**最后按 `running_tasks` 过滤（与现逻辑一致）。
- [ ] **Step 4:** `SubmitTaskResult` 调 repo `FinishLeased`；映射 409。
- [ ] **Step 5:** 单元测试：`AgentSchedulerDB` 可用 fake repo 或少量集成测试。
- [ ] **Step 6:** Commit  

```bash
git commit -m "feat(biz): TaskScheduler and AgentSchedulerDB"
```

---

### Task 7: `AgentService` 接入调度实现

**Files:**
- `internal/service/agent.go`
- `cmd/server/wire.go`（若构造函数签名变）
- `internal/service/agent_test.go`

- [ ] **Step 1:** `NewAgentService` 注入 `scheduler biz.TaskScheduler`（或由 `hub + dispatchRepo + bc` **在 `NewAgentService` 内** `if bc.Agent.DispatchStore=="mysql" && dispatchRepo.DBOk()` 构造 `AgentSchedulerDB`，否则用 `hub` adapter）；**Wire 仍只注具体依赖**，工厂放在 service。
- [ ] **Step 2:** `TaskHeartbeat`、`TaskResult`、`DevEnqueue` 全部走 `scheduler`。
- [ ] **Step 3:** 跑 `cd cmd/server && wire && go build ./...`。
- [ ] **Step 4:** 更新 `agent_test`：内存模式与（可选）mysql 集成门禁。
- [ ] **Step 5:** Commit  

```bash
git commit -m "feat(agent): route TaskHeartbeat through TaskScheduler"
```

---

### Task 8: BOM 入队双写（可选 Phase 2）

**Files:**
- `internal/service/bom_service.go` / `internal/biz` BOM 用例

- [ ] **Step 1:** 生成 `caichip_task_id`（UUID）后 **INSERT `caichip_dispatch_task`**（`pending`），再更新 `bom_search_task.caichip_task_id`；与业务同事务。
- [ ] **Step 2:** 仅当 `dispatch_store=mysql` 时插入；否则保持现逻辑（或仍仅内存 enqueue —— 与产品确认）。
- [ ] **Step 3:** Commit  

```bash
git commit -m "feat(bom): enqueue caichip_dispatch_task with search task"
```

---

### Task 9: 文档与发布检查

**Files:**
- `docs/Agent任务调度-MySQL与Hub改造草图.md`
- `docs/agent-server-实现说明.md`（若存在）

- [ ] **Step 1:** 记录配置样例、`dispatch_store`、依赖 MySQL 8+、监控索引与锁等待。
- [ ] **Step 2:** Commit  

```bash
git commit -m "docs(agent): mysql dispatch runbook and config"
```

---

## 上线与验证清单（非 Task）

- [ ] 预发：`dispatch_store=mysql`，单实例 → 双实例压测 **同队列仅一 `leased`**。
- [ ] Agent 侧已发 **`running_tasks`**（与现 Go Agent 一致）。
- [ ] 人为拔网线：租约回收后任务重回 `pending`，新 Agent 可认领。
- [ ] 结果上报错误 `lease` 返回 **409**，与协议一致。

---

## 风险与依赖

| 项 | 说明 |
|----|------|
| **MySQL 版本** | 必须 8+（`SKIP LOCKED`） |
| **BOM 与 DevEnqueue** | Dev 入队须同样写 DB，否则双实例下拉不到 |
| **data 包测试** | 修复现有 `conf.Data_Database` 引用后再打开全量 `go test ./internal/data/...` |

---

**计划版本:** 2026-03-24 · 关联草图：[Agent任务调度-MySQL与Hub改造草图.md](../../Agent任务调度-MySQL与Hub改造草图.md)
