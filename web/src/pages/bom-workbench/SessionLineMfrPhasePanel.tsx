import type { ManufacturerCanonicalRow, SessionLineMfrCandidate } from '../../api'
import { ManufacturerAliasReviewPanel, type PendingMfrRow } from './ManufacturerAliasReviewPanel'

function pendingRowsFromLineCandidates(items: SessionLineMfrCandidate[]): PendingMfrRow[] {
  return items.map((item) => ({
    kind: 'demand' as const,
    alias: item.mfr,
    recommendedCanonicalId: item.recommended_canonical_id,
    platformIds: [],
    lineIndexes: [item.line_no],
    demandHint: '',
  }))
}

export interface SessionLineMfrPhasePanelProps {
  pendingRows: PendingMfrRow[]
  canonicalRows: ManufacturerCanonicalRow[]
  aliasErr: string | null
  onApprove: (input: { alias: string; canonical_id: string; display_name: string }) => Promise<void>
  onApplyExistingAliases: () => Promise<void>
}

/** 阶段一：需求行厂牌（候选 + 标准厂牌下拉 + 应用已有别名）。 */
export function SessionLineMfrPhasePanel({
  pendingRows,
  canonicalRows,
  aliasErr,
  onApprove,
  onApplyExistingAliases,
}: SessionLineMfrPhasePanelProps) {
  return (
    <section className="space-y-4" data-testid="session-line-mfr-phase">
      <h3 className="text-lg font-bold text-slate-900">{'\u9700\u6c42\u884c\u5382\u724c'}</h3>
      <div className="rounded-lg border border-[#d7e0ed] bg-white p-4 md:max-w-md">
        <div className="text-sm font-bold text-slate-950">{'\u5382\u724c\u5f85\u6e05\u6d17\u884c\u6570'}</div>
        <div className="mt-4 text-3xl font-bold text-[#2457c5]">{pendingRows.length}</div>
      </div>
      {aliasErr && (
        <div className="rounded-lg border border-[#f0c77d] bg-[#fff7e8] px-4 py-3 text-sm text-amber-900">{aliasErr}</div>
      )}
      <ManufacturerAliasReviewPanel
        pendingRows={pendingRows}
        canonicalRows={canonicalRows}
        onApprove={onApprove}
        onApplyExisting={onApplyExistingAliases}
        phaseDescription={
          '\u4ec5\u5904\u7406\u9700\u6c42\u884c\uff08BOM\uff09\u4e0a\u586b\u5199\u7684\u5382\u724c\u5b57\u6bb5\uff1b\u5b8c\u6210\u540e\u65b9\u53ef\u5ba1\u6838\u62a5\u4ef7\u5382\u724c\u3002'
        }
      />
    </section>
  )
}

export { pendingRowsFromLineCandidates }
