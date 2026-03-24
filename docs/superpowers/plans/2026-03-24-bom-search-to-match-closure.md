# BOM 货源搜索 → 报价落库 → 配单闭环 Implementation Plan

> **For agentic workers:** 实现时优先使用 [@c:\Users\Admin\.cursor\skills\superpowers\skills\subagent-driven-development\SKILL.md](../../subagent-driven-development.md)（推荐）或 executing-plans，**按 Task 顺序**完成；步骤使用 `- [ ]` 勾选跟踪。
>
> **建议：** 在独立 git worktree 中实施（见 [using-git-worktrees](../../using-git-worktrees.md)）。

**Goal:** 补齐端到端缺口：**Agent（或等同执行方）完成搜索任务后**，将结果写入 **`bom_search_task` / `bom_quote_cache`**；主站在配单前将 **DB 报价聚合为 `[]*biz.ItemQuotes`** 并注入 **`searchRepo`**（或 **`AutoMatch` 直接读 DB**），使 **`GetReadiness` 的 ready 语义**与 **`AutoMatch` 实际可用报价**一致。

**Architecture:**  
1）定义**任务完成入口**（HTTP 或仅内网 gRPC）：校验 `session_id` + `caichip_task_id`（或任务主键）+ 鉴权，事务内更新 **`bom_search_task.state/last_error`**，并 **UPSERT `bom_quote_cache`**（`quotes_json` 与现有 `GetBOMLines`/`QuoteCacheOutcome` 字段对齐）。  
2）在 **`internal/biz`** 增加 **「从会话维度加载报价」**：按 `bom_session_line` 的型号（与 `bom_search_task.mpn_norm` 对齐）聚合多平台 `Quote`，生成 **`[]*ItemQuotes`**（`Model` = BOM 行型号，`Quotes` = 各平台 `*Quote`）。  
3）**`AutoMatch` 前置**：对 **会话 BOM**（`bom_id == session_id`）先调用 **RefreshSessionQuotesToSearchRepo**（`searchRepo.SaveQuotes(session_id, …)`），再执行现有选型逻辑；或改造 `MatchUseCase` 注入 `QuotesProvider` 接口（DB 实现 + 内存 fallback）。  
4）与现有 **`caichip_dispatch_task` / `caichip_task_id`** 对齐，便于 Agent 回传时关联一行搜索任务。

**Tech Stack:** Go、Kratos、`database/sql`、MySQL、现有 `internal/biz/match.go` / `internal/data/bom_search_task_repo.go` / `internal/data/search_repo.go`、可选 `api/bom/v1` 新 RPC。

**背景结论（当前缺口）:** 见会话梳理：`SaveQuotes` **无调用方**；**无**将 Agent 结果写入 `bom_search_task`/`bom_quote_cache` 的实现；**`AutoMatch` 仅读内存 `quotesCache`**。

---

## 文件结构（创建 / 修改一览）

| 路径 | 职责 |
|------|------|
| **Create** `docs/BOM货源搜索-任务回写与配单衔接.md`（可选） | 状态机、`quotes_json` 形状、与 Agent 约定 |
| **Modify** `api/bom/v1/bom.proto` + 重生 pb | 新增 `SearchTaskReport` 或 `SubmitSearchResult` RPC（若走 HTTP 对内 API） |
| **Create** `internal/service/bom_search_callback.go`（或并入 `bom_service.go`） | 鉴权、参数校验、调 repo 事务 |
| **Modify** `internal/data/bom_search_task_repo.go` | `CompleteSearchTask` / `UpsertQuoteCache` / `SetTaskState`；必要 SELECT FOR UPDATE |
| **Create** `internal/biz/session_quotes_loader.go` | 从 DB 组 `[]*ItemQuotes`（依赖 `BOMSearchTaskRepo` 扩展或新 `QuoteAggregationRepo`） |
| **Modify** `internal/biz/match.go` 或 `internal/service/bom_service.go` | `AutoMatch` 前刷新 quotes；或 `MatchUseCase` 依赖 `QuotesProvider` |
| **Modify** `internal/service/bom_service.go` | `AutoMatch`：`bom_id` 为 session UUID 时先 `RefreshQuotes` |
| **Modify** `cmd/server/wire.go` / `internal/service/provider.go` | 注入新 repo / usecase |
| **Create** `internal/data/bom_search_task_repo_test.go` / `internal/biz/session_quotes_loader_test.go` | 表驱动 + `TEST_MYSQL_DSN` 门禁 |
| **Modify** `docs/BOM货源搜索-技术设计方案.md` 或接口清单 | 新端点与错误码 |

