# HS 型号解析：与设计稿对齐的改动计划

> **关联规格：** [2026-04-15-hs-model-to-code-ts-design.md](../specs/2026-04-15-hs-model-to-code-ts-design.md)  
> **符合度跟踪：** [2026-04-18-hs-model-resolve-12-1-compliance-matrix.md](../specs/2026-04-18-hs-model-resolve-12-1-compliance-matrix.md)  
> **既有实现计划（全链路落地）：** [2026-04-15-hs-model-to-code-ts-implementation.md](./2026-04-15-hs-model-to-code-ts-implementation.md)（本文件侧重**与设计偏差**的增量改动）

**目标：** 在保持 Kratos 分层（设计 §3.3：`service -> biz <- data`）前提下，使行为与数据逐步对齐设计 §4–§8、§10、§12；§2 非目标与 §13 演进不在本期范围。

---

## 阶段与任务

### P0：契约与可验收（设计 §10、§12、§12.1）

| ID | 任务 | 设计锚点 | 主要路径 |
|----|------|----------|----------|
| P0-1 | HTTP `200`（同步完成）与 `202`（转异步）与实现一致 | §10 | `api/bom/v1/bom.proto` / `internal/server`；补 HTTP 层或文档二选一 |
| P0-2 | `force_refresh` 与映射短路、幂等短路顺序定案并实现 | §10 | `internal/biz/hs_model_resolver.go`；必要时修订设计一句 |
| P0-3 | 任务持久化：`GetResolveTask` 跨进程可见，`run_id` 幂等可追溯 | §6、§12.1-2、-4 | GORM `HsModelTaskRepo` + `docs/schema/migrations/` + `cmd/server/wire_gen.go`（替换内存 task repo） |
| P0-4 | §12.1 五条自动化验收（含 P0-1 则断言真实状态码） | §12.1 | `internal/service/*_test.go` 或 e2e |

### P1：数据模型与特征（设计 §4.2–§4.3、§5.2、§8.1）

| ID | 任务 | 设计锚点 | 主要路径 |
|----|------|----------|----------|
| P1-1 | `t_hs_model_features.tech_category_ranked_json` 迁移与模型 | §5.2 | `docs/schema/migrations/`、`internal/data/models.go` |
| P1-2 | 抽取协议：中文五类、`tech_category_ranked`、归一、向后兼容、`evidence` | §8.1 | `internal/data/hs_llm_extract_client.go`、`hs_llm_feature_extractor.go`；归一策略若在 biz 则放 `internal/biz` |
| P1-3 | 特征落库与 `asset_id`、版本字段闭环 | §5.2、§5.4 | `internal/data` repo；`internal/biz` 编排调用（业务状态仍在 biz） |
| P1-4 | BOM 多行选源与 §4.2 顺序一致 | §4.2 | `internal/data/hs_bom_quote_item_*`、`internal/service/hs_resolve_service.go` 的 datasheet 候选构造 |

### P2：候选与推荐（设计 §4.4、§7、§8.2）

| ID | 任务 | 设计锚点 | 主要路径 |
|----|------|----------|----------|
| P2-1 | 按 `tech_category_ranked` 多类目查 `t_hs_item`，并集去重，**不做服务端 TopN 截断** | §7.1–§7.2 | `internal/data/hs_item_query_repo.go`、`internal/biz/hs_candidate_prefilter.go` |
| P2-2 | `tech_category_ranked` 为空：失败或明确降级，禁止全表扫描 | §7.4 | `internal/biz/hs_model_resolver.go` |
| P2-3 | 大候选：分块 LLM + **全局合并语义**（文档化 + 实现） | §7.3 | `internal/data/hs_llm_recommend_client.go`、`internal/biz` 编排 |
| P2-4 | 推荐 JSON、审计字段与 §8.2 / §5.3 一致（含 `reason`、`input_snapshot_json`） | §8.2、§5.3 | `hs_llm_recommend_client.go`、`hs_model_recommendation_repo.go` |
| P2-5 | `run_id` 存储类型与生成策略（设计 `char(36)` vs 当前 `model\|mfr\|trace`）统一 | §5.3、§6 | 迁移改列宽或改为 UUID，并更新 proto/客户端约定 |

### P3：观测与整体验收（设计 §11、§12）

| ID | 任务 | 设计锚点 | 主要路径 |
|----|------|----------|----------|
| P3-1 | 日志字段与指标与 §11 核对 | §11 | `hs_resolve_service`、`biz` observer / 指标埋点 |
| P3-2 | §12 条 1–9 走查（在 P0–P2 完成后） | §12 | 人工 + 集成测试；更新符合度矩阵结论 |

---

## 建议实施顺序

1. **P0-3 → P0-2 → P0-1 → P0-4**（先任务持久化与语义，再 HTTP 与自动化验收）。  
2. **P1**（特征与列为 §7 输入前提）。  
3. **P2**（与设计偏差最大的 §7）。  
4. **P3**（发布前核对）。

---

## 与符合度文档「开发单」映射

| 开发单（矩阵文档） | 本计划 |
|--------------------|--------|
| D1 | P0-1 |
| D2 | P0-2 |
| D3 | P2-1 |
| D4 | P1-1、P1-2、P1-3 |
| D5 | P0-3 |
| D6 | P2-3 |
| D7 | P0-4 |

---

## 变更记录

| 日期 | 说明 |
|------|------|
| 2026-04-18 | 初版：对齐设计的分阶段改动计划 |
