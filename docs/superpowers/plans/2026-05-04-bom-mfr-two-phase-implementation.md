# BOM 厂牌两阶段清洗 — 开发计划

> **执行说明：** 按 **TDD**（见下文「TDD 执行契约」）与任务顺序实现；每步完成后对照 `2026-05-04-bom-mfr-cleaning-two-phase-software-requirements-spec.md`（SRS）验收。未上线，**不保留**旧混合候选与双表回填的兼容路径。  
> **技能：** `using-superpowers`（先读技能再动手）；实现行为变更前遵循 `test-driven-development`（先写失败测试、看到红、再写最小实现、再重构）。

## TDD 执行契约（本计划强制）

对齐 superpowers **test-driven-development**：

1. **铁律：** 无失败测试，不写业务实现（`NO PRODUCTION CODE WITHOUT A FAILING TEST FIRST`）。先写/改测试并 **运行确认失败原因正确**，再写最小通过代码。
2. **循环：** **RED**（失败测试）→ **Verify RED**（必须执行 `go test -run …` / `npm test -- …`，确认红）→ **GREEN**（最小实现通过）→ **Verify GREEN** → **REFACTOR**（整理结构、保持全绿）。
3. **例外（本仓库约定）：** 纯 `docs/schema/migrations/*.sql` DDL、`*.pb.go` / `wire_gen.go` 生成物 —— 可不先写测试；但 **GORM 模型可加载、repo 行为、biz 规则、service 编排、前端交互** 一律 TDD。
4. **禁止：** 先写实现再「补测试贴绿」；保留失败实现当参考再写测试（应删除从零按测试写）。

各 Phase 下 **T-RED / T-GREEN / T-REFACTOR** 为建议最小步序；若一步内有多条行为，拆多条测试、多次红绿循环。

## 依据文档

| 文档 | 路径 |
|------|------|
| 技术方案 | `docs/superpowers/specs/2026-05-04-bom-mfr-cleaning-two-phase-design.md` |
| 软件需求规格 | `docs/superpowers/specs/2026-05-04-bom-mfr-cleaning-two-phase-software-requirements-spec.md` |
| 产品需求概要 | `docs/superpowers/specs/2026-05-04-bom-mfr-cleaning-two-phase-requirements.md` |

## Goal

将厂牌治理拆为 **阶段一（仅 `t_bom_session_line` + 别名表）** 与 **阶段二（仅 `t_bom_quote_item` 通过/不通过 + 改判）**；移除 `ListManufacturerAliasCandidates` 混合列表；阶段一审批仅调用 `BackfillSessionLineManufacturerCanonical`（不写 `quote_item`）；`ApplyKnownManufacturerAliasesToSession` 仅回填需求行。前端「数据清洗」分两块 UI，阶段二 GET 返回 `gate_open`（SRS REQ-API-003）。

## Architecture

- **`internal/biz`**：闸门 `gate_open`、阶段一/二入队条件、`accept`/`reject` 不变量（SRS REQ-DATA-004）、改判；从现有 `collectLineManufacturerAliasCandidates` 拆出可复用的纯逻辑或新函数，**禁止**在 `data` 写状态机。
- **`internal/data`**：GORM migration 对应模型字段；`BackfillSessionLineManufacturerCanonical`（仅行）或等价实现；阶段二按 `quote_item_id` 更新 `manufacturer_review_status` / canonical / reason；**禁止**在 `data` 实现「是否可开闸门」等业务判定。
- **`internal/service`**：编排 proto/HTTP、参数校验、错误码。
- **`internal/server`**：注册新路由；删除旧 `manufacturer-alias-candidates` 与旧 `manufacturer-alias-approvals` 的 HTTP 绑定（若 proto 仍暂留则不得再挂旧语义）。
- **`api/bom/v1/bom.proto`**：新增 RPC + message；旧 RPC 删除或改为新名后重新 `make api` / `protoc`。
- **`web/`**：`SessionDataCleanPanel` 拆子面板；`web/src/api/bomMatchExtras.ts` 换新端点；Vitest 更新。

**Tech Stack：** Go（Kratos）、GORM、MySQL、protobuf、React + TypeScript + Vitest。

---

## 文件结构（预期改动一览）

