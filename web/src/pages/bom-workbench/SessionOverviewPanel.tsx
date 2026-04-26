import { useEffect, useMemo, useState } from 'react'
import { listSessionSearchTasks, type SearchTaskStatusSummary } from '../../api'

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

export function SessionOverviewPanel({
  sessionId,
  sessionStatus,
  lineCount,
}: SessionOverviewPanelProps) {
  const [searchSummary, setSearchSummary] = useState<SearchTaskStatusSummary | null>(null)
  const displayStatus = normalizeStatus(sessionStatus)
  const displayLineCount = typeof lineCount === 'number' ? String(lineCount) : '--'
  const searchRetryableCount = useMemo(() => searchSummary?.retryable ?? 0, [searchSummary])

  useEffect(() => {
    let cancelled = false
    setSearchSummary(null)
    ;(async () => {
      try {
        const reply = await listSessionSearchTasks(sessionId)
        if (!cancelled) setSearchSummary(reply.summary)
      } catch {
        if (!cancelled) setSearchSummary(null)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [sessionId])

  return (
    <section
      className="grid gap-4 xl:grid-cols-4"
      data-testid="session-overview-panel"
      aria-label="\u4f1a\u8bdd\u6982\u89c8"
    >
      <article className="min-h-[174px] rounded-lg border border-[#d7e0ed] bg-white px-5 py-5">
        <h4 className="text-xl font-bold leading-none text-slate-950">{'\u5bfc\u5165\u72b6\u6001'}</h4>
        <div className={`mt-5 text-4xl font-bold leading-none ${statusClassName(sessionStatus)}`}>
          {displayStatus}
        </div>
        <p className="mt-5 text-sm leading-6 text-slate-600">
          {'\u53ea\u5c55\u793a\u4f1a\u8bdd\u6458\u8981\u548c\u4e0b\u4e00\u6b65\u72b6\u6001'}
          <br />
          {'\u4e0d\u51fa\u73b0 BOM \u884c\u5927\u8868'}
        </p>
      </article>

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
    </section>
  )
}
