# BOM 报价评审：默认队列与需求行结案 — 软件需求规格说明书（SRS）

## 文档控制

| 项 | 内容 |
|----|------|
| 文档标识 | SRS-BOM-QUOTE-REVIEW-20260505 |
| 状态 | 草案（与产品评审同步迭代） |
| 依据设计 | `2026-05-05-bom-quote-review-queue-and-line-completion-design.md` |
| 关联产品需求 | `2026-05-05-bom-quote-review-queue-and-line-completion-requirements.md` |
| 关联厂牌 SRS | `2026-05-04-bom-mfr-cleaning-two-phase-software-requirements-spec.md`（`manufacturer_review_status` 语义） |

本文档对 **参与比价集合 E、候选池 S、TopN 队列、规则 B 结案布尔、重算与审计** 给出可测试表述；与 `data_ready` 的耦合见 **§8**；实现任务拆解见 `docs/superpowers/plans/2026-05-05-bom-quote-review-queue-and-line-completion-implementation.md`。

---

## 1. 引言

### 1.1 目的

规定单条 `t_bom_session_line` 下报价评审的：**默认低价 TopN 排序展示**、**规则 B 结案判定**、**边界情况（空池、缺价、动态变化）** 的不变量与重算口径，供后端/前端实现与自动化验收引用。

### 1.2 范围

**在内**

- 可比价 `compare_price` 的排序与稳定次序键。
- 参与比价集合 **E**、候选池 **S**、底价带 **TopK** 与集合 **T** 的形式化定义。
- 规则 B 布尔判定（含 **m=0** 禁止真空通过）。
- TopN 与 TopK 独立参数；可选 B-aux-1 / B-aux-2（由配置或编译期常量锁定其一）。
- 状态变更后的重算权威口径；结案回退的幂等/补偿占位。
- 工作台对 TopK 的可见区分（与产品需求 R1.3 对齐）。

**可另文规定**

- 具体 RPC/REST 路径、分页、消息体字段的最终命名（§9 给出占位语义）。
- `compare_price` 由多平台字段合成的换算公式（本 SRS 只要求存在稳定标量及缺失策略）。

### 1.3 定义

| 术语 | 含义 |
|------|------|
| E | 参与比价集合（硬约束通过，与 `manufacturer_review_status` 正交） |
| S | 候选池：E 上状态为 `pending` 或 `accepted` 的报价行子集 |
| m | \|S\| |
| K | min(3, m) |
| T | S 按 §3.3 排序后的前 K 条（允许同价并列，次序稳定） |
| 规则 B | 本 SRS §4 定义的「本行报价评审维度可结案」布尔判定 |

---

## 2. 引用与追溯

| 设计文档章节 | SRS 需求组 |
|--------------|------------|
| 设计 §3 概念 | REQ-DEF-*、REQ-POOL-* |
| 设计 §4.1 TopN | REQ-QUEUE-* |
| 设计 §4.2 规则 B | REQ-RULE-B-* |
| 设计 §6 边界 | REQ-EDGE-*、REQ-RECALC-* |
| 设计 §6.7 | REQ-AUDIT-* |
| 产品需求 §4 | REQ-UI-*（与 R1.3 对齐） |
| 本文档 §8 与 `data_ready` | REQ-READY-* |

---

## 3. 数据与定义需求

### REQ-DEF-001 可比价 `compare_price`

系统 SHALL 对同一 `t_bom_session_line` 下的各 `t_bom_quote_item` 维护或计算用于排序的标量 `compare_price`，其口径为 **同一数量梯度、币种、税运** 下的可比价格。

系统 SHALL 使用稳定全序：主键 `compare_price` 升序，次序键 SHALL 为稳定字段（建议 `updated_at` 或 `id` 升序），使得 **T 在相同库内容下唯一确定**。

### REQ-DEF-002 可比价缺失策略（待定枚举）

系统 SHALL 支持且仅实现以下策略之一（由配置锁定，与产品需求 R3.1/R3.2 一致）：

- **M1**：缺失 `compare_price` 的报价行 **不得进入 S**（直至补全或标无效并移出 E）。
- **M2**：存在任一进入 E 且状态为 `pending`/`accepted` 但缺失 `compare_price` 的报价行时，**规则 B 判定为不可结案**，并 SHALL 在待办中高亮（交互细节可另文）。

### REQ-POOL-001 参与比价集合 E

系统 SHALL 定义集合 **E** ⊆（该需求行下全部报价行），表示「仍允许参与比价」的行。典型排除原因包括但不限于：数据明显错误、无库存/不可交付、禁售渠道、作废/无效等（具体条件由业务配置或硬编码清单维护，**须文档化**）。

系统 SHALL NOT 将「已判定不可参与比价」但库内仍为 `pending` 且未修正状态的行纳入 **S**，除非产品另行批准并在文档中显式声明（与设计「禁止结案与待办语义分裂」一致）。

