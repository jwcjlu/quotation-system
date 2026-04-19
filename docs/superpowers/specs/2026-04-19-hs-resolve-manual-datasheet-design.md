# HS 型号解析：无 datasheet 时支持手动描述与上传手册

## 1. 目标与范围

### 1.1 背景

当 BOM/报价侧**没有可用 datasheet URL** 或下载失败时，`HsModelResolver` 在 datasheet 阶段会得到 `datasheet not available`，任务失败；HTTP 侧表现为 `error_code = HS_RESOLVE_FAILED` 且 `error_message` 含该字符串（见 `internal/biz/hs_model_resolver.go`、`internal/service/hs_resolve_service.go`）。

### 1.2 目标

在 **不替代** 现有「DB 候选 URL → 下载 → 落库资产 → LLM 抽取 → 预筛/推荐」主路径的前提下，增加 **用户旁路**：

1. **手动填写**电子元器件描述（纯文本）；  
2. **上传使用手册**（以 PDF 为主，与现有 `pdftext.ReadBodyHeadFromFile` 抽取路径一致）。

两者可 **单独或组合** 提供；当 **自动 datasheet 不可用** 时，至少其一须存在，否则返回明确 **4xx**（见 §4），避免无输入的「空跑」解析。

### 1.3 非目标

- 不在首版实现「扫描件 OCR」「DOCX 内嵌对象」等非 PDF 主格式（可后续扩展 MIME 白名单）。  
- 不承诺用户上传内容与官方 datasheet 等价的法律/归类效力；仅作为 **解析输入来源**。  
- 不在本文定义前端 UI 布局（仅定义 API 与后端语义）。

### 1.4 范围边界

- Proto / HTTP：`api/bom/v1/bom.proto` 中 `HsResolveService` 与 `HsResolveByModelRequest`；**新增**上传 RPC（见 §3）。  
- 领域：`internal/biz`（`HsModelResolveRequest`、resolver 阶段逻辑）、`internal/service`（组装请求、上传处理）、`internal/data`（抽取 prompt、资产落库）。  
- 与 `t_hs_model_features` 写入条件一致：需 **`HsDatasheetAssetRecord` 已持久化且 `ID != 0`**（见 `internal/biz/hs_model_resolver_features_persist.go`），故 **用户上传的 PDF 必须走与现网一致的资产 `Save` 路径**。  
- **仅文本旁路**且无 `ID != 0` 资产时：现网 `persistHsModelFeatures` 会 **跳过** 特征落库（不报错）；首版 **接受** 该行为，与 §8.1 一致。

---

## 2. 数据源优先级与合并规则

### 2.1 主路径（不变）

1. 从现有数据源组装 `DatasheetCands`（如 `HsResolveService.buildDatasheetCandidates`）。  
2. `resolveDatasheetAsset`：下载器 + `assetRepo` 可用时走 `ResolveAndPersistDatasheet`；否则从候选中选 URL。  
3. 资产 `DownloadStatus == "ok"` 且非空本地路径（若下载路径）→ 进入 **Extract** 阶段。

### 2.2 旁路触发条件

当且仅当 **主路径在 datasheet 阶段失败**（无候选、下载失败、`DownloadStatus != "ok"` 等，与现网 `datasheet_failed` 语义一致）时，才评估用户旁路：

- `manual_component_description`（trim 后非空），和/或  
- `manual_upload_id` 有效且对应文件已就绪、并成功写入 `HsDatasheetAssetRecord`。

若主路径失败且 **两者皆空** → **拒绝请求**（§4），错误信息须明确提示「需提供 datasheet 或手动描述/上传」。

### 2.3 仅文本、仅 PDF、两者兼有

| 场景 | 行为 |
|------|------|
| 仅描述 | 不读取本地 PDF；在 LLM 抽取 prompt 中注入 `USER_DESCRIPTION`（命名实现时固定），再走与现网相同的 `HsLLMExtractClient` → `HsPrefilterInput`。 |
| 仅上传 PDF | 与现网一致：`LocalPath` → `pdftext` 取头段 → `DATASHEET_DATA`。 |
| 描述 + PDF | **同一 prompt** 中同时包含 PDF 头段与 `USER_DESCRIPTION`；**固定块顺序**（建议：MODEL / MANUFACTURER / DATASHEET_DATA / USER_DESCRIPTION），便于回归与日志对照。 |

### 2.4 当主路径已成功时，用户旁路是否参与（已定）

