import { useMemo, useState } from 'react'
import type { ManufacturerCanonicalRow } from '../../api'

export type PendingMfrRow = {
  kind: 'demand' | 'quote'
  alias: string
  recommendedCanonicalId: string
  platformIds: string[]
  lineIndexes: number[]
  demandHint: string
}

interface ManufacturerAliasReviewPanelProps {
  pendingRows: PendingMfrRow[]
  canonicalRows: ManufacturerCanonicalRow[]
  onApprove: (input: { alias: string; canonical_id: string; display_name: string }) => Promise<void>
  onApplyExisting: () => Promise<void>
}

export function ManufacturerAliasReviewPanel({
  pendingRows,
  canonicalRows,
  onApprove,
  onApplyExisting,
}: ManufacturerAliasReviewPanelProps) {
  const [selected, setSelected] = useState<Record<string, string>>({})
  const [busyKey, setBusyKey] = useState<string | null>(null)
  const canonicalByID = useMemo(() => new Map(canonicalRows.map((row) => [row.canonical_id, row])), [canonicalRows])

  async function approve(row: PendingMfrRow) {
    const canonicalId = selected[row.alias] || row.recommendedCanonicalId
    const canonical = canonicalByID.get(canonicalId)
    if (!canonical) return
    setBusyKey(row.alias)
    try {
      await onApprove({
        alias: row.alias,
        canonical_id: canonical.canonical_id,
        display_name: canonical.display_name,
      })
    } finally {
      setBusyKey(null)
    }
  }

  return (
    <section
      className="rounded-lg border border-[#f0c77d] bg-[#fff7e8] p-4"
      data-testid="manufacturer-alias-review-panel"
    >
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h3 className="font-bold text-slate-950">{'\u5382\u724c\u522b\u540d\u5ba1\u6838'}</h3>
          <p className="mt-1 text-sm text-slate-500">
            {'\u5904\u7406\u578b\u53f7\u548c\u5c01\u88c5\u5df2\u5bf9\u9f50\u4f46\u5382\u724c\u9700\u8981\u4eba\u5de5\u786e\u8ba4\u7684\u62a5\u4ef7\u3002'}
          </p>
        </div>
        <div className="text-xs text-slate-500">
          {pendingRows.length} {'\u4e2a\u5f85\u5ba1'} {' | '} {canonicalRows.length} {'\u4e2a\u6807\u51c6\u5382\u724c'}
        </div>
        <button
          type="button"
          onClick={() => void onApplyExisting()}
          className="h-8 rounded-md bg-[#1f2a3d] px-4 text-sm font-bold text-white"
        >
          {'\u5e94\u7528\u5df2\u6709\u522b\u540d'}
        </button>
      </div>

      {pendingRows.length === 0 ? (
        <div className="mt-4 rounded-lg border border-[#f0c77d] bg-white/70 px-4 py-3 text-sm text-slate-500">
          {'\u6682\u65e0\u9700\u8981\u5ba1\u6838\u7684\u5382\u724c\u522b\u540d'}
        </div>
      ) : (
        <div className="mt-4 space-y-2">
          {pendingRows.map((row) => {
            const value = selected[row.alias] || row.recommendedCanonicalId
            const canApprove = Boolean(value && canonicalByID.has(value))
            return (
            <div key={row.alias} className="rounded-lg border border-[#f0c77d] bg-white/70 px-4 py-3 text-sm">
              <div className="flex flex-wrap items-center gap-3">
                <div className="min-w-[10rem] flex-1">
                  <div className="font-medium text-slate-800">{row.alias}</div>
                  <div className="mt-1 text-xs text-slate-500">
                    {row.kind === 'demand' ? '\u9700\u6c42\u5382\u724c' : '\u62a5\u4ef7\u5382\u724c'} {' | '}
                    {'\u884c'} {row.lineIndexes.join(', ')} {' | '} {row.demandHint || '-'}
                  </div>
                  <div className="mt-1 text-xs text-slate-500">
                    {'canonical_id: '} {value || row.recommendedCanonicalId || '-'}
                  </div>
                </div>
                <select
                  value={value}
                  onChange={(event) => setSelected((prev) => ({ ...prev, [row.alias]: event.target.value }))}
                  className="h-8 min-w-[14rem] rounded-md border border-[#d7e0ed] bg-white px-3 text-sm"
                >
                  <option value="">{'\u9009\u62e9\u6807\u51c6\u5382\u724c'}</option>
                  {canonicalRows.map((canonical) => (
                    <option key={canonical.canonical_id} value={canonical.canonical_id}>
                      {canonical.display_name} ({canonical.canonical_id})
                    </option>
                  ))}
                </select>
                <button
                  type="button"
                  disabled={!canApprove || busyKey === row.alias}
                  onClick={() => void approve(row)}
                  className="h-8 rounded-md bg-[#2457c5] px-4 text-sm font-bold text-white disabled:bg-slate-300"
                >
                  {'\u786e\u8ba4\u6e05\u6d17'}
                </button>
              </div>
            </div>
          )})}
        </div>
      )}
    </section>
  )
}
