# BOM 报价缓存拆表（直接切换）Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 直接将 BOM 报价存储从 `t_bom_quote_cache.quotes_json` 切换为 `t_bom_quote_item` 明细表，并同步代码读写链路；不考虑拆分前后兼容。

**Architecture:** 单阶段切换：先执行 schema 变更（`t_bom_quote_cache` 改 `id` 主键 + 新建 `t_bom_quote_item` + 删除 `quotes_json`），随后一次性切换 `data/biz/service` 到新结构。

**Tech Stack:** Go（Kratos）、GORM、MySQL 8、SQL migration。

---

## Task 1: Schema 直切

**Files:**
- `docs/schema/migrations/20260415_bom_quote_item_split.sql`
- `docs/schema/bom_mysql.sql`
- `docs/schema/bom_mysql_complete.sql`

- [ ] 确认迁移顺序：加 `id` -> 切主键 -> 建唯一索引 -> 建 `t_bom_quote_item` -> 删 `quotes_json`
- [ ] 在测试库执行迁移并重复执行一次（幂等验证）
- [ ] 校验：`t_bom_quote_cache` 无 `quotes_json`，`t_bom_quote_item.quote_id` 外键存在

## Task 2: Data 层改造

**Files:**
- `internal/data/models.go`
- `internal/data/table_names.go`
- `internal/data/migrate.go`
- `internal/data/bom_search_task_repo.go`

- [ ] `FinalizeSearchTask` 改为：upsert cache 主表后 delete+insert item 明细
- [ ] `LoadQuoteCacheByMergeKey`/`LoadQuoteCachesForKeys` 改为返回 item 明细
- [ ] 补 data 层单测并通过

## Task 3: Biz 层改造

**Files:**
- `internal/biz/bom_line_match.go`
- `internal/biz/task_stdout_quotes.go`

- [ ] `LineMatchInput` 去掉 `QuotesJSON`，改成 `[]AgentQuoteRow`
- [ ] 配单选优逻辑改为基于 item 明细
- [ ] 补 biz 层测试并通过

## Task 4: Service 装配改造

**Files:**
- `internal/service/bom_match_session_caches.go`
- `internal/service/bom_service.go`
- `cmd/server/wire.go`
- `internal/data/provider.go`

- [ ] 会话缓存装配改为读取 item 明细
- [ ] `SearchQuotes` / `GetMatchResult` 结果行为回归校验
- [ ] 编译通过

## Task 5: 验证与发布检查

- [ ] `go test ./internal/data/...`
- [ ] `go test ./internal/biz/...`
- [ ] `go test ./internal/service/...`
- [ ] `go build ./cmd/server/...`
- [ ] 2 组真实样例端到端验证（含 `price_tiers`、`lead_time`）

---

## 风险与控制

- **风险：** 直切后旧代码无法运行  
  **控制：** migration 与代码同版本发布，不拆批次。
- **风险：** 配单结果偏差  
  **控制：** 保留固定回归样例，切换后逐条比对。
# BOM 报价缓存拆表（直接切换）Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 直接将 BOM 报价存储从 `t_bom_quote_cache.quotes_json` 切换为 `t_bom_quote_item` 明细表，并同步代码读写链路；本次不考虑拆分前后兼容。

**Architecture:** 单阶段切换：先执行 schema 变更（`t_bom_quote_cache` 改 `id` 主键 + 新建 `t_bom_quote_item` + 删除 `quotes_json`），随后一次性切换 `data/biz/service` 到新结构。只保留 `t_bom_quote_item` 作为报价候选来源。

**Tech Stack:** Go（Kratos）、GORM、MySQL 8、SQL migration。

---

## 文件结构（创建 / 修改一览）

| 路径 | 职责 |
|------|------|
| `docs/schema/migrations/20260415_bom_quote_item_split.sql` | 直切迁移：加 `id`、建 item 表、删 `quotes_json` |
| `docs/schema/bom_mysql.sql` | 新库全量结构与直切方案一致 |
| `docs/schema/bom_mysql_complete.sql` | 完整脚本口径一致 |
| `internal/data/models.go` | 定义 `BomQuoteItem`，调整 `BomQuoteCache` 字段 |
| `internal/data/table_names.go` | 增加 `TableBomQuoteItem` |
| `internal/data/migrate.go` | 注册 `BomQuoteItem` |
| `internal/data/bom_search_task_repo.go` | 回写缓存时改为写 item 子表；读取缓存时联表取 item |
| `internal/biz/task_stdout_quotes.go` | stdout 解析结果用于 item 入库 |
| `internal/biz/bom_line_match.go` | 输入改为 item 明细数组，不再依赖 `QuotesJSON` |
| `internal/service/bom_match_session_caches.go` | 组装会话缓存时读取 item 明细 |

