# BOM 工作台 Tab 重构设计

日期：2026-04-26

## 1. 背景

当前 BOM 工作台已经有会话列表和会话工作区，但工作区内的多个 Tab 职责不清：

- `BOM行`、`缺口处理`、`维护` 三个 Tab 都复用完整的 `SourcingSessionPage`。
- `SourcingSessionPage.tsx` 当前约 1064 行，同时承载导入状态、搜索任务、BOM 行、缺口、匹配 run、单据信息、平台维护和导出操作。
- 完整详情页挂载时会加载多组数据，切换 Tab 容易造成重复内容、重复请求和响应慢。

本设计基于以下原型：

- `docs/bom-workbench-tabs-detail-prototype.svg`
- `docs/bom-workbench-tabs-detail-preview.png`

## 2. 设计目标

1. 每个 Tab 只承载一个明确任务，避免重复展示完整详情页。
2. 大数据量 Tab 提供搜索、筛选、分页，减少信息噪音和渲染压力。
3. 每个 Tab 只加载自身需要的数据，避免切换时请求所有会话详情。
4. 保留当前工作台左侧会话列表、移动端返回列表、上传后进入会话的主流程。
5. 不改变现有后端业务规则；若需要分页参数，作为 API 扩展设计，不在 data 层写业务决策。

## 3. 信息架构

工作台保持左右两栏：

- 左侧：`SessionListPanel`，只负责会话检索、筛选、分页和选择。
- 右侧：`SessionWorkspace`，负责会话摘要、Tab 状态、Tab 到面板的映射。

右侧 Tab 保留六个：

| Tab | 职责 | 是否需要搜索 | 是否需要筛选 | 是否需要分页 |
| --- | --- | --- | --- | --- |
| 概览 | 会话摘要、导入状态、关键计数、下一步入口 | 否 | 否 | 否 |
| BOM 行 | BOM 行列表、新增、编辑、删除、可用性提示 | 是 | 是 | 是 |
| 搜索清洗 | 搜索任务状态、重试、厂家别名审核 | 是 | 是 | 是 |
| 缺口处理 | 无数据、人工报价、替代料、保存匹配 run | 是 | 是 | 可选，超过 50 条启用 |
| 维护 | 单据信息、客户信息、平台选择、导出 | 否 | 简单平台搜索 | 否 |
| 匹配结果 | 最终配单、未解决提示、导出、HS 跳转 | 是 | 是 | 是 |

## 4. Tab 详细设计

### 4.1 概览

概览只展示摘要，不承载大表和表单：

- 导入状态：`parsing`、`ready`、`failed` 等。
- BOM 行数、搜索任务覆盖率、待处理缺口数。
- 当前会话状态和是否可进入匹配结果。
- 下一步入口：进入 BOM 行、搜索清洗、缺口处理或匹配结果。

数据来源：

- `getSession(sessionId)`
- 必要的轻量统计接口；若没有统计接口，第一阶段可复用现有接口但只在概览加载需要的摘要。

### 4.2 BOM 行

BOM 行 Tab 只处理物料明细。

界面结构：

- 顶部统计：总行数、搜索覆盖率、可用性提醒。
- 搜索筛选栏：
  - 关键字：`MPN / 行号`
  - 厂家
  - 可用性：全部、ready、no_data、collection_unavailable、no_match_after_filter
  - 每页条数：默认 50
- 操作区：新增 BOM 行。
- 表格：行号、MPN、厂家、封装、数量、可用性、平台缺口、操作。
- 分页：总数、上一页、页码、下一页。

建议 API：

```text
GET /api/v1/bom-sessions/{session_id}/lines?page=1&page_size=50&q=&mfr=&availability_status=
```

如果后端暂不支持分页，前端第一阶段可以先保留现有 `getBOMLines(sessionId)`，但实现时需明确这是过渡方案；大 BOM 场景应补后端分页。

### 4.3 搜索清洗

搜索清洗 Tab 聚焦搜索任务和厂家别名。

界面结构：

- 顶部统计：总任务、成功、可重试异常、厂家别名待审。
- 搜索筛选栏：
  - 平台：全部、hqchip、icgoo、digikey、mouser 等。
  - 搜索状态：全部、pending、searching、succeeded、no_data、failed、missing。
  - 关键字：`MPN / Agent`
  - 每页条数：默认 50
