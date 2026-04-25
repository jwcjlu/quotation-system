# BOM 缺口处理与配单方案快照二期设计

日期：2026-04-25

## 背景

一期已经把 BOM 行级数据可用性抽象为稳定状态：`ready`、`no_data`、`collection_unavailable`、`no_match_after_filter`、`collecting`。这些状态解决了“为什么这行不能自动配单”的识别问题，但还没有形成完整业务闭环：缺口没有独立处理中心，人工补录报价不能作为统一报价候选沉淀，替代料选择没有触发采集和配单链路，正式配单方案也没有结构化版本快照。

二期目标是在一期 availability 基础上补齐闭环：将缺口落表、将人工补录并入统一报价池、将替代料选择转成可采集可报价的候选，并在用户明确保存方案时生成整单配单快照。这样系统可以回答三个问题：

- 当前 BOM 哪些行仍有缺口，分别需要怎么处理。
- 某一次正式报价方案中每一行最终选择了哪个报价或为什么未解决。
- 人工补录和替代料处理是否已经进入可复用、可审计的配单链路。

## 目标

- 新增 `bom_line_gap` 缺口处理表，承接一期 availability 输出。
- 新增版本化配单方案快照：`bom_match_run` 与 `bom_match_result_item`。
- 人工补录报价写入统一报价池，并标记来源为 `manual`。
- 替代料选择后触发替代型号采集，采集完成后可作为 `substitute_match` 写入方案快照。
- 保存方案时固化每行结构化结果，导出和审计优先基于 `run_id`。
- 遵守 Kratos 分层：业务状态和规则放 `internal/biz`，GORM 持久化放 `internal/data`，API 编排放 `internal/service`。

## 非目标

- 不实现复杂审批流。
- 不实现替代料智能推荐；二期只支持用户选择或输入替代型号。
- 不把每次预览配单都落为正式方案。
- 不自动合并历史方案；只保留版本和 `superseded` 关系。
- `t_bom_match_result_item` 不额外结构化保存 `import_tax_g_name`、`hs_code_status`、`hs_customs_error`、`inspection_required`。

## 方案选择

采用“缺口表 + 配单 run 快照 + 统一报价池”的完整闭环方案。

`AutoMatch` 和 `GetMatchResult` 保持实时预览语义，不产生正式方案版本。用户点击“保存配单方案”时，后端重新读取当前 BOM 行、平台报价、人工报价、替代料报价和缺口状态，创建一次 `bom_match_run`，并为每行写入 `bom_match_result_item`。每个 run 是一版稳定报价方案，后续补录、替代和重新配单需要保存为新 run，旧 run 不被静默改写。

## 领域模型

### `bom_line_gap`

行级缺口处理中心。它绑定 `session_id + line_id + gap_type`，保存缺口类型、原因、处理状态、处理人和处理时间。它回答：“这行为什么不能自动配单，现在处理到哪了？”

缺口类型来自一期 availability：

- `NO_DATA`
- `COLLECTION_UNAVAILABLE`
- `NO_MATCH_AFTER_FILTER`

处理状态：

- `open`：待处理。
- `manual_quote_added`：已补录人工报价。
- `substitute_selected`：已选择替代料并触发后续采集。
- `resolved`：缺口已通过补采、补录或替代料报价解决。
- `ignored`：业务确认忽略。

### `bom_match_run`

一次用户显式保存的配单方案版本。它绑定 session、selection revision、run number、状态、创建人、金额和缺口统计。它回答：“这是第几版正式配单方案？”

状态：

- `saved`：已保存，可导出和审计。
- `superseded`：已有更新方案替代它。
- `canceled`：人为取消。

二期可以不暴露草稿功能；如果实现需要事务内中间态，可以在后端短暂使用 `draft`，但不作为用户可见状态。

### `bom_match_result_item`

某个 run 下每个 BOM 行的一条结构化结果。它回答：“这一版里，这一行最终怎么处理？”

