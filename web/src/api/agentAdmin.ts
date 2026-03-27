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
}

export interface ListAgentsReply {
  agents: AgentSummary[]
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
