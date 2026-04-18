# 代理模块（平台开关 + 快代理 getdps + 任务携带代理）— 设计规格

日期：2026-03-30  
状态：草案（已确认快代理获取失败时的退避策略）

## 1. 目标

1. **平台维度**：配置某采集平台是否 **必须使用代理**（`require_proxy`）。
2. **任务维度**：当需要代理时，调度下发的任务载荷中携带 **代理地址**（`host` + `port`，及按需的 **用户名/密码**），由 Agent 注入环境变量或 argv。
3. **供应商**：使用快代理 **获取私密代理 IP** API（getdps），文档：<https://www.kuaidaili.com/doc/product/api/getdps/>。

## 2. 配置与数据落点（建议）

- **平台是否需要代理**：`t_bom_platform_script.run_params`（JSON）增加布尔字段 **`require_proxy`**（默认 `false`）。与现有 `script_id` / 启用状态一并维护。
- **快代理凭据**：仅服务端配置（如 `conf` 或环境变量），**不**写入平台表。字段示例：`secret_id`、`secret_key`、`sign_type`（token / hmacsha1）、默认 `num`、可选默认地区等。
- **任务载荷**：`t_caichip_dispatch_task.params_json` 中结构化字段，例如：
  - `proxy_host`、`proxy_port`
  - 可选 `proxy_user`、`proxy_password`（当 getdps 使用 `f_auth=1` 等返回鉴权信息时）
  - 或统一 `proxy_url`（`http://user:pass@host:port`），由 Agent 与脚本约定解析方式。

## 3. 快代理客户端（服务端）

- 封装对 `https://dps.kdlapi.com/api/getdps` 的调用：`secret_id`、`signature`、`num`、`format=json`；按需 `area`、`f_auth=1`、`f_et=1` 等。
- 解析 `code == 0` 的 `data.proxy_list`；非 0 映射文档错误码（余额、白名单、频率限制等）。
- 遵守接口 **调用频率**（文档：最快约 10 次/秒）；中心侧可做进程内节流或合并 `num>1` 再分配。

## 4. 获取代理失败时的行为（**已确认**）

采用：**`pending` + 延迟重试（指数退避），仍无可用代理则 `fail`**。

### 4.1 语义

- 任务在 **尚未成功取得可用代理** 前，**不得**进入可被 Agent 正常认领执行的队列语义（避免无代理裸跑违反平台策略）。
- 每次 getdps 失败（网络错误、业务错误码、空列表等）后：
  - 记录尝试次数与下次重试时间；
  - 在到达下次重试时间 **之前**，该任务对认领侧应 **不可见** 或 **不可认领**（见下节实现选项）。
- 超过 **最大重试次数** 或 **总等待时间上限** 后：将任务置为 **终态失败**（与现有 `finished` + `result_status` / `last_error` 约定一致），错误信息中写明最后一次 API 返回或网络错误摘要。

### 4.2 指数退避（建议默认，可在配置中覆盖）

- 第 `k` 次失败后（`k` 从 0 起），等待  
  `delay_sec = min(cap_sec, base_sec * 2^k) + jitter`  
  **`jitter`**：`[0, base_sec)` 内均匀随机，避免惊群。
- **更保守的默认**（进一步压低 getdps 频率、拉长单次等待上限；可在配置中改回更激进）：
  - **`base_sec = 30`**（首败后至少约 30s 再试）
  - **`cap_sec = 1800`**（单次间隔上限 **30 分钟**）
  - **`max_attempts = 12`**（与较长间隔配套，仍避免无限重试；与「总截止时间」二选一时先触顶者生效）
- 可选 **`wall_clock_deadline_sec`**：从首次失败起算的总等待上限，建议默认 **172800** 秒（**48h**），防止任务无限挂起；与 `max_attempts` 并存时先触顶者生效。

**上述默认下间隔示例（无 jitter）**：30 → 60 → 120 → 240 → 480 → 960 → **1800** → 1800 → …（自 **k≥6** 起贴 `cap_sec`）。

