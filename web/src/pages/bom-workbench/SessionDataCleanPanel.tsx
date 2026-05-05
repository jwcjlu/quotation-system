import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  applyManufacturerAliasesToSession,
  approveSessionLineMfrCleaning,
  listManufacturerCanonicals,
  listQuoteItemMfrReviews,
  listSessionLineMfrCandidates,
  type ManufacturerCanonicalRow,
  type QuoteItemMfrReviewItem,
} from '../../api'
import type { PendingMfrRow } from './ManufacturerAliasReviewPanel'
import { QuoteItemMfrReviewSection } from './QuoteItemMfrReviewSection'
import { pendingRowsFromLineCandidates, SessionLineMfrPhasePanel } from './SessionLineMfrPhasePanel'

interface SessionDataCleanPanelProps {
  sessionId: string
}

export function SessionDataCleanPanel({ sessionId }: SessionDataCleanPanelProps) {
  const [pendingRows, setPendingRows] = useState<PendingMfrRow[]>([])
  const [canonicalRows, setCanonicalRows] = useState<ManufacturerCanonicalRow[]>([])
  const [aliasErr, setAliasErr] = useState<string | null>(null)
  const [gateOpen, setGateOpen] = useState(false)
  const [quoteReviews, setQuoteReviews] = useState<QuoteItemMfrReviewItem[]>([])
  const [quoteErr, setQuoteErr] = useState<string | null>(null)
  const [showAllQuoteMfrPending, setShowAllQuoteMfrPending] = useState(false)
  const [quoteMfrAllPendingTotal, setQuoteMfrAllPendingTotal] = useState<number | undefined>(undefined)

  const reloadPhase1 = useCallback(async () => {
    const [candidates, canonicals] = await Promise.all([
      listSessionLineMfrCandidates(sessionId),
      listManufacturerCanonicals(),
    ])
    setPendingRows(pendingRowsFromLineCandidates(candidates.items ?? []))
    setCanonicalRows(canonicals)
  }, [sessionId])

  const reloadPhase2 = useCallback(
    async (opts?: { includeAllPendingOverride?: boolean }) => {
      const includeAll =
        opts?.includeAllPendingOverride !== undefined
          ? opts.includeAllPendingOverride
          : showAllQuoteMfrPending
      const reviews = await listQuoteItemMfrReviews(sessionId, {
        includeAllPendingQuoteMfr: includeAll,
      })
      setGateOpen(Boolean(reviews.gate_open))
      setQuoteReviews(reviews.items ?? [])
      setQuoteMfrAllPendingTotal(
        reviews.all_pending_quote_mfr_count != null
          ? reviews.all_pending_quote_mfr_count
          : (reviews.items ?? []).length,
      )
    },
    [sessionId, showAllQuoteMfrPending],
  )

  useEffect(() => {
    let cancelled = false
    ;(async () => {
      setAliasErr(null)
      setQuoteErr(null)
      try {
        const [candidates, canonicals, reviews] = await Promise.all([
          listSessionLineMfrCandidates(sessionId),
          listManufacturerCanonicals(),
          listQuoteItemMfrReviews(sessionId, {
            includeAllPendingQuoteMfr: false,
          }),
        ])
        if (!cancelled) {
          setPendingRows(pendingRowsFromLineCandidates(candidates.items ?? []))
          setCanonicalRows(canonicals)
          setGateOpen(Boolean(reviews.gate_open))
          setQuoteReviews(reviews.items ?? [])
          setQuoteMfrAllPendingTotal(
            reviews.all_pending_quote_mfr_count != null
              ? reviews.all_pending_quote_mfr_count
              : (reviews.items ?? []).length,
          )
        }
      } catch (error) {
        if (!cancelled) {
          setPendingRows([])
          setCanonicalRows([])
          setQuoteReviews([])
          setQuoteMfrAllPendingTotal(undefined)
          setGateOpen(false)
          setAliasErr(error instanceof Error ? error.message : '厂牌数据加载失败')
        }
      }
    })()
    return () => {
      cancelled = true
    }
  }, [sessionId])

  useEffect(() => {
    setShowAllQuoteMfrPending(false)
  }, [sessionId])

  const sortedQuoteReviews = useMemo(() => {
    if (!quoteReviews.length) return quoteReviews
    return [...quoteReviews].sort(
      (a, b) => a.line_no - b.line_no || a.quote_item_id - b.quote_item_id,
    )
  }, [quoteReviews])

  async function reloadAliasData() {
    await Promise.all([reloadPhase1(), reloadPhase2()])
  }

  async function handleApproveSessionLine(input: { alias: string; canonical_id: string; display_name: string }) {
    await approveSessionLineMfrCleaning(sessionId, input)
    await reloadAliasData()
  }

  async function handleApplyExistingAliases() {
    await applyManufacturerAliasesToSession(sessionId)
    await reloadAliasData()
  }

  return (
    <div className="space-y-6" data-testid="session-data-clean-panel">
      <SessionLineMfrPhasePanel
        pendingRows={pendingRows}
        canonicalRows={canonicalRows}
        aliasErr={aliasErr}
        onApprove={handleApproveSessionLine}
        onApplyExistingAliases={handleApplyExistingAliases}
      />
      {gateOpen &&
        (sortedQuoteReviews.length > 0 || (quoteMfrAllPendingTotal ?? 0) > 0) && (
          <div className="flex flex-wrap items-center gap-3 text-sm text-slate-700">
            <label className="inline-flex cursor-pointer items-center gap-2">
              <input
                type="checkbox"
                checked={showAllQuoteMfrPending}
                onChange={(e) => {
                  const v = e.target.checked
                  setShowAllQuoteMfrPending(v)
                  void reloadPhase2({ includeAllPendingOverride: v })
                }}
                className="rounded border-slate-300"
                data-testid="quote-mfr-show-all-pending"
              />
              <span>{'显示全部待审报价'}</span>
            </label>
            {!showAllQuoteMfrPending &&
            sortedQuoteReviews.length < (quoteMfrAllPendingTotal ?? 0) ? (
              <span className="text-slate-500">
                {'当前展示 '}
                {sortedQuoteReviews.length}
                {' / '}
                {quoteMfrAllPendingTotal}
                {' 条。'}
              </span>
            ) : null}
          </div>
        )}
      <QuoteItemMfrReviewSection
        sessionId={sessionId}
        gateOpen={gateOpen}
        items={sortedQuoteReviews}
        quoteMfrEmptyHint={
          sortedQuoteReviews.length === 0 && (quoteMfrAllPendingTotal ?? 0) > 0
            ? `当前暂无可展示待审报价，系统仍统计到 ${quoteMfrAllPendingTotal} 条待审记录。`
            : undefined
        }
        quoteErr={quoteErr}
        onAfterSubmit={reloadPhase2}
        onSubmitStart={() => setQuoteErr(null)}
        onSubmitError={(msg) => setQuoteErr(msg)}
      />
    </div>
  )
}
