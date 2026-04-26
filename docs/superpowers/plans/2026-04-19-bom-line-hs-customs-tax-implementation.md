# 配单行 HS / 商检 / 关税日缓存 — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: 使用 `superpowers:subagent-driven-development`（推荐）或 `superpowers:executing-plans` **按 Task 顺序**实现；步骤使用 `- [ ]` 勾选跟踪。  
> **建议：** 在独立 git worktree 中实施（仓库备忘：[using-git-worktrees.md](../using-git-worktrees.md)）。

**Goal:** 按 [docs/superpowers/specs/2026-04-19-bom-line-hs-customs-tax-design.md](../specs/2026-04-19-bom-line-hs-customs-tax-design.md)，在 **`GetMatchResult` / `AutoMatch` 共用路径**上为每行 `MatchItem` 增加 HS 状态、`code_ts`、商检标记、进口关税展示字段；关税经 **`t_hs_tax_rate_daily`** 按 `(code_ts, 服务器本地日历日)` 缓存；并与 **`POST /api/hs/resolve/by-model`**（一键找 HS）在 **model / manufacturer 规范化**上对齐。

**Architecture:** 在 **`internal/biz`** 实现纯编排用例（按行批量查 `confirmed` 映射 → 合法 `code_ts` → 批量 `t_hs_item` → 批量日缓存 → 对未命中键限流并发调 `HsTaxRateAPIRepo.FetchByCodeTS` → `ON DUPLICATE`/重试读处理并发）；**`internal/data`** 提供 `HsTaxRateDailyRepo`（GORM）与既有 `HsTaxRateAPIRepo`；**`internal/service`** 在 `computeMatchItems` 成功返回后按 `lines[i]` ↔ `items[i]` 合并 biz 输出到 **`api/bom/v1` `MatchItem`**。不将「是否打外网」决策放在 `data`。

**Tech Stack:** Go、Kratos、GORM、MySQL 8+、Wire、`protoc`（`make api`）、现有 `docs/schema/migrations/20260419_t_hs_tax_rate_daily.sql` 与 `HsTaxRateDaily` 模型。

**Spec / 设计输入:** [2026-04-19-bom-line-hs-customs-tax-design.md](../specs/2026-04-19-bom-line-hs-customs-tax-design.md) · [specs 索引](../specs/README.md) · [tax_rate_api](../../tax_rate_api)

---

## 完成状态（2026-04-19）

**代码侧：** Task 1～7 已在仓库落地（biz 编排、`t_hs_tax_rate_daily` 仓储、税率 API 适配、`MatchItem` 扩展、`BomService` 挂载、`ResolveByModel` 空厂牌、Wire 注入顺序等）。下方计划内对应步骤已勾选。

**仍依赖各环境自行确认：** 生产/测试库是否已执行 `20260419_t_hs_tax_rate_daily.sql`（或 AutoMigrate）；本地是否已跑通 `go test` / `go build` / `wire` 与 **Task 8**（集成或手工调 `GET .../match`）。

**前端（经典配单页）：** `web/src/api/types.ts`、`bomLegacy.ts` 已对齐 `MatchItem` 新字段；`MatchResultPage.tsx` 已展示 HS/商检/进口税列、「一键找 HS」调 `/api/hs/resolve/by-model` 并轮询任务后静默刷新配单。货源会话等其它页面未改。

---

## 文件结构（创建 / 修改一览）

