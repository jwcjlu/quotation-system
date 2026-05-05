import { useEffect, useState } from 'react'
import type { ListSessionSearchTasksReply, SessionSearchTaskRow } from '../../api'

interface SearchTaskStatusPanelProps {
  data: ListSessionSearchTasksReply | null
  loading?: boolean
  retrying?: boolean
  onRefresh: () => void
  onRetryBatch: () => void
  onRetryTask: (task: SessionSearchTaskRow) => void
  quickFilterValue?: SearchTaskQuickFilter
  onQuickFilterChange?: (value: SearchTaskQuickFilter) => void
}

type SearchTaskQuickFilter = SessionSearchTaskRow['search_ui_state'] | 'retryable' | null
type SearchTaskQuickFilterStat = Exclude<SearchTaskQuickFilter, null>

type SearchTaskColumnKey =
  | 'line_no'
  | 'mpn'
  | 'platform'
  | 'search_state'
  | 'dispatch_state'
  | 'attempt'
  | 'updated_at'
  | 'error'
  | 'actions'

const SEARCH_TASK_TABLE_STORAGE_KEY = 'bom.workbench.searchTask.visibleColumns.v1'
const SEARCH_TASK_COLUMNS: Array<{ key: SearchTaskColumnKey; label: string }> = [
  { key: 'line_no', label: '行号' },
  { key: 'mpn', label: 'MPN' },
  { key: 'platform', label: '平台' },
  { key: 'search_state', label: '搜索状态' },
  { key: 'dispatch_state', label: '调度状态' },
  { key: 'attempt', label: '次数' },
  { key: 'updated_at', label: '更新时间' },
  { key: 'error', label: '错误' },
  { key: 'actions', label: '操作' },
]
const DEFAULT_SEARCH_TASK_COLUMNS: SearchTaskColumnKey[] = SEARCH_TASK_COLUMNS.map((c) => c.key)
const CORE_SEARCH_TASK_COLUMNS: SearchTaskColumnKey[] = [
  'line_no',
  'mpn',
  'platform',
  'search_state',
  'error',
]

const STATE_CLASS: Record<string, string> = {
  pending: 'bg-slate-100 text-slate-700 border-slate-200',
  searching: 'bg-blue-50 text-blue-700 border-blue-200',
  succeeded: 'bg-emerald-50 text-emerald-700 border-emerald-200',
  no_data: 'bg-amber-50 text-amber-800 border-amber-200',
  failed: 'bg-rose-50 text-rose-700 border-rose-200',
  skipped: 'bg-slate-100 text-slate-500 border-slate-200',
  cancelled: 'bg-zinc-100 text-zinc-600 border-zinc-200',
  missing: 'bg-orange-50 text-orange-700 border-orange-200',
}

function stateClass(state: string) {
  return STATE_CLASS[state] ?? STATE_CLASS.failed
}

function displayState(state: string) {
  switch (state) {
    case 'pending':
      return '等待'
    case 'searching':
      return '搜索中'
    case 'succeeded':
      return '成功'
    case 'no_data':
      return '无结果'
    case 'failed':
      return '失败'
    case 'skipped':
      return '跳过'
    case 'cancelled':
      return '取消'
    case 'missing':
      return '缺任务'
    default:
      return state || '-'
  }
}

function compactTime(value: string) {
  if (!value) return '-'
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return value
  return d.toLocaleString()
}

