# BOM LLM 异步导入与进度可视化 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 `UploadBOM(parse_mode=llm)` 从同步阻塞改为异步可恢复导入，并提供可轮询的导入进度状态，解决大文件超时与无反馈问题，同时保持 LLM 解析精度。

**Architecture:** 入口 `UploadBOM` 在 llm 模式下仅做快速校验并“受理导入任务”，后台执行“表头识别 + 分块解析 + 落库 + 任务生成”，持续更新 session 导入状态；业务读取接口在 `parsing` 期间统一门禁。导入状态使用结构化字段持久化，避免重启后状态丢失。

**Tech Stack:** Go, Kratos, GORM, MySQL, protobuf (`make api`), Wire, existing BOM session/search repos.

**Spec:** [docs/superpowers/specs/2026-04-21-bom-llm-async-import-design.md](../specs/2026-04-21-bom-llm-async-import-design.md)

---

## 文件结构（先定边界）

| 路径 | 作用 |
|------|------|
| **Modify** `api/bom/v1/bom.proto` | 扩展 `UploadBOMReply`（accepted/status/message）与 `GetSessionReply` 导入进度字段 |
| **Run** `make api` | 生成 `api/bom/v1/*.pb.go` |
| **Modify** `internal/data/models.go` | `BomSession` 增加导入状态结构化字段（status/progress/stage/message/error/error_code/updated_at） |
| **Modify** `internal/data/bom_session_repo.go` | 增加导入状态更新方法（GORM） |
| **Modify** `internal/biz/repo.go` | 增加 session 导入状态更新接口（供 service 调用） |
| **Create** `internal/biz/bom_import_progress.go` | 导入状态常量、进度计算辅助 |
| **Modify** `internal/service/bom_service.go` | `UploadBOM` llm 路径异步化、状态推进、错误收敛 |
| **Create** `internal/service/bom_import_async.go` | 异步任务执行器（分阶段/分块/重试） |
| **Modify** `internal/service/bom_match_parallel.go` / `bom_service.go` | `parsing` 门禁（统一返回 `BOM_NOT_READY`） |
| **Modify** `cmd/server/wire.go` / `wire_gen.go` | 注入新增依赖 |
| **Modify/Create** `internal/service/bom_service_test.go`、`internal/biz/*_test.go`、`internal/data/*_test.go` | 覆盖异步与门禁行为 |

---

### Task 1: Proto 与 API 契约扩展

**Files:**
- Modify: `api/bom/v1/bom.proto`
- Generate: `api/bom/v1/bom.pb.go`, `api/bom/v1/bom_grpc.pb.go`, `api/bom/v1/bom_http.pb.go`

- [ ] **Step 1: 定义返回契约最小扩展**
  - `UploadBOMReply` 新增：
    - `bool accepted`
    - `string import_status`
    - `string import_message`
  - `GetSessionReply`（或其内嵌 session 结构）新增：
    - `string import_status`
    - `int32 import_progress`
    - `string import_stage`
    - `string import_message`
    - `string import_error_code`
    - `string import_error`
    - `string import_updated_at`（或 int64 epoch）

- [ ] **Step 2: 生成代码**

Run: `make api`  
Expected: `api/bom/v1/*.pb.go` 更新且无报错。

- [ ] **Step 3: Commit**

```bash
git add api/bom/v1/bom.proto api/bom/v1/bom.pb.go api/bom/v1/bom_grpc.pb.go api/bom/v1/bom_http.pb.go
git commit -m "feat(api): add async bom import progress fields"
```

---

### Task 2: Session 导入状态持久化（data + biz 接口）

**Files:**
- Modify: `internal/data/models.go`
- Modify: `internal/data/bom_session_repo.go`
- Modify: `internal/biz/repo.go`
- Create: `internal/biz/bom_import_progress.go`
- Test: `internal/data/bom_session_repo_test.go`

- [ ] **Step 1: 扩展模型字段（结构化）**
  - 在 `BomSession` 增加导入状态列：
    - `import_status`、`import_progress`、`import_stage`
    - `import_message`、`import_error_code`、`import_error`
    - `import_updated_at`

