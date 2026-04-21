# BOM LLM 异步导入前端实现设计

## 1. 目标

在现有 BOM 上传与会话页流程上补齐 `parse_mode=llm` 的异步导入前端体验：

- 上传 `llm` BOM 后立即跳转到会话页。
- 会话页在 `import_status=parsing` 时展示进度条、阶段文案和状态说明。
- 会话页轮询 `GetSession` 获取最新导入进度，并在 `ready`/`failed` 时停止轮询。
- 导入失败时向用户展示可读错误信息。
- 导入进行中时，禁用会导致后续流程误操作的入口。

本次只复用现有后端 `UploadBOM` 与 `GetSession` 扩展字段，不新增前端专用后端接口。

## 2. 范围

### 2.1 包含

- `web/src/api/types.ts`
- `web/src/api/bomLegacy.ts`
- `web/src/api/bomSession.ts`
- `web/src/pages/UploadPage.tsx`
- `web/src/pages/SourcingSessionPage.tsx`
- 相关前端测试文件

### 2.2 不包含

- 后端接口或 proto 再次调整
- 新增独立“导入进度”接口
- 匹配页、结果页的大范围交互改造
- WebSocket / SSE 实时推送

## 3. 用户流程

### 3.1 LLM 上传

1. 用户在上传页选择 `llm` 模式并上传文件。
2. 前端调用 `createSession`。
3. 前端调用 `uploadBOM(..., parse_mode=llm, session_id=...)`。
4. 若接口成功并返回 `bom_id`，立即跳转到会话页。
5. 会话页开始基于 `GetSession` 轮询导入状态。

### 3.2 会话页进度展示

- `import_status=parsing`
  - 显示导入状态卡片
  - 显示进度条、阶段、消息
  - 定时轮询
  - 禁用“配单”等后续动作

- `import_status=ready`
  - 停止轮询
  - 显示完成状态
  - 刷新一次会话与行数据
  - 恢复正常操作

- `import_status=failed`
  - 停止轮询
  - 显示失败状态、错误码、错误信息
  - 保留用户在会话页继续查看或重新导入的能力

## 4. API 设计

### 4.1 UploadBOM 返回扩展

前端 `uploadBOM` 返回值新增字段：

- `accepted: boolean`
- `import_status: string`
- `import_message: string`

兼容要求：

- 非 `llm` 模式仍兼容旧的 `items/total`
- `llm` 模式前端不依赖 `items/total`

### 4.2 GetSession 返回扩展

前端 `GetSessionReply` 新增字段：

- `import_status?: string`
- `import_progress?: number`
- `import_stage?: string`
- `import_message?: string`
- `import_error_code?: string`
- `import_error?: string`
- `import_updated_at?: string`

解析层同时兼容 `snake_case` 与 `camelCase`。

## 5. 页面行为

### 5.1 UploadPage

#### LLM 模式

- 上传成功后不等待解析完成。
- 直接调用 `onSuccess(res.bom_id)` 进入会话页。
- 如果后端返回：
  - `accepted=false`
  - 空 `bom_id`
  - 请求异常
  则停留在上传页展示错误。

#### 非 LLM 模式

- 保持现有同步导入逻辑。

### 5.2 SourcingSessionPage

新增导入状态卡片，放在会话标题与会话配置区之间，信息包括：

- 当前导入状态
- 进度百分比
- 阶段标识
- 状态说明文案
- 最近更新时间
- 失败时的错误码与错误详情

轮询策略：

- 仅在 `import_status=parsing` 时开启
- 轮询间隔 2 秒
- 页面卸载时清理 timer
- 状态变为 `ready` 或 `failed` 后立即停止

刷新策略：

- 每次轮询刷新 `getSession`
- 当检测到从 `parsing -> ready` 转换时，再额外刷新一次 `getBOMLines`
- `parsing` 期间不频繁刷新行列表，避免无意义闪动

禁用策略：

- `import_status=parsing` 时禁用：
  - “配单”
  - 手动触发后续流程的按钮
- 普通查看行为保持可用

## 6. UI 文案与状态映射

### 6.1 状态映射

- `idle`: 未开始导入
- `parsing`: 正在解析
- `ready`: 导入完成
- `failed`: 导入失败

### 6.2 阶段映射

- `validating`: 校验文件
- `header_infer`: 识别表头
- `chunk_parsing`: 分块解析
- `persisting`: 写入会话
- `done`: 已完成
- `failed`: 已失败

前端优先显示后端返回的 `import_message`；若为空，则使用上述本地映射兜底。

## 7. 测试设计

### 7.1 API 层

- `uploadBOM` 能解析 `accepted/import_status/import_message`
- `getSession` 能解析新增的导入状态字段

### 7.2 页面层

- 上传页在 `llm` 返回 accepted 后立即触发跳转
- 会话页在 `parsing` 状态时展示进度并启动轮询
- 会话页在状态变为 `ready` 时停止轮询并刷新行数据
- 会话页在状态变为 `failed` 时停止轮询并展示错误信息

## 8. 风险与约束

- 当前前端源码存在部分历史乱码文本；本次改动只在必要范围内触碰相关文件，避免做无关全文重写。
- 由于 `UploadPage.tsx` 与 `SourcingSessionPage.tsx` 已较长，实现时优先抽取小型 helper，避免继续膨胀。
- 若现有前端测试基础较弱，则至少补 API 解析与关键轮询行为测试。

## 9. 实施结论

推荐方案：

- 复用现有会话页作为异步导入承载页
- 上传成功后立即进入会话页
- 由会话页轮询 `GetSession` 展示导入进度

这样改动面最小，用户路径也最符合“进入会话后继续观察和操作”的已有心智。
