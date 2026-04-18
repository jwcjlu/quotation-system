# HS 元数据管理与查询入库设计

**状态：** 已定稿  
**日期：** 2026-04-15  
**范围：** HS 核心元数据维护、HS 查询任务（手动/定时）、外部接口数据入本地库

## 1. 背景与目标

当前已存在两类信息：

- `docs/hs_meta.md`：常见 HS 元数据（技术类别、元器件名称、核心 HS 编码前 6 位、说明）。
- `docs/hs_query_api`：外部 HS 查询接口示例，`filterField=CODE_TS`，`filterValue` 用于按编码过滤。

核心诉求：

1. `hs_meta` 不是静态文档，后续会新增/调整，需提供可管理接口与页面。
2. 基于 `hs_meta` 中的**核心 HS 编码（前 6 位）**，驱动 `hs_query_api` 查询并落库。
3. 支持两种触发方式：手动触发、定时触发。

## 2. 非目标

- 不在本期实现复杂权限体系（先复用现有后台权限能力）。
- 不做外部接口协议改造（请求/响应结构按现有 `hs_query_api` 约定）。
- 不做多数据源聚合（先只接 `hs_query_api`）。

## 3. 总体架构

```
HS 管理页面
   ├─ 维护 HS 元数据（核心码前6位）
   └─ 手动触发查询任务
            │
            ▼
       HS 应用服务（service）
            │
            ▼
       HS 领域层（biz）
   ├─ 元数据管理用例
   ├─ 查询任务编排（按核心码批量调用）
   └─ 定时任务调度入口
            │
            ▼
       数据层（data）
   ├─ hs_meta repo（CRUD）
   ├─ hs_sync_job repo（任务记录）
   ├─ hs_item repo（查询结果入库）
   └─ hs_query_api client（外部HTTP调用）
```

设计遵循仓库 Kratos 分层：`service -> biz <- data`。业务决策（如去重、覆盖策略、失败重试）放 `biz`，`data` 只负责持久化与外部访问。

## 4. 数据模型设计

## 4.1 HS 元数据表（`t_hs_meta`）

建议字段：

- `id` bigint PK
- `category` varchar(64)：技术类别（如半导体器件）
- `component_name` varchar(128)：元器件名称
- `core_hs6` char(6)：核心 HS 编码前 6 位（如 `854110` 或按你现习惯存 `8541xx`，建议统一纯数字）
- `description` varchar(512)：简要说明
- `enabled` tinyint(1)：是否启用（仅启用项参与同步）
- `sort_order` int：排序
- `created_at` / `updated_at`

约束建议：

- `core_hs6` 建索引（任务查询高频使用）。
- `core_hs6 + component_name` 唯一约束（避免同一项重复配置）。

## 4.2 查询任务表（`t_hs_sync_job`）

记录每次手动/定时同步执行：

- `id` bigint PK
- `trigger_type` enum(`manual`,`schedule`)
- `status` enum(`running`,`success`,`partial_success`,`failed`)
- `request_snapshot` json（本次使用的核心码清单、分页参数）
- `result_summary` json（成功/失败数量、异常摘要）
- `started_at` / `finished_at`
- `created_by`（手动触发时记录操作者）

## 4.3 HS 查询结果表（`t_hs_item`）

按 `hs_query_api` 返回字段建标准化结构：

- `id` bigint PK
- `job_id` bigint（来源任务）
- `code_ts` varchar(16)（完整 10 位商品编码）
- `g_name` varchar(512)（商品名称）
- `unit_1` varchar(16)
- `unit_2` varchar(16)
- `control_mark` varchar(64)
- `source_core_hs6` char(6)（由哪个核心码拉取而来）
- `raw_json` json（保留原始行，便于后续补字段）
- `created_at` / `updated_at`

约束建议：

- 唯一键：`code_ts`（如同一编码多次同步则 upsert 更新）。
- 二级索引：`source_core_hs6`、`updated_at`。

## 5. 接口设计

## 5.1 HS 元数据管理接口

- `GET /api/hs/meta/list`
  - 分页查询，支持 `category`、`component_name`、`core_hs6`、`enabled` 过滤。
- `POST /api/hs/meta/create`
  - 新增元数据，校验 `core_hs6` 格式（6 位数字）与唯一性。
- `POST /api/hs/meta/update`
  - 更新元数据（含启用状态）。
- `POST /api/hs/meta/delete`
  - 软删或硬删（二选一，建议软删 + `enabled=0`）。

## 5.2 查询任务接口

- `POST /api/hs/sync/run`
  - 手动触发；支持两种模式：
    - `mode=all_enabled`：拉取全部启用核心码。
    - `mode=selected`：传入指定 `core_hs6[]`。