| 路径 | 职责 |
|------|------|
| **Modify** `api/bom/v1/bom.proto` | 扩展 `MatchItem`：HS 状态、`code_ts`、`control_mark`、关税展示字段、`hs_customs_error`（或 `repeated` 分项）；可选嵌套 `ImportTaxQuote` message |
| **Run** `make api` | 重生成 `api/bom/v1/*.pb.go` |
| **Create** `internal/biz/bom_line_customs_types.go` | 行级结果 DTO、`HsCodeStatus` 常量、错误分项枚举（与 design §6.1.1 一致） |
| **Create** `internal/biz/bom_line_customs.go` | `BomLineCustomsFiller` 或包级函数 `FillBomLineCustoms(ctx, in) ([]BomLineCustomsOut, error)`：批量映射、批量 item、批量日表、外呼去重与 worker 池 |
| **Create** `internal/biz/bom_line_customs_test.go` | 表驱动单测：mock repos + API；覆盖 `hs_found` / `hs_not_mapped` / `hs_code_invalid`、缓存命中、item 缺失仍可取税、并发双写重试读 |
| **Modify** `internal/biz/repo.go` | 新增 `HsTaxRateDailyRepo`（`GetByCodeTSAndBizDate` / `GetByCodeTSAndBizDates` / `Upsert` 等最小集） |
| **Create** `internal/data/hs_tax_rate_daily_repo.go` | GORM 实现；插入冲突时捕获 duplicate → 由 biz 重读（design §4.3-4） |
| **Modify** `internal/data/provider.go` | `NewHsTaxRateDailyRepo`；`NewHsTaxRateAPIRepo`；`wire.Bind(new(biz.HsTaxRateDailyRepo), …)`；为税率 API 定义 `biz` 侧窄接口时增加 `wire.Bind` |
| **Modify** `internal/service/bom_service.go` | `BomService` 注入新依赖；`matchDepsOK` 或单独 `customsDepsOK()` 判定 HS 相关依赖是否可用 |
| **Create** `internal/service/bom_match_customs.go`（或并入 `bom_match_parallel.go`） | `attachCustomsToMatchItems(ctx, view, lines, items)` 调用 biz 并写回 `*v1.MatchItem` |
| **Modify** `internal/service/bom_match_parallel.go` | `computeMatchItems` 末尾在 `items` 与 `lines` 对齐后调用 `attachCustoms…`（**勿**在 `matchOneLine` 内按行打外网，避免重复 `code_ts`） |
| **Modify** `internal/service/hs_resolve_service.go` | **对齐 design §2.1.1**：`ResolveByModel` 在校验上允许 **`manufacturer` 为空字符串**（与 `GetConfirmedByModelManufacturer`、映射表 `manufacturer=''` 一致）；保留 `model`、`request_trace_id` 必填 |
| **Modify** `cmd/server/wire.go` / `wire_gen.go` | `make wire`：`NewBomService` 新参数、税率相关 provider |
| **Modify** `docs/superpowers/specs/README.md`（可选） | 增加本 plan 链接 |

---

### Task 0: DDL 与模型（已具备则跳过）

**Files:**
- `docs/schema/migrations/20260419_t_hs_tax_rate_daily.sql`
- `internal/data/hs_tax_rate_daily.go`
- `internal/data/migrate.go`

- [x] **Step 1:** 确认目标环境已执行 migration（或依赖 `AutoMigrateSchema` 已包含 `HsTaxRateDaily`）。
- [x] **Step 2:** 若无表，在测试库执行上述 SQL。

---

### Task 1: `biz` 接口与类型（TDD 入口）

**Files:**
- `internal/biz/repo.go`
- `internal/biz/bom_line_customs_types.go`

- [x] **Step 1:** 在 `repo.go` 定义 `HsTaxRateDailyRepo`：`GetMany(ctx, keys []CodeTSBizDate) (map[string]*HsTaxRateDailyRecord, error)`，`Upsert(ctx, row *HsTaxRateDailyRecord) error`（键：`code_ts`+`biz_date`）。
- [x] **Step 2:** 定义 `TaxRateAPIFetcher`（或等价）接口，方法返回 **biz 自有**结构（从 `HsTaxRateFetchResult` 映射），避免 `biz` import `internal/data` 具体类型。
- [x] **Step 3:** 在 `bom_line_customs_types.go` 定义 `HsCodeStatus` 常量：`hs_found` / `hs_not_mapped` / `hs_code_invalid`（与 design §2.3 一致）；`BomLineCustomsOut` 含 `LineNo`、`Status`、`CodeTS`、`ControlMark`、税率四字段、分项错误 `[]string` 或位掩码。
- [x] **Step 4:** Commit  

```bash
git add internal/biz/repo.go internal/biz/bom_line_customs_types.go
git commit -m "feat(biz): bom line customs types and tax daily repo iface"
```

---

### Task 2: `biz` 编排核心 `FillBomLineCustoms`（TDD）

**Files:**
- `internal/biz/bom_line_customs.go`
- `internal/biz/bom_line_customs_test.go`

- [x] **Step 1: 写失败测试** — 输入两行：行 A `confirmed`+合法 `code_ts`，行 B 无映射；期望 A `hs_found`，B `hs_not_mapped`；mock `HsModelMappingRepo` / `HsItemReadRepo` / `HsTaxRateDailyRepo` / `TaxRateAPIFetcher`。

- [x] **Step 2:** `go test ./internal/biz/... -run FillBomLineCustoms -v`，预期 FAIL。

