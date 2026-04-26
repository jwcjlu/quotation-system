import { act, fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { BomWorkbenchPage } from './BomWorkbenchPage'

const {
  listSessions,
  listSessionSearchTasks,
  retrySearchTasks,
  autoMatch,
  listManufacturerAliasCandidates,
  listManufacturerCanonicals,
  getSession,
  getBOMLines,
  getSessionSearchTaskCoverage,
  listLineGaps,
  listMatchRuns,
} = vi.hoisted(() => ({
  listSessions: vi.fn(),
  listSessionSearchTasks: vi.fn(),
  retrySearchTasks: vi.fn(),
  autoMatch: vi.fn(),
  listManufacturerAliasCandidates: vi.fn(),
  listManufacturerCanonicals: vi.fn(),
  getSession: vi.fn(),
  getBOMLines: vi.fn(),
  getSessionSearchTaskCoverage: vi.fn(),
  listLineGaps: vi.fn(),
  listMatchRuns: vi.fn(),
}))

vi.mock('../api', async () => {
  const actual = await vi.importActual<Record<string, unknown>>('../api')
  return {
    ...actual,
    listSessions,
    listSessionSearchTasks,
    retrySearchTasks,
    autoMatch,
    listManufacturerAliasCandidates,
    listManufacturerCanonicals,
    getSession,
    getBOMLines,
    getSessionSearchTaskCoverage,
    listLineGaps,
    listMatchRuns,
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
    listSessionSearchTasks.mockResolvedValue({
      session_id: 'session-1',
      summary: {
        total: 0,
        pending: 0,
        searching: 0,
        succeeded: 0,
        no_data: 0,
        failed: 0,
        skipped: 0,
        cancelled: 0,
        missing: 0,
        retryable: 0,
      },
      tasks: [],
    })
    retrySearchTasks.mockResolvedValue({ accepted: 0 })
    autoMatch.mockResolvedValue({ items: [], total_amount: 0 })
    listManufacturerAliasCandidates.mockResolvedValue([])
    listManufacturerCanonicals.mockResolvedValue([])
    getBOMLines.mockResolvedValue({
      lines: [
        {
          line_id: 'line-1',
          line_no: 1,
          mpn: 'STM32F103C8T6',
          mfr: 'ST',
          package: 'LQFP48',
          qty: 100,
          match_status: 'ready',
          platform_gaps: [],
          availability_status: 'ready',
          has_usable_quote: true,
          raw_quote_platform_count: 2,
          usable_quote_platform_count: 1,
        },
      ],
    })
    getSessionSearchTaskCoverage.mockResolvedValue({
      consistent: true,
      orphan_task_count: 0,
      expected_task_count: 1,
      existing_task_count: 1,
      missing_tasks: [],
    })
    listLineGaps.mockResolvedValue({
      gaps: [
        {
          gap_id: 'gap-1',
          session_id: 'session-1',
          line_id: 'line-1',
          line_no: 1,
          mpn: 'STM32F103C8T6',
          gap_type: 'no_quote',
          reason_code: 'no_data',
          reason_detail: 'missing quote',
          resolution_status: 'open',
          substitute_mpn: '',
          substitute_reason: '',
          updated_at: '2026-04-21T13:55:00+08:00',
        },
      ],
    })
    listMatchRuns.mockResolvedValue({
      runs: [
        {
          run_id: 'run-1',
          run_no: 1,
          session_id: 'session-1',
          status: 'saved',
          line_total: 1,
          matched_line_count: 0,
          unresolved_line_count: 1,
          total_amount: 0,
          currency: 'CNY',
          created_at: '2026-04-21T13:55:00+08:00',
          saved_at: '2026-04-21T13:55:00+08:00',
        },
      ],
    })
    getSession.mockResolvedValue({
      session_id: 'session-1',
      title: 'Alpha BOM',
      status: 'searching',
      biz_date: '2026-04-21',
      selection_revision: 1,
      platform_ids: [],
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

    expect(await screen.findByTestId('session-lines-panel')).toBeInTheDocument()
    expect(screen.getByPlaceholderText('MPN / \u884c\u53f7 / \u63cf\u8ff0')).toBeInTheDocument()
    expect(screen.queryByText('session detail: session-1')).not.toBeInTheDocument()
    expect(getBOMLines).toHaveBeenCalledWith('session-1')
  })

  it('opens search clean tools inside the selected session', async () => {
    render(<BomWorkbenchPage />)

    await act(async () => {
      await flushAsyncWork()
    })

    fireEvent.click(screen.getByRole('button', { name: /Alpha BOM/ }))
    fireEvent.click(await screen.findByRole('tab', { name: '\u641c\u7d22\u6e05\u6d17' }))

    expect(await screen.findByTestId('session-search-clean-panel')).toBeInTheDocument()
    expect(screen.getByTestId('manufacturer-alias-review-panel')).toBeInTheDocument()
    expect(listSessionSearchTasks).toHaveBeenCalledWith('session-1')
    expect(listManufacturerAliasCandidates).toHaveBeenCalledWith('session-1')
    expect(autoMatch).not.toHaveBeenCalled()
  })

  it('opens the gap handling workspace without rendering the full session detail page', async () => {
    render(<BomWorkbenchPage />)

    await act(async () => {
      await flushAsyncWork()
    })

    fireEvent.click(screen.getByRole('button', { name: /Alpha BOM/ }))
    fireEvent.click(await screen.findByRole('tab', { name: '\u7f3a\u53e3\u5904\u7406' }))

    expect(await screen.findByTestId('session-gaps-panel')).toBeInTheDocument()
    expect(screen.getByPlaceholderText('MPN / \u539f\u56e0')).toBeInTheDocument()
    expect(screen.queryByText('session detail: session-1')).not.toBeInTheDocument()
    expect(listLineGaps).toHaveBeenCalledWith('session-1')
    expect(listMatchRuns).toHaveBeenCalledWith('session-1')
  })

  it('opens the maintenance workspace without rendering the full session detail page', async () => {
    render(<BomWorkbenchPage />)

    await act(async () => {
      await flushAsyncWork()
    })

    fireEvent.click(screen.getByRole('button', { name: /Alpha BOM/ }))
    fireEvent.click(await screen.findByRole('tab', { name: '\u7ef4\u62a4' }))

    expect(await screen.findByTestId('session-maintenance-panel')).toBeInTheDocument()
    expect(screen.getByText('\u4f1a\u8bdd\u7ef4\u62a4')).toBeInTheDocument()
    expect(screen.queryByText('session detail: session-1')).not.toBeInTheDocument()
    expect(getSession).toHaveBeenCalledWith('session-1')
  })

  it('disables match result tab until the selected session is data_ready', async () => {
    render(<BomWorkbenchPage />)

    await act(async () => {
      await flushAsyncWork()
    })

    fireEvent.click(screen.getByRole('button', { name: /Alpha BOM/ }))

    const matchTab = await screen.findByRole('tab', { name: '\u5339\u914d\u7ed3\u679c' })
    expect(matchTab).toBeDisabled()
    expect(screen.getByText(/\u4f1a\u8bdd\u72b6\u6001/)).toBeInTheDocument()
  })

  it('opens the match result workspace once the selected session is data_ready', async () => {
    getSession.mockResolvedValue({
      session_id: 'session-1',
      title: 'Alpha BOM',
      status: 'data_ready',
      biz_date: '2026-04-21',
      selection_revision: 1,
      platform_ids: [],
    })

    render(<BomWorkbenchPage />)

    await act(async () => {
      await flushAsyncWork()
    })

    fireEvent.click(screen.getByRole('button', { name: /Alpha BOM/ }))
    const matchTab = await screen.findByRole('tab', { name: '\u5339\u914d\u7ed3\u679c' })
    fireEvent.click(matchTab)

    expect(await screen.findByTestId('session-match-result-panel')).toBeInTheDocument()
    expect(screen.getByPlaceholderText('MPN / \u4f9b\u5e94\u5546 / \u5382\u5bb6')).toBeInTheDocument()
  })

  it('offers a back-to-list action for mobile detail flow', async () => {
    render(<BomWorkbenchPage />)

    await act(async () => {
      await flushAsyncWork()
    })

    fireEvent.click(screen.getByRole('button', { name: /Alpha BOM/ }))

    expect(await screen.findByRole('button', { name: '\u8fd4\u56de\u4f1a\u8bdd\u5217\u8868' })).toBeInTheDocument()
  })
})
