import { type ChangeEvent, useEffect, useState } from 'react'
import { login, logout, register, type AuthUser } from '../api/auth'
import { clearSessionToken, setSessionToken } from '../auth/session'

interface AuthPanelProps {
  currentUser: AuthUser | null
  busy?: boolean
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

function DefaultAvatarIcon() {
  return (
    <svg aria-hidden="true" viewBox="0 0 24 24" className="h-5 w-5">
      <path
        fill="currentColor"
        d="M12 12a4 4 0 1 0 0-8 4 4 0 0 0 0 8Zm0 2c-4.4 0-8 2.2-8 5v1h16v-1c0-2.8-3.6-5-8-5Z"
      />
    </svg>
  )
}

export function AuthPanel(props: AuthPanelProps) {
  const { currentUser, busy = false, message, onAuthenticated, onLoggedOut } = props
  const [mode, setMode] = useState<Mode>('login')
  const [username, setUsername] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [password, setPassword] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [localMessage, setLocalMessage] = useState<string | null>(null)
  const [avatarDataUrl, setAvatarDataUrl] = useState<string | null>(null)

  useEffect(() => {
    if (!currentUser) {
      setAvatarDataUrl(null)
      return
    }
    setAvatarDataUrl(window.localStorage.getItem(avatarKey(currentUser)))
  }, [currentUser])

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

  if (currentUser) {
    return (
      <div className="rounded-lg border border-slate-700 bg-slate-800/90 px-4 py-3 text-sm text-slate-100 shadow-lg shadow-black/10">
        <div className="flex items-center gap-3">
          <label className="group relative flex h-11 w-11 shrink-0 cursor-pointer items-center justify-center overflow-hidden rounded-full border border-emerald-400/40 bg-emerald-400/10 text-emerald-200">
            {avatarDataUrl ? (
              <img src={avatarDataUrl} alt="用户头像" className="h-full w-full object-cover" />
            ) : (
              <DefaultAvatarIcon />
            )}
            <span className="absolute inset-0 flex items-center justify-center bg-slate-950/65 text-[10px] font-medium text-white opacity-0 transition group-hover:opacity-100">
              更换
            </span>
            <input type="file" accept="image/*" className="sr-only" aria-label="更换头像" onChange={handleAvatarChange} />
          </label>

          <div className="min-w-0 flex-1">
            <div className="truncate font-semibold text-white">{currentUser.displayName || currentUser.username}</div>
            <div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-slate-300">
              <span className="truncate">{currentUser.username}</span>
              <span className="rounded-full border border-emerald-400/30 bg-emerald-400/10 px-2 py-0.5 font-medium text-emerald-200">
                {currentUser.role === 'admin' ? '管理员' : '用户'}
              </span>
            </div>
          </div>

          <button
            type="button"
            disabled={busy || submitting}
            onClick={async () => {
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
              }
            }}
            className="shrink-0 rounded-md border border-slate-600 px-3 py-1.5 text-xs font-medium text-slate-100 transition hover:border-slate-500 hover:bg-slate-700 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {submitting ? '退出中...' : '退出'}
          </button>
        </div>
        {(localMessage || message) && (
          <div className="mt-3 rounded-md border border-slate-700 bg-slate-900/70 px-3 py-2 text-xs text-slate-300">
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
            type="password"
            className={INPUT_CLS}
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
        className="mt-4 w-full rounded-md bg-blue-600 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-blue-500 disabled:cursor-not-allowed disabled:opacity-50"
      >
        {mode === 'login' ? '登录' : '创建账号'}
      </button>
    </div>
  )
}
