/**
 * 经典 BOM 流程：/api/v1/bom/*
 * 与 docs/BOM货源搜索-接口清单.md 中「无会话」旧路径一致。
 */
import { fetchJson } from './http'
import type { MatchItem, ParsedItem, PlatformQuote } from './types'

const BASE = '/api/v1/bom'

function normQuote(q: Record<string, unknown>): PlatformQuote {
  return {
    platform: (q.platform as string) ?? '',
    matched_model: (q.matched_model ?? q.matchedModel) as string,
    manufacturer: (q.manufacturer as string) ?? '',
    package: (q.package as string) ?? '',
    description: (q.description as string) ?? '',
    stock: Number(q.stock ?? 0),
    lead_time: (q.lead_time ?? q.leadTime) as string,
    moq: Number(q.moq ?? 0),
    increment: Number(q.increment ?? 0),
    price_tiers: (q.price_tiers ?? q.priceTiers) as string,
    hk_price: (q.hk_price ?? q.hkPrice) as string,
    mainland_price: (q.mainland_price ?? q.mainlandPrice) as string,
    unit_price: Number(q.unit_price ?? q.unitPrice ?? 0),
    subtotal: Number(q.subtotal ?? 0),
  }
}

function normMatchItem(m: Record<string, unknown>): MatchItem {
  const quotes = (m.all_quotes ?? m.allQuotes ?? []) as Record<string, unknown>[]
  return {
    index: Number(m.index ?? 0),
    model: (m.model as string) ?? '',
    quantity: Number(m.quantity ?? 0),
    matched_model: (m.matched_model ?? m.matchedModel) as string,
    manufacturer: (m.manufacturer as string) ?? '',
    platform: (m.platform as string) ?? '',
    lead_time: (m.lead_time ?? m.leadTime) as string,
    stock: Number(m.stock ?? 0),
    unit_price: Number(m.unit_price ?? m.unitPrice ?? 0),
    subtotal: Number(m.subtotal ?? 0),
    match_status: (m.match_status ?? m.matchStatus) as string,
    all_quotes: quotes.map(normQuote),
    demand_manufacturer: (m.demand_manufacturer ?? m.demandManufacturer) as string,
    demand_package: (m.demand_package ?? m.demandPackage) as string,
    mfr_mismatch_quote_manufacturers: (m.mfr_mismatch_quote_manufacturers ??
      m.mfrMismatchQuoteManufacturers) as string[] | undefined,
  }
}

export async function downloadTemplate(): Promise<Blob> {
  const json = await fetchJson<{ file?: string; filename?: string }>(`${BASE}/template`)
  const b64 = json.file
  if (!b64) throw new Error('模板数据为空')
  const bin = atob(b64)
  const bytes = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i)
  return new Blob([bytes], { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' })
}

export async function uploadBOM(
  file: File,
  parseMode: string,
  columnMapping: Record<string, string> | undefined,
  opts?: { sessionId?: string }
): Promise<{ bom_id: string; items: ParsedItem[]; total: number }> {
  const buf = await file.arrayBuffer()
  const bytes = new Uint8Array(buf)
  let binary = ''
  for (let i = 0; i < bytes.length; i++) binary += String.fromCharCode(bytes[i])
  const b64 = btoa(binary)

  const body: Record<string, unknown> = {
    file: b64,
    filename: file.name,
    parse_mode: parseMode,
  }
  if (columnMapping && Object.keys(columnMapping).length > 0) body.column_mapping = columnMapping
  if (opts?.sessionId) body.session_id = opts.sessionId

  const json = await fetchJson<Record<string, unknown>>(`${BASE}/upload`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  return {
    bom_id: (json.bom_id ?? json.bomId) as string,
    items: (json.items ?? []) as ParsedItem[],
    total: Number(json.total ?? 0),
  }
}

export async function autoMatch(
  bomId: string,
  strategy: string = 'price_first'
): Promise<{ items: MatchItem[]; total_amount: number }> {
  const json = await fetchJson<Record<string, unknown>>(`${BASE}/match`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ bom_id: bomId, strategy }),
  })
  const items = ((json.items ?? []) as Record<string, unknown>[]).map(normMatchItem)
  return { items, total_amount: Number(json.total_amount ?? json.totalAmount ?? 0) }
}

export async function getMatchResult(bomId: string): Promise<{ items: MatchItem[]; total_amount: number }> {
  const json = await fetchJson<Record<string, unknown>>(`${BASE}/${encodeURIComponent(bomId)}/match`)
  const items = ((json.items ?? []) as Record<string, unknown>[]).map(normMatchItem)
  return { items, total_amount: Number(json.total_amount ?? json.totalAmount ?? 0) }
}

