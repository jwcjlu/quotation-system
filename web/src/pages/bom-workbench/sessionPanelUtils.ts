import { DEFAULT_PAGE_SIZE } from '../pagination'

export type PageSize = 10 | 20 | 50 | 100

export { DEFAULT_PAGE_SIZE }

export const PAGE_SIZE_OPTIONS: PageSize[] = [10, 20, 50, 100]

export function normalizeKeyword(value: string): string {
  return value.trim().toLowerCase()
}

export function textMatchesKeyword(values: Array<string | number | undefined>, keyword: string): boolean {
  const q = normalizeKeyword(keyword)
  if (!q) return true
  return values.some((value) => String(value ?? '').toLowerCase().includes(q))
}

export function paginateRows<T>(rows: T[], page: number, pageSize: PageSize) {
  const totalPages = Math.max(1, Math.ceil(rows.length / pageSize))
  const currentPage = Math.min(Math.max(1, page), totalPages)
  const start = (currentPage - 1) * pageSize

  return {
    page: currentPage,
    total: rows.length,
    totalPages,
    rows: rows.slice(start, start + pageSize),
  }
}

export function pageSummary(page: number, totalPages: number, total: number): string {
  return `共 ${total} 条 | ${page}/${totalPages}`
}
