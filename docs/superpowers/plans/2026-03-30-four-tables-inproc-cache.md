# 四表进程内缓存 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 `t_caichip_agent_script_auth`、`t_bom_platform_script`、`t_caichip_agent_installed_script`、`t_bom_manufacturer_alias` 在**单进程**内实现「定时全量刷新 + 读穿 + 写后删键」，不引入 Redis。

**Architecture:** `internal/data` 中 `InprocKV`（`sync.RWMutex` + `map[string]any`）+ 各表 **Cached*Repo** 装饰器：`读 → cache → miss 则 inner → Set`；`写成功 → Delete 相关 key`。`TableCacheRefresher` 用 `time.Ticker`（或由 `Start`/`Stop` 管理 goroutine），在 Kratos `BeforeStart`/`BeforeStop` 挂接。配置在 `conf.Bootstrap.table_cache`（`enabled`、`refresh_interval_sec`）。

**Tech Stack:** Go、Kratos v2、GORM、Wire、`internal/biz` 接口（`AgentScriptAuthRepo`、`BomPlatformScriptRepo`、`AgentRegistryRepo`、`BomManufacturerAliasRepo`）。

**Spec:** [`docs/superpowers/specs/2026-03-30-four-tables-inproc-cache-design.md`](../specs/2026-03-30-four-tables-inproc-cache-design.md)

---

## 实现状态（仓库快照）

下列与 **caichip 当前主分支实现** 对齐；若你从零实现，请仍按下方 Task 顺序执行并把 `[x]` 当作验收勾选项。

| Task | 状态 | 主要落地文件 |
|------|------|----------------|
| 1 配置与键 | 已实现 | `internal/conf/conf.proto`（`TableCache`）、`internal/data/table_cache_keys.go` |
| 2 InprocKV | 已实现 | `internal/data/inproc_kv.go`、`inproc_kv_test.go` |
| 3 厂牌别名缓存 | 已实现 | `internal/biz/repo.go`（`BomManufacturerAliasRepo`）、`bom_manufacturer_alias_repo_cache.go` |
| 4 AgentScriptAuth | 已实现 | `agent_script_auth_repo_cache.go` |
| 5 BomPlatformScript | 已实现 | `bom_platform_script_repo_cache.go` |
| 6 AgentRegistry | 已实现 | `agent_registry_repo_cache.go` |
| 7 Refresher + 生命周期 | 已实现 | `table_cache_refresher.go`（`refreshOnce`：`platInner.List` + `db.Find` 三表分组 + `aliasInner.ListDistinctCanonicals`）、`cmd/server/app.go`（Start/Stop） |
| 8 联调 | 需持续 | `go test ./...`、`go build ./cmd/server/...` |

**Refresher 实作要点（与设计对照）：** 全量预热包含 **bom 平台列表**、**script_auth 按 agent 分组**、**manufacturer_alias 按 norm 点键**、**installed_script 按 agent 分组**，以及 **canonicals 列表**（`limit` 300/1000）；失败仅打日志、**不清空** KV。

---

## 文件结构（创建 / 修改）

| 路径 | 职责 |
|------|------|
| `internal/data/inproc_kv.go` | `Get` / `Set` / `Delete` / `DeletePrefix` |
| `internal/data/inproc_kv_test.go` | 并发、`DeletePrefix` |
| `internal/data/table_cache_keys.go` | `KeyBomPlatformAll`、`KeyAsAuthAgent`、`KeyAgInstAgent`、`KeyMfrAliasNorm`、`KeyMfrAliasCanonicalsList` 等 |
| `internal/data/agent_script_auth_repo_cache.go` | `CachedAgentScriptAuthRepo` → `biz.AgentScriptAuthRepo` |
| `internal/data/bom_platform_script_repo_cache.go` | `CachedBomPlatformScriptRepo` |
| `internal/data/agent_registry_repo_cache.go` | `CachedAgentRegistryRepo`（`ListInstalledScriptsForAgent` 缓存；心跳写后删 `aginst`） |
| `internal/data/bom_manufacturer_alias_repo_cache.go` | `CachedBomManufacturerAliasRepo` |
| `internal/data/table_cache_refresher.go` | 定时 `refreshOnce` |
| `internal/conf/conf.proto` | `message TableCache { bool enabled; int32 refresh_interval_sec; }` |
| `internal/data/provider.go` | `NewInprocKV`、`NewCached*`、`NewTableCacheRefresher`、`wire.Bind` |
| `cmd/server/wire.go` / `wire_gen.go` | 绑定 Cached 实现 |
| `cmd/server/app.go` | `TableCacheRefresher.Start()` / `Stop()` |
| `internal/service/bom_service.go` | `BomManufacturerAliasRepo` 接口注入（若尚未） |

---

### Task 1: 配置与键常量

**Files:** `internal/conf/conf.proto`、`internal/data/table_cache_keys.go`、生成 `conf.pb.go`

