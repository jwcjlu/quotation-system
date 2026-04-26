# BOM 工作台布局优化 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将现有 `BOM会话` 与 `匹配单` 两个入口合并为一个 `BOM工作台`，在同一页面内完成会话选择、搜索与清洗、缺口处理和匹配单查看。

**Architecture:** 前端采用容器拆分方案：`App.tsx` 只保留一级导航；新建 `BomWorkbenchPage` 管理会话选择和桌面/移动布局；`SessionWorkspace` 管理当前会话的加载、推荐 tab、刷新协调；原 `SourcingSessionPage` 与 `MatchResultPage` 的大块 UI 分阶段抽为可嵌入面板，避免继续扩大超过 300 行的文件。厂牌别名审核归入 `搜索与清洗` tab，匹配单只保留风险提示和结果展示。

**Tech Stack:** React 19、TypeScript、Vite、Vitest、Testing Library、Tailwind CSS、现有 `web/src/api/*` 客户端。

---

## File Structure

新增文件：

- `web/src/pages/BomWorkbenchPage.tsx`：BOM 工作台顶层容器，管理选中会话 ID、移动端列表/详情切换、上传弹窗状态。
- `web/src/pages/BomWorkbenchPage.test.tsx`：工作台容器集成测试。
- `web/src/pages/bom-workbench/SessionListPanel.tsx`：会话筛选、列表、分页、新建入口。
- `web/src/pages/bom-workbench/SessionWorkspace.tsx`：当前会话头部、状态摘要、二级 tab、推荐 tab、刷新协调。
- `web/src/pages/bom-workbench/sessionTabs.ts`：tab 类型、推荐 tab 纯函数、状态标签工具。
- `web/src/pages/bom-workbench/SessionOverviewPanel.tsx`：总览指标和导入状态摘要。
- `web/src/pages/bom-workbench/SessionLinesPanel.tsx`：BOM 行管理，从 `SourcingSessionPage.tsx` 拆出。
- `web/src/pages/bom-workbench/SessionSearchCleanPanel.tsx`：搜索任务状态与厂牌别名审核。
- `web/src/pages/bom-workbench/ManufacturerAliasReviewPanel.tsx`：厂牌别名审核可复用组件，从 `MatchResultPage.tsx` 拆出。
- `web/src/pages/bom-workbench/SessionGapsPanel.tsx`：缺口处理与方案版本，从 `SourcingSessionPage.tsx` 拆出。
- `web/src/pages/bom-workbench/SessionMaintenancePanel.tsx`：单据信息与平台勾选。
- `web/src/pages/bom-workbench/MatchResultWorkspace.tsx`：匹配单嵌入式内容，从 `MatchResultPage.tsx` 拆出。

修改文件：

- `web/src/App.tsx`：将 `bom-list` 改为 `bom-workbench`；移除 `result` 一级导航；传入 HS 跳转回调。
- `web/src/App.test.tsx`：更新权限导航断言。
- `web/src/pages/BomSessionListPage.tsx`：第一阶段可保留导出或改成兼容 wrapper，最终由 `BomWorkbenchPage` 取代。
- `web/src/pages/SourcingSessionPage.tsx`：保留兼容导出时只组合新面板；本计划执行完成后该文件应显著减少行数。
- `web/src/pages/SourcingSessionPage.test.tsx`：迁移或补充到工作台/面板测试。
- `web/src/pages/MatchResultPage.tsx`：保留独立兼容页面时只包装 `MatchResultWorkspace`；厂牌别名审核移出。

现有文件风险：

- `web/src/pages/SourcingSessionPage.tsx` 当前约 1209 行。
- `web/src/pages/MatchResultPage.tsx` 当前约 1430 行。
- 实施时必须按职责拆分；不要在这两个文件里继续追加大块 UI。

---

### Task 1: 一级导航合并为 BOM 工作台

**Files:**
- Modify: `web/src/App.tsx`
- Modify: `web/src/App.test.tsx`
- Create: `web/src/pages/BomWorkbenchPage.tsx`

- [ ] **Step 1: 写失败测试，验证普通用户只看到 BOM工作台，不再看到匹配单一级入口**

在 `web/src/App.test.tsx` 中更新 mock：

```tsx
vi.mock('./pages/BomWorkbenchPage', () => ({
  BomWorkbenchPage: () => <div>bom workbench page</div>,
}))
```

将原来对 `BomSessionListPage` 的 mock 替换掉，并把普通用户测试改成：

```tsx
expect(await screen.findByRole('button', { name: 'BOM\u5de5\u4f5c\u53f0' })).toBeInTheDocument()
expect(screen.queryByRole('button', { name: '\u5339\u914d\u5355' })).not.toBeInTheDocument()
expect(screen.getByText('bom workbench page')).toBeInTheDocument()
```

- [ ] **Step 2: 运行测试确认失败**

Run:

```powershell
cd web
& 'D:\Program Files\nodejs\npm.cmd' test -- src/App.test.tsx
```

Expected: FAIL，原因是 `BOM工作台` 按钮不存在，仍渲染 `BOM会话` 或 `匹配单`。

- [ ] **Step 3: 创建最小 `BomWorkbenchPage`**

Create `web/src/pages/BomWorkbenchPage.tsx`:

```tsx
interface BomWorkbenchPageProps {
  onNavigateToHsResolve?: (model: string, manufacturer: string) => void
}

export function BomWorkbenchPage({ onNavigateToHsResolve: _onNavigateToHsResolve }: BomWorkbenchPageProps) {
  return (
    <div className="space-y-6" data-testid="bom-workbench-page">
      <div>
        <h2 className="text-2xl font-bold text-slate-800">BOM工作台</h2>
        <p className="mt-1 text-sm text-slate-600">集中处理会话、搜索与清洗、缺口和匹配单。</p>
      </div>
    </div>
  )
}
```

- [ ] **Step 4: 修改 `App.tsx` 的页面枚举和导航**

关键改动：

```tsx
import { BomWorkbenchPage } from './pages/BomWorkbenchPage'

type Page = 'guide' | 'bom-workbench' | 'agent-scripts' | 'agent-admin' | 'hs-resolve' | 'hs-meta'

const PAGE_LABELS: Record<Page, string> = {
  guide: '\u4f7f\u7528\u6307\u5357',
  'bom-workbench': 'BOM\u5de5\u4f5c\u53f0',
  'agent-scripts': '\u811a\u672c\u5305',
  'agent-admin': 'Agent\u8fd0\u7ef4',
  'hs-resolve': 'HS\u578b\u53f7\u89e3\u6790',
  'hs-meta': 'HS\u5143\u6570\u636e',
}

const ALLOWED_PAGES: Record<RoleKey, Page[]> = {
  anonymous: ['bom-workbench', 'guide'],
  user: ['bom-workbench', 'hs-resolve', 'guide'],
  admin: ['bom-workbench', 'agent-scripts', 'agent-admin', 'hs-resolve', 'hs-meta', 'guide'],
}
```

