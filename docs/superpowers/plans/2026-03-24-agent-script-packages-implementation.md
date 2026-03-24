# Agent 脚本包分发 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: 实现本计划时优先使用 @c:\Users\Admin\.cursor\skills\superpowers\skills\subagent-driven-development\SKILL.md（推荐）或 @c:\Users\Admin\.cursor\skills\superpowers\skills\executing-plans\SKILL.md，**按 Task 顺序**完成；步骤使用 `- [ ]` 勾选跟踪。
>
> **建议：** 在独立 git worktree 中实施（见 @c:\Users\Admin\.cursor\skills\superpowers\skills\using-git-worktrees\SKILL.md），避免与主线 BOM 开发互相干扰。

**Goal:** 按 [Agent脚本包分发-PRD与接口](../../Agent脚本包分发-PRD与接口.md) 实现：管理端上传/发布/查询、本地 `script_store` 落盘、主站同源静态下载、`ScriptSyncHeartbeat` 比对「当前发布」并返回 `sync_actions`（含主站 `DownloadSpec.url`）；Agent 侧消费 `download` 拉包、校验 sha256、解压与上报状态。

**Architecture:** 数据与文件以 `(platform_id, script_id, version)` 为维；`(platform_id, script_id)` 至多一条 **published**。业务规则放在 `internal/biz`（版本规范化、diff、拼 URL）；持久化在 `internal/data`；`internal/service/AgentService.ScriptSyncHeartbeat` 调用 biz + repo；管理端与静态路由在 `internal/server` 用 Kratos `Route` 注册（与现有 `RegisterAgentHTTPServer` 并列）。遵循 DRY/YAGNI：首版可只做 **zip**、**精确版本匹配**、下载 URL 为相对或绝对由配置 `public_base_url` 控制。

**Tech Stack:** Go 1.22+、Kratos v2、`database/sql`（MySQL/PG 与现有 [internal/data/db.go](../../../internal/data/db.go) 一致）、Wire、现有 `api/agent/v1/agent.proto`；Python/Go Agent 复用已有 HTTP 客户端。

**Spec:** [docs/Agent脚本包分发-PRD与接口.md](../../Agent脚本包分发-PRD与接口.md) · 协议：[docs/分布式采集Agent-API协议.md](../../分布式采集Agent-API协议.md) §3

---

## 文件结构（创建 / 修改一览）

| 路径 | 职责 |
|------|------|
| **Create** `docs/schema/agent_script_package_mysql.sql`（及可选 `_postgres.sql`） | 表 `agent_script_package` + 唯一索引 `(platform_id, script_id, version)`；`status` 枚举；发布语义（单 published / 行上 `status` + 更新旧 published 为 archived） |
| **Modify** `internal/conf/conf.go` | `Bootstrap.ScriptStore`：`root`、`url_prefix`、`public_base_url`、上传 `max_bytes`；可选 `AdminApiKeys []string` 或与现有 `Agent` 并行的 `ScriptAdmin` 小节 |
| **Modify** `configs/config.yaml` | 填入 `script_store` 示例 |
| **Create** `internal/data/agent_script_package_repo.go` | CRUD：Insert（upload）、SetPublished（事务内下线旧 published）、GetPublished(platform, script_id)、List(分页)、GetByID |
| **Modify** `internal/data/provider.go` | `NewAgentScriptPackageRepo` 加入 `ProviderSet`（DB 为 nil 时 repo 返回 no-op 或 error，在计划中选定一种并在 `AgentService` 里短路） |
| **Create** `internal/biz/script_package_sync.go` | `NormalizeVersion(s string) string`（与协议 §6.5 一致：去 `v`/`V` 前缀）；`NeedsSync(published, reported ScriptRow) bool`；构造 `[]*v1.SyncAction` |
| **Modify** `internal/service/agent.go` | `ScriptSyncHeartbeat`：`platform_id` 非空校验（脚本分发开启时）；查 published；对每条 `req.Scripts` 与 **期望集合**（库中该平台所有 published 的 script_id，或仅对上报的 script_id 查询）做 diff；填 `SyncActions` |
| **Modify** `internal/service` | 新建 `agent_script_admin.go` 或同类：**multipart 上传**、publish、current、list（管理鉴权） |
| **Modify** `internal/server/http.go` / **`agent_http.go`** 或 **`script_store_http.go`** | `RegisterScriptStoreStatic`：`url_prefix` + `root` 安全 `FileServer`（`path.Clean`、禁止 `..`、**禁止列目录**可用 `http.FileSystem` 包装）；`POST/GET` 管理路由（前缀 `/api/v1/admin/agent-scripts`） |
| **Modify** `cmd/server/wire.go`（若新增构造参数） | 跑 `cd cmd/server && wire` 生成 `wire_gen.go` |
| **Modify** `internal/agentapp/` + **`agent/`**（Python） | `applySyncActions`：GET `download.url`（Bearer 同上）、写临时文件、sha256、解压到 `data_dir/script_id/version/` |
| **Create** `internal/data/agent_script_package_repo_test.go` | 表驱动集成测试：`TEST_DATABASE_URL` 或跳过（对齐 [internal/data/db_test.go](../../../internal/data/db_test.go)） |
| **Modify** `docs/agent-server-实现说明.md` | 脚本分发开关、环境变量、管理 API 简述 |

