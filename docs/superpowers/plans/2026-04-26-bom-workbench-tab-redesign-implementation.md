# BOM Workbench Tab Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 BOM 工作台从“多个 Tab 复用同一大页面”改为“按任务拆分、按 Tab 懒加载、带搜索和分页的专用工作区”，降低重复信息、首屏负载和 Tab 切换成本。

**Architecture:** 前端以 `SessionWorkspace` 作为会话级容器，每个 Tab 渲染独立面板组件；各面板只请求自己需要的数据，并在面板内维护搜索、筛选、分页、批量操作和空状态。先完成前端结构拆分与本地分页，保留现有 API 兼容；随后扩展列表 API 参数，让大数据量场景改为服务端分页。

**Tech Stack:** React 19、TypeScript、Vite、Vitest、Testing Library、现有 REST API wrapper；后端分页扩展遵循 Kratos 分层，MySQL 访问使用 GORM。

---

## 背景与现状

当前 `web/src/pages/bom-workbench/SessionWorkspace.tsx` 中，`lines`、`gaps`、`maintenance` 三个 Tab 都渲染完整的 `SourcingSessionPage embedded sessionId={sessionId}`。`web/src/pages/SourcingSessionPage.tsx` 同时加载会话信息、BOM 行、搜索任务、缺口、匹配运行、维护配置，导致内容重复、信息密度过高、Tab 切换慢。

目标设计已沉淀在：

- `docs/superpowers/specs/2026-04-26-bom-workbench-tab-redesign.md`
- `docs/bom-workbench-tabs-approval.html`
- `docs/bom-workbench-tabs-detail-prototype.svg`
- `docs/bom-workbench-tabs-detail-preview.png`

---

## 目标信息架构

工作台保留左侧会话列表和顶部会话概览，右侧 Tab 拆为六类：

1. `总览`：会话状态、数据准备度、关键计数、下一步动作。
2. `BOM 行`：BOM 明细、行级编辑、可采购性、搜索、筛选、分页。
3. `搜索清洗`：平台搜索覆盖、异常任务、重试、搜索、筛选、分页。
4. `缺口处理`：待处理缺口、人工报价、替代料、运行记录、筛选、分页。
5. `维护`：平台启用、抬头字段、厂家规范、导出、维护搜索。
6. `匹配结果`：候选结果、供应商、状态、批量确认、搜索、筛选、分页。

---

## 文件计划

新增文件：

- `web/src/pages/bom-workbench/SessionOverviewPanel.tsx`
- `web/src/pages/bom-workbench/SessionLinesPanel.tsx`
- `web/src/pages/bom-workbench/SessionGapsPanel.tsx`
- `web/src/pages/bom-workbench/SessionMaintenancePanel.tsx`
- `web/src/pages/bom-workbench/SessionMatchResultPanel.tsx`
- `web/src/pages/bom-workbench/sessionPanelUtils.ts`

修改文件：

- `web/src/pages/bom-workbench/SessionWorkspace.tsx`
- `web/src/pages/bom-workbench/SessionSearchCleanPanel.tsx`
- `web/src/pages/bom-workbench/MatchResultWorkspace.tsx`
- `web/src/pages/BomWorkbenchPage.test.tsx`
- `web/src/pages/SourcingSessionPage.test.tsx`
- `web/src/api/bomSession.ts`
- `web/src/api/types.ts`

后端分页阶段可能修改：

- `api/caichip/v1/*.proto`
- `internal/service/*`
- `internal/biz/*`
- `internal/data/*`

---

## 开发步骤

### 1. 建立当前行为保护测试

- [ ] 修改 `web/src/pages/BomWorkbenchPage.test.tsx`，增加工作台 Tab 专用面板断言。
  - 点击会话后默认显示 `总览`。
  - 点击 `BOM 行` 时显示 `data-testid="session-lines-panel"`。
  - 点击 `搜索清洗` 时显示 `data-testid="session-search-clean-panel"`。
  - 点击 `缺口处理` 时显示 `data-testid="session-gaps-panel"`。
  - 点击 `维护` 时显示 `data-testid="session-maintenance-panel"`。
  - 点击 `匹配结果` 时显示 `data-testid="session-match-result-panel"`。
