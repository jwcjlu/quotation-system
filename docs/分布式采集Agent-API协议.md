# 分布式采集 Agent · HTTP/JSON 协议（草案）

本文档约定 **caichip 服务端** 与 **Agent** 之间的 **REST + JSON** 接口（与 [分布式采集Agent需求与架构](./分布式采集Agent需求与架构.md) 中的 **REST 长轮询**、**双接口心跳**、**API Key** 一致）。路径前缀示例为 `/api/v1/agent`，**以实现与 OpenAPI 定稿为准**。

---

## 1. 通用约定

### 1.1 传输与内容类型

| 项 | 约定 |
|----|------|
| **协议** | HTTPS（生产）；内网可 HTTP，由部署策略决定。 |
| **内容类型** | 请求/响应体均为 `application/json; charset=utf-8`。 |
| **长轮询** | 心跳类接口（§2、§3）服务端可在 **无新数据时阻塞至 `long_poll_timeout_sec` 或略小于客户端超时**，再返回空业务数据；客户端应设置 **大于** 服务端挂起上限的 HTTP 超时（见 §6）。 |

### 1.2 鉴权（Header）

任选其一（服务端与 Agent 约定一种即可）：

```http
Authorization: Bearer <api_key>
```

或：

```http
X-API-Key: <api_key>
```

无效或缺失时：**401 Unauthorized**，响应体见 §1.4。

### 1.3 通用请求字段（可选）

所有 **Agent 发起** 的 JSON 请求体可包含：

| 字段 | 类型 | 说明 |
|------|------|------|
| `protocol_version` | `string` | 协议版本，如 `"1.0"`，便于向后兼容。 |
| `agent_id` | `string` | Agent 唯一标识，**约定为本机 MAC 规范化字符串**（见需求文档 §4.1）。 |

---

### 1.4 统一错误响应

HTTP 状态码与业务错误：

```json
{
  "error": {
    "code": "UNAUTHORIZED",
    "message": "invalid or missing api key"
  }
}
```

| HTTP | `code` 示例 | 说明 |
|------|-------------|------|
| 400 | `BAD_REQUEST` | JSON 非法、缺少必填字段。 |
| 401 | `UNAUTHORIZED` | API Key 无效/过期。 |
| 403 | `FORBIDDEN` | Key 有效但无权访问该资源。 |
| 404 | `NOT_FOUND` | 脚本版本不存在等。 |
| 429 | `RATE_LIMITED` | 限流。 |
| 409 | `TASK_REASSIGNED` / `LEASE_EXPIRED` | 结果上报 **租约无效**（如已重派），见 **§4.2**。 |
| 500 | `INTERNAL` | 服务端错误。 |

成功时 HTTP **200**，响应体为各接口定义的 JSON（**不使用** 2xx 包裹错误，错误统一用 4xx/5xx + 上表 body）。

---

## 2. 任务心跳（Task Heartbeat）

**用途**：高频拉取待执行任务、维持在线；**默认周期 10s**（由 Agent 定时发起，与长轮询配合）。

### 2.1 请求

`POST /api/v1/agent/task/heartbeat`

**Request body：**

