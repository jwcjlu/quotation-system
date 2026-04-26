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

vi.mock('./SourcingSessionPage', () => ({
  SourcingSessionPage: ({ sessionId }: { sessionId: string }) => <div>session detail: {sessionId}</div>,
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

  it('shows workbench tabs for the selected session', async () => {
    render(<BomWorkbenchPage />)

    await act(async () => {
      await flushAsyncWork()
    })

    fireEvent.click(screen.getByRole('button', { name: /Alpha BOM/ }))

    expect(await screen.findByRole('tab', { name: '\u6982\u89c8' })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'BOM\u884c' })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: '\u641c\u7d22\u6e05\u6d17' })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: '\u7f3a\u53e3\u5904\u7406' })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: '\u7ef4\u62a4' })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: '\u5339\u914d\u7ed3\u679c' })).toBeInTheDocument()
  })

  it('opens the BOM line workspace inside the selected session', async () => {
    render(<BomWorkbenchPage />)

    await act(async () => {
      await flushAsyncWork()
    })

    fireEvent.click(screen.getByRole('button', { name: /Alpha BOM/ }))
    fireEvent.click(await screen.findByRole('tab', { name: 'BOM\u884c' }))

    expect(screen.getByText('session detail: session-1')).toBeInTheDocument()
  })
})
