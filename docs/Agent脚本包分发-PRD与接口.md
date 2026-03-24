# Agent 脚本包分发 · 单页 PRD 与接口

**版本**：0.1（草案）  
**关联**：与 [分布式采集Agent-API协议](./分布式采集Agent-API协议.md)、`api/agent/v1/agent.proto` 中的 `ScriptSyncHeartbeat`、`SyncAction`、`DownloadSpec` 对齐；任务执行侧仍用 `TaskHeartbeat` + `TaskObject`。

---

## 1. 背景与目标

| 项 | 说明 |
|----|------|
| **问题** | 采集脚本随平台演进需要版本化下发；仅靠本地文件或手工拷贝无法规模化，且无法与「期望版本」对齐。 |
| **目标** | 运营/管理员上传 **按 script_id 隔离** 的脚本制品（zip/tar 等，**product 口径下原 `platform_id` 与 `script_id` 合一**），服务端 **落盘 + 入库**；Agent 通过 **脚本同步心跳** 获知需安装的版本与 **下载地址**；下载走 **与主 API 同一 HTTP 服务、主站 URL 子路径**，**不**单独开端口暴露静态目录。 |
| **成功标准** | 某平台发布新版本后，在线 Agent 在合理时间内完成同步；失败可观测（心跳 `message`、可选告警）。 |

### 1.1 版本化管控（`platform_id` 即 `script_id`）

本能力 **不再单独使用 `platform_id` 维度**：库表、落盘路径、静态 URL、同步比对均以 **`script_id`** 为唯一业务键（与口头/历史称呼「平台」对齐时，**约定 `platform_id` ≡ `script_id`**）。

| 维度 | 含义 |
|------|------|
| **隔离** | 同一 `script_id` 下各 `version` 一条记录；不同 `script_id` 完全独立。 |
| **版本化** | 每个 `script_id` 保留多条历史版本；其中 **至多一条** 为 **published**，与 Agent 上报的 `ScriptRow` 比对后决定是否下发 `download`。 |
| **管控闭环** | 管理员按 **`script_id` + `version`** 上传与发布；服务端对 **全部已发布包** 与心跳中的 `scripts[]` 做 diff（同一 Agent 可装多个 `script_id`）。`ScriptSyncHeartbeatRequest` **已无** `platform_id` 字段（proto **reserved**，兼容旧客户端忽略）。 |

---

## 2. 范围与非目标

| 范围内 | 范围外（非目标） |
|--------|------------------|
| 管理端上传、查询、启用某 `script_id` 的「当前」制品 | Agent 侧沙箱、代码审计自动化 |
| 主站挂载只读下载路径 + 路径安全 | 多 CDN 边缘定制（可先同源） |
| `ScriptSyncHeartbeat` 返回 `SyncAction`（含 `DownloadSpec.url` 指向主站路径） | 在本文档中定义具体爬虫业务任务类型 |
| 与现有 `script_id` / `version` / `package_sha256` 字段语义一致 | BOM 配单业务表 `bom_platform_script` 的详细扩展（仅约定 `script_id` 可对齐） |

---

## 3. 用户与主流程

**角色**：管理员（上传/发布）、Agent（下载/安装/上报）、服务端（校验、元数据、对比心跳）。

**流程（简）**：

1. 管理员上传制品 → 服务端校验 → 写入对象存储目录（见 §7）→ 写库 → 标记该 `script_id` 的「当前版本」（可选仅 `upload`，再 `publish`）。  
2. Agent 定时 `POST /api/v1/agent/script-sync/heartbeat`，上报已装 `ScriptRow`（含 `version`、`package_sha256`、`env_status`）。  
3. 服务端对比 **期望版本**（库中当前发布记录）与上报：若不一致或 hash 不匹配 → 在 `sync_actions` 中返回 `action=INSTALL|UPGRADE`（及 `DownloadSpec`，`url` 为 **主站路径**，见 §5.3）。  
4. Agent 拉取 → 校验 `package_sha256` → 解压到工作目录 → 下次心跳更新 `env_status`。

---

## 4. 制品与命名约定

| 项 | 约定 |
|----|------|
| **script_id** | 与任务、BOM 映射一致的业务标识（如 `findchips`、`hqchip`）。 |
| **version** | 语义化字符串，建议 `major.minor.patch` 或与构建号一致；服务端按字符串比较策略在实现层约定（文档层要求 **全量匹配** 或 **服务端显式比较规则**）。 |
| **package_sha256** | 小写十六进制，与制品文件一致。 |
| **文件格式** | 建议 zip；`Content-Type` 与扩展名一致。 |
| **多脚本** | 不同 `script_id` 各自独立；同一 `script_id` 同一时间仅一条 **published**（见 §6）。 |

---

## 5. 接口说明

### 5.1 通用

- **Agent 接口**：鉴权同 [§1.2](./分布式采集Agent-API协议.md)（`Authorization` 或 `X-API-Key`）。  
- **管理端接口**：建议 **独立鉴权**（Session/JWT/管理 API Key），仅管理员可用；路径前缀示例：`/api/v1/admin/agent-scripts`（以实现为准）。

