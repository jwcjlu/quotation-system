# HS 型号解析：§12.1 验收矩阵符合度与开发单

**状态：** 持续维护（含 2026-05 复核）  
**日期：** 2026-04-18（初版）/ 2026-05-05（更新）  
**关联设计：** [2026-04-15-hs-model-to-code-ts-design.md](./2026-04-15-hs-model-to-code-ts-design.md)（§12、§12.1）  
**分阶段改动计划：** [2026-04-18-hs-model-resolve-design-alignment-plan.md](../plans/2026-04-18-hs-model-resolve-design-alignment-plan.md)

本文档将设计稿 §12.1 的可执行断言与当前代码行为对照，并列出待办开发项。代码锚点：`internal/service/hs_resolve_service.go`、`internal/biz/hs_model_resolver.go`、`internal/biz/hs_model_confirm.go`。

---

## 1. §12.1 符合度检查表（2026-05 复核）

| §12.1 项 | 设计断言 | 当前行为（实现要点） | 符合度 |
|----------|----------|----------------------|--------|
| **1 映射命中** | 已存在 `confirmed` 的 `model+manufacturer` → HTTP `200`，`task_status=success`，`result_status=confirmed`，不触发下载与 LLM | `GetConfirmedByModelManufacturer` 命中时直接构造任务记录并返回，不进入 datasheet / extract / prefilter / recommend。 | **符合**。补充：当前实现即使 `force_refresh=true` 也保留 mapping fast-path。 |
| **2 超时转异步** | 下载/抽取等耗时超过 `resolve_sync_timeout_ms` → HTTP `202` + `task_id`；`GET /api/hs/resolve/task` 最终可拿到 `run_id` | 超时分支返回 `accepted=true` 与 `task_id`；HTTP 路由层按 `accepted` 映射 `202`（`internal/server/hs_resolve_http.go`）。轮询可获取 `run_id`。 | **符合**。 |
| **3 低置信待审** | `best_score < auto_accept_threshold` → `result_status=pending_review`，Top3 持久化到 `t_hs_model_recommendation` | 低分时 `task.ResultStatus` 与写入 mapping 的 `status` 均为 `pending_review`；`recoRepo.SaveTopN` 写入至多 3 条推荐审计。 | **符合**（推荐与审计保存成功时）。 |
| **4 幂等校验** | 同一 `request_trace_id` 重复请求 → 复用同一 `run_id`，不新增重复推荐记录 | `GetByRequestTraceID` 命中直接复用已有任务。 | **符合**。补充：`force_refresh` 仍会绕过该短路；仓储若为内存实现则不跨进程。 |
| **5 并发确认防护** | 旧 `run_id` 与新 `run_id` 并发确认 → 仅最新有效 `run_id` 可成功，旧返回冲突 | `Confirm` 对比 `GetLatestByModelManufacturer` 与请求 `run_id`，不一致则 `ErrHsResolverConfirmRunNotLatest`；HTTP 映射为 Conflict。 | **符合**。 |

---

## 2. 开发单（对齐设计 vs 当前缺口，2026-05 复核）

| ID | 标题 | 说明 | 当前状态 | 建议位置 |
|----|------|------|----------|----------|
| D1 | HTTP 202 与 §10 对齐 | 同步完成 `200`/`202` 语义。 | **已完成**（`internal/server/hs_resolve_http.go`）。 | `internal/server` |
| D2 | `force_refresh` 语义 | 明确 `force_refresh` 与 mapping fast-path 的关系。 | **已收敛**：当前「绕过幂等，不绕过 confirmed 映射」。建议在设计文档显式写明。 | `internal/biz/hs_model_resolver.go`、设计稿 §10 |
| D3 | §7 候选无服务端 TopN | 设计为无行级 TopN。 | **已收敛**：默认注入使用 `HsPrefilterUnboundedCap`。 | `internal/biz/hs_candidate_prefilter.go` |
| D4 | `tech_category_ranked` 与表结构 | 设计 §5.2 / §8.1 与特征抽取协议对齐。 | **待完成**。 | `docs/schema/migrations/`、`internal/data`、抽取客户端 |
| D5 | 任务持久化 | 轮询可观测性需跨进程稳定。 | **部分完成**：有 DB 则持久化，无 DB 回落内存。生产需确保 DB 仓储启用。 | `internal/data`、wire |
| D6 | §7.3 分块推荐与合并 | 大候选场景的分块合并策略。 | **待完成/待补文档**。 | `internal/biz`、`internal/data/hs_llm_recommend_client.go` |
| D7 | §12.1 自动化验收 | 矩阵五条纳入集成验证。 | **部分完成**：已有服务测试，建议补全 e2e 和 HTTP 码断言闭环。 | `internal/service/*_test.go`、e2e |

---

## 3. 变更记录

| 日期 | 说明 |
|------|------|
| 2026-04-18 | 初版：§12.1 对照与开发单 |
| 2026-05-05 | 复核：按当前实现更新符合度与 D1~D7 状态（标注已完成/部分完成/待完成） |