### 4.3 与现有调度表的衔接（实现选型，择一或在 spec 评审中定稿）

**方案 A（推荐）**：新增列 **`next_claim_at`**（`DATETIME(3) NULL`），默认 `NULL` 表示立即可认领。  
- 需代理且尚未拿到 IP：插入任务时 `state='pending'`，`next_claim_at = now() + delay`；认领 SQL 增加条件 `AND (next_claim_at IS NULL OR next_claim_at <= NOW(3))`。  
- 每次退避：更新 `next_claim_at` 与 `params_json` 内 `_proxy_retry` 元数据（attempt、last_error）。

**方案 B**：不增列，使用 **`params_json._proxy_retry.next_try_utc`**，认领查询用生成列或应用层过滤（MySQL 8 JSON 索引可选）。复杂度高于 A。

**方案 C**：独立子状态 `pending_proxy`（扩展 `state` 枚举），由定时任务扫描转入 `pending`。需全链路识别新状态。

设计阶段 **推荐方案 A**，与现有 `pending/leased/finished` 兼容最好。  
**BOM 合并 + `require_proxy` 失败** 不采用「占位 pending dispatch + `next_claim_at`」重试 MergeKey，见 **§7 策略 B**。

## 5. Agent 与脚本

- Agent 认领到带 `proxy_*` 的任务后，在子进程环境中设置 `HTTP_PROXY`/`HTTPS_PROXY` 或传入 `--proxy`，与现有爬虫约定一致。
- 不在 Agent 内嵌快代理密钥（除非后续明确改为 Agent 拉取方案）。

## 6. 安全与审计

- 密钥仅存服务端；日志中 **打码** `proxy_password`。
- 可选：在 `last_error` 或审计表中记录最后一次快代理 `code/msg`（不含密钥）。

## 7. BOM 与合并调度（**已决策：策略 B**）

**策略 B：不占 inflight，只重试 MergeKey**

- 当平台 `require_proxy` 且 **getdps 未成功**（或空列表）时：**不** `INSERT t_bom_merge_inflight`、**不** `Enqueue` 调度任务、**不** `attachPendingBOMTasks`。
- 合并键对应的 `BomSearchTask` 保持 **pending**，`caichip_task_id` **保持为空**，直至某次 `TryDispatchMergeKey` 在 **事务外** 取代理成功后再走现有「inflight + enqueue + attach」事务。
- **指数退避**不依赖「占位 dispatch + `next_claim_at`」表达 MergeKey 重试；建议新增轻量表 **`t_bom_merge_proxy_wait`**（或等价名）：主键为 `(mpn_norm, platform_id, biz_date)`，列含 `next_retry_at`、`attempt`、`last_error`（及可选 `first_failed_at`）。失败时 upsert 该行；后台 worker 扫描 `next_retry_at <= NOW()` 调用 `TryDispatchMergeKey`；成功入队后 **删除** 该行；`attempt` 超上限或超过 wall clock 后对相关搜索任务记 **失败** 并删行。
- **`t_caichip_dispatch_task.next_claim_at`** 仍可用于 **非 BOM 合并路径** 或其它需「延迟可认领」的 pending 任务；BOM 合并在策略 B 下 **正常成功路径** 不产生需补代理的 dispatch 占位行。

## 8. 后续（实现清单）

- 迁移：`next_claim_at`（dispatch，可选/与其它任务共用）、`t_bom_merge_proxy_wait`（策略 B 退避）。
- biz 编排、conf proto、快代理客户端、MergeProxyRetry worker、认领 SQL、回归用例。

---

**用户确认记录**

1. 快代理获取失败时采用 **pending + 指数退避重试，耗尽后 fail**（本节 4）；在 dispatch 层用 `next_claim_at` 表达「延迟可认领」仍适用于通用 pending 行。
2. **BOM 合并路径**采用 **策略 B**：**不占 inflight**，**只重试 MergeKey**；退避状态由 **§7** 所述等待表（或等价机制）承载。