- [x] **Step 3:** 实现逻辑顺序（design §2～§4）：
  1. 对每行：`model = trim(mpn)`，`manufacturer = NormalizeMfrString(trim(mfr))`（空指针先变 `""` 再 norm，与 Resolve 对齐口径在 **Task 5** 固化）。
  2. `GetConfirmedByModelManufacturer`；校验 `code_ts` 为 **10 位数字**。
  3. 收集需查的 `code_ts` 集合；`GetByCodeTS` 可批量循环或增加 `ListByCodeTSIn`（若为避免 N+1，优先 **单条 SQL `WHERE code_ts IN (?)`** 在 `HsItemReadRepo` 扩展，或 biz 内并行 `errgroup` 上限 16 —— **计划推荐**：data 层增加 `ListHsItemsByCodeTS(ctx, codes []string) (map[string]*HsItemRecord, error)` 单查询）。
  4. `biz_date`：`time.Now().In(time.Local)` 取 `YYYY-MM-DD` 与 `view.BizDate` 区分：税率缓存用 **§4.2 服务器本地日**（design），在测试中固定 `clock` 或入参 `nowLocal` 便于断言。
  5. 批量 `GetMany` 日缓存；剩余 `code_ts` 用 **worker 池**（如 `ants` 或 `semaphore`，**并发上限 4**）调 `Fetch`；解析 `Items` 中 **首条 `codeTs` 与请求一致**（design §4.3）；`Upsert`；遇 duplicate 则 `GetMany` 重读。
  6. 接口失败：该行税率字段空，`hs_customs_error` 含 `tax_api_failed`（文案常量即可）。

- [x] **Step 4:** 测试全绿。

- [x] **Step 5:** Commit  

```bash
git add internal/biz/bom_line_customs.go internal/biz/bom_line_customs_test.go
git commit -m "feat(biz): fill bom line customs hs item and tax daily"
```

---

### Task 3: `data` — `HsTaxRateDailyRepo` + 可选 `HsItem` 批量读

**Files:**
- `internal/data/hs_tax_rate_daily_repo.go`
- `internal/data/hs_tax_rate_daily_repo_test.go`（sqlite 或 docker mysql 与现有 data 测试一致）
- `internal/data/hs_item_read_repo.go`（若新增 `ListByCodeTS`）
- `internal/biz/repo.go`（若扩展 `HsItemReadRepo`）

- [x] **Step 1:** 实现 `HsTaxRateDailyRepo`：`GetMany` 用 `WHERE (code_ts, biz_date) IN …` 或循环 `IN code_ts AND biz_date = ?`（单日场景一次 `biz_date` 绑定更简单）。
- [x] **Step 2:** `Upsert` 使用 `clause.OnConflict{DoUpdates: …}` 更新税率列与 `updated_at`。
- [x] **Step 3:** `go test ./internal/data/... -run HsTaxRateDaily -v`。
- [x] **Step 4:** Commit  

```bash
git add internal/data/hs_tax_rate_daily_repo.go internal/data/hs_item_read_repo.go internal/biz/repo.go internal/data/provider.go
git commit -m "feat(data): hs tax rate daily repo and optional batch hs item"
```

---

### Task 4: `TaxRateAPIFetcher` 适配器

**Files:**
- `internal/data/tax_rate_api_biz_adapter.go`（或放在 `hs_tax_rate_daily_repo.go` 同包小类型）
- `internal/data/provider.go`

- [x] **Step 1:** 实现 `biz.TaxRateAPIFetcher`，内部持有 `*HsTaxRateAPIRepo`，`Fetch` 将 `*HsTaxRateFetchResult` 转为 biz DTO。
- [x] **Step 2:** Wire：`NewHsTaxRateAPIRepo(bc)` 注入 `data.ProviderSet`；`wire.Bind(new(biz.TaxRateAPIFetcher), new(*TaxRateAPIBizAdapter))`（类型名以实际为准）。
- [x] **Step 3:** `make wire`（见 Task 6 一并执行亦可）。

---

### Task 5: `ResolveByModel` 与映射 **manufacturer 空串** 对齐

**Files:**
- `internal/service/hs_resolve_service.go`
- `internal/service/hs_resolve_service_test.go`（或 `internal/server/hs_resolve_http_test.go`）

- [x] **Step 1:** 将 `ResolveByModel` 入口校验改为：`model != "" && request_trace_id != ""`，**允许** `manufacturer == ""`。
- [x] **Step 2:** 单测：空厂牌 + 合法 trace 返回非 `BadRequest`（可与现有 mock resolver 配合）。
- [x] **Step 3:** 确认 `biz.HsModelResolveRequest` 与 mapping 写入路径接受空厂牌（与 `hs_model_mapping_repo` 行为一致）。
- [x] **Step 4:** Commit  

