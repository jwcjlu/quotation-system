import { type ChangeEvent, useEffect, useRef, useState } from 'react'
import { login, logout, register, type AuthUser } from '../api/auth'
import { clearSessionToken, setSessionToken } from '../auth/session'

interface AuthPanelProps {
  currentUser: AuthUser | null
  busy?: boolean
  navIsDark?: boolean
  message?: string | null
  onAuthenticated: (user: AuthUser) => void
  onLoggedOut: () => void
}

type Mode = 'login' | 'register'

const AVATAR_KEY_PREFIX = 'auth_avatar:'
const INPUT_CLS =
  'w-full rounded border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 outline-none transition focus:border-blue-500 focus:ring-2 focus:ring-blue-200'

function avatarKey(user: AuthUser): string {
  return `${AVATAR_KEY_PREFIX}${user.username}`
}

function displayUserName(user: AuthUser): string {
  return user.displayName || user.username
}

function avatarInitial(user: AuthUser): string {
  return displayUserName(user).trim().slice(0, 1).toUpperCase() || 'U'
}

export function AuthPanel(props: AuthPanelProps) {
  const { currentUser, busy = false, navIsDark = false, message, onAuthenticated, onLoggedOut } = props
  const [mode, setMode] = useState<Mode>('login')
  const [username, setUsername] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [password, setPassword] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [localMessage, setLocalMessage] = useState<string | null>(null)
  const [avatarDataUrl, setAvatarDataUrl] = useState<string | null>(null)
  const [menuOpen, setMenuOpen] = useState(false)
  const menuRef = useRef<HTMLDivElement | null>(null)
  const avatarInputRef = useRef<HTMLInputElement | null>(null)

  useEffect(() => {
    if (!currentUser) {
      setAvatarDataUrl(null)
      setMenuOpen(false)
      return
    }
    setAvatarDataUrl(window.localStorage.getItem(avatarKey(currentUser)))
  }, [currentUser])

  useEffect(() => {
    if (!menuOpen) return

    const handlePointerDown = (event: MouseEvent) => {
      if (!menuRef.current?.contains(event.target as Node)) {
        setMenuOpen(false)
      }
    }
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setMenuOpen(false)
    }

    document.addEventListener('mousedown', handlePointerDown)
    document.addEventListener('keydown', handleKeyDown)
    return () => {
      document.removeEventListener('mousedown', handlePointerDown)
      document.removeEventListener('keydown', handleKeyDown)
    }
  }, [menuOpen])

  const handleAvatarChange = (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    if (!file || !currentUser) return
    if (!file.type.startsWith('image/')) {
      setLocalMessage('请选择图片文件')
      return
    }

    const reader = new FileReader()
    reader.onload = () => {
      const value = typeof reader.result === 'string' ? reader.result : ''
      if (!value) return
      window.localStorage.setItem(avatarKey(currentUser), value)
      setAvatarDataUrl(value)
      setLocalMessage(null)
      setMenuOpen(false)
    }
    reader.onerror = () => setLocalMessage('头像读取失败')
    reader.readAsDataURL(file)
  }

  const submit = async () => {
    if (!username.trim() || !password) {
      setLocalMessage('请先填写用户名和密码')
      return
    }
    if (mode === 'register' && !displayName.trim()) {
      setLocalMessage('请填写显示名')
      return
    }
    if (mode === 'register' && password.length < 8) {
      setLocalMessage('密码至少 8 位')
      return
    }
    setSubmitting(true)
    setLocalMessage(null)
    try {
      if (mode === 'login') {
        const result = await login({ username, password })
        setSessionToken(result.sessionToken)
        onAuthenticated(result.user)
      } else {
        await register({ username, displayName, password })
        setMode('login')
        setPassword('')
        setLocalMessage('注册成功，请使用新账号登录')
      }
    } catch (error) {
      setLocalMessage(error instanceof Error ? error.message : 'auth failed')
    } finally {
      setSubmitting(false)
    }
  }

  const handleLogout = async () => {
    setSubmitting(true)
    setLocalMessage(null)
    try {
      await logout()
    } catch (error) {
      setLocalMessage(error instanceof Error ? error.message : 'logout failed')
    } finally {
      clearSessionToken()
      onLoggedOut()
      setSubmitting(false)
      setMenuOpen(false)
    }
  }

  if (currentUser) {
    const name = displayUserName(currentUser)

    return (
      <div ref={menuRef} className="relative flex justify-end">
        <button
          type="button"
          aria-haspopup="menu"
          aria-expanded={menuOpen}
          aria-label={`${name} 用户菜单`}
          onClick={() => setMenuOpen((open) => !open)}
          className={`flex items-center gap-2 rounded-full border px-2.5 py-1.5 text-sm shadow-sm transition ${
            navIsDark
              ? 'border-white/15 bg-white/10 text-slate-100 hover:bg-white/15'
              : 'border-[#d7e0ed] bg-white text-slate-700 hover:bg-slate-50'
          }`}
        >
          <span
            className={`relative flex h-9 w-9 shrink-0 items-center justify-center overflow-hidden rounded-full border text-sm font-bold ${
              navIsDark
                ? 'border-blue-200/40 bg-slate-100 text-[#244a86]'
                : 'border-slate-200 bg-slate-100 text-[#244a86]'
            }`}
          >
            {avatarDataUrl ? (
              <img src={avatarDataUrl} alt={`${name} 头像`} className="h-full w-full object-cover" />
            ) : (
              <span aria-hidden="true">{avatarInitial(currentUser)}</span>
            )}
          </span>
          <span className={`max-w-[9rem] truncate font-semibold ${navIsDark ? 'text-white' : 'text-slate-950'}`}>{name}</span>
          <span aria-hidden="true" className={`transition ${navIsDark ? 'text-slate-200' : 'text-slate-500'} ${menuOpen ? 'rotate-180' : ''}`}>
            ▾
          </span>
        </button>

        <input
          ref={avatarInputRef}
          type="file"
          accept="image/*"
          className="sr-only"
          aria-label="更换头像"
          onChange={handleAvatarChange}
        />

        {menuOpen && (
          <div
            role="menu"
            className="absolute right-0 top-full z-20 mt-2 w-56 overflow-hidden rounded-lg border border-[#d7e0ed] bg-white text-sm text-slate-700 shadow-xl shadow-slate-900/10"
          >
            <div className="border-b border-[#edf2f7] px-4 py-3">
              <div className="text-xs text-slate-500">Signed in as</div>
              <div className="mt-1 truncate font-semibold text-slate-950">{name}</div>
            </div>
            <button
              type="button"
              role="menuitem"
              onClick={() => avatarInputRef.current?.click()}
              className="block w-full px-4 py-2.5 text-left hover:bg-slate-50"
            >
              更换头像
            </button>
            <button
              type="button"
              role="menuitem"
              disabled={busy || submitting}
              onClick={() => void handleLogout()}
              className="block w-full border-t border-[#edf2f7] px-4 py-2.5 text-left text-red-700 hover:bg-red-50 disabled:cursor-not-allowed disabled:opacity-50"
            >
              {submitting ? '退出中...' : '退出登录'}
            </button>
          </div>
        )}

        {(localMessage || message) && !menuOpen && (
          <div className="absolute right-0 top-[calc(100%+0.5rem)] z-10 w-64 rounded-md border border-[#d7e0ed] bg-white px-3 py-2 text-xs text-slate-600 shadow-lg">
            {localMessage || message}
          </div>
        )}
      </div>
    )
  }

  return (
    <div className="w-full rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
      <div className="mb-4 flex items-center gap-2 rounded-md bg-slate-100 p-1 text-xs font-medium text-slate-600">
        <button
          type="button"
          onClick={() => {
            setMode('login')
            setLocalMessage(null)
          }}
          className={`flex-1 rounded-md px-3 py-2 transition ${
            mode === 'login' ? 'bg-white text-slate-900 shadow-sm' : 'hover:text-slate-900'
          }`}
        >
          登录
        </button>
        <button
          type="button"
          onClick={() => {
            setMode('register')
            setLocalMessage(null)
          }}
          className={`flex-1 rounded-md px-3 py-2 transition ${
            mode === 'register' ? 'bg-white text-slate-900 shadow-sm' : 'hover:text-slate-900'
          }`}
        >
          注册
        </button>
      </div>

      <div className="space-y-3">
        <label className="block">
          <span className="mb-1 block text-xs font-medium text-slate-600">用户名</span>
          <input className={INPUT_CLS} value={username} onChange={(event) => setUsername(event.target.value)} autoComplete="username" />
        </label>

        {mode === 'register' && (
          <label className="block">
            <span className="mb-1 block text-xs font-medium text-slate-600">显示名</span>
            <input className={INPUT_CLS} value={displayName} onChange={(event) => setDisplayName(event.target.value)} autoComplete="nickname" />
          </label>
        )}

        <label className="block">
          <span className="mb-1 block text-xs font-medium text-slate-600">密码</span>
          <input
            className={INPUT_CLS}
            type="password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            autoComplete={mode === 'login' ? 'current-password' : 'new-password'}
          />
        </label>
      </div>

      {(localMessage || message) && <div className="mt-4 rounded-md bg-slate-50 px-3 py-2 text-xs text-slate-600">{localMessage || message}</div>}

      <button
        type="button"
        disabled={busy || submitting}
        onClick={() => void submit()}
        className="mt-4 w-full rounded-md bg-slate-900 px-4 py-2 text-sm font-medium text-white transition hover:bg-slate-800 disabled:cursor-not-allowed disabled:opacity-60"
      >
        {submitting ? '处理中...' : mode === 'login' ? '登录' : '创建账号'}
      </button>
    </div>
  )
}
