# Agent 调度任务自动重试上限与终态失败设计

## 1. 背景

当前通用 Agent 调度任务表 `t_caichip_dispatch_task` 已具备以下基础能力：

- `attempt`：记录执行尝试次数
- `next_claim_at`：控制任务何时可再次被认领
- `last_error`：记录最近一次失败信息
- `state`：当前仅有 `pending`、`leased`、`finished`、`cancelled`

现状问题：

1. Agent 主动上报 `status=failed` 时，服务端会直接将任务结束为 `finished`，不会自动重试。
2. `ReclaimStaleLeases` 对租约过期或 Agent 离线的任务会无条件打回 `pending` 并增加 `attempt`，缺少统一上限。
3. 失败原因没有形成统一的“自动重试中 / 已耗尽终态失败”语义，排障时难以区分“可再试”和“已放弃”。

本设计只覆盖 **通用 Agent 调度任务 `caichip_dispatch_task`**，不联动 BOM 搜索任务等业务表状态。

## 2. 目标与非目标

### 2.1 目标

- 为通用 Agent 调度任务增加统一的自动重试上限。
- Agent 主动上报 `failed` 时，支持按上限自动重试。
- 租约过期 / Agent 离线回收时，与主动失败共用同一套重试预算。
- 超过上限后进入明确的终态失败 `failed_terminal`，不再自动重试。
- 持久化失败原因，便于排障与审计。
- 支持“全局默认值 + 单任务可覆盖”的重试配置。

### 2.2 非目标

- 不改变 BOM 搜索任务、BOM merge、HS resolve 等其他独立重试链路。
- 不为本期引入新的调度 worker 或独立失败补偿表。
- 不把失败任务自动转成 `cancelled`。
- 不引入动态指数算法；首版采用配置化的离散退避序列。

## 3. 关键决策

### 3.1 终态失败状态

`t_caichip_dispatch_task.state` 新增：

- `failed_terminal`

语义：

- 任务已达到自动重试上限或满足不可再试条件。
- 该任务不会再次被认领。
- 与 `finished` 区分开，避免“成功完成”和“失败放弃”混淆。

### 3.2 自动重试范围

以下两类失败共享同一套自动重试预算：

1. Agent 主动上报 `TaskResult.status = "failed"`
2. 服务端回收租约：
   - lease 超时
   - Agent 离线导致的 lease 回收

### 3.3 重试次数定义

- `attempt` 定义为“当前执行次数”，首次入队为 `1`。
- `retry_max` 定义为“首次执行失败后，允许的额外自动重试次数”。
- 因此总执行上限为：

`max_total_attempts = 1 + retry_max`

默认配置：

- `retry_max = 3`
- 默认退避序列：`[60, 300, 900]` 秒

即默认最多执行 4 次：

1. 首次执行
2. 第 1 次自动重试，延迟 60 秒
3. 第 2 次自动重试，延迟 300 秒
4. 第 3 次自动重试，延迟 900 秒

若仍失败，则进入 `failed_terminal`。

### 3.4 退避策略

自动重试时任务写回：

- `state = pending`
- `next_claim_at = now + backoff`

退避序列使用离散配置数组，而非运行时动态计算。

当任务级覆盖或全局配置中的退避数组长度小于 `retry_max` 时：

- 使用数组最后一个值补足后续退避

示例：

- `retry_max = 3`
- `retry_backoff_sec = [60, 300]`

则实际退避为：

- 第 1 次重试：60 秒
- 第 2 次重试：300 秒
- 第 3 次重试：300 秒

## 4. 数据模型设计

### 4.1 调度表扩展

表 `t_caichip_dispatch_task` 新增列：

- `retry_max INT NOT NULL DEFAULT 3`
- `retry_backoff_json JSON NULL`

保留现有列并继续使用：

- `attempt`
- `state`
- `next_claim_at`
- `last_error`
- `result_status`
- `finished_at`

### 4.2 状态集合

`state` 取值扩展为：

- `pending`
- `leased`
- `finished`
- `cancelled`
- `failed_terminal`

### 4.3 任务模型扩展

`biz.QueuedTask` 新增可选字段：

- `RetryMax *int`
- `RetryBackoffSec []int`

语义：

- 未设置时使用全局默认配置。
- 设置时覆盖全局默认，并在入队时持久化到 `t_caichip_dispatch_task`。

## 5. 配置设计

在 `agent` 配置段增加：

- `dispatch_retry_max`
- `dispatch_retry_backoff_sec`

默认值：

- `dispatch_retry_max = 3`
- `dispatch_retry_backoff_sec = [60, 300, 900]`

配置校验规则：

- `dispatch_retry_max < 0` 时回退到默认值 `3`
- `dispatch_retry_backoff_sec` 为空时回退到默认值 `[60,300,900]`
- 退避数组中非正数元素按无效处理；若全数组无有效值，则回退默认值

## 6. 状态流转

### 6.1 成功上报

当 Agent 上报：

- `TaskResult.status = "success"`

则保持现有语义：

- 若租约匹配，任务转 `finished`
- 写入 `finished_at`
- 写入 `result_status`

