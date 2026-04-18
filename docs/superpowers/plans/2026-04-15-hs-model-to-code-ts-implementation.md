# HS Model To CodeTS Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现“按电子元器件型号自动解析并返回 HS `code_ts`”全链路能力，支持映射直查、datasheet 下载、LLM 抽取、候选推荐、Top1 自动回写与 Top3 审计。

**Architecture:** 采用 Kratos 分层：`service -> biz <- data`。`service` 负责 API 与任务接口，`biz` 负责状态机、幂等、阈值与确认优先级，`data` 负责持久化、datasheet 资产下载与 LLM/外部调用适配。推荐链路使用“规则候选预筛 + LLM 精排”，并以 `run_id` 贯穿审计与人工确认。

**Tech Stack:** Go, Kratos, GORM, MySQL, LLM API client, HTTP file download, proto/service, unit tests.

**Spec / 输入文档:** `docs/superpowers/specs/2026-04-15-hs-model-to-code-ts-design.md`

---

## 当前状态（2026-04-16 回填）

- [x] 主链路代码与测试已落地，`data` / `biz` / `service` 测试通过，`cmd/server` 可构建。
- [x] 行为验收已验证：同步超时转异步 `202 + task_id`、同 `request_trace_id` 复用 `run_id`、旧 `run_id` 不能覆盖新结果。
- [x] 可观测性任务已补齐：新增 `internal/biz/hs_model_observability_test.go`，并验证关键指标与结构化日志字段输出。

---

## 文件结构（创建 / 修改一览）

| 路径 | 职责 |
|------|------|
| `docs/schema/migrations/20260415_hs_model_to_codets.sql` | 新增型号映射、特征、推荐审计、datasheet 资产表 |
| `docs/schema/bom_mysql.sql` | 同步 schema |
| `docs/schema/bom_mysql_complete.sql` | 同步完整 schema |
| `internal/data/models.go` | 增加 4 张新表 GORM 模型 |
| `internal/data/table_names.go` | 增加表名常量 |
| `internal/data/migrate.go` | 注册 AutoMigrate |
| `internal/biz/repo.go` | 定义模型解析相关 repo 接口 |
| `internal/data/hs_model_mapping_repo.go` | 映射表读写（含幂等查询） |
| `internal/data/hs_datasheet_asset_repo.go` | datasheet 资产落库与查询 |
| `internal/data/hs_model_features_repo.go` | 抽取结果落库 |
| `internal/data/hs_model_recommendation_repo.go` | Top3 审计记录落库 |
| `internal/data/hs_datasheet_downloader.go` | datasheet 下载与 sha256 计算 |
| `internal/data/hs_llm_extract_client.go` | LLM 抽取客户端 |
| `internal/data/hs_llm_recommend_client.go` | LLM 推荐客户端 |
| `internal/biz/hs_model_resolver.go` | 核心状态机：映射命中、下载、抽取、推荐、回写 |
| `internal/biz/hs_model_confirm.go` | 人工确认逻辑（仅最新 run 可确认） |
| `internal/biz/hs_model_task.go` | 异步任务管理与任务查询 |
| `internal/service/hs_resolve_service.go` | `/resolve/by-model`、`/resolve/task`、`/resolve/confirm`、`/resolve/history` |
| `internal/conf/conf.proto` | 增加 `hs_resolve_sync_timeout_ms`、`hs_auto_accept_threshold` 等配置 |
| `configs/config.yaml` | 增加解析配置默认值 |
| `internal/*/*_test.go` | data/biz/service 单元测试 |

---

### Task 1: Schema 与模型落地（4 张表 + 索引约束）

**Files:**
- Create: `docs/schema/migrations/20260415_hs_model_to_codets.sql`
- Modify: `docs/schema/bom_mysql.sql`
- Modify: `docs/schema/bom_mysql_complete.sql`
- Modify: `internal/data/models.go`
- Modify: `internal/data/table_names.go`
- Modify: `internal/data/migrate.go`
- Test: `internal/data/hs_model_schema_test.go`

- [x] **Step 1: Write the failing test**

```go
func TestHsModelResolveTables_AutoMigrate(t *testing.T) {
    // assert tables exist:
    // t_hs_model_mapping / t_hs_model_features / t_hs_model_recommendation / t_hs_datasheet_asset
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./internal/data/... -run HsModelResolveTables -v`  
Expected: FAIL（表不存在或模型未注册）

