import { fetchJson } from './http'

export interface HsResolveCandidate {
  candidate_rank: number
  code_ts: string
  score: number
  reason: string
}

export interface HsResolveByModelParams {
  model: string
  manufacturer?: string
  request_trace_id?: string
  force_refresh?: boolean
  manual_component_description?: string
  manual_upload_id?: string
}

export interface HsResolveReply {
  accepted: boolean
  task_id: string
  run_id: string
  decision_mode: string
  task_status: string
  result_status: string
  best_code_ts: string
  best_score: number
  candidates: HsResolveCandidate[]
  error_code: string
  error_message: string
}

export interface UploadHsManualDatasheetReply {
  upload_id: string
  expires_at_unix: number
  content_sha256: string
}

export interface HsResolveConfirmParams {
  model: string
  manufacturer: string
  run_id: string
  candidate_rank: number
  expected_code_ts: string
  confirm_request_id: string
}

export interface HsBatchResolveLine {
  line_no: number
  model: string
  manufacturer?: string
  match_status: string
  hs_code_status: string
}

export interface HsBatchResolveReply {
  accepted_count: number
  skipped_count: number
  failed_count: number
  results: Array<{
    line_no: number
    model: string
    manufacturer: string
    task_id: string
    run_id: string
    task_status: string
    result_status: string
    best_code_ts: string
    best_score: number
    error_code: string
    error_message: string
  }>
}

export interface HsPendingReviewItem {
  run_id: string
  model: string
  manufacturer: string
  task_status: string
  result_status: string
  best_code_ts: string
  best_score: number
  updated_at: string
  candidates: HsResolveCandidate[]
}

export interface HsPendingReviewReply {
  items: HsPendingReviewItem[]
  total: number
}

function asString(value: unknown): string {
  return typeof value === 'string' ? value : value == null ? '' : String(value)
}

function asNumber(value: unknown): number {
  const n = Number(value)
  return Number.isFinite(n) ? n : 0
}

function pickField(row: Record<string, unknown>, snakeKey: string, camelKey: string): unknown {
  if (row[snakeKey] !== undefined) return row[snakeKey]
  return row[camelKey]
}

function normalizeCandidate(input: unknown): HsResolveCandidate {
  const row = (input ?? {}) as Record<string, unknown>
  return {
    candidate_rank: asNumber(pickField(row, 'candidate_rank', 'candidateRank')),
    code_ts: asString(pickField(row, 'code_ts', 'codeTs')),
    score: asNumber(row.score),
    reason: asString(row.reason),
  }
}

function normalizeReply(input: unknown): HsResolveReply {
  const row = (input ?? {}) as Record<string, unknown>
  return {
    accepted: Boolean(row.accepted),
    task_id: asString(pickField(row, 'task_id', 'taskId')),
    run_id: asString(pickField(row, 'run_id', 'runId')),
    decision_mode: asString(pickField(row, 'decision_mode', 'decisionMode')),
    task_status: asString(pickField(row, 'task_status', 'taskStatus')),
    result_status: asString(pickField(row, 'result_status', 'resultStatus')),
    best_code_ts: asString(pickField(row, 'best_code_ts', 'bestCodeTs')),
    best_score: asNumber(pickField(row, 'best_score', 'bestScore')),
    candidates: Array.isArray(row.candidates) ? row.candidates.map(normalizeCandidate) : [],
    error_code: asString(pickField(row, 'error_code', 'errorCode')),
    error_message: asString(pickField(row, 'error_message', 'errorMessage')),
  }
}

function normalizeManualUploadReply(input: unknown): UploadHsManualDatasheetReply {
  const row = (input ?? {}) as Record<string, unknown>
  return {
    upload_id: asString(row.upload_id),
    expires_at_unix: asNumber(row.expires_at_unix),
    content_sha256: asString(row.content_sha256),
  }
}

function buildQuery(params: Record<string, string | number | boolean | undefined>): string {
  const search = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === null || value === '') continue
    search.set(key, String(value))
  }
  const query = search.toString()
  return query ? `?${query}` : ''
}

function shouldFallbackBatchResolve(error: unknown): boolean {
  const msg = error instanceof Error ? error.message.toLowerCase() : String(error).toLowerCase()
  return msg.includes('404') || msg.includes('not found')
}

export async function hsResolveByModel(params: HsResolveByModelParams): Promise<HsResolveReply> {
  const payload = await fetchJson<Record<string, unknown>>('/api/hs/resolve/by-model', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(params),
  })
  return normalizeReply(payload)
}

export async function uploadHsManualDatasheet(file: File): Promise<UploadHsManualDatasheetReply> {
  const form = new FormData()
  form.set('file', file)
  const payload = await fetchJson<Record<string, unknown>>('/api/hs/resolve/manual-datasheet/upload', {
    method: 'POST',
    body: form,
  })
  return normalizeManualUploadReply(payload)
}

export async function hsResolveTask(taskId: string): Promise<HsResolveReply> {
  const payload = await fetchJson<Record<string, unknown>>(`/api/hs/resolve/task${buildQuery({ task_id: taskId })}`)
  return normalizeReply(payload)
}

