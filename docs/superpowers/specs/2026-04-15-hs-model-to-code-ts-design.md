# 电子元器件型号到 HS 编码（code_ts）智能匹配设计

**状态：** 已定稿  
**日期：** 2026-04-15  
**范围：** 根据电子元器件型号自动返回 HS 编码，支持映射直查、数据手册下载、LLM 抽取、候选推荐与审计回溯

## 1. 背景与目标

当前系统已具备 HS 元数据与查询结果入库能力（见 `t_hs_item`）。本设计新增“按型号找 HS 编码”能力，目标是：

1. 先查已确认映射，命中即返回；
2. 未命中时自动补齐 datasheet；
3. 从 datasheet 提取结构化技术特征；
4. 基于结构化特征与 `t_hs_item` 候选让 LLM 推荐最匹配 `code_ts`；
5. 返回策略采用 **Top1 自动 + Top3 审计**。

## 2. 非目标

- 本期不实现复杂权限体系（沿用现有后台鉴权）。
- 本期不引入向量数据库（先用 §7 候选构造 + LLM 精排）。
- 本期不覆盖所有品类的完美自动判定（保留 `pending_review` 人工兜底）。

## 3. 总体方案与推荐路线

## 3.1 方案对比

### 方案 A（推荐）候选构造 + LLM 精排

- 结构化特征形成后，按 §7 依 `tech_category_ranked`（至多 3 类）分别从 `t_hs_item` 取并集候选（**不对查询结果做行级筛选或 TopN 截断**），再与特征一并输入 LLM，输出 Top3、置信度与理由。

优点：实现成本低、可解释性好，类目侧召回完整。  
缺点：候选量大时 token 与时延上升，需在工程上控制单次请求上限（分块、模型窗口、异步等，见 §7 说明）。

### 方案 B 纯 LLM 全量推荐

- 直接将大量 `t_hs_item` 数据与 datasheet 特征喂给 LLM 推荐。

优点：初期规则少。  
缺点：token 成本与时延高，稳定性差，不利于线上。

### 方案 C 向量召回 + LLM 重排

- 先向量召回候选，再 LLM 重排输出 Top3。

优点：长期召回效果更强。  
缺点：需引入新基础设施，当前阶段复杂度高。

## 3.2 决策

本期落地 **方案 A**，后续在稳定后演进到 **方案 C**。

## 3.3 分层约束（Kratos）

遵循 `service -> biz <- data`：

- `service`：接口入参校验、任务触发、响应组装；
- `biz`：状态机、重试、自动接收门槛、人工覆盖优先级等业务决策；
- `data`：表读写、文件下载、LLM/外部调用适配，不承载业务判定。

## 4. 端到端流程设计（对应业务 4 步）

1. **映射直查（快速路径）**
   - 查询 `t_hs_model_mapping`（`model + manufacturer` 唯一）。
   - 若命中且状态为 `confirmed`，直接返回 `code_ts`。

2. **数据手册补齐**
   - 未命中映射时，查询 `t_bom_quote_item.datasheet_url`。
   - 若同一 `model + manufacturer` 命中多条，按以下顺序选源：
     1) `datasheet_url` 非空且可下载；
     2) `updated_at` 最新优先；
     3) 若仍冲突，按 `id` 倒序取一条。
   - 若本地未下载：执行下载并记录 `datasheet_path`、`sha256`、下载状态。

3. **LLM 结构化抽取**
   - 读取 datasheet（PDF/文本），抽取：
     - 技术类别：**主类** `tech_category` + **有序备选** `tech_category_ranked`（见 §8.1）。枚举仍为 **半导体器件、集成电路、无源器件、电路板、其他**，字面量须完全一致；无法归入时主类填空字符串、`tech_category_ranked` 为空数组（不得自拟类别）；落库前对非集合值归一为「剔除该条/视为空」。
     - 元器件名称（component_name）
     - 封装形式（package_form）
     - 关键技术与规格参数（key_specs）
   - 落库到 `t_hs_model_features`：`tech_category` 存主类；`tech_category_ranked_json`（可选列，见 §5.2）存归一后的有序列表；`raw_extract_json` 保留模型原始输出与版本信息。

