# HS 元数据管理与查询入库 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 基于已确认 spec，完成 HS 元数据管理、`hs_query_api` 手动/定时触发同步、以及查询结果入本地库的完整可用链路。

**Architecture:** 遵循 Kratos 分层：`service` 提供接口与 DTO 编排，`biz` 承载同步编排/分页重试/入库决策，`data` 负责 GORM 持久化与外部 HTTP 调用。同步流程以 `t_hs_meta.enabled=1` 的 `core_hs6` 为输入，调用 `hs_query_api`（`filterField=CODE_TS`，`filterValue=core_hs6`）并 upsert 到本地 `t_hs_item`，同时记录 `t_hs_sync_job` 执行状态。

**Tech Stack:** Go（Kratos）、GORM、MySQL 8、proto/service、定时任务（项目现有调度方式）。

**Spec / 输入文档:** `docs/superpowers/specs/2026-04-15-hs-meta-management-and-query-design.md`

---

## 文件结构（创建 / 修改一览）

| 路径 | 职责 |
|------|------|
| `docs/schema/migrations/20260415_hs_meta_and_sync.sql` | 新增 `t_hs_meta`、`t_hs_sync_job`、`t_hs_item` 三表及索引/约束 |
| `docs/schema/bom_mysql.sql` | 同步全量 schema 定义 |
| `docs/schema/bom_mysql_complete.sql` | 同步完整 schema 定义 |
| `internal/data/models.go` | 增加 `HsMeta`、`HsSyncJob`、`HsItem` GORM 模型 |
| `internal/data/table_names.go` | 增加三张 HS 表名常量 |
| `internal/data/migrate.go` | 注册 AutoMigrate |
| `internal/biz/repo.go`（或现有 repo 接口文件） | 定义 HS 元数据、任务、结果 repo 接口 |
| `internal/data/hs_meta_repo.go` | `t_hs_meta` CRUD 与查询实现 |
| `internal/data/hs_sync_job_repo.go` | 任务创建、状态更新、列表/详情查询 |
| `internal/data/hs_item_repo.go` | HS 结果 upsert 与查询实现 |
| `internal/data/hs_query_api_client.go` | 外部 `hs_query_api` HTTP client 封装 |
| `internal/biz/hs_sync.go` | 同步编排：分页、重试、入库、任务汇总 |
| `internal/biz/hs_meta.go` | 元数据管理 usecase |
| `internal/service/hs_service.go` | 管理接口、手动触发接口、结果查询接口 |
| `api/...`（按仓库实际 HS proto 路径） | 若缺失则新增 HS service proto 与生成代码 |
| `internal/conf/conf.proto` | 新增 HS 同步配置（enable/cron/rowsPerPage/timeout） |
| `configs/config.yaml` | 增加 HS 同步配置块 |
| `cmd/server/wire.go` + `internal/data/provider.go` + `internal/service/provider.go` | 依赖注入 |
| `internal/*/*_test.go` | data/biz/service 对应单测 |

---

### Task 1: Schema 与模型骨架

**Files:**
- Create: `docs/schema/migrations/20260415_hs_meta_and_sync.sql`
- Modify: `docs/schema/bom_mysql.sql`
- Modify: `docs/schema/bom_mysql_complete.sql`
- Modify: `internal/data/models.go`
- Modify: `internal/data/table_names.go`
- Modify: `internal/data/migrate.go`

- [ ] **Step 1: 写失败测试（data 层最小存在性）**

```go
func TestHsTables_AutoMigrate(t *testing.T) {
    // 启动 data 层迁移后，检查 t_hs_meta / t_hs_sync_job / t_hs_item 存在
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/data/... -run HsTables -v`  
Expected: FAIL（表/模型不存在）

- [ ] **Step 3: 编写 migration + GORM 模型 + AutoMigrate 注册**
  - `t_hs_meta.core_hs6` 索引
  - `t_hs_meta(core_hs6, component_name)` 唯一约束
  - `t_hs_item.code_ts` 唯一约束

- [ ] **Step 4: 重新运行测试确认通过**

Run: `go test ./internal/data/... -run HsTables -v`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add docs/schema/migrations/20260415_hs_meta_and_sync.sql docs/schema/bom_mysql.sql docs/schema/bom_mysql_complete.sql internal/data/models.go internal/data/table_names.go internal/data/migrate.go
git commit -m "feat(schema,data): add HS meta/sync/item tables and models"
```

---

### Task 2: HS 元数据仓储与管理用例（TDD）

**Files:**
- Create: `internal/data/hs_meta_repo.go`
- Create: `internal/biz/hs_meta.go`
- Modify: `internal/biz/repo.go`（或对应 repo 接口文件）
- Test: `internal/data/hs_meta_repo_test.go`
- Test: `internal/biz/hs_meta_test.go`

- [ ] **Step 1: 写失败测试（core_hs6 校验与唯一性）**

```go
func TestCreateHsMeta_InvalidCoreHS6(t *testing.T) {
    // core_hs6 非 6 位数字时应返回参数错误
}

