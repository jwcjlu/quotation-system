# BOM 厂牌两阶段清洗 — 软件需求规格说明书（SRS）

## 文档控制

| 项 | 内容 |
|----|------|
| 文档标识 | SRS-BOM-MFR-2PHASE-20260504 |
| 状态 | 草案（与实现同步迭代） |
| 依据设计 | `2026-05-04-bom-mfr-cleaning-two-phase-design.md` |
| 关联产品说明 | `2026-05-04-bom-mfr-cleaning-two-phase-requirements.md` |
| 开发计划 | `docs/superpowers/plans/2026-05-04-bom-mfr-two-phase-implementation.md`（文末「收尾状态（Phase 6）」汇总废弃接口自代码移除的核对结论） |

本文档为**实现与验收**用的需求规格：对数据、接口、行为、不变量给出可测试表述；设计文档中的架构与分层说明仍以设计为准，SRS 通过「追溯」列引用其章节。

---

## 1. 引言

### 1.1 目的

规定 BOM 工作台「数据清洗」中 **厂牌治理** 的软件行为：分两阶段处理 `t_bom_session_line` 与 `t_bom_quote_item`，消除混合评审与双表同时回填带来的复杂度。

### 1.2 范围

**在内**

- 阶段一：需求行厂牌候选、审批、别名表写入、仅更新需求行。
- 阶段闸门：机判是否允许阶段二。
- 阶段二：报价明细待评审列表、通过/不通过、改判。
- 「应用已有别名」语义收缩为仅需求行（与设计一致）。
- 废弃旧混合候选与旧审批双表回填接口（不保留兼容）。

**可另文规定（本 SRS 仅约束读模型接口）**

- `data_ready` 是否必须阶段二清零待办；建议阻塞直至 `pending` 处理完毕（见设计 §7），以接口字段支撑即可。

### 1.3 定义与缩写

| 术语 | 含义 |
|------|------|
| canonical | 规范厂牌 ID，字段 `manufacturer_canonical_id` |
| 阶段一 | 需求行厂牌清洗 |
| 阶段二 | 报价明细厂牌通过性评审 |
| 闸门 | 允许进入阶段二列表/提交的机判条件 |
| 改判 | 对同一 `quote_item` 重复提交 `accept`/`reject` |

规范化函数：需求厂牌原文规范化与现网 `biz.NormalizeMfrString`（或等价实现）一致，下文记为 `norm_mfr(text)`。

---

## 2. 引用与追溯

| 设计文档章节 | SRS 需求组 |
|--------------|------------|
| 设计 §2 数据模型 | REQ-DATA-* |
| 设计 §3 闸门 | REQ-GATE-* |
| 设计 §4 阶段一 | REQ-S1-* |
| 设计 §5 阶段二 | REQ-S2-* |
| 设计 §6 API | REQ-API-* |
| 设计 §7 读模型 | REQ-RM-* |
| 设计 §8 分层 | REQ-ARCH（约束） |
| 设计 §9 迁移 | REQ-MIG-* |
| 设计 §10 前端 | REQ-UI-* |

---

## 3. 数据需求

### REQ-DATA-001 需求行字段

系统 SHALL 使用 `t_bom_session_line.mfr`（可空）、`manufacturer_canonical_id`（可空）表达需求侧原文与规范 ID；阶段一 SHALL 仅通过审批接口更新目标行的 `manufacturer_canonical_id`（及别名表），SHALL NOT 在同一事务内为「阶段一」目的批量更新 `t_bom_quote_item`。

### REQ-DATA-002 报价明细评审状态

系统 SHALL 在 `t_bom_quote_item` 上持久化 `manufacturer_review_status`，取值为 `pending`（默认）、`accepted`、`rejected`。

### REQ-DATA-003 报价明细可选审计字段

系统 SHOULD 支持 `manufacturer_review_reason`（TEXT）、`manufacturer_reviewed_at`（DATETIME(3)）；若首期省略，SHALL 在接口层保留扩展位（可选 body 字段）。

### REQ-DATA-004 不变量（强制）

对任意 `t_bom_quote_item` 行，系统在每次提交成功后 SHALL 满足：

- `manufacturer_review_status = accepted` ⇒ `manufacturer_canonical_id` 非空，且等于其父 `t_bom_session_line.manufacturer_canonical_id`（同一业务关联关系下）。
- `manufacturer_review_status = rejected` ⇒ `manufacturer_canonical_id IS NULL`。
- `manufacturer_review_status = pending` ⇒ 不将本条计入「厂牌已确认可用」集合（见 REQ-RM-*）。