4. **候选检索 + LLM 推荐**
   - 用第 3 步的 `tech_category_ranked`（至多 3 条合法类目）分别映射为 HS6 前缀（或等价查询条件），各自查询 `t_hs_item`；**各次查询结果不在服务端做行级筛选**，按 `code_ts`（或业务主键）**并集去重**后**全部**作为候选集（细则见 §7）。
   - 将“结构化特征 + 候选集（`g_name`, `code_ts`）”喂给 LLM。
   - 输出 Top3；Top1 作为默认建议编码。

5. **返回策略（已确认）**
   - 返回 Top1（自动链路使用）；
   - 同时返回 Top3（置信度、理由）用于审计与人工复核。

## 5. 数据模型设计

## 5.1 `t_hs_model_mapping`（最终映射）

- `id` bigint PK
- `model` varchar(128)
- `manufacturer` varchar(128)
- `code_ts` char(10)
- `source` enum(`manual`,`llm_auto`)
- `confidence` decimal(5,4)
- `status` enum(`confirmed`,`pending_review`,`rejected`)
- `features_version` varchar(64)
- `recommendation_version` varchar(64)
- `created_at` / `updated_at`

约束建议：
- 唯一键：`uk_model_manufacturer(model, manufacturer)`
- 索引：`idx_code_ts(code_ts)`, `idx_status(status)`
- `code_ts` 仅允许 10 位数字字符串（保留前导 0）

## 5.2 `t_hs_model_features`（datasheet 抽取特征）

- `id` bigint PK
- `model` varchar(128)
- `manufacturer` varchar(128)
- `asset_id` bigint（FK -> `t_hs_datasheet_asset.id`）
- `tech_category` varchar(64)（主类，与抽取 JSON 中 rank=1 一致；无则空）
- `tech_category_ranked_json` json NULL（可选；归一后的有序列表，元素含 `rank`、`tech_category`、`confidence`，最多 3 条；见 §8.1）
- `component_name` varchar(128)
- `package_form` varchar(64)
- `key_specs_json` json
- `raw_extract_json` json
- `extract_model` varchar(64)
- `extract_version` varchar(64)
- `created_at`

约束建议：
- 索引：`idx_model_manufacturer(model, manufacturer)`, `idx_asset_id(asset_id)`

## 5.3 `t_hs_model_recommendation`（推荐审计）

- `id` bigint PK
- `model` varchar(128)
- `manufacturer` varchar(128)
- `run_id` char(36)
- `candidate_rank` tinyint
- `code_ts` char(10)
- `g_name` varchar(512)
- `score` decimal(5,4)
- `reason` varchar(1024)
- `input_snapshot_json` json
- `recommend_model` varchar(64)
- `recommend_version` varchar(64)
- `created_at`

约束建议：
- 唯一键：`uk_run_rank(run_id, candidate_rank)`
- 索引：`idx_model_manufacturer_created(model, manufacturer, created_at)`, `idx_run_id(run_id)`

## 5.4 `t_hs_datasheet_asset`（可选，文件资产）

- `id` bigint PK
- `model` varchar(128)
- `manufacturer` varchar(128)
- `datasheet_url` varchar(1024)
- `local_path` varchar(512)
- `sha256` char(64)
- `download_status` enum(`ok`,`failed`)
- `error_msg` varchar(512)
- `updated_at`

落地约束：

- `t_hs_datasheet_asset` 为 datasheet 资产事实来源；
- `t_hs_model_features` 增加 `asset_id` 引用资产表，避免双写不一致。

## 6. 状态机与幂等

单型号解析任务（可落入现有任务框架）建议状态流：

`init -> mapping_hit -> done`

或

