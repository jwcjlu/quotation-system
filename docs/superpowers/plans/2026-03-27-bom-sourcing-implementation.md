# BOM 货源搜索与配单 — 正式规格落地 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: 使用 `superpowers:subagent-driven-development`（推荐，仓库备忘：[subagent-driven-development.md](../subagent-driven-development.md)）或 `superpowers:executing-plans` **按 Task 顺序**实现；步骤使用 `- [ ]` 勾选跟踪。
>
> **建议：** 在独立 git worktree 中实施（仓库备忘：[using-git-worktrees.md](../using-git-worktrees.md)）。

**Goal:** 按 [docs/superpowers/specs/2026-03-27-bom-sourcing-design.md](../specs/2026-03-27-bom-sourcing-design.md) 实现 **失败/跳过平台策略**、**`bom_search_task` 状态机**、**Excel 列映射导入**，以及 **「数据已准备」就绪判定** 与 BOM 变更时的 **任务增量同步**；与 [docs/schema/bom_mysql.sql](../../schema/bom_mysql.sql) 及 Agent 调度（[docs/schema/agent_dispatch_task_mysql.sql](../../schema/agent_dispatch_task_mysql.sql)）衔接。

**Architecture:** **会话 + 行 + (MPN_norm×platform×biz_date) 任务** 为搜索真相源；报价入 `bom_quote_cache`；任务状态在 `bom_search_task.state` 按设计文档转移；就绪判定由 **事务内聚合查询** 或 **异步 reconciler** 更新 `bom_session.status`。Excel 导入在 **单包**（如 `internal/biz/bom_import.go`）完成表头归一化与校验，再写 `bom_session` / `bom_session_line`。

**Tech Stack:** Go、Kratos、MySQL 8+、Wire、（可选）`excelize` 或现有表格解析库；与现有 Agent `TaskScheduler` 对接派发 `bom_search_task` 对应的 `caichip_dispatch_task`。

**Spec / 设计输入:** [需求要点](../specs/2026-03-27-bom-sourcing-requirements.md) · [设计规格](../specs/2026-03-27-bom-sourcing-design.md) · [specs 索引](../specs/README.md)

---

## 文件结构（创建 / 修改一览）

| 路径 | 职责 |
|------|------|
| **Modify** [docs/schema/bom_mysql.sql](../../schema/bom_mysql.sql) 或 **Create** `docs/schema/migrations/YYYYMMDD_bom_readiness.sql` | 为 `bom_session` 增加 `readiness_mode`（`lenient`/`strict`）及 `status` 扩展枚举备注；若状态机新增 `running`/`failed_retryable` 等与现有 `VARCHAR(32)` 冲突则仅文档约定 |
| **Create** `internal/biz/bom_search_task_fsm.go`（或并入 `bom_search.go`） | 状态转移函数：`TransitionTask(from, event) (to, error)`；非法转移返回哨兵错误 |
| **Create** `internal/biz/bom_readiness.go` | `ComputeSessionReadiness(sessionID)`：`lenient`/`strict` 规则与设计文档 §3.4 一致 |
| **Create** `internal/biz/bom_import_excel.go` | 表头别名映射、行校验、`ParseBomImportRows(r io.Reader) ([]Line, []ImportError)` |
| **Create** `internal/biz/bom_import_excel_test.go` | 表头别名、空 MPN、非法数量、partial 模式 |
| **Create/Modify** `internal/data/bom_search_task_repo.go` | 按状态批量更新、`ListBySessionState`、取消/跳过 API |
| **Create/Modify** `internal/service/bom_*.go` | 导入 HTTP/gRPC、触发任务重建、手动跳过平台 |
| **Modify** `cmd/server/wire.go` | 注入新 repo / service |
| **Create** `docs/superpowers/specs/2026-03-27-bom-sourcing-api-stub.md`（可选） | 仅当需要对外固定路径时再补 OpenAPI 片段 |

---

### Task 1: 规格对齐与 DDL 备注

**Files:**
- `docs/schema/bom_mysql.sql`
- `docs/superpowers/specs/2026-03-27-bom-sourcing-design.md`

- [ ] **Step 1:** 核对 `bom_search_task.state` 默认值 `pending` 是否覆盖设计文档全部枚举；在 `bom_mysql.sql` 顶部或 `COMMENT` 中列出允许值。
- [ ] **Step 2:** 为 `bom_session` 增加 `readiness_mode` 迁移脚本（若表已在生产，用 `ALTER` 迁移文件而非仅改草案）。
- [ ] **Step 3:** Commit  

```bash
git add docs/schema/bom_mysql.sql docs/schema/migrations/
git commit -m "docs(schema): bom readiness_mode and search task states"
```

---

### Task 2: 搜索任务状态机（TDD）

**Files:**
- `internal/biz/bom_search_task_fsm.go`
- `internal/biz/bom_search_task_fsm_test.go`

- [ ] **Step 1: 写失败测试** — `pending` + `claim_dispatch` → `running`；`running` + `error_retryable` → `failed_retryable`；`failed_retryable` + `attempts_exhausted` → `failed_terminal`。

