import { useState } from 'react'
import type { QuoteItemMfrReviewItem } from '../../api'
import { submitQuoteItemMfrReview } from '../../api'

export interface QuoteItemMfrReviewSectionProps {
  sessionId: string
  gateOpen: boolean
  items: QuoteItemMfrReviewItem[]
  quoteErr: string | null
  onAfterSubmit: () => Promise<void>
  onSubmitError?: (message: string) => void
  onSubmitStart?: () => void
}

/** 阶段二：报价厂牌确认（依赖 gate_open；无下拉，仅通过/不通过）。 */
export function QuoteItemMfrReviewSection({
  sessionId,
  gateOpen,
  items,
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
      await submitQuoteItemMfrReview(sessionId, {
        quote_item_id: item.quote_item_id,
        decision,
        ...(reason ? { reason } : {}),
      })
      await onAfterSubmit()
    } catch (e) {
      const msg = e instanceof Error ? e.message : '\u63d0\u4ea4\u5931\u8d25'
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
      <h3 className="text-lg font-bold text-slate-900">{'\u62a5\u4ef7\u5382\u724c\u786e\u8ba4'}</h3>
      {phaseLocked && (
        <p className="text-sm text-slate-600">
          {'\u9700\u5148\u5b8c\u6210\u4e0a\u65b9\u9700\u6c42\u884c\u5382\u724c\uff08\u6240\u6709\u975e\u7a7a\u5382\u724c\u884c\u586b\u9f50 canonical\uff09\u540e\u624d\u53ef\u5ba1\u62a5\u4ef7\u3002'}
        </p>
      )}
      {quoteErr && (
        <div className="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-900">{quoteErr}</div>
      )}
      {gateOpen && items.length === 0 && (
        <p className="text-sm text-slate-500">{'\u6682\u65e0\u5f85\u786e\u8ba4\u62a5\u4ef7\u5382\u724c\u3002'}</p>
      )}
      {gateOpen && items.length > 0 && (
        <div
          className="overflow-x-auto rounded-lg border border-[#d7e0ed] bg-white"
          data-testid="quote-item-mfr-review-table-wrap"
        >
          <table className="min-w-full text-sm">
            <thead>
              <tr className="border-b border-slate-200 bg-slate-50 text-left">
                <th className="px-3 py-2">{'\u884c'}</th>
                <th className="px-3 py-2">{'\u5e73\u53f0'}</th>
                <th className="px-3 py-2">{'\u62a5\u4ef7\u5382\u724c'}</th>
                <th className="px-3 py-2">{'\u9700\u6c42 canonical'}</th>
                <th className="px-3 py-2">{'\u64cd\u4f5c'}</th>
              </tr>
            </thead>
            <tbody>
              {items.map((row) => (
                <tr key={row.quote_item_id} className="border-b border-slate-100">
                  <td className="px-3 py-2 font-mono">{row.line_no}</td>
                  <td className="px-3 py-2 font-mono text-xs">{row.platform_id || '—'}</td>
                  <td className="px-3 py-2">{row.manufacturer}</td>
                  <td className="px-3 py-2 font-mono text-xs">{row.line_manufacturer_canonical_id}</td>
                  <td className="px-3 py-2">
                    <div className="flex min-w-[12rem] flex-col gap-2">
                      <label className="text-xs text-slate-600">
                        {'\u4e0d\u901a\u8fc7\u539f\u56e0\uff08\u53ef\u9009\uff09'}
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
                          {'\u901a\u8fc7'}
                        </button>
                        <button
                          type="button"
                          disabled={phaseLocked || busyId === row.quote_item_id}
                          onClick={() => void handleDecision(row, 'reject')}
                          className="rounded border border-slate-300 px-3 py-1 text-xs font-bold text-slate-800 disabled:opacity-40"
                        >
                          {'\u4e0d\u901a\u8fc7'}
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