`init -> need_datasheet -> datasheet_ready -> feature_extracted -> recommended -> done`

失败分支：

- `datasheet_failed`
- `extract_failed`
- `recommend_failed`

要求：

- 每步记录 `attempt_count` 与 `last_error`；
- 下载/抽取/推荐分层重试；
- 幂等键建议：`(model, manufacturer, request_trace_id)`；
- 支持短路：若近期已有高置信 `confirmed` 结果则直接返回。
- 幂等请求重复执行时，不重复生成新的 `run_id` 与推荐记录。

## 7. 候选构造策略（LLM 前置）

本节约定：**仅由抽取得到的 `tech_category_ranked`（至多 3 类）驱动 `t_hs_item` 查询范围**；对**各次查询返回的行不在服务端做筛选或 TopN 截断**，并集后全部进入推荐 LLM（精排由 LLM 在完整候选上完成）。

### 7.1 类目到查询

- 取归一后的 `tech_category_ranked` 中每一条的 `tech_category`（已属允许枚举）；按 **类目值去重**（若 rank2 与 rank1 同类，只查一次库）。
- 对每个**不同**类目，用既定规则映射到核心 HS6 前缀集合（或等价 WHERE 条件），执行 `t_hs_item` 查询，得到集合 \(S_1, S_2, \ldots\)。

### 7.2 候选并集（不作筛选）

- 候选集 \(C = \bigcup_i S_i\)，在服务端仅做 **并集去重**（推荐以 `code_ts` 为键；若同一 `code_ts` 多行，保留策略由实现固定并在评审中写明）。
- **禁止**在本阶段因 `component_name`、`package_form`、`key_specs` 等特征对 \(C\) 再做剔除、抽样或 TopN 截断（与历史「预筛 / 一级强约束 / 二级软约束收窄」相区别）。
- 可将 `component_name`、`package_form`、`key_specs` 等**作为提示词中的特征上下文**供 LLM 参考，但不得据此在服务端缩小 \(C\)。

### 7.3 工程与风险

- 三类目并集可能行数很大，导致 **token、时延、模型上下文上限** 压力；实现上可采用：**分块多次调用 LLM + 中间合并**、更长上下文模型、异步任务、或对「单次推荐调用」设硬上限并在上限触发时**仅报错/降级为人工**（仍不得在未说明的前提下静默丢弃候选子集）。具体策略由实现文档补充，本节只约束「**不对 `t_hs_item` 查询结果做服务端行级筛选**」。

### 7.4 边界

- 若 `tech_category_ranked` 归一后为空：\(C\) 为空，本步应失败或走明确降级（例如仅返回「需补充特征/人工」），不得隐式扩大为全表扫描。

## 8. LLM 提示词与输出协议

## 8.1 抽取任务（datasheet -> 特征）

输出 JSON 结构：

```json
{
  "tech_category": "",
  "tech_category_ranked": [
    {"rank": 1, "tech_category": "", "confidence": 0.0}
  ],
  "component_name": "",
  "package_form": "",
  "key_specs": {
    "voltage": "",
    "current": "",
    "power": "",
    "frequency": "",
    "temperature": "",
    "other": []
  },
  "evidence": [
    {"field": "", "quote": "", "page": 0}
  ]
}
```

约束：

