import { useState } from 'react'
import { UploadPage } from './UploadPage'
import { SessionListPanel } from './bom-workbench/SessionListPanel'

const LAST_BOM_KEY = 'bom_last_bom_id'
const LAST_SESSION_KEY = 'bom_last_session_id'

interface BomWorkbenchPageProps {
  onNavigateToHsResolve?: (model: string, manufacturer: string) => void
}

export function BomWorkbenchPage({
  onNavigateToHsResolve: _onNavigateToHsResolve,
}: BomWorkbenchPageProps) {
  const [selectedSessionId, setSelectedSessionId] = useState<string | null>(() =>
    localStorage.getItem(LAST_SESSION_KEY)
  )
  const [uploadOpen, setUploadOpen] = useState(false)

  const selectSession = (sessionId: string) => {
    localStorage.setItem(LAST_SESSION_KEY, sessionId)
    localStorage.setItem(LAST_BOM_KEY, sessionId)
    setSelectedSessionId(sessionId)
  }

  const handleUploadSuccess = (sessionId: string) => {
    setUploadOpen(false)
    selectSession(sessionId)
  }

  return (
    <div className="space-y-6" data-testid="bom-workbench-page">
      <div>
        <h2 className="text-2xl font-bold text-slate-800">{'BOM\u5de5\u4f5c\u53f0'}</h2>
        <p className="mt-1 text-sm text-slate-600">
          {'\u96c6\u4e2d\u7ba1\u7406 BOM \u4f1a\u8bdd\u3001\u641c\u7d22\u6e05\u6d17\u3001\u7f3a\u53e3\u5904\u7406\u548c\u5339\u914d\u7ed3\u679c\u3002'}
        </p>
      </div>

      <div className="overflow-hidden rounded-xl border border-slate-200 bg-slate-50 shadow-sm lg:grid lg:grid-cols-[22rem_minmax(0,1fr)]">
        <SessionListPanel
          selectedSessionId={selectedSessionId}
          onSelectSession={selectSession}
          onCreateSession={() => setUploadOpen(true)}
        />
        <section className="min-h-[32rem] p-4">
          {selectedSessionId ? (
            <div
              className="rounded-lg border border-dashed border-slate-300 bg-white p-4 text-sm text-slate-600"
              data-testid="session-workspace-placeholder"
            >
              {selectedSessionId}
            </div>
          ) : (
            <div className="rounded-lg border border-dashed border-slate-300 bg-white p-6 text-sm text-slate-500">
              {'\u4ece\u5de6\u4fa7\u9009\u62e9 BOM \u4f1a\u8bdd'}
            </div>
          )}
        </section>
      </div>

      {uploadOpen && (
        <div
          className="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-black/40 p-4 pt-10"
          role="dialog"
          aria-modal="true"
          aria-labelledby="bom-upload-dialog-title"
          onClick={(event) => {
            if (event.target === event.currentTarget) setUploadOpen(false)
          }}
        >
          <div
            className="max-h-[90vh] w-full max-w-4xl overflow-y-auto rounded-xl border border-slate-200 bg-white p-4 shadow-xl md:p-6"
            onClick={(event) => event.stopPropagation()}
          >
            <div className="mb-4 flex items-center justify-between gap-4">
              <h3 id="bom-upload-dialog-title" className="text-lg font-semibold text-slate-800">
                {'\u4e0a\u4f20 BOM'}
              </h3>
              <button
                type="button"
                onClick={() => setUploadOpen(false)}
                className="px-2 py-1 text-sm text-slate-500 hover:text-slate-800"
              >
                {'\u5173\u95ed'}
              </button>
            </div>
            <UploadPage embedded onSuccess={handleUploadSuccess} />
          </div>
        </div>
      )}
    </div>
  )
}
