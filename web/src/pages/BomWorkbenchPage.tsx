import { useState } from 'react'
import { UploadPage } from './UploadPage'
import { SessionListPanel } from './bom-workbench/SessionListPanel'
import { SessionWorkspace } from './bom-workbench/SessionWorkspace'

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
  const [selectedSessionLineCount, setSelectedSessionLineCount] = useState<number | null>(null)
  const [uploadOpen, setUploadOpen] = useState(false)
  const [mobileDetailOpen, setMobileDetailOpen] = useState(false)

  const selectSession = (sessionId: string, lineCount?: number) => {
    localStorage.setItem(LAST_SESSION_KEY, sessionId)
    localStorage.setItem(LAST_BOM_KEY, sessionId)
    setSelectedSessionId(sessionId)
    setSelectedSessionLineCount(typeof lineCount === 'number' ? lineCount : null)
    setMobileDetailOpen(true)
  }

  const handleUploadSuccess = (sessionId: string) => {
    setUploadOpen(false)
    selectSession(sessionId)
  }

  return (
    <div className="bg-[#f4f6fa] text-slate-950" data-testid="bom-workbench-page">
      <div
        className="overflow-hidden rounded-lg border border-[#cbd6e5] bg-white"
        data-testid="bom-workbench-shell"
      >
        <div className="min-h-[642px] lg:grid lg:grid-cols-[310px_minmax(0,1fr)]">
          <div className={mobileDetailOpen ? 'hidden lg:block' : 'block'}>
            <SessionListPanel
              selectedSessionId={selectedSessionId}
              onSelectSession={selectSession}
              onSelectedSessionLineCount={setSelectedSessionLineCount}
              onCreateSession={() => setUploadOpen(true)}
            />
          </div>
          <section
            className={
              mobileDetailOpen
                ? 'block min-h-[642px] bg-[#f8fafc] p-4 lg:p-7'
                : 'hidden min-h-[642px] bg-[#f8fafc] p-4 lg:block lg:p-7'
            }
          >
            {selectedSessionId ? (
              <SessionWorkspace
                sessionId={selectedSessionId}
                lineCount={selectedSessionLineCount}
                onBackToList={() => setMobileDetailOpen(false)}
                onNavigateToHsResolve={_onNavigateToHsResolve}
              />
            ) : (
              <div className="rounded-lg border border-dashed border-[#d7e0ed] bg-white p-6 text-sm text-slate-500">
                {'\u4ece\u5de6\u4fa7\u9009\u62e9 BOM \u4f1a\u8bdd'}
              </div>
            )}
          </section>
        </div>
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
