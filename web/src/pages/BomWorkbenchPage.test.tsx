import { act, fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { BomWorkbenchPage } from './BomWorkbenchPage'

const { listSessions } = vi.hoisted(() => ({
  listSessions: vi.fn(),
}))

vi.mock('../api', async () => {
  const actual = await vi.importActual<Record<string, unknown>>('../api')
  return {
    ...actual,
    listSessions,
  }
})

vi.mock('./UploadPage', () => ({
  UploadPage: ({ onSuccess }: { onSuccess: (bomId: string) => void }) => (
    <button type="button" onClick={() => onSuccess('session-created')}>
      mock upload success
    </button>
  ),
}))

async function flushAsyncWork() {
  await Promise.resolve()
  await Promise.resolve()
}

describe('BomWorkbenchPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    listSessions.mockResolvedValue({
      total: 1,
      items: [
        {
          session_id: 'session-1',
          title: 'Alpha BOM',
          customer_name: '',
          status: 'searching',
          biz_date: '2026-04-21',
          updated_at: '2026-04-21T13:55:00+08:00',
          line_count: 48,
        },
      ],
    })
  })

  it('selects a session from the left list and opens the workspace on the right', async () => {
    render(<BomWorkbenchPage />)

    await act(async () => {
      await flushAsyncWork()
    })

    fireEvent.click(screen.getByRole('button', { name: /Alpha BOM/ }))

    expect(localStorage.getItem('bom_last_session_id')).toBe('session-1')
    expect(localStorage.getItem('bom_last_bom_id')).toBe('session-1')
    expect(screen.getByTestId('session-workspace-placeholder')).toHaveTextContent('session-1')
  })
})
