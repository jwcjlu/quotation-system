/** Agent 运维只读 API（与 `agent_admin.api_keys` 一致，Bearer / X-API-Key） */

function parseAdminError(text: string, status: number): string {
  let msg = `HTTP ${status}`
  if (!text) return msg
  try {
    const j = JSON.parse(text) as {
      code?: number
      reason?: string
      message?: string
      error?: { message?: string; code?: string }
    }
    if (typeof j.error?.message === 'string' && j.error.message) {
      msg = j.error.message
    } else {
      const reason = typeof j.reason === 'string' ? j.reason : ''
      const message = typeof j.message === 'string' ? j.message : ''
      if (reason && message && reason !== message) {
        msg = `${reason}: ${message}`
      } else if (message) {
        msg = message
      } else if (reason) {
        msg = reason
      } else {
        msg = text.slice(0, 300)
      }
    }
  } catch {
    msg = text.slice(0, 300)
  }
  return msg
}

async function readJson<T>(res: Response): Promise<T> {
  const text = await res.text()
  if (!res.ok) {
    throw new Error(parseAdminError(text, res.status))
  }
  if (!text) return {} as T
  return JSON.parse(text) as T
}

function authHeaders(apiKey: string): HeadersInit {
  return { Authorization: `Bearer ${apiKey.trim()}` }
}

/** 与 Kratos/proto JSON 一致（camelCase） */
export interface AgentSummary {
  agentId: string
  queue: string
  hostname: string
  /** protobuf Timestamp JSON：RFC3339 */
  lastTaskHeartbeatAt?: string
  online: boolean
  /** online | offline | unknown（无 last_task_heartbeat_at） */
  status?: string
}

export interface ListAgentsReply {
  agents: AgentSummary[]
  /** 离线判定窗口（秒），与 agent.offline_min_sec 等一致 */
  offlineWindowSec?: number
  offline_window_sec?: number
}

export async function listAgents(apiKey: string): Promise<ListAgentsReply> {
  const res = await fetch('/api/v1/admin/agents', {
    headers: authHeaders(apiKey),
  })
  return readJson<ListAgentsReply>(res)
}

export interface LeasedTaskRow {
  taskId: string
  scriptId: string
  version: string
  leasedAt?: string
  leaseDeadlineAt?: string
}

export interface ListAgentLeasedTasksReply {
  tasks: LeasedTaskRow[]
}

export async function listAgentLeasedTasks(apiKey: string, agentId: string): Promise<ListAgentLeasedTasksReply> {
  const id = encodeURIComponent(agentId.trim())
  const res = await fetch(`/api/v1/admin/agents/${id}/leased-tasks`, {
    headers: authHeaders(apiKey),
  })
  return readJson<ListAgentLeasedTasksReply>(res)
}

export interface InstalledScriptRow {
  scriptId: string
  version: string
  envStatus: string
  updatedAt?: string
}

export interface ListAgentInstalledScriptsReply {
  scripts: InstalledScriptRow[]
}

export async function listAgentInstalledScripts(
  apiKey: string,
  agentId: string,
): Promise<ListAgentInstalledScriptsReply> {
  const id = encodeURIComponent(agentId.trim())
  const res = await fetch(`/api/v1/admin/agents/${id}/installed-scripts`, {
    headers: authHeaders(apiKey),
  })
  return readJson<ListAgentInstalledScriptsReply>(res)
}

/** 平台登录凭据（密码仅 Upsert 时提交，列表不返回密码） */
export interface AgentScriptAuthRow {
  scriptId: string
  username: string
  updatedAt?: string
}

export interface ListAgentScriptAuthsReply {
  rows?: AgentScriptAuthRow[]
}

