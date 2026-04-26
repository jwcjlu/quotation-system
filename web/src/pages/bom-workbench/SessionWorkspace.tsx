import { useState } from 'react'
import { SourcingSessionPage } from '../SourcingSessionPage'
import { SESSION_WORKBENCH_TABS, type SessionWorkbenchTab } from './sessionTabs'

interface SessionWorkspaceProps {
  sessionId: string
  onBackToList?: () => void
  onNavigateToHsResolve?: (model: string, manufacturer: string) => void
}

function PlaceholderPanel({ label, sessionId }: { label: string; sessionId: string }) {
  return (
    <div className="rounded-lg border border-dashed border-slate-300 bg-white p-4 text-sm text-slate-600">
      <div className="font-medium text-slate-800">{label}</div>
      <div className="mt-1">{sessionId}</div>
    </div>
  )
}

export function SessionWorkspace({
  sessionId,
  onBackToList: _onBackToList,
  onNavigateToHsResolve: _onNavigateToHsResolve,
}: SessionWorkspaceProps) {
  const [currentTab, setCurrentTab] = useState<SessionWorkbenchTab>('overview')
  const currentLabel =
    SESSION_WORKBENCH_TABS.find((tab) => tab.id === currentTab)?.label || '\u6982\u89c8'
  const canUseSessionDetail = currentTab === 'lines' || currentTab === 'gaps' || currentTab === 'maintenance'

  return (
    <div className="space-y-4" data-testid="session-workspace-placeholder">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h3 className="text-lg font-semibold text-slate-800">{'\u4f1a\u8bdd\u5de5\u4f5c\u533a'}</h3>
          <p className="mt-1 text-xs text-slate-500">{sessionId}</p>
        </div>
      </div>

      <div
        role="tablist"
        aria-label="\u4f1a\u8bdd\u5de5\u4f5c\u533a"
        className="flex gap-2 overflow-x-auto rounded-lg border border-slate-200 bg-white p-2"
      >
        {SESSION_WORKBENCH_TABS.map((tab) => {
          const active = tab.id === currentTab
          return (
            <button
              key={tab.id}
              type="button"
              role="tab"
              aria-selected={active}
              onClick={() => setCurrentTab(tab.id)}
              className={`shrink-0 rounded-md px-3 py-2 text-sm font-medium ${
                active ? 'bg-slate-900 text-white' : 'text-slate-600 hover:bg-slate-100'
              }`}
            >
              {tab.label}
            </button>
          )
        })}
      </div>

      {canUseSessionDetail ? (
        <SourcingSessionPage embedded sessionId={sessionId} />
      ) : (
        <PlaceholderPanel label={currentLabel} sessionId={sessionId} />
      )}
    </div>
  )
}
