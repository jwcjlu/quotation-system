import { useEffect, useMemo, useState } from 'react'
import { listLineGaps, listMatchRuns, type BOMLineGap, type MatchRunListItem } from '../../api'
import {
  PAGE_SIZE_OPTIONS,
  type PageSize,
  pageSummary,
  paginateRows,
  textMatchesKeyword,
} from './sessionPanelUtils'

interface SessionGapsPanelProps {
  sessionId: string
}

export function SessionGapsPanel({ sessionId }: SessionGapsPanelProps) {
  const [gaps, setGaps] = useState<BOMLineGap[]>([])
  const [runs, setRuns] = useState<MatchRunListItem[]>([])
  const [loading, setLoading] = useState(false)
  const [keyword, setKeyword] = useState('')
  const [status, setStatus] = useState('')
  const [gapType, setGapType] = useState('')
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState<PageSize>(50)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    ;(async () => {
      try {
        const [gapReply, runReply] = await Promise.all([listLineGaps(sessionId), listMatchRuns(sessionId)])
        if (!cancelled) {
          setGaps(gapReply.gaps)
          setRuns(runReply.runs)
        }
      } catch {
        if (!cancelled) {
          setGaps([])
          setRuns([])
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [sessionId])

  const filtered = useMemo(
    () =>
      gaps.filter((gap) => {
        if (status && gap.resolution_status !== status) return false
        if (gapType && gap.gap_type !== gapType) return false
        return textMatchesKeyword([gap.line_no, gap.mpn, gap.reason_code, gap.reason_detail], keyword)
      }),
    [gapType, gaps, keyword, status]
  )
  const paged = paginateRows(filtered, page, pageSize)

  useEffect(() => {
    setPage(1)
  }, [keyword, status, gapType, pageSize])

  return (
    <section className="space-y-4" data-testid="session-gaps-panel">
      <div className="rounded-lg border border-slate-200 bg-white p-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h4 className="font-semibold text-slate-900">缺口处理</h4>
            <p className="mt-1 text-sm text-slate-500">定位无报价、无库存、未匹配等缺口并查看匹配运行记录</p>
          </div>
          <div className="text-sm text-slate-500">{loading ? '加载中...' : pageSummary(paged.page, paged.totalPages, paged.total)}</div>
        </div>
        <div className="mt-4 grid gap-2 md:grid-cols-[minmax(0,1fr)_10rem_10rem_8rem]">
          <input value={keyword} onChange={(event) => setKeyword(event.target.value)} placeholder="MPN / 原因" className="rounded border border-slate-300 px-3 py-2 text-sm" />
          <select value={status} onChange={(event) => setStatus(event.target.value)} className="rounded border border-slate-300 px-3 py-2 text-sm">
            <option value="">全部状态</option>
            <option value="open">待处理</option>
            <option value="resolved">已处理</option>
            <option value="ignored">已忽略</option>
          </select>
          <select value={gapType} onChange={(event) => setGapType(event.target.value)} className="rounded border border-slate-300 px-3 py-2 text-sm">
            <option value="">全部类型</option>
            <option value="no_quote">无报价</option>
            <option value="no_stock">无库存</option>
            <option value="no_match">未匹配</option>
          </select>
          <select value={pageSize} onChange={(event) => setPageSize(Number(event.target.value) as PageSize)} className="rounded border border-slate-300 px-3 py-2 text-sm">
            {PAGE_SIZE_OPTIONS.map((size) => <option key={size} value={size}>每页 {size}</option>)}
          </select>
        </div>
      </div>
      <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_20rem]">
        <div className="overflow-x-auto rounded-lg border border-slate-200 bg-white">
          <table className="w-full min-w-[680px] text-sm">
            <thead className="bg-slate-50 text-left text-slate-600">
              <tr>
                <th className="px-3 py-2">行号</th>
                <th className="px-3 py-2">MPN</th>
                <th className="px-3 py-2">类型</th>
                <th className="px-3 py-2">状态</th>
                <th className="px-3 py-2">原因</th>
                <th className="px-3 py-2">替代料</th>
              </tr>
            </thead>
            <tbody>
              {paged.rows.length === 0 ? (
                <tr><td colSpan={6} className="px-3 py-8 text-center text-slate-500">暂无匹配缺口</td></tr>
              ) : (
                paged.rows.map((gap) => (
                  <tr key={gap.gap_id} className="border-t border-slate-100">
                    <td className="px-3 py-2">{gap.line_no}</td>
                    <td className="px-3 py-2 font-mono">{gap.mpn}</td>
                    <td className="px-3 py-2">{gap.gap_type || '-'}</td>
                    <td className="px-3 py-2">{gap.resolution_status || '-'}</td>
                    <td className="px-3 py-2">{gap.reason_detail || gap.reason_code || '-'}</td>
                    <td className="px-3 py-2">{gap.substitute_mpn || '-'}</td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
        <aside className="rounded-lg border border-slate-200 bg-white p-4">
          <h5 className="font-medium text-slate-900">匹配运行</h5>
          <div className="mt-3 space-y-2">
            {runs.length === 0 ? (
              <p className="text-sm text-slate-500">暂无运行记录</p>
            ) : (
              runs.slice(0, 5).map((run) => (
                <div key={run.run_id} className="rounded border border-slate-100 p-3 text-sm">
                  <div className="font-medium text-slate-800">#{run.run_no} {run.status}</div>
                  <div className="mt-1 text-slate-500">{run.matched_line_count}/{run.line_total} 已匹配</div>
                </div>
              ))
            )}
          </div>
        </aside>
      </div>
      <div className="flex justify-end gap-2">
        <button type="button" disabled={paged.page <= 1} onClick={() => setPage((value) => value - 1)} className="rounded border border-slate-300 px-3 py-1.5 text-sm disabled:opacity-40">上一页</button>
        <button type="button" disabled={paged.page >= paged.totalPages} onClick={() => setPage((value) => value + 1)} className="rounded border border-slate-300 px-3 py-1.5 text-sm disabled:opacity-40">下一页</button>
      </div>
    </section>
  )
}
