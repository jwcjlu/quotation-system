import { useMemo, useState } from 'react'
import type { ManufacturerCanonicalRow } from '../../api'

import { pickBestCanonicalMatch } from './manufacturerAliasCanonicalPick'
import { SearchableCanonicalSelect } from './SearchableCanonicalSelect'

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
  /** 覆盖默认说明文案（阶段一为需求行厂牌场景）。 */
  phaseDescription?: string
}

const defaultPhaseDescription =
  '\u5904\u7406\u578b\u53f7\u548c\u5c01\u88c5\u5df2\u5bf9\u9f50\u4f46\u5382\u724c\u9700\u8981\u4eba\u5de5\u786e\u8ba4\u7684\u884c\u6216\u62a5\u4ef7\u3002'

export function ManufacturerAliasReviewPanel({
  pendingRows,
  canonicalRows,
  onApprove,
  onApplyExisting,
  phaseDescription = defaultPhaseDescription,
}: ManufacturerAliasReviewPanelProps) {
  const [selected, setSelected] = useState<Record<string, string>>({})
  const [manualAdd, setManualAdd] = useState<Record<string, { canonicalId: string; displayName: string }>>({})
  const [busyKey, setBusyKey] = useState<string | null>(null)
  const canonicalByID = useMemo(() => new Map(canonicalRows.map((row) => [row.canonical_id, row])), [canonicalRows])

  const defaultCanonicalByAlias = useMemo(() => {
    const m: Record<string, string> = {}
    for (const row of pendingRows) {
      m[row.alias] = pickBestCanonicalMatch(
        row.alias,
        row.demandHint,
        row.recommendedCanonicalId,
        canonicalRows,
      )
    }
    return m
  }, [pendingRows, canonicalRows])

  function effectiveCanonicalId(row: PendingMfrRow): string {
    if (Object.prototype.hasOwnProperty.call(selected, row.alias)) {
      return selected[row.alias] ?? ''
    }
    const rec = row.recommendedCanonicalId.trim()
    if (rec && canonicalByID.has(rec)) {
      return rec
    }
    return defaultCanonicalByAlias[row.alias] ?? ''
  }

  function resolveApprovePayload(row: PendingMfrRow) {
    const m = manualAdd[row.alias]
    const cid = m?.canonicalId?.trim() ?? ''
    const dn = m?.displayName?.trim() ?? ''
    if (cid && dn) {
      return { alias: row.alias, canonical_id: cid, display_name: dn }
    }
    const fromSelect = effectiveCanonicalId(row)
    const canonical = canonicalByID.get(fromSelect)
    if (!canonical) return null
    return { alias: row.alias, canonical_id: canonical.canonical_id, display_name: canonical.display_name }
  }

  async function approve(row: PendingMfrRow) {
    const payload = resolveApprovePayload(row)
    if (!payload) return
    setBusyKey(row.alias)
    try {
      await onApprove(payload)
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
          <p className="mt-1 text-sm text-slate-500">{phaseDescription}</p>
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
            const rec = row.recommendedCanonicalId
            const recInList = Boolean(rec && canonicalByID.has(rec))
            const selectValue = effectiveCanonicalId(row)
            const showRecHint = Boolean(rec && !recInList && !defaultCanonicalByAlias[row.alias])
            const canApprove = Boolean(resolveApprovePayload(row))
            const manual = manualAdd[row.alias] ?? { canonicalId: '', displayName: '' }
            const rowKey = `${row.alias}::${row.lineIndexes.join(',')}`
            return (
            <div key={rowKey} className="rounded-lg border border-[#f0c77d] bg-white/70 px-4 py-3 text-sm">
              <div className="flex flex-wrap items-center gap-3">
                <div className="min-w-[10rem] flex-1">
                  <div className="font-medium text-slate-800">{row.alias}</div>
                  <div className="mt-1 text-xs text-slate-500">
                    {row.kind === 'demand' ? '\u9700\u6c42\u5382\u724c' : '\u62a5\u4ef7\u5382\u724c'} {' | '}
                    {'\u884c'} {row.lineIndexes.join(', ')} {' | '} {row.demandHint || '-'}
                  </div>
                  <div className="mt-1 text-xs text-slate-500">
                    {'canonical_id: '}
                    {selectValue || rec || '-'}
                  </div>
                  {showRecHint ? (
                    <div className="mt-1 text-xs text-amber-800">
                      {'\u63a8\u8350 ID \u672a\u5728\u6807\u51c6\u5217\u8868\u4e2d\uff0c\u8bf7\u4ece\u4e0b\u62c9\u9009\u62e9\u6216\u586b\u5199\u4e0b\u65b9\u65b0\u5efa\u3002'}
                    </div>
                  ) : null}
                </div>
                <SearchableCanonicalSelect
                  value={selectValue}
                  onChange={(canonicalId) =>
                    setSelected((prev) => ({ ...prev, [row.alias]: canonicalId }))
                  }
                  options={canonicalRows}
                  placeholder={'\u9009\u62e9\u6807\u51c6\u5382\u724c'}
                  searchPlaceholder={'\u641c\u7d22\u663e\u793a\u540d\u6216 canonical_id\u2026'}
                  aria-label={'\u6807\u51c6\u5382\u724c: ' + row.alias}
                />
                <button
                  type="button"
                  disabled={!canApprove || busyKey === row.alias}
                  onClick={() => void approve(row)}
                  className="h-8 rounded-md bg-[#2457c5] px-4 text-sm font-bold text-white disabled:bg-slate-300"
                >
                  {'\u786e\u8ba4\u6e05\u6d17'}
                </button>
              </div>
              <div className="mt-3 flex flex-wrap items-end gap-2 border-t border-[#f0e6d4] pt-3">
                <span className="w-full text-xs text-slate-600 sm:w-auto">
                  {'\u82e5\u5217\u8868\u4e2d\u65e0\u5408\u9002 canonical_id\uff0c\u53ef\u624b\u52a8\u586b\u5199\uff1a'}
                </span>
                <label className="flex min-w-[10rem] flex-1 flex-col gap-0.5 text-xs text-slate-600">
                  <span>{'canonical_id'}</span>
                  <input
                    type="text"
                    value={manual.canonicalId}
                    onChange={(e) =>
                      setManualAdd((prev) => ({
                        ...prev,
                        [row.alias]: { ...manual, canonicalId: e.target.value },
                      }))
                    }
                    placeholder="MFR_XXX"
                    className="h-8 rounded-md border border-[#d7e0ed] bg-white px-2 font-mono text-sm"
                    autoComplete="off"
                  />
                </label>
                <label className="flex min-w-[10rem] flex-1 flex-col gap-0.5 text-xs text-slate-600">
                  <span>{'\u663e\u793a\u540d'}</span>
                  <input
                    type="text"
                    value={manual.displayName}
                    onChange={(e) =>
                      setManualAdd((prev) => ({
                        ...prev,
                        [row.alias]: { ...manual, displayName: e.target.value },
                      }))
                    }
                    placeholder={'\u4f8b\uff1a Espressif Systems'}
                    className="h-8 rounded-md border border-[#d7e0ed] bg-white px-2 text-sm"
                    autoComplete="off"
                  />
                </label>
              </div>
            </div>
          )})}
        </div>
      )}
    </section>
  )
}
