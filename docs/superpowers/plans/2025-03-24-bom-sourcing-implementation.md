# BOM 货源搜索与配单 — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在现有 caichip（Kratos + `BomService` + Agent 调度）上落地 [BOM货源搜索-技术设计方案.md](../../BOM货源搜索-技术设计方案.md) 与 [BOM货源搜索-接口清单.md](../../BOM货源搜索-接口清单.md)：持久化会话/任务/缓存、轮询就绪、手动重试计数、配单快照与导出，并与现有 `api/agent` 任务链路对接。

**Architecture:** 扩展 `api/bom/v1` 与 `internal/service.BomService`（或拆 `BomSessionService` 若单文件过大）；数据层用 `docs/schema/bom_postgres.sql`（或 MySQL）替换/并存当前内存 `internal/data/bom_repo.go`；搜索任务写入 `bom_search_task` 并由 `internal/biz/agent_hub` 或等价调度对接 `script_id`；进程内事件用 `sync.Map` + channel 或小型 `internal/bomapp/event` 包；前端仅轮询，不提供 WS。

**Tech Stack:** Go 1.22+、Kratos v2、Protobuf/gRPC-Gateway 风格 HTTP、PostgreSQL（主）或 MySQL、Wire、现有 `pkg/parser`、现有 Agent HTTP。

---

## 文件结构映射（落地前锁定）

| 区域 | 路径 | 职责 |
|------|------|------|
| API | `api/bom/v1/bom.proto`（或新建 `session.proto` 再 import） | RPC + `google.api.http` 路径，与接口清单对齐 |
| 生成代码 | `api/bom/v1/*.pb.go`, `*_http.pb.go` | `make api` |
| 业务 | `internal/biz/bom.go`, 新建 `internal/biz/bom_session.go`, `internal/biz/quote_cache.go`, `internal/biz/search_task.go` | 会话、就绪判定、缓存、任务幂等 |
| 数据 | `internal/data/bom_repo.go` → `bom_sql_repo.go` + `internal/data/db.go` | 事务、查询 |
| 服务 | `internal/service/bom_service.go` | 实现新 RPC |
| Agent | `internal/biz/agent_hub.go`, `internal/service/agent.go` | 入队、`caichip_task_id` 回写 |
| 配置 | `internal/conf/conf.go`, `configs/config.yaml` | DSN、重试上限 |
| 依赖注入 | `cmd/server/wire.go` | 注入 DB 与 use case |
| 文档/DDL | `docs/schema/bom_postgres.sql` | 已存在，按需微调 |

---

### Task 1: 数据库 DDL 与迁移脚本可重复执行

**Files:**
- Modify: `docs/schema/bom_postgres.sql`（若 Task 实现中发现缺列）
- Create: `docs/schema/bom_migrate_notes.md`（可选：如何执行 psql）

- [ ] **Step 1:** 在本地 PostgreSQL 执行 `docs/schema/bom_postgres.sql`。

```bash
psql "$DATABASE_URL" -f docs/schema/bom_postgres.sql
```

- [ ] **Step 2:** 验证表存在。

```bash
psql "$DATABASE_URL" -c "\dt bom_*"
```

**Expected:** 列出 `bom_session`, `bom_session_line`, `bom_quote_cache`, `bom_search_task`, `bom_match_result`, `bom_platform_script`。

- [ ] **Step 3:** Commit

```bash
git add docs/schema/
git commit -m "chore(db): apply BOM sourcing DDL"
```

---

### Task 2: 添加 `internal/data` 数据库连接与 BOM SQL 仓储骨架

**Files:**
- Create: `internal/data/db.go`（`*sql.DB` 或 `pgxpool` 封装，与项目风格一致）
- Create: `internal/data/bom_sql_repo.go`（实现 `biz.BOMRepo` 新方法或新接口 `BOMSessionRepo`）
- Modify: `internal/conf/conf.go` — 增加 `Data.Database.Dsn`（若尚无）
- Modify: `configs/config.yaml` — 示例 DSN
- Modify: `go.mod` — `database/sql` + `_ "github.com/jackc/pgx/v5/stdlib"` 或 `github.com/go-sql-driver/mysql`

- [ ] **Step 1:** 编写**失败**的集成测试：连接 DSN 后 `SELECT 1`（仅当 `TEST_DATABASE_URL` 设置时运行）。

**Test file:** `internal/data/db_test.go`

```go
func TestDatabasePing(t *testing.T) {
    dsn := os.Getenv("TEST_DATABASE_URL")
    if dsn == "" {
        t.Skip("TEST_DATABASE_URL not set")
    }
    // Open DB and Ping
}
```

- [ ] **Step 2:** Run（应 FAIL 直至实现连接）

```bash
set TEST_DATABASE_URL=postgres://...
go test ./internal/data/... -run TestDatabasePing -v -count=1
```

**Expected (before impl):** build fail or ping fail  
**Expected (after impl):** PASS

- [ ] **Step 3:** 实现 `OpenDB` + Ping，再跑测试 PASS。

- [ ] **Step 4:** Commit

