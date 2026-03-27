# BOM 货源搜索与配单 — 接口清单

基于 [BOM货源搜索-技术设计方案.md](./BOM货源搜索-技术设计方案.md)，约定 **BOM 应用服务** 对外 HTTP API（与现有 **Agent 任务 API** 分离；Agent 仍走 [分布式采集Agent-API协议](./分布式采集Agent-API协议.md)）。

**扩展（2026-03-24）**：会话 **客户信息（1:1）**、**列表**、**PATCH 头**、**行追加/修改/删除** 见 [superpowers 规格说明](./superpowers/specs/2026-03-24-bom-session-customer-lines-design.md) 与 [接口清单](./superpowers/specs/2026-03-24-bom-session-customer-api-list.md)；实现对应 `api/bom/v1/bom.proto` 中 `ListSessions`、`PatchSession`、`CreateSessionLine`、`PatchSessionLine`、`DeleteSessionLine` 及 `CreateSession`/`GetSession` 客户字段。

**约定**

- Base path：`/api/v1`（与现有服务风格一致时可调整）。
- 鉴权：Header `Authorization: Bearer <token>` 或项目既有方式；下表略。
- 时间：ISO 8601；业务日 `biz_date` 为 **服务器本地日期** `YYYY-MM-DD`。
- `platform_id` 枚举：`find_chips` | `hqchip` | `icgoo` | `ickey` | `szlcsc`。

---

## 1. BOM 会话

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/bom-sessions` | 创建会话（可选先上传文件元信息） |
| `GET` | `/bom-sessions/{session_id}` | 会话详情（状态、当前勾选平台、revision、biz_date） |
| `PATCH` | `/bom-sessions/{session_id}` | 更新会话元数据（如名称） |
| `DELETE` | `/bom-sessions/{session_id}` | 删除/归档会话（软删由实现定） |

### `POST /bom-sessions`

**Request（JSON）**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `title` | string | 否 | 展示用标题 |
| `platform_ids` | string[] | 否 | 初始勾选平台；缺省可用系统默认「全选」 |

**Response**

| 字段 | 类型 | 说明 |
|------|------|------|
| `session_id` | string(uuid) | 新建会话 ID |
| `biz_date` | string(date) | 服务器当前业务日 |
| `selection_revision` | int | 初始为 1 |

---

## 2. 上传与解析

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/bom-sessions/{session_id}/upload` | multipart 上传 Excel，触发解析 |
| `POST` | `/bom-sessions/{session_id}/parse` | 对已存文件重新解析（换解析模式时用） |

### `POST .../upload`

**Request**：`multipart/form-data`：`file`（.xlsx/.xls），可选 `parse_mode`（`szlcsc`|`ickey`|`auto`|`custom`）。

**Response**

| 字段 | 类型 | 说明 |
|------|------|------|
| `line_count` | int | 解析出行数 |
| `parse_warnings` | string[] | 非致命告警 |

解析完成后服务端可按 §5 **重算任务**（若已有 `platform_ids`）。

---

## 3. 平台勾选（上传时 + 解析后可调）

| 方法 | 路径 | 说明 |
|------|------|------|
| `PUT` | `/bom-sessions/{session_id}/platforms` | 全量替换勾选平台，`selection_revision+1`，触发差量任务 |

### `PUT .../platforms`

**Request**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `platform_ids` | string[] | 是 | 至少一项 |
| `expected_revision` | int | 否 | 乐观锁：与当前 revision 不一致则 409 |

**Response**

| 字段 | 类型 | 说明 |
|------|------|------|
| `selection_revision` | int | 新版本号 |

---