### REQ-MIG-001 数据库迁移

系统 SHALL 提供 schema migration：为 `t_bom_quote_item` 增加 `manufacturer_review_status`（默认 `pending`）；可选列按 REQ-DATA-003。

---

## 4. 业务规则需求

### REQ-GATE-001「需要规范厂牌」的需求行

定义集合 **LinesNeedCanon**：会话内满足 `norm_mfr(mfr) != ''` 的 `t_bom_session_line`。

### REQ-GATE-002 闸门打开条件

闸门 **Open** 当且仅当：对会话内所有 `L ∈ LinesNeedCanon`，`L.manufacturer_canonical_id` 非空（去空白后）。

### REQ-GATE-003 `mfr` 为空的需求行

若 `norm_mfr(mfr) == ''`：该行 SHALL NOT 属于 **LinesNeedCanon**；SHALL NOT 出现在阶段一候选列表；SHALL NOT 阻塞 REQ-GATE-002。

### REQ-GATE-004 父行 `mfr` 为空时其下报价

对父行满足 `norm_mfr(mfr) == ''` 的 `t_bom_quote_item`：系统 SHALL 在厂牌评审维度视为已通过（不进入阶段二待办列表）；配单读模型 SHALL 不强制要求用户对该类报价做通过/不通过。实现可采用设计 §3「读模型优先」或「批量 `accepted`」之一，**须在接口文档与发布说明中固定一种**。

### REQ-S1-001 阶段一候选入队

阶段一 GET 返回的每条记录 SHALL 对应一条需求行，且满足：`norm_mfr(mfr) != ''` 且 `manufacturer_canonical_id` 为空（首期默认不允许「覆盖重审」已填 canonical，若产品开启则单列 REQ）。

### REQ-S1-002 阶段一不得依赖报价 JSON 构列表

阶段一候选 SHALL NOT 以报价缓存 JSON 扫描结果作为列表来源。

### REQ-S2-001 阶段二候选入队（闸门已开）

当闸门 Open 时，阶段二列表中的每条 `quote_item` SHALL 同时满足：

- 可关联到父需求行，且父行 `norm_mfr(mfr) != ''`；
- 父行 `manufacturer_canonical_id` 非空；
- 报价 `norm_mfr(manufacturer) != ''`；
- `manufacturer_review_status = pending`（改判后直接切换状态，不要求回到 `pending`，除非产品另定）；
- 业务规则判定为「与父行规范不一致或尚未确认」——与现 `collectLineManufacturerAliasCandidates` 中报价分支意图对齐，且 **父行 canonical 以 `t_bom_session_line.manufacturer_canonical_id` 为准**（设计 §5.1）。

### REQ-S2-002 改判

系统 SHALL 允许对同一 `quote_item_id` 多次调用阶段二 POST：`rejected`→`accepted`、`accepted`→`rejected` 均须支持；每次 `accept` SHALL 将 `quote_item.manufacturer_canonical_id` 更新为 **当前**父行 `manufacturer_canonical_id`。

---

## 5. 接口需求

### REQ-API-001 阶段一查询

提供 `GET /api/v1/bom-sessions/{session_id}/session-line-mfr-candidates`（或 gRPC 等价），返回阶段一候选列表及校验所需字段（至少含：`line_id` 或 `line_no`、`mfr` 原文、推荐 canonical 等，以实现为准）。

### REQ-API-002 阶段一提交

提供 `POST /api/v1/bom-sessions/{session_id}/session-line-mfr-approvals`，Body SHALL 包含：目标行标识、`alias`、`canonical_id`、`display_name`（与设计 §4.2 一致）。成功响应 SHALL 反映行与别名表更新结果；SHALL NOT 隐式修改 `t_bom_quote_item` 的 canonical 以完成阶段一。

### REQ-API-003 阶段二查询与闸门可机读

提供 `GET /api/v1/bom-sessions/{session_id}/quote-item-mfr-reviews`。

系统 SHALL 在响应体中返回可机读的 **`gate_open: boolean`**（或语义等价字段），含义与 REQ-GATE-002 一致。

系统 SHALL 区分：

- `gate_open == false`：阶段二未开放；`items` 可为空数组。
- `gate_open == true` 且 `items` 为空：无待评审报价。
- `gate_open == true` 且 `items` 非空：有待办。

系统 SHALL NOT 仅依赖「空列表」表示未开闸（避免前端无法区分 REQ-API-003 的三种情况）。

### REQ-API-004 阶段二提交

