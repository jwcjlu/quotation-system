import { describe, expect, it, vi } from 'vitest'
import { fetchJson } from './http'

vi.mock('../auth/session', () => ({
  clearSessionToken: vi.fn(),
  getSessionToken: vi.fn(() => null),
}))

describe('fetchJson', () => {
  it('uses nested Kratos error message when present', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({
        ok: false,
        status: 400,
        text: async () => JSON.stringify({ error: { code: 'BAD_REQUEST', message: 'password must be at least 8 characters' } }),
      })),
    )

    await expect(fetchJson('/api/v1/auth/register')).rejects.toThrow('password must be at least 8 characters')
  })
})
