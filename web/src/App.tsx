import { useState } from 'react'
import { UploadPage } from './pages/UploadPage'
import { MatchResultPage } from './pages/MatchResultPage'
import { SourcingSessionPage } from './pages/SourcingSessionPage'
import { MatchHistoryPage } from './pages/MatchHistoryPage'
import { AgentScriptsPage } from './pages/AgentScriptsPage'

const LAST_BOM_KEY = 'bom_last_bom_id'
const LAST_SESSION_KEY = 'bom_last_session_id'

type Page = 'upload-classic' | 'upload-session' | 'sourcing' | 'result' | 'history' | 'agent-scripts'

function App() {
  const [page, setPage] = useState<Page>('upload-classic')
  const [bomId, setBomId] = useState<string | null>(() => localStorage.getItem(LAST_BOM_KEY))
  const [sessionId, setSessionId] = useState<string | null>(() => localStorage.getItem(LAST_SESSION_KEY))

  const onClassicSuccess = (id: string) => {
    setBomId(id)
    localStorage.setItem(LAST_BOM_KEY, id)
    setPage('result')
  }

  const onSessionUploadSuccess = (id: string) => {
    setSessionId(id)
    localStorage.setItem(LAST_SESSION_KEY, id)
    setBomId(id)
    localStorage.setItem(LAST_BOM_KEY, id)
    setPage('sourcing')
  }

  const effectiveBomId = bomId || localStorage.getItem(LAST_BOM_KEY)
  const effectiveSessionId = sessionId || localStorage.getItem(LAST_SESSION_KEY)

  return (
    <div className="min-h-screen bg-slate-50">
      <header className="bg-slate-800 text-white px-6 py-4 shadow">
        <div className="max-w-7xl mx-auto flex flex-wrap items-center justify-between gap-4">
          <h1 className="text-xl font-semibold">BOM 配单系统</h1>
          <nav className="flex flex-wrap gap-2">
            <button
              type="button"
              onClick={() => setPage('upload-classic')}
              className={`px-3 py-1 rounded text-sm ${page === 'upload-classic' ? 'bg-slate-600' : 'hover:bg-slate-700'}`}
            >
              经典上传
            </button>
            <button
              type="button"
              onClick={() => setPage('upload-session')}
              className={`px-3 py-1 rounded text-sm ${page === 'upload-session' ? 'bg-slate-600' : 'hover:bg-slate-700'}`}
            >
              货源会话
            </button>
            <button
              type="button"
              onClick={() => {
                const sid = effectiveSessionId
                if (sid) {
                  setSessionId(sid)
                  setPage('sourcing')
                }
              }}
              className={`px-3 py-1 rounded text-sm ${page === 'sourcing' ? 'bg-slate-600' : 'hover:bg-slate-700'} ${!effectiveSessionId ? 'opacity-50 cursor-not-allowed' : ''}`}
              disabled={!effectiveSessionId}
              title={!effectiveSessionId ? '请先在「货源会话」上传 BOM' : '会话看板'}
            >
              会话看板
            </button>
            <button
              type="button"
              onClick={() => {
                const id = bomId || localStorage.getItem(LAST_BOM_KEY)
                if (id) {
                  setBomId(id)
                  setPage('result')
                }
              }}
              className={`px-3 py-1 rounded text-sm ${page === 'result' ? 'bg-slate-600' : 'hover:bg-slate-700'} ${!effectiveBomId ? 'opacity-50 cursor-not-allowed' : ''}`}
              title={!effectiveBomId ? '请先上传 BOM' : '经典配单结果'}
              disabled={!effectiveBomId}
            >
              匹配单
            </button>
            <button
              type="button"
              onClick={() => setPage('history')}
              className={`px-3 py-1 rounded text-sm ${page === 'history' ? 'bg-slate-600' : 'hover:bg-slate-700'}`}
            >
              配单历史
            </button>
            <button
              type="button"
              onClick={() => setPage('agent-scripts')}
              className={`px-3 py-1 rounded text-sm ${page === 'agent-scripts' ? 'bg-slate-600' : 'hover:bg-slate-700'}`}
            >
              脚本包
            </button>
          </nav>
        </div>
      </header>

      <main className="max-w-7xl mx-auto px-6 py-8">
        {page === 'upload-classic' && <UploadPage flow="classic" onSuccess={onClassicSuccess} />}
        {page === 'upload-session' && <UploadPage flow="session" onSuccess={onSessionUploadSuccess} />}
        {page === 'sourcing' && effectiveSessionId && (
          <SourcingSessionPage
            sessionId={effectiveSessionId}
            onOpenMatch={() => {
              setBomId(effectiveSessionId)
              setPage('result')
            }}
          />
        )}
        {page === 'result' && effectiveBomId && <MatchResultPage bomId={effectiveBomId} />}
        {page === 'history' && <MatchHistoryPage />}
        {page === 'agent-scripts' && <AgentScriptsPage />}
      </main>
    </div>
  )
}

export default App
