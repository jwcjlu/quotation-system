# BOM 搜索任务状态展示与重试设计

## 1. 背景

当前 BOM 搜索流程已经有 `t_bom_search_task` 业务状态和 `RetrySearchTasks` 重试接口，但会话页面只在 BOM 行缺口中展示很薄的一层 `search_ui_state`。当实际搜索任务已经完成、但页面仍显示 `searching` 时，用户无法判断是：

- BOM 搜索任务状态未更新；
- Agent 调度任务仍在 `pending` 或 `leased`；
- Agent 已经 `finished`，但 BOM 业务状态没有完成落库；
- 任务行缺失或与 BOM 行、平台选择不一致。

因此需要为单个 BOM 会话提供独立的搜索任务状态视图，并支持整单异常重试和单任务重试。

## 2. 目标

1. 在 BOM 会话页展示该 BOM 单所有搜索任务的汇总状态和明细状态。
2. 明确区分 BOM 业务状态与 Agent 调度状态，便于排查“为什么还在 searching”。
3. 支持一键重试所有异常任务，也支持逐条重试单个 `MPN + platform` 任务。
4. `no_result` 表示搜索完成但平台无报价，不进入默认异常批量重试，但允许用户单条手动重试。
5. 后端输出可直接消费的 `retryable` 与 UI 状态，前端不复制业务状态机。

## 3. 非目标

- 不改造 Agent 通用调度重试策略。
- 不改变现有 `RetrySearchTasks` 的基本语义。
- 不把 `GetSessionSearchTaskCoverage` 改成任务明细接口；coverage 仍只负责一致性检查。
- 不在前端推导 BOM 搜索状态机。

## 4. 接口设计

新增接口：

```text
GET /api/v1/bom-sessions/{session_id}/search-tasks
```

建议新增 proto：

```proto
message ListSessionSearchTasksRequest {
  string session_id = 1;
}

message SearchTaskStatusSummary {
  int32 total = 1;
  int32 pending = 2;
  int32 running = 3;
  int32 succeeded = 4;
  int32 no_result = 5;
  int32 failed = 6;
  int32 skipped = 7;
  int32 cancelled = 8;
  int32 missing = 9;
  int32 dispatch_pending = 10;
  int32 dispatch_leased = 11;
  int32 dispatch_finished = 12;
  int32 dispatch_failed = 13;
}

message SessionSearchTaskRow {
  string line_id = 1;
  int32 line_no = 2;
  string mpn = 3;
  string mpn_norm = 4;
  string platform_id = 5;
  string search_state = 6;
  string search_ui_state = 7;
  string caichip_task_id = 8;
  string dispatch_state = 9;
  string dispatch_result_status = 10;
  int32 attempt = 11;
  int32 retry_max = 12;
  string leased_to_agent_id = 13;
  string lease_deadline_at = 14;
  string last_error = 15;
  string updated_at = 16;
  bool retryable = 17;
}

message ListSessionSearchTasksReply {
  string session_id = 1;
  SearchTaskStatusSummary summary = 2;
  repeated SessionSearchTaskRow tasks = 3;
}
```

字段说明：

- `search_state`：来自 `t_bom_search_task.state`，缺失任务填 `missing`。
- `search_ui_state`：后端映射给前端展示，建议枚举为 `pending | searching | succeeded | no_data | failed | skipped | cancelled | missing`。
- `dispatch_state`：来自 `t_caichip_dispatch_task.state`，仅当存在 `caichip_task_id` 且能关联到调度任务时返回。
- `last_error`：优先返回 BOM 搜索任务错误；没有时返回调度任务错误。
- `retryable`：后端计算，前端只按该布尔值决定是否显示单条重试入口。

## 5. 状态与重试规则

### 5.1 UI 状态映射

| BOM 状态 | UI 状态 | 说明 |
| --- | --- | --- |
| `pending` | `pending` | 待派发或待认领 |
| `running` | `searching` | 已进入搜索执行链路 |
| `failed_retryable` | `failed` | 可重试失败 |
| `failed_terminal` | `failed` | 终态失败，但允许人工重试 |
| `succeeded` | `succeeded` | 已获取报价 |
| `no_result` | `no_data` | 搜索完成但平台无报价 |
| `skipped` | `skipped` | 已跳过，可人工重试 |
| `cancelled` | `cancelled` | 已取消，不默认重试 |
| 缺失任务行 | `missing` | BOM 行与平台应该有任务但实际没有 |

### 5.2 批量重试范围

“重试异常任务”默认只重试：

- `failed_retryable`
- `failed_terminal`
- `skipped`
- `missing`

不默认批量重试：

- `no_result`：搜索完成但无报价，不视为异常；
- `pending` / `running`：避免重复派发或覆盖正在执行的任务；
- `succeeded`：已有报价；
- `cancelled`：通常由 BOM 行删除、平台移除等业务动作产生。

### 5.3 单条重试范围

单条“重试”按钮允许：

- 批量重试范围内的异常任务；
- `no_result`，用于用户怀疑平台临时无数据或需要重新抓取的场景。

单条重试不允许：

- `pending`
- `running`
- `succeeded`
- `cancelled`

## 6. 后端设计

