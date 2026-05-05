import { useCallback, useEffect, useState } from 'react'
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

  const reloadPhase1 = useCallback(async () => {
    const [candidates, canonicals] = await Promise.all([
      listSessionLineMfrCandidates(sessionId),
      listManufacturerCanonicals(),
    ])
    setPendingRows(pendingRowsFromLineCandidates(candidates.items ?? []))
    setCanonicalRows(canonicals)
  }, [sessionId])

  const reloadPhase2 = useCallback(async () => {
    const q = await listQuoteItemMfrReviews(sessionId)
    setGateOpen(Boolean(q.gate_open))
    setQuoteReviews(q.items ?? [])
  }, [sessionId])

  useEffect(() => {
    let cancelled = false
    ;(async () => {
      setAliasErr(null)
      setQuoteErr(null)
      try {
        const [candidates, canonicals, reviews] = await Promise.all([
          listSessionLineMfrCandidates(sessionId),
          listManufacturerCanonicals(),
          listQuoteItemMfrReviews(sessionId),
        ])
        if (!cancelled) {
          setPendingRows(pendingRowsFromLineCandidates(candidates.items ?? []))
          setCanonicalRows(canonicals)
          setGateOpen(Boolean(reviews.gate_open))
          setQuoteReviews(reviews.items ?? [])
        }
      } catch (error) {
        if (!cancelled) {
          setPendingRows([])
          setCanonicalRows([])
          setQuoteReviews([])
          setGateOpen(false)
          setAliasErr(error instanceof Error ? error.message : '\u5382\u724c\u6570\u636e\u52a0\u8f7d\u5931\u8d25')
        }
      }
    })()
    return () => {
      cancelled = true
    }
  }, [sessionId])

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
      <QuoteItemMfrReviewSection
        sessionId={sessionId}
        gateOpen={gateOpen}
        items={quoteReviews}
        quoteErr={quoteErr}
        onAfterSubmit={reloadPhase2}
        onSubmitStart={() => setQuoteErr(null)}
        onSubmitError={(msg) => setQuoteErr(msg)}
      />
    </div>
  )
}