| 区域 | 路径 | 说明 |
|------|------|------|
| Migration | `docs/schema/migrations/20260504_bom_quote_item_mfr_review.sql`（文件名按仓库惯例调整） | `manufacturer_review_status` 默认 `pending`；可选 `reason`、`reviewed_at` |
| Model | `internal/data/models.go` | `BomQuoteItem` 增字段 |
| Cleaning | `internal/data/bom_manufacturer_cleaning_repo.go` | 拆「仅行回填」；`ApplyKnownAliasesToSession` 去掉 quote 循环 |
| Biz 接口 | `internal/biz/repo.go`（或新建 `bom_mfr_cleaning.go`） | `BomManufacturerCleaningRepo` 扩展或新增方法签名 |
| 候选/评审 | `internal/service/bom_mfr_two_phase.go`（及 `BomService` 方法） | 原计划的独立文件已合并为现实现；旧 `bom_manufacturer_alias_candidates.go` **已删除** |
| 旧文件 | `internal/service/bom_manufacturer_alias_candidates.go`（及同目录 `*_test.go`） | **已删除**；`ListManufacturerAliasCandidates` 等入口已移除 |
| API 实现 | `internal/service/bom_manufacturer_alias_api.go` | 阶段一 `ApproveSessionLineMfrCleaning` 仅行回填；无双表 backfill |
| HTTP | `internal/server` 由 `api/bom/v1` 生成路由注册 | 旧 `bom_alias_candidates_http.go` **已移除**；现为 proto 注解生成之 REST |
| Proto | `api/bom/v1/bom.proto` + 生成代码 | 新 RPC；移除废弃 RPC |
| 配单读模型 | `internal/biz` 下匹配/就绪相关文件 | 按设计 §7 / SRS REQ-RM-* 识别 `manufacturer_review_status` |
| 前端 | `web/src/pages/bom-workbench/SessionDataCleanPanel.tsx`、`ManufacturerAliasReviewPanel.tsx`（或新建行专用/报价专用组件） | 两阶段 UI + `gate_open` |
| API TS | `web/src/api/bomMatchExtras.ts` | 新 fetch 封装 |
| 测试 | `*_test.go`、`*.test.tsx` | 对齐 SRS REQ-TEST-* |

---

## 阶段划分与任务

### Phase 0：Schema 与模型

- [x] **P0-1** 编写 migration：`t_bom_quote_item` 增加 `manufacturer_review_status`（`ENUM`/`VARCHAR` + 常量，默认 `pending`）；按需 `manufacturer_review_reason`、`manufacturer_reviewed_at`。
- [x] **P0-2** 更新 `internal/data/models.go` 中 `BomQuoteItem`；若有 OpenAPI/文档片段则同步（非必须）。

**TDD：**

- [x] **P0-T-RED** 在 `internal/data/*_test.go`（或新建 `bom_quote_item_mfr_review_test.go`）写测试：插入/查询 `BomQuoteItem` 时期望 `manufacturer_review_status` 默认等于 `pending`（或 ORM 零值与 DB 默认一致）；**先跑测试**，在未执行 migration 或模型未加字段时应 **失败且原因符合预期**。
- [x] **P0-T-GREEN** 应用 P0-1 + P0-2 后同一测试 **通过**。
- [x] **P0-T-REFACTOR** 提取状态常量到 `internal/biz` 或 `internal/data` 包级常量，避免魔法字符串。

**验收：** SRS REQ-MIG-001、REQ-DATA-002。

---

### Phase 1：`data` 层回填收缩

- [x] **P1-1** 实现 `BackfillSessionLineManufacturerCanonical`（或等价）：仅更新 `t_bom_session_line`，逻辑从现有 `BackfillSessionManufacturerCanonical` 抽出 **仅 line 循环**。
- [x] **P1-2** 删除或废弃原 `BackfillSessionManufacturerCanonical` 的 **quote_item** 循环；全仓库 grep 确保阶段一/别名审批不再调用双表路径。
- [x] **P1-3** `ApplyKnownAliasesToSession`：仅对 session line 调用 `applyKnownManufacturer`；**移除**对 `listQuoteItemsForCleaning` 的 quote 更新循环（设计 §6、`ApplyKnown` 语义）。
- [x] **P1-4** 实现 `data` 层按 `quote_item_id` 更新 `manufacturer_review_status` + canonical + reason（无业务分支，由 `service`/`biz` 传入终态）。

**TDD：**

