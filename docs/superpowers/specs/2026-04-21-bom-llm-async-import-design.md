# BOM 导入（LLM 模式）异步解析与进度可视化设计

## 1. 目标与范围

### 1.1 背景

当前 `UploadBOM` 在 `parse_mode=llm` 下采用同步全表解析：读取首个工作表全部行，构造大 TSV prompt，一次性调用 LLM，再落库并生成搜索任务。大文件场景下经常因耗时过长导致请求超时，出现“任务尚未生成、用户无反馈”的体验问题。

### 1.2 目标

在保持 LLM 解析精度优先（策略 B）的前提下，实现以下结果：

1. `UploadBOM(parse_mode=llm)` 不再阻塞等待完整解析，避免超时。  
2. 用户可见导入进度（进度条 + 阶段文案），减少“卡死”感知。  
3. 解析完成后自动落库并生成待搜索任务，保持现有业务链路语义。  
4. 失败可观测、可重试，不出现静默失败。

### 1.3 非目标

- 首版不追求“前端零改动”；允许前端增加轮询与状态展示。  
- 首版不引入复杂编排系统（如外部消息队列）；先在现有服务与数据层完成。  
- 首版不改变非 LLM 解析模式行为（`auto/custom/...` 仍按现状同步解析）。

### 1.4 范围边界

- API：`api/bom/v1/bom.proto`（`UploadBOMReply`、`GetSessionReply` 相关扩展）。  
- 应用层：`internal/service/bom_service.go`（入口改异步、状态更新）。  
- 领域与持久化：`internal/biz`、`internal/data`（导入状态与任务持久化读写）。  
- 依赖关系遵循仓库 Kratos 约定：`service -> biz <- data`。

---

## 2. 现状链路与问题定位

现状关键路径（`parse_mode=llm`）：

1. 读取整张首表 `ReadBomImportFirstSheetFromReader`  
2. 构造全量 prompt `BuildBomLLMUserPrompt`  
3. 调用 `openai.Chat(...)`  
4. 解析 JSON `ParseBomImportLinesFromLLMJSON`  
5. `ReplaceSessionLines -> CancelAllTasksBySession -> UpsertPendingTasks`

问题在于：

- 以上步骤均在单次请求内同步完成，请求生命周期过长。  
- 解析未完成前，调用方拿不到阶段反馈。  
- 一旦网络抖动、模型慢响应或大文件触发长尾延迟，容易超时失败。

---

## 3. 总体方案

采用“异步导入任务 + 会话进度状态 + 分阶段高精度解析”。

### 3.1 核心原则

1. 精度优先：保留 LLM 解析主路径。  
2. 稳定性优先于单请求即时结果：上传快速返回。  
3. 可观测：每一阶段有结构化状态。  
4. 幂等与并发可控：同一 session 同时仅允许一个活跃导入任务。

### 3.2 高层流程

1. 客户端调用 `UploadBOM(parse_mode=llm)`。  
2. 服务端快速校验并创建导入任务（状态 `parsing`），立即返回。  
3. 后台 worker/协程执行解析并持续更新进度。  
4. 完成后将 session 状态置为 `ready`，失败置为 `failed`。  
5. 前端轮询 `GetSession`（或导入进度接口）驱动进度条。

---

## 4. API 与状态模型设计

### 4.1 `UploadBOM` 行为调整（llm 模式）

- 仍使用现有 RPC，不新增强制入口。  
- 当 `parse_mode=llm` 时：
  - 不再返回完整 `items/total` 作为完成结果语义；返回“已受理”。  
  - `bom_id` 仍返回 `session_id`，保证会话定位一致。  

为避免兼容性歧义，建议为 `UploadBOMReply` 新增字段：

- `accepted` (bool)：是否已受理导入任务。  
- `import_status` (string)：初始值通常为 `parsing`。  
- `import_message` (string)：例如“导入任务已启动”。

> 兼容策略：旧字段 `items/total` 保留，但在 llm 异步路径下不再作为“已解析完成”的判据。

### 4.2 会话导入状态（建议放入 session 结构化字段）

最小字段集合：

- `import_status`: `idle | parsing | ready | failed`  
- `import_progress`: `0~100`  
- `import_stage`: `validating | header_infer | chunk_parsing | persisting | done | failed`  
- `import_message`: 当前阶段说明  
- `import_error_code`: 失败码（如 `timeout`, `llm_429`, `invalid_json`）  
- `import_error`: 人类可读错误文本  
- `import_updated_at`: 最近更新时间

### 4.3 进度分配建议

