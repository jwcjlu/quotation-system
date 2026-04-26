const TOKEN_KEY = 'caichip_auth_token'
const SESSION_CHANGE_EVENT = 'caichip:session-change'

function notifySessionChange(): void {
  if (typeof window === 'undefined') return
  window.dispatchEvent(new CustomEvent(SESSION_CHANGE_EVENT))
}

export function getSessionToken(): string | null {
  if (typeof window === 'undefined') return null
  return window.localStorage.getItem(TOKEN_KEY)
}

export function hasSessionToken(): boolean {
  const token = getSessionToken()
  return Boolean(token && token.trim())
}

export function setSessionToken(token: string): void {
  if (typeof window === 'undefined') return
  const normalized = token.trim()
  if (normalized) {
    window.localStorage.setItem(TOKEN_KEY, normalized)
  } else {
    window.localStorage.removeItem(TOKEN_KEY)
  }
  notifySessionChange()
}

export function clearSessionToken(): void {
  if (typeof window === 'undefined') return
  window.localStorage.removeItem(TOKEN_KEY)
  notifySessionChange()
}

export function subscribeSessionChange(listener: () => void): () => void {
  if (typeof window === 'undefined') return () => {}
  const handler = () => listener()
  window.addEventListener(SESSION_CHANGE_EVENT, handler)
  return () => window.removeEventListener(SESSION_CHANGE_EVENT, handler)
}