- [ ] 在同一个测试文件中增加“不同 Tab 不再重复渲染完整会话详情”的断言。
  - `BOM 行` 不出现维护配置标题。
  - `缺口处理` 不出现搜索任务表标题。
  - `维护` 不出现缺口处理操作按钮。
- [ ] 运行前端测试，确认这些新增断言在旧实现下失败。

命令：

```powershell
Set-Location D:\workspace\caichip\web
& 'D:\Program Files\nodejs\npm.cmd' test -- src/pages/BomWorkbenchPage.test.tsx
```

期望：新增断言失败，失败原因指向旧 Tab 仍复用 `SourcingSessionPage`。

提交：

```powershell
git add -- web/src/pages/BomWorkbenchPage.test.tsx
git commit -m "test(web): cover dedicated bom workspace tabs"
```

### 2. 增加通用面板工具

- [ ] 新建 `web/src/pages/bom-workbench/sessionPanelUtils.ts`。
- [ ] 提供分页和搜索工具，供多个面板复用。

代码结构：

```ts
export type PageSize = 20 | 50 | 100

export function normalizeKeyword(value: string): string {
  return value.trim().toLowerCase()
}

export function paginateRows<T>(rows: T[], page: number, pageSize: PageSize) {
  const safePage = Math.max(1, page)
  const totalPages = Math.max(1, Math.ceil(rows.length / pageSize))
  const currentPage = Math.min(safePage, totalPages)
  const start = (currentPage - 1) * pageSize

  return {
    page: currentPage,
    totalPages,
    total: rows.length,
    rows: rows.slice(start, start + pageSize),
  }
}
```

- [ ] 增加工具函数单测，放在 `web/src/pages/bom-workbench/sessionPanelUtils.test.ts`。
- [ ] 覆盖空数组、越界页码、关键词空格大小写归一化。

命令：

```powershell
Set-Location D:\workspace\caichip\web
& 'D:\Program Files\nodejs\npm.cmd' test -- src/pages/bom-workbench/sessionPanelUtils.test.ts
```

提交：

```powershell
git add -- web/src/pages/bom-workbench/sessionPanelUtils.ts web/src/pages/bom-workbench/sessionPanelUtils.test.ts
git commit -m "feat(web): add bom workspace panel utilities"
```

### 3. 实现总览面板

- [ ] 新建 `web/src/pages/bom-workbench/SessionOverviewPanel.tsx`。
- [ ] 面板入参使用 `sessionId`、`sessionStatus`、`sessionName`、`onRefresh`。
- [ ] 仅展示会话摘要、数据状态、关键指标入口和下一步操作，不加载 BOM 行、缺口、搜索任务列表。
- [ ] 在 `web/src/pages/bom-workbench/SessionWorkspace.tsx` 中将默认 Tab 改为 `overview` 时渲染该组件。

组件骨架：

```tsx
type SessionOverviewPanelProps = {
  sessionId: number
  sessionName: string
  sessionStatus: string
  onRefresh: () => void
}

export function SessionOverviewPanel(props: SessionOverviewPanelProps) {
  return (
    <section className="session-panel" data-testid="session-overview-panel">
      {/* 会话摘要、状态和下一步动作 */}
    </section>
  )
}
```

- [ ] 更新 `web/src/pages/BomWorkbenchPage.test.tsx`，确认进入会话后显示总览面板。

命令：

```powershell
Set-Location D:\workspace\caichip\web
& 'D:\Program Files\nodejs\npm.cmd' test -- src/pages/BomWorkbenchPage.test.tsx
```

提交：

```powershell
git add -- web/src/pages/bom-workbench/SessionOverviewPanel.tsx web/src/pages/bom-workbench/SessionWorkspace.tsx web/src/pages/BomWorkbenchPage.test.tsx
git commit -m "feat(web): add bom workspace overview panel"
```

### 4. 拆出 BOM 行面板