export interface MatchSourcePlatformEntry {
  platform: string
  cache_hit: boolean
  skip_reason: string
  outcome: string
  quotes_json_size: number
}

export interface MatchSourceLineRecord {
  line_no: number
  mpn: string
  merge_mpn: string
  quantity: number
  demand_manufacturer: string
  demand_package: string
  platforms: MatchSourcePlatformEntry[]
}

function normMatchSourcePlatform(p: Record<string, unknown>): MatchSourcePlatformEntry {
  return {
    platform: String(p.platform ?? ''),
    cache_hit: Boolean(p.cache_hit ?? p.cacheHit ?? false),
    skip_reason: String(p.skip_reason ?? p.skipReason ?? ''),
    outcome: String(p.outcome ?? ''),
    quotes_json_size: Number(p.quotes_json_size ?? p.quotesJsonSize ?? 0),
  }
}

function normMatchSourceLine(l: Record<string, unknown>): MatchSourceLineRecord {
  const plats = (l.platforms ?? []) as Record<string, unknown>[]
  return {
    line_no: Number(l.line_no ?? l.lineNo ?? 0),
    mpn: String(l.mpn ?? ''),
    merge_mpn: String(l.merge_mpn ?? l.mergeMpn ?? ''),
    quantity: Number(l.quantity ?? 0),
    demand_manufacturer: String(l.demand_manufacturer ?? l.demandManufacturer ?? ''),
    demand_package: String(l.demand_package ?? l.demandPackage ?? ''),
    platforms: plats.map(normMatchSourcePlatform),
  }
}

/** GET …/bom/{bom_id}/match-sources — 配单读取的报价缓存摘要（无 quotes_json 正文） */
export async function listMatchSourceRecords(bomId: string): Promise<{
  biz_date: string
  session_platforms: string[]
  lines: MatchSourceLineRecord[]
}> {
  const json = await fetchJson<Record<string, unknown>>(
    `${BASE}/${encodeURIComponent(bomId)}/match-sources`
  )
  const lines = ((json.lines ?? []) as Record<string, unknown>[]).map(normMatchSourceLine)
  const sp = (json.session_platforms ?? json.sessionPlatforms ?? []) as unknown
  return {
    biz_date: String(json.biz_date ?? json.bizDate ?? ''),
    session_platforms: Array.isArray(sp) ? sp.map((x) => String(x)) : [],
    lines,
  }
}

/** 与后端 biz.AgentQuoteRow JSON 字段一致 */
export interface AgentQuoteRow {
  seq: number
  model: string
  manufacturer: string
  package: string
  desc: string
  stock: string
  moq: string
  price_tiers: string
  hk_price: string
  mainland_price: string
  lead_time: string
  query_model?: string
}

/** 与后端 biz.ExplainQuoteRowsForBOMLine / proto QuoteRowMatchEval 一致 */
export interface QuoteRowMatchEval {
  row_index: number
  model_ok: boolean
  model_reason: string
  package_ok: boolean
  package_reason: string
  manufacturer_ok: boolean
  manufacturer_reason: string
  passes_bom_filters: boolean
  summary: string
}

/** 与 Go biz.NormalizeMPNForBOMSearch 一致（前端试算） */
export function normalizeMPNForBomSearchClient(mpn: string): string {
  const m = mpn.trim()
  if (m === '') return '-'
  return m.toUpperCase()
}

/** 与 Go biz.NormalizeMfrString 一致（NFKC + 大写，前端试算） */
export function normalizeMfrStringClient(s: string): string {
  const t = s.trim()
  if (t === '') return ''
  try {
    return t.normalize('NFKC').toUpperCase()
  } catch {
    return t.toUpperCase()
  }
}

function normAgentQuoteRow(r: Record<string, unknown>): AgentQuoteRow {
  return {
    seq: Number(r.seq ?? 0),
    model: String(r.model ?? ''),
    manufacturer: String(r.manufacturer ?? ''),
    package: String(r.package ?? ''),
    desc: String(r.desc ?? r.description ?? ''),
    stock: String(r.stock ?? ''),
    moq: String(r.moq ?? r.MOQ ?? ''),
    price_tiers: String(r.price_tiers ?? r.priceTiers ?? ''),
    hk_price: String(r.hk_price ?? r.hkPrice ?? ''),
    mainland_price: String(r.mainland_price ?? r.mainlandPrice ?? ''),
    lead_time: String(r.lead_time ?? r.leadTime ?? ''),
    query_model: r.query_model != null ? String(r.query_model) : r.queryModel != null ? String(r.queryModel) : undefined,
  }
}