删除 `openMatchPage` 和 `matchNavHint` 的一级匹配单跳转逻辑；工作台内部负责匹配单 tab 状态。

渲染区改为：

```tsx
{page === 'bom-workbench' && (
  <BomWorkbenchPage
    onNavigateToHsResolve={(model, manufacturer) => {
      hsPrefillKeySeq.current += 1
      setHsPrefill({ key: hsPrefillKeySeq.current, model, manufacturer })
      setPage('hs-resolve')
    }}
  />
)}
```

- [ ] **Step 5: 运行 App 测试确认通过**

Run:

```powershell
cd web
& 'D:\Program Files\nodejs\npm.cmd' test -- src/App.test.tsx
```

Expected: PASS。

- [ ] **Step 6: 提交**

```powershell
git add -- web/src/App.tsx web/src/App.test.tsx web/src/pages/BomWorkbenchPage.tsx
git commit -m "feat(web): add bom workbench entry"
```

---

### Task 2: 会话列表改为工作台左栏

**Files:**
- Modify: `web/src/pages/BomWorkbenchPage.tsx`
- Create: `web/src/pages/bom-workbench/SessionListPanel.tsx`
- Test: `web/src/pages/BomWorkbenchPage.test.tsx`

- [ ] **Step 1: 写失败测试，验证会话列表点击后选中右侧工作区**

Create `web/src/pages/BomWorkbenchPage.test.tsx`:

```tsx
import { act, fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { BomWorkbenchPage } from './BomWorkbenchPage'

const { listSessions } = vi.hoisted(() => ({
  listSessions: vi.fn(),
}))

vi.mock('../api', async () => {
  const actual = await vi.importActual<Record<string, unknown>>('../api')
  return {
    ...actual,
    listSessions,
  }
})

async function flushAsyncWork() {
  await Promise.resolve()
  await Promise.resolve()
}

describe('BomWorkbenchPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    listSessions.mockResolvedValue({
      total: 1,
      items: [
        {
          session_id: 'session-1',
          title: '沙特阿拉伯大单',
          customer_name: '',
          status: 'searching',
          biz_date: '2026-04-21',
          updated_at: '2026-04-21T13:55:00+08:00',
          line_count: 48,
        },
      ],
    })
  })

  it('selects a session from the left list and opens the workspace on the right', async () => {
    render(<BomWorkbenchPage />)

    await act(async () => {
      await flushAsyncWork()
    })

    fireEvent.click(screen.getByRole('button', { name: /沙特阿拉伯大单/ }))

    expect(localStorage.getItem('bom_last_session_id')).toBe('session-1')
    expect(localStorage.getItem('bom_last_bom_id')).toBe('session-1')
    expect(screen.getByTestId('session-workspace-placeholder')).toHaveTextContent('session-1')
  })
})
```

- [ ] **Step 2: 运行测试确认失败**

Run:

```powershell
cd web
& 'D:\Program Files\nodejs\npm.cmd' test -- src/pages/BomWorkbenchPage.test.tsx
```

Expected: FAIL，原因是 `SessionListPanel` 和右侧 workspace placeholder 尚不存在。

- [ ] **Step 3: 创建 `SessionListPanel`**

Create `web/src/pages/bom-workbench/SessionListPanel.tsx`:

```tsx
import { useCallback, useEffect, useState } from 'react'
import { listSessions, type SessionListItem } from '../../api'

interface SessionListPanelProps {
  selectedSessionId: string | null
  onSelectSession: (sessionId: string) => void
  onCreateSession: () => void
}

export function SessionListPanel({
  selectedSessionId,
  onSelectSession,
  onCreateSession,
}: SessionListPanelProps) {
  const [listPage, setListPage] = useState(1)
  const [pageSize] = useState(20)
  const [status, setStatus] = useState('')
  const [bizDate, setBizDate] = useState('')
  const [q, setQ] = useState('')
  const [items, setItems] = useState<SessionListItem[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [err, setErr] = useState<string | null>(null)

  const load = useCallback(async () => {
    setLoading(true)
    setErr(null)
    try {
      const res = await listSessions({
        page: listPage,
        page_size: pageSize,
        status: status.trim() || undefined,
        biz_date: bizDate.trim() || undefined,
        q: q.trim() || undefined,
      })
      setItems(res.items)
      setTotal(res.total)
    } catch (e) {
      setErr(e instanceof Error ? e.message : '加载失败')
      setItems([])
      setTotal(0)
    } finally {
      setLoading(false)
    }
  }, [bizDate, listPage, pageSize, q, status])

  useEffect(() => {
    void load()
  }, [load])

  const totalPages = Math.max(1, Math.ceil(total / pageSize) || 1)

  return (
    <aside className="space-y-4 border-slate-200 bg-white p-4 lg:border-r" data-testid="session-list-panel">
      <div className="flex items-start justify-between gap-3">
        <div>
          <h2 className="text-lg font-semibold text-slate-800">BOM会话</h2>
          <p className="mt-1 text-xs text-slate-500">选择会话后在右侧处理。</p>
        </div>
        <button type="button" onClick={onCreateSession} className="rounded-lg bg-slate-800 px-3 py-2 text-sm font-medium text-white">
          新建BOM
        </button>
      </div>

      <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-1">
        <input value={status} onChange={(e) => { setStatus(e.target.value); setListPage(1) }} placeholder="状态" className="rounded border border-slate-300 px-2 py-1.5 text-sm" />
        <input type="date" value={bizDate} onChange={(e) => { setBizDate(e.target.value); setListPage(1) }} className="rounded border border-slate-300 px-2 py-1.5 text-sm" />
        <input value={q} onChange={(e) => { setQ(e.target.value); setListPage(1) }} placeholder="标题 / 客户 / 型号" className="rounded border border-slate-300 px-2 py-1.5 text-sm sm:col-span-2 lg:col-span-1" />
      </div>

      {err && <div className="rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-800">{err}</div>}
      {loading ? <p className="text-sm text-slate-500">加载中...</p> : null}

      <div className="space-y-2">
        {items.map((row) => (
          <button
            key={row.session_id}
            type="button"
            onClick={() => onSelectSession(row.session_id)}
            className={`w-full rounded-lg border px-3 py-2 text-left text-sm ${
              selectedSessionId === row.session_id ? 'border-blue-300 bg-blue-50' : 'border-slate-200 bg-white hover:bg-slate-50'
            }`}
          >
            <span className="block font-medium text-slate-800">{row.title || row.session_id}</span>
            <span className="mt-1 block text-xs text-slate-500">{row.status} · {row.biz_date} · {row.line_count}行</span>
          </button>
        ))}
      </div>

      <div className="flex items-center justify-between text-xs text-slate-500">
        <span>共 {total} 条 · {listPage}/{totalPages}</span>
        <div className="flex gap-2">
          <button type="button" disabled={listPage <= 1} onClick={() => setListPage((p) => Math.max(1, p - 1))} className="rounded border border-slate-300 px-2 py-1 disabled:opacity-40">上一页</button>
          <button type="button" disabled={listPage >= totalPages} onClick={() => setListPage((p) => p + 1)} className="rounded border border-slate-300 px-2 py-1 disabled:opacity-40">下一页</button>
        </div>
      </div>
    </aside>
  )
}
```

