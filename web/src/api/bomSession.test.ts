import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fetchJson } from './http'

vi.mock('./http', () => ({
  fetchJson: vi.fn(async () => ({
    session_id: 'session-1',
    title: 'Session 1',
    status: 'draft',
    biz_date: '2026-04-21',
    selection_revision: 1,
    platform_ids: ['digikey'],
    import_status: 'parsing',
    import_progress: 35,
    import_stage: 'chunk_parsing',
    import_message: 'chunk 1/3',
    import_error_code: '',
    import_error: '',
    import_updated_at: '2026-04-21T12:00:00Z',
  })),
}))

const fetchJsonMock = vi.mocked(fetchJson)

beforeEach(() => {
  fetchJsonMock.mockReset()
})

describe('getSession', () => {
  it('parses import progress fields', async () => {
    fetchJsonMock.mockResolvedValueOnce({
      session_id: 'session-1',
      title: 'Session 1',
      status: 'draft',
      biz_date: '2026-04-21',
      selection_revision: 1,
      platform_ids: ['digikey'],
      import_status: 'parsing',
      import_progress: 35,
      import_stage: 'chunk_parsing',
      import_message: 'chunk 1/3',
      import_error_code: '',
      import_error: '',
      import_updated_at: '2026-04-21T12:00:00Z',
    })

    const { getSession } = await import('./bomSession')
    const result = await getSession('session-1')

    expect(result.import_status).toBe('parsing')
    expect(result.import_progress).toBe(35)
    expect(result.import_stage).toBe('chunk_parsing')
    expect(result.import_message).toBe('chunk 1/3')
    expect(result.import_updated_at).toBe('2026-04-21T12:00:00Z')
  })
})

describe('getReadiness', () => {
  it('parses line availability summary fields', async () => {
    fetchJsonMock.mockResolvedValueOnce({
      sessionId: 'session-1',
      bizDate: '2026-04-25',
      selectionRevision: 2,
      phase: 'blocked',
      canEnterMatch: false,
      blockReason: 'strict_mode_no_quote_per_line',
      lineTotal: 3,
      readyLineCount: 1,
      gapLineCount: 2,
      noDataLineCount: 1,
      collectionUnavailableLineCount: 0,
      noMatchAfterFilterLineCount: 1,
      collectingLineCount: 0,
      hasStrictBlockingGap: true,
    })

    const { getReadiness } = await import('./bomSession')
    const result = await getReadiness('session-1')

    expect(result.line_total).toBe(3)
    expect(result.ready_line_count).toBe(1)
    expect(result.gap_line_count).toBe(2)
    expect(result.no_data_line_count).toBe(1)
    expect(result.no_match_after_filter_line_count).toBe(1)
    expect(result.has_strict_blocking_gap).toBe(true)
  })

  it('getReadiness appends quote mfr merge query when options set', async () => {
    fetchJsonMock.mockResolvedValueOnce({
      sessionId: 'session-1',
      bizDate: '2026-04-21',
      selectionRevision: 1,
      phase: 'searching',
      canEnterMatch: false,
      blockReason: '',
      lineTotal: 0,
      readyLineCount: 0,
      gapLineCount: 0,
      noDataLineCount: 0,
      collectionUnavailableLineCount: 0,
      noMatchAfterFilterLineCount: 0,
      collectingLineCount: 0,
      hasStrictBlockingGap: false,
      quoteMfrReviewGateOpen: true,
      quoteItemMfrReviewItems: [],
      allPendingQuoteMfrCount: 0,
    })
    const { getReadiness } = await import('./bomSession')
    await getReadiness('session-1', {
      includeQuoteItemMfrReviews: true,
      includeAllPendingQuoteMfr: true,
    })
    expect(fetchJsonMock).toHaveBeenCalledWith(
      '/api/v1/bom-sessions/session-1/readiness?include_quote_item_mfr_reviews=true&include_all_pending_quote_mfr=true',
    )
  })
})

describe('getBOMLines', () => {
  it('parses line availability fields', async () => {
    fetchJsonMock.mockResolvedValueOnce({
      lines: [
        {
          lineId: '101',
          lineNo: 1,
          mpn: 'NO-DATA',
          mfr: '',
          package: '',
          qty: 1,
          matchStatus: '',
          platformGaps: [],
          availabilityStatus: 'no_data',
          availabilityReasonCode: 'NO_DATA',
          availabilityReason: 'all platforms reported no data',
          hasUsableQuote: false,
          rawQuotePlatformCount: 0,
          usableQuotePlatformCount: 0,
          resolutionStatus: 'open',
        },
      ],
    })

    const { getBOMLines } = await import('./bomSession')
    const result = await getBOMLines('session-1')
    const line = result.lines[0]

    expect(line.availability_status).toBe('no_data')
    expect(line.availability_reason_code).toBe('NO_DATA')
    expect(line.availability_reason).toBe('all platforms reported no data')
    expect(line.has_usable_quote).toBe(false)
    expect(line.raw_quote_platform_count).toBe(0)
    expect(line.usable_quote_platform_count).toBe(0)
    expect(line.resolution_status).toBe('open')
  })
})

describe('listSessionSearchTasks', () => {
  it('parses snake_case and camelCase fields', async () => {
    fetchJsonMock.mockResolvedValueOnce({
      sessionId: 'session-1',
      summary: { total: 2, failed: 1, missing: 1, retryable: 2 },
      tasks: [
        {
          lineId: '12',
          lineNo: 12,
          mpnRaw: 'TPS5430DDA',
          mpnNorm: 'TPS5430DDA',
          platformId: 'hqchip',
          searchTaskState: 'failed_terminal',
          searchUiState: 'failed',
          retryable: true,
          dispatchTaskId: 'task-1',
          attempt: 3,
          retryMax: 4,
          lastError: 'timeout',
        },
      ],
    })

    const { listSessionSearchTasks } = await import('./bomSession')
    const result = await listSessionSearchTasks('session-1')

    expect(result.summary.total).toBe(2)
    expect(result.summary.retryable).toBe(2)
    expect(result.tasks[0]).toMatchObject({
      line_id: '12',
      platform_id: 'hqchip',
      search_ui_state: 'failed',
      dispatch_task_id: 'task-1',
      retry_max: 4,
    })
  })
})
