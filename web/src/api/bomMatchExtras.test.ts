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

    expect(fetchJsonMock).toHaveBeenCalledWith(
      '/api/v1/bom/manufacturer-canonicals?limit=500'
    )
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

  it('approves manufacturer alias cleaning for current session', async () => {
    fetchJsonMock.mockResolvedValueOnce({ session_line_updated: 2, quote_item_updated: 3 })

    const { approveManufacturerAliasCleaning } = await import('./bomMatchExtras')
    await approveManufacturerAliasCleaning('session-1', {
      alias: 'TI',
      canonical_id: 'MFR_TI',
      display_name: 'Texas Instruments',
    })

    expect(fetchJsonMock).toHaveBeenCalledWith(
      '/api/v1/bom-sessions/session-1/manufacturer-alias-approvals',
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
    fetchJsonMock.mockResolvedValueOnce({ session_line_updated: 2, quote_item_updated: 3 })

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
