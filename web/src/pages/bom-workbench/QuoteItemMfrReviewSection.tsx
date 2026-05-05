import { useState } from 'react'
import type { QuoteItemMfrReviewItem } from '../../api'
import { submitQuoteItemMfrReview } from '../../api'

export interface QuoteItemMfrReviewSectionProps {
  sessionId: string
  gateOpen: boolean
  items: QuoteItemMfrReviewItem[]
  quoteMfrEmptyHint?: string
  quoteErr: string | null
  onAfterSubmit: () => Promise<void>
  onSubmitError?: (message: string) => void
  onSubmitStart?: () => void
}

export function QuoteItemMfrReviewSection({
  sessionId,
  gateOpen,
  items,
  quoteMfrEmptyHint,
  quoteErr,
  onAfterSubmit,
  onSubmitError,
  onSubmitStart,
}: QuoteItemMfrReviewSectionProps) {
  const [busyId, setBusyId] = useState<number | null>(null)
  const [rejectReasonByItem, setRejectReasonByItem] = useState<Record<number, string>>({})

  async function handleDecision(item: QuoteItemMfrReviewItem, decision: 'accept' | 'reject') {
    onSubmitStart?.()
    setBusyId(item.quote_item_id)
    try {
      const reason = decision === 'reject' ? (rejectReasonByItem[item.quote_item_id] ?? '').trim() : undefined
      const canonicalId = item.line_manufacturer_canonical_id?.trim()
      await submitQuoteItemMfrReview(sessionId, {
        quote_item_id: item.quote_item_id,
        decision,
        ...(decision === 'accept'
          ? { manufacturer_canonical_id: canonicalId }
          : {}),
        ...(reason ? { reason } : {}),
      })
      await onAfterSubmit()
    } catch (e) {
      const msg = e instanceof Error ? e.message : '提交失败'
      onSubmitError?.(msg)
    } finally {
      setBusyId(null)
    }
  }

  const phaseLocked = !gateOpen

  return (
    <section
      className={`space-y-4 ${phaseLocked ? 'opacity-60' : ''}`}
      data-testid="quote-item-mfr-phase"
      aria-disabled={phaseLocked}
    >
      <h3 className="text-lg font-bold text-slate-900">{'报价厂牌确认'}</h3>
      {phaseLocked && (
        <p className="text-sm text-slate-600">
          {'需先完成上方需求行厂牌（所有非空厂牌行填齐 canonical）后才可审报价。'}
        </p>
      )}
      {quoteErr && (
        <div className="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-900">{quoteErr}</div>
      )}
      {gateOpen && items.length === 0 && (
        <p className={`text-sm ${quoteMfrEmptyHint ? 'text-amber-900' : 'text-slate-500'}`}>
          {quoteMfrEmptyHint ?? '暂无待确认报价厂牌。'}
        </p>
      )}
      {gateOpen && items.length > 0 && (
        <div
          className="overflow-x-auto rounded-lg border border-[#d7e0ed] bg-white"
          data-testid="quote-item-mfr-review-table-wrap"
        >
          <table className="min-w-full text-sm">
            <thead>
              <tr className="border-b border-slate-200 bg-slate-50 text-left">
                <th className="px-3 py-2">{'Item ID'}</th>
                <th className="px-3 py-2">{'行'}</th>
                <th className="px-3 py-2">{'报价厂牌'}</th>
                <th className="px-3 py-2">{'需求 canonical'}</th>
                <th className="px-3 py-2">{'操作'}</th>
              </tr>
            </thead>
            <tbody>
              {items.map((row) => (
                <tr
                  key={row.quote_item_id}
                  className="border-b border-slate-100"
                  data-testid={`quote-item-mfr-row-${row.quote_item_id}`}
                >
                  <td className="px-3 py-2 font-mono">{row.quote_item_id}</td>
                  <td className="px-3 py-2 font-mono">{row.line_no}</td>
                  <td className="px-3 py-2">{row.manufacturer}</td>
                  <td className="px-3 py-2 font-mono text-xs">{row.line_manufacturer_canonical_id}</td>
                  <td className="px-3 py-2">
                    <div className="flex min-w-[12rem] flex-col gap-2">
                      <label className="text-xs text-slate-600">
                        {'不通过原因（可选）'}
                        <input
                          type="text"
                          value={rejectReasonByItem[row.quote_item_id] ?? ''}
                          disabled={phaseLocked || busyId === row.quote_item_id}
                          onChange={(e) =>
                            setRejectReasonByItem((prev) => ({ ...prev, [row.quote_item_id]: e.target.value }))
                          }
                          className="mt-0.5 w-full rounded border border-slate-200 px-2 py-1 text-xs"
                          placeholder=""
                          autoComplete="off"
                        />
                      </label>
                      <div className="flex flex-wrap gap-2">
                        <button
                          type="button"
                          disabled={phaseLocked || busyId === row.quote_item_id}
                          onClick={() => void handleDecision(row, 'accept')}
                          className="rounded bg-[#2457c5] px-3 py-1 text-xs font-bold text-white disabled:bg-slate-300"
                        >
                          {'通过'}
                        </button>
                        <button
                          type="button"
                          disabled={phaseLocked || busyId === row.quote_item_id}
                          onClick={() => void handleDecision(row, 'reject')}
                          className="rounded border border-slate-300 px-3 py-1 text-xs font-bold text-slate-800 disabled:opacity-40"
                        >
                          {'不通过'}
                        </button>
                      </div>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  )
}
