/**
 * 配单历史：/api/v1/bom-match-history（接口清单 §7）
 */
import { fetchJson } from './http'
import type { ListMatchHistoryReply, MatchItem } from './types'

const BASE = '/api/v1/bom-match-history'

function num(v: unknown, fallback = 0): number {
  if (typeof v === 'number' && !Number.isNaN(v)) return v
  if (typeof v === 'string') return Number(v) || fallback
  return fallback
}

export async function listMatchHistory(page = 1, pageSize = 20): Promise<ListMatchHistoryReply> {
  const q = new URLSearchParams()
  q.set('page', String(page))
  q.set('page_size', String(pageSize))
  const json = await fetchJson<Record<string, unknown>>(`${BASE}?${q.toString()}`)
  const raw = (json.items ?? []) as Record<string, unknown>[]
  const items = raw.map((row) => ({
    match_result_id: num(row.match_result_id ?? row.matchResultId, 0),
    session_id: (row.session_id ?? row.sessionId) as string,
    version: num(row.version, 0),
    strategy: (row.strategy as string) ?? '',
    created_at: (row.created_at ?? row.createdAt) as string,
    total_amount: num(row.total_amount ?? row.totalAmount, 0),
  }))
  return { items, total: num(json.total, 0) }
}

function normQuote(q: Record<string, unknown>) {
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
  }
}

export async function getMatchHistory(matchResultId: number): Promise<{
  match_result_id: number
  session_id: string
  version: number
  strategy: string
  created_at: string
  total_amount: number
  items: MatchItem[]
}> {
  const json = await fetchJson<Record<string, unknown>>(`${BASE}/${matchResultId}`)
  const itemsRaw = (json.items ?? []) as Record<string, unknown>[]
  return {
    match_result_id: num(json.match_result_id ?? json.matchResultId, 0),
    session_id: (json.session_id ?? json.sessionId) as string,
    version: num(json.version, 0),
    strategy: (json.strategy as string) ?? '',
    created_at: (json.created_at ?? json.createdAt) as string,
    total_amount: num(json.total_amount ?? json.totalAmount, 0),
    items: itemsRaw.map(normMatchItem),
  }
}