- 仅输出 JSON，不输出解释文本；
- 缺失字段填空字符串，不得臆造；
- **`tech_category` 与 `tech_category_ranked`**：
  - 每一项 `tech_category` 只能为 **半导体器件**、**集成电路**、**无源器件**、**电路板**、**其他** 之一（与上列中文完全一致）；服务端对非集合值：**丢弃该条**；若全部无效则 `tech_category_ranked` 归一为 `[]`，且顶字段 `tech_category` 归一为 empty。
  - `tech_category_ranked`：**0~3 条**；按 `confidence` **从高到低**排序；`rank` 从 1 连续递增；`confidence` 为 **0~1** 的数值，表示模型自评（非概率校准亦可；**服务端构造 `t_hs_item` 候选并集时不再用 confidence 剔除类目**，至多 3 个合法且去重后的类目各自全量查询并并集，见 §7）。
  - **禁止为凑满 3 条而编造次要类别**；若仅有一类有依据则只输出 1 条。
  - 顶字段 `tech_category` 必须与归一后 `tech_category_ranked[0].tech_category` **一致**；若 `tech_category_ranked` 为空数组，则顶字段 `tech_category` 必须为空字符串。
  - **向后兼容（服务端归一）**：若模型未输出 `tech_category_ranked` 键：当顶字段 `tech_category` 合法非空时，补全为仅含一条的记录（`rank=1`，子字段 `tech_category` 与顶字段相同，`confidence=1.0` 或由配置固定默认值）；否则补全为空数组且顶字段置空。
- `evidence`：尽量覆盖 `component_name`、`package_form`、关键 `key_specs`；若 `tech_category_ranked` 含 **2 条及以上**，则对 **rank ≥ 2** 的每一条，至少有一条 `evidence.field` 能对应到该类判断所依据的字段（例如 `field` 为 `tech_category` 并引用手册中支持「次类」的原文），避免无依据即增加全量查询类目。

## 8.2 推荐任务（特征 + 候选 -> Top3）

输出 JSON 结构：

```json
{
  "best_code_ts": "",
  "best_score": 0.0,
  "top3": [
    {"rank": 1, "code_ts": "", "g_name": "", "score": 0.0, "reason": ""},
    {"rank": 2, "code_ts": "", "g_name": "", "score": 0.0, "reason": ""},
    {"rank": 3, "code_ts": "", "g_name": "", "score": 0.0, "reason": ""}
  ],
  "decision_note": ""
}
```

约束：

- `best_code_ts` 必须来自输入候选；
- `reason` 必须引用输入特征；
- 不确定时降低分值并在 `decision_note` 标记风险点。
- `run_id` 由服务端在本轮推荐开始时生成，LLM 不参与生成。

## 9. 自动接收门槛与人工复核

- `best_score >= auto_accept_threshold`：自动回写 `t_hs_model_mapping`（`source=llm_auto`，`status=confirmed`）。
- 否则：写入 `pending_review`，并返回 Top3 给前端人工确认。
- 人工确认后：
  - 写 `source=manual`；
  - 覆盖同 `(model, manufacturer)` 的自动结果；
  - 后续查询优先命中人工结果。

## 10. API 设计建议

- `POST /api/hs/resolve/by-model`
  - 入参：必填 `model`, `manufacturer`, `request_trace_id`；可选 `force_refresh`。
  - `request_trace_id` 为幂等必填字段（同一请求重试必须复用同值）。
  - 同步模式：最大阻塞 `resolve_sync_timeout_ms`（默认 8000ms）。
    - 若链路在超时内完成，直接返回结果（含 `run_id`）。
    - 若未完成，返回 `accepted` + `task_id`，由客户端轮询任务结果。
  - HTTP 语义：
    - 同步完成：`200`
    - 已转异步：`202`
  - 出参（完成态）：
    - `run_id`, `best_code_ts`, `best_score`
    - `candidates`（Top3）
    - `decision_mode=auto_top1_with_top3_audit`
    - `task_status`（`running`/`success`/`failed`）
    - `result_status`（`confirmed`/`pending_review`/`rejected`）

- `GET /api/hs/resolve/task`
  - 入参：`task_id`
  - 出参：任务状态与结果
    - `task_status`: `running`/`success`/`failed`
    - `error_code`/`error_message`（失败时）
    - 若 `success` 返回完整 `run_id + best_code_ts + candidates + result_status`

