# BOM 报价评审：默认队列与需求行结案 — 开发计划

> **执行说明：** 按 **TDD**（见下文「TDD 执行契约」）与 Phase 顺序实现；每步完成后对照 `docs/superpowers/specs/2026-05-05-bom-quote-review-queue-and-line-completion-software-requirements-spec.md`（**SRS**）验收。  
> **技能：** `using-superpowers`（先读技能再动手）；行为变更前优先 **test-driven-development**（红 → 绿 → 重构）。  
> **产品门禁：** 需求文档 **§7 评审待定**（M1/M2、B-aux-1/2、`data_ready` 是否依赖规则 B）未定稿前，**Phase 5 闸门类任务不得写死为强制阻塞**；以配置/特性开关占位（SRS REQ-READY-003）。

## TDD 执行契约（本计划强制）

对齐 superpowers **test-driven-development**（与 `2026-05-04-bom-mfr-two-phase-implementation.md` 一致）：

1. **铁律：** 无失败测试，不写业务实现。先写/改测试并 **运行确认失败原因正确**，再写最小通过代码。
2. **循环：** **RED** → **Verify RED**（`go test -run …` / `npm test -- …`）→ **GREEN** → **Verify GREEN** → **REFACTOR**。
3. **例外：** 纯 `docs/schema/migrations/*.sql`、`*.pb.go` / `wire_gen.go` —— 可不先写测试；**biz 规则、repo 查询形状、service 编排、前端** 一律 TDD。
4. **禁止：** 先写实现再补测试贴绿。

---

## 依据文档

| 文档 | 路径 |
|------|------|
| 软件需求规格（权威 SHALL / REQ-* / V-*） | `docs/superpowers/specs/2026-05-05-bom-quote-review-queue-and-line-completion-software-requirements-spec.md` |
| 产品需求 | `docs/superpowers/specs/2026-05-05-bom-quote-review-queue-and-line-completion-requirements.md` |
| 设计总览 | `docs/superpowers/specs/2026-05-05-bom-quote-review-queue-and-line-completion-design.md` |
| 厂牌两阶段 SRS（`manufacturer_review_status`） | `docs/superpowers/specs/2026-05-04-bom-mfr-cleaning-two-phase-software-requirements-spec.md` |
| 厂牌实现计划（P4-2 读模型衔接） | `docs/superpowers/plans/2026-05-04-bom-mfr-two-phase-implementation.md` |

---

## Goal

在单条 `t_bom_session_line` 维度实现：

1. **可比价排序与稳定 TopK/TopN**（SRS REQ-DEF-001、REQ-QUEUE-*）。
2. **候选池 E、S 与规则 B 布尔**（REQ-POOL-*、REQ-RULE-B-*、REQ-EDGE-*），含 **m=0** 禁止真空通过（V-5）。
3. **读模型可观测性**与厂牌 `quote_mfr_review_pending_count` **并存**（REQ-READY-001/002）；**`data_ready` 是否依赖规则 B** 按 REQ-READY-003 配置化或延后。
4. **工作台**：默认低价 TopN 展示顺序 + TopK 与 TopN 文案/视觉区分（产品 R1.3、SRS REQ-UI-001）。

---

## Architecture（Kratos 分层）

| 层级 | 职责 |
|------|------|
| **`internal/biz`** | **E** 的成员判定（可纯函数 + 可注入谓词）、**S** 构造、**m/K/T**、**规则 B**、**TopN 切片**、B-aux 分支；**禁止**在 `data` 内实现上述业务布尔。 |
| **`internal/data`** | 按 `session_line_id`（或等价）加载报价行只读视图（`compare_price`、`manufacturer_review_status`、E 所需标志列）；GORM；**无**规则 B 状态机。 |
| **`internal/service`** | 组装 biz、填 `GetReadiness`（或新 RPC）响应字段；参数校验。 |
| **`api/bom/v1/bom.proto`** | 扩展读模型字段（REQ-READY-004 占位命名经评审可改）；`make api` 生成。 |
| **`web/`** | TopN 默认排序、TopK 标识、图例；消费 readiness / 行级 API。 |