- [ ] **Step 4: 接入 `BomWorkbenchPage`**

Replace `web/src/pages/BomWorkbenchPage.tsx` with:

```tsx
import { useState } from 'react'
import { UploadPage } from './UploadPage'
import { SessionListPanel } from './bom-workbench/SessionListPanel'

const LAST_BOM_KEY = 'bom_last_bom_id'
const LAST_SESSION_KEY = 'bom_last_session_id'

interface BomWorkbenchPageProps {
  onNavigateToHsResolve?: (model: string, manufacturer: string) => void
}

export function BomWorkbenchPage({ onNavigateToHsResolve: _onNavigateToHsResolve }: BomWorkbenchPageProps) {
  const [selectedSessionId, setSelectedSessionId] = useState<string | null>(() => localStorage.getItem(LAST_SESSION_KEY))
  const [uploadOpen, setUploadOpen] = useState(false)

  const selectSession = (sessionId: string) => {
    localStorage.setItem(LAST_SESSION_KEY, sessionId)
    localStorage.setItem(LAST_BOM_KEY, sessionId)
    setSelectedSessionId(sessionId)
  }

  const onSessionUploadSuccess = (sessionId: string) => {
    setUploadOpen(false)
    selectSession(sessionId)
  }

  return (
    <div className="space-y-4" data-testid="bom-workbench-page">
      <div>
        <h2 className="text-2xl font-bold text-slate-800">BOM工作台</h2>
        <p className="mt-1 text-sm text-slate-600">集中处理会话、搜索与清洗、缺口和匹配单。</p>
      </div>

      <div className="overflow-hidden rounded-xl border border-slate-200 bg-slate-50 shadow-sm lg:grid lg:grid-cols-[22rem_minmax(0,1fr)]">
        <SessionListPanel
          selectedSessionId={selectedSessionId}
          onSelectSession={selectSession}
          onCreateSession={() => setUploadOpen(true)}
        />
        <section className="min-h-[32rem] p-4">
          {selectedSessionId ? (
            <div data-testid="session-workspace-placeholder">当前会话：{selectedSessionId}</div>
          ) : (
            <div className="rounded-lg border border-dashed border-slate-300 bg-white px-4 py-12 text-center text-sm text-slate-500">
              请选择或新建BOM会话
            </div>
          )}
        </section>
      </div>

      {uploadOpen && (
        <div className="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-black/40 p-4 pt-10" role="dialog" aria-modal="true">
          <div className="w-full max-w-4xl rounded-xl border border-slate-200 bg-white p-4 shadow-xl">
            <div className="mb-4 flex items-center justify-between gap-4">
              <h3 className="text-lg font-semibold text-slate-800">新建BOM</h3>
              <button type="button" onClick={() => setUploadOpen(false)} className="text-sm text-slate-500 hover:text-slate-800">关闭</button>
            </div>
            <UploadPage embedded onSuccess={onSessionUploadSuccess} />
          </div>
        </div>
      )}
    </div>
  )
}
```

- [ ] **Step 5: 运行工作台测试确认通过**

Run:

```powershell
cd web
& 'D:\Program Files\nodejs\npm.cmd' test -- src/pages/BomWorkbenchPage.test.tsx
```

Expected: PASS。

- [ ] **Step 6: 提交**

```powershell
git add web/src/pages/BomWorkbenchPage.tsx web/src/pages/BomWorkbenchPage.test.tsx web/src/pages/bom-workbench/SessionListPanel.tsx
git commit -m "feat(web): add bom workbench session list"
```

---

### Task 3: 当前会话工作区与推荐 tab

**Files:**
- Create: `web/src/pages/bom-workbench/sessionTabs.ts`
- Create: `web/src/pages/bom-workbench/SessionWorkspace.tsx`
- Modify: `web/src/pages/BomWorkbenchPage.tsx`
- Test: `web/src/pages/BomWorkbenchPage.test.tsx`

- [ ] **Step 1: 写推荐 tab 纯函数测试**

Append to `web/src/pages/BomWorkbenchPage.test.tsx`:

```tsx
import { recommendSessionTab } from './bom-workbench/sessionTabs'

it('recommends search-clean when search tasks need attention', () => {
  expect(
    recommendSessionTab({
      searchAttentionCount: 2,
      aliasReviewCount: 0,
      gapCount: 0,
      blockingLineCount: 0,
      preferMatch: false,
      canEnterMatch: false,
    })
  ).toBe('search-clean')
})

it('recommends gaps before lines when search and clean are clear', () => {
  expect(
    recommendSessionTab({
      searchAttentionCount: 0,
      aliasReviewCount: 0,
      gapCount: 3,
      blockingLineCount: 8,
      preferMatch: false,
      canEnterMatch: false,
    })
  ).toBe('gaps')
})

it('recommends match only when requested and ready', () => {
  expect(
    recommendSessionTab({
      searchAttentionCount: 0,
      aliasReviewCount: 0,
      gapCount: 0,
      blockingLineCount: 0,
      preferMatch: true,
      canEnterMatch: true,
    })
  ).toBe('match')
})
```

- [ ] **Step 2: 运行测试确认失败**