func TestCreateHsMeta_DuplicateCoreAndName(t *testing.T) {
    // 同 core_hs6 + component_name 重复创建应失败
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/biz/... ./internal/data/... -run HsMeta -v`  
Expected: FAIL

- [ ] **Step 3: 最小实现通过**
  - repo 实现 list/create/update/delete
  - biz 增加 `core_hs6` 格式校验（6 位数字）
  - 按 spec 支持 `enabled` 过滤

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/biz/... ./internal/data/... -run HsMeta -v`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/data/hs_meta_repo.go internal/biz/hs_meta.go internal/biz/repo.go internal/data/hs_meta_repo_test.go internal/biz/hs_meta_test.go
git commit -m "feat(hs): add HS meta repository and biz validation"
```

---

### Task 3: `hs_query_api` 客户端与分页拉取（TDD）

**Files:**
- Create: `internal/data/hs_query_api_client.go`
- Create: `internal/biz/hs_query_fetch.go`（可并入 `hs_sync.go`）
- Test: `internal/data/hs_query_api_client_test.go`
- Test: `internal/biz/hs_query_fetch_test.go`

- [ ] **Step 1: 写失败测试（参数映射强约束）**

```go
func TestBuildQueryPayload_UsesCoreHS6AsFilterValue(t *testing.T) {
    // 断言 filterField=CODE_TS 且 filterValue == core_hs6
}
```

- [ ] **Step 2: 写失败测试（分页拉取）**

```go
func TestFetchAllPages_CollectsRowsAcrossPages(t *testing.T) {
    // mock page1/page2，最终合并返回全部 data
}
```

- [ ] **Step 3: 运行测试确认失败**

Run: `go test ./internal/biz/... ./internal/data/... -run HsQuery -v`  
Expected: FAIL

- [ ] **Step 4: 实现 client + 分页循环 + 单页重试（最多 3 次）**
  - 第 1 页读取 `pageSumCount`
  - 后续页按页拉取
  - 响应 `status!=success` 视为失败

- [ ] **Step 5: 运行测试确认通过**

Run: `go test ./internal/biz/... ./internal/data/... -run HsQuery -v`  
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/data/hs_query_api_client.go internal/biz/hs_query_fetch.go internal/data/hs_query_api_client_test.go internal/biz/hs_query_fetch_test.go
git commit -m "feat(hs): implement hs_query_api client and paged fetch flow"
```

---

### Task 4: 同步任务编排与结果入库（TDD）

**Files:**
- Create: `internal/data/hs_sync_job_repo.go`
- Create: `internal/data/hs_item_repo.go`
- Create/Modify: `internal/biz/hs_sync.go`
- Modify: `internal/biz/repo.go`（新增 job/item repo 接口）
- Test: `internal/biz/hs_sync_test.go`
- Test: `internal/data/hs_item_repo_test.go`

- [ ] **Step 1: 写失败测试（手动触发全链路）**

```go
func TestRunSync_ManualAllEnabled_Success(t *testing.T) {
    // enabled core_hs6 -> fetch -> upsert t_hs_item -> job status success
}
```

- [ ] **Step 2: 写失败测试（部分失败）**

```go
func TestRunSync_WhenOneCoreFails_MarkPartialSuccess(t *testing.T) {
    // 单个核心码失败时任务状态 partial_success，且 summary 记录失败项
}
```

- [ ] **Step 3: 运行测试确认失败**

Run: `go test ./internal/biz/... ./internal/data/... -run HsSync -v`  
Expected: FAIL

- [ ] **Step 4: 最小实现通过**
  - 创建 job（running）
  - 逐 core_hs6 拉取并 upsert `t_hs_item`（唯一键 `code_ts`）
  - 结束后更新 job 状态与 summary

- [ ] **Step 5: 运行测试确认通过**

Run: `go test ./internal/biz/... ./internal/data/... -run HsSync -v`  
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/data/hs_sync_job_repo.go internal/data/hs_item_repo.go internal/biz/hs_sync.go internal/biz/repo.go internal/biz/hs_sync_test.go internal/data/hs_item_repo_test.go
git commit -m "feat(hs): add sync job orchestration and HS item upsert"
```

---

### Task 5: Service 接口与页面所需能力（TDD）

**Files:**
- Create: `internal/service/hs_service.go`
- Modify/Create: `api/...` HS proto 与生成代码（按仓库实际路径）
- Modify: `internal/service/provider.go`
- Modify: `cmd/server/wire.go`
- Modify: `internal/data/provider.go`
- Test: `internal/service/hs_service_test.go`

- [ ] **Step 1: 写失败测试（管理接口）**

```go
func TestHsService_ListMeta(t *testing.T) {}
func TestHsService_CreateMeta_ValidateCoreHS6(t *testing.T) {}
```

- [ ] **Step 2: 写失败测试（手动触发 + 任务查询）**

```go
func TestHsService_RunSyncAndQueryJobs(t *testing.T) {}
```

- [ ] **Step 3: 运行测试确认失败**

Run: `go test ./internal/service/... -run HsService -v`  
Expected: FAIL

- [ ] **Step 4: 实现 service 编排与 DTO 映射**
  - meta CRUD 接口
  - `POST /api/hs/sync/run` 对应 RPC/handler
  - jobs/job_detail 与 items 查询

- [ ] **Step 5: 运行测试确认通过**

Run: `go test ./internal/service/... -run HsService -v`  
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/service/hs_service.go internal/service/provider.go internal/data/provider.go cmd/server/wire.go cmd/server/wire_gen.go api/
git commit -m "feat(service): expose HS meta and sync management APIs"
```

---

### Task 6: 定时任务与配置接入（TDD）

**Files:**
- Modify: `internal/conf/conf.proto`
- Modify: `internal/conf/conf.pb.go`（生成）
- Modify: `configs/config.yaml`
- Modify: `internal/biz/hs_sync.go`（或独立 scheduler 文件）
- Test: `internal/biz/hs_sync_scheduler_test.go`

- [ ] **Step 1: 写失败测试（scheduler 读取 enabled 元数据触发）**

```go
func TestHsSyncScheduler_RunOnTick(t *testing.T) {
    // 模拟 tick，验证调用 runSync(schedule)
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/biz/... -run HsSyncScheduler -v`  
Expected: FAIL

- [ ] **Step 3: 配置与调度最小实现**
  - `hs_sync_enabled`
  - `hs_sync_cron`
  - `hs_sync_rows_per_page`
  - `hs_sync_timeout_seconds`

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/biz/... -run HsSyncScheduler -v`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/conf/conf.proto internal/conf/conf.pb.go configs/config.yaml internal/biz/hs_sync.go internal/biz/hs_sync_scheduler_test.go
git commit -m "feat(hs): add scheduled sync with configurable cron"
```

---

### Task 7: 全量验证与回归

**Files:**
- Modify (optional): `docs/superpowers/specs/2026-04-15-hs-meta-management-and-query-design.md`（仅当需要补充落地差异）

- [ ] **Step 1:** 运行单元测试集合

Run:
- `go test ./internal/data/...`
- `go test ./internal/biz/...`
- `go test ./internal/service/...`

Expected: 全 PASS

- [ ] **Step 2:** 构建服务

Run: `go build ./cmd/server/...`  
Expected: build 成功

- [ ] **Step 3:** 手工验收（最少 2 组核心码）
  - 在 HS 元数据管理接口创建并启用 `core_hs6`
  - 手动触发同步，确认 job 从 running -> success/partial_success
  - 验证 `t_hs_item` 中 `code_ts` upsert 生效（重复触发无脏重复）

- [ ] **Step 4:** 若有文档变更，单独 commit

```bash
git add docs/superpowers/specs/2026-04-15-hs-meta-management-and-query-design.md
git commit -m "docs: align HS spec with implementation details"
```

---

## 验证清单（完成前必须执行）

- [ ] `go test ./internal/data/...`
- [ ] `go test ./internal/biz/...`
- [ ] `go test ./internal/service/...`
- [ ] `go build ./cmd/server/...`
- [ ] 手动触发同步验证：`filterValue` 实际等于 `core_hs6`
- [ ] 定时触发验证：配置生效并产生 `trigger_type=schedule` 任务记录

---

## 风险与控制

- **风险：** 外部接口不稳定导致同步波动。  
  **控制：** 单页最多 3 次重试 + `partial_success` 状态可观测。
- **风险：** 核心码格式不统一造成查询噪音。  
  **控制：** biz 层强校验 `core_hs6` 必须 6 位数字，service 层拒绝非法输入。
- **风险：** 重复同步写入脏数据。  
  **控制：** `t_hs_item.code_ts` 唯一约束 + upsert 策略。

---

**Plan complete and saved to `docs/superpowers/plans/2026-04-15-hs-meta-management-and-query-implementation.md`. Two execution options:**

**1. Subagent-Driven (recommended)** - I dispatch a fresh subagent per task, review between tasks, fast iteration

**2. Inline Execution** - Execute tasks in this session using executing-plans, batch execution with checkpoints

**Which approach?**
