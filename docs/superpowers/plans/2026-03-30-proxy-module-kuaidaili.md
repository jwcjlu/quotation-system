# 快代理 + 平台 require_proxy + 调度载荷 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 按 [`docs/superpowers/specs/2026-03-30-proxy-module-kuaidaili-design.md`](../specs/2026-03-30-proxy-module-kuaidaili-design.md)，实现平台级 `require_proxy`、服务端调用快代理 [getdps](https://www.kuaidaili.com/doc/product/api/getdps/) 获取私密代理、将 `proxy_*` 写入 `t_caichip_dispatch_task.params_json`。BOM 合并路径采用 **策略 B（已锁定）**：**代理失败不占 inflight、不 enqueue、不 attach**；通过 **`t_bom_merge_proxy_wait`** + worker 对 **MergeKey** 做指数退避重试，耗尽后对相关 `BomSearchTask` 记失败。通用 `pending` 任务仍可用 **`next_claim_at`** 延迟认领（非 BOM 占位场景）。

**Architecture:**  
- **配置**：`conf.Bootstrap`：`Kuaidaili` + `ProxyBackoff`（与 spec §4.2 保守默认一致）。  
- **客户端**：`pkg/kuaidaili`，纯 HTTP + JSON。  
- **数据**：`t_caichip_dispatch_task.next_claim_at`（认领过滤）；**新建** `t_bom_merge_proxy_wait`（策略 B 退避状态）。  
- **BOM 成功路径**：`TryDispatchMergeKey` **事务前** `GetDPS` 成功 → 现有短事务：`inflight` + `enqueue`（`params` 含 `proxy_*`，`next_claim_at` 默认 NULL）+ `attachPendingBOMTasks`。  
- **BOM 失败路径**：**不开启**合并事务；upsert `t_bom_merge_proxy_wait`（`next_retry_at`、`attempt`、`last_error`）。  
- **MergeProxyRetryWorker**：tick 扫描等待表到期行 → `TryDispatchMergeKey`；成功则删等待行；再次失败则更新退避；超限则 fail BOM 行并删等待行。  
- **速率**：进程内对 getdps **限流**（与 worker + 调度并发叠加）。

**Tech Stack:** Go、Kratos、GORM、Wire、快代理 HTTPS API。

**Spec:** [`2026-03-30-proxy-module-kuaidaili-design.md`](../specs/2026-03-30-proxy-module-kuaidaili-design.md)（§7 策略 B）

---

## 文件结构（创建 / 修改）

| 路径 | 职责 |
|------|------|
| **Create** `docs/schema/migrations/YYYYMMDD_dispatch_task_next_claim_at.sql` | `next_claim_at` + 索引 |
| **Create** `docs/schema/migrations/YYYYMMDD_bom_merge_proxy_wait.sql` | `t_bom_merge_proxy_wait` |
| **Modify** `internal/data/models.go` | `CaichipDispatchTask`、`BomMergeProxyWait` 模型 |
| **Create** `pkg/kuaidaili/client.go`、`client_test.go` | getdps |
| **Modify** `internal/conf/conf.proto` | `Kuaidaili`、`ProxyBackoff` |
| **Modify** `internal/data/dispatch_task_repo.go` | `PullAndLeaseForAgent` 过滤 `next_claim_at` |
| **Modify** `internal/data/bom_merge_dispatch.go` | 事务前 `GetDPS`；失败写等待表、return |
| **Create** `internal/data/bom_merge_proxy_wait_repo.go` | Upsert / ListDue / Delete |
| **Create** `internal/biz/proxy_backoff.go` | `ComputeDelay` + jitter |
| **Create** `internal/data/merge_proxy_retry_worker.go` | 扫描 + `TryDispatchMergeKey` |
| **Modify** `internal/biz/repo.go` / `bom_service` | 若需 `TryDispatchMergeKey` 注入 worker |
| **Modify** `cmd/server/app.go`、`wire.go` | Worker 生命周期 |
| **Modify** `docs/分布式采集Agent-API协议.md` | `proxy_*` 字段说明 |

---

### Task 1: 迁移与模型

**Files:** migrations、`models.go`、`migrate.go`

- [ ] **Step 1:** `t_caichip_dispatch_task.next_claim_at`。
- [ ] **Step 2:** `t_bom_merge_proxy_wait`（主键合并键三列 + `next_retry_at`、`attempt`、`last_error`、`first_failed_at` 可选）。
- [ ] **Step 3:** GORM 模型与 AutoMigrate（若启用）。
- [ ] **Step 4:** Commit。

