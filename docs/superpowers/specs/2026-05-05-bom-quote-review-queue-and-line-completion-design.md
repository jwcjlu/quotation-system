# BOM 报价评审：默认队列（TopN）与会话行结案规则（B）— 设计总览

## 0. 文档状态与文档地图

| 项 | 说明 |
|---|------|
| 状态 | **草案**，供评审；未绑定具体迭代与排期 |
| 日期 | 2026-05-05 |
| 类型 | **设计总览**（架构意图、关键取舍、文档编排）；**非**唯一权威条文 |

**分卷规格（请实现与验收优先引用）**：

| 文档 | 职责 |
|------|------|
| [2026-05-05-bom-quote-review-queue-and-line-completion-requirements.md](./2026-05-05-bom-quote-review-queue-and-line-completion-requirements.md) | 产品需求（WHAT）：目标、场景、R* 功能需求、验收走读、评审待定清单 |
| [2026-05-05-bom-quote-review-queue-and-line-completion-software-requirements-spec.md](./2026-05-05-bom-quote-review-queue-and-line-completion-software-requirements-spec.md) | 软件需求规格（SRS）：E/S/TopK/规则 B 的 SHALL、边界、重算、**`data_ready` 读模型占位（§8）**、REQ-* 与验证 |
| [../plans/2026-05-05-bom-quote-review-queue-and-line-completion-implementation.md](../plans/2026-05-05-bom-quote-review-queue-and-line-completion-implementation.md) | 开发计划：TDD 契约、Phase 0～6、REQ/V 追溯、与厂牌 P4-2 / `data_ready` 衔接 |

本文档保留 **问题背景、方案直觉、关键设计取舍与跨文档关系**；**可测试的完整条文**以 SRS 为准，避免三处重复维护。

---

## 1. 背景与问题

在多平台搜索同一需求行（`t_bom_session_line`）时，会产生大量 `t_bom_quote_item`（报价行）。即便订单规模不大，**待审报价行条数**仍可能达到数百至上千，运营在「厂牌/报价确认」上耗时过长。

**业务共识**：在型号一致且其它可比参数满足约束的前提下倾向价格最优；以**确定性规则 + 人工在收窄集合上确认**为主，不依赖大模型自动通过/拒绝。

---

## 2. 方案总览（直觉）

### 2.1 双轨：TopN 与 TopK

- **TopN（默认 N=5）**：解决「**先审哪几条**」——按可比价升序的默认优先队列，**仅为 UI/展示优先级**，不删除数据。
- **TopK（K=min(3,m)）**：解决「**底价带是否已厂牌确认**」——规则 B 用当前候选池 S 内最便宜的一档（最多 3 条）是否**全部** `accepted`。

二者 **独立**，不要求 N=K。工作台须让用户感知 TopK 与 TopN 的差异（见需求 R1.3、SRS REQ-UI-001）。

### 2.2 候选池为什么要排除 `rejected`

若把已否决行仍算进「全池最低价」，会出现「最低价落在已否决行上」与「底价带已确认」**语义自相矛盾**。故 **S = E 上 pending ∪ accepted**，`rejected` 不参与比价与 TopK（详见 SRS REQ-POOL-002）。

### 2.3 参与比价集合 E

在厂牌状态之前，先用 **E** 表示「仍允许参与比价」的行（硬约束：数据错误、不可交付、禁售等剔除）。**S ⊆ E**，避免「不可比价却 pending」挤占 TopK 或污染结案（SRS REQ-POOL-001）。

### 2.4 规则 B 与空候选

**m=|S|=0** 时规则 B 必须为假，**禁止**空 TopK 真空通过。无可用报价时若需人工完结，走**单独终态**，不得套用规则 B 给绿灯（SRS REQ-RULE-B-001）。

### 2.5 「结案」语义边界

规则 B 表示 **S 内底价带厂牌已全部通过**，**不表示**本行每一条库内报价均已处理。若需「行级全清」须另规（需求 §2.2）。

---

## 3. 与厂牌两阶段方案的关系

- `2026-05-04-bom-mfr-cleaning-two-phase-*.md` 定义单条报价的 `manufacturer_review_status` 与阶段闸门。
- **本文档族**解决：审阅**优先级（TopN）**与**单行是否满足「底价带已确认」（规则 B）**；不替代厂牌清洗职责。
- 将来若结案扩展为「厂牌 ∧ 交期 ∧ …」，**S** 应取各维度「仍可参与比价」与 **E** 的交集（单独变更请求）。

---

## 4. 开放问题（索引）

边界、动态回退、M1/M2、B-aux、过期联动、同价多样性等 **产品待定项** 见 **需求文档 §7**；**工程 SHALL** 见 **SRS** 对应 REQ-* 与 **§6、§8**。

---

## 5. 关联文档

- 产品需求：`2026-05-05-bom-quote-review-queue-and-line-completion-requirements.md`
- SRS：`2026-05-05-bom-quote-review-queue-and-line-completion-software-requirements-spec.md`
- 实现计划：`../plans/2026-05-05-bom-quote-review-queue-and-line-completion-implementation.md`
- 厂牌两阶段需求：`2026-05-04-bom-mfr-cleaning-two-phase-requirements.md`
- 厂牌两阶段设计：`2026-05-04-bom-mfr-cleaning-two-phase-design.md`
- 厂牌两阶段 SRS：`2026-05-04-bom-mfr-cleaning-two-phase-software-requirements-spec.md`
- Specs 索引：`README.md`