Run:

```powershell
cd web
& 'D:\Program Files\nodejs\npm.cmd' test -- src/pages/BomWorkbenchPage.test.tsx
```

Expected: FAIL，原因是 `sessionTabs.ts` 不存在。

- [ ] **Step 3: 创建 `sessionTabs.ts`**

```ts
export type SessionWorkbenchTab = 'overview' | 'lines' | 'search-clean' | 'gaps' | 'match' | 'maintenance'

export interface RecommendSessionTabInput {
  searchAttentionCount: number
  aliasReviewCount: number
  gapCount: number
  blockingLineCount: number
  preferMatch: boolean
  canEnterMatch: boolean
}

export function recommendSessionTab(input: RecommendSessionTabInput): SessionWorkbenchTab {
  if (input.searchAttentionCount > 0 || input.aliasReviewCount > 0) return 'search-clean'
  if (input.gapCount > 0) return 'gaps'
  if (input.blockingLineCount > 0) return 'lines'
  if (input.preferMatch && input.canEnterMatch) return 'match'
  return 'overview'
}

export const SESSION_WORKBENCH_TABS: { id: SessionWorkbenchTab; label: string }[] = [
  { id: 'overview', label: '总览' },
  { id: 'lines', label: 'BOM行' },
  { id: 'search-clean', label: '搜索与清洗' },
  { id: 'gaps', label: '缺口方案' },
  { id: 'match', label: '匹配单' },
  { id: 'maintenance', label: '维护信息' },
]
```

- [ ] **Step 4: 创建 `SessionWorkspace` 骨架**

Create `web/src/pages/bom-workbench/SessionWorkspace.tsx`:

```tsx
import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  getBOMLines,
  getSession,
  listLineGaps,
  listMatchRuns,
  listSessionSearchTasks,
  type BOMLineGap,
  type BOMLineRow,
  type GetSessionReply,
  type ListSessionSearchTasksReply,
  type MatchRunListItem,
} from '../../api'
import { recommendSessionTab, SESSION_WORKBENCH_TABS, type SessionWorkbenchTab } from './sessionTabs'

const SESSION_MATCH_READY = 'data_ready'
const BLOCKING_AVAILABILITY_STATUSES = new Set(['no_data', 'collection_unavailable', 'no_match_after_filter'])
const SEARCH_ATTENTION_STATES = new Set(['failed', 'missing'])

interface SessionWorkspaceProps {
  sessionId: string
  preferMatch?: boolean
  onNavigateToHsResolve?: (model: string, manufacturer: string) => void
  onBackToList?: () => void
}

export function SessionWorkspace({
  sessionId,
  preferMatch = false,
  onNavigateToHsResolve: _onNavigateToHsResolve,
  onBackToList,
}: SessionWorkspaceProps) {
  const [session, setSession] = useState<GetSessionReply | null>(null)
  const [lines, setLines] = useState<BOMLineRow[]>([])
  const [searchTasks, setSearchTasks] = useState<ListSessionSearchTasksReply | null>(null)
  const [gaps, setGaps] = useState<BOMLineGap[]>([])
  const [runs, setRuns] = useState<MatchRunListItem[]>([])
  const [activeTab, setActiveTab] = useState<SessionWorkbenchTab>('overview')
  const [tabTouched, setTabTouched] = useState(false)
  const [loading, setLoading] = useState(true)
  const [err, setErr] = useState<string | null>(null)

  const load = useCallback(async () => {
    setLoading(true)
    setErr(null)
    try {
      const [sessionData, lineData, searchData, gapData, runData] = await Promise.all([
        getSession(sessionId),
        getBOMLines(sessionId),
        listSessionSearchTasks(sessionId),
        listLineGaps(sessionId, ['open', 'manual_quote_added', 'substitute_selected']),
        listMatchRuns(sessionId),
      ])
      setSession(sessionData)
      setLines(lineData.lines)
      setSearchTasks(searchData)
      setGaps(gapData.gaps)
      setRuns(runData.runs)
    } catch (e) {
      setErr(e instanceof Error ? e.message : '加载会话失败')
    } finally {
      setLoading(false)
    }
  }, [sessionId])

  useEffect(() => {
    setTabTouched(false)
    void load()
  }, [load])

  const blockingLineCount = lines.filter((line) =>
    BLOCKING_AVAILABILITY_STATUSES.has((line.availability_status || '').trim())
  ).length
  const searchAttentionCount = (searchTasks?.tasks ?? []).filter(
    (task) => task.retryable || SEARCH_ATTENTION_STATES.has(task.search_ui_state)
  ).length
  const canEnterMatch = (session?.status || '').trim() === SESSION_MATCH_READY

  const recommendedTab = useMemo(
    () =>
      recommendSessionTab({
        searchAttentionCount,
        aliasReviewCount: 0,
        gapCount: gaps.length,
        blockingLineCount,
        preferMatch,
        canEnterMatch,
      }),
    [blockingLineCount, canEnterMatch, gaps.length, preferMatch, searchAttentionCount]
  )

  const currentTab = tabTouched ? activeTab : recommendedTab

  if (loading) return <div className="p-8 text-center text-sm text-slate-500">加载会话...</div>
  if (err) return <div className="rounded-lg border border-red-200 bg-red-50 p-4 text-sm text-red-800">{err}</div>

  return (
    <div className="space-y-4" data-testid="session-workspace">
      {onBackToList && (
        <button type="button" onClick={onBackToList} className="rounded border border-slate-300 px-3 py-1.5 text-sm lg:hidden">
          返回会话列表
        </button>
      )}
      <div className="rounded-xl border border-slate-200 bg-white p-4 shadow-sm">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h3 className="text-xl font-semibold text-slate-800">{session?.title || sessionId}</h3>
            <p className="mt-1 text-xs text-slate-500">
              <span className="font-mono">{sessionId}</span> · 状态 {session?.status || '未知'} · 已保存方案 {runs.length}
            </p>
          </div>
          <button type="button" onClick={() => void load()} className="rounded-lg border border-slate-300 bg-white px-3 py-1.5 text-sm hover:bg-slate-50">
            刷新
          </button>
        </div>
      </div>

      <div role="tablist" aria-label="BOM工作台分区" className="flex flex-wrap gap-2">
        {SESSION_WORKBENCH_TABS.map((tab) => {
          const disabled = tab.id === 'match' && !canEnterMatch
          return (
            <button
              key={tab.id}
              type="button"
              role="tab"
              aria-selected={currentTab === tab.id}
              disabled={disabled}
              onClick={() => {
                if (disabled) return
                setActiveTab(tab.id)
                setTabTouched(true)
              }}
              className={
                currentTab === tab.id
                  ? 'rounded-lg bg-slate-900 px-3 py-2 text-sm font-medium text-white'
                  : 'rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm font-medium text-slate-600 disabled:bg-slate-100 disabled:text-slate-400'
              }
            >
              {tab.label}
            </button>
          )
        })}
      </div>

      <div data-testid={`session-tab-panel-${currentTab}`} className="rounded-xl border border-slate-200 bg-white p-4 shadow-sm">
        {currentTab === 'overview' && <div>总览：{lines.length} 行，搜索关注 {searchAttentionCount}，缺口 {gaps.length}</div>}
        {currentTab === 'lines' && <div>BOM行：{blockingLineCount} 行需要关注</div>}
        {currentTab === 'search-clean' && <div>搜索与清洗：{searchAttentionCount} 个任务需要关注</div>}
        {currentTab === 'gaps' && <div>缺口方案：{gaps.length} 个缺口</div>}
        {currentTab === 'match' && <div>匹配单</div>}
        {currentTab === 'maintenance' && <div>维护信息</div>}
      </div>
    </div>
  )
}
```