### REQ-POOL-002 候选池 S

在 M1 下：仅当 `compare_price` 已存在时，方允许该行进入 S 的候选（与 REQ-DEF-002 一致）。

系统 SHALL 定义：

\[
S = \{ q \in E \mid \texttt{manufacturer\_review\_status}(q) \in \{\texttt{pending},\ \texttt{accepted}\} \}
\]

`rejected` 及与产品统一的其它终态 SHALL NOT 属于 S。

### REQ-POOL-003 改判

若产品允许 `accepted` ↔ `rejected` 变更，系统 SHALL 在每次成功改判落库后 **重新计算** E（若 E 依赖可变质字段）、S、m、K、T 与规则 B 布尔。

---

## 4. 规则 B 需求

### REQ-RULE-B-001 空候选（m=0）

令 \(m = |S|\)。当 **m = 0** 时，系统 SHALL 判定规则 B 为 **假**（不可结案）。

系统 SHALL NOT 将 \(T = \emptyset\) 解释为「TopK 全部已通过」。

若产品需要「无可用报价时人工完结本条」，系统 SHALL 使用**单独终态或单独闸门**（如「无候选—已确认放弃」），SHALL NOT 使用本节规则 B 直接输出真。

### REQ-RULE-B-002 TopK 与 T

当 m ≥ 1 时，系统 SHALL 令 \(K = \min(3, m)\)，并将 S 按 REQ-DEF-001 全序排序后取前 K 条为集合 **T**。

### REQ-RULE-B-003 结案条件（主规则）

当 m ≥ 1 时，系统 SHALL 将规则 B 为真定义为 **T 中每一行的 `manufacturer_review_status` 均为 `accepted`**，且满足 REQ-RULE-B-004 中选定的辅助条件。

### REQ-RULE-B-004 辅助条件（二选一锁定）

系统 SHALL 实现且仅实现以下之一（由配置或常量锁定）：

- **B-aux-1**：若 m ≥ 3，则该需求行下（全库内，或限于 E，**须与产品确认后写死同一口径**）`manufacturer_review_status = accepted` 的条数 ≥ 3；若 m < 3，则 SHALL 退化为「S 中每一条均为 accepted」或与产品确认的豁免策略一致（见 REQ-EDGE-002）。
- **B-aux-2**：不施加 accepted 条数下限，仅以 REQ-RULE-B-003 为准。

**推荐默认**：B-aux-2（与产品需求评审一致）。

---

## 5. TopN 队列需求

### REQ-QUEUE-001 TopN 计算

系统 SHALL 在 E 上（若 M1，则仅含已具备 `compare_price` 的行）按 REQ-DEF-001 全序取前 **N** 条作为默认优先队列，**N** SHALL 可配置，默认 **5**。

### REQ-QUEUE-002 TopN 与 TopK 独立性

系统 SHALL NOT 要求 N = K。TopN SHALL 仅影响默认展示/推送优先级；TopK SHALL 仅用于规则 B。

若产品将来选择「仅 TopN 参与 S」，则 SHALL 修订 REQ-POOL-002 及本 SRS 相关节（属变更请求，非默认）。

---

## 6. 边界与重算需求

### REQ-EDGE-001 当 m < 3 且 m ≥ 1

当 m < 3 且 m ≥ 1 时，K = m，规则 B 退化为 **S 全为 `accepted`**（在选定 B-aux-2 时与主规则一致）。

### REQ-EDGE-002 B-aux-1 与 m < 3 冲突

若启用 B-aux-1 且 m < 3，系统 SHALL NOT 处于不可满足的互斥状态；SHALL 实现产品选定的一种：**accepted 数 ≥ m** 等价于 S 全 accepted，或「主管豁免」终态不纳入 S（细则由变更请求定义）。

### REQ-EDGE-003 同价与供给多样性

默认不要求 TopK 内平台/渠道去重。若启用去重/分层策略，SHALL 另增配置与排序键文档（对应设计 §6.5）。

### REQ-EDGE-004 报价时效与过期

报价过期、重抓、汇率/税运变更导致 `compare_price` 或 E 成员变化时，系统 SHALL 触发与 REQ-RECALC-001 一致的重算。是否「过期自动移出 E」— **开放**，选定后 SHALL 写入配置与运维说明。

### REQ-RECALC-001 权威口径

规则 B 是否成立，SHALL 仅基于 **服务端已成功落库** 的 `manufacturer_review_status`、`compare_price`（及 E 的成员条件）计算。

任一改判、改价、新行插入、E 成员变化后，系统 SHALL **重新计算**规则 B，新结果覆盖旧结果。

### REQ-RECALC-002 结案回退与下游

若产品允许「已结案」因新数据回退为未结案，系统 SHALL 在实现文档中约定对下游（会话就绪、通知、配单）的 **幂等或可补偿** 行为，避免重复或漏触发。