### 6.1 biz 层

新增或扩展 `internal/biz`：

- `MapBOMSearchTaskUIState(state string) string`
- `CanRetryBOMSearchTask(state string, mode RetryMode) bool`
- `BuildSearchTaskSummary(rows []SearchTaskStatusRow) SearchTaskStatusSummary`

`RetryMode` 至少区分：

- `batch_anomaly`：整单异常重试；
- `single_manual`：单条手动重试。

这样可以把 `no_result` 的“批量不重试、单条可重试”规则放在业务层。

### 6.2 data 层

`internal/data` 只负责 GORM 查询和结构映射：

1. 按 `session_id` 读取 BOM 行。
2. 按 `session_id` 读取 `t_bom_search_task`。
3. 按非空 `caichip_task_id` 批量读取 `t_caichip_dispatch_task`。
4. 不在 data 层判断可重试规则，不在 data 层做状态机决策。

建议在 `BOMSearchTaskRepo` 增加只读方法，例如：

```go
ListSearchTaskStatusRows(ctx context.Context, sessionID string) ([]biz.SearchTaskStatusRow, error)
```

如果需要读取 dispatch 表，可通过独立 repo 方法或在 data 层提供只读组合查询；业务规则仍由 biz/service 处理。

### 6.3 service 层

`BomService.ListSessionSearchTasks` 负责：

1. 读取 session，确认 `biz_date` 和平台列表。
2. 读取 BOM 行与现有搜索任务。
3. 根据 `line × selected_platform` 构造期望任务集合。
4. 对缺失项生成 `search_state=missing` 的明细行。
5. 关联 dispatch 状态。
6. 调用 biz 方法生成 `search_ui_state`、`retryable` 与 summary。

`RetrySearchTasks` 可复用现有接口；若后续实现发现 `missing` 无法通过现有重试接口恢复，应在 service 中将 `missing` 转换为重新 upsert pending task，再调用 merge dispatch。

## 7. 前端设计

### 7.1 API

`web/src/api/bomSession.ts` 新增：

```ts
export async function listSessionSearchTasks(
  sessionId: string
): Promise<ListSessionSearchTasksReply>
```

`web/src/api/types.ts` 新增对应类型。

### 7.2 页面布局

在 `SourcingSessionPage` 的 BOM 行表上方新增“搜索任务状态”区，采用顶部总览布局：

- 顶部工具栏：刷新、重试异常任务。
- 计数卡：全部、待执行、执行中、已完成、无数据、失败、缺失任务。
- 明细表：行号、MPN、平台、搜索状态、Agent 状态、尝试次数、最后错误、更新时间、操作。

交互规则：

- 页面初次加载时并行加载 BOM 行和搜索任务状态。
- 点击刷新时同时刷新 session、BOM 行、搜索任务状态。
- 点击“重试异常任务”时，从任务列表中过滤 `retryable=true` 且 `search_ui_state != no_data` 的任务。
- 点击单条“重试”时提交该行 `mpn + platform_id`。
- 重试后刷新搜索任务状态和 BOM 行。

### 7.3 文案

推荐中文文案：

- 区块标题：`搜索任务状态`
- 批量按钮：`重试异常任务`
- 单条按钮：`重试`
- 空状态：`当前没有搜索任务`
- 无可重试任务提示：`暂无可重试的异常任务`
- `no_result` 展示：`无报价`
- `running` 展示：`搜索中`

## 8. 测试计划

后端：

- `MapBOMSearchTaskUIState` 覆盖所有状态。
- `CanRetryBOMSearchTask` 覆盖批量与单条模式，特别是 `no_result`。
- `ListSessionSearchTasks` 覆盖：
  - 正常任务明细；
  - dispatch 状态关联；
  - 缺失任务；
  - summary 计数；
  - last_error 优先级；
  - 无 DB 时返回 `DB_DISABLED`。

前端：

- 渲染搜索任务状态总览。
- 渲染任务明细和 Agent 状态。
- “重试异常任务”只提交异常任务，不提交 `no_result`。
- `no_result` 行显示单条重试。
- `pending/running/succeeded/cancelled` 不显示重试按钮。
- 重试完成后刷新任务列表。

## 9. 风险与处理

- **状态重复定义风险**：所有重试规则放在 `internal/biz`，前端只消费 `retryable`。
- **任务缺失修复语义不明确**：先在设计中明确 `missing` 应可批量重试；实现时若现有接口不足，补 service 逻辑创建 pending task。
- **页面信息过载**：默认展示汇总和异常优先排序；完整列表可按状态筛选作为后续增强。
- **`searching` 误导风险**：同时展示 `search_state` 与 `dispatch_state`，让用户能看出是 BOM 业务状态未落库，还是 Agent 仍未完成。

## 10. 验收标准

1. BOM 会话页能看到搜索任务汇总和每个 `MPN + platform` 的状态。
2. 当任务已经完成时，页面不再只显示模糊的 `searching`，而能展示 `succeeded/no_result/failed` 等明确状态。
3. 用户可以一键重试异常任务。
4. 用户可以单条重试某个平台任务，包括 `no_result`。
5. 后端测试和前端测试覆盖主要状态和重试规则。
