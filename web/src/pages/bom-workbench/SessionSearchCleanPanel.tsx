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
    const items = (searchTasks?.tasks ?? [])
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
    <div className="space-y-4" data-testid="search-clean-panel">
      <SearchTaskStatusPanel
        data={searchTasks}
        loading={searchTasksLoading}
        retrying={retrying}
        onRefresh={() => void loadSearchTasks()}
        onRetryBatch={() => void retryBatch()}
        onRetryTask={(task) => void retryTask(task)}
      />
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