---

### Task 1: Schema 直切

**Files:**
- Modify: `docs/schema/migrations/20260415_bom_quote_item_split.sql`
- Modify: `docs/schema/bom_mysql.sql`
- Modify: `docs/schema/bom_mysql_complete.sql`

- [ ] **Step 1:** 在迁移中确保顺序为：加 `id` -> 切主键 -> 建 `uk_bom_quote_cache_merge` -> 建 `t_bom_quote_item` -> 删 `quotes_json`。
- [ ] **Step 2:** 在测试库执行迁移脚本并二次重跑，确认幂等可重复执行。
- [ ] **Step 3:** 验证表结构：`t_bom_quote_cache` 无 `quotes_json`，`t_bom_quote_item.quote_id` 外键存在。
- [ ] **Step 4:** Commit

```bash
git add docs/schema/migrations/20260415_bom_quote_item_split.sql docs/schema/bom_mysql.sql docs/schema/bom_mysql_complete.sql
git commit -m "feat(schema): direct cutover to bom quote item table"
```

---

### Task 2: Data 模型与仓储接口改造

**Files:**
- Modify: `internal/data/models.go`
- Modify: `internal/data/table_names.go`
- Modify: `internal/data/migrate.go`
- Modify: `internal/data/bom_search_task_repo.go`
- Test: `internal/data/*_test.go`

- [ ] **Step 1: 写失败测试**：`FinalizeSearchTask` 后应写入 cache 主行 + N 条 item 明细。
- [ ] **Step 2:** 跑测试确认失败（当前仍依赖 `quotes_json`）。
- [ ] **Step 3:** 实现最小改造：upsert cache 获取 `id`，按 `quote_id` delete+insert item 明细。
- [ ] **Step 4:** 读取接口（`LoadQuoteCacheByMergeKey`/`LoadQuoteCachesForKeys`）返回 item 明细数组。
- [ ] **Step 5:** 测试通过并 Commit。

```bash
git add internal/data/models.go internal/data/table_names.go internal/data/migrate.go internal/data/bom_search_task_repo.go
git commit -m "refactor(data): store and load quote rows from t_bom_quote_item"
```

---

### Task 3: Biz 配单读取切换

**Files:**
- Modify: `internal/biz/bom_line_match.go`
- Modify: `internal/biz/task_stdout_quotes.go`
- Test: `internal/biz/*_test.go`

- [ ] **Step 1: 写失败测试**：仅有 item 明细输入时，`PickBestQuoteForLine` 仍能正确选优。
- [ ] **Step 2:** 改 `LineMatchInput`，移除 `QuotesJSON` 依赖，改为 `[]AgentQuoteRow`。
- [ ] **Step 3:** 适配调用链，确保价格提取、交期、厂牌逻辑行为不变。
- [ ] **Step 4:** 跑 `go test ./internal/biz/...` 全绿并 Commit。

```bash
git add internal/biz/bom_line_match.go internal/biz/task_stdout_quotes.go
git commit -m "refactor(biz): consume quote items instead of quotes_json"
```

---

### Task 4: Service/编排层切换

**Files:**
- Modify: `internal/service/bom_match_session_caches.go`
- Modify: `internal/service/bom_service.go`（若涉及）
- Modify: `cmd/server/wire.go` `internal/data/provider.go`（如接口变化）
- Test: `internal/service/*_test.go`

- [ ] **Step 1:** 会话缓存装配逻辑改为读取 item 明细并传给 biz。
- [ ] **Step 2:** 核对 `SearchQuotes` / `GetMatchResult` 返回行为与切换前一致。
- [ ] **Step 3:** 跑 `go test ./internal/service/...` 与 `go build ./cmd/server/...`。
- [ ] **Step 4:** Commit。

```bash
git add internal/service/bom_match_session_caches.go internal/service/bom_service.go cmd/server/wire.go internal/data/provider.go
git commit -m "refactor(service): wire quote item based matching flow"
```

---

### Task 5: 端到端验证与发布检查

- [ ] **Step 1:** 在测试库准备真实样例（含 `price_tiers`、`lead_time`、`manufacturer`）。
- [ ] **Step 2:** 执行完整流程：任务回写 -> item 入库 -> `SearchQuotes` / `GetMatchResult`。
- [ ] **Step 3:** 对比核心输出：候选数量、最低价、平局交期优先是否符合预期。

