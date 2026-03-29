# BOM 自动配单 — 币种归一 / 阶梯价解析 / 厂牌别名 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: 使用 `superpowers:subagent-driven-development`（推荐）或 `superpowers:executing-plans` **按 Task 顺序**实现；步骤使用 `- [ ]` 勾选跟踪。  
> **建议：** 在独立 git worktree 中实施（仓库备忘：[using-git-worktrees.md](../using-git-worktrees.md)）。

**Goal:** 按 [docs/superpowers/specs/2026-03-28-bom-match-currency-mfr-design.md](../specs/2026-03-28-bom-match-currency-mfr-design.md) 实现 **读取时币种归一**、**`price_tiers` 字符串解析（§1.11）**、**多字段取价优先级（§1.7）**、**MOQ/阶梯选档（§1.9）**、**舍入与平局（§1.10，含交期优先）**、**厂牌别名表与匹配（§2）**，并接入现有 **`t_bom_quote_cache` + `bom_session`/`bom_session_line`** 数据流；为 **`SearchQuotes` / `AutoMatch` / `GetMatchResult`**（当前在 `internal/service/bom_service.go` 多为 stub）提供可验收实现。

**Architecture:** **纯函数 + 表驱动** 放在 `internal/biz`（匹配规则、解析、排序比较器，**无 GORM**）；**汇率行、厂牌别名行** 在 `internal/data` 提供 repo；**配单编排**（按会话/行拉缓存 → 展开 `quotes_json` → 过滤 MPN/封装/参数/厂牌/库存 → 比价）在 `internal/biz` 用例函数中完成，由 `internal/service/bom_service.go` 调用。**平台字段映射** V1 可用 `internal/biz/bom_platform_quote_map.go` 内嵌表（与 spec「单一事实源」一致，后续可迁 DB）。**配置** `base_ccy`、是否解析阶梯字符串、舍入模式等走 `internal/conf/conf.proto` + `configs/config.yaml`。

**Tech Stack:** Go 1.25+、Kratos、GORM、MySQL 8+、Wire、protobuf（`api/bom/v1/bom.proto` 已有 `PlatformQuote` 等）。

**Spec / 需求输入:** [2026-03-28-bom-match-currency-mfr-design.md](../specs/2026-03-28-bom-match-currency-mfr-design.md) · [2026-03-27-bom-sourcing-requirements.md](../specs/2026-03-27-bom-sourcing-requirements.md) §6

---

## 文件结构（创建 / 修改一览）