```bash
git add internal/service/hs_resolve_service.go internal/service/hs_resolve_service_test.go
git commit -m "fix(hs-resolve): allow empty manufacturer for mapping parity"
```

---

### Task 6: Proto + `make api`

**Files:**
- `api/bom/v1/bom.proto`
- `api/bom/v1/bom.pb.go`（生成）

- [x] **Step 1:** 在 `MatchItem` 上新增字段（field numbers **从 15 起递增**，勿改已有序号），例如：
  - `string hs_code_status = 15;`
  - `string code_ts = 16;`
  - `string control_mark = 17;`
  - `string import_tax_g_name = 18;`
  - `string import_tax_imp_ordinary_rate = 19;`
  - `string import_tax_imp_discount_rate = 20;`
  - `string import_tax_imp_temp_rate = 21;`
  - `string hs_customs_error = 22;`（可用 `|` 分隔码或 JSON，与前端约定写进注释）

- [x] **Step 2:** 运行  

```bash
make api
```

  预期：`api/bom/v1/*.pb.go` 无编译错误。

- [x] **Step 3:** Commit proto + 生成代码  

```bash
git add api/bom/v1/bom.proto api/bom/v1/bom.pb.go api/bom/v1/bom_grpc.pb.go api/bom/v1/bom_http.pb.go
git commit -m "feat(api): match item hs customs and import tax fields"
```

---

### Task 7: `BomService` 集成与 Wire

**Files:**
- `internal/service/bom_service.go`
- `internal/service/bom_match_customs.go`（新建）或 `bom_match_parallel.go`
- `cmd/server/wire.go` → `wire_gen.go`

- [x] **Step 1:** `NewBomService` 增加依赖：`biz.HsModelMappingRepo`、`biz.HsItemReadRepo`（或扩展后的批量接口）、`biz.HsTaxRateDailyRepo`、`biz.TaxRateAPIFetcher`；**绑定** `wire.Bind(new(biz.HsModelMappingRepo), new(*data.HsModelMappingRepo))` 若尚未全局绑定。
- [x] **Step 2:** `matchDepsOK`：在现有条件上，增加「HS 扩展可选」策略二选一并在注释中写明：
  - **A（推荐）：** HS 依赖齐全才填扩展字段，缺一则整段 customs 跳过并打 `Debug`（不配单失败）；或
  - **B：** 缺依赖返回 `ServiceUnavailable`（更严格，可能破坏旧环境）。

- [x] **Step 3:** `computeMatchItems` 返回前调用 `attachCustomsToMatchItems`：`outs, err := biz.FillBomLineCustoms(...)`，按 `line.LineNo == item.Index` 对齐写入 proto（`Index` 已为 `line_no`）。

- [x] **Step 4:**  

```bash
cd cmd/server && wire
```

  预期：`wire_gen.go` 更新且无报错。

- [x] **Step 5:**  

```bash
go build -o bin/server ./cmd/server/...
```

  预期：编译通过。

- [x] **Step 6:** Commit  

```bash
git add internal/service cmd/server/wire_gen.go cmd/server/wire.go internal/data/provider.go
git commit -m "feat(bom): attach hs customs and tax to match items"
```

---

### Task 8: 集成 / 回归验证（发布前 checklist，可选）

**Files:**
- （按需）`internal/service/bom_service_test.go` 或新 `bom_match_customs_integration_test.go`

- [ ] **Step 1:** 有测试库时：`GetMatchResult` 路径下断言某行 `hs_code_status` 与 DB 种子一致。
- [ ] **Step 2:** 手工：`curl`/Kratos client 调 `GET /api/v1/bom/{uuid}/match`，检查 JSON 新字段。

Run:

```bash
go test ./internal/biz/... ./internal/data/... ./internal/service/... -count=1
```

预期：全部 PASS（或跳过无 DB 的用例与现有仓库一致）。

---

## 实现注意（摘自设计，执行时必读）

- **仅 `confirmed` 映射**参与 `code_ts`；`pending_review` 仍显示 `hs_not_mapped`（design §5.3）。  
- **分项失败**：商检与税率独立回填（design §6.1.1）。  
- **不设负缓存**：失败不写占位行，可能重复外呼（design §4.3-6）；靠日缓存命中与并发上限缓解。  
- **日志**：勿打印完整税率 URL。

---

## 修订记录

| 日期 | 说明 |
|------|------|
| 2026-04-19 | 初稿：对齐 design spec；补充 `ResolveByModel` 空厂牌与映射一致；`computeMatchItems` 后挂 customs |
| 2026-04-19 | 标记 Task 1～7 完成；Task 8 保留为发布前可选 checklist |
