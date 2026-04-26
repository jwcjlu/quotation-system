# HS 型号解析：手动描述与上传手册 — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: 使用 `superpowers:subagent-driven-development`（推荐）或 `superpowers:executing-plans` **按 Task 顺序**实现；步骤使用 `- [ ]` 勾选跟踪。  
> **建议：** 在独立 git worktree 中实施（仓库备忘：[using-git-worktrees.md](../using-git-worktrees.md)）。

**Goal:** 按 [docs/superpowers/specs/2026-04-19-hs-resolve-manual-datasheet-design.md](../specs/2026-04-19-hs-resolve-manual-datasheet-design.md)，在 **不替代** 现网 datasheet 主路径的前提下，为 `HsResolveService.ResolveByModel` 增加 **手动描述** 与 **PDF 上传（multipart）** 旁路；主路径 datasheet 阶段失败且用户至少提供其一则继续解析；主路径成功则 **忽略** 旁路输入。

**Architecture:** **staging 表** 存上传元数据（高熵 `upload_id`、本地暂存路径、过期时间、可选 `owner_subject`）；**Resolve** 消费 staging → 复制/落盘至 datasheet 资产目录 → `HsDatasheetAssetRepo.Save` 写入 `t_hs_datasheet_asset`（`DatasheetURL = user-upload://{upload_id}`，`DownloadStatus=ok`）。**biz** `HsModelResolver` 在 datasheet 子阶段失败后尝试组装手动资产并重试该子阶段；**data** `HsLLMFeatureExtractor` 扩展抽取 prompt（`USER_DESCRIPTION` 块、固定顺序）。**service** 负责 `makeRunID` 纳入旁路指纹、无候选且无手动时的 **400 早失败**、描述长度与清洗。`HsDatasheetDownloader` 对 `user-upload://` **短路**不发起 HTTP。

**Tech Stack:** Go、Kratos、`github.com/go-kratos/kratos/v2/errors`、GORM、MySQL 8+、Wire、`protoc`（`make api`）、现有 `pkg/pdftext`。

**Spec / 设计输入:** [2026-04-19-hs-resolve-manual-datasheet-design.md](../specs/2026-04-19-hs-resolve-manual-datasheet-design.md)

---

## §8.2 在本计划中的闭合结论（实现按此执行）

| 待决项 | 闭合结论 |
|--------|----------|
| Run 键与旁路输入 | **方案 A**：在 `HsResolveService.makeRunID`（或等价单一入口）中，在现有 `model\|manufacturer\|request_trace_id` 与 `force_refresh` 分支逻辑之上，追加 **旁路指纹**：`sha256(规范化后的 manual_component_description + "\x00" + trim(manual_upload_id))` 取 **前 12 个十六进制字符**，以 `\|manual-{指纹}` 形式附加到非 refresh 的 run id；`force_refresh` 分支同样附加，避免同 trace 换描述仍命中 `GetByRequestTraceID` 旧任务。 |
| 缺输入错误码 | 统一 **`HS_RESOLVE_BAD_REQUEST`**；`error_message`（或 Kratos `Reason`）首行含固定子串 **`DATASHEET_OR_MANUAL_REQUIRED`**，便于客户端与日志检索；**不**新增独立 gRPC error code 枚举（YAGNI）。 |
| `upload_id` 鉴权 | **首版**：`upload_id` 为 **≥ 128 bit** 随机数（建议 UUID 无连字符 + 额外随机字节十六进制）；仅 HTTPS 部署假设。若上下文已有 **统一身份 subject**（如 JWT claims 写入 `context`），则 staging 行写入 `owner_subject`，Resolve 消费时 **不等则 400**；无身份中间件时该列为空，不校验。 |
| 占位 URL 全链路 | 新增 `biz.IsUserUploadDatasheetURL(url string) bool`（前缀 `user-upload://`）；`HsDatasheetDownloader.Download` / `CanDownload` 遇此前缀 **立即失败或 false**，禁止 HTTP GET。 |
| datasheet 失败边界 | **首版与现网一致**：仅 datasheet **子阶段** `retryStage` 返回 error 时视为失败并进入旁路评估（见 `hs_model_resolver.go` 354–363 行附近）。**不**在首版把「`DownloadStatus==ok` 但 `LocalPath` 为空」单独升格为旁路触发（design §8.2 可选增强，列入「后续」）。 |
| 同 trace 多次上传 | 允许多个 staging 行；**仅 Resolve 请求里出现的 `manual_upload_id`** 被消费；消费后标记 `consumed_at` 或删除行；未消费的过期行由清理任务删除。 |
| 空厂牌 + `Save` | `t_hs_datasheet_asset.manufacturer` 为 `NOT NULL`（可为 `''`）。首版 **放宽** `HsDatasheetAssetRepo.Save`：允许 `manufacturer == ""`（与 Resolve 允许空厂牌一致）；`GetLatestByModelManufacturer` 保持现逻辑（空厂牌不查库）**不影响**手动路径（手动资产由 upload 消费路径加载）。 |