### 5.2 管理端（JSON / multipart）

| 方法 | 路径（示例） | 说明 |
|------|----------------|------|
| `POST` | `/api/v1/admin/agent-scripts/packages` | **multipart**：`file`（必填）、`script_id`、`version`、`package_sha256`（可选）、`release_notes`（可选）。成功返回 `package_id`、`download_path`。 |
| `POST` | `/api/v1/admin/agent-scripts/packages/{id}/publish` | 将该包设为该 `script_id` 的 **当前发布**；同 `script_id` 的旧 published 行归档。 |
| `GET` | `/api/v1/admin/agent-scripts/current` | Query：`script_id` → 当前发布元数据。 |
| `GET` | `/api/v1/admin/agent-scripts/packages` | 可选分页列表（审计/回滚查看）。 |

错误码与 §1.4 一致；重复发布同 `version` 可实现为 **409** 或 **覆盖策略**（在实现 README 写明）。

### 5.3 主站公开下载路径（挂主站、同源）

**原则**：静态文件由 **主 HTTP Server** 注册路由提供（如 Go `http.FileSystem` + `StripPrefix`），与 `/api/v1/...` 共用监听端口与 TLS。

**建议 URL 形态**：

```http
GET /static/agent-scripts/{script_id}/{version}/{filename}
```

示例：`GET /static/agent-scripts/findchips/1.2.3/package.zip`

| 项 | 约定 |
|----|------|
| **鉴权** | **可选**：MVP 可依赖 API Key 仅保护上传与管理端，下载 URL **仅 obscure**；增强可为下载路径签发 **短期 token**（query 或 `X-Download-Token`），或由 Nginx `internal` + 内传。**文档要求**：生产环境至少 **防目录穿越**、**禁止列目录**，并对高频下载做限流。 |
| **缓存** | `Cache-Control` 对带版本号的路径可 `public, max-age=…`；`publish` 变更当前版本不改变旧 URL，新版本新路径。 |
| **与 DownloadSpec** | `ScriptSyncHeartbeatReply.sync_actions[].download.url` = **绝对 URL**（`https://主域名/static/agent-scripts/...`）或 **同主域相对路径**（Agent 实现需与基址拼接）；`method=GET`，`headers` 预留（如未来 Signed cookie）。 |

### 5.4 Agent · 脚本同步心跳（已有协议摘要）

- `POST /api/v1/agent/script-sync/heartbeat`  
- 请求体：`ScriptSyncHeartbeatRequest`（`scripts[]`：`script_id`、`version`、`package_sha256`、`env_status` 等；**无** `platform_id`）。  
- 响应：`sync_actions[]`：`action`、`script_id`、`version`、`package_sha256`、`download`（`DownloadSpec`）、`reason`。  

**与任务心跳分工**：任务拉取仍走 `TaskHeartbeat`；**仅**脚本安装/升级走 `ScriptSyncHeartbeat`，避免混用。

---

## 6. 数据模型（建议）

表名示例：`agent_script_package`（以实现为准）。

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | bigint / uuid | 主键 |
| `script_id` | string | 业务脚本标识（分发觉唯一键之一） |
| `version` | string | 版本号 |
| `sha256` | string | 制品摘要 |
| `storage_rel_path` | string | 相对 `script_store.root` 的路径 |
| `filename` | string | 磁盘文件名 |
| `status` | enum | `uploaded` / `published` / `archived` |
| `created_at` / `updated_at` | timestamp | |

**唯一约束**：`(script_id, version)` 唯一。  
**当前发布**：每个 `script_id` 至多一行 `status=published`。

---

## 7. 服务端配置（建议）

与 `configs/config.yaml` 扩展对齐：

```yaml
script_store:
  root: "/var/lib/caichip/agent-scripts"   # 本地落盘根目录
  url_prefix: "/static/agent-scripts"       # 挂主站的路径前缀（对外 URL 路径）
  # public_base_url 可选：生成绝对 DownloadSpec.url 时使用，缺省从请求 Host 推导
```

---

## 8. 风险与开放问题

| 风险/问题 | 说明 |
|-----------|------|
| 大文件与带宽 | 需 limits、超时与可选断点续传（后续迭代）。 |
| 版本比较 | 字符串 vs semver：需在实现中统一，避免误判升级。 |
| 多实例部署 | `root` 需共享存储（NFS/OSS FUSE）或改为对象存储 + 签名 URL（仍可通过主站域名反向代理，保持「主站路径」产品口径）。 |
| Agent 基址 | 若 `download.url` 为相对路径，Agent 必须配置 `server_base_url`。 |

---

## 9. 文档修订记录

| 日期 | 变更 |
|------|------|
| 2025-03-24 | 初稿：单页 PRD + 管理端与主站静态路径约定；与 `agent.proto` 脚本同步字段对齐。 |
| 2025-03-24 | 增补 §1.1「按平台版本化管控」定义及 Agent 侧 `platform_id` 解析约定。 |
| 2026-03-24 | §1.1：`platform_id` 曾纳入 `ScriptSyncHeartbeatRequest`；后改为 **reserved**，产品口径 **platform_id ≡ script_id**。 |