- **默认**：若 datasheet 阶段已成功并得到可用资产，则 **忽略** `manual_component_description` 与 `manual_upload_id`（不合并进抽取 prompt），避免与已发布/已缓存数据源冲突。  
- **扩展（非首版必做）**：可增加可选 bool，例如 `manual_overlay_when_datasheet_ok`（命名待定）；为 `true` 时仍将用户描述（及可选上传）合并进 prompt。首版 **不暴露该字段** 时行为与「默认忽略」一致。

---

## 3. API 设计

### 3.1 `HsResolveByModelRequest` 扩展

在现有字段（`model`、`manufacturer`、`request_trace_id`、`force_refresh`）基础上增加：

| 字段（建议名） | 类型 | 说明 |
|----------------|------|------|
| `manual_component_description` | `string` | 用户填写的元器件说明；**最大长度**建议 8k～16k 字符（实现时选一定额），服务端做与现网类似的文本清洗（可与 `sanitizeTextBlock` 语义对齐）。 |
| `manual_upload_id` | `string` | 由 §3.2 上传接口返回的 **一次性/短期有效** 标识；解析前须能解析到已落盘的文件与 DB 资产行。 |

约束：

- 二者可同时出现；在 §2.2 触发时 **至少其一非空**。  
- `manual_upload_id` 与 `request_trace_id`、`force_refresh` 参与 **run 键 / 幂等** 的规则须在实现计划中写死（避免同 trace 换描述仍命中旧结果）；若现有 `makeRunID` 仅依赖 model/manufacturer/trace/force_refresh，则 **须将旁路输入纳入哈希或单独版本号**，或在文档中明确「改变旁路输入必须换 `request_trace_id`」的产品约定（二选一，实现计划拍板）。

### 3.2 新增：上传使用手册

- **RPC**：例如 `UploadHsManualDatasheet`（具体命名以 proto 为准）。  
- **HTTP**：`POST` + `multipart/form-data`，单文件字段名固定（如 `file`）。  
- **响应**：`upload_id`、`expires_at`（Unix 秒或 RFC3339）、可选 `content_sha256`。  
- **服务端**：校验 **Content-Type / 魔数**（至少 PDF）；大小上限（如 10～20MB，配置化）；落盘至与现有 HS datasheet 下载目录策略一致的受控目录；调用 `HsDatasheetAssetRepo.Save`（或等价封装）写入 **`HsDatasheetAssetRecord`**，`DownloadStatus = "ok"`，`DatasheetURL` 可使用占位如 `user-upload://{upload_id}` 便于审计。  
- **生命周期**：`upload_id` TTL（如 24h）；过期后 `ResolveByModel` 返回明确错误；后台或定时任务清理孤儿文件（实现计划细化）。

### 3.3 为何不把大 PDF Base64 放进 `ResolveByModel`

避免超大 JSON、网关超时与可观测性问题；与仓库内已有 **multipart 上传** 先例（如 admin 脚本包上传）一致。

---

## 4. 错误语义

| 场景 | 建议 HTTP | 说明 |
|------|-----------|------|
| 主路径失败且无描述且无 `upload_id` | `400` | `HS_RESOLVE_BAD_REQUEST` 或细分码 `DATASHEET_REQUIRED`（实现选其一并文档化）。 |
| `upload_id` 无效或过期 | `400` | 明确提示重新上传。 |
| 描述超长 | `400` | 提示上限。 |
| 文件类型/大小不合法 | `400` | 上传接口返回。 |
| 解析器未配置等现网行为 | 保持现有 `503` / `HS_RESOLVE_DISABLED` 等语义。 |

任务级失败（进入 resolver 后）仍可沿用 `HS_RESOLVE_FAILED` + `error_message`；**尽量**在 service 层对「缺输入」做早失败，减少无效任务行。

---

## 5. 安全与运维

- 日志：记录 `upload_id`、`asset_id`、`run_id`；**不**记录描述全文与文件内容。  
- 速率限制：上传接口建议按 IP/用户限流（与全局 API 网关策略对齐）。  
- 病毒扫描：若公司有统一扫描钩子，可在保存前接入（非首版强制，可在实现计划中列为可选）。

---

## 6. 测试要点

- **biz**：无候选 + 仅描述、无候选 + 仅上传、无候选 + 两者、有候选成功时 **忽略** 旁路。  
- **data**：抽取 prompt 组装单测（截断长度、块顺序、中英文与特殊字符清洗）。  
- **集成**：上传 → `ResolveByModel` 带 `manual_upload_id` → 任务非 `datasheet_failed`（在 LLM stub 或集成环境下）。  
- **§8.2 对应单测**：待 §8.2 拍板后，为每条闭合结论补用例——至少覆盖 **run 键与旁路输入**（换描述/换 `upload_id` 是否新 run 或须换 trace）、**缺输入 400 与对外错误码**、`upload_id` **无效/过期/越权**、**`user-upload://` 不触发 HTTP 拉取**、**datasheet 失败边界与旁路触发**、**同 trace 连续/并发上传** 与解析绑定的竞态。

