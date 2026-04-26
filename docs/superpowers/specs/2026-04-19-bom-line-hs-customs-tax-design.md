# 配单行：HS 编码、商检标记与进口关税展示设计

## 1. 目标与范围

### 1.1 目标

在 BOM 配单相关界面中，对每条会话物料行（`t_bom_session_line`）在**已有报价/配单数据加载路径**上，增加：

1. **是否商检**（来自 HS 条目监管标记）  
2. **进口关税税率**（来自单一窗口 TaxRate001 类接口，经本地日缓存降低调用频率）

两者均依赖**已解析且可信任的 HS 编码**（本设计采用 **`code_ts` 十位数字串**，与 `t_hs_model_mapping.code_ts`、`t_hs_item.code_ts` 一致）。

### 1.2 非目标

- 不替代现有 `HsResolveService.ResolveByModel` 流水线；仅定义配单侧如何**读**映射与如何**触发**解析。  
- 不在本文规定「商检」向用户展示的逐字文案（由产品/UI 将 `control_mark` 映射为可读说明即可）。  
- 不对税率接口做法律合规背书；仅缓存与展示接口返回值。

### 1.3 范围边界

- 数据表：`t_bom_session_line`、`t_hs_model_mapping`（**仅 `confirmed`**）、`t_hs_item`、`t_hs_tax_rate_daily`。  
- 外部接口：现有 `HsTaxRateAPIRepo.FetchByCodeTS`（见 `internal/data/hs_tax_rate_api_repo.go`、`docs/tax_rate_api`）。  
- HTTP：已有 `POST /api/hs/resolve/by-model` 作为「一键找 HS」入口。

---

## 2. HS 编码解析规则（配单行 → `code_ts`）

### 2.1 输入

- `mpn`：`t_bom_session_line.mpn`（非空）。  
- `mfr`：`t_bom_session_line.mfr`（可空）；**查询映射时**空指针按**空字符串**参与 `(model, manufacturer)` 查找，且 **`t_hs_model_mapping` 允许 `manufacturer = ''` 的 `confirmed` 行**（唯一键 `(model, manufacturer)` 下与「有厂牌」行并存）。`GetConfirmedByModelManufacturer` 在 **`model` 非空** 时须对 `manufacturer == ""` 仍执行 `WHERE … manufacturer = ''`（与 `HsModelMappingRepo` 实现一致）。

### 2.1.1 `mfr` 规范化与 Resolve 入参一致性（已定）

- **必须与** `POST /api/hs/resolve/by-model`（`HsResolveByModel`）解析映射时使用的 **同一套** `model` / `manufacturer` 规范化规则一致（含空 `mfr` → 空字符串、与 `NormalizeMfrString` 或等价函数的同源调用）。  
- 配单行查 `GetConfirmedByModelManufacturer` 与用户点击「一键找 HS」传参须**同源**，避免出现「Resolve 已写入映射、配单行却查不到」或相反漂移。

### 2.2 映射表查询

1. 使用 `mpn` 作为映射表 `model`，经 §2.1.1 规范化后的 `mfr` 作为 `manufacturer`。  
2. 查询 `t_hs_model_mapping`，且**仅当 `status = 'confirmed'`** 时视为命中，读取 `code_ts`。  
3. 与现有 `HsModelMappingRepo.GetConfirmedByModelManufacturer` 语义一致：**无确认记录则视为「未找到 HS」**（不读取 `pending_review` / `rejected` 用于商检与税率）。

### 2.3 输出状态（行级）

建议行级枚举（实现可用字符串常量）。**合法 `code_ts`**：非空、且为 **10 位数字**（与 `t_hs_item.code_ts` / 税率接口请求约定一致）。

| 状态 | 含义 | 后续是否查 `t_hs_item` / 税率 |
|------|------|--------------------------------|
| `hs_found` | 存在 `confirmed` 映射，且 `code_ts` 合法 | 是 |
| `hs_code_invalid` | 存在 `confirmed` 映射，但 `code_ts` 缺失或非法 | 否 |
| `hs_not_mapped` | 无 `confirmed` 映射 | 否 |

---

## 3. 商检（`ControlMark`）

### 3.1 前置条件

仅当行状态为 `hs_found` 且 `code_ts` 非空。

### 3.2 数据来源

