import { useEffect, useMemo, useState } from 'react'
import {
  getBOMLines,
  getSessionSearchTaskCoverage,
  type BOMLineRow,
  type GetSessionSearchTaskCoverageReply,
} from '../../api'
import {
  DEFAULT_PAGE_SIZE,
  PAGE_SIZE_OPTIONS,
  type PageSize,
  pageSummary,
  paginateRows,
  textMatchesKeyword,
} from './sessionPanelUtils'

interface SessionLinesPanelProps {
  sessionId: string
}

function statusPill(status: string) {
  const normalized = status || 'unknown'
  const tone =
    normalized === 'ready'
      ? 'bg-[#e8f7f0] text-[#12805c]'
      : normalized === 'no_data' || normalized === 'mfr_mismatch' || normalized === 'gap'
        ? 'bg-[#fff2d8] text-[#a76505]'
        : 'bg-slate-100 text-slate-600'

  return (
    <span className={`inline-flex h-6 min-w-16 items-center justify-center rounded-full px-3 text-xs font-bold ${tone}`}>
      {normalized}
    </span>
  )
}

export function SessionLinesPanel({ sessionId }: SessionLinesPanelProps) {
  const [lines, setLines] = useState<BOMLineRow[]>([])
  const [coverage, setCoverage] = useState<GetSessionSearchTaskCoverageReply | null>(null)
  const [loading, setLoading] = useState(false)
  const [keyword, setKeyword] = useState('')
  const [mfr, setMfr] = useState('')
  const [availability, setAvailability] = useState('')
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState<PageSize>(DEFAULT_PAGE_SIZE)

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
        return textMatchesKeyword(
          [
            line.line_no,
            line.mpn,
            line.unified_mpn,
            line.reference_designator,
            line.substitute_mpn,
            line.description,
            line.remark,
            line.mfr,
            line.package,
          ],
          keyword
        )
      }),
    [availability, keyword, lines, mfr]
  )
  const paged = paginateRows(filtered, page, pageSize)
  const readyCount = lines.filter(
    (line) => (line.availability_status || line.match_status) === 'ready' || line.has_usable_quote
  ).length
  const gapCount = Math.max(0, lines.length - readyCount)
  const coveragePercent =
    coverage && coverage.expected_task_count > 0
      ? Math.round((coverage.existing_task_count / coverage.expected_task_count) * 100)
      : lines.length > 0
        ? Math.round((readyCount / lines.length) * 100)
        : 0

  useEffect(() => {
    setPage(1)
  }, [keyword, mfr, availability, pageSize])

  return (
    <section className="space-y-4" data-testid="session-lines-panel">
      <div className="grid gap-4 xl:grid-cols-[1fr_1fr_1.2fr]">
        <div className="rounded-lg border border-[#d7e0ed] bg-white p-4">
          <div className="text-sm font-bold text-slate-950">{'\u884c\u6570\u636e\u6982\u51b5'}</div>
          <div className="mt-4 flex items-end gap-3">
            <div className="text-3xl font-bold text-[#2457c5]">{lines.length}</div>
            <div className="pb-1 text-sm text-slate-600">{'\u884c BOM \u660e\u7ec6'}</div>
          </div>
        </div>
        <div className="rounded-lg border border-[#d7e0ed] bg-white p-4">
          <div className="text-sm font-bold text-slate-950">{'\u641c\u7d22\u8986\u76d6'}</div>
          <div className="mt-4 flex items-end gap-3">
            <div className="text-3xl font-bold text-[#12805c]">{coveragePercent}%</div>
            <div className="pb-1 text-sm text-slate-600">{'\u4efb\u52a1\u5df2\u751f\u6210'}</div>
          </div>
        </div>
        <div className="rounded-lg border border-[#f0c77d] bg-[#fff7e8] p-4">
          <div className="text-sm font-bold text-slate-950">{'\u53ef\u7528\u6027\u63d0\u9192'}</div>
          <p className="mt-4 text-sm text-slate-700">
            {gapCount > 0
              ? `${gapCount} \u884c\u7f3a\u5c11\u53ef\u7528\u62a5\u4ef7\uff0c\u53ef\u8df3\u8f6c\u7f3a\u53e3\u5904\u7406\u3002`
              : '\u5f53\u524d BOM \u884c\u5747\u6709\u53ef\u7528\u62a5\u4ef7\u3002'}
          </p>
        </div>
      </div>

      <div className="rounded-lg border border-[#d7e0ed] bg-white p-3">
        <div className="grid items-center gap-2 xl:grid-cols-[auto_160px_130px_140px_112px_76px_88px]">
          <div>
            <h4 className="whitespace-nowrap text-sm font-bold text-slate-950">{'\u641c\u7d22 / \u7b5b\u9009 / \u5206\u9875'}</h4>
          </div>
          <input
            value={keyword}
            onChange={(event) => setKeyword(event.target.value)}
            placeholder="MPN / 行号 / 描述"
            className="h-8 w-full rounded-md border border-[#d7e0ed] px-3 text-sm"
          />
          <select value={mfr} onChange={(event) => setMfr(event.target.value)} className="h-8 w-full rounded-md border border-[#d7e0ed] px-3 text-sm">
            <option value="">全部厂家</option>
            {mfrOptions.map((item) => (
              <option key={item} value={item}>{item}</option>
            ))}
          </select>
          <select value={availability} onChange={(event) => setAvailability(event.target.value)} className="h-8 w-full rounded-md border border-[#d7e0ed] px-3 text-sm">
            <option value="">全部可用性</option>
            <option value="ready">可采购</option>
            <option value="gap">有缺口</option>
            <option value="unknown">未知</option>
          </select>
          <select value={pageSize} onChange={(event) => setPageSize(Number(event.target.value) as PageSize)} className="h-8 w-full rounded-md border border-[#d7e0ed] px-3 text-sm">
            {PAGE_SIZE_OPTIONS.map((size) => (
              <option key={size} value={size}>每页 {size}</option>
            ))}
          </select>
          <button type="button" className="h-8 w-full rounded-md bg-[#2457c5] text-sm font-bold text-white">
            {'\u641c\u7d22'}
          </button>
          <button type="button" className="h-8 w-full rounded-md bg-[#12805c] text-sm font-bold text-white">
            {'\u65b0\u589e\u884c'}
          </button>
        </div>
      </div>
      <div className="overflow-x-auto rounded-lg border border-[#d7e0ed] bg-white">
        <table className="w-full min-w-[1200px] text-sm">
          <thead className="bg-[#f1f5f9] text-left text-slate-700">
            <tr>
              <th className="px-3 py-2">行号</th>
              <th className="px-3 py-2">客户原型号</th>
              <th className="px-3 py-2">统一型号</th>
              <th className="px-3 py-2">位号</th>
              <th className="px-3 py-2">替代型号</th>
              <th className="px-3 py-2">描述/规格</th>
              <th className="px-3 py-2">备注</th>
              <th className="px-3 py-2">厂家</th>
              <th className="px-3 py-2">数量</th>
              <th className="px-3 py-2">可用性</th>
              <th className="px-3 py-2">平台缺口</th>
              <th className="px-3 py-2">操作</th>
            </tr>
          </thead>
          <tbody>
            {paged.rows.length === 0 ? (
              <tr>
                <td colSpan={12} className="px-3 py-8 text-center text-slate-500">
                  暂无匹配 BOM 行
                </td>
              </tr>
            ) : (
              paged.rows.map((line) => (
                <tr key={line.line_id} className="border-t border-[#d9e1ec]">
                  <td className="px-3 py-3">{line.line_no}</td>
                  <td className="px-3 py-3 font-mono">{line.mpn}</td>
                  <td className="px-3 py-3 font-mono">{line.unified_mpn || '-'}</td>
                  <td className="px-3 py-3">{line.reference_designator || '-'}</td>
                  <td className="px-3 py-3 font-mono">{line.substitute_mpn || '-'}</td>
                  <td className="px-3 py-3">{line.description || '-'}</td>
                  <td className="px-3 py-3">{line.remark || '-'}</td>
                  <td className="px-3 py-3">{line.mfr || '-'}</td>
                  <td className="px-3 py-3">{line.qty}</td>
                  <td className="px-3 py-3">{statusPill(line.availability_status || line.match_status || '-')}</td>
                  <td className="px-3 py-3 text-slate-600">
                    {line.platform_gaps?.length ? line.platform_gaps.join(' / ') : '-'}
                  </td>
                  <td className="px-3 py-3">
                    <button type="button" className="text-sm font-medium text-[#2457c5]">
                      {'\u7f16\u8f91'}
                    </button>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
      <div className="flex justify-end gap-2">
        <span className="mr-2 self-center text-sm text-slate-500">
          {loading ? '\u52a0\u8f7d\u4e2d...' : pageSummary(paged.page, paged.totalPages, paged.total)}
        </span>
        <button type="button" disabled={paged.page <= 1} onClick={() => setPage((value) => value - 1)} className="rounded-md border border-[#d7e0ed] px-4 py-1.5 text-sm disabled:opacity-40">上一页</button>
        <button type="button" disabled={paged.page >= paged.totalPages} onClick={() => setPage((value) => value + 1)} className="rounded-md border border-[#d7e0ed] px-4 py-1.5 text-sm disabled:opacity-40">下一页</button>
      </div>
    </section>
  )
}