---

## 文件结构（创建 / 修改一览）

| 路径 | 职责 |
|------|------|
| **Create** `docs/schema/migrations/20260419_hs_manual_datasheet_upload.sql`（日期按实际提交调整） | `t_hs_manual_datasheet_upload`：`upload_id`（唯一）、`local_path`、`sha256`、`expires_at`、`owner_subject`（可空）、`consumed_at`（可空）、`created_at` |
| **Modify** `internal/data/migrate.go` / `models.go` | 注册新模型；`HsManualDatasheetUpload` GORM 模型 |
| **Create** `internal/data/hs_manual_datasheet_upload_repo.go` | `Create`、`GetByUploadID`、`MarkConsumed`、`DeleteExpired`（或仅 `ListExpired` 供 job）— **仅 GORM** |
| **Modify** `internal/biz/repo.go` | 新增 `HsManualDatasheetUploadRepo` 接口 |
| **Modify** `api/bom/v1/bom.proto` | `HsResolveByModelRequest` 增加 `manual_component_description`、`manual_upload_id`；可选增加 `UploadHsManualDatasheetReply` 等 message（gRPC 对称；HTTP 上传走自定义 handler 仍可复用 reply message） |
| **Run** `make api` | 重生成 `bom.pb.go` / `_http.pb.go` 等 |
| **Modify** `internal/biz/hs_model_task.go` | `HsModelResolveRequest` 增加手动字段；`normalized()` 中 trim / 长度截断 **不在此做**（service 负责校验） |
| **Modify** `internal/biz/hs_model_resolver.go` | datasheet 阶段失败后调用 **旁路装配**（新方法，建议 **`hs_model_resolver_manual_datasheet.go`** 拆文件以遵守 ≤300 行规则） |
| **Create** `internal/biz/hs_model_resolver_manual_datasheet.go` | `tryManualDatasheetBypass(ctx, n, task) (*HsDatasheetAssetRecord, error)`：查 upload、落 `Save`、返回 asset |
| **Modify** `internal/biz/hs_model_resolver.go` 中 `HsModelFeatureExtractor` | `Extract` 增加参数 `manualUserDescription string`（或 `opts *HsExtractOptions`），**所有实现与 mock 一并改** |
| **Modify** `internal/data/hs_llm_feature_extractor.go` | `buildExtractPrompt`：`MODEL` / `MANUFACTURER` / `DATASHEET_DATA` / `USER_DESCRIPTION` 固定顺序；描述走 `sanitizeTextBlock` |
| **Modify** `internal/data/hs_datasheet_downloader.go` | `user-upload://` 短路 |
| **Modify** `internal/data/hs_datasheet_asset_repo.go` | `Save` 允许空 `manufacturer`（见上表） |
| **Modify** `internal/service/hs_resolve_service.go` | `makeRunID` 指纹；`ResolveByModel` 组装新字段；**无候选且无描述且无 upload_id → 400**；描述长度上限；注入 `manualUploadRepo` / 配置 |
| **Create** `internal/service/hs_manual_datasheet_upload.go`（或并入 `hs_resolve_service.go` 若仍 <300 行） | `UploadManualDatasheet(ctx, fileHeader, ownerSubject) (*UploadReply, error)` 业务步骤编排 |
| **Modify** `internal/server/hs_resolve_http.go` | 注册 `POST /api/hs/resolve/manual-datasheet/upload`：**multipart** `file`；调用 service；返回 JSON 与 `upload_id`、`expires_at`、`content_sha256` |
| **Modify** `internal/conf`（Bootstrap 或现有 hs 配置块） | `manual_max_bytes`、`manual_ttl_seconds`、`manual_max_description_runes`（或字节） |
| **Modify** `cmd/server/wire.go` / `wire_gen.go` | `make wire` 注入新 repo 与目录配置 |
| **Modify** `internal/biz/hs_model_resolver_test.go` / `internal/service/hs_resolve_service_test.go` / `internal/data/hs_llm_feature_extractor_test.go`（若不存在则新建） | 覆盖 §6 与 §8.2 行为 |