---

### Task 1: 数据库 schema 与迁移

**Files:**
- Create: `docs/schema/agent_script_package_mysql.sql`
- 参考: `docs/schema/bom_mysql.sql`（风格）

- [ ] **Step 1:** 编写 DDL：`agent_script_package`（字段对齐 PRD §6：`id` BIGINT AI、 `platform_id`、`script_id`、`version`、`sha256`、`storage_rel_path`、`filename`、`status`、时间戳；唯一键 `(platform_id, script_id, version)`；索引 `(platform_id, script_id, status)` 便于查 published）。
- [ ] **Step 2:** 在本地 MySQL 执行该 SQL（或 migrate 工具），`DESCRIBE agent_script_package` 确认。
- [ ] **Step 3:** Commit  

```bash
git add docs/schema/agent_script_package_mysql.sql
git commit -m "docs(schema): add agent_script_package for script store"
```

---

### Task 2: 配置 `script_store` 与管理端鉴权键

**Files:**
- Modify: `internal/conf/conf.go`
- Modify: `configs/config.yaml`

- [ ] **Step 1:** 在 `Bootstrap` 增加 `ScriptStore *ScriptStore`、`ScriptAdmin *ScriptAdmin`（示例字段：`Root` `UrlPrefix` `PublicBaseURL` `MaxUploadMB`；`ApiKeys []string` 专用于 `/api/v1/admin/agent-scripts`，**勿**与 Agent `api_keys` 混用）。
- [ ] **Step 2:** `configs/config.yaml` 增加注释块示例（可与 `agent.enabled` 并列）。
- [ ] **Step 3:** `go test ./internal/conf/...`（若无测试则 `go build ./...`）。
- [ ] **Step 4:** Commit  

```bash
git add internal/conf/conf.go configs/config.yaml
git commit -m "feat(conf): script_store and script admin API keys"
```

---

### Task 3: Repository（TDD：先测后实现）

**Files:**
- Create: `internal/data/agent_script_package_repo_test.go`
- Create: `internal/data/agent_script_package_repo.go`
- Modify: `internal/data/provider.go`

- [ ] **Step 1: 写失败测试** — 测试 `Insert` + `SetPublished` 后 `GetPublished` 返回正确版本（需 `TEST_MYSQL_DSN` 或项目现有 `TEST_DATABASE_URL` 变量名，与 `db_test.go` 一致）。

```go
func TestAgentScriptPackageRepo_PublishRoundTrip(t *testing.T) {
    dsn := os.Getenv("TEST_DATABASE_URL")
    if dsn == "" {
        t.Skip("TEST_DATABASE_URL not set")
    }
    // open db, migrate table if needed, repo.Insert(..., status=uploaded)
    // repo.SetPublished(id) -> GetPublished("plat","findchips") -> version == "1.0.0"
}
```

