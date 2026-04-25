# BOM 行数据可用性与不完整配单设计

日期：2026-04-25

## 背景

BOM 导入后，系统会按型号和平台创建采集任务，再基于采集到的报价进行自动配单。现实中并不是所有型号都能采集到数据：有的平台会明确返回无结果，有的平台可能因为风控、脚本异常、跳过或取消而没有可用数据，也可能采集到了报价但被品牌、封装、价格或汇率规则过滤掉。

如果这些情况都只表现为“无法配单”，业务人员无法判断下一步该补采、人工补录、找替代料，还是可以先给客户部分报价。本设计将“行级数据可用性”作为后端统一判定能力，在默认模式下允许不完整配单，在严格模式下阻断整单，并为后续缺口处理表、人工补录、替代料推荐预留扩展点。

## 目标

- 区分“平台明确无数据”“采集不可用”“有报价但未通过配单筛选”“仍在采集”。
- 保持现有 `readiness_mode = lenient | strict` 语义：默认可继续，严格模式要求完整。
- 在配单页、行列表、导出文件中展示一致的缺口原因。
- 一期不新增复杂缺口处理表，但输出稳定的 code，便于二期落表和处理。

## 非目标

- 一期不实现人工补录报价。
- 一期不实现替代型号或等效料推荐。
- 一期不重构现有采集任务状态机。
- 一期不改变报价匹配核心规则，只复用现有配单筛选结果做汇总。

## 领域状态

新增一个由 `internal/biz` 统一推导的 BOM 行数据可用性模型。它不直接替代采集任务状态，而是从三类事实汇总：

1. 采集任务状态：以 `mpn_norm + platform_id + biz_date` 为键读取每个平台任务状态。
2. 报价缓存结果：平台成功采集的原始报价、明确无型号或无报价的结果。
3. 配单筛选结果：原始报价经过型号、品牌、封装、价格、汇率等规则后的可用候选。

行级状态如下：

| 状态 | 含义 | 典型处理 |
|------|------|----------|
| `ready` | 至少一个平台存在可用于配单的候选报价 | 正常参与自动配单 |
| `no_data` | 所有平台均已终态，且均明确无型号或无报价 | 标记无平台数据，后续人工补录或找替代料 |
| `collection_unavailable` | 所有平台都没有可用报价，且至少一个平台是采集失败、取消、跳过等不可用终态 | 优先补采或排查采集质量 |
| `no_match_after_filter` | 至少一个平台采到报价，但没有任何报价通过品牌、封装、价格、汇率等筛选 | 检查 BOM 约束、品牌别名、封装、价格数据 |
| `collecting` | 仍有 pending、running、failed_retryable 等非终态任务 | 等待采集完成或人工终止 |

状态优先级：

1. 仍有非终态任务时为 `collecting`。
2. 有可用配单候选时为 `ready`。
3. 有原始报价但无可用候选时为 `no_match_after_filter`。
4. 没有原始报价，且存在采集失败、取消、跳过等不可用终态时为 `collection_unavailable`。
5. 没有原始报价，且所有平台明确无型号或无报价时为 `no_data`。

## Readiness 行为

`lenient` 模式：

- 所有任务进入终态后，session 可以进入 `data_ready`。
- 如果存在非 `ready` 行，整单仍可进入配单，但结果应标记为不完整。
- `GetReadiness` 返回缺口统计，前端展示“不完整配单”提示。

`strict` 模式：

- 所有任务进入终态后，先计算每行 availability。
- 只有全部 BOM 行为 `ready` 时，session 才进入 `data_ready`。
- 存在 `no_data`、`collection_unavailable` 或 `no_match_after_filter` 时，session 进入 `blocked`。
- `GetReadiness` 返回阻断原因、缺口统计和可用于定位的行级状态。

无论何种模式，只要存在 `collecting` 行，都不能做最终缺口判断，也不应进入最终配单状态。

## API 设计

优先扩展现有接口，避免新增重接口。

