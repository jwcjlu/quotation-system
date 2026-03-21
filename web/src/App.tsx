import { useState } from 'react'
import { UploadPage } from './pages/UploadPage'
import { MatchResultPage } from './pages/MatchResultPage'

const LAST_BOM_KEY = 'bom_last_bom_id'

type Page = 'upload' | 'result'

function App() {
  const [page, setPage] = useState<Page>('upload')
  const [bomId, setBomId] = useState<string | null>(() => localStorage.getItem(LAST_BOM_KEY))

  const onUploadSuccess = (id: string) => {
    setBomId(id)
    localStorage.setItem(LAST_BOM_KEY, id)
    setPage('result')
  }

  const effectiveBomId = bomId || localStorage.getItem(LAST_BOM_KEY)

  return (
    <div className="min-h-screen bg-slate-50">
      <header className="bg-slate-800 text-white px-6 py-4 shadow">
        <div className="max-w-7xl mx-auto flex items-center justify-between">
          <h1 className="text-xl font-semibold">BOM 配单系统</h1>
          <nav className="flex gap-4">
            <button
              onClick={() => setPage('upload')}
              className={`px-3 py-1 rounded ${page === 'upload' ? 'bg-slate-600' : 'hover:bg-slate-700'}`}
            >
              上传 BOM
            </button>
            <button
              onClick={() => {
                const id = bomId || localStorage.getItem(LAST_BOM_KEY)
                if (id) {
                  setBomId(id)
                  setPage('result')
                }
              }}
              className={`px-3 py-1 rounded ${page === 'result' ? 'bg-slate-600' : 'hover:bg-slate-700'} ${!effectiveBomId ? 'opacity-50 cursor-not-allowed' : ''}`}
              title={!effectiveBomId ? '请先上传 BOM' : '查看最后一次匹配结果'}
              disabled={!effectiveBomId}
            >
              匹配单
            </button>
          </nav>
        </div>
      </header>

      <main className="max-w-7xl mx-auto px-6 py-8">
        {page === 'upload' && <UploadPage onSuccess={onUploadSuccess} />}
        {page === 'result' && effectiveBomId && <MatchResultPage bomId={effectiveBomId} />}
      </main>
    </div>
  )
}

export default App
