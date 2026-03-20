import { useState } from 'react'
import { UploadPage } from './pages/UploadPage'
import { MatchResultPage } from './pages/MatchResultPage'

type Page = 'upload' | 'result'

function App() {
  const [page, setPage] = useState<Page>('upload')
  const [bomId, setBomId] = useState<string | null>(null)

  const onUploadSuccess = (id: string) => {
    setBomId(id)
    setPage('result')
  }

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
            {bomId && (
              <button
                onClick={() => setPage('result')}
                className={`px-3 py-1 rounded ${page === 'result' ? 'bg-slate-600' : 'hover:bg-slate-700'}`}
              >
                配单结果
              </button>
            )}
          </nav>
        </div>
      </header>

      <main className="max-w-7xl mx-auto px-6 py-8">
        {page === 'upload' && <UploadPage onSuccess={onUploadSuccess} />}
        {page === 'result' && bomId && <MatchResultPage bomId={bomId} />}
      </main>
    </div>
  )
}

export default App