```json
{
  "protocol_version": "1.0",
  "agent_id": "aabbccddeeff",
  "queue": "default",
  "tags": ["region=cn", "env=prod"],
  "installed_scripts": [
    {
      "script_id": "hqchip_crawler",
      "version": "1.2.0",
      "env_status": "ready"
    },
    {
      "script_id": "ickey_crawler",
      "version": "2.0.1",
      "env_status": "preparing"
    }
  ],
  "hostname": "pc-01",
  "reported_at": "2026-03-22T10:00:00.000Z",
  "runtime": {
    "python_version": "3.11.2",
    "agent_version": "1.0.0"
  },
  "long_poll_timeout_sec": 50
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `agent_id` | string | 是 | 见 §1.3。 |
| `queue` | string | 否 | Agent 所认领队列，默认 `default`。 |
| `tags` | string[] | 否 | 标签，与需求 §4.6 一致。 |
| **`installed_scripts`** | **array** | **建议必填** | **本 Agent 已安装（已同步）的脚本包列表**，表示「**能跑哪些脚本**」；**调度侧派发任务时优先据此判断**（仅 `env_status=ready` 的可执行对应 `script_id`/`version`）。与 **脚本安装心跳** §3 上报内容同源，任务心跳可 **带轻量快照** 便于服务端快速路由，无需再等 10min 脚本心跳。 |
| `installed_scripts[].script_id` | string | 是 | 脚本包 ID。 |
| `installed_scripts[].version` | string | 是 | 与 `version.txt` 一致；**允许** 可选 `v`/`V` 前缀，比对时规范化（需求文档 §6.5）。 |
| `installed_scripts[].env_status` | string | 是 | `pending` \| `preparing` \| `ready` \| `failed`。 |
| `hostname` | string | 否 | 便于运维展示。 |
| `reported_at` | string | 否 | RFC3339 UTC，Agent 本地生成。 |
| `runtime` | object | 否 | 运行时信息。 |
| `long_poll_timeout_sec` | int | 否 | 希望服务端 **最长挂起秒数**，建议小于客户端 HTTP 超时；服务端可裁剪到上限（如 55）。 |

**语义说明（与需求文档 §4.6「能力位」一致）**：**能力位即已就绪脚本的 `script_id`**——以 **`installed_scripts` 中 `env_status=ready` 的 `script_id` + `version`** 作为调度依据；**不再** 单独使用与 `script_id` 无关的 `capabilities` 数组。若需表达主机环境（如是否有浏览器），请使用 **`tags`**（如 `has_browser=true`）。

**执行中仍发心跳（与需求文档 §4.2、§6.1 一致）**：Agent **正在执行任务时仍须按周期** 发起任务心跳（与任务子进程 **异步**），避免因长时间占用导致在 **`T_offline_sec`** 内无成功心跳、被误判离线（默认 120s，与心跳间隔关系见需求 **§6.1**）。

### 2.2 响应（成功 200）

服务端在 **有待派发任务** 或 **达到挂起超时** 时返回。

```json
{
  "server_time": "2026-03-22T10:00:50.123Z",
  "long_poll_timeout_sec": 50,
  "tasks": [
    {
      "task_id": "550e8400-e29b-41d4-a716-446655440000",
      "script_id": "hqchip_crawler",
      "version": "1.2.0",
      "entry_file": null,
      "argv": [],
      "params": {
        "keyword": "LAN8720",
        "max_pages": 3
      },
      "timeout_sec": 600,
      "lease_id": "01JQ...",
      "idempotency_key": "optional-key",
      "trace_id": "6f2b7e8a-..."
    }
  ]
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `server_time` | string | 服务端时间 RFC3339。 |
| `tasks` | array | **待执行任务列表**；无任务时可为 `[]`。 |
| `tasks[].task_id` | string | 全局唯一任务 ID。 |
| `tasks[].script_id` | string | 脚本包 ID。 |
| `tasks[].version` | string | **即** 本机该 `script_id` **已就绪包**的版本：与包内 **`version.txt` 规范化后须一致**（需求文档 §2、§6.5）；**允许** `v` 前缀，比对时 **规范化** 后相等。 |
| `tasks[].entry_file` | string \| null | 相对包根路径；`null` 表示使用 `main.py`/`run.py` 约定。 |
| `tasks[].argv` | string[] | 传给 Python 的额外参数（可选）。 |
| `tasks[].params` | object | JSON 参数，由脚本约定读取方式（如环境变量 `CAICHIP_TASK_PARAMS`）。 |
| `tasks[].timeout_sec` | int | **可选**。任务执行超时（秒），由 **下发任务** 指定；**省略时 Agent 默认 `300`（5 分钟）**。Agent 在本地计时，**超时则终止该任务进程组**（含子进程）并上报 **`status=timeout`**（需求文档 §6.2）。 |
| `tasks[].lease_id` | string | **可选**（建议有）。本次 **派发租约 ID**；结果上报 **须回传**，供服务端识别 **重派** 与拒收 **迟到结果**（需求文档 §6.4）。 |
| `tasks[].idempotency_key` | string | 可选幂等键。 |
| `tasks[].trace_id` | string | 可选，全链路追踪。 |

**任务对象必填（与需求文档 §4.5 一致）**：`task_id`、`script_id`、`version`。**`entry_file` 为 `null` 或省略时**：入口为脚本包根目录 **`main.py` → `run.py`** 顺序中 **第一个存在的文件**。

**说明**：Agent 收到 `tasks` 后 **执行**；完成后调用 **§4 任务结果上报**（不依赖再次心跳捎带结果，除非另有约定）。

**执行超时**：`timeout_sec` 未出现或无效时，Agent 使用 **默认 300 秒**；计时与 **终止进程组** 均在 Agent 侧完成，超时结果见 **§4.1** `status=timeout`。

---

## 3. 脚本安装心跳（Script Sync Heartbeat）

**用途**：上报已同步脚本及 `env_status`；接收 **待下载/升级** 的脚本元数据；**默认周期 10min**。

**与 §2 的关系**：本节请求体中的 **`scripts`** 为 **权威完整列表**（含 `package_sha256`、`installed_at` 等）。**任务心跳（§2）** 中的 **`installed_scripts`** 为 **同源快照**（字段可更轻），用于 **10s 级** 调度；服务端以 **脚本安装心跳** 为准做校准，发现与任务心跳不一致时以 **§3** 为准或触发对账。

### 3.1 请求

`POST /api/v1/agent/script-sync/heartbeat`

**Request body：**

```json
{
  "protocol_version": "1.0",
  "agent_id": "aabbccddeeff",
  "queue": "default",
  "tags": ["region=cn", "has_browser=true"],
  "scripts": [
    {
      "script_id": "hqchip_crawler",
      "version": "1.2.0",
      "package_sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
      "env_status": "ready",
      "message": "",
      "updated_at": "2026-03-22T09:00:00.000Z",
      "installed_at": "2026-03-22T08:55:00.000Z"
    },
    {
      "script_id": "ickey_crawler",
      "version": "2.0.1",
      "package_sha256": null,
      "env_status": "failed",
      "message": "pip install failed: ...",
      "updated_at": "2026-03-22T09:10:00.000Z",
      "installed_at": null
    }
  ],
  "long_poll_timeout_sec": 50
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `scripts` | array | 是 | 本 Agent 已知的脚本包列表（含失败态）。 |
| `scripts[].script_id` | string | 是 | |
| `scripts[].version` | string | 是 | 与 `version.txt` 一致；**允许** 可选 `v`/`V` 前缀，与服务端、心跳比对时 **规范化**（见需求文档 §2、§6.5）。 |
| `scripts[].package_sha256` | string \| null | 否 | 本地 ZIP 或解压目录校验用；未知可为 `null`。 |
| `scripts[].env_status` | string | 是 | `pending` \| `preparing` \| `ready` \| `failed`。 |
| `scripts[].message` | string | 否 | 失败原因摘要。 |
| `scripts[].updated_at` | string | 否 | RFC3339。 |
| `scripts[].installed_at` | string | 否 | 安装完成时间。 |

### 3.2 响应（成功 200）

```json
{
  "server_time": "2026-03-22T10:10:00.000Z",
  "sync_actions": [
    {
      "action": "download",
      "script_id": "hqchip_crawler",
      "version": "1.3.0",
      "package_sha256": "abcdef...64hex",
      "download": {
        "method": "GET",
        "url": "https://caichip.example.com/api/v1/agent/scripts/hqchip_crawler/versions/1.3.0/package?token=...",
        "headers": {},
        "expires_at": "2026-03-22T10:40:00.000Z"
      }
    },
    {
      "action": "delete",
      "script_id": "legacy_script",
      "version": "0.9.0",
      "reason": "deprecated"
    }
  ]
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `sync_actions` | array | 待执行动作；空数组表示无变更。 |
| `sync_actions[].action` | string | `download`：下载并安装；`delete`：删除本地某版本（可选）。 |
| `download` | object | 当 `action=download` 时必填（见 §5）。 |

---

## 4. 任务结果上报（Task Result）

**用途**：任务执行结束后 **主动上报**（与任务心跳解耦，便于重试与审计）。

### 4.1 请求

`POST /api/v1/agent/task/result`

**Request body：**

```json
{
  "protocol_version": "1.0",
  "agent_id": "aabbccddeeff",
  "task_id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "success",
  "started_at": "2026-03-22T10:00:51.000Z",
  "finished_at": "2026-03-22T10:05:30.000Z",
  "exit_code": 0,
  "stdout_tail": "last 8kb of stdout...",
  "stderr_tail": "",
  "result": {
    "items_count": 42,
    "payload": {
      "parts": [{ "mpn": "LAN8720AI-CP-TR", "stock": 100 }]
    }
  },
  "error": null,
  "idempotency_key": "optional-key",
  "lease_id": "01JQ...",
  "attempt": 1,
  "trace_id": "6f2b7e8a-..."
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `task_id` | string | 是 | 与任务心跳下发一致。 |
| `status` | string | 是 | `success` \| `failed` \| `timeout` \| `skipped`（如 env 未 ready）。 |
| `started_at` / `finished_at` | string | 否 | RFC3339。 |
| `exit_code` | int | 否 | 子进程退出码；非进程型任务可省略。 |
| `stdout_tail` / `stderr_tail` | string | 否 | 日志尾部截断；**单路长度上限** 由服务端/OpenAPI 固定（如各 **32KB**，见需求文档 **§6.6**）。 |
| `result` | object | 否 | **结构化结果**，由业务定义；成功时填充。**须可被服务端直接使用**（见 §4.3）。**序列化后大小** 建议 ≤ **1MB**（可配置），超限由 Agent 截断并可选 `truncated`。 |
| `lease_id` | string | 否 | 与任务下发 **§2.2** `tasks[].lease_id` **一致**；用于 **重派** 后校验；缺失时行为以实现为准。 |
| `error` | object \| null | 否 | 失败时：`{"code":"...","message":"..."}`。 |
| `idempotency_key` | string | 否 | 与任务下发一致，用于幂等。 |
| `attempt` | int | 否 | **执行轮次**，从 `1` 递增；**同一 `task_id` 再次执行**（重跑）时递增，便于与 **重复上报** 区分。 |

### 4.2 幂等与 `task_id` 重复语义（已约定）

| 场景 | 语义 |
|------|------|
| **重复结果上报** | Agent 因 **网络超时/重试** 对 **同一次执行** 多次 `POST /task/result`（相同 `task_id`、`agent_id`，内容相同或等价）。服务端应 **幂等**：返回 **HTTP 200**，**不重复** 触发入库、计费、下游通知等业务副作用（或以业务定义的 **结果指纹** 去重）。 |
| **同一 `task_id` 再次执行（重跑）** | 服务端 **再次** 在任务心跳中下发 **相同 `task_id`**（业务允许重跑）。Agent **允许再次执行** 子进程，并 **再次上报**；请求中建议带 **`attempt`**（第 2 次为 `2`…），服务端按轮次 **分别存储** 或 **仅保留最后一次**——以实现/OpenAPI 为准，须在文档中固定一种。 |
| **冲突处理** | 若重复上报的 **终态不一致**（如先 `failed` 后 `success`），服务端策略须二选一并文档化：**以最后一次为准** 或 **以首次终态锁定**；推荐 **最后一次覆盖** 以便修正误报。 |
| **重派后迟到结果** | 任务已 **重派** 且 **新执行已终态**（或任务已关闭）后，旧 Agent 仍上报 **过期租约** 的结果 → 服务端应 **拒收**（如 **HTTP 409**，`error.code`：`TASK_REASSIGNED` / `LEASE_EXPIRED`），**不得** 覆盖新结果（需求文档 **§6.4**）。 |

### 4.3 业务数据如何进 `result`（`out.json` 等本地文件）

脚本常把结果写到本机文件（如 `out.json`），但 **`out.json` 的路径只在 Agent 机器上有效**，caichip **无法** 像访问磁盘一样去「读取」该路径。

| 做法 | 说明 |
|------|------|
| **推荐（中小数据）** | 任务结束后，由 **Agent**（或脚本约定）**读取** `out.json`，将其中 **JSON 解析后嵌入 `result`**（例如 `result.payload`、或直接把数组放在 `result.items`）。服务端只处理 HTTP 体里的 JSON，**不访问 Agent 磁盘**。 |
| **大文件 / 二进制** | 不在 `result` 里塞整文件；由 Agent **上传到对象存储** 或 **调用 caichip 预留的上传接口**，在 `result` 中只带 **`artifact_url`** / **`artifact_id`** 等引用；若暂无上传接口，可暂用 **分段 base64**（受服务端单请求大小限制，仅适合较小文件）。 |
| **仅调试用** | 可在 `result` 中附加 **`local_output_path`**（即原 `output_path`）便于排障，**业务逻辑不得依赖**服务端能读到该路径。 |

**小结**：要「取到 `out.json` 里的数据」，应在 **Agent 侧读文件 → 把内容放进本请求的 `result`**；协议层不传「请服务端自己去某路径取文件」的语义。

### 4.4 响应（成功 200）

```json
{
  "accepted": true,
  "server_time": "2026-03-22T10:05:31.000Z"
}
```

---

## 5. 脚本包下载（Script Package Download）

**用途**：根据脚本同步心跳返回的 `download` 信息拉取 **ZIP**；也可在 **仅拿到 script_id/version** 时用固定 URL 模式（需鉴权）。

### 5.1 推荐：短期签名 URL（在 `sync_actions[].download` 中返回）

| 字段 | 说明 |
|------|------|
| `method` | 固定 `GET`（或 `POST` 大文件时另议）。 |
| `url` | **一次性或短时有效** 的 HTTPS URL，`token` 或签名在 query/path 中。 |
| `headers` | 若需额外 Header（如 `Authorization`），在此给出；通常为空（token 已在 URL）。 |
| `expires_at` | 过期后返回 **403/404**，Agent 应 **重新打脚本同步心跳** 获取新 URL。 |

**Agent 行为**：

1. `GET url` 下载二进制 body，存为临时文件。  
2. 校验 **Content-Length** 与 **SHA-256**（与 `package_sha256` 一致）。  
3. 解压到 `.../script_id/version/`，再执行 venv / pip（见需求文档 §4.4）。

### 5.2 备选：受控 GET（固定路径 + API Key）

当不使用 query token 时：

```http
GET /api/v1/agent/scripts/{script_id}/versions/{version}/package
Authorization: Bearer <api_key>
```

**Response：** `200`，`Content-Type: application/zip`，body 为 ZIP；可选 Header：`X-Package-Sha256: <hex>` 供校验。

**404**：该版本不存在；**410**：已下线。

---

## 6. 长轮询与超时（实现建议）

| 角色 | 建议值 | 说明 |
|------|--------|------|
| 服务端 `long_poll_max_sec` | ≤ 55 | 小于常见反向代理 idle 超时。 |
| Agent `long_poll_timeout_sec`（请求体） | 50 | 略小于客户端 HTTP 超时。 |
| Agent HTTP Client 超时 | 60～120 | 大于服务端挂起上限，避免误断。 |
| 无任务 / 无同步项 | HTTP **200** + `tasks:[]` 或 `sync_actions:[]` | 避免用 204 导致部分客户端丢弃 body。 |
| Agent **离线判定** `T_offline_sec` | 见需求 **§6.1** | 默认 120s；与 **任务心跳间隔** 取 `max(120, 6×interval)` 等，避免间隔过大误判。 |

---

## 7. 与 caichip 工程集成

- 路由挂载在现有 Kratos `http.Server` 上，前缀 **`/api/v1/agent`**（见需求文档 §4.7）。  
- 配置项（`Bootstrap.agent`）建议包含：`long_poll_max_sec`、日志 tail 最大长度、下载 token 有效期、API Key 校验表或 HMAC 密钥等。
- **Go 契约代码**：以 **Protobuf** 为单一事实来源：`api/agent/v1/agent.proto`（`package api.agent.v1`），通过 `make api` 生成 `*.pb.go`、`*_http.pb.go`、`*_grpc.pb.go`（Kratos `protoc-gen-go-http`）。服务端实现 `AgentServiceHTTPServer`；Agent 侧使用生成的 `AgentServiceHTTPClient`。少量常量（如 `ProtocolVersion`）在 **`caichip/api/agent`**（包名 `agent`）中复用。新增字段时先改 `.proto` 与本文档，再执行 `make api`。

---

## 8. 修订

| 版本 | 日期 | 说明 |
|------|------|------|
| 0.1 | 2026-03-22 | 初稿：双心跳、任务结果、脚本下载 JSON 契约 |
| 0.2 | 2026-03-22 | 任务心跳增加 `installed_scripts`；与 §3 `scripts` 的关系 |
| 0.3 | 2026-03-22 | §4.3 说明 `out.json` 须由 Agent 读入后写入 `result`；示例改为 `payload` |
| 0.4 | 2026-03-22 | 能力位与 `script_id`（ready）对齐；移除独立 `capabilities` 数组，主机特性用 `tags` |
| 0.5 | 2026-03-22 | §2.2：明确任务对象必填字段与无 `entry_file` 时 main.py/run.py |
| 0.6 | 2026-03-22 | §4.2 幂等与 `task_id` 重复；结果请求可选 `attempt`；与需求文档 `version.txt` v 前缀对齐 |
| 0.7 | 2026-03-22 | §2.2：`timeout_sec` 可选，默认 300s；Agent 超时杀进程并上报 `timeout`（对齐需求 §8.5） |
| 0.8 | 2026-03-22 | 任务 `version`=本地包；超时进程组；执行中心跳；需求引用改为 §6；重派见需求 §6.4 |
| 0.9 | 2026-03-22 | §2.2/§4.1 `lease_id`；§4.2 重派 409；§6 `T_offline`；对齐需求 §6.1、§6.4、§6.6 |

---

*关联文档：`docs/分布式采集Agent需求与架构.md`*
