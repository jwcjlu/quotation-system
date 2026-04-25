import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { SourcingSessionPage } from './SourcingSessionPage'

const {
  getSession,
  getBOMLines,
  getSessionSearchTaskCoverage,
  listLineGaps,
  listMatchRuns,
  createSessionLine,
  deleteSessionLine,
  exportSessionFile,
  patchSession,
  patchSessionLine,
  putPlatforms,
  retrySearchTasks,
  resolveLineGapManualQuote,
  saveMatchRun,
  selectLineGapSubstitute,
} = vi.hoisted(() => ({
  getSession: vi.fn(),
  getBOMLines: vi.fn(),
  getSessionSearchTaskCoverage: vi.fn(),
  listLineGaps: vi.fn(),
  listMatchRuns: vi.fn(),
  createSessionLine: vi.fn(),
  deleteSessionLine: vi.fn(),
  exportSessionFile: vi.fn(),
  patchSession: vi.fn(),
  patchSessionLine: vi.fn(),
  putPlatforms: vi.fn(),
  retrySearchTasks: vi.fn(),
  resolveLineGapManualQuote: vi.fn(),
  saveMatchRun: vi.fn(),
  selectLineGapSubstitute: vi.fn(),
}))

vi.mock('../api', async () => {
  const actual = await vi.importActual<Record<string, unknown>>('../api')
  return {
    ...actual,
    PLATFORM_IDS: ['digikey'],
    getSession,
    getBOMLines,
    getSessionSearchTaskCoverage,
    listLineGaps,
    listMatchRuns,
    createSessionLine,
    deleteSessionLine,
    exportSessionFile,
    patchSession,
    patchSessionLine,
    putPlatforms,
    retrySearchTasks,
    resolveLineGapManualQuote,
    saveMatchRun,
    selectLineGapSubstitute,
  }
})

const baseSession = {
  session_id: 'session-1',
  title: 'Session 1',
  status: 'draft',
  biz_date: '2026-04-21',
  selection_revision: 1,
  platform_ids: ['digikey'],
  customer_name: '',
  contact_phone: '',
  contact_email: '',
  contact_extra: '',
  import_status: 'parsing',
  import_progress: 35,
  import_stage: 'chunk_parsing',
  import_message: 'chunk 1/3',
  import_error_code: '',
  import_error: '',
  import_updated_at: '2026-04-21T12:00:00Z',
}

const emptyCoverage = {
  consistent: true,
  orphan_task_count: 0,
  expected_task_count: 0,
  existing_task_count: 0,
  missing_tasks: [],
}

async function flushAsyncWork() {
  await Promise.resolve()
  await Promise.resolve()
}

