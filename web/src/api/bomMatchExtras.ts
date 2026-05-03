import { fetchJson } from './http'

export interface ManufacturerCanonicalRow {
  canonical_id: string
  display_name: string
  alias_count?: number
}

export interface ManufacturerAliasCandidate {
  kind: 'demand' | 'quote'
  alias: string
  recommended_canonical_id: string
  line_nos: number[]
  platform_ids: string[]
  demand_hint: string
}

export interface AgentQuoteRow {
  model: string
  manufacturer: string
  package: string
  stock?: number
  moq?: number
  lead_time?: string
  mainland_price?: string
  hk_price?: string
  price?: string
}

export interface QuoteRowMatchEval {
  row_index: number
  model_match: boolean
  package_match: boolean
  manufacturer_match?: boolean
  manufacturer_ok?: boolean
  passes_bom_filters?: boolean
  summary?: string
  model_reason?: string
  package_reason?: string
  manufacturer_reason?: string
  reason?: string
}

export interface MatchSourcePlatformRecord {
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
  platforms: MatchSourcePlatformRecord[]
}

export interface MatchSourceRecordsReply {
  biz_date: string
  session_platforms: string[]
  lines: MatchSourceLineRecord[]
}

export interface MatchSourceDetailReply {
  merge_mpn: string
  platform: string
  cache_hit: boolean
  skip_reason: string
  outcome: string
  quotes_json: string
  quote_row_evals: QuoteRowMatchEval[]
  bom_demand_mpn: string
  bom_demand_package: string
  bom_demand_manufacturer: string
  no_mpn_detail: string
}

function str(v: unknown): string {
  return typeof v === 'string' ? v : v != null ? String(v) : ''
}

function num(v: unknown): number {
  if (typeof v === 'number' && !Number.isNaN(v)) return v
  if (typeof v === 'string') return Number(v) || 0
  return 0
}

function bool(v: unknown): boolean {
  return v === true || v === 'true' || v === 1
}

export function normalizeMPNForBomSearchClient(v: string): string {
  return v.trim().toUpperCase().replace(/[\s_-]+/g, '')
}

export function normalizeMfrStringClient(v: string): string {
  return v.trim().toUpperCase().replace(/\s+/g, ' ')
}

export function parseAgentQuoteRowsFromCache(raw: string): AgentQuoteRow[] {
  if (!raw.trim()) return []
  try {
    const json = JSON.parse(raw) as unknown
    const rows = Array.isArray(json)
      ? json
      : Array.isArray((json as { rows?: unknown }).rows)
        ? (json as { rows: unknown[] }).rows
        : Array.isArray((json as { items?: unknown }).items)
          ? (json as { items: unknown[] }).items
          : []
    return rows.map((row) => {
      const r = row as Record<string, unknown>
      return {
        model: str(r.model ?? r.mpn ?? r.matched_model),
        manufacturer: str(r.manufacturer ?? r.mfr),
        package: str(r.package ?? r.pkg),
        stock: num(r.stock),
        moq: num(r.moq),
        lead_time: str(r.lead_time ?? r.leadTime),
        mainland_price: str(r.mainland_price ?? r.mainlandPrice),
        hk_price: str(r.hk_price ?? r.hkPrice),
        price: str(r.price ?? r.unit_price),
      }
    })
  } catch {
    return []
  }
}

export async function createManufacturerAlias(
  alias: string,
  canonicalId: string,
  displayName: string
): Promise<void> {
  await fetchJson('/api/v1/bom/manufacturer-aliases', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ alias, canonical_id: canonicalId, display_name: displayName }),
  })
}

export async function listManufacturerCanonicals(
  limit = 500
): Promise<ManufacturerCanonicalRow[]> {
  const q = new URLSearchParams({ limit: String(limit) })
  const json = await fetchJson<Record<string, unknown>>(`/api/v1/bom/manufacturer-canonicals?${q}`)
  const rows = (json.items ?? json.rows ?? []) as Record<string, unknown>[]
  return rows.map((r) => ({
    canonical_id: str(r.canonical_id ?? r.canonicalId),
    display_name: str(r.display_name ?? r.displayName),
    alias_count: num(r.alias_count ?? r.aliasCount),
  }))
}