- `5%`：任务创建与基础校验  
- `15%`：读取工作表与规模检查  
- `20%`：表头语义识别  
- `20%~85%`：分块解析（按块线性推进）  
- `85%~95%`：数据落库（替换 session lines）  
- `95%~100%`：搜索任务重建与收尾

---

## 5. 解析策略（精度优先）

### 5.1 两阶段 LLM（推荐）

阶段 A：表头语义识别  
- 输入：表头 + 少量样本行（例如前 20 行）。  
- 输出：列语义映射（model/manufacturer/package/qty/params/raw）。

阶段 B：分块行解析  
- 仅传数据行，按固定块大小（建议 200~400 行）调用 LLM。  
- 每块独立校验 JSON 与字段，失败可块级重试。  
- 合并各块结果后统一写入。

该方案相较“单次全表”可显著降低单请求 token 与时延长尾，同时保持 LLM 语义解析能力。

### 5.2 失败与重试

- 每块最多重试 1~2 次（指数退避）。  
- 超过阈值后任务失败并记录失败块索引。  
- 失败信息结构化写入 `import_error_code/import_error`，供 UI 展示与排障。

---

## 6. 一致性、门禁与幂等

### 6.1 会话门禁

当 `import_status=parsing` 时：

- `SearchQuotes` / `AutoMatch` / `GetMatchResult` 统一返回未就绪错误（例如 `BOM_NOT_READY`）。  
- 防止读取到旧行数据或空行数据导致错误配单。

### 6.2 并发上传策略

- 同一 `session_id` 在 `parsing` 时再次上传 llm 文件，默认拒绝（建议 `409` 语义）。  
- 后续可扩展 `force_restart`，首版不开放。

### 6.3 导入任务持久化

- 不能只依赖内存 goroutine。  
- 至少保证：服务重启后可识别“进行中/失败/完成”的导入状态，避免进度条卡死。  
- 建议新增轻量导入任务表（或在 session 上结构化持久化关键状态并补恢复逻辑）。

---

## 7. 前端交互约定

1. 上传成功后立即进入“解析中”态。  
2. 每 `1.5~2s` 轮询 `GetSession` 获取导入状态。  
3. 状态处理：
   - `parsing`：展示进度条与 `import_message`  
   - `ready`：跳转/刷新结果页  
   - `failed`：展示错误并提示重试导入

文案示例：
- `chunk_parsing`: “正在解析第 3/12 批物料”  
- `persisting`: “正在写入 BOM 并生成任务”

---

## 8. 观测与运维

日志建议最小字段：

- `session_id`  
- `import_task_id`  
- `stage`  
- `chunk_index` / `chunk_total`  
- `elapsed_ms`  
- `error_code`

指标建议：

- `bom_import_llm_duration_ms`（总耗时）  
- `bom_import_llm_chunk_fail_total`  
- `bom_import_llm_timeout_total`  
- `bom_import_llm_success_total`

---

## 9. 测试与验收

### 9.1 单元测试

- 分块进度计算正确。  
- 块级失败重试与最终失败状态正确。  
- `parsing` 门禁下相关接口返回未就绪。  
- 并发重复上传命中拒绝策略。

### 9.2 集成测试

- 上传 llm 大文件后接口快速返回（不超时）。  
- 后台完成后 session 转 `ready`，且搜索任务已生成。  
- 人为注入 LLM 超时/429，状态转 `failed` 且错误可见。  
- 服务重启后状态可恢复可读（不出现永久 parsing）。

### 9.3 验收标准

1. 大文件导入场景不再因同步等待导致网关超时。  
2. 前端全程可见进度与阶段。  
3. 成功路径下最终行数据与任务生成正确。  
4. 失败路径可诊断、可重试。

---

## 10. 分阶段实施计划（建议）

### Phase 1（止血）

- `UploadBOM(llm)` 异步化  
- session 导入状态字段 + 前端进度条  
- 基础失败落状态

### Phase 2（稳态）

- 两阶段 LLM + 分块解析  
- 块级重试 + 结构化错误码

### Phase 3（增强）

- 重启恢复与断点续跑  
- 运维面板与告警阈值

---

## 11. 待决事项（评审拍板）

1. `UploadBOMReply` 是否新增 `accepted/import_status` 字段，还是仅通过 `GetSession` 表达受理状态。  
2. 导入状态字段落在 `bom_session` 新列还是 `extra_json`（建议新列）。  
3. 是否首版即引入独立 `bom_import_task` 表（建议是）。  
4. 同 session 再上传的错误码与客户端提示文案最终定稿。
