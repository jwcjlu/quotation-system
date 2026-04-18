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
