import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  applyManufacturerAliasesToSession,
  approveManufacturerAliasCleaning,
  listManufacturerAliasCandidates,
  listManufacturerCanonicals,
  listSessionSearchTasks,
  retrySearchTasks,
  type ListSessionSearchTasksReply,
  type ManufacturerAliasCandidate,
  type ManufacturerCanonicalRow,
  type SessionSearchTaskRow,
} from '../../api'
import { SearchTaskStatusPanel } from '../sourcing-session/SearchTaskStatusPanel'
import { ManufacturerAliasReviewPanel, type PendingMfrRow } from './ManufacturerAliasReviewPanel'
import {
  DEFAULT_PAGE_SIZE,
  PAGE_SIZE_OPTIONS,
  type PageSize,
  pageSummary,
  paginateRows,
  textMatchesKeyword,
} from './sessionPanelUtils'

const RETRYABLE_STATUS_FILTER_VALUE = '__retryable__'

function pendingRowsFromCandidates(items: ManufacturerAliasCandidate[]): PendingMfrRow[] {
  return items.map((item) => ({
    kind: item.kind,
    alias: item.alias,
    recommendedCanonicalId: item.recommended_canonical_id,
    platformIds: item.platform_ids,
    lineIndexes: item.line_nos,
    demandHint: item.demand_hint,
  }))
}

interface SessionSearchCleanPanelProps {
  sessionId: string
}