- 表：`t_hs_item`  
- 接口：`HsItemReadRepo.GetByCodeTS(ctx, code_ts)`（已实现）。  
- 字段：`control_mark` → 领域/Proto 中可命名为 `control_mark`，展示层再映射「是否商检」。

### 3.3 未命中 `t_hs_item`

- 行为：商检字段置空或明确 `hs_item_missing`；**不阻塞**税率查询（若税率接口仍可按 `code_ts` 返回）。  
- 是否记入日志：建议 `Warn` 一次，便于数据补齐。

---

## 4. 进口关税税率与日缓存

### 4.1 前置条件

同商检：仅 `hs_found` 且 `code_ts` 合法。

### 4.2 业务日 `biz_date`

- 定义：**应用服务器本地时区**下的日历日 `DATE`（与「当天缓存」一致）。  
- 缓存主键：`(code_ts, biz_date)`。

### 4.3 读取顺序（biz 编排）

1. 按 `(code_ts, biz_date)` 查询 `t_hs_tax_rate_daily`。  
2. **命中**：直接返回表中结构化字段（见 §4.4），**不调用**外部税率接口。  
3. **未命中**：调用 `FetchByCodeTS`；解析响应中 `data.data[]` 数组；在 **`codeTs` 与请求 `code_ts` 一致** 的条目中，若有多条则**取第一条**；写入 `t_hs_tax_rate_daily` 后返回。若无任何匹配项则视为接口数据异常：不写入成功行，行上返回错误供前端提示。  
4. **落库与并发**：写入 `t_hs_tax_rate_daily` 时采用 **先查再写在同一事务内**（或等价：插入前再次确认当日键不存在）；若因并发撞到 `UNIQUE (code_ts, biz_date)` 导致插入失败，则 **放弃本次写入并重试读库**，以已存在行为准。  
5. 接口失败：不写入成功行；行上返回错误码/文案供前端提示。  
6. **外网调用频率（已定）**：**不设负缓存**（失败或不匹配不写「占位」行）。因此只要当日键未命中缓存，**该次配单加载路径即可再次调用外网**；命中日缓存则不打外网。接受由此带来的重复外呼可能（与限流、并行上限一起在实现计划中约束即可）。

### 4.4 表结构（已实现 DDL）

物理表：**`t_hs_tax_rate_daily`**。

迁移文件：`docs/schema/migrations/20260419_t_hs_tax_rate_daily.sql`。

列与 `docs/tax_rate_api` 中响应 `data.data[]` 单条对象对齐：

| 接口字段 | 表列（snake_case） |
|----------|-------------------|
| `codeTs` | `code_ts` |
| `gName` | `g_name` |
| `impDiscountRate` | `imp_discount_rate` |
| `impTempRate` | `imp_temp_rate` |
| `impOrdinaryRate` | `imp_ordinary_rate` |

唯一约束：`UNIQUE (code_ts, biz_date)`。

GORM 模型：`internal/data/hs_tax_rate_daily.go`（`HsTaxRateDaily`），并已加入 `AutoMigrateSchema`。

### 4.5 展示建议

- 主展示字段：`imp_ordinary_rate`（如 `24%`）。  
- `g_name` 可作为副标题或 tooltip。  
- 其余税率字段按需展示。

---

## 5. 前端：未找到 HS 与「一键找 HS」

### 5.1 条件（已定）

当行状态为 **`hs_not_mapped` 或 `hs_code_invalid`** 时展示「一键找 HS」入口（即：**无可用 `confirmed`+合法 `code_ts` 时均允许触发**）。

### 5.2 展示

- 文案示例：**未找到匹配的 HS 编码**（可产品定稿）；`hs_code_invalid` 可用单独文案区分「映射存在但编码异常」（产品定稿）。  
- 按钮：**一键找 HS**，点击后调用已有接口：

  - `POST /api/hs/resolve/by-model`  
  - 请求体：`HsResolveByModelRequest`，其中 `model` ← 行 `mpn`，`manufacturer` ← 行 `mfr` 经 **§2.1.1 与 Resolve 同源** 规范化（空则空字符串）。  
  - 同步/异步：遵循现有 `ResolveByModel` 语义（超时内直接带结果，否则 `task_id` 轮询）。

### 5.3 成功后刷新（已定）

用户确认或任务完成后映射可能写入 `t_hs_model_mapping`；前端在适当时机**重新拉取配单/会话数据**，使下一请求可走 §2～§4 的读库路径。