---

### Task 1: DDL + GORM 模型 + `HsManualDatasheetUploadRepo`

**Files:**
- `docs/schema/migrations/YYYYMMDD_hs_manual_datasheet_upload.sql`
- `internal/data/models.go`
- `internal/data/migrate.go`
- `internal/data/hs_manual_datasheet_upload_repo.go`
- `internal/data/hs_manual_datasheet_upload_repo_test.go`（sqlite 或与现有 data 测试一致）
- `internal/biz/repo.go`

- [ ] **Step 1:** 编写 migration：`upload_id` `VARCHAR(64)` UNIQUE NOT NULL；索引 `expires_at`；`owner_subject` `VARCHAR(128)` NULL。
- [ ] **Step 2:** GORM 模型 + `TableName`；`AutoMigrate` 注册（若环境开启）。
- [ ] **Step 3:** 实现 `Create` / `GetByUploadID` / `MarkConsumed` / `DeleteExpiredBefore(now)`。
- [ ] **Step 4:** `go test ./internal/data/... -run ManualDatasheetUpload -count=1 -v`。
- [ ] **Step 5:** Commit  

```bash
git add docs/schema/migrations/ internal/data/ internal/biz/repo.go
git commit -m "feat(data): hs manual datasheet upload staging table and repo"
```

---

### Task 2: Proto — `HsResolveByModelRequest` 扩展

**Files:**
- `api/bom/v1/bom.proto`
- `make api`

- [ ] **Step 1:** 为 `HsResolveByModelRequest` 增加 `manual_component_description`、`manual_upload_id`（均为 `string`，文档注释写明语义与上限由服务端校验）。
- [ ] **Step 2:** 增加 `rpc UploadHsManualDatasheet(UploadHsManualDatasheetRequest) returns (UploadHsManualDatasheetReply)`：`Request` 含 `bytes file`（与 `UploadBOM` 风格一致）、可选 `filename`；`Reply` 含 `upload_id`、`expires_at_unix`、`content_sha256`。**实现**：`HsResolveService` 单一方法内校验 PDF 魔数、落 staging；**HTTP** multipart handler 将 `file` 读入 `[]byte` 后调用同一方法，避免 gRPC `Unimplemented` 堵编译。
- [ ] **Step 3:** `make api`，实现 `UploadHsManualDatasheet` 并注册 gRPC；HTTP 路由仍可在 `hs_resolve_http.go` 单独注册以返回 JSON 与统一错误格式。

---

### Task 3: `user-upload://` 与 `Save` 空厂牌

**Files:**
- `internal/biz/hs_datasheet_url.go`（新建，放 `IsUserUploadDatasheetURL`）
- `internal/data/hs_datasheet_downloader.go`
- `internal/data/hs_datasheet_asset_repo.go`
- `internal/data/hs_datasheet_downloader_test.go` 或 biz 侧轻量测试

- [ ] **Step 1:** `IsUserUploadDatasheetURL` 实现。
- [ ] **Step 2:** Downloader 对 `user-upload://` 返回明确 error（不发起 HTTP）。
- [ ] **Step 3:** `Save` 去掉「`manufacturer` 非空」硬校验，保留 `model` 非空；`DatasheetURL` 长度校验。
- [ ] **Step 4:** `go test ./internal/data/... -count=1` 相关包。
- [ ] **Step 5:** Commit。