- [ ] **Step 5: 将 `BomWorkbenchPage` placeholder 替换为 `SessionWorkspace`**

```tsx
import { SessionWorkspace } from './bom-workbench/SessionWorkspace'

// inside render
{selectedSessionId ? (
  <SessionWorkspace sessionId={selectedSessionId} onNavigateToHsResolve={onNavigateToHsResolve} />
) : (
  <div className="rounded-lg border border-dashed border-slate-300 bg-white px-4 py-12 text-center text-sm text-slate-500">
    请选择或新建BOM会话
  </div>
)}
```

- [ ] **Step 6: 运行测试确认通过**

Run:

```powershell
cd web
& 'D:\Program Files\nodejs\npm.cmd' test -- src/pages/BomWorkbenchPage.test.tsx
```

Expected: PASS。若原 placeholder 测试失败，将断言改成：

```tsx
expect(screen.getByTestId('session-workspace')).toBeInTheDocument()
```

- [ ] **Step 7: 提交**

```powershell
git add web/src/pages/BomWorkbenchPage.tsx web/src/pages/BomWorkbenchPage.test.tsx web/src/pages/bom-workbench/sessionTabs.ts web/src/pages/bom-workbench/SessionWorkspace.tsx
git commit -m "feat(web): add session workspace tabs"
```

---

### Task 4: 抽出总览、BOM 行、缺口、维护面板

**Files:**
- Create: `web/src/pages/bom-workbench/SessionOverviewPanel.tsx`
- Create: `web/src/pages/bom-workbench/SessionLinesPanel.tsx`
- Create: `web/src/pages/bom-workbench/SessionGapsPanel.tsx`
- Create: `web/src/pages/bom-workbench/SessionMaintenancePanel.tsx`
- Modify: `web/src/pages/bom-workbench/SessionWorkspace.tsx`
- Modify: `web/src/pages/SourcingSessionPage.tsx`
- Test: `web/src/pages/SourcingSessionPage.test.tsx`

- [ ] **Step 1: 写测试，保护 BOM 行异常优先排序**

在 `web/src/pages/SourcingSessionPage.test.tsx` 保留或新增断言：

```tsx
it('prioritizes BOM lines with blocking availability statuses', async () => {
  getSession.mockResolvedValue({
    ...baseSession,
    status: 'searching',
    import_status: 'ready',
    import_progress: 100,
  })
  getBOMLines.mockResolvedValue({
    lines: [
      {
        line_id: 'ready-line',
        line_no: 1,
        mpn: 'READY',
        mfr: '',
        package: '',
        qty: 1,
        match_status: '',
        platform_gaps: [],
        availability_status: 'ready',
        has_usable_quote: true,
        raw_quote_platform_count: 1,
        usable_quote_platform_count: 1,
      },
      {
        line_id: 'blocked-line',
        line_no: 2,
        mpn: 'BLOCKED',
        mfr: '',
        package: '',
        qty: 1,
        match_status: '',
        platform_gaps: [],
        availability_status: 'no_match_after_filter',
        availability_reason: 'raw quote exists but no usable quote remains',
        has_usable_quote: false,
        raw_quote_platform_count: 4,
        usable_quote_platform_count: 0,
      },
    ],
  })

  render(<SourcingSessionPage sessionId="session-1" onEnterMatch={vi.fn()} />)

  await act(async () => {
    await flushAsyncWork()
  })

  const rows = screen.getAllByRole('row')
  expect(rows.findIndex((row) => row.textContent?.includes('BLOCKED'))).toBeLessThan(
    rows.findIndex((row) => row.textContent?.includes('READY'))
  )
})
```

- [ ] **Step 2: 运行测试确认当前行为仍受保护**

Run:

```powershell
cd web
& 'D:\Program Files\nodejs\npm.cmd' test -- src/pages/SourcingSessionPage.test.tsx
```

Expected: PASS 或只因新增组件尚未接入而 FAIL；实现后必须 PASS。

- [ ] **Step 3: 创建面板组件**

从 `SourcingSessionPage.tsx` 迁移 JSX 时保持 props 简单。示例接口：

```tsx
// SessionOverviewPanel.tsx
import type { MatchRunListItem } from '../../api'

interface SessionOverviewPanelProps {
  lineCount: number
  blockingLineCount: number
  searchAttentionCount: number
  gapCount: number
  matchRuns: MatchRunListItem[]
}

export function SessionOverviewPanel(props: SessionOverviewPanelProps) {
  return (
    <section data-testid="session-overview-panel" className="rounded-xl border border-slate-200 bg-white p-6 shadow-sm">
      <h3 className="font-semibold text-slate-800">处理总览</h3>
      <div className="mt-4 grid gap-3 md:grid-cols-4">
        {[
          ['BOM行', props.lineCount],
          ['异常行', props.blockingLineCount],
          ['搜索与清洗', props.searchAttentionCount],
          ['待处理缺口', props.gapCount],
        ].map(([label, value]) => (
          <div key={label} className="rounded-lg border border-slate-200 px-4 py-3">
            <div className="text-xs text-slate-500">{label}</div>
            <div className="mt-1 text-2xl font-semibold text-slate-800">{value}</div>
          </div>
        ))}
      </div>
    </section>
  )
}
```

