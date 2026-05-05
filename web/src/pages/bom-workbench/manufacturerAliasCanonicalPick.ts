import type { ManufacturerCanonicalRow } from '../../api'

function normKey(s: string): string {
  return s.trim().toLowerCase()
}

/** 用于与标准厂牌 display_name / canonical_id 做子串匹配的若干针串（已小写）。 */
export function canonicalMatchNeedles(alias: string, demandHint: string): string[] {
  const out: string[] = []
  const a = normKey(alias)
  const h = normKey(demandHint)
  if (a.length >= 2) {
    out.push(a)
  }
  if (a.includes('(')) {
    const head = a.split('(')[0].trim()
    if (head.length >= 2) {
      out.push(head)
    }
  }
  const atok = a.split(/\s+/)[0] ?? ''
  if (atok.length >= 3) {
    out.push(atok)
  }
  if (h.length >= 2) {
    out.push(h)
  }
  const htok = h.split(/\s+/)[0] ?? ''
  if (htok.length >= 3 && htok !== h) {
    out.push(htok)
  }
  return [...new Set(out.filter(Boolean))]
}

export function scoreCanonicalMatchRow(
  alias: string,
  demandHint: string,
  row: ManufacturerCanonicalRow,
): number {
  const hay = normKey(`${row.display_name} ${row.canonical_id}`)
  let score = 0
  for (const n of canonicalMatchNeedles(alias, demandHint)) {
    if (hay === n) {
      score += 520
      continue
    }
    if (hay.startsWith(n + ' ') || hay.startsWith(n + '(') || hay.startsWith(n + '/')) {
      score += 300
      continue
    }
    if (hay.includes(n)) {
      score += Math.min(160, 14 * n.length)
    }
  }
  return score
}

/** 推荐 ID 在列表中则用之；否则选与 alias / 需求提示匹配度最高的一条（低于阈值则不自动选）。 */
export function pickBestCanonicalMatch(
  alias: string,
  demandHint: string,
  recommendedCanonicalId: string,
  canonicalRows: ManufacturerCanonicalRow[],
  minScore = 48,
): string {
  const rec = recommendedCanonicalId.trim()
  if (rec && canonicalRows.some((r) => r.canonical_id === rec)) {
    return rec
  }
  let bestId = ''
  let bestScore = 0
  for (const row of canonicalRows) {
    const s = scoreCanonicalMatchRow(alias, demandHint, row)
    if (s > bestScore) {
      bestScore = s
      bestId = row.canonical_id
    }
  }
  if (bestScore < minScore) {
    return ''
  }
  return bestId
}