| 路径 | 职责 |
|------|------|
| **Create** `docs/schema/migrations/20260328_bom_manufacturer_alias.sql`（日期按实际提交调整） | 表 `t_bom_manufacturer_alias`：`canonical_id`、`display_name`、`alias`（规范化后唯一索引） |
| **Create** `docs/schema/migrations/20260328_bom_fx_rate.sql` | 表 `t_bom_fx_rate`：`from_ccy`、`to_ccy`、`biz_date`、`rate`、`version`/`source`；与 §1.8 对齐 |
| **Modify** `internal/data/models.go` + `table_names.go` + `migrate.go` | 注册新模型与 AutoMigrate |
| **Modify** `internal/conf/conf.proto` | 新增 `message BomMatch { string base_ccy = 1; bool parse_price_tier_strings = 2; string rounding = 3; ... }`；`Bootstrap` 挂接；`make api`/`protoc` 生成 |
| **Modify** `configs/config.yaml` | 示例块 `bom_match` |
| **Create** `internal/biz/bom_price_tier_parse.go` + `_test.go` | §1.11 整串解析、§1.9 按 Q 选档 |
| **Create** `internal/biz/bom_compare_price.go` + `_test.go` | §1.7 优先级链：unit_price → mainland/hk（同解析器若格式一致）→ price_tiers |
| **Create** `internal/biz/bom_lead_time_parse.go` + `_test.go` | §1.10 `lead_days`：区间下限、N/A、现货占位 |
| **Create** `internal/biz/bom_mfr_normalize.go` + `_test.go` | 基础规范化 + 查表解析 canonical（接口由 biz 定义） |
| **Create** `internal/biz/bom_fx.go` + `_test.go` | 按 `biz_date` 查 rate，`base_ccy` 换算；`fx_date_source` 回退 |
| **Create** `internal/biz/bom_match_sort.go` + `_test.go` | §1.10：`unit_price_base` 舍入（二选一固定）、次键 lead/stock/platform |
| **Create** `internal/biz/bom_line_match.go` | 对单行 + 多平台缓存切片：过滤、选最优、返回审计 DTO（§3） |
| **Create** `internal/biz/bom_platform_quote_map.go` | 每平台 `default_quote_ccy`、`lead_time` 语义、含税换算系数（§1.6）— V1 常量表 |
| **Create** `internal/data/bom_manufacturer_alias_repo.go` | `ListAll` 或 `Resolve(aliasNorm) (canonicalID, ok)` |
| **Create** `internal/data/bom_fx_rate_repo.go` | `GetRate(from, to, date)` |
| **Create/Modify** `internal/data/bom_quote_cache_repo.go`（或等价） | 按 `session`/`mpn_norm`/`platform_ids`/`biz_date` 批量读 `BomQuoteCache`（若无则新增查询方法） |
| **Modify** `internal/service/bom_service.go` | 实现 `SearchQuotes` / `AutoMatch`（若 proto 有）/ `GetMatchResult`：门禁 `data_ready`、调用 biz 配单 |
| **Modify** `cmd/server/wire.go` + `internal/data/provider.go` + `internal/service/provider.go` | 注入 repo 与 `conf.Bootstrap` |
| **Modify** `api/bom/v1/bom.proto`（可选） | `MatchItem` 增加 `original_unit_price`、`original_ccy`、`unit_price_base`、`fx_date` 等 — **与前端 `MatchResultPage` 对齐时再改** |
| **Create** `docs/superpowers/specs/2026-03-28-bom-match-ops.md`（可选） | 别名与汇率种子 SQL、运维说明（§4） |

---

### Task 1: 配置与 DDL 骨架

**Files:**
- `internal/conf/conf.proto`
- `internal/conf/conf.pb.go`（生成）
- `configs/config.yaml`
- `docs/schema/migrations/*_bom_manufacturer_alias.sql`
- `docs/schema/migrations/*_bom_fx_rate.sql`

- [ ] **Step 1:** 在 `conf.proto` 的 `Bootstrap` 增加 `BomMatch`：`base_ccy`（默认 `CNY`）、`parse_price_tier_strings`（默认 `true`）、`rounding_mode` 枚举字符串 `minor_unit` \| `decimal6`、可选 `bom_qty_round`（如 `ceil`）。
- [ ] **Step 2:** 运行项目既有 proto 生成命令，确认 `conf.pb.go` 无冲突。
- [ ] **Step 3:** 编写两张表的 **手工迁移 SQL**（含 `uk_alias_norm`、`idx_fx_lookup`），与 spec §2.7、§1.8 一致。
- [ ] **Step 4:** `configs/config.yaml` 增加注释示例块。
- [ ] **Step 5:** Commit  

```bash
git add internal/conf/conf.proto internal/conf/conf.pb.go configs/config.yaml docs/schema/migrations/
git commit -m "feat(conf,schema): bom_match config and manufacturer_alias/fx_rate DDL"
```

---

### Task 2: GORM 模型与 AutoMigrate

**Files:**
- `internal/data/models.go`
- `internal/data/table_names.go`
- `internal/data/migrate.go`

- [ ] **Step 1:** 定义 `BomManufacturerAlias`、`BomFxRate` 模型，表名常量与迁移一致。
- [ ] **Step 2:** `AutoMigrate` 注册；本地空库跑启动或写一次性 `go test`/`main` 验证无错误。
- [ ] **Step 3:** Commit  