---

### Task 1: 约定 `bom_search_task` 状态与 `bom_quote_cache` 载荷

**Files:**
- Create: `docs/BOM货源搜索-任务回写与配单衔接.md`（可选，或只扩展现有技术方案）

- [x] **Step 1:** 列出 **合法 `state` 迁移**：`pending` → `running` / `dispatched`（若需要）→ `succeeded_quotes` | `succeeded_no_mpn` | `failed`；与 `GetReadiness`/`GetBOMLines` 分支一致。
- [x] **Step 2:** 定义 **`quotes_json` 最小 schema**（与 `biz.Quote` 字段可映射）：至少 `platform`、`matched_model`、`manufacturer`、`package`、`stock`、`lead_time`、`unit_price`；允许多条阶梯价时约定用 `price_tiers` 或单一 `unit_price`（YAGNI）。
- [x] **Step 3:** 定义 **`SubmitSearchResult` 请求必选字段**：`session_id`、`task_id` 或 `(mpn_norm, platform_id, biz_date)`、`caichip_task_id`（推荐与派发一致）、`status`、`quotes` JSON、`error_message`。
- [x] **Step 4:** Commit（按需）

```bash
git add docs/BOM货源搜索-任务回写与配单衔接.md
git commit -m "docs(bom): search task completion contract and quote JSON schema"
```

---

### Task 2: Repository — 更新任务 + 写报价缓存（TDD）

**Files:**
- Modify: `internal/data/bom_search_task_repo.go`
- Create: `internal/data/bom_search_task_repo_callback_test.go`（或 `_integration_test.go`，`TEST_MYSQL_DSN`）

- [x] **Step 1:** **失败测试**：给定已存在 `bom_search_task` 行，`CompleteSearchTaskSuccess` 将 `state` 设为 `succeeded_quotes` 且写入 `bom_quote_cache`（`outcome` 可置 `ok` 或沿用现有 `GetBOMLines` 解析逻辑）。
- [x] **Step 2:** 实现 `UpsertQuoteCache(ctx, mpnNorm, platformID, bizDate, outcome, quotesJSON []byte)`（与表主键 `(mpn_norm, platform_id, biz_date)` 一致）。
- [x] **Step 3:** 实现 `FinalizeSearchTask(...)`：**可选**校验 `caichip_task_id` 与当前行一致（防串任务）；并合并 `UpsertQuoteCache` 于事务内。
- [x] **Step 4:** `go test ./internal/data/... -count=1`（`TestFinalizeSearchTask_MySQL` 无 `TEST_MYSQL_DSN` 则 Skip）。
- [x] **Step 5:** Commit（按需）  

```bash
git add internal/data/bom_search_task_repo.go internal/data/bom_search_task_repo_callback_test.go
git commit -m "feat(data): finalize bom_search_task and upsert bom_quote_cache"
```

---

### Task 3: HTTP / RPC — 任务结果上报入口

**Files:**
- Modify: `api/bom/v1/bom.proto`
- Regenerate: `make api` 或 `protoc ... api/bom/v1/bom.proto`
- Modify: `internal/service/bom_service.go`
- Modify: `internal/server/http.go`（若需单独路由前缀如 `/internal/`）

- [x] **Step 1:** 在 proto 增加 **`SubmitBomSearchResult`**（命名可调整）：body 含上一 Task 字段；返回 `accepted` + `server_time`。
- [x] **Step 2:** **鉴权**：复用现有 BOM 是否需要 API Key — **建议** 独立 `bom_search_callback.api_keys`（`conf.proto`）或复用 `agent.api_keys`，在计划中二选一并写进配置示例。
- [x] **Step 3:** `BomService.SubmitBomSearchResult`：校验 session 存在、`biz_date` 与会话一致，调用 `FinalizeSearchTask` + `UpsertQuoteCache`。
- [x] **Step 4:** `go build ./cmd/server/...`
- [x] **Step 5:** Commit（按需）  