```bash
git add internal/data/db.go internal/conf/ configs/ go.mod go.sum internal/data/db_test.go
git commit -m "feat(data): database bootstrap and ping test"
```

---

### Task 3: 扩展 Protobuf — 会话、平台、readiness、retry（最小增量）

**Files:**
- Modify: `api/bom/v1/bom.proto` — 新增 RPC：`CreateSession`, `GetSession`, `PutPlatforms`, `GetReadiness`, `GetBOMLines`, `RetrySearchTasks`（名称与现有风格一致即可）
- Run: `make api`

- [ ] **Step 1:** 编辑 `bom.proto`，为每个 RPC 添加 `google.api.http` 路径，与 [接口清单](../../BOM货源搜索-接口清单.md) 对齐，例如：
  - `POST /api/v1/bom-sessions`
  - `GET /api/v1/bom-sessions/{session_id}/readiness`
  - `PUT /api/v1/bom-sessions/{session_id}/platforms`

- [ ] **Step 2:** 生成代码

```bash
cd d:\workspace\caichip
make api
```

**Expected:** `api/bom/v1/` 下 `.pb.go` / `_http.pb.go` 更新且无编译错误。

- [ ] **Step 3:** 全量编译

```bash
go build -o bin/server ./cmd/server/...
```

**Expected:** 可能因未实现 Service 方法而失败 — **接受**；下一 Task 实现 stub。

- [ ] **Step 4:** Commit proto + 生成代码

```bash
git add api/bom/v1/
git commit -m "feat(api): BOM session and readiness RPCs"
```

---

### Task 4: `BomService` 实现新 RPC 的编译桩（返回 Unimplemented 或固定假数据）

**Files:**
- Modify: `internal/service/bom_service.go`

- [ ] **Step 1:** 为每个新 RPC 添加方法体：`return nil, errors.NotImplemented(...)` **或** 最小假 JSON，保证 `go build` 通过。

- [ ] **Step 2:** 验证

```bash
go build -o bin/server ./cmd/server/...
```

**Expected:** SUCCESS

- [ ] **Step 3:** Commit

```bash
git add internal/service/bom_service.go
git commit -m "feat(bom): stub new BomService RPCs"
```

---

### Task 5: 会话持久化 — CreateSession + GetSession

**Files:**
- Modify: `internal/biz/` — `BOMSession` 模型与 `BOMSessionRepo` 接口
- Modify: `internal/data/bom_sql_repo.go` — `INSERT/SELECT bom_session`
- Modify: `internal/service/bom_service.go` — 真实实现 `CreateSession`/`GetSession`
- Test: `internal/service/bom_service_test.go` 或 biz 层单测（可用 sqlite 内存 **仅当**团队接受；否则集成测 + TEST_DATABASE_URL）

- [ ] **Step 1:** 写测试：创建会话后 `GetSession` 能读到 `biz_date`、`selection_revision`。

- [ ] **Step 2:** Run test

```bash
set TEST_DATABASE_URL=...
go test ./internal/... -run TestCreateSession -v -count=1
```

**Expected:** PASS

- [ ] **Step 3:** Commit

```bash
git commit -m "feat(bom): persist BOM session"
```

---

### Task 6: 上传/解析与 `bom_session_line` 关联

**Files:**
- Modify: `internal/biz/bom.go` 或新 use case — 将现有 `ParseAndSave` 改为写入 **session_id** 下的行表
- Modify: `internal/service/bom_service.go` — `UploadBOM` 请求带 `session_id`（proto 调整）或先 `CreateSession` 再上传

- [ ] **Step 1:** 更新 `bom.proto` 中 `UploadBOMRequest` 增加 `session_id`（若采用两阶段创建）。

- [ ] **Step 2:** `make api` + 实现解析后 `INSERT bom_session_line` 批量。

- [ ] **Step 3:** 验证

```bash
go test ./internal/biz/... ./internal/service/... -count=1
curl -s -X POST "http://127.0.0.1:8000/api/v1/bom/upload" ... 
```

（以实际生成的 HTTP 路径为准；可用 `grpcurl` 若仅 gRPC）

**Expected:** DB 中 `bom_session_line` 行数 = 解析行数。

- [ ] **Step 4:** Commit

---

### Task 7: `PutPlatforms` + revision 乐观锁 + 差量 `bom_search_task`

**Files:**
- Modify: `internal/biz/search_task.go`
- Modify: `internal/service/bom_service.go`

- [ ] **Step 1:** 单元测试：`expected_revision` 不匹配 → 409。

- [ ] **Step 2:** 实现：revision+1；对新平台×会话内全部 `mpn_norm` 插入 `pending` 任务（若缓存无终态）。

- [ ] **Step 3:** Run

```bash
go test ./internal/biz/... -run TestPutPlatforms -v -count=1
```

- [ ] **Step 4:** Commit

---

### Task 8: `bom_quote_cache` 读写与「就绪」判定函数

**Files:**
- Create: `internal/biz/readiness.go` — `CanEnterMatch(sessionID) (bool, blockReason)`
- Test: `internal/biz/readiness_test.go`

