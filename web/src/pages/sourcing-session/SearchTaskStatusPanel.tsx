import type { ListSessionSearchTasksReply, SessionSearchTaskRow } from '../../api'

interface SearchTaskStatusPanelProps {
  data: ListSessionSearchTasksReply | null
  loading?: boolean
  retrying?: boolean
  defaultOpen?: boolean
  onRefresh: () => void
  onRetryBatch: () => void
  onRetryTask: (task: SessionSearchTaskRow) => void
}

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
const SEARCH_ATTENTION_STATES = new Set(['failed', 'missing'])

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
  defaultOpen = false,
  onRefresh,
  onRetryBatch,
  onRetryTask,
}: SearchTaskStatusPanelProps) {
  const summary = data?.summary
  const tasks = data?.tasks ?? []
  const retryableAnomalies = tasks.filter(
    (task) => task.retryable && task.search_ui_state !== 'no_data'
  )
  const orderedTasks = [...tasks].sort((a, b) => {
    const aAttention = a.retryable || SEARCH_ATTENTION_STATES.has(a.search_ui_state)
    const bAttention = b.retryable || SEARCH_ATTENTION_STATES.has(b.search_ui_state)
    if (aAttention !== bAttention) return aAttention ? -1 : 1
    return a.line_no - b.line_no
  })

  return (
    <details
      data-testid="session-search-task-panel"
      open={defaultOpen || undefined}
      className="rounded-xl border border-slate-200 bg-white shadow-sm"
    >
      <summary className="flex cursor-pointer list-none flex-wrap items-center justify-between gap-3 p-6 [&::-webkit-details-marker]:hidden">
        <div>
          <h3 className="font-semibold text-slate-800">搜索任务状态</h3>
          <p className="mt-1 text-sm text-slate-500">
            {summary ? `共 ${summary.total} 个任务，可重试 ${summary.retryable} 个` : '暂无任务状态'}
          </p>
        </div>
        <span className="rounded-lg border border-slate-200 px-2 py-1 text-xs text-slate-500">
          展开
        </span>
      </summary>
      <div className="border-t border-slate-100 p-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h3 className="font-semibold text-slate-800">搜索任务明细</h3>
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
          {[
            ['等待', summary.pending],
            ['搜索中', summary.searching],
            ['成功', summary.succeeded],
            ['无结果', summary.no_data],
            ['失败', summary.failed],
            ['跳过', summary.skipped],
            ['取消', summary.cancelled],
            ['缺任务', summary.missing],
            ['可重试', summary.retryable],
            ['总数', summary.total],
          ].map(([label, value]) => (
            <div key={label} className="rounded-lg border border-slate-200 px-3 py-2">
              <div className="text-xs text-slate-500">{label}</div>
              <div className="mt-1 text-lg font-semibold text-slate-800">{value}</div>
            </div>
          ))}
        </div>
      )}

      <div className="mt-4 overflow-x-auto">
        <table className="w-full min-w-[900px] text-sm">
          <thead>
            <tr className="border-b border-slate-200 bg-slate-50 text-left">
              <th className="px-2 py-2">行号</th>
              <th className="px-2 py-2">MPN</th>
              <th className="px-2 py-2">平台</th>
              <th className="px-2 py-2">搜索状态</th>
              <th className="px-2 py-2">调度状态</th>
              <th className="px-2 py-2">次数</th>
              <th className="px-2 py-2">更新时间</th>
              <th className="px-2 py-2">错误</th>
              <th className="px-2 py-2">操作</th>
            </tr>
          </thead>
          <tbody>
            {tasks.length === 0 ? (
              <tr>
                <td colSpan={9} className="px-2 py-8 text-center text-slate-500">
                  {loading ? '正在加载搜索任务' : '暂无搜索任务'}
                </td>
              </tr>
            ) : (
              orderedTasks.map((task) => (
                <tr
                  key={`${task.line_id}-${task.platform_id}-${task.search_task_id || 'missing'}`}
                  className="border-b border-slate-100 align-top"
                >
                  <td className="px-2 py-2">{task.line_no}</td>
                  <td className="px-2 py-2 font-mono">{task.mpn_raw || task.mpn_norm}</td>
                  <td className="px-2 py-2 font-mono">{task.platform_id}</td>
                  <td className="px-2 py-2">
                    <span
                      className={`inline-flex min-w-16 justify-center rounded-full border px-2 py-0.5 text-xs font-medium ${stateClass(
                        task.search_ui_state
                      )}`}
                    >
                      {displayState(task.search_ui_state)}
                    </span>
                  </td>
                  <td className="px-2 py-2">
                    <div>{task.dispatch_task_state || '-'}</div>
                    {task.dispatch_agent_id && (
                      <div className="text-xs text-slate-500">{task.dispatch_agent_id}</div>
                    )}
                  </td>
                  <td className="px-2 py-2">
                    {task.attempt}
                    {task.retry_max > 0 ? ` / ${task.retry_max}` : ''}
                  </td>
                  <td className="px-2 py-2 text-xs text-slate-600">
                    {compactTime(task.updated_at)}
                  </td>
                  <td className="max-w-[220px] px-2 py-2 text-xs text-slate-600">
                    <span className="line-clamp-2">
                      {task.last_error || task.retry_blocked_reason || '-'}
                    </span>
                  </td>
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
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
      </div>
    </details>
  )
}