- [ ] **Step 2:** `go test ./internal/data -run TestAgentScriptPackageRepo -v` → **预期 FAIL**（repo 未实现）。
- [ ] **Step 3:** 实现 `AgentScriptPackageRepo`，`SetPublished` 内 **事务**：将同 `(platform_id, script_id)` 的其它 `published` 置 `archived`，目标行置 `published`。
- [ ] **Step 4:** `go test ./internal/data -run TestAgentScriptPackageRepo -v` → **PASS**。
- [ ] **Step 5:** `wire` 相关 Provider 仅当 `DB != nil` 注入（否则 `NewAgentScriptPackageRepo` 返回 stub 且管理接口 503 —— YAGNI 可选：直接要求有 DB）。
- [ ] **Step 6:** Commit  

```bash
git add internal/data/agent_script_package_repo.go internal/data/agent_script_package_repo_test.go internal/data/provider.go
git commit -m "feat(data): agent_script_package repo and publish transaction"
```

---

### Task 4: Biz — 版本规范化与同步 diff

**Files:**
- Create: `internal/biz/script_package_sync.go`
- Test: `internal/biz/script_package_sync_test.go`

- [ ] **Step 1:** 测试 `NormalizeVersion("v1.2.3") == "1.2.3"`、`NeedsSync` 在 version 或 sha256 不等时为 true。
- [ ] **Step 2:** 实现函数；构造 `v1.SyncAction`：`action` 使用协议文档 §3.2 已出现的 **`download`**（与示例 JSON 一致）；`download.method=GET`，`url` 暂由下一步注入占位符或由 biz 接收 `baseURL + path`。
- [ ] **Step 3:** `go test ./internal/biz -run ScriptPackage -v` → PASS。
- [ ] **Step 4:** Commit  

```bash
git add internal/biz/script_package_sync.go internal/biz/script_package_sync_test.go
git commit -m "feat(biz): script package version normalize and sync diff"
```

---

### Task 5: 落盘路径与静态文件 HTTP（防穿越）

**Files:**
- Create: `internal/server/script_store_static.go`（或并入 `http.go`）
- Modify: `internal/server/http.go`

- [ ] **Step 1:** 实现 `secureFileServer(root, urlPrefix string) http.Handler`：请求路径必须前缀 `urlPrefix`，映射到 `root` + rel；任一组件含 `..` 返回 404。
- [ ] **Step 2:** 在 `NewHTTPServer` 中当 `c.ScriptStore != nil && Root != ""` 时，向 Kratos Server 注册路由（查 Kratos `srv.HandlePrefix` 或 `Route` + 标准 `Handler`；与现有 [internal/server/http.go](../../../internal/server/http.go) 兼容）。
- [ ] **Step 3:** 手动 `curl -I http://127.0.0.1:PORT/static/agent-scripts/plat/x/y/z/pkg.zip`（先放一个测试文件）验证 200；`curl` 带 `..` 验证 404。
- [ ] **Step 4:** Commit  

```bash
git add internal/server/script_store_static.go internal/server/http.go
git commit -m "feat(server): serve script_store under url_prefix safely"
```

---

### Task 6: 管理端 HTTP API（multipart + publish + query）

**Files:**
- Create: `internal/service/agent_script_admin.go`
- Modify: `internal/server/agent_http.go`（或新文件 `admin_http.go`）：`r.POST("/api/v1/admin/agent-scripts/packages", ...)` 等

- [ ] **Step 1:** 实现鉴权：`X-API-Key` 或 `Authorization` 命中 `ScriptAdmin.ApiKeys`。
- [ ] **Step 2:** `POST .../packages`：解析 `platform_id`、`script_id`、`version`、`file`；计算 sha256；写入 `{root}/{platform_id}/{script_id}/{version}/{filename}`；`Insert` 行 `uploaded`；返回 `id`、`download_path`（逻辑相对路径）。
- [ ] **Step 3:** `POST .../packages/{id}/publish`：调用 repo `SetPublished`。
- [ ] **Step 4:** `GET .../current?platform_id=&script_id=`、`GET .../packages` 分页。
- [ ] **Step 5:** `curl`/集成测试（可选 Postman）；Commit  