`SessionLinesPanel`、`SessionGapsPanel`、`SessionMaintenancePanel` 使用从 `SourcingSessionPage.tsx` 搬出的现有 JSX 和 handler props；不要改变 API 行为。

- [ ] **Step 4: `SessionWorkspace` 使用新面板替换占位内容**

示例：

```tsx
{currentTab === 'overview' && (
  <SessionOverviewPanel
    lineCount={lines.length}
    blockingLineCount={blockingLineCount}
    searchAttentionCount={searchAttentionCount}
    gapCount={gaps.length}
    matchRuns={runs}
  />
)}
```

- [ ] **Step 5: 保留 `SourcingSessionPage` 兼容导出**

将 `SourcingSessionPage.tsx` 收敛为薄包装：

```tsx
import { SessionWorkspace } from './bom-workbench/SessionWorkspace'

interface SourcingSessionPageProps {
  sessionId: string
  embedded?: boolean
  onEnterMatch?: () => void
}

export function SourcingSessionPage({ sessionId, embedded, onEnterMatch }: SourcingSessionPageProps) {
  return (
    <div className={embedded ? 'space-y-6' : 'space-y-8'}>
      <SessionWorkspace sessionId={sessionId} preferMatch={false} onRequestMatch={onEnterMatch} />
    </div>
  )
}
```

如果 `onRequestMatch` 不在 `SessionWorkspace` 中，新增 prop：

```ts
onRequestMatch?: () => void
```

用于兼容旧调用方。

- [ ] **Step 6: 运行会话页测试**

Run:

```powershell
cd web
& 'D:\Program Files\nodejs\npm.cmd' test -- src/pages/SourcingSessionPage.test.tsx
```

Expected: PASS。

- [ ] **Step 7: 提交**

```powershell
git add web/src/pages/SourcingSessionPage.tsx web/src/pages/SourcingSessionPage.test.tsx web/src/pages/bom-workbench
git commit -m "refactor(web): split session workspace panels"
```

---

### Task 5: 搜索与清洗 tab 加入厂牌别名审核

**Files:**
- Create: `web/src/pages/bom-workbench/SessionSearchCleanPanel.tsx`
- Create: `web/src/pages/bom-workbench/ManufacturerAliasReviewPanel.tsx`
- Modify: `web/src/pages/MatchResultPage.tsx`
- Modify: `web/src/pages/bom-workbench/SessionWorkspace.tsx`
- Test: `web/src/pages/BomWorkbenchPage.test.tsx`

- [ ] **Step 1: 写失败测试，验证搜索与清洗中出现厂牌别名审核**

Append to `web/src/pages/BomWorkbenchPage.test.tsx`:

```tsx
it('shows manufacturer alias review inside search and clean tab', async () => {
  render(<BomWorkbenchPage />)

  await act(async () => {
    await flushAsyncWork()
  })

  fireEvent.click(screen.getByRole('button', { name: /沙特阿拉伯大单/ }))

  expect(await screen.findByRole('tab', { name: /搜索与清洗/ })).toBeInTheDocument()
  expect(screen.getByText('厂牌别名审核')).toBeInTheDocument()
})
```

- [ ] **Step 2: 运行测试确认失败**

Run:

```powershell
cd web
& 'D:\Program Files\nodejs\npm.cmd' test -- src/pages/BomWorkbenchPage.test.tsx
```

Expected: FAIL，原因是 `SessionSearchCleanPanel` 尚未渲染审核区。

- [ ] **Step 3: 抽出 `ManufacturerAliasReviewPanel`**

从 `MatchResultPage.tsx` 迁移：

- `type PendingMfrRow`
- `MFR_PLACEHOLDER`
- `ManualManufacturerAliasForm`
- `MfrAliasReviewPanel`

新文件导出：

```tsx
export type PendingMfrRow = { alias: string; lineIndexes: number[]; demandHint: string }

export interface ManufacturerAliasReviewPanelProps {
  pendingRows: PendingMfrRow[]
  canonicalRows: ManufacturerCanonicalRow[]
  onApproved: (alias: string) => void
  onManualSuccess: () => void
}

export function ManufacturerAliasReviewPanel({
  pendingRows,
  canonicalRows,
  onApproved,
  onManualSuccess,
}: ManufacturerAliasReviewPanelProps) {
  return (
    <section className="rounded-xl border border-slate-200 bg-white shadow-sm">
      <div className="border-b border-slate-100 p-4">
        <h3 className="font-semibold text-slate-800">厂牌别名审核</h3>
        <p className="mt-1 text-sm text-slate-600">搜索结果进入匹配前，在这里清洗报价厂牌别名。</p>
      </div>
      {/* 迁移现有 MfrAliasReviewPanel 和 ManualManufacturerAliasForm 内容 */}
    </section>
  )
}
```

迁移后 `MatchResultPage.tsx` 从新文件导入，保证原匹配单页面行为不丢。

- [ ] **Step 4: 创建 `SessionSearchCleanPanel`**

