import { useState, useCallback, useRef } from 'react'
import { getSession } from './api'
import { MatchResultPage } from './pages/MatchResultPage'
import { BomSessionListPage } from './pages/BomSessionListPage'
import { AgentScriptsPage } from './pages/AgentScriptsPage'
import { AgentAdminPage } from './pages/AgentAdminPage'
import { HsResolvePage, type HsResolvePrefill } from './pages/HsResolvePage'
import { HsMetaAdminPage } from './pages/HsMetaAdminPage'

const LAST_BOM_KEY = 'bom_last_bom_id'

type Page = 'bom-list' | 'result' | 'agent-scripts' | 'agent-admin' | 'hs-resolve' | 'hs-meta'

const SESSION_MATCH_READY = 'data_ready'

function App() {
  const [page, setPage] = useState<Page>('bom-list')
  const [bomId, setBomId] = useState<string | null>(() => localStorage.getItem(LAST_BOM_KEY))
  const [matchNavHint, setMatchNavHint] = useState<string | null>(null)
  const [hsPrefill, setHsPrefill] = useState<HsResolvePrefill | null>(null)
  const hsPrefillKeySeq = useRef(0)

  const effectiveBomId = bomId || localStorage.getItem(LAST_BOM_KEY)

  const openMatchPage = useCallback(async () => {
    const id = bomId || localStorage.getItem(LAST_BOM_KEY)
    setMatchNavHint(null)
    if (!id) return
    try {
      const s = await getSession(id)
      const st = (s.status || '').trim()
      if (st !== SESSION_MATCH_READY) {
        setMatchNavHint(`当前会话状态为「${st || '未知'}」，仅当状态为 data_ready 时可进入配单。`)
        return
      }
      setBomId(id)
      localStorage.setItem(LAST_BOM_KEY, id)
      setPage('result')
    } catch (e) {
      setMatchNavHint(e instanceof Error ? e.message : '无法校验会话状态')
    }
  }, [bomId])

  return (
    <div className="min-h-screen bg-slate-50">
      <header className="bg-slate-800 text-white px-6 py-4 shadow">
        <div className="max-w-7xl mx-auto flex flex-wrap items-center justify-between gap-4">
          <h1 className="text-xl font-semibold">BOM 配单系统</h1>
          <nav className="flex flex-wrap gap-2">
            <button
              type="button"
              onClick={() => {
                setMatchNavHint(null)
                setPage('bom-list')
              }}
              className={`px-3 py-1 rounded text-sm ${page === 'bom-list' ? 'bg-slate-600' : 'hover:bg-slate-700'}`}
            >
              BOM 会话
            </button>
            <button
              type="button"
              onClick={() => void openMatchPage()}
              className={`px-3 py-1 rounded text-sm ${page === 'result' ? 'bg-slate-600' : 'hover:bg-slate-700'} ${!effectiveBomId ? 'opacity-50 cursor-not-allowed' : ''}`}
              title={
                !effectiveBomId
                  ? '请先创建会话并上传 BOM'
                  : '进入配单（需会话状态 data_ready）'
              }
              disabled={!effectiveBomId}
            >
              匹配单
            </button>
            <button
              type="button"
              onClick={() => setPage('agent-scripts')}
              className={`px-3 py-1 rounded text-sm ${page === 'agent-scripts' ? 'bg-slate-600' : 'hover:bg-slate-700'}`}
            >
              脚本包
            </button>
            <button
              type="button"
              onClick={() => setPage('agent-admin')}
              className={`px-3 py-1 rounded text-sm ${page === 'agent-admin' ? 'bg-slate-600' : 'hover:bg-slate-700'}`}
            >
              Agent 运维
            </button>
            <button
              type="button"
              onClick={() => {
                setHsPrefill(null)
                setPage('hs-resolve')
              }}
              className={`px-3 py-1 rounded text-sm ${page === 'hs-resolve' ? 'bg-slate-600' : 'hover:bg-slate-700'}`}
            >
              HS 型号解析
            </button>
            <button
              type="button"
              onClick={() => setPage('hs-meta')}
              className={`px-3 py-1 rounded text-sm ${page === 'hs-meta' ? 'bg-slate-600' : 'hover:bg-slate-700'}`}
            >
              HS 元数据
            </button>
          </nav>
        </div>
      </header>

      {matchNavHint && (
        <div className="bg-amber-50 border-b border-amber-200 text-amber-950 text-sm px-6 py-2 max-w-7xl mx-auto flex justify-between gap-4 items-start">
          <span>{matchNavHint}</span>
          <button type="button" className="text-amber-900 underline shrink-0" onClick={() => setMatchNavHint(null)}>
            关闭
          </button>
        </div>
      )}

      <main className="max-w-7xl mx-auto px-6 py-8">
        {page === 'bom-list' && (
          <BomSessionListPage
            onEnterMatch={(sid) => {
              setBomId(sid)
              localStorage.setItem(LAST_BOM_KEY, sid)
              setMatchNavHint(null)
              setPage('result')
            }}
          />
        )}
        {page === 'result' && effectiveBomId && (
          <MatchResultPage
            bomId={effectiveBomId}
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