- [ ] **Step 2: 定义 biz 层状态常量**
  - `idle/parsing/ready/failed`
  - `validating/header_infer/chunk_parsing/persisting/done/failed`
  - 辅助函数：`ClampProgress`, `BuildImportMessage`

- [ ] **Step 3: repo 增加状态更新方法**
  - 例如 `UpdateImportStatus(ctx, sessionID, patch)`，内部 GORM 原子更新。
  - 保证并发更新安全，`import_updated_at` 每次刷新。

- [ ] **Step 4: 运行测试**

Run: `go test ./internal/data/... -run BomSession -count=1 -v`  
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/data/models.go internal/data/bom_session_repo.go internal/biz/repo.go internal/biz/bom_import_progress.go internal/data/bom_session_repo_test.go
git commit -m "feat(data): persist bom import progress on session"
```

---

### Task 3: `UploadBOM(llm)` 改为受理即返回

**Files:**
- Modify: `internal/service/bom_service.go`
- Create: `internal/service/bom_import_async.go`
- Test: `internal/service/bom_service_test.go`

- [ ] **Step 1: 重构 `UploadBOM` llm 分支**
  - 快速校验：`session_id`、`dbOK`、`openai != nil`、文件非空/基础限制。
  - 在进入后台任务前写入：
    - `status=parsing`, `progress=5`, `stage=validating`, `message=导入任务已启动`
  - 启动后台执行器（goroutine + 可替换 runner）。
  - 立即返回：
    - `bom_id=session_id`
    - `accepted=true`
    - `import_status=parsing`
    - `import_message=...`

- [ ] **Step 2: 抽出异步执行器骨架**
  - `runLLMImportJob(ctx, sessionID, fileBytes, parseModeRaw, columnMapping)`
  - 阶段推进方法统一封装，避免散乱写 DB。

- [ ] **Step 3: 单测**
  - llm 模式立即返回，不阻塞等待 chat。
  - 返回值 `accepted=true`。
  - 状态从 `idle -> parsing` 被写入。

- [ ] **Step 4: Commit**

```bash
git add internal/service/bom_service.go internal/service/bom_import_async.go internal/service/bom_service_test.go
git commit -m "feat(service): make bom llm import asynchronous"
```

---

### Task 4: 分阶段 + 分块解析（精度优先）

**Files:**
- Modify: `internal/service/bom_import_async.go`
- Modify: `internal/biz/bom_import_llm.go`（必要时新增分块 prompt 辅助）
- Test: `internal/service/bom_import_async_test.go`

- [ ] **Step 1: 实现阶段 A（header infer）**
  - 输入：header + 样本行。
  - 输出：列语义映射（model/mfr/package/qty/params/raw）。
  - 更新状态：`header_infer`, progress `15->20`。

- [ ] **Step 2: 实现阶段 B（chunk parsing）**
  - 按 200~400 行分块（配置化默认 300）。
  - 每块：
    - 调 LLM
    - 解析 JSON
    - 累积 `BomImportLine`
    - 更新进度与 message（第 i/n 块）
  - 块级失败重试 1~2 次（指数退避）。

- [ ] **Step 3: 落库与任务生成**
  - 成功后执行现有链路：
    - `ReplaceSessionLines`
    - `CancelAllTasksBySession`
    - `UpsertPendingTasks`
    - `tryMergeDispatchSession`
  - 收尾状态：`ready/done/100`。

- [ ] **Step 4: 失败状态收敛**
  - 任意阶段失败：
    - `status=failed`, `stage=failed`
    - `error_code`（timeout/llm_429/invalid_json/chunk_failed/...）
    - `error` 文案

- [ ] **Step 5: 测试**

Run: `go test ./internal/service/... -run BomImportAsync -count=1 -v`  
Expected: PASS（成功路径、块失败重试、最终失败路径）。

- [ ] **Step 6: Commit**

```bash
git add internal/service/bom_import_async.go internal/biz/bom_import_llm.go internal/service/bom_import_async_test.go
git commit -m "feat(service): add staged chunked llm import pipeline"
```

---

### Task 5: 读取门禁（parsing 期间统一未就绪）

**Files:**
- Modify: `internal/service/bom_service.go`
- Modify: `internal/service/bom_match_parallel.go`（如需要）
- Test: `internal/service/bom_service_test.go`

- [ ] **Step 1: 增加 session 导入状态门禁**
  - `SearchQuotes` / `AutoMatch` / `GetMatchResult` 在 session `import_status=parsing` 时返回：
    - `ServiceUnavailable("BOM_NOT_READY", "...")`（沿用现有语义）

- [ ] **Step 2: 单测**
  - parsing 状态下上述接口均被拒绝。
  - ready 状态下恢复正常。

- [ ] **Step 3: Commit**

```bash
git add internal/service/bom_service.go internal/service/bom_match_parallel.go internal/service/bom_service_test.go
git commit -m "fix(service): gate match APIs when bom import is parsing"
```

---

### Task 6: 并发幂等策略（同 session 重复上传）

**Files:**
- Modify: `internal/service/bom_service.go`
- Modify: `internal/data/bom_session_repo.go`
- Test: `internal/service/bom_service_test.go`

- [ ] **Step 1: 固化策略**
  - 同一 `session_id` 若 `import_status=parsing` 再上传 llm：
    - 返回冲突语义（建议 `AlreadyExists`/`FailedPrecondition` 对外映射 409/400）
    - message: “当前导入仍在进行中”

- [ ] **Step 2: 单测并发场景**
  - 首次 accepted，二次命中冲突。

- [ ] **Step 3: Commit**

```bash
git add internal/service/bom_service.go internal/data/bom_session_repo.go internal/service/bom_service_test.go
git commit -m "feat(service): enforce single active llm import per session"
```

---

### Task 7: Wire 与构建验证

**Files:**
- Modify: `cmd/server/wire.go`
- Generate: `cmd/server/wire_gen.go`

- [ ] **Step 1: 更新依赖注入**
  - 新增 repo/interface 注入与绑定，确保 service 可调用状态更新接口。

- [ ] **Step 2: 生成 wire**

Run: `cd cmd/server && wire`  
Expected: `wire_gen.go` 更新成功。

- [ ] **Step 3: 编译**

Run: `go build -o bin/server ./cmd/server/...`  
Expected: PASS。

- [ ] **Step 4: Commit**

```bash
git add cmd/server/wire.go cmd/server/wire_gen.go
git commit -m "chore(wire): inject async bom import dependencies"
```

---

### Task 8: 全量测试与回归检查

**Files:**
- Modify/Test as needed across `internal/service`, `internal/data`, `internal/biz`

- [ ] **Step 1: 运行关键测试集**

Run: `go test ./internal/biz/... ./internal/data/... ./internal/service/... -count=1`  
Expected: PASS。

- [ ] **Step 2: 端到端手工验证**
  - 上传 llm 大文件后接口立即返回 `accepted=true`。
  - `GetSession` 可见进度从 parsing 递增到 ready。
  - ready 后 `SearchQuotes/AutoMatch/GetMatchResult` 正常。
  - 注入故障时状态进入 failed 且错误可见。

- [ ] **Step 3: 可观测性检查**
  - 日志包含 `session_id`, `stage`, `chunk_index/chunk_total`, `elapsed_ms`, `error_code`。

- [ ] **Step 4: Commit**

```bash
git add .
git commit -m "test: verify async llm bom import flow end-to-end"
```

---

## 计划执行注意事项

- 保持 `server -> service -> biz <- data` 分层；业务编排不下沉到 data。  
- 数据库读写全部使用 GORM。  
- `internal/service/bom_service.go` 已较长，新增逻辑优先拆到 `internal/service/bom_import_async.go`。  
- 非 llm 模式行为保持不变，避免引入回归。  
- 先保证“异步 + 进度 + 门禁”闭环，再做高级优化（断点续跑/恢复 worker）。

---

## 执行交接

Plan complete and saved to `docs/superpowers/plans/2026-04-21-bom-llm-async-import-implementation.md`. Two execution options:

1. **Subagent-Driven (recommended)** - 每个 Task 派发独立子代理执行并逐步复核。  
2. **Inline Execution** - 在当前会话按 Task 顺序直接实现。  

你想选 **1** 还是 **2**？  