---

## 文件结构（预期改动一览）

| 区域 | 路径（建议，实施时可等价调整） | 说明 |
|------|-------------------------------|------|
| Migration（若缺可比价列） | `docs/schema/migrations/*_bom_quote_item_compare_price.sql` | 持久化或缓存 `compare_price`；与 SRS REQ-DEF-001 对齐；若已存在等价列则跳过 |
| Model | `internal/data/models.go` | `BomQuoteItem` 等增补字段 |
| Biz（新） | `internal/biz/bom_quote_review_rule_b.go`（或拆子文件，遵守单文件 ≤300 行） | `LineQuoteReviewRuleB`、`TopNQuoteItemIDs`、`QuoteReviewConfig{M1/M2, BAux, N}` 等纯逻辑 + 表测 |
| Biz 测试 | `internal/biz/bom_quote_review_rule_b_test.go` | 覆盖 SRS **§9** V-1～V-6 |
| Repo 接口 | `internal/biz/repo.go` | 仅签名：按行拉取报价视图切片（若尚无） |
| Data 实现 | `internal/data/bom_*.go` | 实现 repo；**禁止**内嵌规则 B |
| Service | `internal/service/bom_availability.go` 或新建 `bom_quote_review_readiness.go` | 合并 readiness；与现有 `GetReadiness` 对齐 |
| Proto | `api/bom/v1/bom.proto` | `line_quote_review_rule_b_ok` 等（REQ-READY-004） |
| Conf（可选） | `internal/conf` + 配置文件 | `quote_review_default_top_n`、M1/M2、B-aux 开关 |
| 前端 | `web/src/pages/bom-workbench/…` 报价/厂牌子面板 | REQ-UI-001 |
| 测试 | `*_test.go`、Vitest | V-7、UI 回归 |

---

## SRS 追溯矩阵（Phase → REQ → 验收）

| Phase | 主要 REQ 组 | SRS 验证用例 |
|-------|-------------|--------------|
| P0 配置与数据前提 | REQ-DEF-002、REQ-RULE-B-004、REQ-QUEUE-001 | 配置单测 / 文档化 |
| P1 `biz` 规则核 | REQ-DEF-001、REQ-POOL-*、REQ-RULE-B-*、REQ-QUEUE-*、REQ-EDGE-001/002 | V-1～V-6 |
| P2 `data` 读路径 | REQ-RECALC-001（数据来源） | 集成测或 repo 测 |
| P3 `service` + proto | REQ-READY-001、002、004 | **V-7** |
| P4 前端 | REQ-UI-001、产品 R1.1～R1.3 | Vitest + 手工走查 |
| P5 闸门（可选） | REQ-READY-003、REQ-RECALC-002 | 依产品定稿后补用例 |

---

## 阶段划分与任务

### Phase 0：产品参数与数据前提

- [x] **P0-1** 在配置或代码常量中 **锁定**（直至产品改单）：`compare_price` 缺失策略 **M1 或 M2**（REQ-DEF-002）；**B-aux-1 或 B-aux-2**（REQ-RULE-B-004）；默认 **TopN = 5**（REQ-QUEUE-001）。**已实现：** `biz.DefaultQuoteReviewConfig()`（M1 + B-aux-2 + TopN=5）；后续可改为 `internal/conf`。
- [x] **P0-2** 核对 `t_bom_quote_item`（或读模型所用表）是否具备 **稳定排序** 所需字段：`compare_price`（或可推导）、`manufacturer_review_status`、`id`/`updated_at`。缺失则 **P0-3** migration + 模型更新（可无单测先合 DDL，模型加载测建议保留）。**结论：** 可比价由 `HKPrice`/`MainlandPrice`/`PriceTiers` + `ToBaseCCY` 现算（与配单一致），**未**新增 DB 列。
- [x] **P0-4** 文档化 **E** 的首期谓词（REQ-POOL-001）：**已实现** `BuildQuoteReviewRowInputs` 对返回行 `InE=true`（调用方传入全量 `ListBomQuoteItemsForSessionLineRead` 结果）；剔除规则后续可注入。

