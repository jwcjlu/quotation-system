import { describe, expect, it } from 'vitest'

import {
  DEFAULT_PAGE_SIZE,
  PAGE_SIZE_OPTIONS,
  normalizeKeyword,
  paginateRows,
  textMatchesKeyword,
} from './sessionPanelUtils'

describe('sessionPanelUtils', () => {
  it('normalizes keywords for local filtering', () => {
    expect(normalizeKeyword('  STM32F103  ')).toBe('stm32f103')
  })

  it('matches keyword across multiple values', () => {
    expect(textMatchesKeyword([1, 'STM32F103C8T6', 'ST'], 'f103')).toBe(true)
    expect(textMatchesKeyword([1, 'STM32F103C8T6', 'ST'], 'ti')).toBe(false)
    expect(textMatchesKeyword([1, 'STM32F103C8T6', 'ST'], '')).toBe(true)
  })

  it('paginates rows and clamps invalid page numbers', () => {
    const rows = Array.from({ length: 45 }, (_, index) => index + 1)

    expect(paginateRows(rows, 2, 20)).toEqual({
      page: 2,
      total: 45,
      totalPages: 3,
      rows: rows.slice(20, 40),
    })
    expect(paginateRows(rows, 9, 20).page).toBe(3)
    expect(paginateRows(rows, -1, 20).page).toBe(1)
  })

  it('defaults paginated workbench tables to ten rows', () => {
    expect(DEFAULT_PAGE_SIZE).toBe(10)
    expect(PAGE_SIZE_OPTIONS[0]).toBe(10)
    expect(paginateRows(Array.from({ length: 12 }, (_, index) => index), 1, DEFAULT_PAGE_SIZE).rows).toHaveLength(10)
  })
})