---

### Task 4: Service — `makeRunID`、早失败、组装 `HsModelResolveRequest`

**Files:**
- `internal/service/hs_resolve_service.go`
- `internal/service/hs_resolve_service_test.go`
- `internal/conf/*.proto` 或 `configs/config.yaml` + 解析代码（按仓库现有模式）

- [ ] **Step 1:** 实现 **旁路指纹** 拼接到 run id（见 §8.2 表）；`force_refresh` 与现网 `|refresh-nano` 规则兼容（指纹放在 refresh 后缀 **之前** 或 **之后**须固定一种，写入单测断言）。
- [ ] **Step 2:** `buildDatasheetCandidates` 返回空切片 **且** `manual_*` 皆空 → `kerrors.BadRequest("HS_RESOLVE_BAD_REQUEST", "DATASHEET_OR_MANUAL_REQUIRED: ...")` **在启动 goroutine 之前**返回。
- [ ] **Step 3:** `manual_component_description` 超长 → `BadRequest`；trim + `sanitizeTextBlock` 或与 data 层共享的 **包级** 清洗函数（避免 service import data 具体类型：可将 `SanitizeManualComponentDescription` 放在 `internal/biz` 或 `pkg/strutil`）。
- [ ] **Step 4:** 单测：`makeRunID` 随描述变化；无候选无手动 400。
- [ ] **Step 5:** Commit。

---

### Task 5: 上传编排 + HTTP multipart 路由

**Files:**
- `internal/service/hs_manual_datasheet_upload.go`（或 `hs_resolve_service.go`）
- `internal/server/hs_resolve_http.go`
- `cmd/server/wire.go` / `wire_gen.go`

- [ ] **Step 1:** Service 方法：校验 **Content-Type / `%PDF-` 魔数**、配置 **大小上限**；写入 `{assetDir}/manual_staging/{upload_id}.pdf`（路径常量）；计算 SHA256；`upload_id` 使用 `crypto/rand` 高熵十六进制；`expires_at = now+TTL`；`Create` staging 行；`owner_subject` 从 context 可选读取（若项目尚无标准，先 **TODO** 接口 `func OwnerSubjectFromContext(ctx) string` 返回空）。
- [ ] **Step 2:** `RegisterHsResolveServiceHTTPServer` 增加 `POST /api/hs/resolve/manual-datasheet/upload`，`FormFile("file")`，错误返回 **400** + 明确文案。
- [ ] **Step 3:** 集成风格单测或 `httptest` 调用 handler（可选 Task 7 再做）。
- [ ] **Step 4:** `make wire`；`go build ./...`。
- [ ] **Step 5:** Commit。

---

### Task 6: Biz — datasheet 失败后旁路装配

**Files:**
- `internal/biz/hs_model_task.go`
- `internal/biz/hs_model_resolver.go`
- `internal/biz/hs_model_resolver_manual_datasheet.go`
- `internal/biz/repo.go`（若需 `HsDatasheetAssetRepo` 扩展 `GetByID` — 消费 upload 后已有 `Save` 返回 ID，可仅依赖 Save 返回值）
- `internal/biz/hs_model_resolver_test.go`

- [ ] **Step 1:** `HsModelResolveRequest` 增加 `ManualComponentDescription`、`ManualUploadID`；`normalized()` trim。
- [ ] **Step 2:** 在 `resolveDatasheetAsset` 失败路径 **之后**（`retryStage` 失败回调外层或 datasheet closure 内第二段）：若 `Manual*` 有值 → `tryManualDatasheetBypass`：  
  - 查 staging `GetByUploadID`；校验未过期、未消费；**owner** 校验（若有）；  
  - 复制文件到正式 asset 目录（命名含 `sha256` 前缀防碰撞）；  
  - 构造 `HsDatasheetAssetRecord`：`DatasheetURL=user-upload://{id}`，`LocalPath` 为正式路径，`DownloadStatus=ok`，`Model`/`Manufacturer` 来自 **当前 resolve 请求**；`Save`；`MarkConsumed`。  
  - 返回 asset，使 datasheet 子阶段 **视为成功** 并进入 Extract。