`source_type`：

- `auto_match`：使用平台自动采集报价。
- `manual_quote`：使用人工补录报价。
- `substitute_match`：使用替代料报价。
- `unresolved`：仍未解决，引用 open gap。

成功行保存报价、库存、交期、金额、币种等结构字段；失败行保存 `gap_id` 和未解决原因。所有行保留 `snapshot_json`，用于固化当时完整 MatchItem 或行结果原貌。

### 统一报价池

人工补录报价写入现有 `t_bom_quote_cache / t_bom_quote_item` 体系，并标记来源为 `manual`。这样人工报价可以和平台报价一样被筛选、比较、导出和快照引用。

替代料不作为备注直接写入结果。用户选择替代型号后，系统创建替代型号采集任务；采集完成后替代料报价进入统一候选，保存方案时以 `source_type=substitute_match` 写入结果行，并保留 `original_mpn`、`substitute_mpn`、`substitute_reason`。

## 数据流

### 采集完成后同步缺口

当采集任务进入终态后，后端继续计算 availability。只要 BOM 行出现 `no_data`、`collection_unavailable` 或 `no_match_after_filter`，就 upsert `bom_line_gap`。

同步规则：

- 同一行同类 open gap 不重复创建。
- 如果后续补采、人工补录或替代料报价让该行可配单，则关闭对应 open gap，更新为 `resolved` 或更具体的处理状态。
- 缺口同步失败不影响采集任务落终态，但必须记录日志；`GetReadiness` 或 `ListLineGaps` 可以按需触发轻量同步，避免页面永久看不到缺口。

### 预览配单不落快照

`AutoMatch` 和 `GetMatchResult` 仍用于实时预览。它们可以返回当前可配单结果和缺口提示，但不创建 `bom_match_run`。这样页面刷新、轮询或反复试算不会制造无意义方案版本。

### 保存方案时落快照

新增“保存配单方案”动作。后端在一个事务中：

1. 读取 session、BOM 行、平台选择、报价候选、人工报价、替代料报价和 open gap。
2. 运行当前配单逻辑，得到每行最终候选或未解决原因。
3. 创建 `bom_match_run`，分配该 session 下递增的 `run_no`。
4. 批量写入 `bom_match_result_item`。
5. 汇总金额、成功行数、未解决行数，更新 run 汇总字段。
6. 可选地将同 session 旧 run 标记为 `superseded`。

保存失败时事务整体回滚，不能产生半截 run。

### 人工补录闭环

用户针对 open gap 补录报价时：

1. 校验 gap 存在且属于当前 session。
2. 在同一事务中写入人工报价到统一报价池。
3. 更新 gap 为 `manual_quote_added`，记录处理人和备注。
4. 后续用户可以重新预览配单或保存新方案。

如果报价入池失败，gap 状态不能更新。

### 替代料闭环

用户针对 open gap 选择替代型号时：

1. 记录原型号、替代型号和替代理由。
2. 为替代型号创建搜索任务。
3. 任务创建成功后更新 gap 为 `substitute_selected`。
4. 替代料采集完成后可重新预览。
5. 保存方案时，如果替代料报价被选中，结果行写 `source_type=substitute_match`。

如果搜索任务创建失败，不更新 `resolution_status=substitute_selected`。

## 表结构设计

### `t_bom_line_gap`

建议字段：

| 字段 | 说明 |
|------|------|
| `id` | 主键 |
| `session_id` | BOM 会话 ID |
| `line_id` | BOM 行 ID |
| `line_no` | 行号快照 |
| `mpn` | 原始型号快照 |
| `gap_type` | `NO_DATA` / `COLLECTION_UNAVAILABLE` / `NO_MATCH_AFTER_FILTER` |
| `reason_code` | 稳定原因码 |
| `reason_detail` | 人类可读说明 |
| `resolution_status` | `open` / `manual_quote_added` / `substitute_selected` / `resolved` / `ignored` |
| `resolved_by` | 处理人 |
| `resolved_at` | 处理时间 |
| `resolution_note` | 处理备注 |
| `substitute_mpn` | 已选择替代型号，可为空 |
| `substitute_reason` | 替代理由，可为空 |
| `created_at` | 创建时间 |
| `updated_at` | 更新时间 |