**验收：** 团队内对需求 §7 有书面结论或明确「按 SRS 推荐默认（M1 待定 / B-aux-2）」的 ADR 一句。

---

### Phase 1：`biz` 规则核心（纯函数 + 表测）

实现 **不依赖 DB** 的输入类型（如 `QuoteReviewRowInput`：价格指针、状态枚举、是否在 E、`id`、时间戳），输出：`S` 成员下标、`m`、`K`、`T`、`RuleBOk`、`TopNIDs`。

- [x] **P1-1** `E` 过滤函数（REQ-POOL-001）。
- [x] **P1-2** `S` 构造（REQ-POOL-002、与 M1/M2 联动 REQ-DEF-002）。
- [x] **P1-3** 稳定排序与 **T**、**TopN**（REQ-DEF-001、REQ-RULE-B-002、REQ-QUEUE-001）。
- [x] **P1-4** **规则 B** 主规则 + B-aux 分支（REQ-RULE-B-001～004、REQ-EDGE-001/002）。
- [x] **P1-5** 改判/重算语义在纯函数层可复用（REQ-POOL-003、REQ-RECALC-001 的「输入快照 → 输出」）。

**TDD：**

- [x] **P1-T-RED/GREEN** 为 **V-1～V-6** 各写表驱动用例（文件 `bom_quote_review_rule_b_test.go`），严格对齐 SRS **§9** 期望列（另含 M2、B-aux-1 补充用例）。
- [ ] **P1-T-REFACTOR** 拆函数、去重复；单文件逼近 300 行时按职责拆文件。

**验收：** SRS **§9** V-1～V-6 全绿；`go test ./internal/biz/... -run QuoteReview -count=1`。

---

### Phase 2：`data` 层只读装载

- [x] **P2-1** 实现 `biz` 所需接口：按 `session_id` + `session_line_id` 列出报价行视图（字段满足 P1 输入结构）。**已复用** `ListBomQuoteItemsForSessionLineRead`。
- [x] **P2-2** 查询 **不得** 在 SQL 中实现规则 B；仅 **WHERE** 会话/行归属 + 可选软删；排序 **可**在 SQL 做一层，但与 `biz` 稳定序约定一致（避免双实现漂移，推荐 **DB 按同序排序 → biz 再算 T** 或 **biz 全包排序** 二选一，在实现 PR 描述中写死）。**已定：** 排序在 `biz.ComputeQuoteReviewLineOutcome` 内完成。

**TDD：**

- [ ] **P2-T-RED/GREEN** repo 集成测（`TEST_DATABASE_URL`）或轻量 fake：给定插入数据，返回切片顺序与字段与 P1 用例衔接。

**验收：** REQ-RECALC-001 的数据来源可追溯；无业务分支于 `data`。

---

### Phase 3：`service` + `proto` 读模型

- [x] **P3-1** 扩展 `GetReadiness`（或经评审的新 RPC）响应：至少满足 **REQ-READY-001**（每行或聚合可推导 `line_quote_review_rule_b_ok`）；字段名采用 REQ-READY-004 占位或评审后改名，**proto 注释保留语义映射**。
- [x] **P3-2** 同响应中 **保留** `quote_mfr_review_pending_count`（或现网字段），满足 **REQ-READY-002**。
- [x] **P3-3** `session_lines_quote_review_rule_b_not_ok_count`、`session_quote_review_default_top_n` 等按产品优先级 **SHOULD** 逐项落地或标注「二期」。
- [x] **P3-4** Wire 注入 `biz` 与 repo；**禁止**在 `service` 重复实现规则 B 公式。

**TDD：**

- [ ] **P3-T-RED/GREEN** `internal/service/*_test.go`：mock repo 固定返回行集，断言 readiness 中规则 B 相关字段与 P1 预期一致（**V-7**）。

**验收：** SRS **V-7**；与 P4-2 字段并存无覆盖。

---

### Phase 4：前端工作台