```bash
git add internal/data/models.go internal/data/table_names.go internal/data/migrate.go
git commit -m "feat(data): GORM models for BOM manufacturer alias and FX rate"
```

---

### Task 3: `price_tiers` 解析与选档（TDD）

**Files:**
- `internal/biz/bom_price_tier_parse.go`
- `internal/biz/bom_price_tier_parse_test.go`

- [ ] **Step 1: 写失败测试** — 使用 spec §1.11.3 三平台样例字符串；`Q=100` ickey → `10.2928` CNY；`Q=500` find_chips → `4.6114` USD；任一段非法则 **整串失败**。

```go
func TestParsePriceTiers_Ickey_Q100(t *testing.T) {
    s := "1+ ￥14.0729 | 10+ ￥12.7522 | 30+ ￥10.5661 | 100+ ￥10.2928 | 300+ ￥9.7281 | 1000+ ￥9.1087"
    p, ccy, ok := PickCompareUnitPriceFromPriceTiers(s, 100)
    if !ok || ccy != "CNY" || math.Abs(p-10.2928) > 1e-6 {
        t.Fatalf("got %v %s %v", p, ccy, ok)
    }
}
```

- [ ] **Step 2:** `go test ./internal/biz/... -run PriceTier -v`，预期 FAIL。
- [ ] **Step 3:** 实现 `ParsePriceTiers` + `PickCompareUnitPriceFromPriceTiers(s string, q int) (price float64, ccy string, ok bool)`，正则与 spec §1.11.1 一致。
- [ ] **Step 4:** 测试全绿。
- [ ] **Step 5:** Commit  

```bash
git add internal/biz/bom_price_tier_parse.go internal/biz/bom_price_tier_parse_test.go
git commit -m "feat(biz): parse price_tiers per BOM match spec §1.11"
```

---

### Task 4: §1.7 取价链 + §1.9 MOQ/库存门槛（TDD）

**Files:**
- `internal/biz/bom_compare_price.go`
- `internal/biz/bom_compare_price_test.go`
- （依赖 Task 3）

- [ ] **Step 1:** 构造 `PlatformQuote` 等价 Go 结构体输入（避免测试依赖整个 proto 生成包时可用最小 struct），覆盖：`unit_price` 优先；`unit_price` 缺失时 `mainland_price` 与 `price_tiers` 同形；仅 `price_tiers`；`parse_price_tier_strings=false` 时跳过第 4 步。
- [ ] **Step 2:** 实现 `ExtractCompareUnitPrice(q *QuoteFields, platformID string, bomQty int, cfg ExtractConfig) (res ComparePriceResult, ok bool)`，`ComparePriceResult` 含 `Source`（枚举：`unit_price` / `mainland_price` / `hk_price` / `price_tiers_parsed`）、`Price`、`Ccy`。
- [ ] **Step 3:** MOQ：若存在结构化 `moq` 且 `moq > bomQty` → `ok=false`。库存：若可解析 `stock < bomQty` → 剔除（与需求 §6 一致）。
- [ ] **Step 4:** `go test ./internal/biz/... -run ComparePrice -v` 全绿。
- [ ] **Step 5:** Commit  

```bash
git add internal/biz/bom_compare_price.go internal/biz/bom_compare_price_test.go
git commit -m "feat(biz): compare unit price extraction chain §1.7 §1.9"
```

---

### Task 5: 汇率换算与 `unit_price_base`（TDD）

**Files:**
- `internal/biz/bom_fx.go`
- `internal/biz/bom_fx_test.go`
- `internal/data/bom_fx_rate_repo.go`