---

## 验证清单（完成前必须执行）

- [ ] `go test ./internal/data/...`
- [ ] `go test ./internal/biz/...`
- [ ] `go test ./internal/service/...`
- [ ] `go build ./cmd/server/...`
- [ ] migration 在测试库执行成功，且 `t_bom_quote_cache` 不再包含 `quotes_json`
- [ ] 至少 2 组平台样例端到端通过

---

## 风险与控制

- **风险：** 一次性切换导致旧代码不可用。  
  **控制：** 严格按 Task 顺序推进，迁移与代码同版本发布。
- **风险：** 读写链路改造后配单结果偏差。  
  **控制：** 对关键样例做回归集校验。
- **风险：** migration 失败阻塞发布。  
  **控制：** 预演 + 幂等验证 + 发布窗口前预检查。

---

**Plan complete and saved to `docs/superpowers/plans/2026-04-15-bom-quote-item-split-implementation.md`. Two execution options:**

**1. Subagent-Driven (recommended)** - I dispatch a fresh subagent per task, review between tasks, fast iteration

**2. Inline Execution** - Execute tasks in this session using executing-plans, batch execution with checkpoints

**Which approach?**
# BOM 报价缓存拆表（直接切换）Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 直接将 BOM 报价存储从 `t_bom_quote_cache.quotes_json` 切换为 `t_bom_quote_item` 明细表，并同步代码读写链路；本次不考虑拆分前后兼容。

**Architecture:** 单阶段切换：先执行 schema 变更（`t_bom_quote_cache` 改 `id` 主键 + 新建 `t_bom_quote_item` + 删除 `quotes_json`），随后一次性切换 `data/biz/service` 到新结构。只保留 `t_bom_quote_item` 作为报价候选来源。

**Tech Stack:** Go（Kratos）、GORM、MySQL 8、SQL migration。

---

## 文件结构（创建 / 修改一览）

| 路径 | 职责 |
|------|------|
| `docs/schema/migrations/20260415_bom_quote_item_split.sql` | 直切迁移：加 `id`、建 item 表、删 `quotes_json` |
| `docs/schema/bom_mysql.sql` | 新库全量结构与直切方案一致 |
| `docs/schema/bom_mysql_complete.sql` | 完整脚本口径一致 |
| `internal/data/models.go` | 定义 `BomQuoteItem`，调整 `BomQuoteCache` 字段 |
| `internal/data/table_names.go` | 增加 `TableBomQuoteItem` |
| `internal/data/migrate.go` | 注册 `BomQuoteItem` |
| `internal/data/bom_search_task_repo.go` | 回写缓存时改为写 item 子表；读取缓存时联表取 item |
| `internal/biz/task_stdout_quotes.go` | stdout 解析结果用于 item 入库 |
| `internal/biz/bom_line_match.go` | 输入改为 item 明细数组，不再依赖 `QuotesJSON` |
| `internal/service/bom_match_session_caches.go` | 组装会话缓存时读取 item 明细 |

---

### Task 1: Schema 直切

**Files:**
- Modify: `docs/schema/migrations/20260415_bom_quote_item_split.sql`
- Modify: `docs/schema/bom_mysql.sql`
- Modify: `docs/schema/bom_mysql_complete.sql`

- [ ] **Step 1:** 在迁移中确保顺序为：加 `id` -> 切主键 -> 建 `uk_bom_quote_cache_merge` -> 建 `t_bom_quote_item` -> 删 `quotes_json`。
- [ ] **Step 2:** 在测试库执行迁移脚本并二次重跑，确认幂等可重复执行。
- [ ] **Step 3:** 验证表结构：`t_bom_quote_cache` 无 `quotes_json`，`t_bom_quote_item.quote_id` 外键存在。
- [ ] **Step 4:** Commit

```bash
git add docs/schema/migrations/20260415_bom_quote_item_split.sql docs/schema/bom_mysql.sql docs/schema/bom_mysql_complete.sql
git commit -m "feat(schema): direct cutover to bom quote item table"
```

---

### Task 2: Data 模型与仓储接口改造

**Files:**
- Modify: `internal/data/models.go`
- Modify: `internal/data/table_names.go`
- Modify: `internal/data/migrate.go`
- Modify: `internal/data/bom_search_task_repo.go`
- Test: `internal/data/*_test.go`

