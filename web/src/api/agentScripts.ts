/** Agent 脚本包管理端 API（与 `script_admin.api_keys` 一致，Bearer / X-API-Key） */

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

export interface UploadPackageReply {
  package_id: number
  download_path: string
  sha256: string
}

export async function uploadAgentScriptPackage(
  apiKey: string,
  params: {
    scriptId: string
    version: string
    file: File
    releaseNotes?: string
    packageSha256?: string
  },
): Promise<UploadPackageReply> {
  const fd = new FormData()
  fd.append('script_id', params.scriptId.trim())
  fd.append('version', params.version.trim())
  fd.append('file', params.file)
  if (params.releaseNotes?.trim()) {
    fd.append('release_notes', params.releaseNotes.trim())
  }
  if (params.packageSha256?.trim()) {
    fd.append('package_sha256', params.packageSha256.trim().toLowerCase())
  }
  const res = await fetch('/api/v1/admin/agent-scripts/packages', {
    method: 'POST',
    headers: authHeaders(apiKey),
    body: fd,
  })
  return readJson<UploadPackageReply>(res)
}

export interface PublishReply {
  published: boolean
  id: number
}

export async function publishAgentScriptPackage(apiKey: string, packageId: number): Promise<PublishReply> {
  const res = await fetch(`/api/v1/admin/agent-scripts/packages/${packageId}/publish`, {
    method: 'POST',
    headers: authHeaders(apiKey),
  })
  return readJson<PublishReply>(res)
}

export interface CurrentPackageReply {
  id: number
  script_id: string
  version: string
  sha256: string
  storage_rel_path: string
  filename: string
  status: string
  public_path: string
}

export async function getCurrentAgentScript(apiKey: string, scriptId: string): Promise<CurrentPackageReply> {
  const q = new URLSearchParams({ script_id: scriptId.trim() })
  const res = await fetch(`/api/v1/admin/agent-scripts/current?${q}`, {
    headers: authHeaders(apiKey),
  })
  return readJson<CurrentPackageReply>(res)
}

export interface PackageListItem {
  id: number
  script_id: string
  version: string
  sha256: string
  status: string
  storage_rel_path: string
  filename: string
}

export interface ListPackagesReply {
  packages: PackageListItem[]
}

export async function listAgentScriptPackages(
  apiKey: string,
  offset = 0,
  limit = 20,
): Promise<ListPackagesReply> {
  const q = new URLSearchParams({ offset: String(offset), limit: String(limit) })
  const res = await fetch(`/api/v1/admin/agent-scripts/packages?${q}`, {
    headers: authHeaders(apiKey),
  })
  return readJson<ListPackagesReply>(res)
}