- `GET /api/hs/sync/jobs`
  - 查询任务列表与状态。
- `GET /api/hs/sync/job_detail?id=xxx`
  - 查看单任务执行明细（失败原因、每个核心码结果）。

## 5.3 查询结果接口（给业务检索/页面展示）

- `GET /api/hs/items`
  - 支持 `code_ts`、`g_name`、`source_core_hs6` 过滤。
- `GET /api/hs/items/:code_ts`
  - 查看单个 HS 条目详情（含最近同步来源）。

## 6. `hs_query_api` 调用与入库规则

## 6.1 参数映射（关键约束）

每个 `core_hs6` 触发一次（或多页）外部查询，请求体核心映射：

- `paramName = "CusComplex"`（按现有示例固定）
- `filterField = "CODE_TS"`（按现有示例固定）
- `filterValue = core_hs6`（来自 `t_hs_meta.core_hs6`）

说明：你提到“`filterValue` 就是 `hs_meta` 的核心 HS 编码（前 6 位）”，本设计将其作为强约束写入实现与校验逻辑。

## 6.2 分页策略

根据返回字段 `pageSumCount` / `totalRows`：

1. 先查第一页获取总页数；
2. 循环拉取后续页；
3. 单页失败可重试（最多 3 次，指数退避）；
4. 全部页完成后汇总该核心码执行结果。

## 6.3 入库策略

- 同步结果写入 `t_hs_item`，以 `code_ts` 为主键做 upsert：
  - 存在则更新 `g_name/unit/control_mark/raw_json/source_core_hs6/updated_at`。
  - 不存在则插入新行。
- 每批次写入事务边界：
  - 建议“按核心码 + 分页分批事务”，避免超大事务。

## 7. 定时任务设计

- 配置项（`internal/conf`）：
  - `hs_sync_enabled`（是否启用）
  - `hs_sync_cron`（cron 表达式）
  - `hs_sync_rows_per_page`（分页大小，默认 100）
  - `hs_sync_timeout_seconds`（单请求超时）
- 执行逻辑：
  1. 读取 `enabled=1` 的核心码列表；
  2. 逐个核心码调用外部接口并入库；
  3. 写任务记录到 `t_hs_sync_job`；
  4. 输出结构化日志（job_id、core_hs6、page、耗时、结果）。

并发建议：

- 首版串行执行，保证稳定与可追踪；
- 二期可按核心码并发（限制 worker 数，避免触发外部限流）。

## 8. 管理页面设计

页面分三块：

1. **HS 元数据管理页**
   - 列表 + 筛选 + 新增/编辑/启停
   - 列展示：类别、名称、核心码、说明、启用状态、更新时间
2. **同步任务页**
   - 手动触发按钮（全量/选中）
   - 任务列表（状态、触发方式、耗时、成功/失败统计）
   - 任务详情弹窗（失败核心码与原因）
3. **HS 结果查询页**
   - 按 `code_ts`/名称/核心码检索
   - 展示计量单位、监管条件、最近更新时间

前端交互重点：

- 手动触发后轮询任务状态（running -> success/failed）。
- 任务失败时展示可读错误（网络超时、接口异常、数据解析失败）。

## 9. 错误处理与可观测性

- 错误分层：
  - 外部请求错误（HTTP/超时/响应非 success）
  - 解析错误（JSON 格式或字段缺失）
  - 入库错误（唯一键冲突之外的 DB 错误）
- 日志字段统一：
  - `job_id`、`trigger_type`、`core_hs6`、`page_no`、`elapsed_ms`、`error_code`
- 指标建议：
  - `hs_sync_job_total{status}`
  - `hs_sync_api_latency_ms`
  - `hs_sync_item_upsert_total`

## 10. 安全与合规

- 外部接口 URL/token 统一放配置，不硬编码到业务代码。
- 管理接口需挂现有后台鉴权中间件。
- `request_snapshot` / `raw_json` 如包含敏感字段，按脱敏策略落库（当前示例字段风险较低）。

## 11. 验收标准

满足以下条件即通过：

1. 可在页面新增/编辑/启停 `hs_meta`，并在接口层校验 `core_hs6` 合法性。
2. 手动触发可按 `hs_meta.core_hs6` 拉取数据，任务状态可查。
3. 定时任务按配置自动执行，且失败不会中断下一次调度。
4. `hs_query_api` 的 `filterValue` 实际使用 `hs_meta` 核心 6 位编码。
5. 查询结果可在本地库稳定检索，重复同步无重复脏数据（upsert 生效）。

## 12. 后续演进（非本期）

- 支持核心码优先级与分组调度（热点先同步）。
- 增加“仅增量更新”模式（按更新时间对比）。
- 为 `t_hs_item` 增加历史版本表，保留税则变化轨迹。
