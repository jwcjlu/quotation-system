import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { SourcingSessionPage } from './SourcingSessionPage'

const {
  getSession,
  getBOMLines,
  getSessionSearchTaskCoverage,
  listLineGaps,
  listMatchRuns,
  listSessionSearchTasks,
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
  listSessionSearchTasks: vi.fn(),
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
    listSessionSearchTasks,
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

const emptySearchTasks = {
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
    listSessionSearchTasks.mockResolvedValue(emptySearchTasks)
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

  it('shows line availability gaps returned by the API', async () => {
    getSession.mockResolvedValue({
      ...baseSession,
      status: 'data_ready',
      import_status: 'ready',
      import_progress: 100,
      import_stage: 'completed',
      import_message: 'import finished',
    })
    getBOMLines.mockResolvedValue({
      lines: [
        {
          line_id: 'line-1',
          line_no: 1,
          mpn: 'NO-DATA',
          mfr: '',
          package: '',
          qty: 1,
          match_status: '',
          platform_gaps: [],
          availability_status: 'no_data',
          availability_reason: 'NO_DATA_REASON',
          has_usable_quote: false,
          raw_quote_platform_count: 0,
          usable_quote_platform_count: 0,
          resolution_status: 'open',
        },
      ],
    })

    render(<SourcingSessionPage sessionId="session-1" onEnterMatch={vi.fn()} />)

    await act(async () => {
      await flushAsyncWork()
    })

    expect(screen.getByText(/当前 BOM 有/)).toHaveTextContent('1')
    expect(screen.getByText('无数据')).toBeInTheDocument()
    expect(screen.getByText('NO_DATA_REASON')).toBeInTheDocument()
  })

  it('shows search task status and retries retryable tasks', async () => {
    getSession.mockResolvedValue({
      ...baseSession,
      status: 'data_ready',
      import_status: 'ready',
      import_progress: 100,
    })
    listSessionSearchTasks.mockResolvedValue({
      session_id: 'session-1',
      summary: {
        total: 2,
        pending: 0,
        searching: 1,
        succeeded: 0,
        no_data: 0,
        failed: 1,
        skipped: 0,
        cancelled: 0,
        missing: 0,
        retryable: 1,
      },
      tasks: [
        {
          line_id: 'line-1',
          line_no: 1,
          mpn_raw: 'TPS5430DDA',
          mpn_norm: 'TPS5430DDA',
          platform_id: 'hqchip',
          platform_name: 'HQChip',
          search_task_id: 'task-1',
          search_task_state: 'failed_terminal',
          search_ui_state: 'failed',
          retryable: true,
          retry_blocked_reason: '',
          dispatch_task_id: 'dispatch-1',
          dispatch_task_state: 'failed',
          dispatch_agent_id: '',
          dispatch_result: '',
          lease_deadline_at: '',
          attempt: 3,
          retry_max: 4,
          updated_at: '',
          last_error: 'timeout',
        },
        {
          line_id: 'line-2',
          line_no: 2,
          mpn_raw: 'VDRS10P300BSE',
          mpn_norm: 'VDRS10P300BSE',
          platform_id: 'hqchip',
          platform_name: 'HQChip',
          search_task_id: 'task-2',
          search_task_state: 'running',
          search_ui_state: 'searching',
          retryable: false,
          retry_blocked_reason: '',
          dispatch_task_id: 'dispatch-2',
          dispatch_task_state: 'running',
          dispatch_agent_id: '',
          dispatch_result: '',
          lease_deadline_at: '',
          attempt: 1,
          retry_max: 4,
          updated_at: '',
          last_error: '',
        },
      ],
    })

    render(<SourcingSessionPage sessionId="session-1" onEnterMatch={vi.fn()} />)

    await act(async () => {
      await flushAsyncWork()
    })

    expect(screen.getByText('搜索任务状态')).toBeInTheDocument()
    expect(screen.getByText('TPS5430DDA')).toBeInTheDocument()

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: '重试异常任务 (1)' }))
      await flushAsyncWork()
    })

    expect(retrySearchTasks).toHaveBeenCalledWith('session-1', [
      { mpn: 'TPS5430DDA', platform_id: 'hqchip' },
    ])
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
describe('SourcingSessionPage compact layout', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    getBOMLines.mockResolvedValue({ lines: [] })
    getSessionSearchTaskCoverage.mockResolvedValue(emptyCoverage)
    listLineGaps.mockResolvedValue({ gaps: [] })
    listMatchRuns.mockResolvedValue({ runs: [] })
    listSessionSearchTasks.mockResolvedValue(emptySearchTasks)
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.clearAllMocks()
  })

  it('shows the overview tab when there are no anomalies', async () => {
    getSession.mockResolvedValue({
      ...baseSession,
      status: 'data_ready',
      import_status: 'ready',
      import_progress: 100,
    })

    render(<SourcingSessionPage sessionId="session-1" onEnterMatch={vi.fn()} />)

    await act(async () => {
      await flushAsyncWork()
    })

    expect(screen.getByTestId('session-dashboard-tabs')).toBeInTheDocument()
    expect(screen.getByTestId('session-tab-overview')).toHaveAttribute('aria-selected', 'true')
    expect(screen.getByTestId('session-overview-panel')).toBeInTheDocument()
  })

  it('opens the gap panel when unresolved gaps need attention', async () => {
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

    render(<SourcingSessionPage sessionId="session-1" onEnterMatch={vi.fn()} />)

    await act(async () => {
      await flushAsyncWork()
    })

    expect(screen.getByTestId('session-tab-gaps')).toHaveAttribute('aria-selected', 'true')
    expect(screen.getByTestId('session-gaps-panel')).toHaveAttribute('open')
  })

  it('uses tabs and puts abnormal BOM lines first', async () => {
    getSession.mockResolvedValue({
      ...baseSession,
      status: 'data_ready',
      import_status: 'ready',
      import_progress: 100,
    })
    getBOMLines.mockResolvedValue({
      lines: [
        {
          line_id: 'line-1',
          line_no: 1,
          mpn: 'OK-DATA',
          mfr: '',
          package: '',
          qty: 1,
          match_status: '',
          platform_gaps: [],
          availability_status: 'ready',
          availability_reason: '',
          has_usable_quote: true,
          raw_quote_platform_count: 1,
          usable_quote_platform_count: 1,
          resolution_status: '',
        },
        {
          line_id: 'line-2',
          line_no: 2,
          mpn: 'NO-DATA',
          mfr: '',
          package: '',
          qty: 1,
          match_status: '',
          platform_gaps: [],
          availability_status: 'no_data',
          availability_reason: 'NO_DATA_REASON',
          has_usable_quote: false,
          raw_quote_platform_count: 0,
          usable_quote_platform_count: 0,
          resolution_status: 'open',
        },
      ],
    })

    render(<SourcingSessionPage sessionId="session-1" onEnterMatch={vi.fn()} />)

    await act(async () => {
      await flushAsyncWork()
    })

    expect(screen.getByTestId('session-dashboard-tabs')).toBeInTheDocument()
    expect(screen.getByTestId('session-tab-lines')).toHaveAttribute('aria-selected', 'true')
    expect(screen.getAllByTestId('session-line-mpn').map((el) => el.textContent)).toEqual([
      'NO-DATA',
      'OK-DATA',
    ])
  })
})
