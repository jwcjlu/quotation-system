import { useEffect, useState } from 'react'
import { getSession } from '../../api'
import { SessionGapsPanel } from './SessionGapsPanel'
import { SessionLinesPanel } from './SessionLinesPanel'
import { SessionMaintenancePanel } from './SessionMaintenancePanel'
import { SessionMatchResultPanel } from './SessionMatchResultPanel'
import { SessionOverviewPanel } from './SessionOverviewPanel'
import { SessionSearchCleanPanel } from './SessionSearchCleanPanel'
import { SESSION_WORKBENCH_TABS, type SessionWorkbenchTab } from './sessionTabs'

const SESSION_MATCH_READY = 'data_ready'

interface SessionWorkspaceProps {
  sessionId: string
  lineCount?: number | null
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
  lineCount,
  onBackToList,
  onNavigateToHsResolve,
}: SessionWorkspaceProps) {
  const [currentTab, setCurrentTab] = useState<SessionWorkbenchTab>('lines')
  const [sessionStatus, setSessionStatus] = useState('')
  const [sessionName, setSessionName] = useState('')
  const currentLabel =
    SESSION_WORKBENCH_TABS.find((tab) => tab.id === currentTab)?.label || '\u6982\u89c8'
  const canEnterMatch = sessionStatus === SESSION_MATCH_READY

  useEffect(() => {
    let cancelled = false
    setSessionStatus('')
    ;(async () => {
      try {
        const session = await getSession(sessionId)
        if (!cancelled) {
          setSessionStatus((session.status || '').trim())
          setSessionName(session.title || '')
        }
      } catch {
        if (!cancelled) {
          setSessionStatus('')
          setSessionName('')
        }
      }
    })()
    return () => {
      cancelled = true
    }
  }, [sessionId])

  return (
    <div className="space-y-4" data-testid="session-workspace-placeholder">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h3 className="text-2xl font-bold leading-tight text-slate-950">{'\u4f1a\u8bdd\u5de5\u4f5c\u533a'}</h3>
          <p className="mt-3 text-sm text-slate-700">
            {sessionName ? `${sessionName} / ${sessionId}` : sessionId}
          </p>
        </div>
        {onBackToList && (
          <button
            type="button"
            onClick={onBackToList}
            className="rounded-md border border-[#d7e0ed] bg-white px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50 lg:hidden"
          >
            {'\u8fd4\u56de\u4f1a\u8bdd\u5217\u8868'}
          </button>
        )}
      </div>

      <div
        role="tablist"
        aria-label="\u4f1a\u8bdd\u5de5\u4f5c\u533a"
        className="flex gap-3 overflow-x-auto rounded-lg border border-[#d7e0ed] bg-white px-4 py-2"
      >
        {SESSION_WORKBENCH_TABS.map((tab) => {
          const active = tab.id === currentTab
          const disabled = tab.id === 'match' && !canEnterMatch
          return (
            <button
              key={tab.id}
              type="button"
              role="tab"
              aria-selected={active}
              disabled={disabled}
              onClick={() => {
                if (!disabled) setCurrentTab(tab.id)
              }}
              className={`h-9 shrink-0 rounded-md px-4 text-sm font-bold transition ${
                active
                  ? 'bg-[#111827] text-white'
                  : disabled
                    ? 'cursor-not-allowed text-slate-400'
                    : 'text-slate-950 hover:bg-slate-100'
              }`}
            >
              {tab.label}
            </button>
          )
        })}
      </div>

      {!canEnterMatch && (
        <div className="sr-only rounded-lg border border-[#f0c77d] bg-[#fff7e8] px-4 py-3 text-sm text-amber-950">
          {'\u4f1a\u8bdd\u72b6\u6001 '}
          <span className="font-mono">{sessionStatus || 'unknown'}</span>
          {' \u5c1a\u4e0d\u662f data_ready\uff0c\u6682\u4e0d\u80fd\u8fdb\u5165\u5339\u914d\u7ed3\u679c\u3002'}
        </div>
      )}

      {currentTab === 'match' && canEnterMatch ? (
        <SessionMatchResultPanel bomId={sessionId} onNavigateToHsResolve={onNavigateToHsResolve} />
      ) : currentTab === 'search-clean' ? (
        <SessionSearchCleanPanel sessionId={sessionId} />
      ) : currentTab === 'lines' ? (
        <SessionLinesPanel sessionId={sessionId} />
      ) : currentTab === 'gaps' ? (
        <SessionGapsPanel sessionId={sessionId} />
      ) : currentTab === 'maintenance' ? (
        <SessionMaintenancePanel sessionId={sessionId} />
      ) : currentTab === 'overview' ? (
        <SessionOverviewPanel
          sessionId={sessionId}
          sessionStatus={sessionStatus}
          lineCount={lineCount}
        />
      ) : (
        <PlaceholderPanel label={currentLabel} sessionId={sessionId} />
      )}
    </div>
  )
}
