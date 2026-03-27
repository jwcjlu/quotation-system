import { useCallback, useEffect, useState } from 'react'
import {
  PLATFORM_IDS,
  createSessionLine,
  deleteSessionLine,
  exportSessionFile,
  getBOMLines,
  getSession,
  getSessionSearchTaskCoverage,
  patchSession,
  patchSessionLine,
  putPlatforms,
  retrySearchTasks,
  type BOMLineRow,
  type GetSessionSearchTaskCoverageReply,
} from '../api'
import { validateSessionHeaderFields, type ReadinessMode } from '../utils/sessionFields'

interface SourcingSessionPageProps {
  sessionId: string
  onOpenMatch: () => void
  /** 嵌入弹框时去掉与外层重复的页面大标题 */
  embedded?: boolean
}

type LineDraft = { mpn: string; mfr: string; package: string; qty: string }

export function SourcingSessionPage({ sessionId, onOpenMatch, embedded }: SourcingSessionPageProps) {
  const [sessionTitle, setSessionTitle] = useState('')
  const [customerName, setCustomerName] = useState('')
  const [contactPhone, setContactPhone] = useState('')
  const [contactEmail, setContactEmail] = useState('')
  const [contactExtra, setContactExtra] = useState('')
  const [readinessMode, setReadinessMode] = useState<ReadinessMode>('lenient')
  const [revision, setRevision] = useState(1)
  const [selectedPlatforms, setSelectedPlatforms] = useState<string[]>([...PLATFORM_IDS])
  const [lines, setLines] = useState<BOMLineRow[]>([])
  const [searchCoverage, setSearchCoverage] = useState<GetSessionSearchTaskCoverageReply | null>(null)
  const [loading, setLoading] = useState(true)
  const [err, setErr] = useState<string | null>(null)
  const [platformErr, setPlatformErr] = useState<string | null>(null)
  const [savingPlatforms, setSavingPlatforms] = useState(false)
  const [exporting, setExporting] = useState(false)
  const [savingHeader, setSavingHeader] = useState(false)
  const [lineMsg, setLineMsg] = useState<string | null>(null)

  const [newLine, setNewLine] = useState<LineDraft>({ mpn: '', mfr: '', package: '', qty: '' })
  const [addingLine, setAddingLine] = useState(false)

  const [editingId, setEditingId] = useState<string | null>(null)
  const [editDraft, setEditDraft] = useState<LineDraft>({ mpn: '', mfr: '', package: '', qty: '' })

  const loadSession = useCallback(async () => {
    try {
      const s = await getSession(sessionId)
      setSessionTitle(s.title || sessionId.slice(0, 8))
      setCustomerName(s.customer_name || '')
      setContactPhone(s.contact_phone || '')
      setContactEmail(s.contact_email || '')
      setContactExtra(s.contact_extra || '')
      const rm = (s.readiness_mode || 'lenient').toLowerCase()
      setReadinessMode(rm === 'strict' ? 'strict' : 'lenient')
      setRevision(s.selection_revision)
      if (s.platform_ids?.length) setSelectedPlatforms(s.platform_ids)
    } catch (e) {
      setErr(e instanceof Error ? e.message : '加载会话失败')
    }
  }, [sessionId])

  const loadLines = useCallback(async () => {
    try {
      const { lines: rows } = await getBOMLines(sessionId)
      setLines(rows)
      try {
        const c = await getSessionSearchTaskCoverage(sessionId)
        setSearchCoverage(c)
      } catch {
        setSearchCoverage(null)
      }
    } catch {
      setLines([])
      setSearchCoverage(null)
    }
  }, [sessionId])

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    ;(async () => {
      await loadSession()
      await loadLines()
      if (!cancelled) setLoading(false)
    })()
    return () => {
      cancelled = true
    }
  }, [loadSession, loadLines])

  const togglePlatform = (id: string) => {
    setSelectedPlatforms((prev) =>
      prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]
    )
  }

  const handleSaveHeader = async () => {
    const fieldErr = validateSessionHeaderFields({
      title: sessionTitle,
      customerName,
      contactPhone,
      contactEmail,
      contactExtra,
    })
    if (fieldErr) {
      setLineMsg(fieldErr)
      return
    }
    setSavingHeader(true)
    setLineMsg(null)
    try {
      await patchSession(sessionId, {
        title: sessionTitle,
        customer_name: customerName,
        contact_phone: contactPhone,
        contact_email: contactEmail,
        contact_extra: contactExtra,
        readiness_mode: readinessMode,
      })
      await loadSession()
      setLineMsg('单据信息已保存')
    } catch (e) {
      setLineMsg(e instanceof Error ? e.message : '保存失败')
    } finally {
      setSavingHeader(false)
    }
  }

  const handleSavePlatforms = async () => {
    if (selectedPlatforms.length === 0) {
      setPlatformErr('请至少选择一个平台')
      return
    }
    setPlatformErr(null)
    setSavingPlatforms(true)
    try {
      const r = await putPlatforms(sessionId, selectedPlatforms, revision)
      setRevision(r.selection_revision)
      await loadSession()
    } catch (e) {
      setPlatformErr(e instanceof Error ? e.message : '保存平台失败')
    } finally {
      setSavingPlatforms(false)
    }
  }

  const handleExport = async (format: 'xlsx' | 'csv') => {
    setExporting(true)
    try {
      const { blob, filename } = await exportSessionFile(sessionId, format)
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = filename
      a.click()
      URL.revokeObjectURL(url)
    } catch (e) {
      setErr(e instanceof Error ? e.message : '导出失败')
    } finally {
      setExporting(false)
    }
  }

  const handleRetryFirstGap = async () => {
    const row = lines.find((l) => l.platform_gaps?.length)
    const g = row?.platform_gaps?.[0]
    if (!row || !g) return
    try {
      await retrySearchTasks(sessionId, [{ mpn: row.mpn, platform_id: g.platform_id }])
      await loadLines()
    } catch (e) {
      setErr(e instanceof Error ? e.message : '重试失败')
    }
  }

  const handleAddLine = async () => {
    const mpn = newLine.mpn.trim()
    if (!mpn) {
      setLineMsg('请填写型号 MPN')
      return
    }
    setAddingLine(true)
    setLineMsg(null)
    try {
      let qty: number | undefined
      if (newLine.qty.trim() !== '') {
        const n = Number(newLine.qty)
        if (Number.isNaN(n)) {
          setLineMsg('数量须为数字')
          setAddingLine(false)
          return
        }
        qty = n
      }
      await createSessionLine(sessionId, {
        mpn,
        mfr: newLine.mfr.trim(),
        package: newLine.package.trim(),
        qty,
      })
      setNewLine({ mpn: '', mfr: '', package: '', qty: '' })
      await loadLines()
      setLineMsg('已添加一行')
    } catch (e) {
      setLineMsg(e instanceof Error ? e.message : '添加失败')
    } finally {
      setAddingLine(false)
    }
  }

  const startEdit = (row: BOMLineRow) => {
    setEditingId(row.line_id)
    setEditDraft({
      mpn: row.mpn,
      mfr: row.mfr,
      package: row.package,
      qty: String(row.qty ?? ''),
    })
    setLineMsg(null)
  }

  const cancelEdit = () => {
    setEditingId(null)
  }

  const saveEdit = async () => {
    if (!editingId) return
    const mpn = editDraft.mpn.trim()
    if (!mpn) {
      setLineMsg('型号不能为空')
      return
    }
    setLineMsg(null)
    try {
      let qty: number | undefined
      if (editDraft.qty.trim() !== '') {
        const n = Number(editDraft.qty)
        if (Number.isNaN(n)) {
          setLineMsg('数量须为数字')
          return
        }
        qty = n
      }
      await patchSessionLine(sessionId, editingId, {
        mpn,
        mfr: editDraft.mfr,
        package: editDraft.package,
        qty,
      })
      setEditingId(null)
      await loadLines()
      setLineMsg('行已更新')
    } catch (e) {
      setLineMsg(e instanceof Error ? e.message : '保存行失败')
    }
  }

  const handleDeleteLine = async (lineId: string) => {
    if (!window.confirm('确定删除该行？')) return
    setLineMsg(null)
    try {
      await deleteSessionLine(sessionId, lineId)
      if (editingId === lineId) setEditingId(null)
      await loadLines()
      setLineMsg('已删除')
    } catch (e) {
      setLineMsg(e instanceof Error ? e.message : '删除失败')
    }
  }

  if (loading) {
    return (
      <div className="flex justify-center py-20 text-slate-500">加载会话...</div>
    )
  }

  return (
    <div className={embedded ? 'space-y-6' : 'space-y-8'}>
      <div>
        {!embedded && <h2 className="text-2xl font-bold text-slate-800">货源会话</h2>}
        <p className={`text-slate-600 ${embedded ? '' : 'mt-1'}`}>
          <span className="font-medium text-slate-800">{sessionTitle}</span>
          {' · '}
          <span className="font-mono text-sm">{sessionId}</span>
        </p>
        <div className="mt-3 flex flex-wrap gap-2 items-center">
          <button
            type="button"
            disabled={exporting}
            onClick={() => void handleExport('xlsx')}
            className="rounded-lg border border-slate-300 bg-white px-3 py-1.5 text-sm hover:bg-slate-50 disabled:opacity-50"
          >
            {exporting ? '导出中…' : '导出 Excel'}
          </button>
          <button
            type="button"
            disabled={exporting}
            onClick={() => void handleExport('csv')}
            className="rounded-lg border border-slate-300 bg-white px-3 py-1.5 text-sm hover:bg-slate-50 disabled:opacity-50"
          >
            导出 CSV
          </button>
          <button
            type="button"
            onClick={() => void loadLines()}
            className="rounded-lg border border-slate-300 bg-white px-3 py-1.5 text-sm hover:bg-slate-50"
          >
            刷新行列表
          </button>
          <button
            type="button"
            onClick={onOpenMatch}
            className="rounded-lg bg-blue-600 px-3 py-1.5 text-sm text-white hover:bg-blue-700"
          >
            打开经典配单页
          </button>
          <span className="text-xs text-slate-500 self-center">GET /bom-sessions/.../export</span>
        </div>
      </div>

      {err && (
        <div className="rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-900">
          <strong className="block mb-1">接口提示</strong>
          {err}
        </div>
      )}

      <section className="rounded-xl border border-slate-200 bg-white p-6 shadow-sm">
        <h3 className="font-semibold text-slate-800 mb-3">单据信息（PATCH /bom-sessions）</h3>
        <div className="grid gap-3 sm:grid-cols-2">
          <div className="sm:col-span-2">
            <label className="block text-xs text-slate-500 mb-1">标题</label>
            <input
              type="text"
              value={sessionTitle}
              onChange={(e) => setSessionTitle(e.target.value)}
              className="w-full border border-slate-300 rounded-lg px-3 py-2 text-sm"
            />
          </div>
          <div>
            <label className="block text-xs text-slate-500 mb-1">客户名称</label>
            <input
              type="text"
              value={customerName}
              onChange={(e) => setCustomerName(e.target.value)}
              className="w-full border border-slate-300 rounded-lg px-3 py-2 text-sm"
            />
          </div>
          <div>
            <label className="block text-xs text-slate-500 mb-1">联系电话</label>
            <input
              type="text"
              value={contactPhone}
              onChange={(e) => setContactPhone(e.target.value)}
              className="w-full border border-slate-300 rounded-lg px-3 py-2 text-sm"
            />
          </div>
          <div>
            <label className="block text-xs text-slate-500 mb-1">邮箱</label>
            <input
              type="email"
              value={contactEmail}
              onChange={(e) => setContactEmail(e.target.value)}
              className="w-full border border-slate-300 rounded-lg px-3 py-2 text-sm"
            />
          </div>
          <div>
            <label className="block text-xs text-slate-500 mb-1">备注</label>
            <input
              type="text"
              value={contactExtra}
              onChange={(e) => setContactExtra(e.target.value)}
              className="w-full border border-slate-300 rounded-lg px-3 py-2 text-sm"
            />
          </div>
          <div className="sm:col-span-2">
            <label className="block text-xs text-slate-500 mb-1">数据就绪策略（readiness_mode）</label>
            <select
              value={readinessMode}
              onChange={(e) => setReadinessMode(e.target.value as ReadinessMode)}
              className="w-full max-w-lg border border-slate-300 rounded-lg px-3 py-2 text-sm"
            >
              <option value="lenient">宽松：各平台终态即可</option>
              <option value="strict">严格：每行至少一平台 succeeded</option>
            </select>
          </div>
        </div>
        <div className="mt-4 flex items-center gap-3">
          <button
            type="button"
            disabled={savingHeader}
            onClick={() => void handleSaveHeader()}
            className="rounded-lg bg-slate-800 px-4 py-2 text-sm text-white hover:bg-slate-900 disabled:opacity-50"
          >
            {savingHeader ? '保存中…' : '保存单据信息'}
          </button>
          {lineMsg && <span className="text-sm text-slate-600">{lineMsg}</span>}
        </div>
      </section>

      <section className="rounded-xl border border-slate-200 bg-white p-6 shadow-sm">
        <h3 className="font-semibold text-slate-800 mb-3">勾选平台（PUT /platforms）</h3>
        <p className="text-sm text-slate-600 mb-3">与接口清单 platform_id 枚举一致，全量替换。</p>
        <div className="flex flex-wrap gap-3 mb-4">
          {PLATFORM_IDS.map((id) => (
            <label key={id} className="flex items-center gap-2 cursor-pointer text-sm">
              <input
                type="checkbox"
                checked={selectedPlatforms.includes(id)}
                onChange={() => togglePlatform(id)}
              />
              {id}
            </label>
          ))}
        </div>
        {platformErr && <p className="text-red-600 text-sm mb-2">{platformErr}</p>}
        <button
          type="button"
          disabled={savingPlatforms}
          onClick={() => void handleSavePlatforms()}
          className="rounded-lg bg-slate-800 px-4 py-2 text-sm text-white hover:bg-slate-900 disabled:opacity-50"
        >
          {savingPlatforms ? '保存中...' : '保存平台勾选'}
        </button>
      </section>

      <section className="rounded-xl border border-slate-200 bg-white p-6 shadow-sm">
        <div className="flex flex-wrap items-center justify-between gap-3 mb-3">
          <h3 className="font-semibold text-slate-800">BOM 行（GET /lines · 增删改）</h3>
          <button
            type="button"
            onClick={() => void handleRetryFirstGap()}
            className="text-sm text-blue-600 hover:underline"
          >
            重试第一条缺口（示例）
          </button>
        </div>

        <div className="mb-6 rounded-lg border border-dashed border-slate-300 p-4 bg-slate-50/80">
          <p className="text-sm font-medium text-slate-700 mb-2">添加一行（POST /lines）</p>
          <div className="flex flex-wrap gap-2 items-end">
            <input
              placeholder="MPN *"
              value={newLine.mpn}
              onChange={(e) => setNewLine((n) => ({ ...n, mpn: e.target.value }))}
              className="border border-slate-300 rounded px-2 py-1.5 text-sm w-40 font-mono"
            />
            <input
              placeholder="厂牌"
              value={newLine.mfr}
              onChange={(e) => setNewLine((n) => ({ ...n, mfr: e.target.value }))}
              className="border border-slate-300 rounded px-2 py-1.5 text-sm w-28"
            />
            <input
              placeholder="封装"
              value={newLine.package}
              onChange={(e) => setNewLine((n) => ({ ...n, package: e.target.value }))}
              className="border border-slate-300 rounded px-2 py-1.5 text-sm w-24"
            />
            <input
              placeholder="数量"
              value={newLine.qty}
              onChange={(e) => setNewLine((n) => ({ ...n, qty: e.target.value }))}
              className="border border-slate-300 rounded px-2 py-1.5 text-sm w-20"
            />
            <button
              type="button"
              disabled={addingLine}
              onClick={() => void handleAddLine()}
              className="rounded-lg bg-emerald-600 text-white px-3 py-1.5 text-sm hover:bg-emerald-700 disabled:opacity-50"
            >
              {addingLine ? '添加中…' : '添加'}
            </button>
          </div>
        </div>

        {searchCoverage && !searchCoverage.consistent && (
          <div className="mb-3 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-950">
            搜索任务未完全对齐：缺 {searchCoverage.missing_tasks.length} 条应有任务（期望 {searchCoverage.expected_task_count} / 库内{' '}
            {searchCoverage.existing_task_count}）。
            {searchCoverage.orphan_task_count > 0 && (
              <span> 另有 {searchCoverage.orphan_task_count} 条历史任务与当前行/平台不一致（仅统计，不自动删除）。</span>
            )}
          </div>
        )}

        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-slate-200 bg-slate-50 text-left">
                <th className="py-2 px-2">行</th>
                <th className="py-2 px-2">MPN</th>
                <th className="py-2 px-2">厂牌</th>
                <th className="py-2 px-2">封装</th>
                <th className="py-2 px-2">数量</th>
                <th className="py-2 px-2">match_status</th>
                <th className="py-2 px-2">platform（四态 / phase）</th>
                <th className="py-2 px-2 w-40">操作</th>
              </tr>
            </thead>
            <tbody>
              {lines.length === 0 ? (
                <tr>
                  <td colSpan={8} className="py-8 text-center text-slate-500">
                    暂无行数据（可上方添加，或上传 Excel）
                  </td>
                </tr>
              ) : (
                lines.map((row) => (
                  <tr key={row.line_id || String(row.line_no)} className="border-b border-slate-100 align-top">
                    <td className="py-2 px-2">{row.line_no}</td>
                    {editingId === row.line_id ? (
                      <>
                        <td className="py-2 px-2">
                          <input
                            value={editDraft.mpn}
                            onChange={(e) => setEditDraft((d) => ({ ...d, mpn: e.target.value }))}
                            className="w-full border border-slate-300 rounded px-1 py-0.5 font-mono text-xs"
                          />
                        </td>
                        <td className="py-2 px-2">
                          <input
                            value={editDraft.mfr}
                            onChange={(e) => setEditDraft((d) => ({ ...d, mfr: e.target.value }))}
                            className="w-full border border-slate-300 rounded px-1 py-0.5 text-xs"
                          />
                        </td>
                        <td className="py-2 px-2">
                          <input
                            value={editDraft.package}
                            onChange={(e) => setEditDraft((d) => ({ ...d, package: e.target.value }))}
                            className="w-full border border-slate-300 rounded px-1 py-0.5 text-xs"
                          />
                        </td>
                        <td className="py-2 px-2">
                          <input
                            value={editDraft.qty}
                            onChange={(e) => setEditDraft((d) => ({ ...d, qty: e.target.value }))}
                            className="w-20 border border-slate-300 rounded px-1 py-0.5 text-xs"
                          />
                        </td>
                      </>
                    ) : (
                      <>
                        <td className="py-2 px-2 font-mono">{row.mpn}</td>
                        <td className="py-2 px-2">{row.mfr || '—'}</td>
                        <td className="py-2 px-2">{row.package || '—'}</td>
                        <td className="py-2 px-2">{row.qty}</td>
                      </>
                    )}
                    <td className="py-2 px-2">{row.match_status || '—'}</td>
                    <td className="py-2 px-2 max-w-xs">
                      {(row.platform_gaps || []).length === 0 ? (
                        '—'
                      ) : (
                        <ul className="list-disc pl-4 space-y-1 text-xs">
                          {row.platform_gaps.map((g, i) => (
                            <li key={i}>
                              <span className="font-mono">{g.platform_id}</span>
                              {g.search_ui_state && (
                                <span className="ml-1 text-violet-700 font-medium">[{g.search_ui_state}]</span>
                              )}{' '}
                              {g.phase}{' '}
                              {g.reason_code && <span className="text-slate-500">({g.reason_code})</span>}{' '}
                              {g.message}
                            </li>
                          ))}
                        </ul>
                      )}
                    </td>
                    <td className="py-2 px-2 whitespace-nowrap">
                      {editingId === row.line_id ? (
                        <span className="flex flex-wrap gap-1">
                          <button
                            type="button"
                            onClick={() => void saveEdit()}
                            className="text-emerald-600 text-xs hover:underline"
                          >
                            保存
                          </button>
                          <button type="button" onClick={cancelEdit} className="text-slate-500 text-xs hover:underline">
                            取消
                          </button>
                        </span>
                      ) : (
                        <span className="flex flex-wrap gap-2">
                          <button
                            type="button"
                            onClick={() => startEdit(row)}
                            className="text-blue-600 text-xs hover:underline"
                          >
                            编辑
                          </button>
                          <button
                            type="button"
                            onClick={() => void handleDeleteLine(row.line_id)}
                            className="text-red-600 text-xs hover:underline"
                          >
                            删除
                          </button>
                        </span>
                      )}
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </section>
    </div>
  )
}