- **仍为 `hs_not_mapped` 属预期**：Resolve 可能写入 **`pending_review`**，按 §2 仍不算命中；**刷新后若仍为 `hs_not_mapped`，表示待人工确认映射**，与 UI 提示一致，**不视为前端或接口故障**。

---

## 6. API 形态（推荐）

### 6.1 推荐方案

在**现有配单结果或会话行载荷**上扩展字段（例如 `GetMatchResult` / 会话详情中与行绑定的 message），避免 N 次独立请求：

- `hs_code_status`  
- `code_ts`（有则填）  
- `control_mark`（可空）  
- `import_tax_g_name`、`import_tax_imp_ordinary_rate` 等（或嵌套 `ImportTaxQuote` 消息）  
- `hs_customs_error`（分项：映射缺失 / 编码非法 / item 缺失 / 税率接口失败；可与各展示字段并存）

### 6.1.1 分项失败与部分有值（已定）

- **允许**同一行上 **商检与税率独立**：例如 `control_mark` 有值而税率字段因外呼失败为空，或 `t_hs_item` 缺失而税率来自接口成功；前端按字段与 `hs_customs_error` 分项展示即可。

### 6.2 实现要点

- **批量**：对会话内多行一次性收集 `code_ts` 集合，批量查 `t_hs_item`、批量查 `t_hs_tax_rate_daily`，避免按行 N+1。  
- **税率未缓存行**：对「当日未缓存」的 `code_ts` 可并行限流调用外部接口（并发上限在实现计划中约定）；落库策略见 §4.3 第 4 点。

---

## 7. 分层与依赖（Kratos）

| 层级 | 职责 |
|------|------|
| `internal/service` | 将 Proto 请求转为 biz 入参；组装行级 DTO；不写业务分支决策。 |
| `internal/biz` | 定义「按行解析 HS → 商检 → 税率（含读缓存与落缓存决策）」用例；定义 `HsTaxRateDailyRepo` 等接口。 |
| `internal/data` | GORM 访问 `t_hs_tax_rate_daily`、`t_hs_item`、映射表；`HsTaxRateAPIRepo` 仅 HTTP。 |

禁止在 `data` 层实现「是否应调用外部接口」的跨资源业务决策；该决策放在 `biz`。

---

## 8. 配置与安全

- 税率接口 URL：沿用 `CAICHIP_HS_TAX_RATE_API_URL` 与 `Bootstrap` 超时配置（与 `HsTaxRateAPIRepo` 一致）。  
- 日志：禁止打印完整密钥 URL；错误日志可记录 `code_ts` 与 `biz_date`。

---

## 9. 测试建议

- 单元：`biz` 在「缓存命中 / 未命中 mock API / item 缺失」下的输出。  
- 集成：迁移表存在时，`GetByCodeTS` + 写入 `t_hs_tax_rate_daily` + 第二次读命中。  
- 回归：`confirmed` 与 `pending_review` 分支——仅 `confirmed` 出现 `code_ts` 于配单扩展字段。  
- 并发：同一 `(code_ts, biz_date)` 并行未命中后仅一条落库、其余重试读命中。

---

## 10. 文档与迁移索引

| 类型 | 路径 |
|------|------|
| 税率接口样例 | `docs/tax_rate_api` |
| 日缓存 DDL | `docs/schema/migrations/20260419_t_hs_tax_rate_daily.sql` |
| HS 型号→编码（背景） | `docs/superpowers/specs/2026-04-15-hs-model-to-code-ts-design.md` |

---

## 11. 修订记录

| 日期 | 说明 |
|------|------|
| 2026-04-19 | 初稿：配单行 HS、商检、关税日缓存与「一键找 HS」交互 |
| 2026-04-19 | 评审收口：`hs_code_invalid`；一键找 HS 覆盖 `hs_not_mapped`/`hs_code_invalid`；刷新仍 `hs_not_mapped`=待确认；mfr 与 Resolve 同源；日缓存先事务/重试读、多匹配取首条；允许分项部分成功；不设负缓存故未命中可重复打外网 |
| 2026-04-19 | 映射仓储：`GetConfirmedByModelManufacturer` / `Save` 支持空厂牌（`manufacturer = ''`）的 `confirmed` 行；§2.1 与实现一致 |
