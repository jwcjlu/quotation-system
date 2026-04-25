import { describe, expect, it, vi } from 'vitest'
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

describe('getSession', () => {
  it('parses import progress fields', async () => {
    const { getSession } = await import('./bomSession')
    const result = await getSession('session-1')

    expect(result.import_status).toBe('parsing')
    expect(result.import_progress).toBe(35)
    expect(result.import_stage).toBe('chunk_parsing')
    expect(result.import_message).toBe('chunk 1/3')
    expect(result.import_updated_at).toBe('2026-04-21T12:00:00Z')
  })
})

describe('listSessionSearchTasks', () => {
  it('parses snake_case and camelCase fields', async () => {
    vi.mocked(fetchJson).mockResolvedValueOnce({
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