`GetReadinessReply` 增加整单统计：

- `line_total`
- `ready_line_count`
- `gap_line_count`
- `no_data_line_count`
- `collection_unavailable_line_count`
- `no_match_after_filter_line_count`
- `can_enter_match`
- `block_reason`

`BOMLineRow` 增加行级字段：

- `availability_status`
- `availability_reason`
- `has_usable_quote`
- `raw_quote_platform_count`
- `usable_quote_platform_count`
- `resolution_status`

其中 `resolution_status` 一期默认为空或 `open`，二期用于人工补录和替代料闭环。

`PlatformGap` 保持平台维度细节，并收敛展示语义：

- `search_ui_state`: `pending | searching | succeeded | no_data | failed | skipped`
- `reason_code`: `NO_MPN | NO_QUOTES | CAPTCHA_FAILED | FETCH_FAILED | PLATFORM_SKIPPED | FILTERED_BY_MFR | FILTERED_BY_PACKAGE | PRICE_UNAVAILABLE | FX_UNAVAILABLE`

展示文案由前端或导出层基于 code 映射，后端返回稳定 code 和必要的简短 reason。

## 前端体验

`SourcingSessionPage` 的 BOM 行表增加“数据状态”列：

- `可配单`
- `无平台数据`
- `采集不可用`
- `有报价但未匹配`
- `采集中`

配单结果页顶部展示不完整提示：

> 本 BOM 有 X 行无法自动配单：Y 行无数据，Z 行采集不可用，N 行有报价但未通过筛选。

行级展开可继续显示平台维度状态，帮助业务判断下一步是补采、人工补录还是找替代料。

## 导出设计

导出文件保留所有 BOM 行。对缺口行，报价相关列留空，但增加或填充原因列：

- `无平台数据：ICGOO/LCSC 均未返回型号`
- `采集不可用：ICGOO 风控失败，LCSC 跳过`
- `有报价但未匹配：品牌或封装不匹配`

导出、页面、配单接口都应复用同一套后端 availability 结果，避免不同入口解释不一致。

## 二期扩展点

一期 availability code 未来可直接映射到缺口处理表：

```text
bom_line_gap.type:
  NO_DATA
  COLLECTION_UNAVAILABLE
  NO_MATCH_AFTER_FILTER

bom_line_gap.resolution_status:
  open
  manual_quote_added
  substitute_selected
  ignored
```

人工补录报价上线后，`manual_quote_added` 可让该行重新参与配单。替代料推荐上线后，`substitute_selected` 可关联原型号、替代型号和替代理由。

## 测试策略

后端单元测试覆盖：

- 全部平台成功且有可用候选：`lenient` 和 `strict` 都进入 `data_ready`。
- 某行所有平台明确无结果：`lenient` 为 `data_ready` 且不完整，`strict` 为 `blocked`，行状态为 `no_data`。
- 某行一个平台 `failed_terminal`、一个平台 `no_result`：行状态为 `collection_unavailable`。
- 有报价但品牌或封装过滤掉：行状态为 `no_match_after_filter`。
- 仍有 `pending` 或 `running`：行状态为 `collecting`，不进入最终配单状态。
- `GetReadiness` 的统计字段与行级状态一致。
- 导出包含缺口行和原因列。

前端测试覆盖：

- 行表能展示五类数据状态。
- 不完整提示按统计正确渲染。
- strict 阻断时展示阻断原因。
- 导出入口在不完整配单场景仍可用，并提示导出包含缺口原因。

## 实施边界

遵循 Kratos 分层：

- `internal/biz` 放置 availability 判定、readiness 规则和 code 定义。
- `internal/data` 只通过 GORM 读取任务、报价缓存和行数据，不写业务判定。
- `internal/service` 负责组装 API 返回。
- `web/` 只做展示和交互，不复制后端判定逻辑。

新增或修改 Go 文件应保持职责集中，默认不超过 300 行；若需要扩展较多逻辑，应拆分为独立 availability 文件和测试文件。