function normalizeScriptAuthRows(raw: ListAgentScriptAuthsReply): AgentScriptAuthRow[] {
  const rows = raw.rows
  if (!Array.isArray(rows)) return []
  return rows.map((r) => {
    const o = r as unknown as Record<string, unknown>
    const scriptId =
      typeof o.scriptId === 'string'
        ? o.scriptId
        : typeof o.script_id === 'string'
          ? o.script_id
          : ''
    const username =
      typeof o.username === 'string'
        ? o.username
        : typeof o.userName === 'string'
          ? o.userName
          : ''
    let updatedAt: string | undefined
    const u = o.updatedAt ?? o.updated_at
    if (typeof u === 'string') updatedAt = u
    else if (u && typeof u === 'object' && 'seconds' in u) {
      const sec = (u as { seconds?: string | number }).seconds
      const nano = (u as { nanos?: number }).nanos ?? 0
      if (sec != null) {
        const t = Number(sec) * 1000 + Math.floor(nano / 1e6)
        updatedAt = new Date(t).toISOString()
      }
    }
    return { scriptId, username, updatedAt }
  })
}

export async function listAgentScriptAuths(
  apiKey: string,
  agentId: string,
): Promise<{ rows: AgentScriptAuthRow[] }> {
  const id = encodeURIComponent(agentId.trim())
  const res = await fetch(`/api/v1/admin/agents/${id}/script-auths`, {
    headers: authHeaders(apiKey),
  })
  const raw = await readJson<ListAgentScriptAuthsReply>(res)
  return { rows: normalizeScriptAuthRows(raw) }
}

export async function upsertAgentScriptAuth(
  apiKey: string,
  params: { agentId: string; scriptId: string; username: string; password: string },
): Promise<void> {
  const aid = encodeURIComponent(params.agentId.trim())
  const sid = encodeURIComponent(params.scriptId.trim())
  // Kratos 对 proto 请求体使用 protojson：字段名须为 camelCase（agentId/scriptId），snake_case 会被 DiscardUnknown 丢掉
  const res = await fetch(`/api/v1/admin/agents/${aid}/script-auths/${sid}`, {
    method: 'PUT',
    headers: { ...authHeaders(apiKey), 'Content-Type': 'application/json' },
    body: JSON.stringify({
      agentId: params.agentId.trim(),
      scriptId: params.scriptId.trim(),
      username: params.username.trim(),
      password: params.password,
    }),
  })
  await readJson<Record<string, unknown>>(res)
}

export async function deleteAgentScriptAuth(
  apiKey: string,
  agentId: string,
  scriptId: string,
): Promise<void> {
  const aid = encodeURIComponent(agentId.trim())
  const sid = encodeURIComponent(scriptId.trim())
  const res = await fetch(`/api/v1/admin/agents/${aid}/script-auths/${sid}`, {
    method: 'DELETE',
    headers: authHeaders(apiKey),
  })
  await readJson<Record<string, unknown>>(res)
}

// ---------- BOM 采集平台 /api/v1/admin/bom-platforms（同 agent_admin 鉴权）----------

function unwrapProtoValue(v: unknown): unknown {
  if (v === null || typeof v !== 'object' || Array.isArray(v)) return v
  const o = v as Record<string, unknown>
  if ('numberValue' in o) return o.numberValue
  if ('stringValue' in o) return o.stringValue
  if ('boolValue' in o) return o.boolValue
  if ('nullValue' in o) return null
  return v
}

/** 将 API 中的 Struct / 平铺 JSON 转为可编辑的 plain 对象 */
export function runParamsToPlain(input: unknown): Record<string, unknown> | null {
  if (input == null) return null
  if (typeof input !== 'object' || Array.isArray(input)) return null
  const o = input as Record<string, unknown>
  const fields = o.fields
  if (fields && typeof fields === 'object' && !Array.isArray(fields)) {
    const out: Record<string, unknown> = {}
    for (const [k, val] of Object.entries(fields as Record<string, unknown>)) {
      out[k] = unwrapProtoValue(val)
    }
    return out
  }
  return { ...o }
}

