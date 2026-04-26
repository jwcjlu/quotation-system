import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { getMe, type AuthUser } from './api/auth'
import { hasSessionToken, subscribeSessionChange } from './auth/session'
import { AuthPanel } from './components/AuthPanel'
import { BomWorkbenchPage } from './pages/BomWorkbenchPage'
import { AgentScriptsPage } from './pages/AgentScriptsPage'
import { AgentAdminPage } from './pages/AgentAdminPage'
import { HsResolvePage, type HsResolvePrefill } from './pages/HsResolvePage'
import { HsMetaAdminPage } from './pages/HsMetaAdminPage'
import { GuidePage } from './pages/GuidePage'

type Page = 'guide' | 'bom-workbench' | 'agent-scripts' | 'agent-admin' | 'hs-resolve' | 'hs-meta'
type RoleKey = 'anonymous' | 'user' | 'admin'

const PAGE_LABELS: Record<Page, string> = {
  guide: '\u4f7f\u7528\u6307\u5357',
  'bom-workbench': 'BOM\u5de5\u4f5c\u53f0',
  'agent-scripts': '\u811a\u672c\u5305',
  'agent-admin': 'Agent\u8fd0\u7ef4',
  'hs-resolve': 'HS\u578b\u53f7\u89e3\u6790',
  'hs-meta': 'HS\u5143\u6570\u636e',
}

const ALLOWED_PAGES: Record<RoleKey, Page[]> = {
  anonymous: ['bom-workbench', 'guide'],
  user: ['bom-workbench', 'hs-resolve', 'guide'],
  admin: ['bom-workbench', 'agent-scripts', 'agent-admin', 'hs-resolve', 'hs-meta', 'guide'],
}

function roleKey(user: AuthUser | null): RoleKey {
  if (!user) return 'anonymous'
  return user.role === 'admin' ? 'admin' : 'user'
}

function firstAllowedPage(user: AuthUser | null): Page {
  return ALLOWED_PAGES[roleKey(user)][0]
}

