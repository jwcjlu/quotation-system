import { clearSessionToken, getSessionToken } from '../auth/session'

function dispatchAuthEvent(name: string): void {
  if (typeof window === 'undefined') return
  window.dispatchEvent(new CustomEvent(name))
}

export async function fetchJson<T>(url: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers)
  const token = getSessionToken()
  if (token?.trim()) {
    headers.set('Authorization', `Bearer ${token.trim()}`)
  }

  const res = await fetch(url, { ...init, headers })
  const text = await res.text()

  if (res.status === 401) {
    clearSessionToken()
    dispatchAuthEvent('auth:unauthorized')
  } else if (res.status === 403) {
    dispatchAuthEvent('auth:forbidden')
  }

  if (!res.ok) {
    let msg = `HTTP ${res.status}`
    if (text) {
      try {
        const json = JSON.parse(text) as Record<string, unknown>
        const message =
          (typeof json.message === 'string' && json.message) ||
          (typeof (json as { error?: { message?: string } }).error?.message === 'string' &&
            (json as { error: { message: string } }).error.message) ||
          (typeof (json as { error?: string }).error === 'string' && (json as { error: string }).error) ||
          (typeof (json as { reason?: string }).reason === 'string' && (json as { reason: string }).reason)
        if (message) msg = message
      } catch {
        msg = text.slice(0, 200)
      }
    }
    throw new Error(msg)
  }

  if (!text) return {} as T
  return JSON.parse(text) as T
}
