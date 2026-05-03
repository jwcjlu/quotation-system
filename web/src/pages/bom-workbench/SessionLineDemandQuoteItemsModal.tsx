import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { getBomLineQuoteItems, type BomLineQuoteItemsReply } from '../../api'
import {
  filterQuoteItems,
  type QuoteItemSortKey,
  sortQuoteItems,
} from './SessionLineDemandQuoteItemsModal.table'

function cellText(v: string, maxLen = 80): string {
  const t = v || ''
  if (t.length <= maxLen) return t
  return `${t.slice(0, maxLen)}…`
}

const SORT_LABELS: Record<QuoteItemSortKey, string> = {
  platform: '平台',
  model: '型号',
  mfr: '厂牌',
  pkg: '封装',
  stock: '库存',
  lead: '货期',
  mainland: '大陆价',
  hk: 'HK价',
}

export function SessionLineDemandQuoteItemsModal({
  bomId,
  lineNo,
  onClose,
}: {
  bomId: string
  lineNo: number
  onClose: () => void
}) {
  const [loading, setLoading] = useState(true)
  const [err, setErr] = useState<string | null>(null)
  const [data, setData] = useState<BomLineQuoteItemsReply | null>(null)
  const [keyword, setKeyword] = useState('')
  const [stockKeyword, setStockKeyword] = useState('')
  const [sortKey, setSortKey] = useState<QuoteItemSortKey | null>(null)
  const [sortDir, setSortDir] = useState<'asc' | 'desc'>('asc')

  useEffect(() => {
    let cancelled = false
    ;(async () => {
      setLoading(true)
      setErr(null)
      try {
        const reply = await getBomLineQuoteItems(bomId, lineNo)
        if (!cancelled) setData(reply)
      } catch (e) {
        if (!cancelled) setErr(e instanceof Error ? e.message : '加载失败')
      } finally {
        if (!cancelled) setLoading(false)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [bomId, lineNo])

  const d = data?.demand

  const displayItems = useMemo(() => {
    if (!data?.items.length) return []
    const filtered = filterQuoteItems(data.items, keyword, stockKeyword)
    return sortQuoteItems(filtered, sortKey, sortDir)
  }, [data, keyword, stockKeyword, sortKey, sortDir])

  function onSortClick(key: QuoteItemSortKey): void {
    if (sortKey === key) {
      setSortDir((prev) => (prev === 'asc' ? 'desc' : 'asc'))
    } else {
      setSortKey(key)
      setSortDir('asc')
    }
  }

  function sortHint(key: QuoteItemSortKey): string {
    if (sortKey !== key) return '点击排序'
    return sortDir === 'asc' ? '升序 · 再点降序' : '降序 · 再点升序'
  }

  function thSort(key: QuoteItemSortKey): ReactNode {
    const active = sortKey === key
    const arrow = active ? (sortDir === 'asc' ? ' ↑' : ' ↓') : ''
    return (
      <th className="px-2 py-2">
        <button
          type="button"
          title={sortHint(key)}
          onClick={() => onSortClick(key)}
          className={`font-medium hover:text-blue-800 ${active ? 'text-blue-700' : 'text-slate-700'}`}
        >
          {SORT_LABELS[key]}
          {arrow}
        </button>
      </th>
    )
  }

  return (
    <div className="fixed inset-0 z-[60] flex items-center justify-center bg-black/50" onClick={onClose}>
      <div
        className="max-h-[92vh] w-full max-w-5xl overflow-hidden rounded-lg bg-white shadow-xl"
        onClick={(event) => event.stopPropagation()}
      >
        <div className="flex items-center justify-between border-b border-slate-200 px-4 py-3">
          <div>
            <h3 className="text-lg font-semibold text-slate-900">原始需求与报价子表</h3>
            <p className="text-xs text-slate-500">
              行 {lineNo}
              {data?.biz_date ? ` · 业务日 ${data.biz_date}` : ''}
              {data?.merge_mpn ? ` · merge_mpn ${data.merge_mpn}` : ''}
            </p>
          </div>
          <button type="button" onClick={onClose} className="text-slate-500 hover:text-slate-700">
            ✕
          </button>
        </div>

        <div className="max-h-[82vh] overflow-auto p-4">
          {loading && <div className="text-sm text-slate-500">加载中…</div>}
          {err && <div className="rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{err}</div>}

          {!loading && !err && d ? (
            <div className="space-y-6">
              <section>
                <h4 className="mb-2 text-sm font-semibold text-slate-800">原始需求（t_bom_session_line）</h4>
                <dl className="grid gap-2 rounded border border-slate-200 bg-slate-50/80 p-3 text-xs sm:grid-cols-2">
                  <div>
                    <dt className="text-slate-500">line_no / line_db_id</dt>
                    <dd className="font-mono text-slate-900">
                      {d.line_no} / {d.line_db_id}
                    </dd>
                  </div>
                  <div>
                    <dt className="text-slate-500">mpn</dt>
                    <dd className="font-mono text-slate-900">{d.mpn || '-'}</dd>
                  </div>
                  <div>
                    <dt className="text-slate-500">unified_mpn</dt>
                    <dd className="font-mono text-slate-900">{d.unified_mpn || '-'}</dd>
                  </div>
                  <div>
                    <dt className="text-slate-500">qty</dt>
                    <dd className="text-slate-900">{d.quantity != null ? String(d.quantity) : '-'}</dd>
                  </div>
                  <div>
                    <dt className="text-slate-500">mfr / canonical</dt>
                    <dd className="text-slate-900">
                      {d.demand_manufacturer || '-'} / {d.manufacturer_canonical_id || '-'}
                    </dd>
                  </div>
                  <div>
                    <dt className="text-slate-500">package</dt>
                    <dd className="text-slate-900">{d.demand_package || '-'}</dd>
                  </div>
                  <div className="sm:col-span-2">
                    <dt className="text-slate-500">raw_text</dt>
                    <dd className="whitespace-pre-wrap break-all text-slate-900">{d.raw_text || '(空)'}</dd>
                  </div>
                  <div>
                    <dt className="text-slate-500">reference_designator</dt>
                    <dd className="text-slate-900">{d.reference_designator || '-'}</dd>
                  </div>
                  <div>
                    <dt className="text-slate-500">substitute_mpn</dt>
                    <dd className="font-mono text-slate-900">{d.substitute_mpn || '-'}</dd>
                  </div>
                  <div className="sm:col-span-2">
                    <dt className="text-slate-500">remark</dt>
                    <dd className="whitespace-pre-wrap text-slate-900">{d.remark || '-'}</dd>
                  </div>
                  <div className="sm:col-span-2">
                    <dt className="text-slate-500">description</dt>
                    <dd className="whitespace-pre-wrap text-slate-900">{d.description || '-'}</dd>
                  </div>
                  <div className="sm:col-span-2">
                    <dt className="text-slate-500">extra_json</dt>
                    <dd className="max-h-32 overflow-auto whitespace-pre-wrap break-all font-mono text-slate-800">
                      {d.extra_json || '(空)'}
                    </dd>
                  </div>
                </dl>
              </section>

              <section>
                <h4 className="mb-2 text-sm font-semibold text-slate-800">
                  报价子表明细（t_bom_quote_item — 平台 / 型号 / 厂牌 / 封装 / 库存 / 货期 / 大陆价 / HK价）
                </h4>
                {data.items.length === 0 ? (
                  <div className="text-sm text-slate-500">暂无子表行。</div>
                ) : (
                  <div className="space-y-2">
                    <div className="flex flex-wrap items-end gap-2 text-xs">
                      <div className="min-w-[200px] flex-1">
                        <label className="mb-0.5 block text-slate-500">全文搜索</label>
                        <input
                          value={keyword}
                          onChange={(e) => setKeyword(e.target.value)}
                          placeholder="平台、型号、厂牌、封装、货期、价格…"
                          className="h-8 w-full rounded border border-slate-300 px-2 text-sm"
                        />
                      </div>
                      <div className="min-w-[160px]">
                        <label className="mb-0.5 block text-slate-500">库存（≥）</label>
                        <input
                          value={stockKeyword}
                          onChange={(e) => setStockKeyword(e.target.value)}
                          placeholder="数字：库存数值 ≥ 输入；非数字则子串匹配"
                          className="h-8 w-full rounded border border-slate-300 px-2 text-sm"
                        />
                      </div>
                      <button
                        type="button"
                        className="h-8 rounded border border-slate-300 px-2 text-sm text-slate-600 hover:bg-slate-50"
                        onClick={() => {
                          setKeyword('')
                          setStockKeyword('')
                          setSortKey(null)
                          setSortDir('asc')
                        }}
                      >
                        重置筛选/排序
                      </button>
                      <span className="pb-1 text-slate-500">
                        显示 {displayItems.length} / {data.items.length}
                      </span>
                    </div>
                    <div className="overflow-x-auto rounded border border-slate-200">
                      <table className="w-full min-w-[720px] text-left text-xs">
                        <thead className="bg-slate-100">
                          <tr>
                            {thSort('platform')}
                            {thSort('model')}
                            {thSort('mfr')}
                            {thSort('pkg')}
                            {thSort('stock')}
                            {thSort('lead')}
                            {thSort('mainland')}
                            {thSort('hk')}
                          </tr>
                        </thead>
                        <tbody>
                          {displayItems.map((it) => (
                            <tr key={it.item_id} className="border-t border-slate-100 align-top">
                              <td className="px-2 py-1.5 font-medium">{it.platform || '-'}</td>
                              <td className="px-2 py-1.5 font-mono" title={it.model}>
                                {cellText(it.model, 40)}
                              </td>
                              <td className="px-2 py-1.5" title={it.manufacturer}>
                                {cellText(it.manufacturer, 28)}
                              </td>
                              <td className="px-2 py-1.5">{cellText(it.package, 20)}</td>
                              <td className="px-2 py-1.5 font-mono" title={it.stock}>
                                {cellText(it.stock, 16)}
                              </td>
                              <td className="px-2 py-1.5">{cellText(it.lead_time, 16)}</td>
                              <td className="px-2 py-1.5 max-w-[180px]" title={it.mainland_price}>
                                {cellText(it.mainland_price, 36)}
                              </td>
                              <td className="px-2 py-1.5 max-w-[180px]" title={it.hk_price}>
                                {cellText(it.hk_price, 36)}
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  </div>
                )}
              </section>
            </div>
          ) : null}
        </div>
      </div>
    </div>
  )
}