- [ ] **Step 1:** 表驱动测试：全平台 `has_quotes` / `no_mpn_match` → true；缺一则 false。

- [ ] **Step 2:**

```bash
go test ./internal/biz/... -run TestCanEnterMatch -v -count=1
```

**Expected:** PASS

- [ ] **Step 3:** Commit

---

### Task 9: `GetReadiness` + `GetBOMLines`（含 `platform_gaps`）

**Files:**
- Modify: `internal/service/bom_service.go`

- [ ] **Step 1:** 实现聚合查询：对每个 `mpn`×`platform` 合并任务状态 + 缓存 outcome，生成 `reason_code`（见接口清单）。

- [ ] **Step 2:** 手动验证：启动 server，创建会话→上传→设平台→GET readiness。

```bash
go run ./cmd/server -conf configs/config.yaml
curl -s "http://127.0.0.1:8000/api/v1/bom-sessions/{id}/readiness"
```

**Expected:** JSON 含 `can_enter_match`、`phase`。

- [ ] **Step 3:** Commit

---

### Task 10: Agent 入队 — `bom_search_task` → `agent_hub` 派发

**Files:**
- Modify: `internal/biz/agent_hub.go` 或新建 `internal/biz/bom_task_enqueue.go`
- Modify: `internal/service/agent.go` — 任务结果回调路径上增加「若 task 带 `bom_search_task_id` 则更新缓存」

- [ ] **Step 1:** 定义任务 payload 扩展字段（与 `api/agent/v1` 对齐）：`bom_search_task_id`, `mpn_norm`, `platform_id`。

- [ ] **Step 2:** 写测试：mock Agent 返回成功结果 → `bom_quote_cache` 有记录，`bom_search_task.state=succeeded_quotes`。

- [ ] **Step 3:** Commit

---

### Task 11: 进程内事件 — 新任务入队触发调度尝试

**Files:**
- Create: `internal/bomapp/event.go`（包名可调）— `Publish(TaskEnqueued{})`
- Modify: 调度循环订阅事件

- [ ] **Step 1:** 单元测试：发布事件后调度函数被调用 1 次（mock）。

- [ ] **Step 2:** Commit

---

### Task 12: `RetrySearchTasks` — `manual_attempt` 递增并入队

**Files:**
- Modify: `internal/biz/search_task.go`
- Modify: `internal/service/bom_service.go`

- [ ] **Step 1:** 测试：`manual_attempt` 与 `auto_attempt` 独立递增。

- [ ] **Step 2:**

```bash
go test ./internal/biz/... -run TestManualRetry -v -count=1
```

- [ ] **Step 3:** Commit

---

### Task 13: `AutoMatch` 持久化 `bom_match_result` + `GetMatchResult`

**Files:**
- Modify: `internal/biz/match.go`（已有则扩展）
- Modify: `internal/data/bom_sql_repo.go`

- [ ] **Step 1:** 测试：match 后 `bom_match_result.version` 递增。

- [ ] **Step 2:** Commit

---

### Task 14: 导出与历史列表（最小）

**Files:**
- Modify: `bom.proto` — `ExportMatchResult`, `ListMatchHistory`
- Modify: `internal/service/bom_service.go`

- [ ] **Step 1:** 导出 CSV 字节流或返回下载 URL（YAGNI：先 CSV 字节）。

- [ ] **Step 2:** `curl` 下载验证非空。

- [ ] **Step 3:** Commit

---

### Task 15: Wire 注入与配置收束

**Files:**
- Modify: `cmd/server/wire.go`
- Modify: `cmd/server/main.go`

- [ ] **Step 1:**

```bash
cd cmd/server && wire
```

- [ ] **Step 2:**

```bash
go build -o bin/server ./cmd/server/...
go test ./... -count=1
```

**Expected:** 全绿（跳过需 DSN 的测试时设 `-short` 若已支持）。

- [ ] **Step 3:** Commit

---

### Task 16: 文档与接口清单路径对齐

**Files:**
- Modify: `docs/BOM货源搜索-接口清单.md` — 将示例路径与 `bom.proto` **实际** `google.api.http` 行对齐（若与 `/api/v1/bom-sessions` 有差异则更新文档）

- [ ] **Step 1:** 全文搜索 `bom-sessions`，与生成 HTTP 路由一致。

- [ ] **Step 2:** Commit

---

## Plan Review（自助）

对照 [BOM单货源搜索与询价需求文档.md](../../BOM单货源搜索与询价需求文档.md) §1.3、§8 与 [技术设计方案](../../BOM货源搜索-技术设计方案.md) §4–§7，确认：

- [ ] 失败不可记为 `no_mpn_match`
- [ ] 手动/自动重试分计
- [ ] 同型号多行合并任务

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2025-03-24-bom-sourcing-implementation.md`. Two execution options:

**1. Subagent-Driven (recommended)** — Dispatch a fresh subagent per task, review between tasks, fast iteration. **REQUIRED SUB-SKILL:** superpowers:subagent-driven-development.

**2. Inline Execution** — Execute tasks in this session using superpowers:executing-plans, batch execution with checkpoints.

**Which approach?**