export function SearchTaskStatusPanel({
  data,
  loading,
  retrying,
  onRefresh,
  onRetryBatch,
  onRetryTask,
  quickFilterValue,
  onQuickFilterChange,
}: SearchTaskStatusPanelProps) {
  const [showColumnSettings, setShowColumnSettings] = useState(false)
  const [localQuickFilter, setLocalQuickFilter] = useState<SearchTaskQuickFilter>(null)
  const [visibleColumns, setVisibleColumns] = useState<SearchTaskColumnKey[]>(() => {
    if (typeof window === 'undefined') return DEFAULT_SEARCH_TASK_COLUMNS
    try {
      const raw = window.localStorage.getItem(SEARCH_TASK_TABLE_STORAGE_KEY)
      if (!raw) return DEFAULT_SEARCH_TASK_COLUMNS
      const parsed = JSON.parse(raw)
      if (!Array.isArray(parsed)) return DEFAULT_SEARCH_TASK_COLUMNS
      const valid = parsed.filter((k): k is SearchTaskColumnKey =>
        SEARCH_TASK_COLUMNS.some((c) => c.key === k)
      )
      return valid.length > 0 ? valid : DEFAULT_SEARCH_TASK_COLUMNS
    } catch {
      return DEFAULT_SEARCH_TASK_COLUMNS
    }
  })
  const summary = data?.summary
  const tasks = data?.tasks ?? []
  const quickFilter = quickFilterValue ?? localQuickFilter
  const setQuickFilter = (value: SearchTaskQuickFilter) => {
    if (onQuickFilterChange) {
      onQuickFilterChange(value)
      return
    }
    setLocalQuickFilter(value)
  }
  const filteredTasks =
    quickFilter === null
      ? tasks
      : tasks.filter((task) =>
          quickFilter === 'retryable' ? task.retryable : task.search_ui_state === quickFilter
        )
  const retryableAnomalies = tasks.filter(
    (task) => task.retryable && task.search_ui_state !== 'no_data'
  )
  const activeFilterLabel =
    quickFilter === null
      ? ''
      : quickFilter === 'retryable'
        ? '可重试'
        : displayState(quickFilter)
  const isVisible = (key: SearchTaskColumnKey) => visibleColumns.includes(key)
  const toggleColumn = (key: SearchTaskColumnKey) => {
    setVisibleColumns((prev) => {
      if (prev.includes(key)) {
        if (prev.length <= 1) return prev
        return prev.filter((x) => x !== key)
      }
      const order = SEARCH_TASK_COLUMNS.map((c) => c.key)
      return [...prev, key].sort((a, b) => order.indexOf(a) - order.indexOf(b))
    })
  }
  useEffect(() => {
    if (typeof window === 'undefined') return
    window.localStorage.setItem(SEARCH_TASK_TABLE_STORAGE_KEY, JSON.stringify(visibleColumns))
  }, [visibleColumns])

  return (
    <section className="rounded-xl border border-slate-200 bg-white p-6 shadow-sm">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h3 className="font-semibold text-slate-800">搜索任务状态</h3>
          <p className="mt-1 text-sm text-slate-500">
            {summary ? `共 ${summary.total} 个任务，可重试 ${summary.retryable} 个` : '暂无任务状态'}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <button
            type="button"
            onClick={onRefresh}
            disabled={loading}
            className="rounded-lg border border-slate-300 bg-white px-3 py-1.5 text-sm hover:bg-slate-50 disabled:opacity-50"
          >
            {loading ? '刷新中...' : '刷新'}
          </button>
          <button
            type="button"
            onClick={onRetryBatch}
            disabled={retrying || retryableAnomalies.length === 0}
            className="rounded-lg bg-blue-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-700 disabled:bg-slate-300 disabled:text-slate-500"
          >
            {retrying ? '重试中...' : `重试异常任务 (${retryableAnomalies.length})`}
          </button>
        </div>
      </div>

      {summary && (
        <div className="mt-4 grid grid-cols-2 gap-2 sm:grid-cols-5 lg:grid-cols-10">
          {([
            ['等待', summary.pending, 'pending'],
            ['搜索中', summary.searching, 'searching'],
            ['成功', summary.succeeded, 'succeeded'],
            ['无结果', summary.no_data, 'no_data'],
            ['失败', summary.failed, 'failed'],
            ['跳过', summary.skipped, 'skipped'],
            ['取消', summary.cancelled, 'cancelled'],
            ['缺任务', summary.missing, 'missing'],
            ['可重试', summary.retryable, 'retryable'],
            ['总数', summary.total, null],
          ] as Array<[string, number, SearchTaskQuickFilter]>).map(([label, value, filter]) => (
            <button
              key={label}
              type="button"
              onClick={() => setQuickFilter(quickFilter === filter ? null : (filter as SearchTaskQuickFilterStat | null))}
              className={`rounded-lg border px-3 py-2 text-left transition-colors ${
                quickFilter === filter
                  ? 'border-blue-400 bg-blue-50'
                  : 'border-slate-200 hover:border-slate-300 hover:bg-slate-50'
              }`}
              title={`点击按${label}过滤`}
            >
              <div className="text-xs text-slate-500">{label}</div>
              <div className="mt-1 text-lg font-semibold text-slate-800">{value}</div>
            </button>
          ))}
        </div>
      )}

      <div className="mt-4 overflow-x-auto">
        <div className="mb-2 flex flex-wrap items-center gap-2 text-sm text-slate-600">
          <span>当前筛选：{activeFilterLabel || '全部'}</span>
          {quickFilter !== null && (
            <button
              type="button"
              onClick={() => setQuickFilter(null)}
              className="rounded border border-slate-300 px-2 py-0.5 text-xs text-slate-700 hover:bg-slate-50"
            >
              清空筛选
            </button>
          )}
        </div>
        <div className="mb-3">
          <button
            type="button"
            className="h-8 rounded-md border border-slate-300 px-3 text-sm text-slate-700 hover:bg-slate-50"
            onClick={() => setShowColumnSettings((v) => !v)}
          >
            {showColumnSettings ? '收起显示字段' : '显示字段设置'}
          </button>
          {showColumnSettings && (
            <div className="mt-3 rounded-md border border-slate-200 bg-slate-50 p-3">
              <div className="mb-2 text-sm font-medium text-slate-700">自定义显示字段（至少保留 1 列）</div>
              <div className="grid grid-cols-2 gap-2 md:grid-cols-3 xl:grid-cols-4">
                {SEARCH_TASK_COLUMNS.map((col) => (
                  <label key={col.key} className="flex items-center gap-2 text-sm text-slate-700">
                    <input
                      type="checkbox"
                      checked={isVisible(col.key)}
                      onChange={() => toggleColumn(col.key)}
                      disabled={isVisible(col.key) && visibleColumns.length <= 1}
                    />
                    {col.label}
                  </label>
                ))}
              </div>
              <div className="mt-3">
                <button
                  type="button"
                  className="mr-2 h-8 rounded-md border border-slate-300 px-3 text-sm text-slate-700 hover:bg-white"
                  onClick={() => setVisibleColumns(DEFAULT_SEARCH_TASK_COLUMNS)}
                >
                  恢复默认字段
                </button>
                <button
                  type="button"
                  className="mr-2 h-8 rounded-md border border-slate-300 px-3 text-sm text-slate-700 hover:bg-white"
                  onClick={() => setVisibleColumns(CORE_SEARCH_TASK_COLUMNS)}
                >
                  仅核心字段
                </button>
                <button
                  type="button"
                  className="h-8 rounded-md border border-slate-300 px-3 text-sm text-slate-700 hover:bg-white"
                  onClick={() => setVisibleColumns(SEARCH_TASK_COLUMNS.map((c) => c.key))}
                >
                  全选字段
                </button>
              </div>
            </div>
          )}
        </div>
        <table className="w-full min-w-[900px] text-sm">
          <thead>
            <tr className="border-b border-slate-200 bg-slate-50 text-left">
              {isVisible('line_no') && <th className="px-2 py-2">行号</th>}
              {isVisible('mpn') && <th className="px-2 py-2">MPN</th>}
              {isVisible('platform') && <th className="px-2 py-2">平台</th>}
              {isVisible('search_state') && <th className="px-2 py-2">搜索状态</th>}
              {isVisible('dispatch_state') && <th className="px-2 py-2">调度状态</th>}
              {isVisible('attempt') && <th className="px-2 py-2">次数</th>}
              {isVisible('updated_at') && <th className="px-2 py-2">更新时间</th>}
              {isVisible('error') && <th className="px-2 py-2">错误</th>}
              {isVisible('actions') && <th className="px-2 py-2">操作</th>}
            </tr>
          </thead>
          <tbody>
            {filteredTasks.length === 0 ? (
              <tr>
                <td colSpan={Math.max(visibleColumns.length, 1)} className="px-2 py-8 text-center text-slate-500">
                  {loading ? '正在加载搜索任务' : quickFilter ? '当前筛选下暂无搜索任务' : '暂无搜索任务'}
                </td>
              </tr>
            ) : (
              filteredTasks.map((task) => (
                <tr
                  key={`${task.line_id}-${task.platform_id}-${task.search_task_id || 'missing'}`}
                  className="border-b border-slate-100 align-top"
                >
                  {isVisible('line_no') && <td className="px-2 py-2">{task.line_no}</td>}
                  {isVisible('mpn') && (
                    <td className="px-2 py-2 font-mono">{task.mpn_raw || task.mpn_norm}</td>
                  )}
                  {isVisible('platform') && (
                    <td className="px-2 py-2 font-mono">{task.platform_id}</td>
                  )}
                  {isVisible('search_state') && (
                    <td className="px-2 py-2">
                      <span
                        className={`inline-flex min-w-16 justify-center rounded-full border px-2 py-0.5 text-xs font-medium ${stateClass(
                          task.search_ui_state
                        )}`}
                      >
                        {displayState(task.search_ui_state)}
                      </span>
                    </td>
                  )}
                  {isVisible('dispatch_state') && (
                    <td className="px-2 py-2">
                      <div>{task.dispatch_task_state || '-'}</div>
                      {task.dispatch_agent_id && (
                        <div className="text-xs text-slate-500">{task.dispatch_agent_id}</div>
                      )}
                    </td>
                  )}
                  {isVisible('attempt') && (
                    <td className="px-2 py-2">
                      {task.attempt}
                      {task.retry_max > 0 ? ` / ${task.retry_max}` : ''}
                    </td>
                  )}
                  {isVisible('updated_at') && (
                    <td className="px-2 py-2 text-xs text-slate-600">{compactTime(task.updated_at)}</td>
                  )}
                  {isVisible('error') && (
                    <td className="max-w-[220px] px-2 py-2 text-xs text-slate-600">
                      <span className="line-clamp-2">
                        {task.last_error || task.retry_blocked_reason || '-'}
                      </span>
                    </td>
                  )}
                  {isVisible('actions') && (
                    <td className="px-2 py-2">
                      <button
                        type="button"
                        onClick={() => onRetryTask(task)}
                        disabled={retrying || !task.retryable}
                        className="text-xs font-medium text-blue-600 hover:underline disabled:text-slate-400 disabled:no-underline"
                      >
                        重试
                      </button>
                    </td>
                  )}
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </section>
  )
}