- [x] **P1-T-RED** `bom_manufacturer_cleaning_repo_test.go`（或等价）：给定 session 内同时存在匹配 `alias_norm` 的 line + quote_item，调用 **仅行回填** 后断言 **quote_item 未被更新**；当前双表实现下若仍走旧路径则调整测试使其先红（或先断言旧行为再改实现后断言新行为）。**必须**先运行见红。
- [x] **P1-T-GREEN** 实现 `BackfillSessionLine…` 并删 quote 循环，测试绿。
- [x] **P1-T-RED2** 为 `ApplyKnownAliasesToSession` 写「quote 行数不变 / quote canonical 不变」用例，跑红。
- [x] **P1-T-GREEN2** 删 `ApplyKnown` 中 quote 循环，测试绿。
- [x] **P1-T-RED3** 为 `UpdateQuoteItemMfrReview`（命名自定）写表驱动用例：`accept`/`reject` 终态字段；先红。
- [x] **P1-T-GREEN3** 最小 GORM 更新实现，测试绿。

**验收：** SRS REQ-API-002、REQ-API-006；设计 §4.3、§6 应用别名。

---

### Phase 2：`biz` 闸门与入队

- [x] **P2-1** 实现 `SessionLineMfrGateOpen(sessionID)`（命名自定）：`LinesNeedCanon` 内全部 `manufacturer_canonical_id` 非空 ⇒ `true`（SRS REQ-GATE-001～003）。
- [x] **P2-2** 阶段一候选列表纯函数/用例：`norm_mfr(mfr)!=""` 且 canonical 为空；不依赖报价 JSON（SRS REQ-S1-001～002）。
- [x] **P2-3** 阶段二入队：闸门开、父行 `mfr` 非空、父行 canonical 非空、报价 `manufacturer` 非空、`pending`、与父行 canonical 比较规则与设计 §5.1 对齐（复用现 `collectLineManufacturerAliasCandidates` 中 quote 比较思想，**demand canonical 固定为行字段**）（SRS REQ-S2-001）。
- [x] **P2-4** `accept`/`reject`/`改判` 不变量校验（SRS REQ-DATA-004、REQ-S2-002）。
- [x] **P2-5** 父行 `mfr` 为空：子报价不入阶段二列表；闸门不阻塞（SRS REQ-GATE-003、REQ-GATE-004）——实现选定「读模型优先」或「批量 accepted」**一种**并写注释。

**TDD：** 纯函数与规则优先放在 `internal/biz`，文件超长则拆 `bom_mfr_gate_test.go`、`bom_mfr_line_candidates_test.go` 等。

- [x] **P2-T-RED** 闸门表驱动测试：`LinesNeedCanon` 非空且有人缺 canonical ⇒ `false`；全齐 ⇒ `true`；仅存在 `mfr` 空行 ⇒ `true`。先写期望再实现。
- [x] **P2-T-GREEN** 实现 P2-1，全绿。
- [x] **P2-T-RED2** 阶段一候选：给定内存 `[]BomSessionLine`（或 DTO），断言输出条数与 line_no；含「`mfr` 空」行应被排除。红后实现 P2-2。
- [x] **P2-T-RED3** 阶段二入队 + 不变量：对 `accept`/`reject`/改判输入写失败用例（非法组合应返回 `error` 或 Result 类型）。红后实现 P2-3～P2-5。

**验收：** `internal/biz` 单测覆盖闸门、空 `mfr`、不变量。

---

### Phase 3：Proto + `service` + HTTP

- [x] **P3-1** 在 `api/bom/v1/bom.proto` 定义：  
  - `ListSessionLineMfrCandidates` + Reply（行列表字段）  
  - `ApproveSessionLineMfrCleaning`（或同名）+ Request（`session_id`、`line_id`/`line_no`、`alias`、`canonical_id`、`display_name`）+ Reply（`session_line_updated`）  
  - `ListQuoteItemMfrReviews` + Reply（**`gate_open`**、`items[]` 含 `quote_item_id`、报价厂牌、平台、`line_manufacturer_canonical_id`、展示名等）  
  - `SubmitQuoteItemMfrReview` + Request（`quote_item_id`、`decision`、`reason`）+ Reply  
  删除：`ListManufacturerAliasCandidates` / `ApproveManufacturerAliasCleaning` 若在 proto 中不存在则仅删 HTTP 包装；若在 proto 中则 **删除 RPC** 并再生 pb。
- [x] **P3-2** `internal/service`：实现上述 RPC；阶段一：`CreateRow` + `BackfillSessionLineOnly`；阶段二：调 `biz` 校验后 `data` 更新 quote。
- [x] **P3-3** `internal/server`：注册新 HTTP 路径（与设计 §6 表一致）；**移除** `bom_alias_candidates_http.go` 中旧 `GET .../manufacturer-alias-candidates` 与旧 approvals 路由（或整文件替换）。
- [x] **P3-4** `wire` / `BomService` 构造函数：注入无变更则跳过。