- [x] **P4-1** 列表默认按 **TopN** 思路排序或分段展示（产品 R1.1；与后端约定一致即可）。**已实现：** `SessionDataCleanPanel` 的 `sortedQuoteReviews` — 先 `line_no` 升序，行内 TopK 序（`top_k` 数组）→ TopN 序（`top_n` 数组）→ 其余按 `quote_item_id`；图例一句说明。
- [x] **P4-2** **TopK** 行视觉标识 + 文案区分「优先审核」vs「结案关键底价带」（REQ-UI-001、产品 R1.3）。**已实现：** `SessionDataCleanPanel` 拉 `getReadiness`+`getBOMLines`，`QuoteItemMfrReviewSection` 列「队列」+ 图例 + 行底色。
- [x] **P4-3** 若 readiness 暴露 `line_quote_review_rule_b_ok`：在行级 UI 展示「规则 B 是否满足」（可选徽章）。**已实现：** 列「规则 B」+ `lineRuleBOk` 徽章（满足 / 未满足）；`data-testid` `quote-rule-b-ok-{quote_item_id}`。

**TDD：**

- [x] **P4-T** Vitest：`BomWorkbenchPage.test` 断言图例与「底价带」展示（`quote-review-queue-legend`）。

**验收：** 产品走查 + REQ-UI-001。

**已做（API + 数据清洗 UI）：** `web/src/api/types.ts`、`bomSession.getReadiness`；`SessionDataCleanPanel` / `QuoteItemMfrReviewSection` 消费 `line_quote_review_readiness`。

---

### Phase 5：`data_ready` / 闸门（产品定稿后）

- [ ] **P5-1** 根据 **REQ-READY-003** 定稿：在 `TryMarkSessionDataReady`（或等价）中增加/不增加「全行 `line_quote_review_rule_b_ok`」条件；**须配置化**或明确文档「不阻塞」。
- [ ] **P5-2** 若允许结案回退：实现 **REQ-RECALC-002**（幂等事件、去重通知）；与现网 `data_ready` 状态机文档对齐。

**TDD：**

- [ ] **P5-T** 集成或单测：`data_ready` 在规则 B 未满足时 **应/不应** 失败（依配置断言）。

**验收：** REQ-READY-003 关闭为明确行为 + 用例。

---

### Phase 6（建议）：审计与运维

- [ ] **P6-1** `manufacturer_review_status` 变更审计（REQ-AUDIT-001，SHOULD）：迁移可选列 + 写入路径；或接口扩展位。
- [ ] **P6-2** 报价过期联动 **REQ-EDGE-004**：定稿后纳入 E 谓词或定时任务，并更新本计划。

---

## 风险与依赖

| 风险 | 缓解 |
|------|------|
| `compare_price` 未落地或口径不一致 | P0-2/P0-3；与配单侧 `no_compare_price_after_filters` 等现网理由对齐 |
| E 与「硬过滤」产品未定 | P0-4 最小 E；后续变更请求替换谓词 |
| 与厂牌 pending 语义混淆 | 文档与 UI 同时展示两指标；REQ-READY-002 |
| 单文件超长 | 拆 `bom_quote_review_*.go`；遵守仓库 300 行约定 |

---

## 修订记录

| 日期 | 说明 |
|------|------|
| 2026-05-05 | 占位稿：三文档链接 + 粗顺序 |
| 2026-05-05 | 按 SRS 全文展开：TDD 契约、分层、文件结构、Phase 0～6、REQ/V 追溯矩阵、风险表 |
| 2026-05-05 | **执行：** `biz` 规则 B + 输入装配 + `GetReadiness` proto 扩展 + `bom_readiness_quote_review.go`；`protoc` 重生成 `api/bom/v1/*.pb.go`；前端 types / `getReadiness` 解析 |
| 2026-05-05 | **UI：** 数据清洗 `QuoteItemMfrReviewSection` TopK/TopN 列与图例；`SessionDataCleanPanel` 并行拉 `getReadiness`+`getBOMLines`；Vitest 补充 `getReadiness` mock |
| 2026-05-05 | **P4-1/P4-3：** 报价表默认排序 +「规则 B」列；`BomWorkbenchPage.test` 断言行序与 `quote-rule-b-ok-*` |