### REQ-AUDIT-001 变更审计（建议）

系统 SHOULD 持久化 `manufacturer_review_status` 变更的 **操作者与时间**（及可选原因码），以满足争议复盘；首期若省略，SHALL 在接口层保留扩展位。

---

## 7. 界面需求（与产品 R1.3 对齐）

### REQ-UI-001 TopK 可见标识

工作台 SHALL 对当前参与规则 B 计算的 **T** 所含报价行提供用户可感知的标识（列、标签、色块、图例等任选），并与 TopN 列表在文案上区分「优先审核」与「结案关键底价带」。

---

## 8. 与 `data_ready` 及读模型（占位）

本节约定 **可观测性** 与 **闸门耦合策略的占位**，不锁定具体 RPC 名；定稿时在 `api/bom/v1/bom.proto`（或等价）中落地并更新实现计划。

### REQ-READY-001 规则 B 可查询

系统 SHALL 支持按本 SRS **计算单条需求行的规则 B 布尔**（或等价：返回 m、T 的标识集合由前端推导，**不推荐**——易漂移）。

读模型载体 MAY 为：

- 现有「会话/需求行就绪」类接口的扩展字段；或  
- 独立 `List…` / `Get…` 接口。

SHALL 在实现计划中写明入口与缓存/重算触发点（与 REQ-RECALC-001 一致）。

### REQ-READY-002 与厂牌 pending 计数并存

系统 SHALL NOT 用规则 B 替代厂牌 SRS 中已定义的 `manufacturer_review_status` 语义；两者 MAY 同时出现在同一会话读模型中。

与 `2026-05-04-bom-mfr-two-phase-implementation.md` **P4-2** 已交付的 `quote_mfr_review_pending_count`（或现网名）的关系：

- **规则 B 聚合**与**厂牌阶段二 pending 计数** SHALL **可同时查询**（同一响应或两次调用均可，但产品不得依赖「无法同时得到」的竞态拼凑）。

### REQ-READY-003 `data_ready` 是否依赖规则 B（开放）

会话级 `data_ready`（或 `TryMarkSessionDataReady` 等价逻辑）是否 **必须** 要求「所有需求行规则 B 均为真」— **开放**，由产品与现网闸门策略定稿。

定稿前，系统 SHOULD 预留配置位或代码分支注释，避免静默写死与产品结论冲突。

### REQ-READY-004 建议 proto 字段占位（命名可改，语义保留）

下列为 **占位命名**，合并/改名时须在实现计划与 proto 注释中保留语义追溯。

| 占位字段 | 粒度 | 语义 |
|----------|------|------|
| `line_quote_review_rule_b_ok` | 每需求行 | 按本 SRS 计算，该行为真当且仅当规则 B 成立 |
| `line_quote_review_candidate_pool_m` | 每需求行 | \(m=\|S\|\)，便于 UI 与排障 |
| `session_lines_quote_review_rule_b_not_ok_count` | 会话聚合 | 规则 B 为假的需求行行数 |
| `session_quote_review_default_top_n` | 会话或全局配置回声 | 当前 TopN 的 N，与 REQ-QUEUE-001 一致 |

**首期允许**：仅服务端内部计算 + 结构化日志，不暴露 proto；但一旦产品需要工作台展示「是否可因报价评审推进就绪」，SHALL 升级为 REQ-READY-001 的对外读模型。

---

## 9. 验证与验收（可映射自动化用例）

| 编号 | 场景 | 期望 | 覆盖 REQ |
|------|------|------|----------|
| V-1 | S 共 3 条，价格 10/20/30，均 pending | 规则 B 假；全 accepted 后真 | RULE-B |
| V-2 | S 共 5 条，最便宜 3 条中 1 条 pending | 规则 B 假 | RULE-B |
| V-3 | 最便宜一条 rejected | 不参与 S；T 在剩余集上重算 | POOL-002 |
| V-4 | 同价并列 | T 由稳定全序唯一确定 | DEF-001 |
| V-5 | m = 0 | 规则 B 假；禁止空 T 通过 | RULE-B-001 |
| V-6 | 最便宜 K 条均 accepted，下方大量 pending | 规则 B 真（B-aux-2） | RULE-B-003 |
| V-7 | 读模型含占位字段或等价推导 | 可得到本行 `line_quote_review_rule_b_ok` 或与 m+排序等价的断言 | READY-001 |

---

## 10. 关联文档

- 总览设计：`2026-05-05-bom-quote-review-queue-and-line-completion-design.md`
- 产品需求：`2026-05-05-bom-quote-review-queue-and-line-completion-requirements.md`
- 实现计划：`../plans/2026-05-05-bom-quote-review-queue-and-line-completion-implementation.md`
- 厂牌两阶段 SRS：`2026-05-04-bom-mfr-cleaning-two-phase-software-requirements-spec.md`