function App() {
  const [page, setPage] = useState<Page>(() => firstAllowedPage(null))
  const [currentUser, setCurrentUser] = useState<AuthUser | null>(null)
  const [authReady, setAuthReady] = useState(false)
  const [authMessage, setAuthMessage] = useState<string | null>(null)
  const [hsPrefill, setHsPrefill] = useState<HsResolvePrefill | null>(null)
  const hsPrefillKeySeq = useRef(0)

  const allowedPages = useMemo(() => ALLOWED_PAGES[roleKey(currentUser)], [currentUser])

  const refreshCurrentUser = useCallback(async () => {
    if (!hasSessionToken()) {
      setCurrentUser(null)
      setAuthReady(true)
      return
    }

    try {
      const reply = await getMe()
      setCurrentUser(reply.user)
      setAuthMessage(null)
    } catch (error) {
      setCurrentUser(null)
      setAuthMessage(error instanceof Error ? error.message : '\u767b\u5f55\u72b6\u6001\u5df2\u5931\u6548')
    } finally {
      setAuthReady(true)
    }
  }, [])

  useEffect(() => {
    void refreshCurrentUser()
    return subscribeSessionChange(() => {
      void refreshCurrentUser()
    })
  }, [refreshCurrentUser])

  useEffect(() => {
    const handleUnauthorized = () => {
      setCurrentUser(null)
      setAuthMessage('\u767b\u5f55\u5df2\u5931\u6548\uff0c\u8bf7\u91cd\u65b0\u767b\u5f55')
    }
    const handleForbidden = () => {
      setAuthMessage('\u65e0\u6743\u9650\u8bbf\u95ee\u5f53\u524d\u529f\u80fd')
    }

    window.addEventListener('auth:unauthorized', handleUnauthorized)
    window.addEventListener('auth:forbidden', handleForbidden)
    return () => {
      window.removeEventListener('auth:unauthorized', handleUnauthorized)
      window.removeEventListener('auth:forbidden', handleForbidden)
    }
  }, [])

  useEffect(() => {
    if (!allowedPages.includes(page)) {
      setPage(firstAllowedPage(currentUser))
    }
  }, [allowedPages, currentUser, page])

  const renderNavButton = (target: Page) => {
    if (!allowedPages.includes(target)) return null

    return (
      <button
        key={target}
        type="button"
        onClick={() => {
          if (target === 'hs-resolve') {
            setHsPrefill(null)
          }
          setPage(target)
        }}
        className={`rounded-md px-3 py-2 text-sm font-medium transition ${
          page === target
            ? 'bg-white text-slate-950 shadow-sm'
            : 'text-slate-300 hover:bg-slate-700 hover:text-white'
        }`}
      >
        {PAGE_LABELS[target]}
      </button>
    )
  }

  return (
    <div className="min-h-screen bg-slate-50 text-slate-900">
      <header className="border-b border-slate-800 bg-slate-900 text-white shadow-lg shadow-slate-900/10">
        <div className="mx-auto grid max-w-7xl gap-5 px-4 py-5 sm:px-6 lg:grid-cols-[minmax(0,1fr)_22rem] lg:items-start">
          <div className="min-w-0">
            <div>
              <p className="text-xs font-semibold uppercase text-blue-300">CAICHIP</p>
              <div className="mt-1 flex flex-wrap items-center gap-3">
                <h1 className="text-2xl font-semibold tracking-tight text-white">{'BOM\u914d\u5355\u7cfb\u7edf'}</h1>
                <div className="rounded-md border border-emerald-400/40 bg-emerald-400/10 px-3 py-1.5 text-xs font-medium text-emerald-200">
                  {currentUser ? (currentUser.role === 'admin' ? 'Admin' : 'User') : '\u672a\u767b\u5f55'}
                </div>
              </div>
            </div>
            <p className="mt-3 max-w-2xl text-sm leading-6 text-slate-300">
              {'\u96c6\u4e2d\u7ba1\u7406 BOM \u4f1a\u8bdd\u3001\u5339\u914d\u5355\u3001HS \u578b\u53f7\u89e3\u6790\u548c Agent \u8fd0\u7ef4\u6d41\u7a0b\u3002'}
            </p>
            <nav className="mt-5 flex flex-wrap items-center gap-2 rounded-lg border border-slate-700 bg-slate-800 p-2 shadow-inner shadow-black/10">
              {(Object.keys(PAGE_LABELS) as Page[]).map(renderNavButton)}
            </nav>
          </div>

          <div className="w-full lg:justify-self-end">
            <AuthPanel
              currentUser={currentUser}
              busy={!authReady}
              message={authMessage}
              onAuthenticated={(user) => {
                setCurrentUser(user)
                setAuthMessage(null)
                setPage(firstAllowedPage(user))
              }}
              onLoggedOut={() => {
                setCurrentUser(null)
                setPage(firstAllowedPage(null))
              }}
            />
          </div>
        </div>
      </header>

      {!authReady && hasSessionToken() && (
        <div className="mx-auto max-w-7xl px-6 pt-4">
          <div className="rounded-2xl border border-slate-200 bg-white/80 px-4 py-3 text-sm text-slate-600 shadow-sm">
            {'\u6b63\u5728\u6062\u590d\u767b\u5f55\u72b6\u6001...'}
          </div>
        </div>
      )}

      <main className="mx-auto max-w-7xl px-4 py-6 sm:px-6 lg:py-8">
        {page === 'guide' && <GuidePage />}
        {page === 'bom-workbench' && (
          <BomWorkbenchPage
            onNavigateToHsResolve={(model, manufacturer) => {
              hsPrefillKeySeq.current += 1
              setHsPrefill({ key: hsPrefillKeySeq.current, model, manufacturer })
              setPage('hs-resolve')
            }}
          />
        )}
        {page === 'agent-scripts' && <AgentScriptsPage />}
        {page === 'agent-admin' && <AgentAdminPage />}
        {page === 'hs-resolve' && <HsResolvePage prefill={hsPrefill} />}
        {page === 'hs-meta' && <HsMetaAdminPage />}
      </main>
    </div>
  )
}

export default App
