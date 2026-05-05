# BOM 厂牌清洗：两阶段评审 — 技术方案

## 1. 文档定位

- 落实产品需求：`2026-05-04-bom-mfr-cleaning-two-phase-requirements.md`
- **软件需求规格（SRS，验收口径）**：`2026-05-04-bom-mfr-cleaning-two-phase-software-requirements-spec.md`
- **开发计划**：`docs/superpowers/plans/2026-05-04-bom-mfr-two-phase-implementation.md`（文末 **「收尾状态（Phase 6）」** 记录废弃接口在代码中的移除与核对结论）
- 替代旧实现思路：**不再**使用「`ListManufacturerAliasCandidates` 混合 demand/quote + `ApproveManufacturerAliasCleaning` 内 `BackfillSessionManufacturerCanonical` 同时写 `session_line` 与 `quote_item`」作为正式路径（产品未上线，可直接替换）。
- 与字段含义总述的关系：`2026-04-18-manufacturer-canonicalization-design.md` 仍可参考别名表与规范化；**报价侧「待清洗」语义**由本方案中 `manufacturer_review_status` 与阶段二规则承接并细化。

---

## 2. 数据模型

### 2.1 `t_bom_session_line`

- 继续使用 `mfr`（需求厂牌原文）、`manufacturer_canonical_id`（需求侧规范 ID）。
- 阶段一仅写入本表（及别名表），**不在本接口内**更新 `t_bom_quote_item`。

### 2.2 `t_bom_quote_item`（新增与语义）

| 字段 | 类型建议 | 说明 |
|------|----------|------|
| `manufacturer_review_status` | `ENUM` 或 `TINYINT` + 常量 | `pending`（默认）、`accepted`、`rejected` |
| `manufacturer_canonical_id` | 已有 | **仅当 `accepted` 时**与父行 `manufacturer_canonical_id` **相等**；`rejected` 时为 `NULL`；`pending` 为未决 |
| `manufacturer_review_reason` | `TEXT` 可选 | 不通过原因 |
| `manufacturer_reviewed_at` | `DATETIME(3)` 可选 | 审计 |

**不变量（须在 `biz` 校验 + 测试断言）**

- `accepted` ⇒ `manufacturer_canonical_id` 非空且等于父行 `manufacturer_canonical_id`。
- `rejected` ⇒ `manufacturer_canonical_id IS NULL`。
- `pending` ⇒ 不参与「厂牌已确认可用」的报价集合。

### 2.3 `t_bom_manufacturer_alias`

- 阶段一写入规则与现网一致：`alias` / `alias_norm` / `canonical_id` / `display_name`；冲突策略不变。

---

## 3. 阶段闸门（机判）

定义 **需要需求侧规范厂牌** 的行：

- `normalize(mfr) != ''` **且**（业务若另有「豁免列」再扩展）。

**闸门打开**当且仅当：会话内所有「需要需求侧规范厂牌」的行均满足 `manufacturer_canonical_id IS NOT NULL`。

**`mfr` 为空（或规范化后为空）的行**

- 不计入「待完成」集合；**不阻塞**闸门。
- 其下报价：产品默认 **厂牌维度全部通过** —— 实现二选一（定一种写死）：

  - **读模型优先（实现最简）**：父行 `mfr` 为空时，配单/评审逻辑将子报价视为厂牌已通过，**不**进入阶段二列表；可不写 `manufacturer_review_status`。
  - **状态可审计**：在闸门计算通过或定时任务中，将该行下 quote 批量置 `manufacturer_review_status=accepted`，`manufacturer_canonical_id` 保持 `NULL`（父行也无 canonical）。

---

## 4. 阶段一：需求行 — 候选、审批、落库

### 4.1 候选入队

- 输入：会话 ID、当前勾选平台等（与现会话视图一致）。
- 规则：仅 **需求行** —— `mfr` 非空且 `manufacturer_canonical_id` 为空（或产品允许「覆盖重审」时再包含已填行，默认不包含）。
- **不**扫描报价 JSON 生成阶段一列表。

### 4.2 审批请求

- Body 至少包含：目标行标识（`line_id` 或 `session_id+line_no`）、厂牌原文 `alias`、`canonical_id`、`display_name`（与写别名表字段一致）。
- 服务层：`service` 校验 → `biz` 决策（别名冲突、行锁定等）→ `data` GORM 写别名表 + **仅更新对应 session_line**。

### 4.3 删除/替换的旧行为

- **删除**或收窄 `BomManufacturerCleaningRepo.BackfillSessionManufacturerCanonical` 中「按 `alias_norm` 同步更新所有 `quote_item`」的逻辑；阶段一 **不得**调用该双表路径。
- 若保留函数名，另增 `BackfillSessionLineManufacturerCanonical`（仅行）更清晰。

---

## 5. 阶段二：报价明细 — 列表、通过、不通过、改判

### 5.1 列表入队（闸门已开）

对每个 `t_bom_quote_item`（本会话、能关联到父行、且在勾选平台等业务范围内）：

