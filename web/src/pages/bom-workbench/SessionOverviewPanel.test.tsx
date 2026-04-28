import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { SessionOverviewPanel } from './SessionOverviewPanel'

const { listSessionSearchTasks } = vi.hoisted(() => ({
  listSessionSearchTasks: vi.fn(),
}))

vi.mock('../../api', async () => {
  const actual = await vi.importActual<Record<string, unknown>>('../../api')
  return {
    ...actual,
    listSessionSearchTasks,
  }
})

describe('SessionOverviewPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    listSessionSearchTasks.mockResolvedValue({
      session_id: 'session-1',
      summary: {
        total: 12,
        pending: 1,
        searching: 2,
        succeeded: 8,
        no_data: 0,
        failed: 1,
        skipped: 0,
        cancelled: 0,
        missing: 0,
        retryable: 3,
      },
      tasks: [],
    })
  })

  it('shows the search task summary as an overview card', async () => {
    render(<SessionOverviewPanel sessionId="session-1" sessionStatus="data_ready" lineCount={48} />)

    expect(screen.getByText('\u641c\u7d22\u4efb\u52a1')).toBeInTheDocument()
    expect(await screen.findByText('12')).toBeInTheDocument()
    expect(
      screen.getByText((text) => text.includes('2 \u5904\u7406\u4e2d / 3 \u53ef\u91cd\u8bd5'))
    ).toBeInTheDocument()
    expect(listSessionSearchTasks).toHaveBeenCalledWith('session-1')
  })
})