**TDD：** Proto 生成后可 **stub / httptest** 或 `BomService` + mock repo 单测，避免无测试直接接 HTTP。

- [x] **P3-T-RED** `internal/service/bom_mfr_two_phase_test.go`（新建）：stub `BomSessionRepo` + cleaning + alias，断言 `ListQuoteItemMfrReviews` 在「闸门未开」时 `gate_open=false` 且 `items` 可为空；闸门开且无语义待办时 `gate_open=true` 且 `items` 为空。先运行失败。
- [x] **P3-T-GREEN** 实现 `service` 编排 + HTTP 注册，`go test` 绿。
- [x] **P3-T-RED2** 阶段一审批：mock 期望 **仅调用** `BackfillSessionLine…`，**不调用** 双表 backfill；红后绿。
- [x] **P3-T-REFACTOR** 拆分 `service` 文件避免超 300 行。

**验收：** SRS REQ-API-001～005；`gate_open` 三种组合由自动化测试覆盖（替代或补充手工 curl）。

---

### Phase 4：配单 / 就绪读模型

- [x] **P4-1** 在 `biz` 匹配或报价可用性判断处：父行 `mfr` 非空时，仅 `accepted` 且 quote canonical 等于父行 canonical 视为厂牌可用；`pending`/`rejected` 按 SRS REQ-RM-002。
- [x] **P4-2** 若会话 `data_ready` 依赖厂牌：增加「阶段二 `pending` 计数」或复用列表 API 阻塞策略（与设计 §7「建议阻塞」对齐，**在 SRS 开放项内定稿**）。**已交付：** `GetReadiness` 返回 `quote_mfr_review_pending_count`（`CountQuoteMfrReviewPendingForSession`）。**未纳入本轮：** 因 pending 阻塞 `data_ready` / `TryMarkSessionDataReady`（待 SRS 定稿后再做）。

**TDD：**

- [x] **P4-T-RED** 在现有 match / quote eval 测试旁新增用例：构造 `manufacturer_review_status=pending` 的 quote，断言 **不可用**；`accepted` + canonical 与父行一致 ⇒ 可用；`rejected` ⇒ 不可用。先红后绿。
- [x] **P4-T-GREEN** 改读模型实现，全绿后重构。

**验收：** 关键路径单测或回归用例；与 `bom_service` / match 相关测试更新。

---

### Phase 5：前端

- [x] **P5-1** `web/src/api/bomMatchExtras.ts`：删除旧 `listManufacturerAliasCandidates` / `approveManufacturerAliasCleaning`；新增四个函数对应新 REST（或统一 `fetchJson` 封装）。
- [x] **P5-2** `SessionDataCleanPanel`：拆 **需求行厂牌** 子面板（列表 + 选 canonical + 提交 + 应用已有别名）与 **报价厂牌确认** 子面板（依赖 `gate_open`；只读父 canonical；通过/不通过/原因）。
- [x] **P5-3** 可复用或拆分 `ManufacturerAliasReviewPanel`：阶段一保留下拉/手动；阶段二新组件无下拉。
- [x] **P5-4** Vitest：`BomWorkbenchPage.test.tsx`、`bomMatchExtras.test.ts` 等更新为新 API；覆盖 `gate_open == false` 时第二块禁用逻辑。

**TDD：**

- [x] **P5-T-RED** 先改/写 `bomMatchExtras.test.ts`：mock `fetch`，断言新 URL 与 body 形状；删除旧 API 测试引用。运行 **红**。
- [x] **P5-T-GREEN** 实现 `bomMatchExtras.ts`，测试绿。
- [x] **P5-T-RED2** `BomWorkbenchPage.test.tsx`（或组件单测）：`gate_open=false` 时第二块按钮/区域 `disabled` 或 `aria-disabled`。红。
- [x] **P5-T-GREEN2** 实现 `SessionDataCleanPanel` 拆分与 props，绿后 **REFACTOR** 子组件文件拆分。

**验收：** SRS REQ-UI-001～003。

---

### Phase 6：清理与文档收尾

- [x] **P6-1** 删除死代码：`bom_manufacturer_alias_candidates.go` 中不再使用的类型/导出；更新 `internal/service/bom_manufacturer_alias_candidates_test.go` 为 **新**单测文件或迁移用例到 `bom_mfr_*_test.go`。
- [x] **P6-2** 全仓库 grep：`manufacturer-alias-candidates`、`manufacturer-alias-approvals`、`ListManufacturerAliasCandidates`、旧名 `BackfillSessionManufacturerCanonical` 确保无残留；阶段一应仅见 `BackfillSessionLineManufacturerCanonical`。
- [x] **P6-3** 在本计划文末更新「完成日期」；若有 AGENTS.md / README 引用旧接口则改链接。

