import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fetchJson } from './http'

vi.mock('./http', () => ({
  fetchJson: vi.fn(async () => ({ items: [] })),
}))

const fetchJsonMock = vi.mocked(fetchJson)

beforeEach(() => {
  fetchJsonMock.mockReset()
})

describe('manufacturer alias API', () => {
  it('lists canonicals through the generated Kratos route', async () => {
    fetchJsonMock.mockResolvedValueOnce({ items: [] })

    const { listManufacturerCanonicals } = await import('./bomMatchExtras')
    await listManufacturerCanonicals(500)

    expect(fetchJsonMock).toHaveBeenCalledWith('/api/v1/bom/manufacturer-canonicals?limit=500')
  })

  it('creates aliases through the generated Kratos route', async () => {
    fetchJsonMock.mockResolvedValueOnce({})

    const { createManufacturerAlias } = await import('./bomMatchExtras')
    await createManufacturerAlias('TI', 'mfr-ti', 'Texas Instruments')

    expect(fetchJsonMock).toHaveBeenCalledWith('/api/v1/bom/manufacturer-aliases', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        alias: 'TI',
        canonical_id: 'mfr-ti',
        display_name: 'Texas Instruments',
      }),
    })
  })

  it('posts session-line-mfr-approvals (phase 1)', async () => {
    fetchJsonMock.mockResolvedValueOnce({ session_line_updated: 2, quote_item_updated: 0 })

    const { approveSessionLineMfrCleaning } = await import('./bomMatchExtras')
    await approveSessionLineMfrCleaning('session-1', {
      alias: 'TI',
      canonical_id: 'MFR_TI',
      display_name: 'Texas Instruments',
    })

    expect(fetchJsonMock).toHaveBeenCalledWith(
      '/api/v1/bom-sessions/session-1/session-line-mfr-approvals',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({
          alias: 'TI',
          canonical_id: 'MFR_TI',
          display_name: 'Texas Instruments',
        }),
      })
    )
  })

  it('applies known manufacturer aliases for current session', async () => {
    fetchJsonMock.mockResolvedValueOnce({ session_line_updated: 2, quote_item_updated: 0 })

    const { applyManufacturerAliasesToSession } = await import('./bomMatchExtras')
    await applyManufacturerAliasesToSession('session-1')

    expect(fetchJsonMock).toHaveBeenCalledWith(
      '/api/v1/bom-sessions/session-1/manufacturer-aliases/apply',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({}),
      })
    )
  })
})

describe('two-phase BOM mfr REST', () => {
  it('GET session-line-mfr-candidates normalizes items', async () => {
    fetchJsonMock.mockResolvedValueOnce({
      items: [{ line_no: 3, mfr: ' ST ', recommended_canonical_id: 'MFR_ST' }],
    })
    const { listSessionLineMfrCandidates } = await import('./bomMatchExtras')
    const reply = await listSessionLineMfrCandidates('sid-1')
    expect(fetchJsonMock).toHaveBeenCalledWith('/api/v1/bom-sessions/sid-1/session-line-mfr-candidates')
    expect(reply.items).toEqual([{ line_no: 3, mfr: ' ST ', recommended_canonical_id: 'MFR_ST' }])
  })

  it('GET quote-item-mfr-reviews normalizes gate_open and items', async () => {
    fetchJsonMock.mockResolvedValueOnce({
      gateOpen: true,
      items: [
        {
          quoteItemId: 9,
          lineNo: 1,
          lineManufacturerCanonicalId: 'MFR_X',
          manufacturer: 'TI',
          platformId: 'ickey',
        },
      ],
    })
    const { listQuoteItemMfrReviews } = await import('./bomMatchExtras')
    const reply = await listQuoteItemMfrReviews('sid-2')
    expect(fetchJsonMock).toHaveBeenCalledWith(
      '/api/v1/bom-sessions/sid-2/quote-item-mfr-reviews',
    )
    expect(reply.gate_open).toBe(true)
    expect(reply.items).toEqual([
      {
        quote_item_id: 9,
        line_no: 1,
        line_manufacturer_canonical_id: 'MFR_X',
        manufacturer: 'TI',
        platform_id: 'ickey',
      },
    ])
  })

  it('GET quote-item-mfr-reviews appends include_all_pending_quote_mfr when requested', async () => {
    fetchJsonMock.mockResolvedValueOnce({ gateOpen: true, items: [], allPendingQuoteMfrCount: 0 })
    const { listQuoteItemMfrReviews } = await import('./bomMatchExtras')
    await listQuoteItemMfrReviews('sid-x', { includeAllPendingQuoteMfr: true })
    expect(fetchJsonMock).toHaveBeenCalledWith(
      '/api/v1/bom-sessions/sid-x/quote-item-mfr-reviews?include_all_pending_quote_mfr=true',
    )
  })

  it('POST quote-item-mfr-reviews sends snake_case body with optional reason', async () => {
    fetchJsonMock.mockResolvedValueOnce({})
    const { submitQuoteItemMfrReview } = await import('./bomMatchExtras')
    await submitQuoteItemMfrReview('sid-3', { quote_item_id: 9, decision: 'reject', reason: ' mismatch ' })
    expect(fetchJsonMock).toHaveBeenCalledWith(
      '/api/v1/bom-sessions/sid-3/quote-item-mfr-reviews',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ quote_item_id: 9, decision: 'reject', reason: ' mismatch ' }),
      })
    )
  })

  it('POST quote-item-mfr-review omits empty reason', async () => {
    fetchJsonMock.mockResolvedValueOnce({})
    const { submitQuoteItemMfrReview } = await import('./bomMatchExtras')
    await submitQuoteItemMfrReview('sid-4', { quote_item_id: 1, decision: 'accept', reason: '  ' })
    expect(fetchJsonMock).toHaveBeenCalledWith(
      '/api/v1/bom-sessions/sid-4/quote-item-mfr-reviews',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ quote_item_id: 1, decision: 'accept' }),
      })
    )
  })
})
