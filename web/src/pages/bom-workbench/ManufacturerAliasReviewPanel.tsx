import type { ManufacturerCanonicalRow } from '../../api'

export type PendingMfrRow = {
  alias: string
  lineIndexes: number[]
  demandHint: string
}

interface ManufacturerAliasReviewPanelProps {
  pendingRows: PendingMfrRow[]
  canonicalRows: ManufacturerCanonicalRow[]
  onApproved: (alias: string) => void
  onManualSuccess: () => void
}

export function ManufacturerAliasReviewPanel({
  pendingRows,
  canonicalRows,
  onApproved: _onApproved,
  onManualSuccess: _onManualSuccess,
}: ManufacturerAliasReviewPanelProps) {
  return (
    <section
      className="rounded-xl border border-slate-200 bg-white p-6 shadow-sm"
      data-testid="manufacturer-alias-review-panel"
    >
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h3 className="font-semibold text-slate-800">{'\u5382\u724c\u522b\u540d\u5ba1\u6838'}</h3>
          <p className="mt-1 text-sm text-slate-500">
            {'\u5904\u7406\u578b\u53f7\u548c\u5c01\u88c5\u5df2\u5bf9\u9f50\u4f46\u5382\u724c\u9700\u8981\u4eba\u5de5\u786e\u8ba4\u7684\u62a5\u4ef7\u3002'}
          </p>
        </div>
        <div className="text-xs text-slate-500">
          {pendingRows.length} {'\u4e2a\u5f85\u5ba1'} {' | '} {canonicalRows.length} {'\u4e2a\u6807\u51c6\u5382\u724c'}
        </div>
      </div>

      {pendingRows.length === 0 ? (
        <div className="mt-4 rounded-lg border border-slate-200 bg-slate-50 px-4 py-3 text-sm text-slate-500">
          {'\u6682\u65e0\u9700\u8981\u5ba1\u6838\u7684\u5382\u724c\u522b\u540d'}
        </div>
      ) : (
        <div className="mt-4 space-y-2">
          {pendingRows.map((row) => (
            <div key={row.alias} className="rounded-lg border border-slate-200 px-4 py-3 text-sm">
              <div className="font-medium text-slate-800">{row.alias}</div>
              <div className="mt-1 text-xs text-slate-500">
                {'\u884c'} {row.lineIndexes.join(', ')} {' | '} {row.demandHint || '-'}
              </div>
            </div>
          ))}
        </div>
      )}
    </section>
  )
}
