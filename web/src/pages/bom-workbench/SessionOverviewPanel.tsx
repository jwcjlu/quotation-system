import { useEffect, useMemo, useState } from 'react'
import {
  getSession,
  listSessionSearchTasks,
  type GetSessionReply,
  type SearchTaskStatusSummary,
} from '../../api'

interface SessionOverviewPanelProps {
  sessionId: string
  sessionStatus: string
  lineCount?: number | null
}

function normalizeStatus(status: string) {
  const trimmed = status.trim()
  if (trimmed === 'data_ready') return 'ready'
  return trimmed || 'unknown'
}

function statusClassName(status: string) {
  const normalized = normalizeStatus(status)
  if (normalized === 'ready') return 'text-[#12805c]'
  if (normalized === 'unknown') return 'text-slate-500'
  return 'text-[#a76505]'
}

function normalizeImportStatusText(status?: string) {
  const s = (status || '').trim().toLowerCase()
  if (s === 'ready') return '已完成'
  if (s === 'parsing') return '解析中'
  if (s === 'failed') return '失败'
  if (s === 'idle') return '待导入'
  return status?.trim() || '未知'
}

function importProgressClass(status?: string) {
  const s = (status || '').trim().toLowerCase()
  if (s === 'ready') return 'text-[#12805c]'
  if (s === 'failed') return 'text-[#c2410c]'
  if (s === 'parsing') return 'text-[#2b59c3]'
  return 'text-slate-500'
}

function importProgressBarClass(status?: string) {
  const s = (status || '').trim().toLowerCase()
  if (s === 'ready') return 'bg-[#12805c]'
  if (s === 'failed') return 'bg-[#c2410c]'
  if (s === 'parsing') return 'bg-[#2b59c3]'
  return 'bg-slate-400'
}

export function SessionOverviewPanel({
  sessionId,
  sessionStatus,
  lineCount,
}: SessionOverviewPanelProps) {
  const [searchSummary, setSearchSummary] = useState<SearchTaskStatusSummary | null>(null)
  const [sessionMeta, setSessionMeta] = useState<GetSessionReply | null>(null)
  const displayStatus = normalizeStatus(sessionStatus)
  const displayLineCount = typeof lineCount === 'number' ? String(lineCount) : '--'
  const searchRetryableCount = useMemo(() => searchSummary?.retryable ?? 0, [searchSummary])
  const importStatusText = normalizeImportStatusText(sessionMeta?.import_status)
  const importProgressNumber =
    typeof sessionMeta?.import_progress === 'number'
      ? Math.max(0, Math.min(100, sessionMeta.import_progress))
      : displayStatus === 'ready'
        ? 100
        : 0
  const importProgressValue = `${importProgressNumber}%`
  const importDetail =
    sessionMeta?.import_error?.trim() ||
    sessionMeta?.import_message?.trim() ||
    sessionMeta?.import_stage?.trim() ||
    '暂无导入任务'

  useEffect(() => {
    let cancelled = false
    setSearchSummary(null)
    setSessionMeta(null)
    ;(async () => {
      try {
        const [summaryReply, sessionReply] = await Promise.all([
          listSessionSearchTasks(sessionId),
          getSession(sessionId),
        ])
        if (!cancelled) {
          setSearchSummary(summaryReply.summary)
          setSessionMeta(sessionReply)
        }
      } catch {
        if (!cancelled) {
          setSearchSummary(null)
          setSessionMeta(null)
        }
      }
    })()
    return () => {
      cancelled = true
    }
  }, [sessionId])

  return (
    <section data-testid="session-overview-panel" aria-label="\u4f1a\u8bdd\u6982\u89c8" className="space-y-4">
      <article className="rounded-lg border border-[#d7e0ed] bg-white px-5 py-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h4 className="text-lg font-bold leading-none text-slate-950">导入进度</h4>
          <div className={`text-xl font-bold leading-none ${importProgressClass(sessionMeta?.import_status)}`}>
            {importProgressValue}
          </div>
        </div>
        <div className="mt-3 h-2.5 overflow-hidden rounded-full bg-slate-200">
          <div
            className={`h-full rounded-full transition-all ${importProgressBarClass(sessionMeta?.import_status)}`}
            style={{ width: `${importProgressNumber}%` }}
            role="progressbar"
            aria-label="导入进度条"
            aria-valuemin={0}
            aria-valuemax={100}
            aria-valuenow={importProgressNumber}
          />
        </div>
        <div className="mt-2 flex flex-wrap items-center gap-2 text-sm">
          <span className="font-medium text-slate-700">{importStatusText}</span>
          <span className="text-slate-500">{importDetail}</span>
        </div>
      </article>

      <div className="grid gap-4 xl:grid-cols-3">
      <article className="min-h-[174px] rounded-lg border border-[#d7e0ed] bg-white px-5 py-5">
        <h4 className="text-xl font-bold leading-none text-slate-950">{'BOM \u884c\u6570'}</h4>
        <div className="mt-5 text-4xl font-bold leading-none text-[#2b59c3]">
          {displayLineCount}
        </div>
        <p className="mt-5 text-sm leading-6 text-slate-600">
          {'\u70b9\u51fb BOM \u884c Tab \u540e'}
          <br />
          {'\u624d\u52a0\u8f7d\u5b8c\u6574\u884c\u6570\u636e'}
        </p>
      </article>

      <article className="min-h-[174px] rounded-lg border border-[#d7e0ed] bg-white px-5 py-5">
        <h4 className="text-xl font-bold leading-none text-slate-950">{'\u641c\u7d22\u4efb\u52a1'}</h4>
        <div className="mt-5 text-4xl font-bold leading-none text-[#2b59c3]">
          {searchSummary ? searchSummary.total : '--'}
        </div>
        <p className="mt-5 text-sm leading-6 text-slate-600">
          {searchSummary
            ? `${searchSummary.searching} \u5904\u7406\u4e2d / ${searchRetryableCount} \u53ef\u91cd\u8bd5`
            : '\u70b9\u51fb\u641c\u7d22\u6e05\u6d17 Tab \u540e'}
          <br />
          {'\u67e5\u770b\u5e73\u53f0\u4efb\u52a1\u4e0e\u91cd\u8bd5\u72b6\u6001'}
        </p>
      </article>

      <article className="min-h-[174px] rounded-lg border border-[#d7e0ed] bg-white px-5 py-5">
        <h4 className="text-xl font-bold leading-none text-slate-950">{'\u5f85\u5904\u7406\u7f3a\u53e3'}</h4>
        <div className="mt-5 text-4xl font-bold leading-none text-[#a76505]">--</div>
        <p className="mt-5 text-sm leading-6 text-slate-600">
          {'\u70b9\u51fb\u7f3a\u53e3\u5904\u7406 Tab \u540e'}
          <br />
          {'\u518d\u770b\u4eba\u5de5\u62a5\u4ef7\u4e0e\u66ff\u4ee3\u6599'}
        </p>
      </article>
      </div>
    </section>
  )
}