- [x] **Step 3: Write minimal implementation**
  - 新增 4 张表 migration；
  - `code_ts` 使用 `char(10)`；
  - 增加 `code_ts` 仅允许 10 位数字字符串的校验约束（保留前导 0）；
  - `t_hs_model_recommendation` 添加 `run_id` 与 `uk_run_rank(run_id, candidate_rank)`；
  - `t_hs_model_features` 使用 `asset_id` 引用资产表。

- [x] **Step 4: Run test to verify it passes**

Run: `go test ./internal/data/... -run HsModelResolveTables -v`  
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add docs/schema/migrations/20260415_hs_model_to_codets.sql docs/schema/bom_mysql.sql docs/schema/bom_mysql_complete.sql internal/data/models.go internal/data/table_names.go internal/data/migrate.go internal/data/hs_model_schema_test.go
git commit -m "feat(schema): add HS model resolve tables and constraints"
```

---

### Task 2: Repository 层实现（映射/资产/特征/推荐）

**Files:**
- Create: `internal/data/hs_model_mapping_repo.go`
- Create: `internal/data/hs_datasheet_asset_repo.go`
- Create: `internal/data/hs_model_features_repo.go`
- Create: `internal/data/hs_model_recommendation_repo.go`
- Modify: `internal/biz/repo.go`
- Test: `internal/data/hs_model_mapping_repo_test.go`
- Test: `internal/data/hs_model_recommendation_repo_test.go`

- [x] **Step 1: Write the failing test**

```go
func TestHsModelMappingRepo_GetConfirmedByModelManufacturer(t *testing.T) {
    // seed confirmed row, then query should hit
}