- [ ] 新建 `web/src/pages/bom-workbench/SessionLinesPanel.tsx`。
- [ ] 从 `SourcingSessionPage` 迁移 BOM 行读取、行编辑、新增、删除、覆盖统计显示相关逻辑。
- [ ] 面板首次挂载时调用 `getBOMLines(sessionId)` 和 `getSessionSearchTaskCoverage(sessionId)`。
- [ ] 添加工具栏：
  - 关键词搜索：行号、MPN、描述。
  - 厂家筛选。
  - 可用性筛选。
  - 每页条数：20、50、100。
- [ ] 使用 `sessionPanelUtils.ts` 对已加载行做本地筛选和分页。
- [ ] 增加空状态：
  - 未导入行：提示导入或新增 BOM 行。
  - 筛选无结果：提示清除筛选。
- [ ] 更新 `SessionWorkspace.tsx`，`lines` Tab 渲染 `SessionLinesPanel`。

测试：

- [ ] 在 `web/src/pages/BomWorkbenchPage.test.tsx` 中断言 `BOM 行` Tab 显示搜索框、分页和行表。
- [ ] 新增或扩展测试，覆盖输入 MPN 后仅显示匹配行。
- [ ] 覆盖点击下一页时当前页变化。

命令：

```powershell
Set-Location D:\workspace\caichip\web
& 'D:\Program Files\nodejs\npm.cmd' test -- src/pages/BomWorkbenchPage.test.tsx
```

提交：

```powershell
git add -- web/src/pages/bom-workbench/SessionLinesPanel.tsx web/src/pages/bom-workbench/SessionWorkspace.tsx web/src/pages/BomWorkbenchPage.test.tsx
git commit -m "feat(web): split bom lines workspace panel"
```

### 5. 强化搜索清洗面板

- [ ] 修改 `web/src/pages/bom-workbench/SessionSearchCleanPanel.tsx`。
- [ ] 保留现有会话搜索任务读取、异常任务识别、重试逻辑。
- [ ] 增加工具栏：
  - 关键词搜索：MPN、任务标识、错误信息。
  - 平台筛选。
  - 状态筛选。
  - 每页条数：20、50、100。
- [ ] 使用 `sessionPanelUtils.ts` 对任务列表做本地筛选和分页。
- [ ] 调整重试按钮作用域：默认重试当前筛选结果中的失败任务，并在按钮文案中显示数量。
- [ ] 保留整体异常任务数量摘要，避免用户误以为只存在当前页异常。

测试：

- [ ] 更新 `web/src/pages/BomWorkbenchPage.test.tsx` 中搜索清洗相关断言。
- [ ] 覆盖状态筛选后重试按钮数量变化。
- [ ] 覆盖分页切换不触发额外重试调用。

命令：

```powershell
Set-Location D:\workspace\caichip\web
& 'D:\Program Files\nodejs\npm.cmd' test -- src/pages/BomWorkbenchPage.test.tsx
```

提交：

```powershell
git add -- web/src/pages/bom-workbench/SessionSearchCleanPanel.tsx web/src/pages/BomWorkbenchPage.test.tsx
git commit -m "feat(web): add search clean filters and pagination"
```

### 6. 拆出缺口处理面板

- [ ] 新建 `web/src/pages/bom-workbench/SessionGapsPanel.tsx`。
- [ ] 从 `SourcingSessionPage` 迁移缺口列表、缺口状态筛选、匹配运行保存、人工报价、替代料选择逻辑。
- [ ] 面板首次挂载时调用 `listLineGaps(sessionId, statuses)` 和 `listMatchRuns(sessionId)`。
- [ ] 添加工具栏：
  - 关键词搜索：MPN、厂家、原因。
  - 缺口类型筛选。
  - 处理状态筛选。
  - 每页条数：20、50、100。
- [ ] 使用 `sessionPanelUtils.ts` 对缺口列表做本地筛选和分页。
- [ ] 将运行记录放在面板右侧或底部的固定区域，避免和缺口明细混在同一表格。
- [ ] 更新 `SessionWorkspace.tsx`，`gaps` Tab 渲染 `SessionGapsPanel`。

测试：

- [ ] 在 `web/src/pages/BomWorkbenchPage.test.tsx` 中断言 `缺口处理` Tab 显示缺口过滤器和运行记录。
- [ ] 保留 `web/src/pages/SourcingSessionPage.test.tsx` 中缺口保存、人工报价、替代料选择的行为断言，并补充到新面板测试路径。
- [ ] 覆盖缺口状态筛选后只显示对应缺口。

