import type { BomQuoteItemReadRow } from '../../api'

export type QuoteItemSortKey =
  | 'platform'
  | 'model'
  | 'mfr'
  | 'pkg'
  | 'stock'
  | 'lead'
  | 'mainland'
  | 'hk'

/** 从库存类字符串中抽出首个整数，用于排序与库存筛选 */
export function parseStockNumber(raw: string): number {
  const m = String(raw || '').replace(/,/g, '').match(/\d+/g)
  if (!m || m.length === 0) return Number.NaN
  const n = parseInt(m.join('').slice(0, 15), 10)
  return Number.isFinite(n) ? n : Number.NaN
}

/** 库存搜索框输入解析为阈值（去逗号、trim）；非数字返回 null */
export function parseStockThreshold(raw: string): number | null {
  const s = String(raw || '').trim().replace(/,/g, '')
  if (!s) return null
  const n = Number(s)
  if (!Number.isFinite(n)) return null
  return n
}

/** 从价档/文本中抽出首个可解析小数 */
export function parsePriceLoose(raw: string): number {
  const s = String(raw || '').replace(/,/g, '')
  const m = s.match(/\d+(?:\.\d+)?/)
  if (!m) return Number.NaN
  const n = parseFloat(m[0])
  return Number.isFinite(n) ? n : Number.NaN
}

function rowMatchesKeyword(it: BomQuoteItemReadRow, q: string): boolean {
  if (!q.trim()) return true
  const hay = [
    it.platform,
    it.model,
    it.manufacturer,
    it.package,
    it.stock,
    it.lead_time,
    it.mainland_price,
    it.hk_price,
  ]
    .join('\n')
    .toLowerCase()
  return hay.includes(q.trim().toLowerCase())
}

function rowMatchesStock(it: BomQuoteItemReadRow, stockQ: string): boolean {
  const t = stockQ.trim()
  if (!t) return true
  const threshold = parseStockThreshold(t)
  if (threshold === null) {
    return String(it.stock || '')
      .toLowerCase()
      .includes(t.toLowerCase())
  }
  const v = parseStockNumber(it.stock)
  if (Number.isNaN(v)) return false
  return v >= threshold
}

export function filterQuoteItems(
  items: BomQuoteItemReadRow[],
  keyword: string,
  stockKeyword: string
): BomQuoteItemReadRow[] {
  return items.filter((it) => rowMatchesKeyword(it, keyword) && rowMatchesStock(it, stockKeyword))
}

function compareStrings(a: string, b: string, dir: 'asc' | 'desc'): number {
  const c = a.localeCompare(b, 'zh-CN')
  return dir === 'asc' ? c : -c
}

function compareNums(a: number, b: number, dir: 'asc' | 'desc'): number {
  const aa = Number.isNaN(a) ? (dir === 'asc' ? Number.POSITIVE_INFINITY : Number.NEGATIVE_INFINITY) : a
  const bb = Number.isNaN(b) ? (dir === 'asc' ? Number.POSITIVE_INFINITY : Number.NEGATIVE_INFINITY) : b
  return dir === 'asc' ? aa - bb : bb - aa
}

export function sortQuoteItems(
  items: BomQuoteItemReadRow[],
  key: QuoteItemSortKey | null,
  dir: 'asc' | 'desc'
): BomQuoteItemReadRow[] {
  if (!key) {
    const next = [...items]
    next.sort((a, b) => {
      const p = a.platform.localeCompare(b.platform, 'zh-CN')
      if (p !== 0) return p
      return a.item_id - b.item_id
    })
    return next
  }
  const next = [...items]
  next.sort((a, b) => {
    switch (key) {
      case 'platform':
        return compareStrings(a.platform || '', b.platform || '', dir)
      case 'model':
        return compareStrings(a.model || '', b.model || '', dir)
      case 'mfr':
        return compareStrings(a.manufacturer || '', b.manufacturer || '', dir)
      case 'pkg':
        return compareStrings(a.package || '', b.package || '', dir)
      case 'stock':
        return compareNums(parseStockNumber(a.stock), parseStockNumber(b.stock), dir)
      case 'lead':
        return compareStrings(a.lead_time || '', b.lead_time || '', dir)
      case 'mainland':
        return compareNums(parsePriceLoose(a.mainland_price), parsePriceLoose(b.mainland_price), dir)
      case 'hk':
        return compareNums(parsePriceLoose(a.hk_price), parsePriceLoose(b.hk_price), dir)
      default:
        return 0
    }
  })
  return next
}