export async function hsResolveConfirm(params: HsResolveConfirmParams): Promise<Record<string, unknown>> {
  return fetchJson('/api/hs/resolve/confirm', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(params),
  })
}

export async function hsResolveHistory(model: string, manufacturer?: string): Promise<Record<string, unknown>> {
  return fetchJson(`/api/hs/resolve/history${buildQuery({ model, manufacturer })}`)
}

export async function hsBatchResolveByModels(params: {
  session_id: string
  request_id: string
  lines: HsBatchResolveLine[]
}): Promise<HsBatchResolveReply> {
  try {
    const payload = await fetchJson<Record<string, unknown>>('/api/hs/resolve/by-models:batch', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(params),
    })
    const row = (payload ?? {}) as Record<string, unknown>
    return {
      accepted_count: asNumber(row.accepted_count),
      skipped_count: asNumber(row.skipped_count),
      failed_count: asNumber(row.failed_count),
      results: Array.isArray(row.results)
        ? row.results.map((it) => {
            const r = (it ?? {}) as Record<string, unknown>
            return {
            line_no: asNumber(pickField(r, 'line_no', 'lineNo')),
              model: asString(r.model),
              manufacturer: asString(r.manufacturer),
            task_id: asString(pickField(r, 'task_id', 'taskId')),
            run_id: asString(pickField(r, 'run_id', 'runId')),
            task_status: asString(pickField(r, 'task_status', 'taskStatus')),
            result_status: asString(pickField(r, 'result_status', 'resultStatus')),
            best_code_ts: asString(pickField(r, 'best_code_ts', 'bestCodeTs')),
            best_score: asNumber(pickField(r, 'best_score', 'bestScore')),
            error_code: asString(pickField(r, 'error_code', 'errorCode')),
            error_message: asString(pickField(r, 'error_message', 'errorMessage')),
            }
          })
        : [],
    }
  } catch (error) {
    if (!shouldFallbackBatchResolve(error)) {
      throw error
    }
    const reply: HsBatchResolveReply = {
      accepted_count: 0,
      skipped_count: 0,
      failed_count: 0,
      results: [],
    }
    for (const line of params.lines) {
      const model = (line.model || '').trim()
      const manufacturer = (line.manufacturer || '').trim()
      const matchStatus = (line.match_status || '').trim()
      const hsCodeStatus = (line.hs_code_status || '').trim()
      if (!model || matchStatus !== 'exact' || hsCodeStatus === 'hs_found') {
        reply.skipped_count += 1
        reply.results.push({
          line_no: line.line_no,
          model,
          manufacturer,
          task_id: '',
          run_id: '',
          task_status: '',
          result_status: '',
          best_code_ts: '',
          best_score: 0,
          error_code: 'HS_RESOLVE_SKIPPED',
          error_message: 'line is not eligible',
        })
        continue
      }
      try {
        const r = await hsResolveByModel({
          model,
          manufacturer,
          request_trace_id: `${params.request_id}:${line.line_no}`,
        })
        const failed = !!r.error_code || r.task_status === 'failed'
        if (failed) {
          reply.failed_count += 1
        } else {
          reply.accepted_count += 1
        }
        reply.results.push({
          line_no: line.line_no,
          model,
          manufacturer,
          task_id: r.task_id,
          run_id: r.run_id,
          task_status: r.task_status,
          result_status: r.result_status,
          best_code_ts: r.best_code_ts,
          best_score: r.best_score,
          error_code: r.error_code,
          error_message: r.error_message,
        })
      } catch (lineError) {
        reply.failed_count += 1
        reply.results.push({
          line_no: line.line_no,
          model,
          manufacturer,
          task_id: '',
          run_id: '',
          task_status: '',
          result_status: '',
          best_code_ts: '',
          best_score: 0,
          error_code: 'HS_RESOLVE_FAILED',
          error_message: lineError instanceof Error ? lineError.message : String(lineError),
        })
      }
    }
    return reply
  }
}

export async function hsListPendingReviews(params: {
  page?: number
  page_size?: number
  model?: string
  manufacturer?: string
}): Promise<HsPendingReviewReply> {
  const payload = await fetchJson<Record<string, unknown>>(
    `/api/hs/resolve/pending-reviews${buildQuery(params)}`
  )
  const row = (payload ?? {}) as Record<string, unknown>
  return {
    items: Array.isArray(row.items)
      ? row.items.map((it) => {
          const r = (it ?? {}) as Record<string, unknown>
          return {
            run_id: asString(pickField(r, 'run_id', 'runId')),
            model: asString(r.model),
            manufacturer: asString(r.manufacturer),
            task_status: asString(pickField(r, 'task_status', 'taskStatus')),
            result_status: asString(pickField(r, 'result_status', 'resultStatus')),
            best_code_ts: asString(pickField(r, 'best_code_ts', 'bestCodeTs')),
            best_score: asNumber(pickField(r, 'best_score', 'bestScore')),
            updated_at: asString(pickField(r, 'updated_at', 'updatedAt')),
            candidates: Array.isArray(r.candidates) ? r.candidates.map(normalizeCandidate) : [],
          }
        })
      : [],
    total: asNumber(row.total),
  }
}