- 父行 `manufacturer_canonical_id` 非空（父行 `mfr` 为空已在 §3 整行跳过，不进本列表）。
- 报价 `manufacturer` 非空。
- `manufacturer_review_status = pending`（若允许改判后回到 pending，再定义；默认改判直接 `accepted`/`rejected` 切换）。
- **与父行规范不一致或尚未确认**：与现 `collectLineManufacturerAliasCandidates` 中 quote 分支的「解析 canonical 与需求 canonical 比较」思路对齐，但 **需求 canonical 以父行字段为准**（不再与「仅行上陈旧 canonical」纠缠的细节由 `biz` 单测覆盖）。

列表项需返回：`quote_item_id`、报价厂牌原文、平台、`line_no` / `line_id`、**只读** `line_manufacturer_canonical_id`（及可选 `display_name`）。

### 5.2 提交 `accept`

- 校验父行 `manufacturer_canonical_id` 仍非空。
- 更新：`manufacturer_review_status=accepted`，`manufacturer_canonical_id = 父行.manufacturer_canonical_id`。

### 5.3 提交 `reject`

- 更新：`manufacturer_review_status=rejected`，`manufacturer_canonical_id=NULL`，可选 `reason`、`reviewed_at`。

### 5.4 改判

- 同一 `quote_item_id` 允许再次 `POST`：`rejected` ↔ `accepted` 均允许（与产品需求 R4.1/R4.2 一致）。
- **每次**改判均重新校验父行 canonical；`accept` 时始终 **拷贝当前父行** canonical 到 quote。

---

## 6. API 形态（建议，可直接替换旧接口）

以下为 REST 形态示例；若工程统一走 gRPC/proto，则映射为等价 RPC。

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/api/v1/bom-sessions/{session_id}/session-line-mfr-candidates` | 阶段一候选 |
| `POST` | `/api/v1/bom-sessions/{session_id}/session-line-mfr-approvals` | 阶段一提交 |
| `GET` | `/api/v1/bom-sessions/{session_id}/quote-item-mfr-reviews` | 阶段二列表；响应体须含 `gate_open`（见 SRS REQ-API-003） |
| `POST` | `/api/v1/bom-sessions/{session_id}/quote-item-mfr-reviews` | 阶段二 accept/reject/改判 |

**废弃（不保留兼容）**

- `GET .../manufacturer-alias-candidates`（混合列表）
- `POST .../manufacturer-alias-approvals`（双表回填）

> **实现状态（2026-05-04）：** 仓库内运行代码已无上述旧 HTTP 能力；收尾核对与「归档文档仍含旧名」的说明见 [`2026-05-04-bom-mfr-two-phase-implementation.md`](../plans/2026-05-04-bom-mfr-two-phase-implementation.md) 文末 **「收尾状态（Phase 6）」**。

`POST .../manufacturer-aliases/apply`（应用已有别名）：语义收缩为 **仅对 session_line** 尝试按别名表补全 canonical；**不写** `quote_item`，避免与阶段二职责冲突。

---

## 7. 配单与读模型（biz 约定）

- 报价行参与「厂牌已对齐」判断时：
  - 父行 `mfr` 为空：**不**要求 quote 做任何厂牌确认；视为无厂牌约束（与旧设计一致）。
  - 父行 `mfr` 非空：仅 `manufacturer_review_status=accepted` 且 `manufacturer_canonical_id` 等于父行时，视为报价厂牌维度可用；`rejected` 排除；`pending` 视为未就绪（是否阻塞会话就绪另与 `data_ready` 规则对齐，建议 **阻塞** 直至阶段二处理或产品定义自动策略）。

---

## 8. 分层与实现位置（Kratos）

- `internal/biz`：闸门、入队条件、accept/reject 不变量、改判校验；禁止把状态机散落在 `data`。
- `internal/data`：GORM 更新行/quote_item/别名表；纯按 ID 或条件更新。
- `internal/service`：HTTP/gRPC 绑定、参数校验、错误码。
- `internal/server`：路由注册。

---

## 9. 数据库迁移

- 新建 migration：`t_bom_quote_item` 增加 `manufacturer_review_status`（默认 `pending`）及可选 `reason` / `reviewed_at`。
- 存量数据（若有）：已有 `manufacturer_canonical_id` 且与父行一致的 quote 可批量标为 `accepted`；不一致或孤儿数据走一次性脚本（未上线环境可省略）。

---

## 10. 前端

- `SessionDataCleanPanel`（或等价页）拆为两块 UI：**需求行厂牌** → **报价厂牌确认**；第二块依赖闸门 API 或会话派生状态。
- 第二块：只读父行 canonical + 通过/不通过；无下拉。

---

## 11. 测试清单（摘要）

- 阶段一后：仅 `session_line` + 别名表变化，**quote_item** 无隐式更新。
- 闸门：`mfr` 非空且缺 canonical 时阶段二不可用；**全部** `mfr` 为空会话闸门可开且阶段二无父行有 canonical 的待办（或仅其他行有待办）。
- `accept`：quote canonical **等于** 父行当前值。
- `reject`：canonical 为空，`status=rejected`。
- 改判：`rejected`→`accept`、`accept`→`reject` 后状态与不变量成立。

---

## 12. 修订记录

| 日期 | 说明 |
|------|------|
| 2026-05-04 | 初稿：数据字段、API 替换、闸门、空 mfr、改判、分层与迁移 |
| 2026-05-04 | 关联 SRS；阶段二 GET 定稿为响应体 `gate_open` |