命令：

```powershell
Set-Location D:\workspace\caichip\web
& 'D:\Program Files\nodejs\npm.cmd' test -- src/pages/BomWorkbenchPage.test.tsx src/pages/SourcingSessionPage.test.tsx
```

提交：

```powershell
git add -- web/src/pages/bom-workbench/SessionGapsPanel.tsx web/src/pages/bom-workbench/SessionWorkspace.tsx web/src/pages/BomWorkbenchPage.test.tsx web/src/pages/SourcingSessionPage.test.tsx
git commit -m "feat(web): split bom gap handling panel"
```

### 7. 拆出维护面板

- [ ] 新建 `web/src/pages/bom-workbench/SessionMaintenancePanel.tsx`。
- [ ] 从 `SourcingSessionPage` 迁移会话字段维护、平台启用、厂家规范、导出相关逻辑。
- [ ] 面板首次挂载时加载维护所需数据，不加载 BOM 行、缺口、搜索任务列表。
- [ ] 添加维护搜索：
  - 平台名称搜索。
  - 厂家规范搜索。
  - 字段名搜索。
- [ ] 将高频维护动作放在顶部操作区：保存、导出、刷新。
- [ ] 更新 `SessionWorkspace.tsx`，`maintenance` Tab 渲染 `SessionMaintenancePanel`。

测试：

- [ ] 在 `web/src/pages/BomWorkbenchPage.test.tsx` 中断言 `维护` Tab 显示平台维护和导出操作。
- [ ] 覆盖维护搜索过滤平台列表。
- [ ] 覆盖保存平台配置时调用 `putPlatforms`。

命令：

```powershell
Set-Location D:\workspace\caichip\web
& 'D:\Program Files\nodejs\npm.cmd' test -- src/pages/BomWorkbenchPage.test.tsx src/pages/SourcingSessionPage.test.tsx
```

提交：

```powershell
git add -- web/src/pages/bom-workbench/SessionMaintenancePanel.tsx web/src/pages/bom-workbench/SessionWorkspace.tsx web/src/pages/BomWorkbenchPage.test.tsx web/src/pages/SourcingSessionPage.test.tsx
git commit -m "feat(web): split bom maintenance panel"
```

### 8. 完善匹配结果面板

- [ ] 新建 `web/src/pages/bom-workbench/SessionMatchResultPanel.tsx`。
- [ ] 保留现有 `MatchResultWorkspace` 的数据来源和自动匹配入口。
- [ ] 在新面板中增加：
  - 关键词搜索：MPN、供应商、厂家。
  - 状态筛选。
  - 每页条数：20、50、100。
  - 批量确认操作入口。
- [ ] 修改 `web/src/pages/bom-workbench/MatchResultWorkspace.tsx`，将匹配结果列表区域作为 `SessionMatchResultPanel` 的数据展示入口。
- [ ] 更新 `SessionWorkspace.tsx`，`match` Tab 渲染 `SessionMatchResultPanel`，继续保留 `data_ready` 之前的禁用逻辑。

测试：

- [ ] 在 `web/src/pages/BomWorkbenchPage.test.tsx` 中断言 `data_ready=false` 时 `匹配结果` Tab 禁用。
- [ ] 在 `data_ready=true` 场景断言 `匹配结果` Tab 显示搜索框、状态筛选和分页。
- [ ] 覆盖关键词搜索后候选结果列表收敛。

命令：

```powershell
Set-Location D:\workspace\caichip\web
& 'D:\Program Files\nodejs\npm.cmd' test -- src/pages/BomWorkbenchPage.test.tsx
```

提交：

```powershell
git add -- web/src/pages/bom-workbench/SessionMatchResultPanel.tsx web/src/pages/bom-workbench/MatchResultWorkspace.tsx web/src/pages/bom-workbench/SessionWorkspace.tsx web/src/pages/BomWorkbenchPage.test.tsx
git commit -m "feat(web): add match result workspace panel"
```

### 9. 降低旧大页面职责