## 4. 轮询：就绪与阻塞（核心）

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/bom-sessions/{session_id}/readiness` | 会话级：进度、是否可配单、摘要阻塞原因 |
| `GET` | `/bom-sessions/{session_id}/lines` | 行列表 + 匹配状态 + `platform_gaps` |

### `GET .../readiness`

**Response**

| 字段 | 类型 | 说明 |
|------|------|------|
| `session_id` | string | |
| `biz_date` | string(date) | |
| `selection_revision` | int | |
| `phase` | string | 如 `parsing` / `searching` / `ready_to_match` / `matching` / `done` |
| `search_progress` | object | 可选：`total_mpn_tasks`, `completed`, `failed_pending_manual` |
| `can_enter_match` | bool | 所有「型号×所选平台」均达当日终态时可 true |
| `block_reason` | string | 可读的阻塞摘要 |
| `server_time` | string | 便于前端对齐轮询间隔 |

### `GET .../lines`

**Query**：`include_quotes`（bool，默认 true）、分页参数。

**行对象（每条）**

| 字段 | 类型 | 说明 |
|------|------|------|
| `line_id` | string | |
| `line_no` | int | 展示序号 |
| `mpn` | string | 归一化型号 |
| `mfr` | string | 厂牌 |
| `package` | string | 封装 |
| `qty` | number | 数量 |
| `match_status` | string | `full_match` / `pending_confirm` / `no_match`（与需求 §3.6 对齐） |
| `platform_gaps` | array | **待确认**时必填；见下表 |
| `recommended` | object | 可选；推荐行：平台、单价、小计等 |
| `quotes_by_platform` | object | 可选；各平台报价摘要，供「显示更多」 |

**`platform_gaps[]` 元素**

| 字段 | 类型 | 说明 |
|------|------|------|
| `platform_id` | string | |
| `phase` | string | `pending` / `running` / `failed` / `no_mpn` / `ok` |
| `reason_code` | string | `NO_RECORD` / `PENDING` / `RUNNING` / `FAILED_RETRY` / `FAILED_MANUAL` / `NO_MPN` |
| `message` | string | 展示用短文案 |
| `auto_attempt` | int | 可选 |
| `manual_attempt` | int | 可选 |
| `search_ui_state` | string | 四态：`pending`（待搜索）/ `searching`（搜索中）/ `succeeded`（成功）/ `failed`（失败）；缺任务行为 `missing` |

---

## 5. 搜索任务与手动重试

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/bom-sessions/{session_id}/search-tasks/coverage` | **只读**：检查当前 `bom_session_line` × 勾选平台 与 `bom_search_task` 是否对齐（不写入） |
| `POST` | `/bom-sessions/{session_id}/search-tasks/retry` | 对手动失败或待重试项触发 **手动重试**（`manual_attempt+1`） |

### `GET .../search-tasks/coverage`

**Response（摘要）**

| 字段 | 类型 | 说明 |
|------|------|------|
| `consistent` | bool | 无缺失任务（`missing_tasks` 为空）时为 `true` |
| `orphan_task_count` | int | 库中仍存但当前行/平台集合已不覆盖的任务行数（仅统计，不自动删除） |
| `expected_task_count` | int | 期望任务数（去重 MPN × 勾选平台） |
| `existing_task_count` | int | 当前业务日下 `bom_search_task` 行数 |
| `missing_tasks` | array | 每项：`line_id`, `line_no`, `mpn_norm`, `platform_id`, `reason`（如 `no_task_row`） |

### `POST .../search-tasks/retry`

**Request**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `items` | array | 是 | 每项：`mpn`, `platform_id`（限定本会话所选平台内） |

**Response**

| 字段 | 类型 | 说明 |
|------|------|------|
| `accepted` | int | 入队重试数 |
| `rejected` | array | `{mpn, platform_id, reason}` |

---

## 6. 触发配单与结果

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/bom-sessions/{session_id}/match` | 在满足 `can_enter_match`（或部分行就绪策略若后续扩展）时执行自动配单 |
| `GET` | `/bom-sessions/{session_id}/match-result` | 当前配单结果（最新版本） |
| `POST` | `/bom-sessions/{session_id}/match-result/confirm` | 用户确认行级选择（若产品需要） |

### `POST .../match`

**Request（可选）**

| 字段 | 类型 | 说明 |
|------|------|------|
| `strategy` | string | `price_first` / `stock_first` / `lead_time_first` / `composite` |

**Response**：`match_result_id`、`version`。

### `GET .../match-result`

返回快照：行级推荐、策略、版本、时间；结构同持久化表 `bom_match_result.payload_json`。

---

## 7. 导出

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/bom-sessions/{session_id}/export` | 导出 Excel/CSV（`Accept` 或 `?format=`） |

---

## 8. 内部 / 回调（实现侧，可选对外屏蔽）

| 方法 | 路径 | 说明 |
|------|------|------|
| — | 现有 `POST .../agent/task/result` | Agent 上报后，业务层消费：写 `bom_quote_cache`、更新任务状态、**进程内事件** 通知搜索调度 |

若任务由 caichip 统一队列承载，可增加 **业务 task_id** 与 `bom_search_task.id` 映射，在结果回调中更新 BOM 域表。

---

## 9. 错误码（建议）

| HTTP | code | 说明 |
|------|------|------|
| 409 | `REVISION_CONFLICT` | 平台勾选乐观锁冲突 |
| 409 | `NOT_READY_TO_MATCH` | 尚有搜索未闭环 |
| 422 | `INVALID_PLATFORM` | 平台 ID 非法或未启用 |
| 422 | `PARSE_FAILED` | 解析失败 |

---

## 10. 相关文档

| 文档 | 说明 |
|------|------|
| [BOM货源搜索-技术设计方案.md](./BOM货源搜索-技术设计方案.md) | 状态机与边界 |
| [schema/bom_mysql.sql](./schema/bom_mysql.sql) | 表结构（MySQL） |

*版本：v1.0*
