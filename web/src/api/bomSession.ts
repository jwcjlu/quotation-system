/**
 * BOM 货源会话：/api/v1/bom-sessions/*
 * 与 docs/BOM货源搜索-接口清单.md 及 2026-03-24 客户/列表/行 CRUD 规格一致。
 */
import { fetchJson } from './http'
import type {
  BOMLineGap,
  CreateSessionReply,
  GetBOMLinesReply,
  GetReadinessReply,
  GetSessionReply,
  GetSessionSearchTaskCoverageReply,
  ListSessionSearchTasksReply,
  ListSessionsReply,
  MatchRunListItem,
} from './types'

const BASE = '/api/v1/bom-sessions'

function num(v: unknown, fallback = 0): number {
  if (typeof v === 'number' && !Number.isNaN(v)) return v
  if (typeof v === 'string') return Number(v) || fallback
  return fallback
}

function str(v: unknown): string {
  return typeof v === 'string' ? v : v != null ? String(v) : ''
}

function bool(v: unknown): boolean {
  if (typeof v === 'boolean') return v
  if (typeof v === 'string') return v.trim().toLowerCase() === 'true'
  return Boolean(v)
}

export async function createSession(body: {
  title?: string
  platform_ids?: string[]
  customer_name?: string
  contact_phone?: string
  contact_email?: string
  contact_extra?: string
  readiness_mode?: string
}): Promise<CreateSessionReply> {
  const payload: Record<string, unknown> = {
    title: body.title ?? '',
    platform_ids: body.platform_ids ?? [],
    customer_name: body.customer_name ?? '',
    contact_phone: body.contact_phone ?? '',
    contact_email: body.contact_email ?? '',
    contact_extra: body.contact_extra ?? '',
  }
  if (body.readiness_mode != null && String(body.readiness_mode).trim() !== '') {
    payload.readiness_mode = String(body.readiness_mode).trim()
  }
  const json = await fetchJson<Record<string, unknown>>(BASE, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  return {
    session_id: (json.session_id ?? json.sessionId) as string,
    biz_date: (json.biz_date ?? json.bizDate) as string,
    selection_revision: num(json.selection_revision ?? json.selectionRevision, 1),
  }
}

function parseGetSession(json: Record<string, unknown>): GetSessionReply {
  return {
    session_id: (json.session_id ?? json.sessionId) as string,
    title: str(json.title),
    status: str(json.status),
    biz_date: str(json.biz_date ?? json.bizDate),
    selection_revision: num(json.selection_revision ?? json.selectionRevision, 1),
    platform_ids: (json.platform_ids ?? json.platformIds ?? []) as string[],
    customer_name: str(json.customer_name ?? json.customerName),
    contact_phone: str(json.contact_phone ?? json.contactPhone),
    contact_email: str(json.contact_email ?? json.contactEmail),
    contact_extra: str(json.contact_extra ?? json.contactExtra),
    readiness_mode: str(json.readiness_mode ?? json.readinessMode) || undefined,
    import_status: str(json.import_status ?? json.importStatus) || undefined,
    import_progress: num(json.import_progress ?? json.importProgress, 0),
    import_stage: str(json.import_stage ?? json.importStage) || undefined,
    import_message: str(json.import_message ?? json.importMessage) || undefined,
    import_error_code: str(json.import_error_code ?? json.importErrorCode) || undefined,
    import_error: str(json.import_error ?? json.importError) || undefined,
    import_updated_at: str(json.import_updated_at ?? json.importUpdatedAt) || undefined,
  }
}

export async function getSession(sessionId: string): Promise<GetSessionReply> {
  const json = await fetchJson<Record<string, unknown>>(
    `${BASE}/${encodeURIComponent(sessionId)}`
  )
  return parseGetSession(json)
}

export async function listSessions(params: {
  page?: number
  page_size?: number
  status?: string
  biz_date?: string
  q?: string
}): Promise<ListSessionsReply> {
  const q = new URLSearchParams()
  if (params.page != null) q.set('page', String(params.page))
  if (params.page_size != null) q.set('page_size', String(params.page_size))
  if (params.status) q.set('status', params.status)
  if (params.biz_date) q.set('biz_date', params.biz_date)
  if (params.q) q.set('q', params.q)
  const qs = q.toString()
  const json = await fetchJson<Record<string, unknown>>(`${BASE}${qs ? `?${qs}` : ''}`)
  const itemsRaw = (json.items ?? []) as Record<string, unknown>[]
  const items = itemsRaw.map((row) => ({
    session_id: str(row.session_id ?? row.sessionId),
    title: str(row.title),
    customer_name: str(row.customer_name ?? row.customerName),
    status: str(row.status),
    biz_date: str(row.biz_date ?? row.bizDate),
    updated_at: str(row.updated_at ?? row.updatedAt),
    line_count: num(row.line_count ?? row.lineCount, 0),
  }))
  return { items, total: num(json.total, items.length) }
}

export async function patchSession(
  sessionId: string,
  patch: {
    title?: string
    customer_name?: string
    contact_phone?: string
    contact_email?: string
    contact_extra?: string
    readiness_mode?: string
  }
): Promise<GetSessionReply> {
  const body: Record<string, string> = { session_id: sessionId }
  if (patch.title !== undefined) body.title = patch.title
  if (patch.customer_name !== undefined) body.customer_name = patch.customer_name
  if (patch.contact_phone !== undefined) body.contact_phone = patch.contact_phone
  if (patch.contact_email !== undefined) body.contact_email = patch.contact_email
  if (patch.contact_extra !== undefined) body.contact_extra = patch.contact_extra
  if (patch.readiness_mode !== undefined) body.readiness_mode = patch.readiness_mode
  const json = await fetchJson<Record<string, unknown>>(`${BASE}/${encodeURIComponent(sessionId)}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  return parseGetSession(json)
}

export async function putPlatforms(
  sessionId: string,
  platformIds: string[],
  expectedRevision?: number
): Promise<{ selection_revision: number }> {
  const json = await fetchJson<Record<string, unknown>>(
    `${BASE}/${encodeURIComponent(sessionId)}/platforms`,
    {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        session_id: sessionId,
        platform_ids: platformIds,
        expected_revision: expectedRevision ?? 0,
      }),
    }
  )
  return {
    selection_revision: num(json.selection_revision ?? json.selectionRevision, 1),
  }
}

export async function getReadiness(sessionId: string): Promise<GetReadinessReply> {
  const json = await fetchJson<Record<string, unknown>>(
    `${BASE}/${encodeURIComponent(sessionId)}/readiness`
  )
  return {
    session_id: (json.session_id ?? json.sessionId) as string,
    biz_date: (json.biz_date ?? json.bizDate) as string,
    selection_revision: num(json.selection_revision ?? json.selectionRevision, 1),
    phase: (json.phase as string) ?? '',
    can_enter_match: bool(json.can_enter_match ?? json.canEnterMatch),
    block_reason: (json.block_reason ?? json.blockReason) as string,
    line_total: num(json.line_total ?? json.lineTotal, 0),
    ready_line_count: num(json.ready_line_count ?? json.readyLineCount, 0),
    gap_line_count: num(json.gap_line_count ?? json.gapLineCount, 0),
    no_data_line_count: num(json.no_data_line_count ?? json.noDataLineCount, 0),
    collection_unavailable_line_count: num(
      json.collection_unavailable_line_count ?? json.collectionUnavailableLineCount,
      0
    ),
    no_match_after_filter_line_count: num(
      json.no_match_after_filter_line_count ?? json.noMatchAfterFilterLineCount,
      0
    ),
    collecting_line_count: num(json.collecting_line_count ?? json.collectingLineCount, 0),
    has_strict_blocking_gap: bool(
      json.has_strict_blocking_gap ?? json.hasStrictBlockingGap
    ),
  }
}

export async function getBOMLines(sessionId: string): Promise<GetBOMLinesReply> {
  const json = await fetchJson<Record<string, unknown>>(
    `${BASE}/${encodeURIComponent(sessionId)}/lines`
  )
  const linesRaw = (json.lines ?? []) as Record<string, unknown>[]
  const lines = linesRaw.map((row) => {
    const gaps = (row.platform_gaps ?? row.platformGaps ?? []) as Record<string, unknown>[]
    return {
      line_id: (row.line_id ?? row.lineId) as string,
      line_no: num(row.line_no ?? row.lineNo, 0),
      mpn: (row.mpn as string) ?? '',
      unified_mpn: str(row.unified_mpn ?? row.unifiedMpn) || undefined,
      reference_designator:
        str(row.reference_designator ?? row.referenceDesignator) || undefined,
      substitute_mpn: str(row.substitute_mpn ?? row.substituteMpn) || undefined,
      remark: str(row.remark) || undefined,
      description: str(row.description) || undefined,
      mfr: (row.mfr as string) ?? '',
      package: (row.package as string) ?? '',
      qty: num(row.qty, 0),
      match_status: (row.match_status ?? row.matchStatus) as string,
      platform_gaps: gaps.map((g) => ({
        platform_id: (g.platform_id ?? g.platformId) as string,
        phase: (g.phase as string) ?? '',
        reason_code: (g.reason_code ?? g.reasonCode) as string,
        message: (g.message as string) ?? '',
        auto_attempt: num(g.auto_attempt ?? g.autoAttempt, 0),
        manual_attempt: num(g.manual_attempt ?? g.manualAttempt, 0),
        search_ui_state: str(g.search_ui_state ?? g.searchUiState) || undefined,
      })),
      availability_status:
        str(row.availability_status ?? row.availabilityStatus) || undefined,
      availability_reason_code:
        str(row.availability_reason_code ?? row.availabilityReasonCode) || undefined,
      availability_reason:
        str(row.availability_reason ?? row.availabilityReason) || undefined,
      has_usable_quote: bool(row.has_usable_quote ?? row.hasUsableQuote),
      raw_quote_platform_count: num(
        row.raw_quote_platform_count ?? row.rawQuotePlatformCount,
        0
      ),
      usable_quote_platform_count: num(
        row.usable_quote_platform_count ?? row.usableQuotePlatformCount,
        0
      ),
      resolution_status: str(row.resolution_status ?? row.resolutionStatus) || undefined,
    }
  })
  return { lines }
}

export async function getSessionSearchTaskCoverage(
  sessionId: string
): Promise<GetSessionSearchTaskCoverageReply> {
  const json = await fetchJson<Record<string, unknown>>(
    `${BASE}/${encodeURIComponent(sessionId)}/search-tasks/coverage`
  )
  const missingRaw = (json.missing_tasks ?? json.missingTasks ?? []) as Record<string, unknown>[]
  return {
    consistent: Boolean(json.consistent),
    orphan_task_count: num(json.orphan_task_count ?? json.orphanTaskCount, 0),
    expected_task_count: num(json.expected_task_count ?? json.expectedTaskCount, 0),
    existing_task_count: num(json.existing_task_count ?? json.existingTaskCount, 0),
    missing_tasks: missingRaw.map((m) => ({
      line_id: str(m.line_id ?? m.lineId),
      line_no: num(m.line_no ?? m.lineNo, 0),
      mpn_norm: str(m.mpn_norm ?? m.mpnNorm),
      platform_id: str(m.platform_id ?? m.platformId),
      reason: str(m.reason),
    })),
  }
}

function parseSearchTaskSummary(json: Record<string, unknown>) {
  return {
    total: num(json.total, 0),
    pending: num(json.pending, 0),
    searching: num(json.searching, 0),
    succeeded: num(json.succeeded, 0),
    no_data: num(json.no_data ?? json.noData, 0),
    failed: num(json.failed, 0),
    skipped: num(json.skipped, 0),
    cancelled: num(json.cancelled, 0),
    missing: num(json.missing, 0),
    retryable: num(json.retryable, 0),
  }
}

export async function listSessionSearchTasks(
  sessionId: string
): Promise<ListSessionSearchTasksReply> {
  const json = await fetchJson<Record<string, unknown>>(
    `${BASE}/${encodeURIComponent(sessionId)}/search-tasks`
  )
  const tasksRaw = (json.tasks ?? []) as Record<string, unknown>[]
  return {
    session_id: str(json.session_id ?? json.sessionId),
    summary: parseSearchTaskSummary((json.summary ?? {}) as Record<string, unknown>),
    tasks: tasksRaw.map((row) => ({
      line_id: str(row.line_id ?? row.lineId),
      line_no: num(row.line_no ?? row.lineNo, 0),
      mpn_raw: str(row.mpn_raw ?? row.mpnRaw),
      mpn_norm: str(row.mpn_norm ?? row.mpnNorm),
      platform_id: str(row.platform_id ?? row.platformId),
      platform_name: str(row.platform_name ?? row.platformName),
      search_task_id: str(row.search_task_id ?? row.searchTaskId),
      search_task_state: str(row.search_task_state ?? row.searchTaskState),
      search_ui_state: str(row.search_ui_state ?? row.searchUiState),
      retryable: bool(row.retryable),
      retry_blocked_reason: str(row.retry_blocked_reason ?? row.retryBlockedReason),
      dispatch_task_id: str(row.dispatch_task_id ?? row.dispatchTaskId),
      dispatch_task_state: str(row.dispatch_task_state ?? row.dispatchTaskState),
      dispatch_agent_id: str(row.dispatch_agent_id ?? row.dispatchAgentId),
      dispatch_result: str(row.dispatch_result ?? row.dispatchResult),
      lease_deadline_at: str(row.lease_deadline_at ?? row.leaseDeadlineAt),
      attempt: num(row.attempt, 0),
      retry_max: num(row.retry_max ?? row.retryMax, 0),
      updated_at: str(row.updated_at ?? row.updatedAt),
      last_error: str(row.last_error ?? row.lastError),
    })),
  }
}

function parseLineGap(row: Record<string, unknown>): BOMLineGap {
  return {
    gap_id: str(row.gap_id ?? row.gapId),
    session_id: str(row.session_id ?? row.sessionId),
    line_id: str(row.line_id ?? row.lineId),
    line_no: num(row.line_no ?? row.lineNo, 0),
    mpn: str(row.mpn),
    gap_type: str(row.gap_type ?? row.gapType),
    reason_code: str(row.reason_code ?? row.reasonCode),
    reason_detail: str(row.reason_detail ?? row.reasonDetail),
    resolution_status: str(row.resolution_status ?? row.resolutionStatus),
    substitute_mpn: str(row.substitute_mpn ?? row.substituteMpn),
    substitute_reason: str(row.substitute_reason ?? row.substituteReason),
    updated_at: str(row.updated_at ?? row.updatedAt),
  }
}

function parseMatchRun(row: Record<string, unknown>): MatchRunListItem {
  return {
    run_id: str(row.run_id ?? row.runId),
    run_no: num(row.run_no ?? row.runNo, 0),
    session_id: str(row.session_id ?? row.sessionId),
    status: str(row.status),
    line_total: num(row.line_total ?? row.lineTotal, 0),
    matched_line_count: num(row.matched_line_count ?? row.matchedLineCount, 0),
    unresolved_line_count: num(row.unresolved_line_count ?? row.unresolvedLineCount, 0),
    total_amount: num(row.total_amount ?? row.totalAmount, 0),
    currency: str(row.currency),
    created_at: str(row.created_at ?? row.createdAt),
    saved_at: str(row.saved_at ?? row.savedAt),
  }
}

export async function listLineGaps(
  sessionId: string,
  statuses: string[] = []
): Promise<{ gaps: BOMLineGap[] }> {
  const q = new URLSearchParams()
  statuses.forEach((status) => {
    if (status) q.append('statuses', status)
  })
  const qs = q.toString()
  const json = await fetchJson<Record<string, unknown>>(
    `/api/bom/sessions/${encodeURIComponent(sessionId)}/gaps${qs ? `?${qs}` : ''}`
  )
  const gapsRaw = (json.gaps ?? []) as Record<string, unknown>[]
  return { gaps: gapsRaw.map(parseLineGap) }
}

export async function saveMatchRun(sessionId: string): Promise<{ run_id: string; run_no: number }> {
  const json = await fetchJson<Record<string, unknown>>(
    `/api/bom/sessions/${encodeURIComponent(sessionId)}/match-runs`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ session_id: sessionId }),
    }
  )
  return {
    run_id: str(json.run_id ?? json.runId),
    run_no: num(json.run_no ?? json.runNo, 0),
  }
}

export async function listMatchRuns(sessionId: string): Promise<{ runs: MatchRunListItem[] }> {
  const json = await fetchJson<Record<string, unknown>>(
    `/api/bom/sessions/${encodeURIComponent(sessionId)}/match-runs`
  )
  const runsRaw = (json.runs ?? []) as Record<string, unknown>[]
  return { runs: runsRaw.map(parseMatchRun) }
}

export async function resolveLineGapManualQuote(
  gapId: string,
  body: {
    model: string
    manufacturer?: string
    package?: string
    stock?: string
    lead_time?: string
    price_tiers?: string
    hk_price?: string
    mainland_price?: string
    note?: string
  }
): Promise<{ accepted: boolean }> {
  const json = await fetchJson<Record<string, unknown>>(
    `/api/bom/gaps/${encodeURIComponent(gapId)}/manual-quote`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ gap_id: gapId, ...body }),
    }
  )
  return { accepted: Boolean(json.accepted) }
}

export async function selectLineGapSubstitute(
  gapId: string,
  body: { substitute_mpn: string; reason?: string }
): Promise<{ accepted: boolean }> {
  const json = await fetchJson<Record<string, unknown>>(
    `/api/bom/gaps/${encodeURIComponent(gapId)}/substitute`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ gap_id: gapId, ...body }),
    }
  )
  return { accepted: Boolean(json.accepted) }
}

export async function createSessionLine(
  sessionId: string,
  body: {
    mpn: string
    mfr?: string
    package?: string
    qty?: number
    raw?: string
    extra_json?: string
  }
): Promise<{ line_id: string; line_no: number }> {
  const json = await fetchJson<Record<string, unknown>>(
    `${BASE}/${encodeURIComponent(sessionId)}/lines`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        session_id: sessionId,
        mpn: body.mpn,
        mfr: body.mfr ?? '',
        package: body.package ?? '',
        qty: body.qty ?? 0,
        raw: body.raw ?? '',
        extra_json: body.extra_json ?? '',
      }),
    }
  )
  return {
    line_id: str(json.line_id ?? json.lineId),
    line_no: num(json.line_no ?? json.lineNo, 0),
  }
}

export async function patchSessionLine(
  sessionId: string,
  lineId: string,
  patch: {
    mpn?: string
    mfr?: string
    package?: string
    qty?: number
    raw?: string
    extra_json?: string
  }
): Promise<{ line_id: string; line_no: number }> {
  const body: Record<string, unknown> = {
    session_id: sessionId,
    line_id: lineId,
  }
  if (patch.mpn !== undefined) body.mpn = patch.mpn
  if (patch.mfr !== undefined) body.mfr = patch.mfr
  if (patch.package !== undefined) body.package = patch.package
  if (patch.qty !== undefined) body.qty = patch.qty
  if (patch.raw !== undefined) body.raw = patch.raw
  if (patch.extra_json !== undefined) body.extra_json = patch.extra_json
  const json = await fetchJson<Record<string, unknown>>(
    `${BASE}/${encodeURIComponent(sessionId)}/lines/${encodeURIComponent(lineId)}`,
    {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    }
  )
  return {
    line_id: str(json.line_id ?? json.lineId),
    line_no: num(json.line_no ?? json.lineNo, 0),
  }
}

export async function deleteSessionLine(sessionId: string, lineId: string): Promise<void> {
  await fetchJson<Record<string, unknown>>(
    `${BASE}/${encodeURIComponent(sessionId)}/lines/${encodeURIComponent(lineId)}`,
    { method: 'DELETE' }
  )
}

export async function retrySearchTasks(
  sessionId: string,
  items: { mpn: string; platform_id: string }[]
): Promise<{ accepted: number }> {
  const json = await fetchJson<Record<string, unknown>>(
    `${BASE}/${encodeURIComponent(sessionId)}/search-tasks/retry`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        session_id: sessionId,
        items: items.map((i) => ({ mpn: i.mpn, platform_id: i.platform_id })),
      }),
    }
  )
  return { accepted: num(json.accepted, 0) }
}

export async function exportSessionFile(
  sessionId: string,
  format: 'xlsx' | 'csv' = 'xlsx'
): Promise<{ blob: Blob; filename: string }> {
  const q = new URLSearchParams()
  if (format) q.set('format', format)
  const qs = q.toString()
  const json = await fetchJson<Record<string, unknown>>(
    `${BASE}/${encodeURIComponent(sessionId)}/export${qs ? `?${qs}` : ''}`
  )
  const b64 = json.file as string
  if (!b64) throw new Error('导出数据为空')
  const bin = atob(b64)
  const bytes = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i)
  const name =
    ((json.filename ?? json.fileName) as string) ||
    `export.${format === 'csv' ? 'csv' : 'xlsx'}`
  const mime =
    format === 'csv'
      ? 'text/csv;charset=utf-8'
      : 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet'
  return { blob: new Blob([bytes], { type: mime }), filename: name }
}