```tsx
import { useEffect, useMemo, useState } from 'react'
import {
  autoMatch,
  listManufacturerCanonicals,
  type ListSessionSearchTasksReply,
  type ManufacturerCanonicalRow,
  type MatchItem,
  type SessionSearchTaskRow,
} from '../../api'
import { SearchTaskStatusPanel } from '../sourcing-session/SearchTaskStatusPanel'
import { ManufacturerAliasReviewPanel, type PendingMfrRow } from './ManufacturerAliasReviewPanel'

function collectPendingMfrRows(items: MatchItem[]): PendingMfrRow[] {
  const map = new Map<string, { lines: Set<number>; demand: Set<string> }>()
  for (const it of items) {
    const arr = it.mfr_mismatch_quote_manufacturers
    if (!arr?.length) continue
    const dm = (it.demand_manufacturer || '').trim()
    for (const raw of arr) {
      const alias = (raw || '').trim()
      if (!alias) continue
      let g = map.get(alias)
      if (!g) {
        g = { lines: new Set(), demand: new Set() }
        map.set(alias, g)
      }
      g.lines.add(it.index)
      if (dm) g.demand.add(dm)
    }
  }
  return Array.from(map.entries()).map(([alias, g]) => ({
    alias,
    lineIndexes: [...g.lines].sort((a, b) => a - b),
    demandHint: [...g.demand].join('，') || '—',
  }))
}

interface SessionSearchCleanPanelProps {
  sessionId: string
  data: ListSessionSearchTasksReply | null
  loading: boolean
  retrying: boolean
  onRefresh: () => void
  onRetryBatch: () => void
  onRetryTask: (task: SessionSearchTaskRow) => void
}

export function SessionSearchCleanPanel(props: SessionSearchCleanPanelProps) {
  const [items, setItems] = useState<MatchItem[]>([])
  const [canonicalRows, setCanonicalRows] = useState<ManufacturerCanonicalRow[]>([])
  const [aliasErr, setAliasErr] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    ;(async () => {
      setAliasErr(null)
      try {
        const [match, canonicals] = await Promise.all([
          autoMatch(props.sessionId),
          listManufacturerCanonicals(),
        ])
        if (!cancelled) {
          setItems(match.items)
          setCanonicalRows(canonicals.items)
        }
      } catch (e) {
        if (!cancelled) {
          setItems([])
          setAliasErr(e instanceof Error ? e.message : '厂牌别名候选加载失败')
        }
      }
    })()
    return () => {
      cancelled = true
    }
  }, [props.sessionId])

  const pendingRows = useMemo(() => collectPendingMfrRows(items), [items])

  return (
    <div className="space-y-4">
      <SearchTaskStatusPanel
        data={props.data}
        loading={props.loading}
        retrying={props.retrying}
        defaultOpen
        onRefresh={props.onRefresh}
        onRetryBatch={props.onRetryBatch}
        onRetryTask={props.onRetryTask}
      />
      {aliasErr && (
        <div className="rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-900">
          {aliasErr}
        </div>
      )}
      <ManufacturerAliasReviewPanel
        pendingRows={pendingRows}
        canonicalRows={canonicalRows}
        onApproved={() => props.onRefresh()}
        onManualSuccess={() => props.onRefresh()}
      />
    </div>
  )
}
```

- [ ] **Step 5: `SessionWorkspace` 使用 `SessionSearchCleanPanel`**

在 `search-clean` tab 中替换占位内容，并复用现有 retry handler：

```tsx
{currentTab === 'search-clean' && (
  <SessionSearchCleanPanel
    sessionId={sessionId}
    data={searchTasks}
    loading={searchTasksLoading}
    retrying={retryingSearchTasks}
    onRefresh={() => void loadSearchTasks()}
    onRetryBatch={() => void handleRetrySearchTaskBatch()}
    onRetryTask={(task) => void handleRetrySearchTask(task)}
  />
)}
```

如果 `SessionWorkspace` 还没有 `searchTasksLoading`、`retryingSearchTasks`、retry handler，将其从 `SourcingSessionPage.tsx` 迁入。

- [ ] **Step 6: 运行测试**

Run:

```powershell
cd web
& 'D:\Program Files\nodejs\npm.cmd' test -- src/pages/BomWorkbenchPage.test.tsx src/pages/SourcingSessionPage.test.tsx
```

Expected: PASS。

- [ ] **Step 7: 提交**

```powershell
git add web/src/pages/MatchResultPage.tsx web/src/pages/BomWorkbenchPage.test.tsx web/src/pages/bom-workbench/SessionWorkspace.tsx web/src/pages/bom-workbench/SessionSearchCleanPanel.tsx web/src/pages/bom-workbench/ManufacturerAliasReviewPanel.tsx
git commit -m "feat(web): move manufacturer alias review into search clean"
```

---

### Task 6: 匹配单嵌入工作台

**Files:**
- Create: `web/src/pages/bom-workbench/MatchResultWorkspace.tsx`
- Modify: `web/src/pages/MatchResultPage.tsx`
- Modify: `web/src/pages/bom-workbench/SessionWorkspace.tsx`
- Test: `web/src/pages/BomWorkbenchPage.test.tsx`

- [ ] **Step 1: 写失败测试，验证未就绪时匹配单 tab 禁用并提示原因**

Append:

```tsx
it('disables match tab until the selected session is data_ready', async () => {
  render(<BomWorkbenchPage />)

  await act(async () => {
    await flushAsyncWork()
  })

  fireEvent.click(screen.getByRole('button', { name: /沙特阿拉伯大单/ }))

  const matchTab = await screen.findByRole('tab', { name: '匹配单' })
  expect(matchTab).toBeDisabled()
  expect(screen.getByText(/匹配单未就绪/)).toBeInTheDocument()
})
```

- [ ] **Step 2: 运行测试确认失败**

Run:

```powershell
cd web
& 'D:\Program Files\nodejs\npm.cmd' test -- src/pages/BomWorkbenchPage.test.tsx
```

Expected: FAIL，原因是未就绪提示尚未渲染。

- [ ] **Step 3: 抽出 `MatchResultWorkspace`**

从 `MatchResultPage.tsx` 中抽出主体逻辑：

```tsx
interface MatchResultWorkspaceProps {
  bomId: string
  onNavigateToHsResolve?: (model: string, manufacturer: string) => void
}

export function MatchResultWorkspace({ bomId, onNavigateToHsResolve }: MatchResultWorkspaceProps) {
  // 迁移原 MatchResultPage 的 state、load、filter、table、modal 逻辑
  return (
    <div data-testid="match-result-workspace">
      {/* 原匹配单主体内容 */}
    </div>
  )
}
```

`MatchResultPage.tsx` 收敛为：

```tsx
import { MatchResultWorkspace } from './bom-workbench/MatchResultWorkspace'

interface MatchResultPageProps {
  bomId: string
  onNavigateToHsResolve?: (model: string, manufacturer: string) => void
}

export function MatchResultPage(props: MatchResultPageProps) {
  return <MatchResultWorkspace {...props} />
}
```

- [ ] **Step 4: `SessionWorkspace` 接入匹配单 tab**

```tsx
{currentTab === 'match' && canEnterMatch && (
  <MatchResultWorkspace bomId={sessionId} onNavigateToHsResolve={onNavigateToHsResolve} />
)}
{!canEnterMatch && (
  <div className="rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-950">
    匹配单未就绪：当前会话状态为 {session?.status || '未知'}，请先完成搜索与清洗、缺口处理或等待会话进入 data_ready。
  </div>
)}
```

- [ ] **Step 5: 运行测试**

Run:

```powershell
cd web
& 'D:\Program Files\nodejs\npm.cmd' test -- src/pages/BomWorkbenchPage.test.tsx
```

Expected: PASS。

- [ ] **Step 6: 提交**