- [ ] **Step 1: 写失败测试**：`FinalizeSearchTask` 后应写入 cache 主行 + N 条 item 明细。
- [ ] **Step 2:** 跑测试确认失败（当前仍依赖 `quotes_json`）。
- [ ] **Step 3:** 实现最小改造：upsert cache 获取 `id`，按 `quote_id` delete+insert item 明细。
- [ ] **Step 4:** 读取接口（`LoadQuoteCacheByMergeKey`/`LoadQuoteCachesForKeys`）返回 item 明细数组。
- [ ] **Step 5:** 测试通过并 Commit。

```bash
git add internal/data/models.go internal/data/table_names.go internal/data/migrate.go internal/data/bom_search_task_repo.go
git commit -m "refactor(data): store and load quote rows from t_bom_quote_item"
```

---

### Task 3: Biz 配单读取切换

**Files:**
- Modify: `internal/biz/bom_line_match.go`
- Modify: `internal/biz/task_stdout_quotes.go`
- Test: `internal/biz/*_test.go`

- [ ] **Step 1: 写失败测试**：仅有 item 明细输入时，`PickBestQuoteForLine` 仍能正确选优。
- [ ] **Step 2:** 改 `LineMatchInput`，移除 `QuotesJSON` 依赖，改为 `[]AgentQuoteRow`。
- [ ] **Step 3:** 适配调用链，确保价格提取、交期、厂牌逻辑行为不变。
- [ ] **Step 4:** 跑 `go test ./internal/biz/...` 全绿并 Commit。

```bash
git add internal/biz/bom_line_match.go internal/biz/task_stdout_quotes.go
git commit -m "refactor(biz): consume quote items instead of quotes_json"
```

---

### Task 4: Service/编排层切换

**Files:**
- Modify: `internal/service/bom_match_session_caches.go`
- Modify: `internal/service/bom_service.go`（若涉及）
- Modify: `cmd/server/wire.go` `internal/data/provider.go`（如接口变化）
- Test: `internal/service/*_test.go`

- [ ] **Step 1:** 会话缓存装配逻辑改为读取 item 明细并传给 biz。
- [ ] **Step 2:** 核对 `SearchQuotes` / `GetMatchResult` 返回行为与切换前一致。
- [ ] **Step 3:** 跑 `go test ./internal/service/...` 与 `go build ./cmd/server/...`。
- [ ] **Step 4:** Commit。

```bash
git add internal/service/bom_match_session_caches.go internal/service/bom_service.go cmd/server/wire.go internal/data/provider.go
git commit -m "refactor(service): wire quote item based matching flow"
```

---

### Task 5: 端到端验证与发布检查

**Files:**
- Modify: `docs/superpowers/specs/2026-03-28-bom-match-currency-mfr-design.md`（如需补实现备注）

- [ ] **Step 1:** 在测试库准备真实样例（含 `price_tiers`、`lead_time`、`manufacturer`）。
- [ ] **Step 2:** 执行完整流程：任务回写 -> item 入库 -> `SearchQuotes` / `GetMatchResult`。
- [ ] **Step 3:** 对比核心输出：候选数量、最低价、平局交期优先是否符合预期。
- [ ] **Step 4:** 完成发布前检查清单并 Commit 文档更新（如有）。

---

## 验证清单（完成前必须执行）

- [ ] `go test ./internal/data/...`
- [ ] `go test ./internal/biz/...`
- [ ] `go test ./internal/service/...`
- [ ] `go build ./cmd/server/...`
- [ ] migration 在测试库执行成功，且 `t_bom_quote_cache` 不再包含 `quotes_json`
- [ ] 至少 2 组平台样例端到端通过

---

## 风险与控制

- **风险：** 一次性切换导致旧代码不可用。  
  **控制：** 严格按 Task 顺序推进，迁移与代码同版本发布。
- **风险：** 读写链路改造后配单结果偏差。  
  **控制：** 对关键样例做切换前后结果对比，锁定回归集。
- **风险：** migration 失败阻塞发布。  
  **控制：** 预演 + 幂等验证 + 发布窗口前预检查。

---

**Plan complete and saved to `docs/superpowers/plans/2026-04-15-bom-quote-item-split-implementation.md`. Two execution options:**

**1. Subagent-Driven (recommended)** - I dispatch a fresh subagent per task, review between tasks, fast iteration

**2. Inline Execution** - Execute tasks in this session using executing-plans, batch execution with checkpoints

**Which approach?**
