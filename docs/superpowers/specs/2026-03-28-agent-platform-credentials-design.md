# Agent × 平台登录凭据管理 — 需求与设计草案

日期：2026-03-28  
状态：**产品要点已定**（§7 全部确认，可进入实现计划）

## 1. 业务目标

部分爬虫站点需登录。需要：

1. **配置**：在「某个 Agent + 某个 `script_id`」上维护用户名、密码（及可选扩展字段）；与调度任务上的脚本标识一致。
2. **下发**：当该 Agent 被分配执行某 `script_id` 的任务时，若存在对应凭据，则把匹配项随任务交给 Agent，供脚本登录使用。

## 2. 与现有系统的关系（仓库现状）

- 调度任务已有 **`script_id`**（及 BOM 侧 `platform_id` 等业务键）；凭据绑定以 **`script_id` 为准**，与 `TaskObject` 上的脚本一致。Agent 侧 `TaskObject` 含 `params`（`google.protobuf.Struct`）。
- 凭据**不必**新增 Agent 协议字段即可落地：**服务端在租约/组装 `TaskObject` 时**，将解析后的字段写入 `params`**（或约定子键如 `platform_auth`），Agent 脚本按约定读取。
- 风险：`params` 会出现在长轮询响应与日志中，必须 **禁止明文打日志**、传输依赖 **HTTPS**。

## 3. 方案对比（2～3 种）

### 方案 A — 服务端注入 `TaskObject.params`（推荐首版）

- **做法**：库表存 `(agent_id, script_id)` → 用户名/密文密码；`PullAndLease`（或等价组装 Task 处）按 **`agent_id` + 任务 `script_id`** 查表，解密后写入 `params`。
- **优点**：协议不变；Agent 改动小（脚本读 params）；实现快。
- **缺点**：凭据随每次任务响应传输；服务端与日志需严格脱敏。

### 方案 B — 任务级一次性取密接口

- **做法**：Task 只带 `credential_ref` / 短期 token；Agent 用 `task_id + lease` 再调一次 HTTP 拉取明文，内存使用、不落盘。
- **优点**：长轮询包里不重复带密码；可审计单次拉取。
- **缺点**：多一次 RTT；需新 API 与鉴权；实现复杂度高。

### 方案 C — 宿主机环境变量 / 本地密文文件（运维下发）

- **做法**：不由调度注入，由运维在机器上配置；任务只带业务参数，凭据不走中心库。
- **优点**：中心库不存密；适合强管控环境。
- **缺点**：与「按 Agent 在控制台配置」产品路径不一致；多机扩展难。

**建议**：先做 **方案 A**，密钥 **落库加密**（应用层 AES-GCM 或 KMS，至少单主密钥 + 随机 nonce）；后续若合规要求再高，可演进 **B** 或混合（敏感平台走 B）。

## 4. 推荐数据模型（概念）

- 表（示例名）：`t_caichip_agent_script_auth`（或沿用内部统一命名规范）  
  - `agent_id`（FK 逻辑，与 `t_caichip_agent.agent_id` 一致）  
  - **`script_id`**（与调度任务 `caichip_dispatch_task.script_id`、脚本包 `script_id` **字符串完全一致**）  
  - `username`（可明文或轻量编码）  
  - `password_cipher`（密文 + 必要元数据）  
  - `updated_at` / `created_at`  
  - 唯一约束：`UNIQUE(agent_id, script_id)`

- **匹配规则**：租约任务上已有 `script_id` → 当 `(leased_to_agent_id, script_id)` 命中一行时注入 `platform_auth`；**未配置时照常下发任务，不写入 `platform_auth`**（由脚本决定继续或报错，见 §7.1）。

- **说明**：BOM 等业务里的 `platform_id`（如 `ickey`）与 `script_id`（如爬虫包名）的对应关系由**现有脚本发布/任务入队**保证；凭据表不存 `platform_id`，避免双键歧义。

## 5. 下发与契约

- 在组装 `TaskObject` 时增加字段（示例，实际键名可定规范）：

```json
{
  "platform_auth": {
    "username": "...",
    "password": "..."
  }
}
```

- 脚本约定：若存在 `params.platform_auth` 则用于本 `script_id` 对应站点的登录；**若不存在**，脚本自行选择无登录爬取、跳过或返回明确错误（服务端不因此拒绝租约/不下发任务）。JSON 键名仍可叫 `platform_auth`（表示站点登录），与「按 script_id 查凭据」不矛盾。
- **日志**：任何层禁止 `fmt`/`log` 整个 `params`；调试可用 `platform_auth_present=true` 之类。

## 6. 安全与运维

- 传输：仅 HTTPS；内网也需 TLS 或等价保护。
- 存储：密码不可逆明文落库；主密钥来自环境变量或密钥管理服务。
- 轮换：更新表行即可；进行中的租约仍用旧密（可接受）或支持版本号（YAGNI 可后做）。
- 权限：管理 API 与 `script_admin` / `agent_admin` 同级或更严；列表接口不返回密码明文。

## 7. 产品结论（已定）

1. **未配置凭据时**：**照常下发任务**，**不**带 `platform_auth`；由脚本自行决定是否继续、无登录尝试或报错。服务端不因缺凭据而拦截调度。  
2. **绑定主键**：凭据按 **`(agent_id, script_id)`** 绑定，**不**按 `platform_id` 存表；多站点由不同 `script_id`（不同脚本包）区分。若未来单脚本多站点登录，需拆脚本或扩展设计。  
3. **多账号池**（同一 Agent 同一 `script_id` 多个账号轮询）：**首版不做**；后续若有再单独立项。

## 8. 实现阶段建议

1. Schema + 加密工具 + Repo（biz 接口在 `internal/biz`，实现 `internal/data`）。  
2. 管理 API：CRUD（密码仅写、不回显，或仅 `***`）。  
3. 调度组装 `TaskObject` 时注入 `params`。  
4. 文档：脚本读取约定 + 运维配置说明。

---

**下一步**：可另起 `writing-plans`（或等价）产出实现清单、迁移 SQL、API 草案与测试点。
