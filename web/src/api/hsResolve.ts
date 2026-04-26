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

export interface HsResolveConfirmParams {
  run_id: string
  selected_code_ts: string
  confirm_note?: string
}

function asString(value: unknown): string {
  return typeof value === 'string' ? value : value == null ? '' : String(value)
}

function asNumber(value: unknown): number {
  const n = Number(value)
  return Number.isFinite(n) ? n : 0
}

function normalizeCandidate(input: unknown): HsResolveCandidate {
  const row = (input ?? {}) as Record<string, unknown>
  return {
    candidate_rank: asNumber(row.candidate_rank),
    code_ts: asString(row.code_ts),
    score: asNumber(row.score),
    reason: asString(row.reason),
  }
}

function normalizeReply(input: unknown): HsResolveReply {
  const row = (input ?? {}) as Record<string, unknown>
  return {
    accepted: Boolean(row.accepted),
    task_id: asString(row.task_id),
    run_id: asString(row.run_id),
    decision_mode: asString(row.decision_mode),
    task_status: asString(row.task_status),
    result_status: asString(row.result_status),
    best_code_ts: asString(row.best_code_ts),
    best_score: asNumber(row.best_score),
    candidates: Array.isArray(row.candidates) ? row.candidates.map(normalizeCandidate) : [],
    error_code: asString(row.error_code),
    error_message: asString(row.error_message),
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

export async function hsResolveByModel(params: HsResolveByModelParams): Promise<HsResolveReply> {
  const payload = await fetchJson<Record<string, unknown>>('/api/hs/resolve/by-model', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(params),
  })
  return normalizeReply(payload)
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
