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
    mfr_mismatch_quote_manufacturers: (() => {
      const raw = m.mfr_mismatch_quote_manufacturers ?? m.mfrMismatchQuoteManufacturers
      if (!Array.isArray(raw)) return undefined
      return raw.map((x) => String(x ?? ''))
    })(),
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
