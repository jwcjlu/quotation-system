import { useState, useEffect, useMemo } from 'react'
import {
  getMatchResult,
  getSession,
  autoMatch,
  createManufacturerAlias,
  listManufacturerCanonicals,
  listMatchSourceRecords,
  getMatchSourceDetail,
  parseAgentQuoteRowsFromCache,
  normalizeMPNForBomSearchClient,
  normalizeMfrStringClient,
  matchItemNeedsHsResolve,
  hsBatchResolveByModels,
  type ManufacturerCanonicalRow,
  type MatchItem,
  type PlatformQuote,
  type AgentQuoteRow,
  type QuoteRowMatchEval,
} from '../api'

interface MatchResultPageProps {
  bomId: string
  /** 配单行「一键找 HS」：跳转 HS 型号解析页并预填 */
  onNavigateToHsResolve?: (model: string, manufacturer: string) => void
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

/** 与货源会话状态一致：仅此时允许调用配单相关接口 */
const SESSION_MATCH_READY = 'data_ready'

/** 与后端 biz.MfrMismatchEmptyPlaceholder 一致 */
const MFR_PLACEHOLDER = '(报价厂牌为空)'

const MATCH_RESULT_VISIBLE_COLS_KEY = 'match_result_visible_cols_v1'
const OPTIONAL_COLUMNS = [
  { key: 'index', label: '序号' },
  { key: 'model', label: '需求型号' },
  { key: 'manufacturer', label: '厂牌' },
  { key: 'package', label: '封装' },
  { key: 'quantity', label: '数量' },
  { key: 'matchStatus', label: '结果' },
  { key: 'bestModel', label: '推荐最优型号' },
  { key: 'stock', label: '库存' },
  { key: 'leadTime', label: '货期' },
  { key: 'unitPrice', label: '单价' },
  { key: 'subtotal', label: '小计' },
  { key: 'hs', label: 'HS' },
  { key: 'control', label: '商检' },
  { key: 'ordinaryTax', label: '普通税率' },
  { key: 'discountTax', label: '最惠税率' },
  { key: 'tempTax', label: '暂定税率' },
] as const
type OptionalColumnKey = (typeof OPTIONAL_COLUMNS)[number]['key']
const DEFAULT_VISIBLE_OPTIONAL_COLUMNS: Record<OptionalColumnKey, boolean> = {
  index: true,
  model: true,
  manufacturer: true,
  package: true,
  quantity: true,
  matchStatus: true,
  bestModel: true,
  stock: true,
  leadTime: true,
  unitPrice: true,
  subtotal: true,
  hs: true,
  control: true,
  ordinaryTax: true,
  discountTax: true,
  tempTax: true,
}

function hsStatusLabel(status: string | undefined): string {
  const s = (status || '').trim()
  if (s === 'hs_found') return '已匹配'
  if (s === 'hs_not_mapped') return '未找到HS'
  if (s === 'hs_code_invalid') return 'HS编码异常'
  return s ? s : '—'
}

type PendingMfrRow = { alias: string; lineIndexes: number[]; demandHint: string }

function collectPendingMfrRows(items: MatchItem[]): PendingMfrRow[] {
  const map = new Map<string, { lines: Set<number>; demand: Set<string> }>()
  for (const it of items) {
    const arr = it.mfr_mismatch_quote_manufacturers
    if (!arr?.length) continue
    const dm = (it.demand_manufacturer || '').trim()
    for (const raw of arr) {
      const alias = (raw || '').trim()
      if (!alias) continue
      let g = map.get(alias)
      if (!g) {
        g = { lines: new Set(), demand: new Set() }
        map.set(alias, g)
      }
      g.lines.add(it.index)
      if (dm) g.demand.add(dm)
    }
  }
  return Array.from(map.entries()).map(([alias, g]) => ({
    alias,
    lineIndexes: [...g.lines].sort((a, b) => a - b),
    demandHint: [...g.demand].join('；') || '—',
  }))
}

function ManualManufacturerAliasForm({
  canonicalRows,
  onSuccess,
}: {
  canonicalRows: ManufacturerCanonicalRow[]
  onSuccess: () => void
}) {
  const [alias, setAlias] = useState('')
  const [canonicalId, setCanonicalId] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [busy, setBusy] = useState(false)
  const [msg, setMsg] = useState<string | null>(null)

  return (
    <div className="space-y-3 text-sm max-w-3xl">
      <datalist id="bom-mfr-canonical-datalist-manual">
        {canonicalRows.map((c) => (
          <option key={c.canonical_id} value={c.canonical_id}>
            {c.display_name}
          </option>
        ))}
      </datalist>
      <div className="flex flex-wrap gap-3 items-end">
        <div className="min-w-[160px] flex-1">
          <label className="block text-xs text-slate-500 mb-0.5">报价厂牌原文（alias）</label>
          <input
            value={alias}
            onChange={(e) => setAlias(e.target.value)}
            className="w-full border border-slate-300 rounded px-2 py-1.5 text-sm"
            placeholder="如 TI / 德州仪器"
          />
        </div>
        <div className="w-44">
          <label className="block text-xs text-slate-500 mb-0.5">canonical_id</label>
          <input
            list="bom-mfr-canonical-datalist-manual"
            value={canonicalId}
            onChange={(e) => setCanonicalId(e.target.value)}
            className="w-full border border-slate-300 rounded px-2 py-1.5 text-sm"
            placeholder="如 MFR_TI"
          />
        </div>
        <div className="w-48">
          <label className="block text-xs text-slate-500 mb-0.5">display_name</label>
          <input
            value={displayName}
            onChange={(e) => setDisplayName(e.target.value)}
            className="w-full border border-slate-300 rounded px-2 py-1.5 text-sm"
            placeholder="展示名"
          />
        </div>
        <button
          type="button"
          disabled={busy || !alias.trim() || !canonicalId.trim() || !displayName.trim()}
          onClick={async () => {
            setBusy(true)
            setMsg(null)
            try {
              await createManufacturerAlias(alias, canonicalId.trim(), displayName.trim())
              setAlias('')
              setCanonicalId('')
              setDisplayName('')
              onSuccess()
            } catch (e) {
              setMsg(e instanceof Error ? e.message : '提交失败')
            } finally {
              setBusy(false)
            }
          }}
          className="px-4 py-2 rounded-lg bg-slate-800 text-white text-sm font-medium hover:bg-slate-900 disabled:opacity-50"
        >
          {busy ? '提交中…' : '写入别名'}
        </button>
      </div>
      {msg && <div className="text-xs text-red-700 bg-red-50 border border-red-200 rounded px-3 py-2">{msg}</div>}
    </div>
  )
}

function MfrAliasReviewPanel({
  pendingRows,
  onApproved,
  canonicalRows,
}: {
  pendingRows: PendingMfrRow[]
  onApproved: (alias: string) => void
  canonicalRows: ManufacturerCanonicalRow[]
}) {
  const [draft, setDraft] = useState<Record<string, { canonicalId: string; displayName: string }>>({})
  const [busyAlias, setBusyAlias] = useState<string | null>(null)
  const [msg, setMsg] = useState<string | null>(null)

  useEffect(() => {
    setDraft((prev) => {
      const next = { ...prev }
      for (const r of pendingRows) {
        if (!next[r.alias]) {
          next[r.alias] = { canonicalId: '', displayName: r.alias }
        }
      }
      return next
    })
  }, [pendingRows])

  return (
    <div className="p-4 space-y-3 bg-amber-50/40">
      <p className="text-sm text-amber-900/90">
        以下字符串来自配单时「型号/封装已对齐、但与需求厂牌不一致」的报价原文。填写规范 ID 与展示名后点击通过，将写入{' '}
        <code className="text-xs bg-white/80 px-1 rounded">t_bom_manufacturer_alias</code>；之后请重新配单生效。
      </p>
      <datalist id="bom-mfr-canonical-datalist">
        {canonicalRows.map((c) => (
          <option key={c.canonical_id} value={c.canonical_id}>
            {c.display_name}
          </option>
        ))}
      </datalist>
      {msg && <div className="text-sm text-red-700 bg-red-50 border border-red-200 rounded px-3 py-2">{msg}</div>}
      <div className="space-y-3">
        {pendingRows.map((r) => {
          const st = draft[r.alias] ?? { canonicalId: '', displayName: r.alias }
          const isPlaceholder = r.alias === MFR_PLACEHOLDER
          return (
            <div
              key={r.alias}
              className="flex flex-wrap items-end gap-3 rounded-md border border-amber-200/80 bg-white/70 p-3 text-sm"
            >
              <div className="min-w-[200px] flex-1">
                <div className="text-xs text-slate-500 mb-0.5">报价厂牌原文</div>
                <div className="font-medium text-slate-800 break-all">{r.alias}</div>
                <div className="text-xs text-slate-500 mt-1">
                  行号 {r.lineIndexes.join(', ')} · 需求厂牌参考 {r.demandHint}
                </div>
              </div>
              <div className="w-44">
                <label className="block text-xs text-slate-500 mb-0.5">canonical_id</label>
                <input
                  list="bom-mfr-canonical-datalist"
                  value={st.canonicalId}
                  disabled={isPlaceholder}
                  onChange={(e) =>
                    setDraft((d) => ({
                      ...d,
                      [r.alias]: { ...st, canonicalId: e.target.value },
                    }))
                  }
                  placeholder="如 MFR_TI"
                  className="w-full border border-slate-300 rounded px-2 py-1.5 text-sm disabled:opacity-50"
                />
              </div>
              <div className="w-48">
                <label className="block text-xs text-slate-500 mb-0.5">display_name</label>
                <input
                  value={st.displayName}
                  disabled={isPlaceholder}
                  onChange={(e) =>
                    setDraft((d) => ({
                      ...d,
                      [r.alias]: { ...st, displayName: e.target.value },
                    }))
                  }
                  className="w-full border border-slate-300 rounded px-2 py-1.5 text-sm disabled:opacity-50"
                />
              </div>
              <button
                type="button"
                disabled={isPlaceholder || busyAlias !== null || !st.canonicalId.trim() || !st.displayName.trim()}
                onClick={async () => {
                  setMsg(null)
                  setBusyAlias(r.alias)
                  try {
                    await createManufacturerAlias(r.alias, st.canonicalId.trim(), st.displayName.trim())
                    onApproved(r.alias)
                  } catch (e) {
                    setMsg(e instanceof Error ? e.message : '写入失败')
                  } finally {
                    setBusyAlias(null)
                  }
                }}
                className="px-3 py-1.5 rounded-lg bg-emerald-600 text-white text-sm font-medium hover:bg-emerald-700 disabled:opacity-45"
              >
                {busyAlias === r.alias ? '提交中…' : '审核通过'}
              </button>
              {isPlaceholder && (
                <span className="text-xs text-slate-500">占位项表示报价缺厂牌，请从数据源修正，无法在此入库。</span>
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}

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

function localModelTrial(demandMpn: string, row: AgentQuoteRow): { ok: boolean; reason: string } {
  const qm = row.model.trim()
  if (!qm) return { ok: false, reason: '报价型号为空' }
  const a = normalizeMPNForBomSearchClient(demandMpn)
  const b = normalizeMPNForBomSearchClient(row.model)
  if (a !== b) return { ok: false, reason: `归一化键 需求 ${a} ≠ 报价 ${b}` }
  return { ok: true, reason: '归一化型号与当前试算需求一致' }
}

function localPackageTrial(demandPkg: string, row: AgentQuoteRow): { ok: boolean; reason: string } {
  const qm = row.model.trim()
  const d = demandPkg.trim()
  if (d === '') return { ok: true, reason: '试算需求未填封装，不校验' }
  if (!qm) return { ok: false, reason: '报价型号为空，跳过封装比对' }
  const a = normalizeMfrStringClient(demandPkg)
  const b = normalizeMfrStringClient(row.package)
  if (a !== b) return { ok: false, reason: `归一化 需求 ${a} ≠ 报价 ${b}` }
  return { ok: true, reason: '归一化封装与试算需求一致' }
}

function evalMapFromList(evals: QuoteRowMatchEval[]): Map<number, QuoteRowMatchEval> {
  const m = new Map<number, QuoteRowMatchEval>()
  for (const e of evals) m.set(e.row_index, e)
  return m
}

function MatchSourceDetailModal({
  bomId,
  lineNo,
  platform,
  titleHint,
  onClose,
}: {
  bomId: string
  lineNo: number
  platform: string
  titleHint: string
  onClose: () => void
}) {
  const [loading, setLoading] = useState(true)
  const [err, setErr] = useState<string | null>(null)
  const [data, setData] = useState<Awaited<ReturnType<typeof getMatchSourceDetail>> | null>(null)
  const [demandMpn, setDemandMpn] = useState('')
  const [demandPkg, setDemandPkg] = useState('')
  const [demandMfr, setDemandMfr] = useState('')
  const [rowFilter, setRowFilter] = useState('')

  useEffect(() => {
    let cancelled = false
    ;(async () => {
      setLoading(true)
      setErr(null)
      try {
        const d = await getMatchSourceDetail(bomId, lineNo, platform)
        if (!cancelled) {
          setData(d)
          setDemandMpn(d.bom_demand_mpn)
          setDemandPkg(d.bom_demand_package)
          setDemandMfr(d.bom_demand_manufacturer)
        }
      } catch (e) {
        if (!cancelled) setErr(e instanceof Error ? e.message : '加载失败')
      } finally {
        if (!cancelled) setLoading(false)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [bomId, lineNo, platform])

  const quoteRows = useMemo(() => (data ? parseAgentQuoteRowsFromCache(data.quotes_json) : []), [data])
  const serverEvalByRow = useMemo(
    () => (data ? evalMapFromList(data.quote_row_evals) : new Map<number, QuoteRowMatchEval>()),
    [data]
  )

  const demandMatchesSnapshot =
    data != null &&
    demandMpn === data.bom_demand_mpn &&
    demandPkg === data.bom_demand_package &&
    demandMfr === data.bom_demand_manufacturer

  const filteredRows = useMemo(() => {
    const q = rowFilter.trim().toLowerCase()
    if (!q) return quoteRows.map((row, i) => ({ row, i }))
    return quoteRows
      .map((row, i) => ({ row, i }))
      .filter(
        ({ row }) =>
          row.model.toLowerCase().includes(q) ||
          row.manufacturer.toLowerCase().includes(q) ||
          row.package.toLowerCase().includes(q)
      )
  }, [quoteRows, rowFilter])

  return (
    <div className="fixed inset-0 z-[60] flex items-center justify-center bg-black/40" onClick={onClose}>
      <div
        className="max-h-[90vh] w-full max-w-6xl overflow-hidden flex flex-col rounded-lg bg-white shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex justify-between items-start gap-3 px-5 py-4 border-b border-slate-200 shrink-0">
          <div>
            <h3 className="text-lg font-bold text-slate-800">缓存明细 · 报价表</h3>
            <p className="text-sm text-slate-500 mt-1">{titleHint}</p>
          </div>
          <button type="button" onClick={onClose} className="text-slate-500 hover:text-slate-700 shrink-0">
            ✕
          </button>
        </div>
        <div className="overflow-auto flex-1 p-5 text-sm min-h-0">
          {loading && <div className="text-slate-500">加载中…</div>}
          {err && <div className="text-red-600 bg-red-50 border border-red-200 rounded px-3 py-2">{err}</div>}
          {!loading && !err && data && (
            <div className="space-y-4">
              <div className="grid grid-cols-2 sm:grid-cols-4 gap-x-3 gap-y-1 text-xs">
                <span className="text-slate-500">merge_mpn</span>
                <span className="break-all font-mono col-span-1 sm:col-span-3">{data.merge_mpn || '—'}</span>
                <span className="text-slate-500">platform</span>
                <span>{data.platform}</span>
                <span className="text-slate-500">cache / skip</span>
                <span>
                  {data.cache_hit ? '命中' : '未命中'} · {data.skip_reason || '—'}
                </span>
                <span className="text-slate-500">outcome</span>
                <span className="break-all">{data.outcome || '—'}</span>
              </div>

              <div className="rounded-lg border border-slate-200 bg-slate-50/80 p-3 space-y-2">
                <div className="text-xs font-medium text-slate-700">试算需求（型号 / 封装可即时重算；厂牌依赖别名表，仅在与打开时快照一致时显示服务端判定）</div>
                {!demandMatchesSnapshot ? (
                  <p className="text-xs text-amber-800 bg-amber-50 border border-amber-200 rounded px-2 py-1.5">
                    已修改试算字段：型号/封装列为动态结果；「厂牌」「整行通过」仍以打开本窗口时的服务端快照为准，完整刷新请关闭后再次点「查看详情」。
                  </p>
                ) : null}
                <div className="grid grid-cols-1 sm:grid-cols-3 gap-2">
                  <label className="block text-xs">
                    <span className="text-slate-500">需求型号</span>
                    <input
                      value={demandMpn}
                      onChange={(e) => setDemandMpn(e.target.value)}
                      className="mt-0.5 w-full border border-slate-300 rounded px-2 py-1 text-sm font-mono"
                    />
                  </label>
                  <label className="block text-xs">
                    <span className="text-slate-500">需求封装</span>
                    <input
                      value={demandPkg}
                      onChange={(e) => setDemandPkg(e.target.value)}
                      className="mt-0.5 w-full border border-slate-300 rounded px-2 py-1 text-sm"
                    />
                  </label>
                  <label className="block text-xs">
                    <span className="text-slate-500">需求厂牌</span>
                    <input
                      value={demandMfr}
                      onChange={(e) => setDemandMfr(e.target.value)}
                      className="mt-0.5 w-full border border-slate-300 rounded px-2 py-1 text-sm"
                    />
                  </label>
                </div>
                <div className="flex flex-wrap items-center gap-2">
                  <span className="text-xs text-slate-500">筛选报价行</span>
                  <input
                    value={rowFilter}
                    onChange={(e) => setRowFilter(e.target.value)}
                    placeholder="型号 / 厂牌 / 封装"
                    className="border border-slate-300 rounded px-2 py-1 text-xs flex-1 min-w-[140px]"
                  />
                </div>
              </div>

              {data.no_mpn_detail.trim() ? (
                <div>
                  <div className="text-slate-600 font-medium mb-1 text-xs">no_mpn_detail</div>
                  <pre className="text-xs bg-slate-50 border border-slate-200 rounded p-2 overflow-auto max-h-32 whitespace-pre-wrap break-all">
                    {data.no_mpn_detail}
                  </pre>
                </div>
              ) : null}

              <div>
                <div className="text-slate-600 font-medium mb-2 text-sm">quotes_json（表格）</div>
                {quoteRows.length === 0 ? (
                  <p className="text-slate-500 text-xs">无可解析的报价数组（空或非 JSON）</p>
                ) : (
                  <div className="overflow-x-auto rounded border border-slate-200">
                    <table className="w-full text-xs min-w-[920px]">
                      <thead>
                        <tr className="bg-slate-100 text-left border-b border-slate-200">
                          <th className="py-2 px-2 w-8">#</th>
                          <th className="py-2 px-2">型号</th>
                          <th className="py-2 px-2">厂牌</th>
                          <th className="py-2 px-2">封装</th>
                          <th className="py-2 px-2">库存</th>
                          <th className="py-2 px-2">MOQ</th>
                          <th className="py-2 px-2">货期</th>
                          <th className="py-2 px-2">大陆价</th>
                          <th className="py-2 px-2">HK价</th>
                          <th className="py-2 px-2">型号试算</th>
                          <th className="py-2 px-2">封装试算</th>
                          <th className="py-2 px-2">厂牌（服务端）</th>
                          <th className="py-2 px-2">结论</th>
                        </tr>
                      </thead>
                      <tbody>
                        {filteredRows.map(({ row, i }) => {
                          const mt = localModelTrial(demandMpn, row)
                          const pt = localPackageTrial(demandPkg, row)
                          const sev = serverEvalByRow.get(i)
                          const mfrCell =
                            demandMatchesSnapshot && sev ? (
                              <span className={sev.manufacturer_ok ? 'text-emerald-700' : 'text-red-700'}>
                                {sev.manufacturer_ok ? '通过' : '未通过'}
                              </span>
                            ) : (
                              <span className="text-slate-400" title="与快照一致时才显示服务端厂牌判定">
                                —
                              </span>
                            )
                          const summaryCell =
                            demandMatchesSnapshot && sev ? (
                              <div>
                                <div
                                  className={
                                    sev.passes_bom_filters
                                      ? 'font-medium text-emerald-800'
                                      : 'font-medium text-red-800'
                                  }
                                >
                                  {sev.passes_bom_filters ? '通过 BOM 筛选' : '未通过 BOM 筛选'}
                                </div>
                                <div className="text-slate-600 mt-0.5 leading-snug">{sev.summary}</div>
                                <details className="mt-1 text-slate-500">
                                  <summary className="cursor-pointer hover:text-slate-700">分项原因</summary>
                                  <ul className="mt-1 pl-3 list-disc space-y-0.5">
                                    <li>型号：{sev.model_reason}</li>
                                    <li>封装：{sev.package_reason}</li>
                                    <li>厂牌：{sev.manufacturer_reason}</li>
                                  </ul>
                                </details>
                              </div>
                            ) : (
                              <div>
                                <div
                                  className={
                                    mt.ok && pt.ok ? 'font-medium text-teal-800' : 'font-medium text-amber-900'
                                  }
                                >
                                  {mt.ok && pt.ok ? '型号/封装试算均通过' : '型号/封装试算未全部通过'}
                                </div>
                                <div className="text-slate-600 mt-0.5 leading-snug">
                                  {mt.ok ? '✓' : '✗'} 型号：{mt.reason}
                                  <br />
                                  {pt.ok ? '✓' : '✗'} 封装：{pt.reason}
                                </div>
                                <p className="text-slate-400 mt-1 text-[11px]">厂牌与整行结论请对齐快照后查看或重新打开本窗口。</p>
                              </div>
                            )
                          return (
                            <tr key={i} className="border-b border-slate-100 align-top hover:bg-slate-50/60">
                              <td className="py-2 px-2 text-slate-500">{i + 1}</td>
                              <td className="py-2 px-2 font-mono break-all">{row.model || '—'}</td>
                              <td className="py-2 px-2 break-all">{row.manufacturer || '—'}</td>
                              <td className="py-2 px-2 break-all">{row.package || '—'}</td>
                              <td className="py-2 px-2 whitespace-nowrap">{row.stock || '—'}</td>
                              <td className="py-2 px-2 whitespace-nowrap">{row.moq || '—'}</td>
                              <td className="py-2 px-2 whitespace-nowrap">{row.lead_time || '—'}</td>
                              <td className="py-2 px-2 break-all max-w-[100px]">{row.mainland_price || '—'}</td>
                              <td className="py-2 px-2 break-all max-w-[100px]">{row.hk_price || '—'}</td>
                              <td className="py-2 px-2">
                                <span className={mt.ok ? 'text-emerald-700' : 'text-red-700'}>{mt.ok ? '通过' : '未通过'}</span>
                                <div className="text-slate-500 text-[11px] mt-0.5 leading-snug">{mt.reason}</div>
                              </td>
                              <td className="py-2 px-2">
                                <span className={pt.ok ? 'text-emerald-700' : 'text-red-700'}>{pt.ok ? '通过' : '未通过'}</span>
                                <div className="text-slate-500 text-[11px] mt-0.5 leading-snug">{pt.reason}</div>
                              </td>
                              <td className="py-2 px-2">
                                {mfrCell}
                                {demandMatchesSnapshot && sev ? (
                                  <div className="text-slate-500 text-[11px] mt-0.5 leading-snug">{sev.manufacturer_reason}</div>
                                ) : null}
                              </td>
                              <td className="py-2 px-2 max-w-[220px]">{summaryCell}</td>
                            </tr>
                          )
                        })}
                      </tbody>
                    </table>
                  </div>
                )}
              </div>

              <details className="text-xs">
                <summary className="cursor-pointer text-slate-600 hover:text-slate-800">原始 quotes_json</summary>
                <pre className="mt-2 text-xs bg-slate-900 text-slate-100 rounded p-3 overflow-auto max-h-48 whitespace-pre-wrap break-all font-mono">
                  {(() => {
                    const t = data.quotes_json.trim()
                    if (!t) return '(空)'
                    try {
                      return JSON.stringify(JSON.parse(t), null, 2)
                    } catch {
                      return data.quotes_json
                    }
                  })()}
                </pre>
              </details>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

function MatchSourcesModal({ bomId, onClose }: { bomId: string; onClose: () => void }) {
  const [loading, setLoading] = useState(true)
  const [err, setErr] = useState<string | null>(null)
  const [payload, setPayload] = useState<Awaited<ReturnType<typeof listMatchSourceRecords>> | null>(null)
  const [detail, setDetail] = useState<{ lineNo: number; platform: string; hint: string } | null>(null)

  useEffect(() => {
    let cancelled = false
    ;(async () => {
      setLoading(true)
      setErr(null)
      try {
        const d = await listMatchSourceRecords(bomId)
        if (!cancelled) setPayload(d)
      } catch (e) {
        if (!cancelled) setErr(e instanceof Error ? e.message : '加载失败')
      } finally {
        if (!cancelled) setLoading(false)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [bomId])

  return (
    <>
      <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={onClose}>
        <div
          className="max-h-[90vh] w-full max-w-5xl overflow-hidden flex flex-col rounded-lg bg-white shadow-xl"
          onClick={(e) => e.stopPropagation()}
        >
          <div className="flex justify-between items-center px-5 py-4 border-b border-slate-200">
            <div>
              <h3 className="text-lg font-bold text-slate-800">配单数据源（报价缓存摘要）</h3>
              {payload ? (
                <p className="text-xs text-slate-500 mt-1">
                  业务日 {payload.biz_date || '—'} · 会话平台 {payload.session_platforms.join(', ') || '—'}
                </p>
              ) : null}
            </div>
            <button type="button" onClick={onClose} className="text-slate-500 hover:text-slate-700">
              ✕
            </button>
          </div>
          <div className="overflow-auto flex-1 p-4">
            {loading ? <div className="text-slate-500 py-8 text-center">加载中…</div> : null}
            {err ? <div className="text-red-600 bg-red-50 border border-red-200 rounded px-3 py-2">{err}</div> : null}
            {!loading && !err && payload && payload.lines.length === 0 ? (
              <div className="text-slate-500 py-8 text-center">暂无 BOM 行</div>
            ) : null}
            {!loading && !err && payload && payload.lines.length > 0 ? (
              <div className="space-y-4">
                {payload.lines.map((line) => (
                  <div key={line.line_no} className="rounded-lg border border-slate-200 overflow-hidden">
                    <div className="bg-slate-50 px-3 py-2 text-sm flex flex-wrap gap-x-4 gap-y-1">
                      <span>
                        <span className="text-slate-500">行号</span> {line.line_no}
                      </span>
                      <span className="font-medium text-slate-800 break-all">{line.mpn}</span>
                      {line.merge_mpn !== line.mpn ? (
                        <span className="text-xs text-slate-500 break-all">merge: {line.merge_mpn}</span>
                      ) : null}
                      <span className="text-slate-500">×{line.quantity}</span>
                      {line.demand_manufacturer || line.demand_package ? (
                        <span className="text-xs text-slate-600">
                          {[line.demand_manufacturer, line.demand_package].filter(Boolean).join(' · ')}
                        </span>
                      ) : null}
                    </div>
                    <table className="w-full text-xs">
                      <thead>
                        <tr className="bg-slate-100 text-left">
                          <th className="py-2 px-2">平台</th>
                          <th className="py-2 px-2">缓存</th>
                          <th className="py-2 px-2">跳过原因</th>
                          <th className="py-2 px-2">outcome</th>
                          <th className="py-2 px-2">JSON 大小</th>
                          <th className="py-2 px-2 w-24">操作</th>
                        </tr>
                      </thead>
                      <tbody>
                        {line.platforms.map((p) => (
                          <tr key={p.platform} className="border-t border-slate-100 hover:bg-slate-50/80">
                            <td className="py-2 px-2 font-medium">{p.platform}</td>
                            <td className="py-2 px-2">{p.cache_hit ? '命中' : '未命中'}</td>
                            <td className="py-2 px-2 break-all text-slate-600">{p.skip_reason || '—'}</td>
                            <td className="py-2 px-2 break-all text-slate-600">{p.outcome || '—'}</td>
                            <td className="py-2 px-2 tabular-nums">{p.quotes_json_size}</td>
                            <td className="py-2 px-2">
                              <button
                                type="button"
                                className="text-blue-600 hover:underline"
                                onClick={() =>
                                  setDetail({
                                    lineNo: line.line_no,
                                    platform: p.platform,
                                    hint: `行 ${line.line_no} · ${p.platform} · ${line.mpn}`,
                                  })
                                }
                              >
                                查看详情
                              </button>
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                ))}
              </div>
            ) : null}
          </div>
        </div>
      </div>
      {detail ? (
        <MatchSourceDetailModal
          bomId={bomId}
          lineNo={detail.lineNo}
          platform={detail.platform}
          titleHint={detail.hint}
          onClose={() => setDetail(null)}
        />
      ) : null}
    </>
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
        {(item.hs_code_status ||
          item.code_ts ||
          item.control_mark ||
          item.import_tax_imp_ordinary_rate ||
          item.hs_customs_error) && (
          <div className="mb-4 rounded-lg border border-emerald-200 bg-emerald-50/60 p-4">
            <h4 className="mb-2 font-medium text-slate-700">HS / 商检 / 进口税</h4>
            <div className="grid grid-cols-2 gap-x-6 gap-y-1 text-sm sm:grid-cols-3">
              <span className="text-slate-500">HS 状态</span>
              <span>{hsStatusLabel(item.hs_code_status)}</span>
              <span className="text-slate-500">code_ts</span>
              <span className="break-all">{item.code_ts?.trim() || '—'}</span>
              <span className="text-slate-500">商检标记</span>
              <span className="break-all">{item.control_mark?.trim() || '—'}</span>
              <span className="text-slate-500">关税品名</span>
              <span className="break-all">{item.import_tax_g_name?.trim() || '—'}</span>
              <span className="text-slate-500">普通税率</span>
              <span>{item.import_tax_imp_ordinary_rate?.trim() || '—'}</span>
              <span className="text-slate-500">进口最惠国税率</span>
              <span>{item.import_tax_imp_discount_rate?.trim() || '—'}</span>
              <span className="text-slate-500">进口暂定税率</span>
              <span>{item.import_tax_imp_temp_rate?.trim() || '—'}</span>
              <span className="text-slate-500">分项错误</span>
              <span className="break-all text-amber-900">{item.hs_customs_error?.trim() || '—'}</span>
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

function MatchRow({
  item,
  statusFilter,
  onShowDetail,
  onHsResolve,
  sessionReady,
  visibleColumns,
  tableCols,
}: {
  item: MatchItem
  statusFilter: string
  onShowDetail: (item: MatchItem) => void
  onHsResolve: (item: MatchItem) => void
  sessionReady: boolean
  visibleColumns: Record<OptionalColumnKey, boolean>
  tableCols: number
}) {
  const [expanded, setExpanded] = useState(false)
  const show = statusFilter === 'all' || item.match_status === statusFilter
  if (!show) return null

  const hasQuotes = item.all_quotes && item.all_quotes.length > 0
  const showHsBtn = matchItemNeedsHsResolve(item)
  const hsMain =
    (item.hs_code_status || '').trim() === 'hs_found' && (item.code_ts || '').trim()
      ? item.code_ts!.trim()
      : hsStatusLabel(item.hs_code_status)

  return (
    <>
      <tr className="border-b border-slate-200 hover:bg-slate-50">
        {visibleColumns.index && <td className="py-3 px-3">{item.index}</td>}
        {visibleColumns.model && <td className="py-3 px-3 text-slate-800">{item.model}</td>}
        {visibleColumns.manufacturer && <td className="py-3 px-3">{item.demand_manufacturer || '-'}</td>}
        {visibleColumns.package && <td className="py-3 px-3">{item.demand_package || '-'}</td>}
        {visibleColumns.quantity && <td className="py-3 px-3">{item.quantity}</td>}
        {visibleColumns.matchStatus && (
          <td className="py-3 px-3">
            <StatusIcon status={item.match_status} />
          </td>
        )}
        {visibleColumns.bestModel && (
          <td className="py-3 px-3">
            <div className="flex flex-col gap-0.5">
              <span className="font-medium">{item.matched_model || '-'}</span>
              {(item.manufacturer || item.platform) && (
                <span className="text-slate-500 text-sm">
                  {item.manufacturer}{item.manufacturer && item.platform ? ' · ' : ''}{item.platform}
                </span>
              )}
              {item.mfr_mismatch_quote_manufacturers && item.mfr_mismatch_quote_manufacturers.length > 0 && (
                <span className="text-amber-700 text-xs" title="型号/封装已对齐但厂牌与需求不一致的报价原文（去重）">
                  未匹配厂牌：{item.mfr_mismatch_quote_manufacturers.join('；')}
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
        )}
        {visibleColumns.stock && <td className="py-3 px-3">{item.stock ?? '-'}</td>}
        {visibleColumns.leadTime && <td className="py-3 px-3">{item.lead_time || '-'}</td>}
        {visibleColumns.unitPrice && <td className="py-3 px-3">¥{item.unit_price?.toFixed(2) ?? '-'}</td>}
        {visibleColumns.subtotal && <td className="py-3 px-3 font-medium">¥{item.subtotal?.toFixed(2) ?? '-'}</td>}
        {visibleColumns.hs && (
          <td className="py-3 px-3 text-sm align-top">
            <div className="flex flex-col gap-1 max-w-[140px]">
              <span className="font-mono text-xs break-all" title={item.hs_code_status}>
                {hsMain}
              </span>
              {item.hs_customs_error ? (
                <span className="text-amber-800 text-xs break-all" title={item.hs_customs_error}>
                  {item.hs_customs_error}
                </span>
              ) : null}
            </div>
          </td>
        )}
        {visibleColumns.control && (
          <td className="py-3 px-3 text-sm align-top break-all max-w-[100px]">
            {(item.control_mark || '').trim() || '—'}
          </td>
        )}
        {visibleColumns.ordinaryTax && (
          <td className="py-3 px-3 text-sm align-top" title={item.import_tax_g_name?.trim() || undefined}>
            {(item.import_tax_imp_ordinary_rate || '').trim() || '—'}
          </td>
        )}
        {visibleColumns.discountTax && (
          <td className="py-3 px-3 text-sm align-top" title={item.import_tax_g_name?.trim() || undefined}>
            {(item.import_tax_imp_discount_rate || '').trim() || '—'}
          </td>
        )}
        {visibleColumns.tempTax && (
          <td className="py-3 px-3 text-sm align-top" title={item.import_tax_g_name?.trim() || undefined}>
            {(item.import_tax_imp_temp_rate || '').trim() || '—'}
          </td>
        )}
        <td className="py-3 px-3 align-top">
          <div className="flex flex-col gap-1">
            {hasQuotes && (
              <button
                onClick={() => onShowDetail(item)}
                className="text-blue-600 text-sm hover:underline text-left"
              >
                查看详情
              </button>
            )}
            {showHsBtn && (
              <button
                type="button"
                disabled={!sessionReady}
                title={!sessionReady ? '需会话状态 data_ready' : undefined}
                onClick={() => onHsResolve(item)}
                className="text-sm rounded border border-blue-300 bg-blue-50 px-2 py-1 text-blue-800 hover:bg-blue-100 disabled:opacity-50"
              >
                一键找HS
              </button>
            )}
          </div>
        </td>
      </tr>
      {expanded && hasQuotes && (
        <tr>
          <td colSpan={tableCols} className="bg-slate-50 p-0 align-top">
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

export function MatchResultPage({ bomId, onNavigateToHsResolve }: MatchResultPageProps) {
  const [items, setItems] = useState<MatchItem[]>([])
  const [totalAmount, setTotalAmount] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [sessionReady, setSessionReady] = useState(false)
  const [sessionGateMsg, setSessionGateMsg] = useState<string | null>(null)
  const [statusFilter, setStatusFilter] = useState('all')
  const [strategy, setStrategy] = useState('price_first')
  const [running, setRunning] = useState(false)
  const [detailItem, setDetailItem] = useState<MatchItem | null>(null)
  const [canonicalRows, setCanonicalRows] = useState<ManufacturerCanonicalRow[]>([])
  const [approvedMfrAliases, setApprovedMfrAliases] = useState<string[]>([])
  const [mfrReviewExpanded, setMfrReviewExpanded] = useState(false)
  const [sourcesOpen, setSourcesOpen] = useState(false)
  const [mfrAliasNotice, setMfrAliasNotice] = useState<string | null>(null)
  const [batchHsBusy, setBatchHsBusy] = useState(false)
  const [batchHsMessage, setBatchHsMessage] = useState<string | null>(null)
  const [visibleColumns, setVisibleColumns] = useState<Record<OptionalColumnKey, boolean>>(DEFAULT_VISIBLE_OPTIONAL_COLUMNS)

  useEffect(() => {
    try {
      const raw = localStorage.getItem(MATCH_RESULT_VISIBLE_COLS_KEY)
      if (!raw) return
      const parsed = JSON.parse(raw) as Partial<Record<OptionalColumnKey, boolean>>
      setVisibleColumns({
        index: parsed.index ?? true,
        model: parsed.model ?? true,
        manufacturer: parsed.manufacturer ?? true,
        package: parsed.package ?? true,
        quantity: parsed.quantity ?? true,
        matchStatus: parsed.matchStatus ?? true,
        bestModel: parsed.bestModel ?? true,
        stock: parsed.stock ?? true,
        leadTime: parsed.leadTime ?? true,
        unitPrice: parsed.unitPrice ?? true,
        subtotal: parsed.subtotal ?? true,
        hs: parsed.hs ?? true,
        control: parsed.control ?? true,
        ordinaryTax: parsed.ordinaryTax ?? true,
        discountTax: parsed.discountTax ?? true,
        tempTax: parsed.tempTax ?? true,
      })
    } catch {
      setVisibleColumns(DEFAULT_VISIBLE_OPTIONAL_COLUMNS)
    }
  }, [])

  useEffect(() => {
    localStorage.setItem(MATCH_RESULT_VISIBLE_COLS_KEY, JSON.stringify(visibleColumns))
  }, [visibleColumns])

  const tableCols = useMemo(
    () => 1 + Object.values(visibleColumns).filter(Boolean).length,
    [visibleColumns]
  )

  const pendingMfrRows = useMemo(() => {
    const all = collectPendingMfrRows(items)
    const set = new Set(approvedMfrAliases)
    return all.filter((r) => !set.has(r.alias))
  }, [items, approvedMfrAliases])

  const load = async () => {
    setLoading(true)
    setError(null)
    setSessionGateMsg(null)
    try {
      const sess = await getSession(bomId)
      const ok = (sess.status || '').trim() === SESSION_MATCH_READY
      setSessionReady(ok)
      if (!ok) {
        const st = (sess.status || '').trim() || '未知'
        setSessionGateMsg(`会话状态为「${st}」，须为 data_ready 后才可加载配单结果。请返回 BOM 会话看板等待搜索完成。`)
        setItems([])
        setTotalAmount(0)
        return
      }
      try {
        const res = await getMatchResult(bomId)
        setItems(res.items || [])
        setTotalAmount(res.total_amount || 0)
      } catch (e2) {
        setError(e2 instanceof Error ? e2.message : '配单结果加载失败')
        setItems([])
        setTotalAmount(0)
      }
    } catch (e) {
      setSessionReady(false)
      setSessionGateMsg(null)
      setError(e instanceof Error ? e.message : '加载会话失败')
      setItems([])
      setTotalAmount(0)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
  }, [bomId])

  useEffect(() => {
    setApprovedMfrAliases([])
    setMfrReviewExpanded(false)
    setCanonicalRows([])
    setMfrAliasNotice(null)
  }, [bomId])

  useEffect(() => {
    if (!mfrReviewExpanded) {
      return
    }
    if (canonicalRows.length > 0) {
      return
    }
    let cancelled = false
    ;(async () => {
      try {
        const rows = await listManufacturerCanonicals(500)
        if (!cancelled) setCanonicalRows(rows)
      } catch {
        if (!cancelled) setCanonicalRows([])
      }
    })()
    return () => {
      cancelled = true
    }
  }, [mfrReviewExpanded, canonicalRows.length])

  const handleHsResolve = (item: MatchItem) => {
    setError(null)
    if (!sessionReady) {
      setError('会话未处于 data_ready，无法使用一键找 HS')
      return
    }
    if (!onNavigateToHsResolve) return
    onNavigateToHsResolve((item.model || '').trim(), (item.demand_manufacturer || '').trim())
  }

  const handleBatchHsResolve = async () => {
    if (!sessionReady) {
      setError('会话未处于 data_ready，无法批量触发 HS 解析')
      return
    }
    const lines = items
      .filter((item) => item.match_status === 'exact' && matchItemNeedsHsResolve(item))
      .map((item) => ({
        line_no: item.index,
        model: (item.model || '').trim(),
        manufacturer: (item.demand_manufacturer || '').trim(),
        match_status: item.match_status || '',
        hs_code_status: item.hs_code_status || '',
      }))
      .filter((item) => item.model)
    if (lines.length === 0) {
      setBatchHsMessage('当前没有“已匹配且缺少HS”的行可触发。')
      return
    }
    setBatchHsBusy(true)
    setError(null)
    setBatchHsMessage(null)
    try {
      const res = await hsBatchResolveByModels({
        session_id: bomId,
        request_id: `batch-${Date.now()}`,
        lines,
      })
      setBatchHsMessage(
        `已提交：成功 ${res.accepted_count}，跳过 ${res.skipped_count}，失败 ${res.failed_count}`
      )
    } catch (e) {
      setError(e instanceof Error ? e.message : '批量触发 HS 解析失败')
    } finally {
      setBatchHsBusy(false)
    }
  }

  const runMatchOnly = async () => {
    setRunning(true)
    setError(null)
    try {
      const sess = await getSession(bomId)
      if ((sess.status || '').trim() !== SESSION_MATCH_READY) {
        setError('会话未处于 data_ready，无法配单')
        return
      }
      const res = await autoMatch(bomId, strategy)
      setItems(res.items || [])
      setTotalAmount(res.total_amount || 0)
    } catch (e) {
      setError(e instanceof Error ? e.message : '配单失败')
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
            disabled={!sessionReady}
            className="border border-slate-300 rounded px-3 py-2 disabled:opacity-50"
          >
            {STRATEGY_OPTIONS.map((s) => (
              <option key={s.value} value={s.value}>
                {s.label}
              </option>
            ))}
          </select>
          <button
            type="button"
            onClick={() => setSourcesOpen(true)}
            disabled={!sessionReady}
            className="px-4 py-2 border border-slate-300 rounded-lg text-slate-700 text-sm font-medium hover:bg-slate-50 disabled:opacity-50"
          >
            配单数据源
          </button>
          <button
            type="button"
            onClick={() => void handleBatchHsResolve()}
            disabled={batchHsBusy || !sessionReady}
            className="px-4 py-2 border border-blue-300 bg-blue-50 rounded-lg text-blue-800 text-sm font-medium hover:bg-blue-100 disabled:opacity-50"
          >
            {batchHsBusy ? '批量提交中...' : '批量解析HS（已匹配未填HS）'}
          </button>
          <button
            onClick={() => void runMatchOnly()}
            disabled={running || !sessionReady}
            className="px-4 py-2 bg-blue-600 text-white rounded-lg font-medium hover:bg-blue-700 disabled:opacity-50"
          >
            {running ? '配单中...' : '重新配单'}
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
        <div className="flex items-center gap-3">
          <details className="relative">
            <summary className="cursor-pointer border border-slate-300 bg-white rounded-lg px-3 py-1.5 text-sm text-slate-700 select-none">
              显示字段
            </summary>
            <div className="absolute right-0 z-10 mt-2 w-56 rounded-lg border border-slate-200 bg-white p-3 shadow-lg">
              <div className="mb-2 text-xs text-slate-500">勾选后在列表显示对应列</div>
              <div className="space-y-1.5">
                {OPTIONAL_COLUMNS.map((col) => (
                  <label key={col.key} className="flex items-center gap-2 text-sm text-slate-700">
                    <input
                      type="checkbox"
                      checked={visibleColumns[col.key]}
                      onChange={(e) =>
                        setVisibleColumns((prev) => ({
                          ...prev,
                          [col.key]: e.target.checked,
                        }))
                      }
                    />
                    <span>{col.label}</span>
                  </label>
                ))}
              </div>
            </div>
          </details>
          <div className="text-slate-600">
            合计: <span className="font-bold text-slate-800">¥{totalAmount.toFixed(2)}</span>
          </div>
        </div>
      </div>

      {sessionGateMsg && (
        <div className="p-4 bg-amber-50 text-amber-950 rounded-lg border border-amber-200 text-sm">{sessionGateMsg}</div>
      )}
      {error && (
        <div className="p-4 bg-red-50 text-red-700 rounded-lg">{error}</div>
      )}
      {mfrAliasNotice && (
        <div className="p-3 bg-sky-50 text-sky-900 rounded-lg text-sm flex justify-between gap-3 border border-sky-200">
          <span>{mfrAliasNotice}</span>
          <button type="button" className="text-sky-800 underline shrink-0" onClick={() => setMfrAliasNotice(null)}>
            关闭
          </button>
        </div>
      )}
      {batchHsMessage && (
        <div className="p-3 bg-emerald-50 text-emerald-900 rounded-lg text-sm border border-emerald-200">
          {batchHsMessage}
        </div>
      )}


      <div className="overflow-x-auto rounded-lg border border-slate-200 bg-white">
        <table className="w-full" style={{ minWidth: 1360 }}>
          <thead>
            <tr className="bg-slate-100">
              {visibleColumns.index && <th className="py-3 px-3 text-left whitespace-nowrap">序号</th>}
              {visibleColumns.model && <th className="py-3 px-3 text-left whitespace-nowrap">需求型号</th>}
              {visibleColumns.manufacturer && <th className="py-3 px-3 text-left whitespace-nowrap">厂牌</th>}
              {visibleColumns.package && <th className="py-3 px-3 text-left whitespace-nowrap">封装</th>}
              {visibleColumns.quantity && <th className="py-3 px-3 text-left whitespace-nowrap">数量</th>}
              {visibleColumns.matchStatus && <th className="py-3 px-3 text-left whitespace-nowrap">结果</th>}
              {visibleColumns.bestModel && <th className="py-3 px-3 text-left whitespace-nowrap">推荐最优型号</th>}
              {visibleColumns.stock && <th className="py-3 px-3 text-left whitespace-nowrap">库存</th>}
              {visibleColumns.leadTime && <th className="py-3 px-3 text-left whitespace-nowrap">货期</th>}
              {visibleColumns.unitPrice && <th className="py-3 px-3 text-left whitespace-nowrap">单价</th>}
              {visibleColumns.subtotal && <th className="py-3 px-3 text-left whitespace-nowrap">小计</th>}
              {visibleColumns.hs && <th className="py-3 px-3 text-left whitespace-nowrap">HS</th>}
              {visibleColumns.control && <th className="py-3 px-3 text-left whitespace-nowrap">商检</th>}
              {visibleColumns.ordinaryTax && (
                <th className="py-3 px-3 text-left whitespace-nowrap" title="进口普通税率（接口字段 imp_ordinary_rate）">
                  普通税率
                </th>
              )}
              {visibleColumns.discountTax && (
                <th className="py-3 px-3 text-left whitespace-nowrap" title="进口最惠国税率（接口字段 imp_discount_rate）">
                  最惠税率
                </th>
              )}
              {visibleColumns.tempTax && (
                <th className="py-3 px-3 text-left whitespace-nowrap" title="进口暂定税率（接口字段 imp_temp_rate）">
                  暂定税率
                </th>
              )}
              <th className="py-3 px-3 text-left whitespace-nowrap">操作</th>
            </tr>
          </thead>
          <tbody>
            {items.length === 0 ? (
              <tr>
                <td colSpan={tableCols} className="py-12 text-center text-slate-500">
                  暂无配单结果，请点击「重新搜索并配单」获取报价
                </td>
              </tr>
            ) : (
              items.map((item) => (
                <MatchRow
                  key={item.index}
                  item={item}
                  statusFilter={statusFilter}
                  onShowDetail={setDetailItem}
                  onHsResolve={handleHsResolve}
                  sessionReady={sessionReady}
                  visibleColumns={visibleColumns}
                  tableCols={tableCols}
                />
              ))
            )}
          </tbody>
        </table>
      </div>

      <div className="rounded-lg border border-amber-200 bg-white shadow-sm overflow-hidden">
        <button
          type="button"
          onClick={() => setMfrReviewExpanded((e) => !e)}
          className="w-full flex items-center justify-between gap-3 px-4 py-3 text-left text-sm font-medium text-amber-950 bg-amber-50/90 hover:bg-amber-100/90 transition-colors"
        >
          <span>厂牌别名审核</span>
          <span className="text-slate-600 font-normal shrink-0">
            {mfrReviewExpanded
              ? '收起'
              : pendingMfrRows.length > 0
                ? `展开（待处理 ${pendingMfrRows.length} 项）`
                : '展开'}
          </span>
        </button>
        {mfrReviewExpanded && (
          <div className="border-t border-amber-100">
            {pendingMfrRows.length > 0 ? (
              <MfrAliasReviewPanel
                pendingRows={pendingMfrRows}
                onApproved={(alias) => {
                  setApprovedMfrAliases((prev) => [...prev, alias])
                  setCanonicalRows([])
                }}
                canonicalRows={canonicalRows}
              />
            ) : (
              <p className="px-4 py-3 text-sm text-slate-600 bg-amber-50/30 border-b border-amber-100/80">
                当前配单结果中无「型号/封装已对齐但与需求厂牌不一致」的待审核项。若仍需录入别名，请使用下方表单。
              </p>
            )}
            <div className="px-4 py-3 bg-white">
              <p className="text-xs font-medium text-slate-700 mb-2">手动新增厂牌别名</p>
              <ManualManufacturerAliasForm
                canonicalRows={canonicalRows}
                onSuccess={() => {
                  setMfrAliasNotice('已写入别名表，请重新配单后生效。')
                  setCanonicalRows([])
                }}
              />
            </div>
          </div>
        )}
      </div>

      {detailItem && (
        <DetailModal item={detailItem} onClose={() => setDetailItem(null)} />
      )}

      {sourcesOpen ? <MatchSourcesModal bomId={bomId} onClose={() => setSourcesOpen(false)} /> : null}
    </div>
  )
}
