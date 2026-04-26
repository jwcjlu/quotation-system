import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  autoMatch,
  listManufacturerCanonicals,
  listSessionSearchTasks,
  retrySearchTasks,
  type ListSessionSearchTasksReply,
  type ManufacturerCanonicalRow,
  type MatchItem,
  type SessionSearchTaskRow,
} from '../../api'
import { SearchTaskStatusPanel } from '../sourcing-session/SearchTaskStatusPanel'
import { ManufacturerAliasReviewPanel, type PendingMfrRow } from './ManufacturerAliasReviewPanel'
import {
  PAGE_SIZE_OPTIONS,
  type PageSize,
  pageSummary,
  paginateRows,
  textMatchesKeyword,
} from './sessionPanelUtils'

function collectPendingMfrRows(items: MatchItem[]): PendingMfrRow[] {
  const grouped = new Map<string, { lines: Set<number>; demand: Set<string> }>()
  for (const item of items) {
    for (const raw of item.mfr_mismatch_quote_manufacturers ?? []) {
      const alias = raw.trim()
      if (!alias) continue
      let group = grouped.get(alias)
      if (!group) {
        group = { lines: new Set(), demand: new Set() }
        grouped.set(alias, group)
      }
      group.lines.add(item.index)
      if (item.demand_manufacturer?.trim()) group.demand.add(item.demand_manufacturer.trim())
    }
  }
  return Array.from(grouped.entries()).map(([alias, group]) => ({
    alias,
    lineIndexes: Array.from(group.lines).sort((a, b) => a - b),
    demandHint: Array.from(group.demand).join(', '),
  }))
}

interface SessionSearchCleanPanelProps {
  sessionId: string
}