提供 `POST /api/v1/bom-sessions/{session_id}/quote-item-mfr-reviews`，Body SHALL 包含：`quote_item_id`、`decision`（`accept` | `reject`）、可选 `reason`。须满足 REQ-DATA-004 与 REQ-S2-002。

### REQ-API-005 废弃接口

系统 SHALL 移除或不再对外注册以下正式能力（未上线可不保留兼容期）：

- `GET .../manufacturer-alias-candidates`
- `POST .../manufacturer-alias-approvals`

> **实现状态（2026-05-04）：** 上述路径已在运行代码（Go/TS）与对外路由中移除；全仓核对结论与归档说明见 [开发计划 · 收尾状态（Phase 6）](../plans/2026-05-04-bom-mfr-two-phase-implementation.md)。客户端或脚本若仍引用旧 URL，须迁移至 REQ-API-001～004 所列新路径。

### REQ-API-006 应用已有别名

`POST .../manufacturer-aliases/apply`（或等价路径）SHALL 仅对 `t_bom_session_line` 按别名表补全 canonical；SHALL NOT 写入 `t_bom_quote_item.manufacturer_canonical_id` 作为该操作的语义组成部分。

---

## 6. 读模型与配单（biz 对外约束）

### REQ-RM-001 父行无需求厂牌

当父行 `norm_mfr(mfr) == ''`：REQ-GATE-004 成立；配单 SHALL 不按「需求厂牌约束」卡该父行关联报价的厂牌对齐（与 `2026-04-18-manufacturer-canonicalization-design.md` 一致）。

### REQ-RM-002 父行有需求厂牌

当父行 `norm_mfr(mfr) != ''`：报价行 SHALL 仅在 `manufacturer_review_status=accepted` 且 `manufacturer_canonical_id` 等于父行 canonical 时，计入「厂牌已确认可用」；`rejected` SHALL 排除；`pending` SHALL 不计入已确认可用集合。是否据此阻塞会话 `data_ready` 由会话就绪规则配置化，但 SHALL 暴露可查询的待办计数或列表以支撑 UI。

---

## 7. 架构约束（非功能）

### REQ-ARCH-001 分层

闸门判定、入队条件、accept/reject 不变量、改判合法性 SHALL 位于 `internal/biz`（或等价领域层）；`internal/data` SHALL 仅执行持久化；禁止在 `data` 层实现带业务分支的「是否可提交阶段二」决策。

---

## 8. 前端需求

### REQ-UI-001 数据清洗布局

数据清洗界面 SHALL 分为两块：**需求行厂牌**、**报价厂牌确认**；顺序固定为先第一块后第二块。

### REQ-UI-002 闸门与禁用

当 `gate_open == false` 时，第二块 SHALL 禁用或展示明确说明，且 SHALL 使用 REQ-API-003 的 `gate_open`，不得仅根据空列表禁用。

### REQ-UI-003 阶段二交互

第二块 SHALL 只读展示父行 `manufacturer_canonical_id`（及展示名若有）；SHALL NOT 提供 canonical 下拉或手写报价 canonical；SHALL 提供通过、不通过（及可选原因）。

---

## 9. 验收与测试需求

### REQ-TEST-001 阶段一隔离

执行阶段一审批成功后，对未参与该操作的 `quote_item` 抽样断言：其 `manufacturer_canonical_id` 与 `manufacturer_review_status` 不因该请求发生预期外变更（与 REQ-API-002 一致）。

### REQ-TEST-002 闸门与空 `mfr`

构造会话仅含 `mfr` 为空的行：闸门 SHALL Open（按 REQ-GATE-002/003）；阶段二列表 SHALL 不包含父行 `mfr` 为空的 quote 待办（REQ-GATE-004）。

### REQ-TEST-003 accept/reject 不变量

自动化测试 SHALL 覆盖 REQ-DATA-004 的三条不变量及 REQ-S2-002 改判路径。

---

## 10. 需求追溯矩阵（节选）

| SRS ID | 设计文档 |
|--------|----------|
| REQ-DATA-001～004 | §2 |
| REQ-GATE-001～004 | §3 |
| REQ-S1-001～002 | §4 |
| REQ-S2-001～002 | §5 |
| REQ-API-001～006 | §6 |
| REQ-RM-001～002 | §7 |
| REQ-ARCH-001 | §8 |
| REQ-MIG-001 | §9 |
| REQ-UI-001～003 | §10 |
| REQ-TEST-001～003 | §11 |

---

## 11. 修订记录

| 日期 | 版本 | 说明 |
|------|------|------|
| 2026-05-04 | 0.1 | 初版 SRS；定稿 REQ-API-003 `gate_open` 区分未开闸与无待办 |