唯一性建议：避免同一 `session_id + line_id + gap_type` 同时存在多个 open gap。MySQL 下可以用 `active_key` 或业务层事务加锁实现，不依赖部分唯一索引。

### `t_bom_match_run`

建议字段：

| 字段 | 说明 |
|------|------|
| `id` | 主键 |
| `run_no` | session 内递增版本号 |
| `session_id` | BOM 会话 ID |
| `selection_revision` | 平台选择 revision |
| `status` | `saved` / `superseded` / `canceled` |
| `source` | `manual_save` 等 |
| `line_total` | 行总数 |
| `matched_line_count` | 已匹配行数 |
| `unresolved_line_count` | 未解决行数 |
| `total_amount` | 总金额 |
| `currency` | 币种 |
| `created_by` | 创建人 |
| `created_at` | 创建时间 |
| `saved_at` | 保存时间 |
| `superseded_at` | 被替代时间 |

唯一性建议：`session_id + run_no` 唯一。

### `t_bom_match_result_item`

建议字段：

| 字段 | 说明 |
|------|------|
| `id` | 主键 |
| `run_id` | 配单方案 ID |
| `session_id` | 冗余 session ID，便于查询 |
| `line_id` | BOM 行 ID |
| `line_no` | 行号快照 |
| `source_type` | `auto_match` / `manual_quote` / `substitute_match` / `unresolved` |
| `match_status` | `exact` / `no_match` 等 |
| `gap_id` | 未解决或处理来源 gap |
| `quote_item_id` | 选中的报价明细 |
| `platform_id` | 报价平台或 `manual` |
| `demand_mpn` | BOM 需求型号 |
| `demand_mfr` | BOM 需求品牌 |
| `demand_package` | BOM 需求封装 |
| `demand_qty` | BOM 需求数量 |
| `matched_mpn` | 命中型号 |
| `matched_mfr` | 命中品牌 |
| `matched_package` | 命中封装 |
| `stock` | 库存 |
| `lead_time` | 交期 |
| `unit_price` | 单价 |
| `subtotal` | 小计 |
| `currency` | 币种 |
| `original_mpn` | 原型号，替代料场景使用 |
| `substitute_mpn` | 替代型号 |
| `substitute_reason` | 替代理由 |
| `code_ts` | 海关编码 code_ts 快照 |
| `control_mark` | 监管条件标记快照 |
| `import_tax_imp_ordinary_rate` | 普通进口税率快照 |
| `import_tax_imp_discount_rate` | 最惠国/优惠进口税率快照 |
| `import_tax_imp_temp_rate` | 暂定进口税率快照 |
| `snapshot_json` | 完整行结果快照 |
| `created_at` | 创建时间 |

唯一性建议：`run_id + line_id` 唯一。

### 报价池扩展

建议在报价缓存或报价明细体系中补充：

| 字段 | 说明 |
|------|------|
| `source_type` | `platform` / `manual` |
| `session_id` | 人工报价所属 session，可为空 |
| `line_id` | 人工报价所属行，可为空 |
| `created_by` | 人工报价录入人 |

具体落在哪张表以实现便利为准，但匹配逻辑必须能把人工报价读成现有候选结构，避免为人工报价单独复制一套配单规则。

## API 设计

优先扩展 BOM API，不改变现有预览接口语义。

### 缺口接口

- `ListLineGaps(session_id, status?)`：列出缺口。
- `ResolveLineGapManualQuote(gap_id, quote_payload)`：补录人工报价并更新 gap。
- `SelectLineGapSubstitute(gap_id, substitute_mpn, reason)`：选择替代料并触发采集。