/** 解析 bom_quote_cache.quotes_json：数组或 { results: [] } */
export function parseAgentQuoteRowsFromCache(raw: string): AgentQuoteRow[] {
  const t = raw.trim()
  if (!t) return []
  let parsed: unknown
  try {
    parsed = JSON.parse(t)
  } catch {
    return []
  }
  if (Array.isArray(parsed)) {
    return (parsed as Record<string, unknown>[]).map(normAgentQuoteRow)
  }
  if (parsed && typeof parsed === 'object' && Array.isArray((parsed as { results?: unknown }).results)) {
    return ((parsed as { results: Record<string, unknown>[] }).results ?? []).map(normAgentQuoteRow)
  }
  return []
}

function normQuoteRowEval(r: Record<string, unknown>): QuoteRowMatchEval {
  return {
    row_index: Number(r.row_index ?? r.rowIndex ?? 0),
    model_ok: Boolean(r.model_ok ?? r.modelOk ?? false),
    model_reason: String(r.model_reason ?? r.modelReason ?? ''),
    package_ok: Boolean(r.package_ok ?? r.packageOk ?? false),
    package_reason: String(r.package_reason ?? r.packageReason ?? ''),
    manufacturer_ok: Boolean(r.manufacturer_ok ?? r.manufacturerOk ?? false),
    manufacturer_reason: String(r.manufacturer_reason ?? r.manufacturerReason ?? ''),
    passes_bom_filters: Boolean(r.passes_bom_filters ?? r.passesBomFilters ?? false),
    summary: String(r.summary ?? ''),
  }
}

/** GET …/detail?line_no=&platform= — 单行×单平台完整 quotes_json + 逐行匹配说明 */
export async function getMatchSourceDetail(
  bomId: string,
  lineNo: number,
  platform: string
): Promise<{
  merge_mpn: string
  platform: string
  cache_hit: boolean
  skip_reason: string
  outcome: string
  quotes_json: string
  no_mpn_detail: string
  quote_row_evals: QuoteRowMatchEval[]
  bom_demand_mpn: string
  bom_demand_package: string
  bom_demand_manufacturer: string
}> {
  const q = new URLSearchParams({
    line_no: String(lineNo),
    platform,
  })
  const json = await fetchJson<Record<string, unknown>>(
    `${BASE}/${encodeURIComponent(bomId)}/match-sources/detail?${q.toString()}`
  )
  const evalsRaw = (json.quote_row_evals ?? json.quoteRowEvals ?? []) as Record<string, unknown>[]
  return {
    merge_mpn: String(json.merge_mpn ?? json.mergeMpn ?? ''),
    platform: String(json.platform ?? ''),
    cache_hit: Boolean(json.cache_hit ?? json.cacheHit ?? false),
    skip_reason: String(json.skip_reason ?? json.skipReason ?? ''),
    outcome: String(json.outcome ?? ''),
    quotes_json: String(json.quotes_json ?? json.quotesJson ?? ''),
    no_mpn_detail: String(json.no_mpn_detail ?? json.noMpnDetail ?? ''),
    quote_row_evals: evalsRaw.map(normQuoteRowEval),
    bom_demand_mpn: String(json.bom_demand_mpn ?? json.bomDemandMpn ?? ''),
    bom_demand_package: String(json.bom_demand_package ?? json.bomDemandPackage ?? ''),
    bom_demand_manufacturer: String(json.bom_demand_manufacturer ?? json.bomDemandManufacturer ?? ''),
  }
}

export interface ManufacturerCanonicalRow {
  canonical_id: string
  display_name: string
}

/** GET /bom/manufacturer-alias/canonicals */
export async function listManufacturerCanonicals(limit?: number): Promise<ManufacturerCanonicalRow[]> {
  const q = limit != null && limit > 0 ? `?limit=${limit}` : ''
  const json = await fetchJson<Record<string, unknown>>(`${BASE}/manufacturer-alias/canonicals${q}`)
  const rows = (json.rows ?? []) as Record<string, unknown>[]
  return rows.map((r) => ({
    canonical_id: String(r.canonical_id ?? r.canonicalId ?? ''),
    display_name: String(r.display_name ?? r.displayName ?? ''),
  }))
}

/** POST /bom/manufacturer-alias — 审核通过后写入 t_bom_manufacturer_alias */
export async function createManufacturerAlias(
  alias: string,
  canonicalId: string,
  displayName: string
): Promise<{ alias_norm: string }> {
  const json = await fetchJson<Record<string, unknown>>(`${BASE}/manufacturer-alias`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      alias,
      canonical_id: canonicalId,
      display_name: displayName,
    }),
  })
  return { alias_norm: String(json.alias_norm ?? json.aliasNorm ?? '') }
}
