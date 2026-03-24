# BOM 货源搜索：任务回写与配单衔接

## 任务状态机（`bom_search_task.state`）

合法终态与行为应与 `GetReadiness` / `GetBOMLines`（`platformGapFromTask`）一致：

| 状态 | 含义 |
|------|------|
| `pending` / `dispatched` / `running` | 进行中，`GetBOMLines` 展示为 pending |
| `succeeded_quotes` | 已拿到可解析报价，应写入 `bom_quote_cache`（`outcome` 多为 `ok`） |
| `succeeded_no_mpn` | 平台无匹配型号，`bom_quote_cache.outcome` 为 `no_mpn_match` |
| `failed` / `cancelled` | 失败或取消，`last_error` 可读 |

迁移：`pending` → `running`/`dispatched`（由调度/Agent）→ **`succeeded_quotes` | `succeeded_no_mpn` | `failed`**（由 `SubmitBomSearchResult` 或等价回写入口落终态）。

## `bom_quote_cache.quotes_json` 最小约定

与 `biz.Quote` JSON 标签对齐，建议数组元素至少包含：

- `platform`（可省略，回传时由服务端按 `platform_id` 补全）
- `matched_model`
- `manufacturer`
- `package`（可选，参与配单「型号/厂牌/封装」完全匹配）
- `stock`（数值）
- `lead_time`（字符串）
- `unit_price`（数值）

支持形态：

- JSON 数组：`[{...},{...}]`
- 单对象：`{...}`
- 包装：`{"quotes":[...]}`

阶梯价等可放在 `price_tiers` 字符串字段（与 `biz.Quote` 一致）；首版配单仍以 `unit_price` 为主。

## `SubmitBomSearchResult` 请求要点

HTTP：`POST /api/v1/bom-sessions/{session_id}/search-results`  
鉴权：`bom_search_callback.api_keys`（`X-API-Key` 或 `Authorization: Bearer <key>`）。

Body 字段：

| 字段 | 必选 | 说明 |
|------|------|------|
| `session_id` | 是 | 与路径一致 |
| `mpn_norm` | 是 | 将与内部 `NormalizeMPNForTask` 一致化 |
| `platform_id` | 是 | 与任务行一致 |
| `caichip_task_id` | 推荐 | 若库内已有非空值，必须与之一致，否则 `409 SEARCH_TASK_ID_MISMATCH` |
| `status` | 是 | `succeeded_quotes` \| `succeeded_no_mpn` \| `failed` |
| `error_message` | `failed` 时建议 | 写入 `last_error` |
| `quotes_json` | `succeeded_quotes` 时 | 原始 JSON 字符串 |
| `no_mpn_detail_json` | 可选 | 写入 `no_mpn_detail` |

业务日 `biz_date` 取自 `bom_session`，请求中无需重复传入。

## 配单闭环

会话 BOM 的 `bom_id` 即 `session_id`（UUID）。`AutoMatch` 在 DB 就绪时从 `bom_search_task` / `bom_quote_cache` 聚合为 `[]*biz.ItemQuotes`，再 `searchRepo.SaveQuotes(bom_id, …)` 后执行既有选型逻辑。
