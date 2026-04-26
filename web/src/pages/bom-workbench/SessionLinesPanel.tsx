import { useEffect, useMemo, useState } from 'react'
import {
  getBOMLines,
  getSessionSearchTaskCoverage,
  type BOMLineRow,
  type GetSessionSearchTaskCoverageReply,
} from '../../api'
import {
  PAGE_SIZE_OPTIONS,
  type PageSize,
  pageSummary,
  paginateRows,
  textMatchesKeyword,
} from './sessionPanelUtils'

interface SessionLinesPanelProps {
  sessionId: string
}

export function SessionLinesPanel({ sessionId }: SessionLinesPanelProps) {
  const [lines, setLines] = useState<BOMLineRow[]>([])
  const [coverage, setCoverage] = useState<GetSessionSearchTaskCoverageReply | null>(null)
  const [loading, setLoading] = useState(false)
  const [keyword, setKeyword] = useState('')
  const [mfr, setMfr] = useState('')
  const [availability, setAvailability] = useState('')
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState<PageSize>(50)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    ;(async () => {
      try {
        const [lineReply, coverageReply] = await Promise.all([
          getBOMLines(sessionId),
          getSessionSearchTaskCoverage(sessionId),
        ])
        if (!cancelled) {
          setLines(lineReply.lines)
          setCoverage(coverageReply)
        }
      } catch {
        if (!cancelled) {
          setLines([])
          setCoverage(null)
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [sessionId])

  const mfrOptions = useMemo(
    () => Array.from(new Set(lines.map((line) => line.mfr).filter(Boolean))).sort(),
    [lines]
  )
  const filtered = useMemo(
    () =>
      lines.filter((line) => {
        if (mfr && line.mfr !== mfr) return false
        if (availability && (line.availability_status || 'unknown') !== availability) return false
        return textMatchesKeyword([line.line_no, line.mpn, line.mfr, line.package], keyword)
      }),
    [availability, keyword, lines, mfr]
  )
  const paged = paginateRows(filtered, page, pageSize)

  useEffect(() => {
    setPage(1)
  }, [keyword, mfr, availability, pageSize])

  return (
    <section className="space-y-4" data-testid="session-lines-panel">
      <div className="rounded-lg border border-slate-200 bg-white p-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h4 className="font-semibold text-slate-900">BOM 行</h4>
            <p className="mt-1 text-sm text-slate-500">
              {coverage
                ? `搜索任务 ${coverage.existing_task_count}/${coverage.expected_task_count}`
                : '查看行明细、报价可用性和平台缺口'}
            </p>
          </div>
          <div className="text-sm text-slate-500">{loading ? '加载中...' : pageSummary(paged.page, paged.totalPages, paged.total)}</div>
        </div>
        <div className="mt-4 grid gap-2 md:grid-cols-[minmax(0,1fr)_10rem_10rem_8rem]">
          <input
            value={keyword}
            onChange={(event) => setKeyword(event.target.value)}
            placeholder="MPN / 行号 / 描述"
            className="rounded border border-slate-300 px-3 py-2 text-sm"
          />
          <select value={mfr} onChange={(event) => setMfr(event.target.value)} className="rounded border border-slate-300 px-3 py-2 text-sm">
            <option value="">全部厂家</option>
            {mfrOptions.map((item) => (
              <option key={item} value={item}>{item}</option>
            ))}
          </select>
          <select value={availability} onChange={(event) => setAvailability(event.target.value)} className="rounded border border-slate-300 px-3 py-2 text-sm">
            <option value="">全部可用性</option>
            <option value="ready">可采购</option>
            <option value="gap">有缺口</option>
            <option value="unknown">未知</option>
          </select>
          <select value={pageSize} onChange={(event) => setPageSize(Number(event.target.value) as PageSize)} className="rounded border border-slate-300 px-3 py-2 text-sm">
            {PAGE_SIZE_OPTIONS.map((size) => (
              <option key={size} value={size}>每页 {size}</option>
            ))}
          </select>
        </div>
      </div>
      <div className="overflow-x-auto rounded-lg border border-slate-200 bg-white">
        <table className="w-full min-w-[760px] text-sm">
          <thead className="bg-slate-50 text-left text-slate-600">
            <tr>
              <th className="px-3 py-2">行号</th>
              <th className="px-3 py-2">MPN</th>
              <th className="px-3 py-2">厂家</th>
              <th className="px-3 py-2">封装</th>
              <th className="px-3 py-2">数量</th>
              <th className="px-3 py-2">可用性</th>
              <th className="px-3 py-2">平台报价</th>
            </tr>
          </thead>
          <tbody>
            {paged.rows.length === 0 ? (
              <tr><td colSpan={7} className="px-3 py-8 text-center text-slate-500">暂无匹配 BOM 行</td></tr>
            ) : (
              paged.rows.map((line) => (
                <tr key={line.line_id} className="border-t border-slate-100">
                  <td className="px-3 py-2">{line.line_no}</td>
                  <td className="px-3 py-2 font-mono">{line.mpn}</td>
                  <td className="px-3 py-2">{line.mfr || '-'}</td>
                  <td className="px-3 py-2">{line.package || '-'}</td>
                  <td className="px-3 py-2">{line.qty}</td>
                  <td className="px-3 py-2">{line.availability_status || line.match_status || '-'}</td>
                  <td className="px-3 py-2">{line.usable_quote_platform_count}/{line.raw_quote_platform_count}</td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
      <div className="flex justify-end gap-2">
        <button type="button" disabled={paged.page <= 1} onClick={() => setPage((value) => value - 1)} className="rounded border border-slate-300 px-3 py-1.5 text-sm disabled:opacity-40">上一页</button>
        <button type="button" disabled={paged.page >= paged.totalPages} onClick={() => setPage((value) => value + 1)} className="rounded border border-slate-300 px-3 py-1.5 text-sm disabled:opacity-40">下一页</button>
      </div>
    </section>
  )
}
