import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { BomSessionListPage } from './BomSessionListPage'

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
    <button type="button" onClick={() => onSuccess('session-1')}>
      mock upload success
    </button>
  ),
}))

vi.mock('./SourcingSessionPage', () => ({
  SourcingSessionPage: ({ sessionId }: { sessionId: string }) => <div>session detail: {sessionId}</div>,
}))

describe('BomSessionListPage', () => {
  it('does not show the old persistent onboarding card anymore', async () => {
    listSessions.mockResolvedValue({ items: [], total: 0 })

    render(<BomSessionListPage />)

    expect(await screen.findByText('会话列表')).toBeInTheDocument()
    expect(screen.queryByText('新手入门')).not.toBeInTheDocument()
  })

  it('opens session detail immediately after upload success', async () => {
    listSessions.mockResolvedValue({ items: [], total: 0 })

    render(<BomSessionListPage />)

    fireEvent.click(screen.getByRole('button', { name: '新建 BOM 单' }))
    fireEvent.click(screen.getByRole('button', { name: 'mock upload success' }))

    expect(await screen.findByText('session detail: session-1')).toBeInTheDocument()
  })
})