export function SessionSearchCleanPanel({ sessionId }: SessionSearchCleanPanelProps) {
  const [searchTasks, setSearchTasks] = useState<ListSessionSearchTasksReply | null>(null)
  const [searchTasksLoading, setSearchTasksLoading] = useState(false)
  const [retrying, setRetrying] = useState(false)
  const [matchItems, setMatchItems] = useState<MatchItem[]>([])
  const [canonicalRows, setCanonicalRows] = useState<ManufacturerCanonicalRow[]>([])
  const [aliasErr, setAliasErr] = useState<string | null>(null)
  const [keyword, setKeyword] = useState('')
  const [platform, setPlatform] = useState('')
  const [status, setStatus] = useState('')
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState<PageSize>(50)

  const loadSearchTasks = useCallback(async () => {
    setSearchTasksLoading(true)
    try {
      setSearchTasks(await listSessionSearchTasks(sessionId))
    } catch {
      setSearchTasks(null)
    } finally {
      setSearchTasksLoading(false)
    }
  }, [sessionId])

  useEffect(() => {
    void loadSearchTasks()
  }, [loadSearchTasks])

  useEffect(() => {
    let cancelled = false
    ;(async () => {
      setAliasErr(null)
      try {
        const [matchReply, canonicals] = await Promise.all([
          autoMatch(sessionId),
          listManufacturerCanonicals(),
        ])
        if (!cancelled) {
          setMatchItems(matchReply.items)
          setCanonicalRows(canonicals)
        }
      } catch (error) {
        if (!cancelled) {
          setMatchItems([])
          setCanonicalRows([])
          setAliasErr(error instanceof Error ? error.message : '\u5382\u724c\u522b\u540d\u52a0\u8f7d\u5931\u8d25')
        }
      }
    })()
    return () => {
      cancelled = true
    }
  }, [sessionId])

  const pendingRows = useMemo(() => collectPendingMfrRows(matchItems), [matchItems])
  const filteredTasks = useMemo(() => {
    const tasks = searchTasks?.tasks ?? []
    return tasks.filter((task) => {
      if (platform && task.platform_id !== platform) return false
      if (status && task.search_ui_state !== status) return false
      return textMatchesKeyword(
        [
          task.line_no,
          task.mpn_raw,
          task.mpn_norm,
          task.platform_id,
          task.platform_name,
          task.last_error,
          task.retry_blocked_reason,
        ],
        keyword
      )
    })
  }, [keyword, platform, searchTasks?.tasks, status])
  const pagedTasks = paginateRows(filteredTasks, page, pageSize)
  const platformOptions = useMemo(
    () => Array.from(new Set((searchTasks?.tasks ?? []).map((task) => task.platform_id).filter(Boolean))).sort(),
    [searchTasks?.tasks]
  )
  const filteredSearchTasks = useMemo<ListSessionSearchTasksReply | null>(() => {
    if (!searchTasks) return null
    const summary = {
      total: filteredTasks.length,
      pending: filteredTasks.filter((task) => task.search_ui_state === 'pending').length,
      searching: filteredTasks.filter((task) => task.search_ui_state === 'searching').length,
      succeeded: filteredTasks.filter((task) => task.search_ui_state === 'succeeded').length,
      no_data: filteredTasks.filter((task) => task.search_ui_state === 'no_data').length,
      failed: filteredTasks.filter((task) => task.search_ui_state === 'failed').length,
      skipped: filteredTasks.filter((task) => task.search_ui_state === 'skipped').length,
      cancelled: filteredTasks.filter((task) => task.search_ui_state === 'cancelled').length,
      missing: filteredTasks.filter((task) => task.search_ui_state === 'missing').length,
      retryable: filteredTasks.filter((task) => task.retryable).length,
    }
    return { ...searchTasks, summary, tasks: pagedTasks.rows }
  }, [filteredTasks, pagedTasks.rows, searchTasks])

  useEffect(() => {
    setPage(1)
  }, [keyword, platform, status, pageSize])

  const retryTask = async (task: SessionSearchTaskRow) => {
    setRetrying(true)
    try {
      await retrySearchTasks(sessionId, [{ mpn: task.mpn_norm || task.mpn_raw, platform_id: task.platform_id }])
      await loadSearchTasks()
    } finally {
      setRetrying(false)
    }
  }

  const retryBatch = async () => {
    const items = filteredTasks
      .filter((task) => task.retryable && task.search_ui_state !== 'no_data')
      .map((task) => ({ mpn: task.mpn_norm || task.mpn_raw, platform_id: task.platform_id }))
    if (items.length === 0) return
    setRetrying(true)
    try {
      await retrySearchTasks(sessionId, items)
      await loadSearchTasks()
    } finally {
      setRetrying(false)
    }
  }

  return (
    <div className="space-y-4" data-testid="session-search-clean-panel">
      <div className="rounded-lg border border-slate-200 bg-white p-4" data-testid="search-clean-panel">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h4 className="font-semibold text-slate-900">搜索清洗</h4>
            <p className="mt-1 text-sm text-slate-500">按平台、状态、MPN 和错误信息定位搜索任务</p>
          </div>
          <div className="text-sm text-slate-500">
            {pageSummary(pagedTasks.page, pagedTasks.totalPages, pagedTasks.total)}
          </div>
        </div>
        <div className="mt-4 grid gap-2 md:grid-cols-[minmax(0,1fr)_10rem_10rem_8rem]">
          <input
            value={keyword}
            onChange={(event) => setKeyword(event.target.value)}
            placeholder="MPN / 任务 / 错误"
            className="rounded border border-slate-300 px-3 py-2 text-sm"
          />
          <select
            value={platform}
            onChange={(event) => setPlatform(event.target.value)}
            className="rounded border border-slate-300 px-3 py-2 text-sm"
          >
            <option value="">全部平台</option>
            {platformOptions.map((item) => (
              <option key={item} value={item}>
                {item}
              </option>
            ))}
          </select>
          <select
            value={status}
            onChange={(event) => setStatus(event.target.value)}
            className="rounded border border-slate-300 px-3 py-2 text-sm"
          >
            <option value="">全部状态</option>
            <option value="pending">待搜索</option>
            <option value="searching">搜索中</option>
            <option value="succeeded">成功</option>
            <option value="no_data">无数据</option>
            <option value="failed">失败</option>
            <option value="missing">缺任务</option>
          </select>
          <select
            value={pageSize}
            onChange={(event) => setPageSize(Number(event.target.value) as PageSize)}
            className="rounded border border-slate-300 px-3 py-2 text-sm"
          >
            {PAGE_SIZE_OPTIONS.map((size) => (
              <option key={size} value={size}>
                每页 {size}
              </option>
            ))}
          </select>
        </div>
      </div>
      <SearchTaskStatusPanel
        data={filteredSearchTasks}
        loading={searchTasksLoading}
        retrying={retrying}
        onRefresh={() => void loadSearchTasks()}
        onRetryBatch={() => void retryBatch()}
        onRetryTask={(task) => void retryTask(task)}
      />
      <div className="flex justify-end gap-2">
        <button
          type="button"
          disabled={pagedTasks.page <= 1}
          onClick={() => setPage((value) => value - 1)}
          className="rounded border border-slate-300 px-3 py-1.5 text-sm disabled:opacity-40"
        >
          上一页
        </button>
        <button
          type="button"
          disabled={pagedTasks.page >= pagedTasks.totalPages}
          onClick={() => setPage((value) => value + 1)}
          className="rounded border border-slate-300 px-3 py-1.5 text-sm disabled:opacity-40"
        >
          下一页
        </button>
      </div>
      {aliasErr && (
        <div className="rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-900">
          {aliasErr}
        </div>
      )}
      <ManufacturerAliasReviewPanel
        pendingRows={pendingRows}
        canonicalRows={canonicalRows}
        onApproved={() => void loadSearchTasks()}
        onManualSuccess={() => void loadSearchTasks()}
      />
    </div>
  )
}
