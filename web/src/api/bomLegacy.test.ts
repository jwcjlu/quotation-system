import { describe, expect, it, vi } from 'vitest'

vi.mock('./http', () => ({
  fetchJson: vi.fn(async () => ({
    bom_id: 'session-1',
    accepted: true,
    import_status: 'parsing',
    import_message: 'import started',
    items: [],
    total: 0,
  })),
}))

describe('uploadBOM', () => {
  it('parses async import response fields', async () => {
    const file = {
      name: 'bom.xlsx',
      arrayBuffer: vi.fn(async () => new TextEncoder().encode('bom').buffer),
    } as unknown as File
    const { uploadBOM } = await import('./bomLegacy')

    const result = await uploadBOM(file, 'llm', undefined, { sessionId: 'session-1' })

    expect(result.accepted).toBe(true)
    expect(result.import_status).toBe('parsing')
    expect(result.import_message).toBe('import started')
  })
})