describe('SourcingSessionPage', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    getBOMLines.mockResolvedValue({ lines: [] })
    getSessionSearchTaskCoverage.mockResolvedValue(emptyCoverage)
    listLineGaps.mockResolvedValue({ gaps: [] })
    listMatchRuns.mockResolvedValue({ runs: [] })
    createSessionLine.mockResolvedValue(undefined)
    deleteSessionLine.mockResolvedValue(undefined)
    exportSessionFile.mockResolvedValue({ blob: new Blob(), filename: 'session.xlsx' })
    patchSession.mockResolvedValue(baseSession)
    patchSessionLine.mockResolvedValue(undefined)
    putPlatforms.mockResolvedValue({ selection_revision: 1 })
    retrySearchTasks.mockResolvedValue(undefined)
    resolveLineGapManualQuote.mockResolvedValue({ accepted: true })
    saveMatchRun.mockResolvedValue({ run_id: '1', run_no: 1 })
    selectLineGapSubstitute.mockResolvedValue({ accepted: true })
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.clearAllMocks()
  })

  it('polls session progress while import is parsing', async () => {
    getSession
      .mockResolvedValueOnce(baseSession)
      .mockResolvedValueOnce({
        ...baseSession,
        import_progress: 60,
        import_message: 'chunk 2/3',
      })

    render(<SourcingSessionPage sessionId="session-1" onEnterMatch={vi.fn()} />)

    await act(async () => {
      await flushAsyncWork()
    })

    expect(screen.getByText('BOM 导入进度')).toBeInTheDocument()
    expect(screen.getByText('导入中')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '配单' })).toBeDisabled()

    await act(async () => {
      vi.advanceTimersByTime(2000)
      await flushAsyncWork()
    })

    expect(getSession).toHaveBeenCalledTimes(2)
    expect(screen.getByText('chunk 2/3')).toBeInTheDocument()
  })

  it('stops polling and refreshes lines when import becomes ready', async () => {
    getSession
      .mockResolvedValueOnce(baseSession)
      .mockResolvedValueOnce({
        ...baseSession,
        status: 'data_ready',
        import_status: 'ready',
        import_progress: 100,
        import_stage: 'completed',
        import_message: 'import finished',
      })

    render(<SourcingSessionPage sessionId="session-1" onEnterMatch={vi.fn()} />)

    await act(async () => {
      await flushAsyncWork()
    })

    expect(screen.getByText('导入中')).toBeInTheDocument()
    expect(getBOMLines).toHaveBeenCalledTimes(1)

    await act(async () => {
      vi.advanceTimersByTime(2000)
      await flushAsyncWork()
    })

    expect(screen.getByText('导入完成')).toBeInTheDocument()
    expect(getBOMLines).toHaveBeenCalledTimes(2)
    expect(screen.getByRole('button', { name: '配单' })).not.toBeDisabled()

    await act(async () => {
      vi.advanceTimersByTime(4000)
      await flushAsyncWork()
    })

    expect(getSession).toHaveBeenCalledTimes(2)
  })

  it('shows failed state and stops polling when import fails', async () => {
    getSession
      .mockResolvedValueOnce(baseSession)
      .mockResolvedValueOnce({
        ...baseSession,
        import_status: 'failed',
        import_stage: 'failed',
        import_message: 'parse failed',
        import_error_code: 'LLM_IMPORT_FAILED',
        import_error: 'parser could not understand the sheet',
      })

    render(<SourcingSessionPage sessionId="session-1" onEnterMatch={vi.fn()} />)

    await act(async () => {
      await flushAsyncWork()
    })

    expect(screen.getByText('导入中')).toBeInTheDocument()

    await act(async () => {
      vi.advanceTimersByTime(2000)
      await flushAsyncWork()
    })

    expect(screen.getByText('导入失败')).toBeInTheDocument()
    expect(screen.getByText('LLM_IMPORT_FAILED')).toBeInTheDocument()
    expect(screen.getByText('parser could not understand the sheet')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '配单' })).toBeDisabled()

    await act(async () => {
      vi.advanceTimersByTime(4000)
      await flushAsyncWork()
    })

    expect(getSession).toHaveBeenCalledTimes(2)
  })

  it('shows open gaps and saves a match run', async () => {
    getSession.mockResolvedValue({
      ...baseSession,
      status: 'data_ready',
      import_status: 'ready',
      import_progress: 100,
    })
    listLineGaps.mockResolvedValue({
      gaps: [
        {
          gap_id: '99',
          session_id: 'session-1',
          line_id: '2',
          line_no: 2,
          mpn: 'NO-DATA',
          gap_type: 'NO_DATA',
          reason_code: 'NO_DATA',
          reason_detail: 'all selected platforms returned no data',
          resolution_status: 'open',
          substitute_mpn: '',
          substitute_reason: '',
          updated_at: '',
        },
      ],
    })
    listMatchRuns.mockResolvedValueOnce({ runs: [] }).mockResolvedValueOnce({
      runs: [
        {
          run_id: '7',
          run_no: 1,
          session_id: 'session-1',
          status: 'saved',
          line_total: 2,
          matched_line_count: 1,
          unresolved_line_count: 1,
          total_amount: 10,
          currency: 'CNY',
          created_at: '',
          saved_at: '',
        },
      ],
    })
    saveMatchRun.mockResolvedValue({ run_id: '7', run_no: 1 })

    render(<SourcingSessionPage sessionId="session-1" onEnterMatch={vi.fn()} />)

    await act(async () => {
      await flushAsyncWork()
    })

    expect(screen.getByText('NO-DATA')).toBeInTheDocument()

    await act(async () => {
      fireEvent.click(screen.getAllByRole('button', { name: '保存配单方案' })[0])
      await flushAsyncWork()
    })

    expect(saveMatchRun).toHaveBeenCalledWith('session-1')
    expect(screen.getAllByText(/配单 V1/).length).toBeGreaterThan(0)
  })
})
