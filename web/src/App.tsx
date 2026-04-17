import { useState } from 'react'
import { MatchResultPage } from './pages/MatchResultPage'
import { BomSessionListPage } from './pages/BomSessionListPage'
import { AgentScriptsPage } from './pages/AgentScriptsPage'
import { AgentAdminPage } from './pages/AgentAdminPage'
import { HsResolvePage } from './pages/HsResolvePage'
import { HsMetaAdminPage } from './pages/HsMetaAdminPage'

const LAST_BOM_KEY = 'bom_last_bom_id'

type Page = 'bom-list' | 'result' | 'agent-scripts' | 'agent-admin' | 'hs-resolve' | 'hs-meta'

function App() {
  const [page, setPage] = useState<Page>('bom-list')
  const [bomId, setBomId] = useState<string | null>(() => localStorage.getItem(LAST_BOM_KEY))

  const effectiveBomId = bomId || localStorage.getItem(LAST_BOM_KEY)

  return (
    <div className="min-h-screen bg-slate-50">
      <header className="bg-slate-800 text-white px-6 py-4 shadow">
        <div className="max-w-7xl mx-auto flex flex-wrap items-center justify-between gap-4">
          <h1 className="text-xl font-semibold">BOM 配单系统</h1>
          <nav className="flex flex-wrap gap-2">
            <button
              type="button"
              onClick={() => setPage('bom-list')}
              className={`px-3 py-1 rounded text-sm ${page === 'bom-list' ? 'bg-slate-600' : 'hover:bg-slate-700'}`}
            >
              BOM 会话
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
              title={!effectiveBomId ? '请先创建会话并上传 BOM' : '配单结果'}
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
              onClick={() => setPage('hs-resolve')}
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

      <main className="max-w-7xl mx-auto px-6 py-8">
        {page === 'bom-list' && (
          <BomSessionListPage
            onNavigateToMatch={(bid) => {
              setBomId(bid)
              localStorage.setItem(LAST_BOM_KEY, bid)
              setPage('result')
            }}
          />
        )}
        {page === 'result' && effectiveBomId && <MatchResultPage bomId={effectiveBomId} />}
        {page === 'agent-scripts' && <AgentScriptsPage />}
        {page === 'agent-admin' && <AgentAdminPage />}
        {page === 'hs-resolve' && <HsResolvePage />}
        {page === 'hs-meta' && <HsMetaAdminPage />}
      </main>
    </div>
  )
}

export default App