- [ ] 检查 `web/src/pages/SourcingSessionPage.tsx` 中已迁移的逻辑。
- [ ] 保留它作为独立历史页面入口时，只承担单页流程兼容职责。
- [ ] 从 `SessionWorkspace.tsx` 删除 `SourcingSessionPage` 引用。
- [ ] 将 `SourcingSessionPage.test.tsx` 中只属于工作台 Tab 的断言迁移到工作台测试；保留历史页面必要测试。
- [ ] 确认 `SourcingSessionPage.tsx` 未新增更多职责。

命令：

```powershell
Set-Location D:\workspace\caichip\web
& 'D:\Program Files\nodejs\npm.cmd' test -- src/pages/SourcingSessionPage.test.tsx src/pages/BomWorkbenchPage.test.tsx
```

提交：

```powershell
git add -- web/src/pages/SourcingSessionPage.tsx web/src/pages/SourcingSessionPage.test.tsx web/src/pages/bom-workbench/SessionWorkspace.tsx web/src/pages/BomWorkbenchPage.test.tsx
git commit -m "refactor(web): decouple sourcing page from bom workspace tabs"
```

### 10. 扩展前端 API 查询参数

- [ ] 修改 `web/src/api/types.ts`，增加列表查询参数类型。

建议类型：

```ts
export type ListQueryParams = {
  q?: string
  page?: number
  pageSize?: number
  status?: string
  platform?: string
  manufacturer?: string
}

export type PaginatedResult<T> = {
  items: T[]
  total: number
  page: number
  pageSize: number
}
```

- [ ] 修改 `web/src/api/bomSession.ts`：
  - `getBOMLines(sessionId, params?)`
  - `listSessionSearchTasks(sessionId, params?)`
  - `listLineGaps(sessionId, statuses?, params?)`
- [ ] 参数序列化使用 `URLSearchParams`，避免手写拼接查询字符串。
- [ ] 保持旧调用方式可用。
- [ ] 修改 `web/src/api/bomSession.test.ts`，覆盖无参数和带参数请求地址。

命令：

```powershell
Set-Location D:\workspace\caichip\web
& 'D:\Program Files\nodejs\npm.cmd' test -- src/api/bomSession.test.ts
```

提交：

```powershell
git add -- web/src/api/types.ts web/src/api/bomSession.ts web/src/api/bomSession.test.ts
git commit -m "feat(web): add bom workspace list query params"
```

### 11. 服务端分页接口落地

- [ ] 根据实际 HTTP 路由和 proto 定义，给 BOM 行、搜索任务、缺口列表、匹配结果列表增加查询参数。
- [ ] 在 `internal/service` 解析 `q`、`page`、`page_size`、`status`、`platform`、`manufacturer`。
- [ ] 在 `internal/biz` 定义列表查询对象和分页返回对象，业务层负责参数校验和默认值。
- [ ] 在 `internal/data` 使用 GORM 构造查询，禁止拼接 SQL 字符串。
- [ ] 数据库查询必须有稳定排序，优先按会话 ID、行号、创建时间或主键排序。
- [ ] 返回格式保留现有字段，并附带总数、页码、页大小。
- [ ] 为仓储实现增加单元测试或集成测试，覆盖关键词、状态、分页边界。

后端验证命令按仓库现有脚本选择：

```powershell
Set-Location D:\workspace\caichip
go test ./...
```

提交：

```powershell
git add -- api internal
git commit -m "feat(bom): support paginated workspace lists"
```

### 12. 前端切换到服务端分页

- [ ] 将 `SessionLinesPanel`、`SessionSearchCleanPanel`、`SessionGapsPanel`、`SessionMatchResultPanel` 的搜索、筛选、分页状态传入 API。
- [ ] 输入框使用受控状态，并在提交搜索或防抖后请求数据。
- [ ] 后端返回分页元数据时使用服务端总数；后端未返回分页元数据时保留本地分页兼容路径。
- [ ] 分页按钮切换时只刷新当前面板数据，不刷新整个工作台。
- [ ] 清除筛选时回到第一页。

测试：

- [ ] 覆盖修改搜索条件后 API 请求参数变化。
- [ ] 覆盖切换分页后只调用当前面板 API。
- [ ] 覆盖清除筛选后页码重置。