**TDD：** 删除旧代码前 **保留**覆盖新行为的测试已绿；删除后全量 `go test` / `npm test`。若删测试导致绿但行为未覆盖，**禁止**合并。

**验收：** SRS REQ-TEST-001～003；`go test ./...` 与 `npm test` 通过（范围按 CI）。

---

## 风险与依赖

| 风险 | 缓解 |
|------|------|
| Proto 变更面大 | 未上线可一次性删旧 RPC；合并前全量 `go test` |
| 单文件超 300 行 | `service`/`biz` 按职责拆文件 |
| 配单路径多 | P4 先定位 `manufacturer_canonical_id` / quote 使用点再改 |

---

## 修订记录

| 日期 | 说明 |
|------|------|
| 2026-05-04 | 初稿：依据设计 + SRS 的分阶段开发计划 |
| 2026-05-04 | 嵌入 TDD 铁律、各 Phase 的 T-RED/T-GREEN/T-REFACTOR 步序；Phase 3/5 明确先测后码 |
| 2026-05-04 | Phase 0 勾选：`bom_quote_item_mfr_review_test.go` 集成测（`TEST_DATABASE_URL`）；`BomQuoteItem` 评审列对齐 migration |
| 2026-05-04 | Phase 1：`BackfillSessionLineManufacturerCanonical` 入 `biz` 接口；删未用 quote 辅助；`bom_manufacturer_cleaning_repo_test.go` 集成测 |
| 2026-05-04 | 移除 `BackfillSessionManufacturerCanonical` 接口与实现，仅保留 `BackfillSessionLineManufacturerCanonical` |
| 2026-05-04 | Phase 2：`SessionLineMfrGateOpen`、`SessionLinesNeedingPhase1MfrCleaning`、`QuoteItemEligibleForPhase2ReviewList`、`RequireParentManufacturerCanonicalForQuoteMfrReview`；`bom_mfr_two_phase` 编排下沉 |
| 2026-05-04 | Phase 3：`bom.proto` 四 RPC + `make api` 等价生成；删 `ApproveManufacturerAliasCleaning` 与手写 `bom_mfr_two_phase_http`；`bom_mfr_two_phase_test.go` |
| 2026-05-04 | Phase 4：`biz` 行过滤 REQ-RM-002（`bom_line_match_row_filter.go`）；`service` 合并 `ListBomQuoteItemsForSessionLineRead` 至配单与可用性；`GetReadinessReply.quote_mfr_review_pending_count`；单测 `bom_line_match_test.go`、`bom_quote_row_mfr_merge_test.go` |
| 2026-05-04 | Phase 5：前端 `approveSessionLineMfrCleaning` + 两阶段 GET 解析；`SessionLineMfrPhasePanel` / `QuoteItemMfrReviewSection`；`ManufacturerAliasReviewPanel` 可配置说明与行 key；Vitest `bomMatchExtras` + `BomWorkbenchPage`（`gate_open` / `aria-disabled`） |
| 2026-05-04 | Phase 6：核对 `internal`/`cmd`/`api`/`web` 无旧 HTTP/RPC 符号；旧 `bom_manufacturer_alias_candidates*` 已不在仓库；计划表「文件结构」与文末收尾说明对齐现状；`AGENTS.md` 无旧接口引用 |

---

## 收尾状态（Phase 6）

- **完成日期：** 2026-05-04  
- **代码核对（`*.go` / `*.ts` / `*.tsx`）：** 无 `manufacturer-alias-candidates`、`manufacturer-alias-approvals`、`ListManufacturerAliasCandidates`、`ApproveManufacturerAliasCleaning`、`BackfillSessionManufacturerCanonical`；阶段一回填仅 **`BackfillSessionLineManufacturerCanonical`**。  
- **已删除文件：** `internal/service/bom_manufacturer_alias_candidates.go`、`bom_manufacturer_alias_candidates_test.go`（及手写旧 HTTP）；同类行为由 **`bom_mfr_two_phase.go`**、**`bom_manufacturer_alias_api.go`** 与 **`bom_manufacturer_alias_api_test.go`** / **`bom_mfr_two_phase_test.go`** 等覆盖。  
- **归档文档：** `docs/superpowers/plans/2026-04-26-bom-manufacturer-cleaning-implementation.md` 等仍可出现旧名，作历史记录，**非运行代码**。