export async function approveManufacturerAliasCleaning(
  sessionId: string,
  input: { alias: string; canonical_id: string; display_name: string }
): Promise<{ session_line_updated: number; quote_item_updated: number }> {
  return fetchJson(`/api/v1/bom-sessions/${encodeURIComponent(sessionId)}/manufacturer-alias-approvals`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
}

export async function applyManufacturerAliasesToSession(
  sessionId: string
): Promise<{ session_line_updated: number; quote_item_updated: number }> {
  return fetchJson(`/api/v1/bom-sessions/${encodeURIComponent(sessionId)}/manufacturer-aliases/apply`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({}),
  })
}

export async function listManufacturerAliasCandidates(
  sessionId: string
): Promise<ManufacturerAliasCandidate[]> {
  const json = await fetchJson<Record<string, unknown>>(
    `/api/v1/bom-sessions/${encodeURIComponent(sessionId)}/manufacturer-alias-candidates`
  )
  const rows = (json.items ?? []) as Record<string, unknown>[]
  return rows.map((r) => ({
    kind: str(r.kind) === 'demand' ? 'demand' : 'quote',
    alias: str(r.alias),
    recommended_canonical_id: str(r.recommended_canonical_id ?? r.recommendedCanonicalId),
    line_nos: ((r.line_nos ?? r.lineNos ?? []) as unknown[]).map(num).filter((v) => v > 0),
    platform_ids: ((r.platform_ids ?? r.platformIds ?? []) as unknown[]).map(str).filter(Boolean),
    demand_hint: str(r.demand_hint ?? r.demandHint),
  }))
}

export async function listMatchSourceRecords(bomId: string): Promise<MatchSourceRecordsReply> {
  const json = await fetchJson<Record<string, unknown>>(
    `/api/v1/bom/${encodeURIComponent(bomId)}/match-sources`
  )
  const lines = ((json.lines ?? []) as Record<string, unknown>[]).map((line) => ({
    line_no: num(line.line_no ?? line.lineNo),
    mpn: str(line.mpn),
    merge_mpn: str(line.merge_mpn ?? line.mergeMpn),
    quantity: num(line.quantity),
    demand_manufacturer: str(line.demand_manufacturer ?? line.demandManufacturer),
    demand_package: str(line.demand_package ?? line.demandPackage),
    platforms: ((line.platforms ?? []) as Record<string, unknown>[]).map((p) => ({
      platform: str(p.platform),
      cache_hit: bool(p.cache_hit ?? p.cacheHit),
      skip_reason: str(p.skip_reason ?? p.skipReason),
      outcome: str(p.outcome),
      quotes_json_size: num(p.quotes_json_size ?? p.quotesJsonSize),
    })),
  }))
  return {
    biz_date: str(json.biz_date ?? json.bizDate),
    session_platforms: ((json.session_platforms ?? json.sessionPlatforms ?? []) as unknown[]).map(str),
    lines,
  }
}

export async function getMatchSourceDetail(
  bomId: string,
  lineNo: number,
  platform: string
): Promise<MatchSourceDetailReply> {
  const json = await fetchJson<Record<string, unknown>>(
    `/api/v1/bom/${encodeURIComponent(bomId)}/match-sources/${encodeURIComponent(String(lineNo))}/${encodeURIComponent(platform)}`
  )
  return {
    merge_mpn: str(json.merge_mpn ?? json.mergeMpn),
    platform: str(json.platform),
    cache_hit: bool(json.cache_hit ?? json.cacheHit),
    skip_reason: str(json.skip_reason ?? json.skipReason),
    outcome: str(json.outcome),
    quotes_json: str(json.quotes_json ?? json.quotesJson),
    quote_row_evals: ((json.quote_row_evals ?? json.quoteRowEvals ?? []) as Record<string, unknown>[]).map((r) => ({
      row_index: num(r.row_index ?? r.rowIndex),
      model_match: bool(r.model_match ?? r.modelMatch),
      package_match: bool(r.package_match ?? r.packageMatch),
      manufacturer_match: bool(r.manufacturer_match ?? r.manufacturerMatch),
      manufacturer_ok: bool(r.manufacturer_ok ?? r.manufacturerOk ?? r.manufacturer_match ?? r.manufacturerMatch),
      passes_bom_filters: bool(r.passes_bom_filters ?? r.passesBomFilters),
      summary: str(r.summary),
      model_reason: str(r.model_reason ?? r.modelReason),
      package_reason: str(r.package_reason ?? r.packageReason),
      manufacturer_reason: str(r.manufacturer_reason ?? r.manufacturerReason),
      reason: str(r.reason),
    })),
    bom_demand_mpn: str(json.bom_demand_mpn ?? json.bomDemandMpn),
    bom_demand_package: str(json.bom_demand_package ?? json.bomDemandPackage),
    bom_demand_manufacturer: str(json.bom_demand_manufacturer ?? json.bomDemandManufacturer),
    no_mpn_detail: str(json.no_mpn_detail ?? json.noMpnDetail),
  }
}