- [ ] **Step 3:** 若主路径已成功得到 asset，**不读** `Manual*`（与 spec §2.4 一致）。
- [ ] **Step 4:** 单测：mock `HsManualDatasheetUploadRepo` + `HsDatasheetAssetRepo` + staging fs（`t.TempDir()`）。
- [ ] **Step 5:** Commit。

---

### Task 7: Extract prompt — `USER_DESCRIPTION` 与接口签名

**Files:**
- `internal/biz/hs_model_resolver.go`（interface）
- `internal/data/hs_llm_feature_extractor.go`
- 所有 `HsModelFeatureExtractor` 实现与测试 mock

- [ ] **Step 1:** 扩展 `Extract` 签名，增加 `manualUserDescription string`（调用处传 `n.ManualComponentDescription`，仅当 datasheet 阶段实际使用了旁路 **或** 主路径失败且仅描述：见下）。
- [ ] **Step 2:** 主路径成功 → 传空描述（忽略旁路）。  
  主路径失败 → 若用户填了描述，**无论**是否上传 PDF，均传入清洗后描述（仅 PDF 时 DATASHEET 有值 + USER_DESCRIPTION 为空 — spec 允许仅 PDF，此时传 `""`）。
- [ ] **Step 3:** `buildExtractPrompt` 固定块顺序；单测快照或包含子串断言。
- [ ] **Step 4:** `go test ./internal/biz/... ./internal/data/... -count=1`。
- [ ] **Step 5:** Commit。

---

### Task 8: 清理任务（可选 / 可后续 PR）

**Files:**
- `internal/data/hs_manual_datasheet_upload_repo.go`：`DeleteExpiredBefore` 先查行、仅删路径含 `/manual_staging/` 且后缀 `.pdf` 的本地文件，再按 `id IN` 删库。
- `internal/data/hs_manual_datasheet_janitor.go`：每小时 tick，调用 `DeleteExpiredBefore(time.Now().UTC())`。
- `internal/data/provider.go`：`NewHsManualDatasheetJanitor`
- `cmd/server/app.go` / `cmd/server/wire_gen.go`：`BeforeStart` / `BeforeStop` 挂载 Janitor。

- [x] **Step 1:** 删除 `expires_at < now` 且已消费或未消费 staging 行；**谨慎**：仅删除 staging 路径下文件，正式 asset 由既有策略管理。
- [x] **Step 2:** 文档注释配置 TTL 默认 24h（见 `HsManualDatasheetJanitor` 与 `biz.HsResolveConfig` 默认 TTL）。

---

### Task 9: 全量验证与 spec 回链

- [ ] **Step 1:** 本地执行 `go test ./...` 与 `go build -o bin/server ./cmd/server/...`（Agent 环境未装 Go 时在本机 CI/开发机跑）。
- [ ] **Step 2:** 在 [2026-04-19-hs-resolve-manual-datasheet-design.md](../specs/2026-04-19-hs-resolve-manual-datasheet-design.md) §8.2 表末行增加「**已在 plan 闭合**」指向本文件（可选）。
- [ ] **Step 3:** Commit。

---

## 执行后手递（writing-plans 要求）

**Plan 已保存至 `docs/superpowers/plans/2026-04-19-hs-resolve-manual-datasheet-implementation.md`。可选执行方式：**

1. **Subagent-Driven（推荐）** — 每 Task 派生子代理并在 Task 间做简短复核；需配合 `superpowers:subagent-driven-development`。  
2. **Inline Execution** — 本会话按 Task 顺序实现，配合 `superpowers:executing-plans` 与检查点。

如需我直接在仓库里开始改代码，请说明选用 **1** 或 **2**。

---

## Plan 评审（可选）

可将本 plan 与 spec 路径一并交给 `plan-document-reviewer` 子流程做一轮独立评审（见 `writing-plans` SKILL）。
