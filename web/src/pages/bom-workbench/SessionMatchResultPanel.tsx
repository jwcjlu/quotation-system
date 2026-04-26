import { useEffect, useMemo, useState } from 'react'
import { autoMatch, type MatchItem } from '../../api'
import {
  PAGE_SIZE_OPTIONS,
  type PageSize,
  pageSummary,
  paginateRows,
  textMatchesKeyword,
} from './sessionPanelUtils'

interface SessionMatchResultPanelProps {
  bomId: string
  onNavigateToHsResolve?: (model: string, manufacturer: string) => void
}

export function SessionMatchResultPanel({
  bomId,
  onNavigateToHsResolve,
}: SessionMatchResultPanelProps) {
  const [items, setItems] = useState<MatchItem[]>([])
  const [keyword, setKeyword] = useState('')
  const [status, setStatus] = useState('')
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState<PageSize>(50)

  useEffect(() => {
    let cancelled = false
    ;(async () => {
      try {
        const reply = await autoMatch(bomId)
        if (!cancelled) setItems(reply.items)
      } catch {
        if (!cancelled) setItems([])
      }
    })()
    return () => {
      cancelled = true
    }
  }, [bomId])

  const filtered = useMemo(
    () =>
      items.filter((item) => {
        if (status && item.match_status !== status) return false
        return textMatchesKeyword(
          [item.index, item.model, item.matched_model, item.manufacturer, item.platform, item.demand_manufacturer],
          keyword
        )
      }),
    [items, keyword, status]
  )
  const paged = paginateRows(filtered, page, pageSize)

  useEffect(() => {
    setPage(1)
  }, [keyword, status, pageSize])

  return (
    <section className="space-y-4" data-testid="session-match-result-panel">
      <div className="rounded-lg border border-slate-200 bg-white p-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h4 className="font-semibold text-slate-900">匹配结果</h4>
            <p className="mt-1 text-sm text-slate-500">查看候选报价、供应商和匹配状态</p>
          </div>
          <div className="text-sm text-slate-500">{pageSummary(paged.page, paged.totalPages, paged.total)}</div>
        </div>
        <div className="mt-4 grid gap-2 md:grid-cols-[minmax(0,1fr)_10rem_8rem]">
          <input value={keyword} onChange={(event) => setKeyword(event.target.value)} placeholder="MPN / 供应商 / 厂家" className="rounded border border-slate-300 px-3 py-2 text-sm" />
          <select value={status} onChange={(event) => setStatus(event.target.value)} className="rounded border border-slate-300 px-3 py-2 text-sm">
            <option value="">全部状态</option>
            <option value="exact">精确匹配</option>
            <option value="pending">待确认</option>
            <option value="no_match">无匹配</option>
          </select>
          <select value={pageSize} onChange={(event) => setPageSize(Number(event.target.value) as PageSize)} className="rounded border border-slate-300 px-3 py-2 text-sm">
            {PAGE_SIZE_OPTIONS.map((size) => <option key={size} value={size}>每页 {size}</option>)}
          </select>
        </div>
      </div>
      <div className="overflow-x-auto rounded-lg border border-slate-200 bg-white">
        <table className="w-full min-w-[820px] text-sm">
          <thead className="bg-slate-50 text-left text-slate-600">
            <tr>
              <th className="px-3 py-2">行号</th>
              <th className="px-3 py-2">需求型号</th>
              <th className="px-3 py-2">匹配型号</th>
              <th className="px-3 py-2">供应商</th>
              <th className="px-3 py-2">库存</th>
              <th className="px-3 py-2">单价</th>
              <th className="px-3 py-2">状态</th>
              <th className="px-3 py-2">操作</th>
            </tr>
          </thead>
          <tbody>
            {paged.rows.length === 0 ? (
              <tr><td colSpan={8} className="px-3 py-8 text-center text-slate-500">暂无匹配结果</td></tr>
            ) : (
              paged.rows.map((item) => (
                <tr key={`${item.index}-${item.model}-${item.platform}`} className="border-t border-slate-100">
                  <td className="px-3 py-2">{item.index}</td>
                  <td className="px-3 py-2 font-mono">{item.model}</td>
                  <td className="px-3 py-2 font-mono">{item.matched_model || '-'}</td>
                  <td className="px-3 py-2">{item.platform || item.manufacturer || '-'}</td>
                  <td className="px-3 py-2">{item.stock}</td>
                  <td className="px-3 py-2">{item.unit_price}</td>
                  <td className="px-3 py-2">{item.match_status || '-'}</td>
                  <td className="px-3 py-2">
                    <button type="button" onClick={() => onNavigateToHsResolve?.(item.model, item.manufacturer)} className="text-sm font-medium text-blue-600 hover:underline">HS</button>
                  </td>
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