- 操作：批量重试、刷新。
- 表格：MPN、平台、搜索状态、Agent 状态、更新时间、错误信息、操作。
- 厂家别名审核：alias、推荐 canonical、通过、忽略、手动修正。

建议 API：

```text
GET /api/v1/bom-sessions/{session_id}/search-tasks?page=1&page_size=50&q=&platform_id=&search_ui_state=&retryable=
```

批量重试仍使用现有 `retrySearchTasks`，但只对筛选后的可重试异常执行，且不重复重试 `no_data`。

### 4.4 缺口处理

缺口处理 Tab 聚焦未解决行。

界面结构：

- 顶部统计：开放缺口、人工报价已补、最近匹配 run。
- 搜索筛选栏：
  - 状态：open、manual_quote_added、substitute_selected
  - 类型：NO_DATA、MFR_MISMATCH、NO_MATCH_AFTER_FILTER 等
  - 关键字：MPN、厂家
- 主内容：
  - 左侧：当前缺口卡片，展示原因、人工报价表单。
  - 右侧：替代料建议、选择替代料。
- 底部：保存匹配 run、未解决提示、总数/页码。

分页策略：

- 默认显示开放缺口列表或第一条缺口详情。
- 缺口数超过 50 时启用分页；低于 50 时只使用筛选和滚动。

建议 API：

```text
GET /api/v1/bom-sessions/{session_id}/line-gaps?page=1&page_size=50&q=&status=&gap_type=
```

缺口处理的业务规则仍保留在 biz 层，前端只提交用户选择或人工报价。

### 4.5 维护

维护 Tab 放置配置类操作，不展示大表。

界面结构：

- 单据信息：标题、客户、联系人、电话、邮箱、备注。
- 平台选择：平台 chip，支持平台名称搜索。
- 导出：XLSX、CSV。
- 维护提示：平台保存会更新 selection_revision，并影响后续搜索任务覆盖。

不需要分页。平台数量增多时，只做本地平台搜索。

### 4.6 匹配结果

匹配结果 Tab 只在会话 `data_ready` 后启用。

界面结构：

- 顶部统计：匹配状态、已匹配行、未解决行、总金额。
- 搜索筛选栏：
  - 关键字：`MPN / 供应商`
  - 匹配状态：全部、matched、manual_quote、substitute、unresolved
  - 每页条数：默认 50
- 表格：行号、需求 MPN、推荐供应、库存、单价、匹配状态、HS 动作。
- 底部：未解决提示、导出配单、批量 HS。

建议 API：

```text
GET /api/v1/bom-sessions/{session_id}/match-result?page=1&page_size=50&q=&match_status=
```

若现有 `MatchResultPage` 使用 `autoMatch` 一次性加载全部结果，应设计分页版本，避免大 BOM 下结果页卡顿。

## 5. 组件拆分

推荐新增或调整以下前端组件：

```text
web/src/pages/bom-workbench/
  SessionWorkspace.tsx
  SessionOverviewPanel.tsx
  SessionLinesPanel.tsx
  SessionSearchCleanPanel.tsx
  SessionGapsPanel.tsx
  SessionMaintenancePanel.tsx
  MatchResultWorkspace.tsx
  sessionTabs.ts
```

职责边界：

- `SessionWorkspace`：只维护当前 Tab、会话状态、是否可进入匹配结果。
- `SessionOverviewPanel`：加载摘要和下一步入口。
- `SessionLinesPanel`：加载和操作 BOM 行。
- `SessionSearchCleanPanel`：加载搜索任务、重试、厂家别名。
- `SessionGapsPanel`：加载缺口、提交人工报价、选择替代料、保存 run。
- `SessionMaintenancePanel`：维护单据信息、平台、导出。
- `MatchResultWorkspace`：保留匹配结果入口，逐步支持分页筛选。

`SourcingSessionPage` 不再作为多个 Tab 的共同内容。第一阶段可以把它瘦身为组合壳或兼容旧入口；最终应拆到 300 行以内，符合仓库单文件长度规则。

## 6. 数据加载策略

当前问题是一个完整详情页挂载时加载所有数据。改造后按 Tab 加载：

