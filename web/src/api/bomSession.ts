/**
 * BOM 货源会话：/api/v1/bom-sessions/*
 * 与 docs/BOM货源搜索-接口清单.md 第 1～5 节路径一致（当前后端为 gRPC-Gateway 已实现路径）。
 * 说明：上传仍走 POST /api/v1/bom/upload + body.session_id（清单 §2 草案为 multipart 专用路径，以后端为准）。
 */
import { fetchJson } from './http'
import type {
  CreateSessionReply,
  GetBOMLinesReply,
  GetReadinessReply,
  GetSessionReply,
} from './types'

const BASE = '/api/v1/bom-sessions'

function num(v: unknown, fallback = 0): number {
  if (typeof v === 'number' && !Number.isNaN(v)) return v
  if (typeof v === 'string') return Number(v) || fallback
  return fallback
}

export async function createSession(body: {
  title?: string
  platform_ids?: string[]
}): Promise<CreateSessionReply> {
  const json = await fetchJson<Record<string, unknown>>(BASE, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      title: body.title ?? '',
      platform_ids: body.platform_ids ?? [],
    }),
  })
  return {
    session_id: (json.session_id ?? json.sessionId) as string,
    biz_date: (json.biz_date ?? json.bizDate) as string,
    selection_revision: num(json.selection_revision ?? json.selectionRevision, 1),
  }
}

export async function getSession(sessionId: string): Promise<GetSessionReply> {
  const json = await fetchJson<Record<string, unknown>>(`${BASE}/${encodeURIComponent(sessionId)}`)
  return {
    session_id: (json.session_id ?? json.sessionId) as string,
    title: (json.title as string) ?? '',
    status: (json.status as string) ?? '',
    biz_date: (json.biz_date ?? json.bizDate) as string,
    selection_revision: num(json.selection_revision ?? json.selectionRevision, 1),
    platform_ids: (json.platform_ids ?? json.platformIds ?? []) as string[],
  }
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
      })),
    }
  })
  return { lines }
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