export function SessionSearchCleanPanel({ sessionId }: SessionSearchCleanPanelProps) {
  const [searchTasks, setSearchTasks] = useState<ListSessionSearchTasksReply | null>(null)
  const [searchTasksLoading, setSearchTasksLoading] = useState(false)
  const [retrying, setRetrying] = useState(false)
  const [pendingRows, setPendingRows] = useState<PendingMfrRow[]>([])
  const [canonicalRows, setCanonicalRows] = useState<ManufacturerCanonicalRow[]>([])
  const [aliasErr, setAliasErr] = useState<string | null>(null)
  const [keyword, setKeyword] = useState('')
  const [platform, setPlatform] = useState('')
  const [status, setStatus] = useState('')
  const [cardQuickFilter, setCardQuickFilter] = useState<'retryable' | null>(null)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState<PageSize>(DEFAULT_PAGE_SIZE)

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
        const [candidates, canonicals] = await Promise.all([
          listManufacturerAliasCandidates(sessionId),
          listManufacturerCanonicals(),
        ])
        if (!cancelled) {
          setPendingRows(pendingRowsFromCandidates(candidates))
          setCanonicalRows(canonicals)
        }
      } catch (error) {
        if (!cancelled) {
          setPendingRows([])
          setCanonicalRows([])
          setAliasErr(error instanceof Error ? error.message : '\u5382\u724c\u522b\u540d\u52a0\u8f7d\u5931\u8d25')
        }
      }
    })()
    return () => {
      cancelled = true
    }
  }, [sessionId])

  const filteredTasks = useMemo(() => {
    const tasks = searchTasks?.tasks ?? []
    return tasks.filter((task) => {
      if (platform && task.platform_id !== platform) return false
      if (status && task.search_ui_state !== status) return false
      if (cardQuickFilter === 'retryable' && !task.retryable) return false
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
  }, [cardQuickFilter, keyword, platform, searchTasks?.tasks, status])
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
  }, [cardQuickFilter, keyword, platform, status, pageSize])

  const quickFilterValue = useMemo<
    SessionSearchTaskRow['search_ui_state'] | 'retryable' | null
  >(() => {
    if (cardQuickFilter === 'retryable') return 'retryable'
    if (!status) return null
    return status as SessionSearchTaskRow['search_ui_state']
  }, [cardQuickFilter, status])
  const statusSelectValue = cardQuickFilter === 'retryable' ? RETRYABLE_STATUS_FILTER_VALUE : status

  const handleCardQuickFilterChange = useCallback(
    (value: SessionSearchTaskRow['search_ui_state'] | 'retryable' | null) => {
      setPage(1)
      if (value === null) {
        setCardQuickFilter(null)
        setStatus('')
        return
      }
      if (value === 'retryable') {
        setCardQuickFilter('retryable')
        setStatus('')
        return
      }
      setCardQuickFilter(null)
      setStatus(value)
    },
    []
  )

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

  async function handleApproveManufacturerAlias(input: { alias: string; canonical_id: string; display_name: string }) {
    await approveManufacturerAliasCleaning(sessionId, input)
    const [candidates, canonicals] = await Promise.all([
      listManufacturerAliasCandidates(sessionId),
      listManufacturerCanonicals(),
      loadSearchTasks(),
    ])
    setPendingRows(pendingRowsFromCandidates(candidates))
    setCanonicalRows(canonicals)
  }

  async function handleApplyExistingAliases() {
    await applyManufacturerAliasesToSession(sessionId)
    const [candidates, canonicals] = await Promise.all([
      listManufacturerAliasCandidates(sessionId),
      listManufacturerCanonicals(),
      loadSearchTasks(),
    ])
    setPendingRows(pendingRowsFromCandidates(candidates))
    setCanonicalRows(canonicals)
  }

  const summary = searchTasks?.summary
  const failedCount = (summary?.failed ?? 0) + (summary?.missing ?? 0)

  return (
    <div className="space-y-4" data-testid="session-search-clean-panel">
      <div className="grid gap-4 xl:grid-cols-4">
        <div className="rounded-lg border border-[#d7e0ed] bg-white p-4">
          <div className="text-sm font-bold text-slate-950">{'\u603b\u4efb\u52a1'}</div>
          <div className="mt-4 text-3xl font-bold text-[#2457c5]">{summary?.total ?? 0}</div>
        </div>
        <div className="rounded-lg border border-[#d7e0ed] bg-white p-4">
          <div className="text-sm font-bold text-slate-950">{'\u6210\u529f'}</div>
          <div className="mt-4 text-3xl font-bold text-[#12805c]">{summary?.succeeded ?? 0}</div>
        </div>
        <div className="rounded-lg border border-[#d7e0ed] bg-white p-4">
          <div className="text-sm font-bold text-slate-950">{'\u53ef\u91cd\u8bd5\u5f02\u5e38'}</div>
          <div className="mt-4 text-3xl font-bold text-[#a76505]">{failedCount}</div>
        </div>
        <div className="rounded-lg border border-[#d7e0ed] bg-white p-4">
          <div className="text-sm font-bold text-slate-950">{'\u5382\u5bb6\u522b\u540d\u5f85\u5ba1'}</div>
          <div className="mt-4 text-3xl font-bold text-[#2457c5]">{pendingRows.length}</div>
        </div>
      </div>

      <div className="rounded-lg border border-[#d7e0ed] bg-white p-3" data-testid="search-clean-panel">
        <div className="flex flex-wrap items-center gap-3">
          <div className="text-sm font-bold text-slate-950">{'\u5e73\u53f0 / \u72b6\u6001 / \u5173\u952e\u5b57'}</div>
          <div className="ml-auto text-sm text-slate-500">
            {pageSummary(pagedTasks.page, pagedTasks.totalPages, pagedTasks.total)}
          </div>
          <input
            value={keyword}
            onChange={(event) => setKeyword(event.target.value)}
            placeholder="MPN / 任务 / 错误"
            className="h-8 min-w-[12rem] flex-1 rounded-md border border-[#d7e0ed] px-3 text-sm"
          />
          <select
            value={platform}
            onChange={(event) => setPlatform(event.target.value)}
            className="h-8 rounded-md border border-[#d7e0ed] px-3 text-sm"
          >
            <option value="">全部平台</option>
            {platformOptions.map((item) => (
              <option key={item} value={item}>
                {item}
              </option>
            ))}
          </select>
          <select
            value={statusSelectValue}
            onChange={(event) => {
              const value = event.target.value
              if (value === RETRYABLE_STATUS_FILTER_VALUE) {
                setCardQuickFilter('retryable')
                setStatus('')
                return
              }
              setCardQuickFilter(null)
              setStatus(value)
            }}
            className={`h-8 rounded-md border px-3 text-sm ${
              statusSelectValue
                ? 'border-blue-400 bg-blue-50 text-blue-700'
                : 'border-[#d7e0ed] bg-white text-slate-900'
            }`}
          >
            <option value="">全部状态</option>
            <option value="pending">待搜索</option>
            <option value="searching">搜索中</option>
            <option value="succeeded">成功</option>
            <option value="no_data">无数据</option>
            <option value="failed">失败</option>
            <option value="missing">缺任务</option>
            <option value={RETRYABLE_STATUS_FILTER_VALUE}>可重试</option>
          </select>
          <select
            value={pageSize}
            onChange={(event) => setPageSize(Number(event.target.value) as PageSize)}
            className="h-8 rounded-md border border-[#d7e0ed] px-3 text-sm"
          >
            {PAGE_SIZE_OPTIONS.map((size) => (
              <option key={size} value={size}>
                每页 {size}
              </option>
            ))}
          </select>
          <button
            type="button"
            onClick={() => void retryBatch()}
            disabled={retrying || filteredTasks.length === 0}
            className="h-8 rounded-md bg-[#2457c5] px-6 text-sm font-bold text-white disabled:bg-slate-300"
          >
            {'\u6279\u91cf\u91cd\u8bd5'}
          </button>
          <button
            type="button"
            onClick={() => void loadSearchTasks()}
            disabled={searchTasksLoading}
            className="h-8 rounded-md bg-[#1f2a3d] px-6 text-sm font-bold text-white disabled:bg-slate-300"
          >
            {'\u5237\u65b0'}
          </button>
        </div>
      </div>
      <SearchTaskStatusPanel
        data={filteredSearchTasks}
        loading={searchTasksLoading}
        retrying={retrying}
        onRefresh={() => void loadSearchTasks()}
        onRetryBatch={() => void retryBatch()}
        onRetryTask={(task) => void retryTask(task)}
        quickFilterValue={quickFilterValue}
        onQuickFilterChange={handleCardQuickFilterChange}
      />
      <div className="flex justify-end gap-2">
        <button
          type="button"
          disabled={pagedTasks.page <= 1}
          onClick={() => setPage((value) => value - 1)}
          className="rounded-md border border-[#d7e0ed] px-4 py-1.5 text-sm disabled:opacity-40"
        >
          上一页
        </button>
        <button
          type="button"
          disabled={pagedTasks.page >= pagedTasks.totalPages}
          onClick={() => setPage((value) => value + 1)}
          className="rounded-md border border-[#d7e0ed] px-4 py-1.5 text-sm disabled:opacity-40"
        >
          下一页
        </button>
      </div>
      {aliasErr && (
        <div className="rounded-lg border border-[#f0c77d] bg-[#fff7e8] px-4 py-3 text-sm text-amber-900">
          {aliasErr}
        </div>
      )}
      <ManufacturerAliasReviewPanel
        pendingRows={pendingRows}
        canonicalRows={canonicalRows}
        onApprove={handleApproveManufacturerAlias}
        onApplyExisting={handleApplyExistingAliases}
      />
    </div>
  )
}
