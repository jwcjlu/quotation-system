/** 统一 fetch JSON，兼容 Kratos 错误体 */
export async function fetchJson<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, init)
  const text = await res.text()
  if (!res.ok) {
    let msg = `HTTP ${res.status}`
    if (text) {
      try {
        const j = JSON.parse(text) as Record<string, unknown>
        const m =
          (typeof j.message === 'string' && j.message) ||
          (typeof (j as { error?: string }).error === 'string' && (j as { error: string }).error) ||
          (typeof (j as { reason?: string }).reason === 'string' && (j as { reason: string }).reason)
        if (m) msg = m
      } catch {
        msg = text.slice(0, 200)
      }
    }
    throw new Error(msg)
  }
  if (!text) return {} as T
  return JSON.parse(text) as T
}
