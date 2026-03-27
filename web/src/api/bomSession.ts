/**
 * BOM 货源会话：/api/v1/bom-sessions/*
 * 与 docs/BOM货源搜索-接口清单.md 及 2026-03-24 客户/列表/行 CRUD 规格一致。
 */
import { fetchJson } from './http'
import type {
  CreateSessionReply,
  GetBOMLinesReply,
  GetReadinessReply,
  GetSessionReply,
  GetSessionSearchTaskCoverageReply,
  ListSessionsReply,
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

export async function createSession(body: {
  title?: string
  platform_ids?: string[]
  customer_name?: string
  contact_phone?: string
  contact_email?: string
  contact_extra?: string
}): Promise<CreateSessionReply> {
  const json = await fetchJson<Record<string, unknown>>(BASE, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      title: body.title ?? '',
      platform_ids: body.platform_ids ?? [],
      customer_name: body.customer_name ?? '',
      contact_phone: body.contact_phone ?? '',
      contact_email: body.contact_email ?? '',
      contact_extra: body.contact_extra ?? '',
    }),
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
  }
}

export async function getSession(sessionId: string): Promise<GetSessionReply> {
  const json = await fetchJson<Record<string, unknown>>(`${BASE}/${encodeURIComponent(sessionId)}`)
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
  }
): Promise<GetSessionReply> {
  const body: Record<string, string> = { session_id: sessionId }
  if (patch.title !== undefined) body.title = patch.title
  if (patch.customer_name !== undefined) body.customer_name = patch.customer_name
  if (patch.contact_phone !== undefined) body.contact_phone = patch.contact_phone
  if (patch.contact_email !== undefined) body.contact_email = patch.contact_email
  if (patch.contact_extra !== undefined) body.contact_extra = patch.contact_extra
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
    can_enter_match: Boolean(json.can_enter_match ?? json.canEnterMatch),
    block_reason: (json.block_reason ?? json.blockReason) as string,
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

/** GET /bom-sessions/{id}/export?format=xlsx|csv — 返回与模板相同的 base64 file */
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
  const name = ((json.filename ?? json.fileName) as string) || `export.${format === 'csv' ? 'csv' : 'xlsx'}`
  const mime =
    format === 'csv'
      ? 'text/csv;charset=utf-8'
      : 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet'
  return { blob: new Blob([bytes], { type: mime }), filename: name }
}
