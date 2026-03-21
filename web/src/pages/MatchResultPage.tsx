import { useState, useEffect } from 'react'
import { getMatchResult, autoMatch, searchQuotes, type MatchItem, type PlatformQuote } from '../api'

interface MatchResultPageProps {
  bomId: string
}

const STATUS_OPTIONS = [
  { value: 'all', label: '全部' },
  { value: 'exact', label: '完全匹配' },
  { value: 'pending', label: '待确认' },
  { value: 'no_match', label: '无法匹配' },
] as const

const STRATEGY_OPTIONS = [
  { value: 'price_first', label: '价格优先' },
  { value: 'stock_first', label: '库存优先' },
  { value: 'leadtime_first', label: '货期优先' },
  { value: 'comprehensive', label: '综合排序' },
] as const

function StatusIcon({ status }: { status: string }) {
  if (status === 'exact') return <span className="text-green-600 font-bold">✓</span>
  if (status === 'pending') return <span className="text-amber-600 font-bold">!</span>
  if (status === 'no_match') return <span className="text-red-600 font-bold">✗</span>
  return null
}

function QuoteRow({ q, isSelected }: { q: PlatformQuote; isSelected?: boolean }) {
  return (
    <tr className={`border-b border-slate-100 hover:bg-slate-50 ${isSelected ? 'bg-blue-50' : ''}`}>
      <td className="py-2 px-3">{q.platform}</td>
      <td className="py-2 px-3">{q.matched_model}</td>
      <td className="py-2 px-3">{q.manufacturer}</td>
      <td className="py-2 px-3">{q.package || '-'}</td>
      <td className="py-2 px-3">{q.stock}</td>
      <td className="py-2 px-3">{q.lead_time}</td>
      <td className="py-2 px-3">{q.price_tiers || '-'}</td>
      <td className="py-2 px-3">¥{q.unit_price?.toFixed(2) ?? '-'}</td>
    </tr>
  )
}