```powershell
git add web/src/pages/MatchResultPage.tsx web/src/pages/bom-workbench/MatchResultWorkspace.tsx web/src/pages/bom-workbench/SessionWorkspace.tsx web/src/pages/BomWorkbenchPage.test.tsx
git commit -m "feat(web): embed match result in bom workbench"
```

---

### Task 7: 移动端列表/详情切换

**Files:**
- Modify: `web/src/pages/BomWorkbenchPage.tsx`
- Modify: `web/src/pages/bom-workbench/SessionWorkspace.tsx`
- Test: `web/src/pages/BomWorkbenchPage.test.tsx`

- [ ] **Step 1: 写失败测试，验证详情里有返回会话列表按钮**

Append:

```tsx
it('offers a back-to-list action for mobile detail flow', async () => {
  render(<BomWorkbenchPage />)

  await act(async () => {
    await flushAsyncWork()
  })

  fireEvent.click(screen.getByRole('button', { name: /沙特阿拉伯大单/ }))

  expect(await screen.findByRole('button', { name: '返回会话列表' })).toBeInTheDocument()
})
```

- [ ] **Step 2: 运行测试确认失败**

Run:

```powershell
cd web
& 'D:\Program Files\nodejs\npm.cmd' test -- src/pages/BomWorkbenchPage.test.tsx
```

Expected: FAIL，如果 `onBackToList` 尚未传入。

- [ ] **Step 3: 增加移动端显示状态**

在 `BomWorkbenchPage.tsx`：

```tsx
const [mobileDetailOpen, setMobileDetailOpen] = useState(false)

const selectSession = (sessionId: string) => {
  localStorage.setItem(LAST_SESSION_KEY, sessionId)
  localStorage.setItem(LAST_BOM_KEY, sessionId)
  setSelectedSessionId(sessionId)
  setMobileDetailOpen(true)
}
```

布局 class：

```tsx
<div className="overflow-hidden rounded-xl border border-slate-200 bg-slate-50 shadow-sm lg:grid lg:grid-cols-[22rem_minmax(0,1fr)]">
  <div className={mobileDetailOpen ? 'hidden lg:block' : 'block'}>
    <SessionListPanel ... />
  </div>
  <section className={mobileDetailOpen ? 'block min-h-[32rem] p-4' : 'hidden min-h-[32rem] p-4 lg:block'}>
    <SessionWorkspace
      sessionId={selectedSessionId}
      onBackToList={() => setMobileDetailOpen(false)}
      ...
    />
  </section>
</div>
```

- [ ] **Step 4: 运行测试确认通过**

Run:

```powershell
cd web
& 'D:\Program Files\nodejs\npm.cmd' test -- src/pages/BomWorkbenchPage.test.tsx
```

Expected: PASS。

- [ ] **Step 5: 提交**

```powershell
git add web/src/pages/BomWorkbenchPage.tsx web/src/pages/BomWorkbenchPage.test.tsx web/src/pages/bom-workbench/SessionWorkspace.tsx
git commit -m "feat(web): add mobile bom workbench flow"
```

---

### Task 8: 全量回归、构建和收尾

**Files:**
- Modify as needed: `web/src/pages/BomWorkbenchPage.test.tsx`
- Modify as needed: `web/src/pages/SourcingSessionPage.test.tsx`
- Modify as needed: `web/src/App.test.tsx`

- [ ] **Step 1: 运行目标测试**

Run:

```powershell
cd web
& 'D:\Program Files\nodejs\npm.cmd' test -- src/App.test.tsx src/pages/BomWorkbenchPage.test.tsx src/pages/SourcingSessionPage.test.tsx
```

Expected: PASS。

- [ ] **Step 2: 运行匹配单相关测试**

如果已有 `MatchResultPage` 测试文件：

```powershell
cd web
& 'D:\Program Files\nodejs\npm.cmd' test -- src/pages/MatchResultPage.test.tsx
```

Expected: PASS。

如果不存在该测试文件，本步骤记录为“无现有匹配单专项测试”，不要临时创建空测试。

- [ ] **Step 3: 运行前端构建**

Run:

```powershell
cd web
& 'D:\Program Files\nodejs\npm.cmd' run build
```

Expected: exit 0，TypeScript 和 Vite build 均通过。

- [ ] **Step 4: 检查文件长度**

Run:

```powershell
Get-ChildItem web\src\pages -Recurse -File -Include *.tsx,*.ts |
  Where-Object { $_.FullName -notmatch '\\.test\\.' } |
  ForEach-Object {
    $count = (Get-Content $_.FullName).Count
    if ($count -gt 300) { "$count $($_.FullName)" }
  }
```

Expected: 新增或重构文件不超过 300 行。若 `MatchResultWorkspace.tsx` 因迁移暂超 300 行，在 PR/评审说明中列出后续拆分原因；不要把新的大块逻辑继续塞回旧文件。

- [ ] **Step 5: 最终提交**

如果只有测试或轻微修正：

```powershell
git add web/src
git commit -m "test(web): cover bom workbench layout"
```

- [ ] **Step 6: 手动验收路径**

在浏览器中验证：

1. 顶部一级导航显示 `BOM工作台`，不显示 `匹配单`。
2. 左侧选择 `d68dcedd-f3bc-4075-b349-7803f8340389`。
3. 右侧默认进入 `搜索与清洗`。
4. `搜索与清洗` 展示任务状态和厂牌别名审核。
5. 当前状态非 `data_ready` 时，`匹配单` tab 禁用并提示原因。
6. 移动宽度下先显示会话列表，进入详情后可返回列表。

---

## Self-Review

Spec coverage:

- 顶部一级导航合并：Task 1。
- 左侧会话列表 + 右侧工作区：Task 2、Task 3。
- 二级 tab 与推荐 tab：Task 3。
- `搜索与清洗` 与厂牌别名审核：Task 5。
- 匹配单嵌入和未就绪提示：Task 6。
- 移动端列表/详情切换：Task 7。
- 测试和构建：Task 8。

No-placeholder scan:

- 本计划不使用占位词或模糊占位作为任务内容。
- 若实施中发现现有 `MatchResultPage` 迁移量过大，仍按 Task 6 先抽出 `MatchResultWorkspace`，再在 Task 8 文件长度检查中记录后续拆分项。

Type consistency:

- 工作台 tab 类型统一为 `SessionWorkbenchTab`。
- 搜索与清洗 tab id 统一为 `search-clean`。
- 当前会话 ID 存储沿用 `bom_last_session_id` 和 `bom_last_bom_id`。
