# HS 型号解析：§12.1 验收矩阵符合度与开发单

**状态：** 跟踪用（随实现更新）  
**日期：** 2026-04-18  
**关联设计：** [2026-04-15-hs-model-to-code-ts-design.md](./2026-04-15-hs-model-to-code-ts-design.md)（§12、§12.1）  
**分阶段改动计划：** [2026-04-18-hs-model-resolve-design-alignment-plan.md](../plans/2026-04-18-hs-model-resolve-design-alignment-plan.md)

本文档将设计稿 §12.1 的可执行断言与当前代码行为对照，并列出待办开发项。代码锚点：`internal/service/hs_resolve_service.go`、`internal/biz/hs_model_resolver.go`、`internal/biz/hs_model_confirm.go`。

---

## 1. §12.1 符合度检查表

| §12.1 项 | 设计断言 | 当前行为（实现要点） | 符合度 |
|----------|----------|----------------------|--------|
| **1 映射命中** | 已存在 `confirmed` 的 `model+manufacturer` → HTTP `200`，`task_status=success`，`result_status=confirmed`，不触发下载与 LLM | `!ForceRefresh` 且 `GetConfirmedByModelManufacturer` 命中时直接构造任务记录并返回，不进入 datasheet / extract / prefilter / recommend。 | **语义基本符合**。**边界**：`force_refresh=true` 时跳过映射短路与幂等短路，会走全链路。HTTP 层未单独为「仅成功」区分特殊码。 |
| **2 超时转异步** | 下载/抽取等耗时超过 `resolve_sync_timeout_ms` → HTTP `202` + `task_id`；`GET /api/hs/resolve/task` 最终可拿到 `run_id` | 超时分支返回 `accepted=true` 与 `task_id`；`task_id` 与 resolver 内 `run_id` 均为 `model\|manufacturer\|request_trace_id` 形式，轮询可拿到同一标识。 | **语义部分符合**。异步接纳与 `run_id` 可观测性符合。**缺口**：默认 Kratos HTTP 成功多为 `200`，未见按 `accepted` 返回 `202`；`internal/service/hs_resolve_service_test.go` 中超时用例未断言 HTTP 状态码。 |
| **3 低置信待审** | `best_score < auto_accept_threshold` → `result_status=pending_review`，Top3 持久化到 `t_hs_model_recommendation` | 低分时 `task.ResultStatus` 与写入 mapping 的 `status` 均为 `pending_review`；`recoRepo.SaveTopN` 写入至多 3 条推荐审计。 | **符合**（推荐与审计保存成功时）。 |
| **4 幂等校验** | 同一 `request_trace_id` 重复请求 → 复用同一 `run_id`，不新增重复推荐记录 | `GetByRequestTraceID` 命中则直接返回已有任务，不重复跑推荐。 | **符合**（同一 `HsModelTaskRepo` 生命周期内）。**注意**：`force_refresh` 会绕过该短路；默认 Wire 路径下任务仓储为内存实现时不跨进程。 |
| **5 并发确认防护** | 旧 `run_id` 与新 `run_id` 并发确认 → 仅最新有效 `run_id` 可成功，旧返回冲突 | `Confirm` 对比 `GetLatestByModelManufacturer` 与请求 `run_id`，不一致则 `ErrHsResolverConfirmRunNotLatest`；HTTP 映射为 Conflict。 | **符合**。 |

---

## 2. 开发单（对齐设计 vs 当前缺口）

| ID | 标题 | 说明 | 建议位置 |
|----|------|------|----------|
| D1 | HTTP 202 与 §10 对齐 | 设计约定同步完成 `200`、转异步 `202`。实现以 body 的 `accepted` 表达异步时，需在 HTTP 层或 proto 注解中显式返回 `202`，或修订设计为「仅以 body 为准」。 | `internal/server`、`api/bom/v1/bom.proto` |
| D2 | `force_refresh` 语义 | 当前 `ForceRefresh=true` 会跳过映射命中与幂等短路。需与设计 §10 对齐（例如：仅跳过幂等但仍尊重已确认映射，或文档明确「强制重跑全链路」）。 | `internal/biz/hs_model_resolver.go`、设计稿 §10 |
| D3 | §7 候选无服务端 TopN | 设计：类目查询并集、不做行级 TopN。当前 `HsCandidatePrefilter` 仍带 `limit`（与 `MaxCandidates` 相关的 `prefilterTopN`）。 | `internal/biz/hs_candidate_prefilter.go`、`internal/data/hs_item_query_repo.go` |
| D4 | `tech_category_ranked` 与表结构 | 设计 §5.2 / §8.1 与 `HsModelFeatures`、抽取协议对齐（含迁移）。 | `docs/schema/migrations/`、`internal/data`、抽取客户端 |
| D5 | 任务持久化 | 超时后轮询依赖任务可查；内存 `HsModelTaskRepo` 不跨进程。生产验收需持久化任务表或与设计一致的仓储实现。 | `internal/data`、wire |
| D6 | §7.3 分块推荐与合并 | 大候选时分块调用 LLM 与全局合并策略需在实现文档与代码中落地，避免块内 Top3 误作全集最优。 | `internal/biz`、`internal/data/hs_llm_recommend_client.go` |
| D7 | §12.1 自动化验收 | 将上表五条纳入 e2e 或 API 集成测试（若修 D1 则断言真实 HTTP 状态码）。 | `internal/service/*_test.go` 或独立 e2e |

---

## 3. 变更记录

| 日期 | 说明 |
|------|------|
| 2026-04-18 | 初版：§12.1 对照与开发单 |