function DetailModal({ item, onClose }: { item: MatchItem; onClose: () => void }) {
  const byPlatform = (item.all_quotes || []).reduce<Record<string, PlatformQuote[]>>((acc, q) => {
    const p = q.platform || '未知'
    if (!acc[p]) acc[p] = []
    acc[p].push(q)
    return acc
  }, {})
  const isSelected = (q: PlatformQuote) =>
    q.platform === item.platform && q.matched_model === item.matched_model

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={onClose}>
      <div
        className="max-h-[85vh] w-full max-w-4xl overflow-auto rounded-lg bg-white p-6 shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="mb-4 flex justify-between">
          <h3 className="text-lg font-bold text-slate-800">
            查看详情 - {item.model} {item.demand_manufacturer && `(${item.demand_manufacturer})`}
          </h3>
          <button onClick={onClose} className="text-slate-500 hover:text-slate-700">✕</button>
        </div>
        {item.matched_model && (
          <div className="mb-4 rounded-lg border border-blue-200 bg-blue-50 p-4">
            <h4 className="mb-2 font-medium text-slate-700">当前匹配</h4>
            <div className="grid grid-cols-2 gap-x-6 gap-y-1 text-sm sm:grid-cols-4">
              <span className="text-slate-500">平台</span>
              <span>{item.platform || '-'}</span>
              <span className="text-slate-500">型号</span>
              <span>{item.matched_model}</span>
              <span className="text-slate-500">厂牌</span>
              <span>{item.manufacturer || '-'}</span>
              <span className="text-slate-500">封装</span>
              <span>{item.demand_package || '-'}</span>
              <span className="text-slate-500">库存</span>
              <span>{item.stock ?? '-'}</span>
              <span className="text-slate-500">货期</span>
              <span>{item.lead_time || '-'}</span>
              <span className="text-slate-500">单价</span>
              <span>¥{item.unit_price?.toFixed(2) ?? '-'}</span>
            </div>
          </div>
        )}
        <p className="mb-4 text-sm text-slate-600">
          各平台搜索到的全部报价，当前选中已高亮
        </p>
        <div className="space-y-4">
          {Object.entries(byPlatform).map(([platform, quotes]) => (
            <div key={platform}>
              <h4 className="mb-2 font-medium text-slate-700">{platform}</h4>
              <table className="w-full text-sm">
                <thead>
                  <tr className="bg-slate-100">
                    <th className="py-2 px-3 text-left">平台</th>
                    <th className="py-2 px-3 text-left">型号</th>
                    <th className="py-2 px-3 text-left">厂牌</th>
                    <th className="py-2 px-3 text-left">封装</th>
                    <th className="py-2 px-3 text-left">库存</th>
                    <th className="py-2 px-3 text-left">货期</th>
                    <th className="py-2 px-3 text-left">价格梯度</th>
                    <th className="py-2 px-3 text-left">单价</th>
                  </tr>
                </thead>
                <tbody>
                  {quotes.map((q, i) => (
                    <QuoteRow key={i} q={q} isSelected={isSelected(q)} />
                  ))}
                </tbody>
              </table>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}

function MatchRow({ item, statusFilter, onShowDetail }: { item: MatchItem; statusFilter: string; onShowDetail: (item: MatchItem) => void }) {
  const [expanded, setExpanded] = useState(false)
  const show = statusFilter === 'all' || item.match_status === statusFilter
  if (!show) return null

  const hasQuotes = item.all_quotes && item.all_quotes.length > 0

  return (
    <>
      <tr className="border-b border-slate-200 hover:bg-slate-50">
        <td className="py-3 px-3">{item.index}</td>
        <td className="py-3 px-3 text-slate-800">{item.model}</td>
        <td className="py-3 px-3">{item.demand_manufacturer || '-'}</td>
        <td className="py-3 px-3">{item.demand_package || '-'}</td>
        <td className="py-3 px-3">{item.quantity}</td>
        <td className="py-3 px-3">
          <StatusIcon status={item.match_status} />
        </td>
        <td className="py-3 px-3">
          <div className="flex flex-col gap-0.5">
            <span className="font-medium">{item.matched_model || '-'}</span>
            {(item.manufacturer || item.platform) && (
              <span className="text-slate-500 text-sm">
                {item.manufacturer}{item.manufacturer && item.platform ? ' · ' : ''}{item.platform}
              </span>
            )}
            {hasQuotes && (
              <button
                onClick={() => setExpanded(!expanded)}
                className="text-blue-600 text-sm hover:underline"
              >
                {expanded ? '收起' : '显示更多'}
              </button>
            )}
          </div>
        </td>
        <td className="py-3 px-3">{item.stock ?? '-'}</td>
        <td className="py-3 px-3">{item.lead_time || '-'}</td>
        <td className="py-3 px-3">¥{item.unit_price?.toFixed(2) ?? '-'}</td>
        <td className="py-3 px-3 font-medium">¥{item.subtotal?.toFixed(2) ?? '-'}</td>
        <td className="py-3 px-3">
          {hasQuotes && (
            <button
              onClick={() => onShowDetail(item)}
              className="text-blue-600 text-sm hover:underline"
            >
              查看详情
            </button>
          )}
        </td>
      </tr>
      {expanded && hasQuotes && (
        <tr>
          <td colSpan={12} className="bg-slate-50 p-0 align-top">
            <div className="py-2" style={{ paddingLeft: '47%' }}>
              <table className="w-full min-w-[600px] text-sm">
                <colgroup>
                  <col style={{ width: '10%' }} />
                  <col style={{ width: '14%' }} />
                  <col style={{ width: '12%' }} />
                  <col style={{ width: '12%' }} />
                  <col style={{ width: '10%' }} />
                  <col style={{ width: '12%' }} />
                  <col style={{ width: '20%' }} />
                  <col style={{ width: '10%' }} />
                </colgroup>
                <thead>
                  <tr className="bg-slate-100">
                    <th className="py-2 px-3 text-left">平台</th>
                    <th className="py-2 px-3 text-left">型号</th>
                    <th className="py-2 px-3 text-left">厂牌</th>
                    <th className="py-2 px-3 text-left">封装</th>
                    <th className="py-2 px-3 text-left">库存</th>
                    <th className="py-2 px-3 text-left">货期</th>
                    <th className="py-2 px-3 text-left">价格梯度</th>
                    <th className="py-2 px-3 text-left">单价</th>
                  </tr>
                </thead>
                <tbody>
                  {item.all_quotes!.map((q, i) => (
                    <QuoteRow key={i} q={q} isSelected={q.platform === item.platform && q.matched_model === item.matched_model} />
                  ))}
                </tbody>
              </table>
            </div>
          </td>
        </tr>
      )}
    </>
  )
}

export function MatchResultPage({ bomId }: MatchResultPageProps) {
  const [items, setItems] = useState<MatchItem[]>([])
  const [totalAmount, setTotalAmount] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [statusFilter, setStatusFilter] = useState('all')
  const [strategy, setStrategy] = useState('price_first')
  const [running, setRunning] = useState(false)
  const [detailItem, setDetailItem] = useState<MatchItem | null>(null)

  const load = async () => {
    setLoading(true)
    setError(null)
    try {
      const res = await getMatchResult(bomId)
      setItems(res.items || [])
      setTotalAmount(res.total_amount || 0)
    } catch {
      setItems([])
      setTotalAmount(0)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
  }, [bomId])

  const runSearchAndMatch = async () => {
    setRunning(true)
    setError(null)
    try {
      await searchQuotes(bomId)
      const res = await autoMatch(bomId, strategy)
      setItems(res.items || [])
      setTotalAmount(res.total_amount || 0)
    } catch (e) {
      setError(e instanceof Error ? e.message : '搜索或配单失败')
    } finally {
      setRunning(false)
    }
  }

  if (loading) {
    return (
      <div className="flex justify-center py-20">
        <div className="text-slate-500">加载中...</div>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <h2 className="text-2xl font-bold text-slate-800">配单结果</h2>
        <div className="flex flex-wrap items-center gap-4">
          <select
            value={strategy}
            onChange={(e) => setStrategy(e.target.value)}
            className="border border-slate-300 rounded px-3 py-2"
          >
            {STRATEGY_OPTIONS.map((s) => (
              <option key={s.value} value={s.value}>
                {s.label}
              </option>
            ))}
          </select>
          <button
            onClick={runSearchAndMatch}
            disabled={running}
            className="px-4 py-2 bg-blue-600 text-white rounded-lg font-medium hover:bg-blue-700 disabled:opacity-50"
          >
            {running ? '搜索配单中...' : '重新搜索并配单'}
          </button>
        </div>
      </div>

      <div className="flex justify-between items-center">
        <div className="flex flex-wrap items-center gap-2">
          {STATUS_OPTIONS.map((s) => {
            const count = s.value === 'all' ? items.length : items.filter((i) => i.match_status === s.value).length
            return (
              <button
                key={s.value}
                onClick={() => setStatusFilter(s.value)}
                className={`px-3 py-1 rounded ${statusFilter === s.value ? 'bg-slate-600 text-white' : 'bg-slate-200 text-slate-700 hover:bg-slate-300'}`}
              >
                {s.label}({count})
              </button>
            )
          })}
        </div>
        <div className="text-slate-600">
          合计: <span className="font-bold text-slate-800">¥{totalAmount.toFixed(2)}</span>
        </div>
      </div>

      {error && (
        <div className="p-4 bg-red-50 text-red-700 rounded-lg">{error}</div>
      )}

      <div className="p-4 bg-amber-50 text-amber-800 rounded-lg text-sm">
        价格/库存可能波动，以结算为准
      </div>

      <div className="overflow-x-auto rounded-lg border border-slate-200 bg-white">
        <table className="w-full table-fixed" style={{ minWidth: 900 }}>
          <colgroup>
            <col style={{ width: '4%' }} />
            <col style={{ width: '12%' }} />
            <col style={{ width: '10%' }} />
            <col style={{ width: '10%' }} />
            <col style={{ width: '6%' }} />
            <col style={{ width: '5%' }} />
            <col style={{ width: '18%' }} />
            <col style={{ width: '8%' }} />
            <col style={{ width: '10%' }} />
            <col style={{ width: '8%' }} />
            <col style={{ width: '5%' }} />
            <col style={{ width: '9%' }} />
          </colgroup>
          <thead>
            <tr className="bg-slate-100">
              <th className="py-3 px-3 text-left">序号</th>
              <th className="py-3 px-3 text-left">需求型号</th>
              <th className="py-3 px-3 text-left">厂牌</th>
              <th className="py-3 px-3 text-left">封装</th>
              <th className="py-3 px-3 text-left">数量</th>
              <th className="py-3 px-3 text-left">结果</th>
              <th className="py-3 px-3 text-left">推荐最优型号</th>
              <th className="py-3 px-3 text-left">库存</th>
              <th className="py-3 px-3 text-left">货期</th>
              <th className="py-3 px-3 text-left">单价</th>
              <th className="py-3 px-3 text-left">小计</th>
              <th className="py-3 px-3 text-left">操作</th>
            </tr>
          </thead>
          <tbody>
            {items.length === 0 ? (
              <tr>
                <td colSpan={12} className="py-12 text-center text-slate-500">
                  暂无配单结果，请点击「重新搜索并配单」获取报价
                </td>
              </tr>
            ) : (
              items.map((item) => (
                <MatchRow key={item.index} item={item} statusFilter={statusFilter} onShowDetail={setDetailItem} />
              ))
            )}
          </tbody>
        </table>
      </div>
      {detailItem && (
        <DetailModal item={detailItem} onClose={() => setDetailItem(null)} />
      )}
    </div>
  )
}