### 6.2 主动失败上报

当 Agent 上报：

- `TaskResult.status = "failed"`

服务端不再直接转 `finished`，而是：

1. 判断当前失败后是否还有剩余重试额度
2. 若还有额度：
   - `state = pending`
   - 清空 `lease_id`
   - 清空 `leased_to_agent_id`
   - 清空 `leased_at`
   - 清空 `lease_deadline_at`
   - `attempt = attempt + 1`
   - `next_claim_at = now + backoff`
   - 更新 `last_error`
3. 若已耗尽：
   - `state = failed_terminal`
   - `finished_at = now`
   - `result_status = "failed_terminal"`
   - `last_error = failure reason`
   - 清空租约字段

### 6.3 租约回收

`ReclaimStaleLeases` 不再无条件把任务打回 `pending`。

回收时对每个命中的 `leased` 任务：

1. 判断失败原因：
   - `LEASE_EXPIRED`
   - `AGENT_OFFLINE_RECLAIMED`
2. 与主动失败上报共用同一套额度判断
3. 有额度时回 `pending` 并写 `next_claim_at`
4. 无额度时转 `failed_terminal`

### 6.4 不参与认领的状态

任务认领查询仍只处理：

- `pending`

以下状态不得再次被自动认领：

- `leased`
- `finished`
- `cancelled`
- `failed_terminal`

## 7. 失败原因记录

### 7.1 Agent 主动失败

`TaskResultRequest` 新增可选字段：

- `error_message`

失败原因写入优先级：

1. `error_message` 非空时，写入 `last_error`
2. 否则尝试使用 `stdout` 摘要
3. 若两者均为空，则写入固定兜底文案 `task failed`

### 7.2 租约回收失败

服务端生成固定失败原因：

- lease 超时：`LEASE_EXPIRED`
- Agent 离线：`AGENT_OFFLINE_RECLAIMED`

该原因写入 `last_error`。

## 8. 仓储与服务改动边界

### 8.1 仓储层

重点改动：

- `internal/data/dispatch_task_repo.go`

新增或调整职责：

- 入队时持久化 `retry_max` 与 `retry_backoff_json`
- 成功完成与失败重试分离
- 新增统一的“失败后重试或转终态失败”分支
- `ReclaimStaleLeases` 改成按单任务判断额度，而非批量无条件 `attempt + 1`

### 8.2 业务层

重点改动：

- `internal/biz/db_task_scheduler.go`
- `internal/biz/agent_hub.go`

职责：

- 扩展 `QueuedTask`
- 调度器仍保持薄封装，不在 biz 层复制仓储状态机逻辑

### 8.3 服务层

重点改动：

- `internal/service/agent.go`
- `api/agent/v1/agent.proto`

职责：

- `TaskResult` 接收 `error_message`
- 透传到调度仓储

## 9. 兼容性

### 9.1 旧任务行兼容

历史任务行即使没有显式写入新列值，也按以下策略兼容：

- `retry_max` 缺失或无效时，按全局默认值处理
- `retry_backoff_json` 为空时，按全局默认退避处理

### 9.2 旧 Agent 兼容

旧 Agent 未上报 `error_message` 时：

- 服务端使用 `stdout` 摘要或固定文案兜底

因此协议变更对旧 Agent 为向后兼容。

## 10. 测试要求

### 10.1 仓储测试

新增或补强以下用例：

- 首次失败且未超上限：写回 `pending`，设置正确的 `next_claim_at`
- 第 N 次失败且刚好耗尽：转 `failed_terminal`
- `ReclaimStaleLeases` 在有额度时回 `pending`
- `ReclaimStaleLeases` 在无额度时转 `failed_terminal`
- 任务级覆盖优先于全局默认
- 退避数组长度不足时复用最后一个值

### 10.2 服务测试

新增或补强以下用例：

- `TaskResult(status=failed)` 不再直接 `finished`
- `error_message` 正确透传为 `last_error`
- 旧 Agent 无 `error_message` 时仍可写入兜底失败原因

### 10.3 回归测试

需确认以下行为不回归：

- `success` 正常完成路径
- `cancelled` 不被再次认领
- `failed_terminal` 不被再次认领
- 长轮询拉任务仍只返回可认领的 `pending` 任务

## 11. 风险与取舍

- `ReclaimStaleLeases` 从批量更新改为按任务判断，会增加实现复杂度，但这是统一预算与失败原因记录的必要代价。
- 将任务级配置显式落列而不是塞入 `params_json`，会增加 schema 变更，但可换来更清晰的类型语义与排障可读性。
- `failed_terminal` 独立于 `finished`，会增加一个状态分支，但可显著降低后续误判和统计污染。

## 12. 实施边界总结

本期只做：

- 通用 Agent 调度任务自动重试上限
- 主动失败自动重试
- lease 回收统一预算
- 超限后 `failed_terminal`
- 失败原因持久化
- 全局默认值 + 任务级覆盖

本期不做：

- BOM 搜索任务业务态联动
- 失败任务人工恢复台
- 统一改造所有现有重试链路