export interface BomPlatformRow {
  platformId: string
  scriptId: string
  displayName: string
  enabled: boolean
  runParams: Record<string, unknown> | null
  updatedAt?: string
}

function pickStr(o: Record<string, unknown>, camel: string, snake: string): string {
  const a = o[camel]
  const b = o[snake]
  if (typeof a === 'string') return a
  if (typeof b === 'string') return b
  return ''
}

function tsToIso(v: unknown): string | undefined {
  if (typeof v === 'string') return v
  if (v && typeof v === 'object' && 'seconds' in (v as object)) {
    const sec = (v as { seconds?: string | number }).seconds
    const nano = (v as { nanos?: number }).nanos ?? 0
    if (sec != null) {
      const t = Number(sec) * 1000 + Math.floor(nano / 1e6)
      return new Date(t).toISOString()
    }
  }
  return undefined
}

function normalizeBomPlatformRow(raw: Record<string, unknown>): BomPlatformRow {
  const runParamsRaw = raw.runParams ?? raw.run_params
  return {
    platformId: pickStr(raw, 'platformId', 'platform_id'),
    scriptId: pickStr(raw, 'scriptId', 'script_id'),
    displayName: pickStr(raw, 'displayName', 'display_name'),
    enabled: raw.enabled !== false && raw.enabled !== 'false' && raw.enabled !== 0,
    runParams: runParamsToPlain(runParamsRaw),
    updatedAt: tsToIso(raw.updatedAt ?? raw.updated_at),
  }
}

export interface ListBomPlatformsReply {
  items: BomPlatformRow[]
}

export async function listBomPlatforms(apiKey: string): Promise<ListBomPlatformsReply> {
  const res = await fetch('/api/v1/admin/bom-platforms', {
    headers: authHeaders(apiKey),
  })
  const raw = await readJson<{ items?: unknown[] }>(res)
  const items = Array.isArray(raw.items) ? raw.items : []
  return {
    items: items.map((it) => normalizeBomPlatformRow(it as Record<string, unknown>)),
  }
}

export async function getBomPlatform(apiKey: string, platformId: string): Promise<BomPlatformRow | null> {
  const id = encodeURIComponent(platformId.trim())
  const res = await fetch(`/api/v1/admin/bom-platforms/${id}`, {
    headers: authHeaders(apiKey),
  })
  const raw = await readJson<{ item?: Record<string, unknown> }>(res)
  const item = raw.item
  if (!item || typeof item !== 'object') return null
  return normalizeBomPlatformRow(item)
}

export async function upsertBomPlatform(
  apiKey: string,
  body: {
    platformId: string
    scriptId: string
    displayName: string
    enabled: boolean
    /** 不传或 undefined：请求体不含 runParams，后端将 run_params 置空（NULL）；传对象则全量写入（可为 {}） */
    runParams?: Record<string, unknown>
  },
): Promise<BomPlatformRow> {
  const id = encodeURIComponent(body.platformId.trim())
  const payload: Record<string, unknown> = {
    platformId: body.platformId.trim(),
    scriptId: body.scriptId.trim(),
    displayName: body.displayName.trim(),
    enabled: body.enabled,
  }
  if (body.runParams !== undefined) {
    payload.runParams = body.runParams
  }
  const res = await fetch(`/api/v1/admin/bom-platforms/${id}`, {
    method: 'PUT',
    headers: { ...authHeaders(apiKey), 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  const raw = await readJson<{ item?: Record<string, unknown> }>(res)
  const item = raw.item
  if (!item || typeof item !== 'object') {
    throw new Error('Upsert 成功但未返回 item')
  }
  return normalizeBomPlatformRow(item)
}

export async function deleteBomPlatform(apiKey: string, platformId: string): Promise<void> {
  const id = encodeURIComponent(platformId.trim())
  const res = await fetch(`/api/v1/admin/bom-platforms/${id}`, {
    method: 'DELETE',
    headers: authHeaders(apiKey),
  })
  await readJson<Record<string, unknown>>(res)
}