- [ ] **Step 1:** 定义 `FXRateLookup` 接口（`biz`）：`Rate(ctx, from, to, date) (rate float64, version string, ok bool)`；**无汇率**时测试期望：明确错误 or 跳过候选（在计划中 **固定为：候选不参与自动排序并记原因**，与保守回退一致）。
- [ ] **Step 2:** 实现 `ToBaseCCY(price, fromCCY, baseCCY, date, lookup) (base float64, fxMeta, err)`；同币则 rate=1。
- [ ] **Step 3:** `bom_fx_rate_repo.go` 实现 GORM 查询 `biz_date` 最近一条或当日精确匹配（行为写死并测）。
- [ ] **Step 4:** `go test ./internal/biz/... ./internal/data/... -run Fx -v`（data 层可用 sqlite 或测试 MySQL，遵循仓库惯例）。
- [ ] **Step 5:** Commit  

```bash
git add internal/biz/bom_fx.go internal/biz/bom_fx_test.go internal/data/bom_fx_rate_repo.go
git commit -m "feat(biz,data): FX conversion for BOM match base_ccy"
```

---

### Task 6: 交期解析与排序比较器（TDD）

**Files:**
- `internal/biz/bom_lead_time_parse.go`
- `internal/biz/bom_lead_time_parse_test.go`
- `internal/biz/bom_match_sort.go`
- `internal/biz/bom_match_sort_test.go`

- [ ] **Step 1:** `ParseLeadDays(leadTime string, platformID string) (days int, ok bool)` — `N/A` → ok false；`3-5天` → 3；现货映射为 0（与 `bom_platform_quote_map` 配置表一致）。
- [ ] **Step 2:** 实现 `LessMatchCandidate(a, b MatchSortKey) bool`：`unit_price_base`（先按 `rounding_mode` 量化）→ `lead_days` → `stock` → `platform_id`。
- [ ] **Step 3:** 表测：同价不同交期时交期短者胜。
- [ ] **Step 4:** Commit  

```bash
git add internal/biz/bom_lead_time_parse.go internal/biz/bom_lead_time_parse_test.go internal/biz/bom_match_sort.go internal/biz/bom_match_sort_test.go
git commit -m "feat(biz): lead time parse and match tie-break sort §1.10"
```

---

### Task 7: 厂牌别名解析（TDD + data repo）

**Files:**
- `internal/biz/bom_mfr_normalize.go`
- `internal/biz/bom_mfr_normalize_test.go`
- `internal/data/bom_manufacturer_alias_repo.go`

- [ ] **Step 1:** `NormalizeMfrString(s string) string` — trim、全半角、大小写（规则与需求 §6「同一规范化」对齐，**先与现有 `NormalizeMPNForBOMSearch` 风格一致或抽公共 `normalize` 包**）。
- [ ] **Step 2:** `ResolveCanonical(ctx, raw string, lookup AliasLookup) (canonicalID string, hit bool)` — 严格策略：未命中 → `hit=false`（§2.3）。
- [ ] **Step 3:** Repo：`LoadAll` 内存 map 或按别名点查；**唯一索引冲突**在插入时由 DB 拒绝。
- [ ] **Step 4:** BOM 空厂牌：不调用解析，直接 **通过**（§2.5）；报价空厂牌：若 BOM 有厂牌 → **不匹配**（§2.6）。
- [ ] **Step 5:** Commit  

```bash
git add internal/biz/bom_mfr_normalize.go internal/biz/bom_mfr_normalize_test.go internal/data/bom_manufacturer_alias_repo.go
git commit -m "feat(biz,data): manufacturer alias resolution for BOM match §2"
```

---

### Task 8: 单行多平台配单编排（biz）

**Files:**
- `internal/biz/bom_line_match.go`
- `internal/biz/bom_line_match_test.go`（表驱动 + fake FX/alias）
- `internal/biz/bom_platform_quote_map.go`

