import { describe, expect, it } from 'vitest'
import type { BomQuoteItemReadRow } from '../../api'
import {
  filterQuoteItems,
  parsePriceLoose,
  parseStockNumber,
  parseStockThreshold,
  sortQuoteItems,
} from './SessionLineDemandQuoteItemsModal.table'

const row = (over: Partial<BomQuoteItemReadRow>): BomQuoteItemReadRow => ({
  platform: 'p1',
  quote_id: 1,
  item_id: 1,
  model: 'X',
  manufacturer: 'TI',
  manufacturer_canonical_id: '',
  package: 'SOP8',
  stock: '12,000',
  desc: '',
  moq: '',
  lead_time: '3d',
  price_tiers: '',
  hk_price: '0.5',
  mainland_price: '¥1.2',
  query_model: '',
  datasheet_url: '',
  source_type: '',
  session_id: '',
  line_id: 0,
  ...over,
})

describe('SessionLineDemandQuoteItemsModal.table', () => {
  it('parseStockNumber handles comma groups', () => {
    expect(parseStockNumber('12,000')).toBe(12000)
    expect(parseStockNumber('0')).toBe(0)
    expect(parseStockNumber('')).toBeNaN()
  })

  it('parsePriceLoose reads first number', () => {
    expect(parsePriceLoose('¥1.2')).toBeCloseTo(1.2)
    expect(parsePriceLoose('0.5')).toBeCloseTo(0.5)
  })

  it('parseStockThreshold', () => {
    expect(parseStockThreshold('1,000')).toBe(1000)
    expect(parseStockThreshold(' 500 ')).toBe(500)
    expect(parseStockThreshold('abc')).toBeNull()
  })

  it('filterQuoteItems: stock search is numeric >= threshold', () => {
    const items = [
      row({ item_id: 1, stock: '1000', model: 'ABC' }),
      row({ item_id: 2, stock: '200', model: 'DEF' }),
      row({ item_id: 3, stock: '12,000', model: 'GHI' }),
    ]
    expect(filterQuoteItems(items, 'DEF', '').length).toBe(1)
    expect(filterQuoteItems(items, '', '1000').map((r) => r.item_id).sort()).toEqual([1, 3])
    expect(filterQuoteItems(items, '', '5000').map((r) => r.item_id)).toEqual([3])
    expect(filterQuoteItems(items, 'ABC', '500').length).toBe(1)
  })

  it('filterQuoteItems: non-numeric stock filter falls back to substring', () => {
    const items = [row({ item_id: 1, stock: '1K pcs' }), row({ item_id: 2, stock: '900' })]
    expect(filterQuoteItems(items, '', 'K').length).toBe(1)
  })

  it('sortQuoteItems by stock numeric', () => {
    const items = [row({ item_id: 1, stock: '9' }), row({ item_id: 2, stock: '100' })]
    const asc = sortQuoteItems(items, 'stock', 'asc')
    expect(asc[0].stock).toBe('9')
    expect(asc[1].stock).toBe('100')
  })
})