---

## 7. 参考代码路径

- Resolver 与 datasheet 失败：`internal/biz/hs_model_resolver.go`（`resolveDatasheetAsset`、`ResolveByModel` 阶段机）。  
- 抽取 prompt：`internal/data/hs_llm_feature_extractor.go`（`buildExtractPrompt`、`pdftext.ReadBodyHeadFromFile`）。  
- HTTP 入口：`internal/service/hs_resolve_service.go`（`ResolveByModel`、`buildDatasheetCandidates`）。  
- 特征落库前置：`internal/biz/hs_model_resolver_features_persist.go`（`asset.ID != 0`）。

---

## 8. 已决与待决事项（评审闭合）

### 8.1 已决事项

| 事项 | 结论 |
|------|------|
| 旁路启用时机 | 仅当主路径 datasheet 阶段失败时评估手动描述/上传；主路径已成功则默认 **忽略** 旁路（§2.2、§2.4）。 |
| 大文件进解析请求 | **不** 使用 Base64 塞进 `ResolveByModel`；独立 multipart 上传（§3.3）。 |
| 用户上传 PDF 落库 | 必须 `Save` 得到 **`HsDatasheetAssetRecord` 且 `DownloadStatus = "ok"`**，以满足抽取与审计；占位 URL `user-upload://{upload_id}` 可用（§1.4、§3.2）。 |
| 仅文本、无持久化资产 | **`t_hs_model_features` 可不写入**（与现网 `asset.ID == 0` 时跳过 persist 一致）；解析流水线仍以 LLM 抽取结果驱动预筛/推荐（§1.4 补充、`hs_model_resolver_features_persist.go`）。 |
| Prompt 合并 | 描述 + PDF 时 **同一 prompt、固定块顺序**（§2.3）。 |

### 8.2 待决事项（写入实现计划时必须闭合）

| 事项 | 选项 / 说明 |
|------|-------------|
| **Run 键与旁路输入** | **A**：将 `manual_component_description`（规范化哈希）与/或 `manual_upload_id` 纳入 `makeRunID`（或等价 run 键）；**B**：产品约定「变更旁路输入须换 `request_trace_id`」，代码不改 run 公式。二选一写死并补单测（§3.1）。 |
| **缺输入错误码** | `HS_RESOLVE_BAD_REQUEST` 与细分 `DATASHEET_REQUIRED`（或 reason 字段）**择一**作为对外稳定契约，proto 与客户端文档对齐（§4）。 |
| **`upload_id` 鉴权** | 明确：仅创建者/同租户可消费，或对齐现网某认证中间件；防 `upload_id` 枚举与跨用户复用（§5 延伸）。 |
| **占位 URL 全链路** | 任何对 `DatasheetURL` 发起 HTTP 下载或「按 URL 拉取」的逻辑须识别 `user-upload://`，避免误请求（§3.2）。 |
| **「datasheet 失败」边界** | 无 downloader 时合成仅 URL、无本地路径/无 DB ID 等分支，是否算主路径失败、是否允许进入旁路——在实现计划中列清条件与用例（§2.1–§2.2）。 |
| **同 trace 多次上传** | 并发或连续两次上传时，`upload_id` 与待解析任务绑定顺序、是否允许多个 pending：实现计划写一条，避免与幂等策略冲突。 |
| **特征表强约束（可选后续）** | 若产品将来要求「仅描述」也必须落 `t_hs_model_features`：需另 spec（占位资产、`AssetID` 可空等），**非首版**。 |

**已在 plan 闭合：** 上表各项的拍板结论与落地范围以实现计划为对照清单 → [docs/superpowers/plans/2026-04-19-hs-resolve-manual-datasheet-implementation.md](../plans/2026-04-19-hs-resolve-manual-datasheet-implementation.md)（文内「§8.2 在本计划中的闭合结论」及 Task 1–9）。

---

## 9. 后续工作

1. 实现计划已产出：[docs/superpowers/plans/2026-04-19-hs-resolve-manual-datasheet-implementation.md](../plans/2026-04-19-hs-resolve-manual-datasheet-implementation.md)；合并前在本机跑通 `go test` / `go build` / `wire`（见 plan Task 9）。  
2. 可选：spec 评审子流程（若团队启用 `spec-document-reviewer`）。  
3. 第二版：`manual_overlay_when_datasheet_ok` 与更多 MIME 类型。