- `POST /api/hs/resolve/confirm`
  - 入参：`model`, `manufacturer`, `run_id`, `candidate_rank`, `expected_code_ts`, `confirm_request_id`。
  - 人工确认某候选为最终 `code_ts`，并回写 `t_hs_model_mapping`。
  - 若 `run_id + candidate_rank + expected_code_ts` 不一致则拒绝，避免并发误确认。
  - 仅允许确认“当前最新有效 run_id”；旧 `run_id` 返回冲突错误。
  - 使用 `confirm_request_id` 幂等：重复提交返回同一确认结果。

- `GET /api/hs/resolve/history`
  - 入参：`model`, `manufacturer`（可选 `run_id`）。
  - 查询某型号历史推荐记录与变更轨迹（按 `run_id` 可回放输入快照）。

## 10.1 状态枚举定义（避免混用）

- `task_status`（任务执行态）：
  - `running`: 下载/抽取/推荐流程进行中
  - `success`: 流程完成，已有推荐结果
  - `failed`: 流程失败，需重试
- `result_status`（映射结果态）：
  - `confirmed`: 最终可直接使用（人工或自动高置信）
  - `pending_review`: 自动结果低于阈值，待人工确认
  - `rejected`: 人工判定本轮候选均不接受

## 11. 可观测性与错误处理

关键日志字段：

- `model`, `manufacturer`
- `task_id`, `stage`
- `datasheet_url`, `datasheet_path`
- `extract_model`, `recommend_model`
- `candidate_count`, `best_score`, `final_status`
- `error_code`, `error_message`

关键指标：

- `hs_resolve_total{status}`
- `hs_resolve_stage_latency_ms{stage}`
- `hs_resolve_auto_accept_ratio`
- `hs_resolve_manual_override_total`

## 12. 验收标准

满足以下条件即通过：

1. 已存在映射时可直接返回 `code_ts`；
2. 未下载 datasheet 时可自动下载并记录路径；
3. LLM 可稳定产出结构化特征并入库；
4. 推荐结果可返回 Top1 + Top3（含 score/reason）；
5. Top1 自动写入与低置信人工待审逻辑生效；
6. 全链路有可追溯记录（抽取版本、推荐版本、输入快照）。
7. 同一幂等键重复请求不产生重复推荐结果（`run_id` 复用）；
8. 人工确认后优先级长期高于自动结果；
9. 失败状态可重试恢复，并可在任务查询接口观察到完整阶段轨迹。

## 12.1 可执行验收矩阵

1. 映射命中：
   - 输入：已存在 `confirmed` 的 `model+manufacturer`
   - 断言：`200` 返回，`task_status=success`，`result_status=confirmed`，不触发下载与LLM
2. 超时转异步：
   - 输入：模拟下载/抽取耗时 > `resolve_sync_timeout_ms`
   - 断言：`202` 返回 `task_id`，`GET /task` 最终可拿到 `run_id`
3. 低置信待审：
   - 输入：推荐 `best_score < auto_accept_threshold`
   - 断言：`result_status=pending_review`，Top3 持久化到 `t_hs_model_recommendation`
4. 幂等校验：
   - 输入：同一 `request_trace_id` 重复请求
   - 断言：复用同一 `run_id`，不新增重复推荐记录
5. 并发确认防护：
   - 输入：旧 `run_id` 与新 `run_id` 并发确认
   - 断言：仅最新有效 `run_id` 可成功，旧 `run_id` 返回冲突

## 13. 后续演进（非本期）

- 引入向量召回，在 §7 并集候选之外或之上增强召回与排序；
- 增加 prompt/version A/B 对比评估；
- 对 `pending_review` 样本做反馈学习，持续提升自动命中率。

## 14. 关联跟踪文档

- **分阶段对齐改动计划：** [2026-04-18-hs-model-resolve-design-alignment-plan.md](../plans/2026-04-18-hs-model-resolve-design-alignment-plan.md)
- **§12.1 符合度检查表与开发单：** [2026-04-18-hs-model-resolve-12-1-compliance-matrix.md](./2026-04-18-hs-model-resolve-12-1-compliance-matrix.md)