```go
func TestFSM_RetryExhaustedToTerminal(t *testing.T) {
    to, err := BomSearchTaskTransition("failed_retryable", "attempts_exhausted")
    if err != nil || to != "failed_terminal" {
        t.Fatalf("got %q %v", to, err)
    }
}
```

- [ ] **Step 2:** `go test ./internal/biz/... -run BomSearchTask -v`，预期 FAIL（未实现）。
- [ ] **Step 3:** 实现完整转移表（与设计文档 Mermaid 一致），非法组合返回 `ErrInvalidTaskTransition`。
- [ ] **Step 4:** 测试全绿。
- [ ] **Step 5:** Commit  

```bash
git add internal/biz/bom_search_task_fsm.go internal/biz/bom_search_task_fsm_test.go
git commit -m "feat(biz): bom search task FSM transitions"
```

---

### Task 3: 会话就绪判定（TDD）

**Files:**
- `internal/biz/bom_readiness.go`
- `internal/biz/bom_readiness_test.go`

- [ ] **Step 1: 写测试** — 给定内存中任务行列表：宽松模式下全部终态 → `Ready=true`；存在 `pending` → `Ready=false`；严格模式下某行无 `succeeded` → `Ready=false`。

- [ ] **Step 2:** 实现纯函数 `ReadinessFromTasks(mode, tasks []TaskSnapshot, platformIDs []string, lineIDs []int64) bool`（便于单测，不连 DB）。
- [ ] **Step 3:** `go test ./internal/biz/... -run Readiness -v`。
- [ ] **Step 4:** Commit  

```bash
git commit -m "feat(biz): bom session readiness lenient vs strict"
```

---

### Task 4: Excel 列映射与校验（TDD）

**Files:**
- `internal/biz/bom_import_excel.go`
- `internal/biz/bom_import_excel_test.go`
- `go.mod`（若新增 `github.com/xuri/excelize/v2`）

- [ ] **Step 1:** 按设计文档 §6 建立 `var headerAliases = map[string][]string{...}`。
- [ ] **Step 2: 失败测试** — 表头 `型号`+`数量` 行 5 空型号 → `errors` 含 `row=5, field=mpn`。
- [ ] **Step 3:** 实现首行表头解析、行遍历、默认 `qty=1`、可选 `partial` 行为。
- [ ] **Step 4:** `go test ./internal/biz/... -run BomImport -v`。
- [ ] **Step 5:** Commit  

```bash
git commit -m "feat(biz): BOM excel import column mapping and validation"
```

---

### Task 5: 数据层 — 任务查询与批量状态更新

**Files:**
- `internal/data/bom_search_task_repo.go`
- `internal/data/bom_search_task_repo_test.go`（或 `_integration_test.go` + `TEST_MYSQL_DSN`）

- [ ] **Step 1:** `ListActiveBySession(ctx, sessionID)`：`state IN ('pending','running','failed_retryable')`。
- [ ] **Step 2:** `CancelBySessionPlatform(ctx, sessionID, platformID)`、`MarkSkipped(...)` 与设计文档 §5 一致。
- [ ] **Step 3:** 集成测试或 sqlmock：更新行数与 WHERE 条件正确。
- [ ] **Step 4:** Commit  

```bash
git commit -m "feat(data): bom search task list cancel skip"
```

---

### Task 6: 应用服务 — 导入与会话状态推进

**Files:**
- `internal/service/bom_service.go`（或等价新建）
- `internal/server/http.go` / `grpc.go`（注册路由）

- [ ] **Step 1:** `ImportBomExcel`：调 `Parse` → 写 `bom_session_line` → 调「任务增量」为每行×平台创建 `pending`。
- [ ] **Step 2:** Agent 回写结果时调用 FSM + 写 `bom_quote_cache`；成功后 `TryMarkSessionDataReady(sessionID)` 内调 `ReadinessFromTasks` + `UPDATE bom_session`。
- [ ] **Step 3:** `go build ./...`。
- [ ] **Step 4:** Commit  

```bash
git commit -m "feat(service): BOM excel import and data_ready transition"
```

---

### Task 7: 验证与 handoff

**Files:**
- （无新文件，运行全仓测试）

- [ ] **Step 1:** `go test ./...`（或项目约定 `make test`）。
- [ ] **Step 2:** 在 spec `§9 修订记录` 追加一行「实现完成」链到本 plan（可选）。
- [ ] **Step 3:** 最终 commit（若有文档小改）。

---

## Plan review

- 编写完成后可使用 **plan-document-reviewer** 子代理对照 [2026-03-27-bom-sourcing-design.md](../specs/2026-03-27-bom-sourcing-design.md) 审阅本计划；有 ❌ 则修订至 ✅。

---

**计划已保存至 `docs/superpowers/plans/2026-03-27-bom-sourcing-implementation.md`。执行方式可选：**

1. **Subagent-Driven（推荐）** — 每 Task 新开子代理、Task 间 review；技能：`subagent-driven-development`  
2. **Inline Execution** — 本会话按 Task 批量执行并设检查点；技能：`executing-plans`

如需我代为执行其中某一 Task，请直接指定 Task 编号。