命令：

```powershell
Set-Location D:\workspace\caichip\web
& 'D:\Program Files\nodejs\npm.cmd' test -- src/pages/BomWorkbenchPage.test.tsx src/api/bomSession.test.ts
```

提交：

```powershell
git add -- web/src/pages/bom-workbench web/src/pages/BomWorkbenchPage.test.tsx web/src/api/bomSession.ts web/src/api/bomSession.test.ts
git commit -m "feat(web): use server pagination in bom workspace tabs"
```

### 13. 视觉和交互验收

- [ ] 启动前端开发服务。

命令：

```powershell
Set-Location D:\workspace\caichip\web
& 'D:\Program Files\nodejs\npm.cmd' run dev
```

- [ ] 在浏览器访问 `http://localhost:5173/`。
- [ ] 使用账号 `admin`、密码 `12345678` 登录。
- [ ] 打开 BOM 工作台并选择一个已有会话。
- [ ] 验收以下交互：
  - 默认进入 `总览`，不会一次性加载所有列表。
  - 每个 Tab 只出现该 Tab 对应功能。
  - `BOM 行`、`搜索清洗`、`缺口处理`、`匹配结果` 均有搜索、筛选、分页。
  - `维护` 只展示维护相关功能。
  - Tab 切换不丢失左侧会话列表和顶部会话上下文。
  - `匹配结果` 在数据未准备完成时继续禁用。

### 14. 最终回归

- [ ] 运行前端定向测试。

```powershell
Set-Location D:\workspace\caichip\web
& 'D:\Program Files\nodejs\npm.cmd' test -- src/pages/BomWorkbenchPage.test.tsx src/pages/SourcingSessionPage.test.tsx src/api/bomSession.test.ts
```

- [ ] 运行前端完整测试。

```powershell
Set-Location D:\workspace\caichip\web
& 'D:\Program Files\nodejs\npm.cmd' test
```

- [ ] 运行前端构建。

```powershell
Set-Location D:\workspace\caichip\web
& 'D:\Program Files\nodejs\npm.cmd' run build
```

- [ ] 如果执行了后端分页阶段，运行后端测试。

```powershell
Set-Location D:\workspace\caichip
go test ./...
```

- [ ] 检查未提交文件，确认没有覆盖用户已有改动。

```powershell
Set-Location D:\workspace\caichip
git status --short
```

---

## 自审清单

- [ ] `SessionWorkspace.tsx` 不再让多个 Tab 渲染同一个 `SourcingSessionPage`。
- [ ] 每个新面板都有 `data-testid`，便于测试和后续定位。
- [ ] 每个列表类 Tab 都具备搜索、筛选、分页。
- [ ] 大列表请求只发生在对应 Tab 首次进入或筛选分页变化时。
- [ ] 现有 API 调用保持向后兼容。
- [ ] 后端数据库读写全部通过 GORM 完成。
- [ ] 新增或修改的代码文件默认不超过 300 行；超过时继续拆分组件或工具函数。
- [ ] 文档和用户可见文案使用中文，代码标识符保持英文。

---

## 风险与处理

- `SourcingSessionPage.tsx` 当前体积较大，迁移时容易遗漏副作用。处理方式：先补测试，再按 Tab 迁移，每次只移动一类职责。
- 服务端分页会影响接口契约。处理方式：前端 API wrapper 先保持旧返回兼容，再逐步启用分页返回。
- 搜索清洗、缺口处理存在批量操作。处理方式：批量按钮明确显示作用范围，例如“重试当前筛选失败任务 8 个”。
- 匹配结果依赖 `data_ready`。处理方式：保留现有禁用逻辑，只改变可进入后的列表组织方式。

---

## 完成标准

- `BOM 行`、`搜索清洗`、`缺口处理`、`维护`、`匹配结果` 五个 Tab 的内容互不重复。
- 进入单个 Tab 时只加载该 Tab 必要数据。
- 列表类 Tab 可搜索、可筛选、可分页。
- 旧的会话详情大页不再作为工作台多个 Tab 的共同渲染体。
- 定向测试、完整前端测试、前端构建通过；若执行后端阶段，`go test ./...` 通过。