| Tab | 加载时机 | 主要请求 |
| --- | --- | --- |
| 概览 | 选择会话后默认加载 | `getSession`、轻量统计 |
| BOM 行 | 首次进入 BOM 行 Tab | `listSessionLines`、coverage |
| 搜索清洗 | 首次进入搜索清洗 Tab | `listSessionSearchTasks`、厂家别名相关 |
| 缺口处理 | 首次进入缺口处理 Tab | `listLineGaps`、`listMatchRuns` |
| 维护 | 首次进入维护 Tab | `getSession` |
| 匹配结果 | `data_ready` 且进入匹配结果 Tab | 分页 match result |

缓存策略：

- 同一会话同一 Tab 的数据可在前端状态中保留，手动点击刷新时重新拉取。
- 切换会话时清空 Tab 内数据。
- 修改操作成功后只刷新当前 Tab 需要的数据，不刷新所有 Tab。

## 7. 后端与 API 影响

若后端已有全量接口，第一阶段可以先前端改结构；但为了真正解决慢响应，建议补分页参数：

- BOM 行列表分页。
- 搜索任务分页。
- 缺口列表分页或可选分页。
- 匹配结果分页。

后端实现约束：

- 数据库读写使用 GORM。
- 查询参数必须参数化，不在业务代码拼接 SQL。
- 分页、筛选的业务语义放在 `internal/biz` 或 service 编排中；`internal/data` 只做持久化查询实现。

## 8. 交互与状态

- 匹配结果 Tab：
  - `session.status !== data_ready` 时禁用。
  - 禁用提示显示当前状态和进入条件。
- 搜索/筛选：
  - 输入关键字后点击搜索触发请求。
  - 变更筛选条件后页码回到 1。
- 分页：
  - 默认每页 50。
  - 分页控件显示总数、当前页、上一页、下一页。
- 移动端：
  - 保留返回会话列表按钮。
  - Tab 横向滚动。
  - 表格横向滚动，不压缩关键列。

## 9. 测试设计

前端测试：

- 选择会话后默认进入概览。
- 每个 Tab 渲染独立面板，不再出现 `SourcingSessionPage` 的完整内容重复。
- `BOM 行` Tab 展示搜索、筛选、分页控件。
- `搜索清洗` Tab 展示搜索、筛选、分页和批量重试。
- `缺口处理` Tab 展示状态/类型筛选和保存 run。
- `维护` Tab 不展示分页。
- `匹配结果` Tab 在非 `data_ready` 时禁用，在 `data_ready` 时展示搜索、筛选、分页。
- 操作成功后只刷新当前 Tab 数据。

后端测试（如新增分页接口）：

- 分页参数默认值和边界值。
- 关键字、状态、平台、厂家筛选。
- GORM 查询不拼接用户输入。
- 总数和当前页数据一致。

## 10. 分阶段实施

### 阶段一：前端结构拆分

目标：解决重复内容和信息杂乱。

- 新增概览、BOM 行、缺口处理、维护面板。
- 更新 `SessionWorkspace` 的 Tab 映射。
- `搜索清洗` 和 `匹配结果` 先复用现有能力。
- 补充 Tab 渲染测试。

### 阶段二：搜索、筛选、分页

目标：解决大数据量下的性能和查找效率。

- 为 BOM 行、搜索任务、匹配结果补分页参数。
- 缺口处理按数据量决定是否启用分页。
- 前端接入分页控件和筛选参数。

### 阶段三：瘦身旧详情页

目标：满足单文件长度规则并降低维护成本。

- 将 `SourcingSessionPage` 拆分或退役。
- 保留必要兼容入口。
- 删除不再使用的重复逻辑。

## 11. 验收标准

- `BOM 行`、`搜索清洗`、`缺口处理`、`维护`、`匹配结果` 五个 Tab 的首屏内容明显不同。
- 用户在任一 Tab 中只看到该任务相关操作。
- 大表类 Tab 都有搜索/筛选/分页。
- 切换 Tab 不触发无关数据请求。
- 移动端可选择会话、进入详情、返回列表、横向查看表格。
- 现有登录、上传 BOM、选择会话、进入匹配结果、HS 跳转流程不回退。

## 12. 非目标

- 本次不重做主导航和登录面板。
- 本次不改变后端匹配、缺口判定、搜索任务重试的业务规则。
- 本次不引入新的 UI 组件库。
- 本次不处理无关的 `bomMatchExtras.ts` 本地改动。