- [ ] **Step 1:** 输入：`BomSessionLine` 字段（mpn/package/mfr/param/qty）、各平台 `BomQuoteCache` 行（`quotes_json`、`outcome`、`biz_date`）、`Bootstrap.BomMatch`。
- [ ] **Step 2:** 展开 `quotes_json` → 候选列表；过滤 **MPN/封装/参数** 字符串完全匹配（与需求 §6）；厂牌按 Task 7；库存/MOQ 按 Task 4。
- [ ] **Step 3:** 对每个候选算 `unit_price_base` 与排序键，取 **最优**；输出 **审计 DTO**（§3：`compare_price_field`、`fx_date`、`fx_date_source`、`original_ccy` 等）。
- [ ] **Step 4:** `go test ./internal/biz/... -run LineMatch -v`。
- [ ] **Step 5:** Commit  

```bash
git add internal/biz/bom_line_match.go internal/biz/bom_line_match_test.go internal/biz/bom_platform_quote_map.go
git commit -m "feat(biz): BOM line match orchestration over quote cache"
```

---

### Task 9: 读缓存 API + service 接入

**Files:**
- `internal/data/bom_quote_cache_repo.go`（或现有文件扩展）
- `internal/service/bom_service.go`
- `cmd/server/wire.go`、`internal/data/provider.go`、`internal/service/provider.go`

- [ ] **Step 1:** 实现按 **session_id** 拉取所有行 + 会话 `platform_ids` + `biz_date`，对每行每平台查询 `bom_quote_cache`（键：`mpn_norm`,`platform_id`,`biz_date`）；**无行**则该平台无候选。
- [ ] **Step 2:** `BomService.SearchQuotes`：若 proto 定义为实时算价，则组装 `ItemQuotes`；若与 `AutoMatch` 合并，按现有 `bom.proto` 注释实现 **最小可用** 路径。
- [ ] **Step 3:** `GetMatchResult`：映射 `bom_id` → `session_id`（与前端约定一致；若仅支持 session，返回明确错误）。
- [ ] **Step 4:** Wire 注入 `BomMatch` 配置、fx repo、alias repo、quote cache repo。
- [ ] **Step 5:** `go build -o NUL ./cmd/server/...`
- [ ] **Step 6:** Commit  

```bash
git add internal/data/bom_quote_cache_repo.go internal/service/bom_service.go cmd/server/wire.go internal/data/provider.go internal/service/provider.go cmd/server/wire_gen.go
git commit -m "feat(service): wire BOM match and implement SearchQuotes/GetMatchResult stubs"
```

---

### Task 10: 集成验证与文档

**Files:**
- `docs/superpowers/specs/2026-03-28-bom-match-ops.md`（可选）
- `web/src/...`（仅当 Task 9 扩展了 proto JSON 字段时需要）

- [ ] **Step 1:** 准备种子数据：2～3 条 `bom_manufacturer_alias`、数日 `bom_fx_rate`（CNY/USD）。
- [ ] **Step 2:** 手工或脚本：插入 `bom_quote_cache` 样例（使用 §1.11.3 字符串），调用 HTTP `SearchQuotes`/`GetMatchResult`，核对 **双币** 与 **平局交期** 行为。
- [ ] **Step 3:** 在 ops 文档中写明：**映射表** 与 **汇率** 更新流程（§4）。
- [ ] **Step 4:** Commit  

```bash
git add docs/superpowers/specs/2026-03-28-bom-match-ops.md
git commit -m "docs: BOM match ops seed and FX/alias maintenance"
```

---

## 计划评审与执行交接

1. 建议将本 plan 与 spec 路径一并交给 **plan-document-reviewer**（见 `superpowers:writing-plans` §Plan Review Loop）做一轮结构审查。  
2. 实现时 **优先完成 Task 3→8**（biz 全绿），再接 Task 9（I/O 与 Wire），降低联调面。

**Plan 已保存至 `docs/superpowers/plans/2026-03-28-bom-match-currency-mfr-implementation.md`。执行方式可选：**

1. **Subagent-Driven（推荐）** — 每 Task 独立子代理 + Task 间复核（`superpowers:subagent-driven-development`）。  
2. **本会话顺序执行** — 使用 `superpowers:executing-plans`，按 Task 勾选推进。

需要我按其中一种方式从 **Task 1** 开始落地时，直接说明即可。