func TestHsModelRecommendationRepo_SaveTop3ByRunID(t *testing.T) {
    // save rank 1..3, duplicate rank under same run_id should fail
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./internal/data/... -run HsModelMappingRepo -v`  
Expected: FAIL

- [x] **Step 3: Write minimal implementation**
  - repo 接口与 data 实现对齐；
  - 映射查询仅返回 `confirmed`；
  - 推荐表按 `run_id + rank` 幂等写入。

- [x] **Step 4: Run test to verify it passes**

Run: `go test ./internal/data/... -run HsModelMappingRepo -v`  
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add internal/biz/repo.go internal/data/hs_model_mapping_repo.go internal/data/hs_datasheet_asset_repo.go internal/data/hs_model_features_repo.go internal/data/hs_model_recommendation_repo.go internal/data/hs_model_mapping_repo_test.go internal/data/hs_model_recommendation_repo_test.go
git commit -m "feat(data): add repositories for HS model resolve pipeline"
```

---

### Task 3: Datasheet 下载与资产管理

**Files:**
- Create: `internal/data/hs_datasheet_downloader.go`
- Modify: `internal/biz/hs_model_resolver.go`
- Test: `internal/data/hs_datasheet_downloader_test.go`
- Test: `internal/biz/hs_model_resolver_datasheet_test.go`

- [x] **Step 1: Write the failing test**

```go
func TestSelectDatasheetURL_PickLatestValidURL(t *testing.T) {
    // multiple rows: pick non-empty and latest updated_at
}

func TestSelectDatasheetURL_TieBreakByIDDESC(t *testing.T) {
    // same updated_at and both valid -> pick larger id
}

func TestDownloader_SaveAndHash(t *testing.T) {
    // download mock file -> local path + sha256 persisted
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./internal/data/... ./internal/biz/... -run Datasheet -v`  
Expected: FAIL

- [x] **Step 3: Write minimal implementation**
  - datasheet URL 选源规则按 spec 固化；
  - 下载后写资产表；
  - 重复 sha256 可复用本地文件。

- [x] **Step 4: Run test to verify it passes**

Run: `go test ./internal/data/... ./internal/biz/... -run Datasheet -v`  
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add internal/data/hs_datasheet_downloader.go internal/biz/hs_model_resolver.go internal/data/hs_datasheet_downloader_test.go internal/biz/hs_model_resolver_datasheet_test.go
git commit -m "feat(hs): add datasheet selection and download asset flow"
```

---

### Task 4: LLM 抽取与推荐客户端（严格 JSON 协议）

**Files:**
- Create: `internal/data/hs_llm_extract_client.go`
- Create: `internal/data/hs_llm_recommend_client.go`
- Test: `internal/data/hs_llm_extract_client_test.go`
- Test: `internal/data/hs_llm_recommend_client_test.go`

- [x] **Step 1: Write the failing test**

```go
func TestExtractClient_ParseStrictJSON(t *testing.T) {
    // non-json response should fail
}

func TestRecommendClient_RejectCodeOutsideCandidates(t *testing.T) {
    // best_code_ts not in candidate set => error
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./internal/data/... -run LLM -v`  
Expected: FAIL

- [x] **Step 3: Write minimal implementation**
  - 封装 prompt 模板；
  - 解析 JSON 并字段校验；
  - 推荐结果必须来自候选集。

- [x] **Step 4: Run test to verify it passes**

Run: `go test ./internal/data/... -run LLM -v`  
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add internal/data/hs_llm_extract_client.go internal/data/hs_llm_recommend_client.go internal/data/hs_llm_extract_client_test.go internal/data/hs_llm_recommend_client_test.go
git commit -m "feat(llm): add strict extract and recommend clients for HS resolve"
```

---

### Task 5: 候选预筛引擎（TopN）与检索评分

**Files:**
- Create: `internal/biz/hs_candidate_prefilter.go`
- Create: `internal/data/hs_item_query_repo.go`
- Modify: `internal/biz/repo.go`
- Test: `internal/biz/hs_candidate_prefilter_test.go`
- Test: `internal/data/hs_item_query_repo_test.go`

- [x] **Step 1: Write the failing test**

```go
func TestPrefilter_ReturnTopNByRules(t *testing.T) {
    // tech_category + component_name + package + key_specs -> top N sorted candidates
}

func TestPrefilter_EmptyCandidates(t *testing.T) {
    // no match -> returns empty list and typed error for resolver fallback
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./internal/biz/... ./internal/data/... -run Prefilter -v`  
Expected: FAIL

- [x] **Step 3: Write minimal implementation**
  - 从 `t_hs_item` 按规则检索候选；
  - 输出 TopN（默认 20~40，可配置）；
  - 返回候选评分明细用于审计。

- [x] **Step 4: Run test to verify it passes**

Run: `go test ./internal/biz/... ./internal/data/... -run Prefilter -v`  
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add internal/biz/hs_candidate_prefilter.go internal/data/hs_item_query_repo.go internal/biz/repo.go internal/biz/hs_candidate_prefilter_test.go internal/data/hs_item_query_repo_test.go
git commit -m "feat(hs): add TopN candidate prefilter before LLM ranking"
```

---

### Task 6: Biz 核心状态机（幂等、run_id、阈值、Top3审计）

**Files:**
- Create: `internal/biz/hs_model_resolver.go`
- Create: `internal/biz/hs_model_task.go`
- Create: `internal/biz/hs_model_confirm.go`
- Test: `internal/biz/hs_model_resolver_test.go`
- Test: `internal/biz/hs_model_confirm_test.go`

- [x] **Step 1: Write the failing test**

```go
func TestResolveByModel_IdempotentRunIDReuse(t *testing.T) {
    // same model+manufacturer+request_trace_id -> same run_id
}

func TestResolveByModel_AutoAcceptThreshold(t *testing.T) {
    // high score => confirmed; low score => pending_review
}

func TestConfirm_OnlyLatestRunAllowed(t *testing.T) {
    // old run confirm must fail
}

func TestConfirm_IdempotentByConfirmRequestID(t *testing.T) {
    // same confirm_request_id should return same result without duplicate writes
}

func TestConfirm_RejectWhenCandidateTupleMismatch(t *testing.T) {
    // run_id + candidate_rank + expected_code_ts mismatch must fail
}

func TestResolver_FailureStagesAndRetryCounters(t *testing.T) {
    // datasheet_failed/extract_failed/recommend_failed with attempt_count and last_error
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./internal/biz/... -run HsModelResolver -v`  
Expected: FAIL

- [x] **Step 3: Write minimal implementation**
  - `request_trace_id` 幂等去重；
  - `run_id` 服务端生成并贯穿保存；
  - Top3 审计写入；
  - 自动接收阈值与人工确认优先级；
  - 下载/抽取/推荐失败分层重试 + 失败状态落库。

- [x] **Step 4: Run test to verify it passes**

Run: `go test ./internal/biz/... -run HsModelResolver -v`  
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add internal/biz/hs_model_resolver.go internal/biz/hs_model_task.go internal/biz/hs_model_confirm.go internal/biz/hs_model_resolver_test.go internal/biz/hs_model_confirm_test.go
git commit -m "feat(biz): implement HS model resolve state machine and confirm flow"
```

---

### Task 7: Service API 与同步转异步协议

**Files:**
- Create: `internal/service/hs_resolve_service.go`
- Modify: `api/...`（按仓库实际 HS proto 路径）
- Modify: `internal/service/provider.go`
- Modify: `internal/data/provider.go`
- Modify: `cmd/server/wire.go`
- Modify: `cmd/server/wire_gen.go`
- Test: `internal/service/hs_resolve_service_test.go`

- [x] **Step 1: Write the failing test**

```go
func TestResolveByModel_Return202WhenTimeout(t *testing.T) {
    // unresolved within sync timeout => 202 + task_id
}

func TestResolveByModel_Return200WhenCompleted(t *testing.T) {
    // completed in time => 200 + result payload
}

func TestResolveByModel_ResponseFields(t *testing.T) {
    // assert run_id, decision_mode, task_status, result_status
}

func TestResolveTask_FailedPayload(t *testing.T) {
    // failed task should include error_code and error_message
}

func TestResolveByModel_RequireRequestTraceID(t *testing.T) {
    // missing request_trace_id should fail with validation error
}

func TestResolveByModel_ForceRefreshBypassesMappingCache(t *testing.T) {
    // force_refresh=true should trigger new resolve pipeline
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./internal/service/... -run HsResolveService -v`  
Expected: FAIL

- [x] **Step 3: Write minimal implementation**
  - `POST /api/hs/resolve/by-model`；
  - `GET /api/hs/resolve/task`；
  - `POST /api/hs/resolve/confirm`；
  - `GET /api/hs/resolve/history`；
  - 明确 `task_status` 与 `result_status` 字段。
  - 生成代码而非手改：更新 proto 后执行生成命令。

- [x] **Step 3.1: Generate proto/wire artifacts**

Run: `go generate ./api/... && wire ./cmd/server`  
Expected: 生成 `*.pb.go` 与 `wire_gen.go`，无错误

- [x] **Step 4: Run test to verify it passes**

Run: `go test ./internal/service/... -run HsResolveService -v`  
Expected: PASS
Expected details: `by-model` 完成态 `HTTP 200`，超时转异步 `HTTP 202 + task_id`

- [x] **Step 5: Commit**

```bash
git add internal/service/hs_resolve_service.go internal/service/provider.go internal/data/provider.go cmd/server/wire.go cmd/server/wire_gen.go api/ internal/service/hs_resolve_service_test.go
git commit -m "feat(service): expose HS resolve APIs with sync-async contract"
```

---

### Task 8: 配置接入与全量验证

**Files:**
- Modify: `internal/conf/conf.proto`
- Modify: `internal/conf/conf.pb.go`
- Modify: `configs/config.yaml`
- Test: `internal/biz/hs_model_config_test.go`

- [x] **Step 1: Write the failing test**

```go
func TestHsResolveConfig_DefaultsAndBounds(t *testing.T) {
    // timeout and threshold are loaded and validated
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./internal/biz/... -run HsResolveConfig -v`  
Expected: FAIL

- [x] **Step 3: Write minimal implementation**
  - 新增配置：
    - `hs_resolve_sync_timeout_ms`
    - `hs_auto_accept_threshold`
    - `hs_resolve_max_candidates`
    - `hs_resolve_retry_max`
  - 配置边界校验（阈值 0~1）。

- [x] **Step 3.1: Regenerate config bindings**

Run: `go generate ./internal/conf/...`  
Expected: `conf.pb.go` 更新且可编译

- [x] **Step 4: Run test to verify it passes**

Run: `go test ./internal/biz/... -run HsResolveConfig -v`  
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add internal/conf/conf.proto internal/conf/conf.pb.go configs/config.yaml internal/biz/hs_model_config_test.go
git commit -m "feat(conf): add HS resolve runtime configs"
```

---

### Task 9: 回归测试与验收脚本

**Files:**
- Create: `internal/biz/hs_model_acceptance_test.go`
- Create: `scripts/hs_resolve_acceptance.ps1`

- [x] **Step 1: Write the failing test**

```go
func TestAcceptance_IdempotentReplay_NoDuplicateRun(t *testing.T) {}
func TestAcceptance_ConfirmLatestRunOnly(t *testing.T) {}
func TestAcceptance_TimeoutToAsyncAndTaskPoll(t *testing.T) {}
func TestAcceptance_MappingHit_NoDownloadNoLLM(t *testing.T) {}
func TestAcceptance_ConfirmConflict_OldRunRejected(t *testing.T) {}
func TestAcceptance_ManualOverrideHasLongTermPriority(t *testing.T) {}
func TestAcceptance_HistoryReplayByRunID(t *testing.T) {}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./internal/biz/... -run Acceptance -v`  
Expected: FAIL

- [x] **Step 3: Write minimal implementation**
  - 补齐缺失行为直至验收测试通过；
  - 不以实现反向修改 spec，若发现冲突先提设计变更评审。
  - 覆盖人工确认后优先于自动结果的长期命中行为。

- [x] **Step 4: Run test to verify it passes**

Run: `go test ./internal/biz/... -run Acceptance -v`  
Expected: PASS

- [x] **Step 4.1: Add executable acceptance script**

Run (PowerShell): `powershell -ExecutionPolicy Bypass -File scripts/hs_resolve_acceptance.ps1`  
Run (Quick mode): `powershell -ExecutionPolicy Bypass -File scripts/hs_resolve_acceptance.ps1 -Quick`  
Expected: 关键链路回归（Acceptance / Service / Observability）通过，非 Quick 模式额外验证 `data` 包回归与 `cmd/server` 可构建。

- [x] **Step 5: Commit**

```bash
git add internal/biz/hs_model_acceptance_test.go
git commit -m "test(hs): add acceptance coverage for resolve workflow"
```

---

### Task 10: 可观测性落地（日志字段与指标）

**Files:**
- Modify: `internal/biz/hs_model_resolver.go`
- Modify: `internal/service/hs_resolve_service.go`
- Create: `internal/biz/hs_model_observability_test.go`

- [x] **Step 1: Write the failing test**

```go
func TestObservability_EmitMetricsByStage(t *testing.T) {
    // assert hs_resolve_total / stage latency / auto_accept_ratio updates
}

func TestObservability_LogFieldsContainRequiredKeys(t *testing.T) {
    // assert model, manufacturer, task_id, run_id, stage, datasheet_url,
    // datasheet_path, extract_model, recommend_model, candidate_count,
    // best_score, final_status, error_code fields
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./internal/biz/... -run Observability -v`  
Expected: FAIL

- [x] **Step 3: Write minimal implementation**
  - 在关键阶段打结构化日志字段；
  - 上报核心指标（总量、分阶段耗时、自动接收比、人工覆盖次数）。
  - 指标至少覆盖：`hs_resolve_total`、`hs_resolve_stage_latency_ms`、`hs_resolve_auto_accept_ratio`、`hs_resolve_manual_override_total`。

- [x] **Step 4: Run test to verify it passes**

Run: `go test ./internal/biz/... -run Observability -v`  
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add internal/biz/hs_model_resolver.go internal/service/hs_resolve_service.go internal/biz/hs_model_observability_test.go
git commit -m "feat(hs): add observability for resolve workflow"
```

---

## 完成前验证清单（必须执行）

- [x] `powershell -ExecutionPolicy Bypass -File scripts/hs_resolve_acceptance.ps1`
- [x] `go test ./internal/data/...`
- [x] `go test ./internal/biz/...`
- [x] `go test ./internal/service/...`
- [x] `go build ./cmd/server/...`
- [x] 验证同步转异步：`/resolve/by-model` 超时返回 `202 + task_id`
- [x] 验证幂等：同 `request_trace_id` 重试复用 `run_id`
- [x] 验证确认保护：旧 `run_id` 无法覆盖新结果

---

## 风险与控制

- **风险：** datasheet URL 质量不稳定导致下载失败。  
  **控制：** 选源规则 + 下载重试 + 失败可观测。
- **风险：** LLM 输出不符合 JSON 协议。  
  **控制：** 严格解析与失败重试，失败进入 `recommend_failed`。
- **风险：** 并发请求造成映射覆盖。  
  **控制：** 幂等键、最新 run 限制、确认原子条件更新。