### 配单方案接口

- `SaveMatchRun(session_id, strategy?)`：显式保存当前配单方案，创建 run 和 result items。
- `ListMatchRuns(session_id)`：查看方案版本列表。
- `GetMatchRun(run_id)`：查看某一版方案详情。
- `CancelMatchRun(run_id)`：可选，取消方案。
- `SupersedeMatchRun(run_id)`：可选，明确标记旧方案被替代。

### 导出接口

导出优先支持 `run_id`。传入 `run_id` 时，导出严格使用 `bom_match_result_item` 快照，保证导出和当时保存的方案一致。没有 `run_id` 时，保留现有 BOM 行导出或预览导出语义。

## 错误处理

- 缺口同步失败：记录日志，不阻断采集终态；后续查询可重试同步。
- 人工补录失败：报价入池和 gap 更新同事务回滚。
- 替代料选择失败：采集任务创建失败时不更新 gap 状态。
- 保存方案失败：run 和 result items 同事务写入，失败不留下半截数据。
- 已保存 run 不静默修改：后续补录、替代和重配单保存为新 run。

## 前端体验

- BOM 行表继续展示 availability 状态和缺口原因。
- 新增缺口处理面板，按 `open`、`manual_quote_added`、`substitute_selected`、`resolved` 筛选。
- open gap 支持两个主操作：人工补录报价、选择替代料。
- 配单结果页新增“保存配单方案”按钮。
- 方案列表展示 `V1/V2`、保存时间、总金额、未解决行数、状态。
- 导出时允许选择某个已保存 run，默认使用最新 `saved` run。

## 测试策略

### `internal/biz`

- gap 状态转换：`open -> manual_quote_added -> resolved`。
- gap 状态转换：`open -> substitute_selected -> resolved`。
- 保存 run 时每行 `source_type` 判定正确。
- run 汇总统计：总行数、匹配行数、未解决行数、金额。

### `internal/data`

- 使用 GORM upsert open gap，不重复创建同类 open gap。
- 创建 run 时 `run_no` 按 session 递增。
- run 和 result item 同事务写入。
- result item 保存指定的五个海关/税率字段。
- 人工报价入池字段正确。

### `internal/service`

- `ListLineGaps` 能返回 availability 同步后的缺口。
- `ResolveLineGapManualQuote` 能写人工报价并更新 gap。
- `SelectLineGapSubstitute` 能创建替代料搜索任务并更新 gap。
- `SaveMatchRun` 能保存自动匹配、人工补录、替代料和 unresolved 四类行。
- `GetMatchRun` 和导出按 run 快照读取，不受后续报价变化影响。

### 前端

- 缺口列表按状态渲染。
- 人工补录表单提交后缺口状态变化。
- 替代料选择后展示采集中或已选择状态。
- 方案版本列表展示 run。
- 按 run 导出入口可用。

## 实施边界

- `internal/biz` 放置 gap 状态机、run 汇总和 result item 来源判定。
- `internal/data` 只实现 GORM model、migration 对应 repo 和事务持久化，不承载业务判断。
- `internal/service` 编排 API、调用匹配逻辑、调用 repo。
- 新增或修改 Go 文件默认不超过 300 行；职责过大时拆分为 gap、match run、manual quote、substitute flow 等独立文件。
- 所有数据库读写通过 GORM 完成，migration SQL 仅放在 `docs/schema/migrations/`。

## 自检

- 规格覆盖三块完整闭环：缺口落表、人工补录、替代料选择。
- 规格包含用户确认的版本化整单快照方案。
- 规格明确预览不落快照，显式保存才创建 run。
- 规格明确人工补录进入统一报价池。
- 规格明确替代料选择后触发采集。
- `t_bom_match_result_item` 只结构化保存用户指定的五个海关/税率字段。
