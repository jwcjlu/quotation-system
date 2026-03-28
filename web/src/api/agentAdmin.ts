/** Agent 运维只读 API（与 `agent_admin.api_keys` 一致，Bearer / X-API-Key） */

function parseAdminError(text: string, status: number): string {
  let msg = `HTTP ${status}`
  if (!text) return msg
  try {
    const j = JSON.parse(text) as {
      message?: string
      error?: { message?: string; code?: string }
    }
    if (typeof j.error?.message === 'string' && j.error.message) {
      msg = j.error.message
    } else if (typeof j.message === 'string' && j.message) {
      msg = j.message
    } else {
      msg = text.slice(0, 300)
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
  const res = await fetch(`/api/v1/admin/agents/${aid}/script-auths/${sid}`, {
    method: 'PUT',
    headers: { ...authHeaders(apiKey), 'Content-Type': 'application/json' },
    body: JSON.stringify({
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