---

### Task 2: 认领 SQL 过滤

**Files:** `internal/data/dispatch_task_repo.go`

- [ ] **Step 1:** `pending` 增加 `(next_claim_at IS NULL OR next_claim_at <= NOW(3))`。
- [ ] **Step 2:** `ReclaimStaleLeases` 不打断代理相关语义（见 spec）。
- [ ] **Step 3:** 测试 / `go build`。
- [ ] **Step 4:** Commit。

---

### Task 3: 快代理客户端

**Files:** `pkg/kuaidaili/*`

- [ ] **Step 1:** 签名 + JSON 解析 + 错误码。
- [ ] **Step 2:** `httptest` 单测。
- [ ] **Step 3:** Commit。

---

### Task 4: 配置 + 退避函数

**Files:** `conf.proto`、`proxy_backoff.go`、生成 pb

- [ ] **Step 1:** 默认与 spec §4.2 一致。
- [ ] **Step 2:** 单测边界（jitter 可注入）。
- [ ] **Step 3:** Commit。

---

### Task 5: BomMergeDispatch — 策略 B

**Files:** `bom_merge_dispatch.go`、`bom_merge_proxy_wait_repo.go`、解析 `require_proxy`

- [ ] **Step 1:** 从 `RunParamsJSON` 读 `require_proxy`。
- [ ] **Step 2:** **不需要代理**：现状不变。
- [ ] **Step 3:** **需要代理**：在 **`tryDispatchMergeKeyTx` 的 `Transaction` 之前** 调用 `GetDPS`（带限流）。  
  - **成功**：将 `proxy_*` merge 进即将构建的 `QueuedTask.Params`，`next_claim_at` 不设；进入 **现有** 事务（inflight + enqueue + attach）。  
  - **失败**：**不**调用 `Transaction`；`Upsert` `t_bom_merge_proxy_wait`（`attempt` 自增或首建、`next_retry_at=now+ComputeDelay(attempt)`、`last_error` 摘要）；**return nil**（或返回可观测的哨兵错误由上层决定是否打日志 —— 建议 **nil** 避免上层误当作硬错误中断批处理）。
- [ ] **Step 4:** 成功入队后 **无需** 删等待行（因从未插入）；若同一合并键曾失败后又 **其它路径** 成功，应 **delete wait row**（可在成功事务提交后 `DeleteWait(mpn, platform, date)` 幂等清理）。
- [ ] **Step 5:** Commit。

---

### Task 6: MergeProxyRetryWorker

**Files:** `merge_proxy_retry_worker.go`、`app.go`、`wire.go`

- [ ] **Step 1:** 查询 `next_retry_at <= NOW(3)` LIMIT N。
- [ ] **Step 2:** 每条调用 `TryDispatchMergeKey(ctx, mpn, platform, bizDate)`。  
  - 若再次因代理失败，Task 5 会 upsert 等待行（更新 `next_retry_at`）。  
  - 若 `attempt` / wall clock 超限：**不**再延长；将关联 `pending` 的 `BomSearchTask` 标记为失败（`last_error` 含代理摘要），**delete** 等待行（需查询规则：同合并键多 session 行一并失败或仅未附着 task —— **首版**：该合并键下仍 `pending` 且 `caichip_task_id` 为空的行批量失败）。
- [ ] **Step 3:** 成功调度后 **delete** 等待行（Task 5 成功路径末尾或 worker 在 Try 返回后检测 inflight 已建 —— **更简单**：Task 5 成功事务后 `DeleteWait`）。
- [ ] **Step 4:** `Start`/`Stop`、配置开关。
- [ ] **Step 5:** Commit。

---

### Task 7: Agent 协议

**Files:** `docs/分布式采集Agent-API协议.md`

- [ ] **Step 1:** 文档化 `proxy_host` / `proxy_port` / 鉴权字段。
- [ ] **Step 2:** Commit。

---

### Task 8: 集成验收

- [ ] **Step 1:** `go test`、`go build ./cmd/server/...`。
- [ ] **Step 2:** Commit。

---

## 风险与依赖

| 项 | 说明 |
|----|------|
| 超限 fail | 多 session 同合并键时批量失败策略需在 Task 6 写清，避免误伤已 `running` 行。 |
| getdps 频率 | Worker 批量 `TryDispatchMergeKey` 可能放大调用；**必须**限流。 |
| 密钥 | 仅服务端配置；日志打码。 |

---

## Execution Handoff

实现顺序：**Task 1 → 2 → 3 → 4 → 5 → 6 → 7 → 8**。
