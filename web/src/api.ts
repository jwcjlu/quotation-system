const API_BASE = '/api/v1/bom'

export interface ParsedItem {
  index: number
  raw: string
  model: string
  manufacturer: string
  package: string
  quantity: number
  params: string
}

export interface PlatformQuote {
  platform: string
  matched_model: string
  manufacturer: string
  package: string
  description: string
  stock: number
  lead_time: string
  moq: number
  increment: number
  price_tiers: string
  hk_price: string
  mainland_price: string
  unit_price: number
  subtotal: number
}

export interface MatchItem {
  index: number
  model: string
  quantity: number
  matched_model: string
  manufacturer: string
  platform: string
  lead_time: string
  stock: number
  unit_price: number
  subtotal: number
  match_status: string
  all_quotes: PlatformQuote[]
  demand_manufacturer: string  // 需求厂牌
  demand_package: string      // 需求封装
}

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
  }
}

export async function downloadTemplate(): Promise<Blob> {
  const res = await fetch(`${API_BASE}/template`)
  if (!res.ok) throw new Error('下载模板失败')
  const json = await res.json()
  const b64 = json.file
  const bin = atob(b64)
  const bytes = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i)
  return new Blob([bytes], { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' })
}

export async function uploadBOM(
  file: File,
  parseMode: string = 'auto',
  columnMapping?: Record<string, string>
): Promise<{ bom_id: string; items: ParsedItem[]; total: number }> {
  const buf = await file.arrayBuffer()
  const bytes = new Uint8Array(buf)
  let binary = ''
  for (let i = 0; i < bytes.length; i++) binary += String.fromCharCode(bytes[i])
  const b64 = btoa(binary)

  const body: Record<string, unknown> = { file: b64, filename: file.name, parse_mode: parseMode }
  if (columnMapping && Object.keys(columnMapping).length > 0) body.column_mapping = columnMapping

  const res = await fetch(`${API_BASE}/upload`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) {
    const err = await res.json().catch(() => ({}))
    throw new Error(err.message || '上传失败')
  }
  const json = await res.json()
  return {
    bom_id: json.bom_id ?? json.bomId,
    items: json.items ?? [],
    total: json.total ?? 0,
  }
}

export async function searchQuotes(bomId: string, platforms?: string[]): Promise<{ item_quotes: { model: string; quantity: number; quotes: PlatformQuote[] }[] }> {
  const res = await fetch(`${API_BASE}/search`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ bom_id: bomId, platforms: platforms || [] }),
  })
  if (!res.ok) throw new Error('搜索失败')
  return res.json()
}

export async function autoMatch(bomId: string, strategy: string = 'price_first'): Promise<{ items: MatchItem[]; total_amount: number }> {
  const res = await fetch(`${API_BASE}/match`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ bom_id: bomId, strategy }),
  })
  if (!res.ok) throw new Error('配单失败')
  const json = await res.json()
  const items = (json.items ?? []).map((m: Record<string, unknown>) => normMatchItem(m))
  return { items, total_amount: Number(json.total_amount ?? json.totalAmount ?? 0) }
}

export async function getMatchResult(bomId: string): Promise<{ items: MatchItem[]; total_amount: number }> {
  const res = await fetch(`${API_BASE}/${bomId}/match`)
  if (!res.ok) throw new Error('获取配单结果失败')
  const json = await res.json()
  const items = (json.items ?? []).map((m: Record<string, unknown>) => normMatchItem(m))
  return { items, total_amount: Number(json.total_amount ?? json.totalAmount ?? 0) }
}