```bash
git add api/bom/v1/bom.proto api/bom/v1/*.pb.go internal/service/bom_service.go internal/conf/conf.proto internal/conf/conf.pb.go configs/config.yaml
git commit -m "feat(bom): submit search result API for agent callback"
```

---

### Task 4: `session_id` → `[]*ItemQuotes` 聚合器

**Files:**
- Create: `internal/biz/session_quotes_loader.go`
- Modify: `internal/data/bom_search_task_repo.go`（如缺「按 session 列出成功任务 + join cache」查询）

- [x] **Step 1:** 实现 **`biz.LoadItemQuotesForSession`** + **`BOMSearchTaskRepo.LoadSucceededQuoteRowsForSession`**（join 缓存）；`biz_date` 由调用方传入（来自 `GetSession`）。
- [x] **Step 2:** 单元测试：`session_quotes_loader_test.go`。
- [x] **Step 3:** Commit（按需）

```bash
git add internal/biz/session_quotes_loader.go internal/data/bom_search_task_repo.go
git commit -m "feat(biz): aggregate DB quotes into ItemQuotes for session"
```

---

### Task 5: 配单前刷新 `searchRepo`（闭环核心）

**Files:**
- Modify: `internal/service/bom_service.go`
- Modify: `internal/biz/match.go`（可选：提取 `AutoMatchWithQuotesProvider`）

- [x] **Step 1:** 在 **`AutoMatch`** 开头：若 `uuid.Parse(bom_id)` 成功 **且** `BOMSearchTaskRepo.DBOk()`，调用 `LoadItemQuotesForSession` + **`searchRepo.SaveQuotes(bom_id, quotes)`**。
- [x] **Step 2:** （YAGNI）就绪但聚合为空不额外返回 409/503，与计划可选说明一致。
- [x] **Step 3:** `go test ./internal/service/... ./internal/biz/... -count=1`
- [x] **Step 4:** Commit（按需）

```bash
git add internal/service/bom_service.go internal/biz/match.go
git commit -m "feat(bom): refresh searchRepo quotes from DB before AutoMatch"
```

---

### Task 6: Agent 侧契约（文档与样例）

**Files:**
- Modify: `docs/分布式采集Agent-API协议.md` 或单独 `docs/BOM-Agent-搜索回传.md`

- [x] **Step 1:** 见 `docs/BOM-Agent-搜索回传.md` 推荐顺序。
- [x] **Step 2:** 同文件内 `curl` 示例。
- [x] **Step 3:** Commit（按需）

```bash
git add docs/BOM-Agent-搜索回传.md
git commit -m "docs(bom): agent callback for search results"
```

---

### Task 7: 回归与验收

- [ ] **Step 1:** 手工：`CreateSession` → `UploadBOM` → `PutPlatforms` → 模拟 `SubmitBomSearchResult` 一行 → `GetBOMLines` 有报价摘要 → `AutoMatch` 非全 `no_match`（构造数据需覆盖 `filterFullyMatched`）。**（需在环境内自测）**
- [x] **Step 2:** 已对 `internal/biz`、`internal/data`、`internal/service` 跑 `go test -count=1`；全量 `./...` 可按需本地执行。
- [x] **Step 3:** 本文件 Task 1–6 已勾选；Task 7 Step 1 待联调环境。

---

## 非目标（YAGNI / 另立计划）

- 不重写 `MatchUseCase` 选型算法；不引入实时 WebSocket。
- 不强制 Redis；不迁移 `BOMRepo` 到 MySQL（会话仍依赖内存 BOM + session lines）。

---

## 风险

| 项 | 缓解 |
|----|------|
| `quotes_json` 与爬虫输出不一致 | 首版在 loader 内做 **宽松解析** + 字段缺省；协议文档锁定一版 schema |
| 并发完成同一任务 | DB 层 **幂等** + `caichip_task_id` 校验 + 终态不可降级 |
| `biz.Model` 与 `mpn_norm` 不一致 | 统一用 **`normalizeMPNForTask`**（与 `bom_search_task_repo` 一致） |

---

**计划版本:** 2026-03-24 · 依据对话结论：「搜索回写未实现、`SaveQuotes` 未调用、`AutoMatch` 未读 DB 报价」。