export interface BomLineDemandSnapshot {
  line_no: number
  line_db_id: number
  raw_text: string
  mpn: string
  unified_mpn: string
  reference_designator: string
  substitute_mpn: string
  remark: string
  description: string
  demand_manufacturer: string
  manufacturer_canonical_id: string
  demand_package: string
  quantity: number
  extra_json: string
}

export interface BomQuoteItemReadRow {
  platform: string
  quote_id: number
  item_id: number
  model: string
  manufacturer: string
  manufacturer_canonical_id: string
  package: string
  stock: string
  desc: string
  moq: string
  lead_time: string
  price_tiers: string
  hk_price: string
  mainland_price: string
  query_model: string
  datasheet_url: string
  source_type: string
  session_id: string
  line_id: number
}

export interface BomLineQuoteItemsReply {
  biz_date: string
  merge_mpn: string
  demand: BomLineDemandSnapshot
  items: BomQuoteItemReadRow[]
}

export async function getBomLineQuoteItems(bomId: string, lineNo: number): Promise<BomLineQuoteItemsReply> {
  const json = await fetchJson<Record<string, unknown>>(
    `/api/v1/bom/${encodeURIComponent(bomId)}/lines/${encodeURIComponent(String(lineNo))}/quote-items`
  )
  const d = (json.demand ?? {}) as Record<string, unknown>
  const demand: BomLineDemandSnapshot = {
    line_no: num(d.line_no ?? d.lineNo),
    line_db_id: num(d.line_db_id ?? d.lineDbId),
    raw_text: str(d.raw_text ?? d.rawText),
    mpn: str(d.mpn),
    unified_mpn: str(d.unified_mpn ?? d.unifiedMpn),
    reference_designator: str(d.reference_designator ?? d.referenceDesignator),
    substitute_mpn: str(d.substitute_mpn ?? d.substituteMpn),
    remark: str(d.remark),
    description: str(d.description),
    demand_manufacturer: str(d.demand_manufacturer ?? d.demandManufacturer),
    manufacturer_canonical_id: str(d.manufacturer_canonical_id ?? d.manufacturerCanonicalId),
    demand_package: str(d.demand_package ?? d.demandPackage),
    quantity: Number(d.quantity ?? 0) || 0,
    extra_json: str(d.extra_json ?? d.extraJson),
  }
  const items = ((json.items ?? []) as Record<string, unknown>[]).map((r) => ({
    platform: str(r.platform),
    quote_id: num(r.quote_id ?? r.quoteId),
    item_id: num(r.item_id ?? r.itemId),
    model: str(r.model),
    manufacturer: str(r.manufacturer),
    manufacturer_canonical_id: str(r.manufacturer_canonical_id ?? r.manufacturerCanonicalId),
    package: str(r.package),
    stock: str(r.stock),
    desc: str(r.desc),
    moq: str(r.moq),
    lead_time: str(r.lead_time ?? r.leadTime),
    price_tiers: str(r.price_tiers ?? r.priceTiers),
    hk_price: str(r.hk_price ?? r.hkPrice),
    mainland_price: str(r.mainland_price ?? r.mainlandPrice),
    query_model: str(r.query_model ?? r.queryModel),
    datasheet_url: str(r.datasheet_url ?? r.datasheetUrl),
    source_type: str(r.source_type ?? r.sourceType),
    session_id: str(r.session_id ?? r.sessionId),
    line_id: num(r.line_id ?? r.lineId),
  }))
  return {
    biz_date: str(json.biz_date ?? json.bizDate),
    merge_mpn: str(json.merge_mpn ?? json.mergeMpn),
    demand,
    items,
  }
}