```bash
git add internal/service/agent_script_admin.go internal/server/*.go cmd/server/wire_gen.go
git commit -m "feat(admin): agent script package upload publish list"
```

---

### Task 7: `ScriptSyncHeartbeat` 接入 repo + `sync_actions`

**Files:**
- Modify: `internal/service/agent.go`（`NewAgentService` 注入 repo、`conf.Bootstrap` 中 `ScriptStore`）
- Modify: `cmd/server/wire.go` + `wire_gen.go`

- [ ] **Step 1:** 当 `req.platform_id == ""` 且配置启用脚本分发时返回 `400 BAD_REQUEST`（与协议 §3 一致）；未启用分发时可保持旧行为仅更新 Hub。
- [ ] **Step 2:** 对每个 **published** 的 `script_id`（或仅对请求中已上报的 script 合并全集，按产品：**至少**推送库里有但机器缺的 script），调用 Task 4 的 biz 生成 `SyncAction`；`download.url` = `PublicBaseURL` + `url_prefix` + 相对路径（无 `PublicBaseURL` 时用 `Host`，见 PRD §7）。
- [ ] **Step 3:** 本地起服务 + `curl` 心跳样例，响应含非空 `sync_actions`（先手动 publish 一包）。
- [ ] **Step 4:** Commit  

```bash
git add internal/service/agent.go cmd/server/wire_gen.go
git commit -m "feat(agent): ScriptSyncHeartbeat returns sync_actions from published packages"
```

---

### Task 8: Agent 执行 `sync_actions`（Python 优先，与 Go agent 一致）

**Files:**
- Modify: `agent/app.py`、`agent/` 下新建 `sync_actions.py` 或并入 `runner.py`
- Modify: `internal/agentapp/`（已有 `applySyncActions` 钩子则填满实现）

- [ ] **Step 1:** 解析 `sync_actions`：`action==` **`download`**（协议示例；若服务端发 `INSTALL` 需与 proto 文档对齐统一为小写 `download`）。
- [ ] **Step 2:** HTTP GET + API Key header；落临时文件；sha256；解压到 `AGENT_DATA_DIR/{script_id}/{version}/`；写 `version.txt`。
- [ ] **Step 3:** 失败写入 `env_status=failed` 与 `message`（下次心跳上报）。
- [ ] **Step 4:** 端到端：管理端上传 → publish → Agent 脚本心跳 → 目录出现文件。
- [ ] **Step 5:** Commit  

```bash
git add agent/ internal/agentapp/
git commit -m "feat(agent): apply script sync download actions"
```

---

### Task 9: 文档与条目核对

**Files:**
- Modify: `docs/agent-server-实现说明.md`
- [ ] 核对 [Agent脚本包分发-PRD与接口.md](../../Agent脚本包分发-PRD与接口.md) §5.2 路径与实现一致；`action` 枚举与 [分布式采集Agent-API协议.md](../../分布式采集Agent-API协议.md) §3.2 **一致**（若 PRD 写 INSTALL/UPGRADE，在 PRD 或协议中改正为实际 JSON 值）。
- [ ] Commit：`docs: script package rollout notes`

---

## Plan review（可选）

完成全文后，可将 **本计划路径** 与 **PRD 路径** 交给独立审阅角色（若使用 superpowers 的 plan-document-reviewer 提示词）；有 ❌ 则修订计划再开发。

---

## Execution handoff

**计划已保存至：** `docs/superpowers/plans/2026-03-24-agent-script-packages-implementation.md`。

**执行方式任选：**

1. **Subagent-Driven（推荐）** — 每 Task 派生子代理，Task 间 review；配合 @c:\Users\Admin\.cursor\skills\superpowers\skills\subagent-driven-development\SKILL.md  
2. **Inline Execution** — 本会话内按 Task 推进、checkpoint 人工确认；配合 @c:\Users\Admin\.cursor\skills\superpowers\skills\executing-plans\SKILL.md  

**需要我按哪一种开始执行？**