- [ ] **Step 1:** `Bootstrap` 增加 `table_cache`：`enabled`、`refresh_interval_sec`（0 可表示仅手动/单次策略，与代码注释一致）。
- [ ] **Step 2:** 生成 proto。
- [ ] **Step 3:** `table_cache_keys.go` 集中 key 构造函数，避免魔法字符串。
- [ ] **Step 4:** Commit。

---

### Task 2: InprocKV 与单元测试

**Files:** `internal/data/inproc_kv.go`、`internal/data/inproc_kv_test.go`

- [ ] **Step 1:** `InprocKV`：`Get`/`Set`/`Delete`/`DeletePrefix`，`nil` 安全。
- [ ] **Step 2:** `go test -race ./internal/data/ -run InprocKV -count=1` → PASS。
- [ ] **Step 3:** Commit。

---

### Task 3: BomManufacturerAlias 接口化 + 缓存装饰器

**Files:** `internal/biz/repo.go`、`internal/data/bom_manufacturer_alias_repo.go`、`bom_manufacturer_alias_repo_cache.go`、`internal/service/bom_service.go`、`provider.go`

- [ ] **Step 1:** `biz.BomManufacturerAliasRepo` 与 data 实现一致；编译断言 `var _ biz.BomManufacturerAliasRepo = (*BomManufacturerAliasRepo)(nil)`。
- [ ] **Step 2:** `CachedBomManufacturerAliasRepo`：读穿 + `CreateRow` 后删 norm 键与 `mfalias:canonicals:` 前缀（与现实现一致）。
- [ ] **Step 3:** `go test ./internal/data/... -run ManufacturerAlias -count=1`（若有）或 `go build ./cmd/server/...`。
- [ ] **Step 4:** Commit。

---

### Task 4: CachedAgentScriptAuthRepo

**Files:** `agent_script_auth_repo_cache.go`、`provider.go`、`wire.go`

- [ ] **Step 1:** `ListByAgent` / `GetPlatformAuth` 读穿；`Upsert`/`Delete` 后删 `asauth:agent:{id}` 及可选单行 key。
- [ ] **Step 2:** `wire.Bind(new(biz.AgentScriptAuthRepo), new(*CachedAgentScriptAuthRepo))`。
- [ ] **Step 3:** `go build ./cmd/server/...`。
- [ ] **Step 4:** Commit。

---

### Task 5: CachedBomPlatformScriptRepo

**Files:** `bom_platform_script_repo_cache.go`、`provider.go`、`wire.go`

- [ ] **Step 1:** `List` → `bomplat:all`；`Get` 可从全量切片派生或读穿单行。
- [ ] **Step 2:** `Upsert`/`Delete` → 删 `bomplat:all` 与 `bomplat:{platform_id}`。
- [ ] **Step 3:** Commit。

---

### Task 6: CachedAgentRegistryRepo

**Files:** `agent_registry_repo_cache.go`、`provider.go`、`wire.go`

- [ ] **Step 1:** 缓存 `ListInstalledScriptsForAgent`（`aginst:agent:{id}`）。
- [ ] **Step 2:** `UpsertTaskHeartbeat` 成功 → `Delete(KeyAgInstAgent(agentID))`。
- [ ] **Step 3:** Commit。

---

### Task 7: TableCacheRefresher + Kratos 生命周期

**Files:** `table_cache_refresher.go`、`provider.go`、`cmd/server/app.go`、`wire.go`/`wire_gen.go`

- [ ] **Step 1:** `refreshOnce`：与设计 §4 一致；失败 **保留** 旧缓存。
- [ ] **Step 2:** `enabled=false` 时 **不**起 ticker（与现 `Start` 逻辑一致）；`interval<=0` 时行为与注释一致。
- [ ] **Step 3:** `BeforeStart` 调用 `refresher.Start()`，`BeforeStop` 调用 `Stop()`。
- [ ] **Step 4:** 本地配置短间隔，观察日志。
- [ ] **Step 5:** Commit。

---

### Task 8: 全量验收

- [ ] **Step 1:** `go test ./...`（或至少 `internal/data`、`internal/biz`、`internal/service`）。
- [ ] **Step 2:** `go build ./cmd/server/...`。
- [ ] **Step 3:**（可选）在示例配置中增加 `table_cache` 片段。
- [ ] **Step 4:** Commit。

---

## Plan Review（建议）

1. 将本 plan 与 spec 对照：写路径是否覆盖 **Admin HTTP** 与 **Agent 心跳** 等所有落库入口的 **cache 失效**。
2. 确认 **单实例** 部署假设仍成立后再上生产。

## Execution Handoff

- **执行方式：** subagent-driven-development（按 Task 派生子代理）或 executing-plans（本会话顺序执行）。
- **Spec 变更时：** 先改 `docs/superpowers/specs/2026-03-30-four-tables-inproc-cache-design.md`，再同步本 plan 的 Task 7/键空间表。
