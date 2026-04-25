import { useCallback, useEffect, useRef, useState } from 'react'
import {
  PLATFORM_IDS,
  createSessionLine,
  deleteSessionLine,
  exportSessionFile,
  getBOMLines,
  getSession,
  getSessionSearchTaskCoverage,
  listLineGaps,
  listMatchRuns,
  listSessionSearchTasks,
  patchSession,
  patchSessionLine,
  putPlatforms,
  retrySearchTasks,
  resolveLineGapManualQuote,
  saveMatchRun,
  selectLineGapSubstitute,
  type BOMLineGap,
  type BOMLineRow,
  type GetSessionSearchTaskCoverageReply,
  type ListSessionSearchTasksReply,
  type MatchRunListItem,
  type SessionSearchTaskRow,
} from '../api'
import { SessionImportStatusCard } from './sourcing-session/SessionImportStatusCard'
import { SearchTaskStatusPanel } from './sourcing-session/SearchTaskStatusPanel'

const SESSION_MATCH_READY = 'data_ready'
const BLOCKING_AVAILABILITY_STATUSES = new Set([
  'no_data',
  'collection_unavailable',
  'no_match_after_filter',
])

function normalizeAvailabilityStatus(status?: string) {
  return (status || '').trim()
}

function availabilityStatusLabel(status?: string) {
  switch (normalizeAvailabilityStatus(status)) {
    case 'ready':
      return '可配单'
    case 'collecting':
      return '采集中'
    case 'no_data':
      return '无数据'
    case 'collection_unavailable':
      return '采集不可用'
    case 'no_match_after_filter':
      return '筛选后无匹配'
    default:
      return '待判断'
  }
}

function availabilityStatusClass(status?: string) {
  switch (normalizeAvailabilityStatus(status)) {
    case 'ready':
      return 'border-emerald-200 bg-emerald-50 text-emerald-700'
    case 'collecting':
      return 'border-sky-200 bg-sky-50 text-sky-700'
    case 'no_data':
    case 'collection_unavailable':
    case 'no_match_after_filter':
      return 'border-amber-200 bg-amber-50 text-amber-800'
    default:
      return 'border-slate-200 bg-slate-50 text-slate-600'
  }
}

interface SourcingSessionPageProps {
  sessionId: string
  embedded?: boolean
  onEnterMatch?: () => void
}

type LineDraft = { mpn: string; mfr: string; package: string; qty: string }
type ManualQuoteDraft = {
  model: string
  manufacturer: string
  package: string
  stock: string
  price_tiers: string
  note: string
}
type SubstituteDraft = { substitute_mpn: string; reason: string }

const GAP_STATUSES = ['open', 'manual_quote_added', 'substitute_selected']

export function SourcingSessionPage({ sessionId, embedded, onEnterMatch }: SourcingSessionPageProps) {
  const [sessionTitle, setSessionTitle] = useState('')
  const [customerName, setCustomerName] = useState('')
  const [contactPhone, setContactPhone] = useState('')
  const [contactEmail, setContactEmail] = useState('')
  const [contactExtra, setContactExtra] = useState('')
  const [revision, setRevision] = useState(1)
  const [selectedPlatforms, setSelectedPlatforms] = useState<string[]>([...PLATFORM_IDS])
  const [lines, setLines] = useState<BOMLineRow[]>([])
  const [lineGaps, setLineGaps] = useState<BOMLineGap[]>([])
  const [matchRuns, setMatchRuns] = useState<MatchRunListItem[]>([])
  const [searchCoverage, setSearchCoverage] = useState<GetSessionSearchTaskCoverageReply | null>(null)
  const [searchTasks, setSearchTasks] = useState<ListSessionSearchTasksReply | null>(null)
  const [searchTasksLoading, setSearchTasksLoading] = useState(false)
  const [retryingSearchTasks, setRetryingSearchTasks] = useState(false)
  const [loading, setLoading] = useState(true)
  const [err, setErr] = useState<string | null>(null)
  const [platformErr, setPlatformErr] = useState<string | null>(null)
  const [savingPlatforms, setSavingPlatforms] = useState(false)
  const [exporting, setExporting] = useState(false)
  const [savingHeader, setSavingHeader] = useState(false)
  const [savingRun, setSavingRun] = useState(false)
  const [gapActionId, setGapActionId] = useState<string | null>(null)
  const [lineMsg, setLineMsg] = useState<string | null>(null)
  const [runMsg, setRunMsg] = useState<string | null>(null)
  const [sessionStatus, setSessionStatus] = useState('')
  const [importStatus, setImportStatus] = useState('')
  const [importProgress, setImportProgress] = useState(0)
  const [importStage, setImportStage] = useState('')
  const [importMessage, setImportMessage] = useState('')
  const [importErrorCode, setImportErrorCode] = useState('')
  const [importError, setImportError] = useState('')
  const [importUpdatedAt, setImportUpdatedAt] = useState('')

  const [newLine, setNewLine] = useState<LineDraft>({ mpn: '', mfr: '', package: '', qty: '' })
  const [addingLine, setAddingLine] = useState(false)
  const [manualQuote, setManualQuote] = useState<ManualQuoteDraft>({
    model: '',
    manufacturer: '',
    package: '',
    stock: '',
    price_tiers: '',
    note: '',
  })
  const [substitute, setSubstitute] = useState<SubstituteDraft>({
    substitute_mpn: '',
    reason: '',
  })

  const [editingId, setEditingId] = useState<string | null>(null)
  const [editDraft, setEditDraft] = useState<LineDraft>({ mpn: '', mfr: '', package: '', qty: '' })
  const previousImportStatusRef = useRef('')

  const loadSession = useCallback(async () => {
    try {
      const s = await getSession(sessionId)
      setSessionStatus((s.status || '').trim())
      setSessionTitle(s.title || sessionId.slice(0, 8))
      setCustomerName(s.customer_name || '')
      setContactPhone(s.contact_phone || '')
      setContactEmail(s.contact_email || '')
      setContactExtra(s.contact_extra || '')
      setRevision(s.selection_revision)
      setImportStatus((s.import_status || '').trim())
      setImportProgress(s.import_progress ?? 0)
      setImportStage((s.import_stage || '').trim())
      setImportMessage((s.import_message || '').trim())
      setImportErrorCode((s.import_error_code || '').trim())
      setImportError((s.import_error || '').trim())
      setImportUpdatedAt((s.import_updated_at || '').trim())
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

  const loadGapAndRuns = useCallback(async () => {
    try {
      const [{ gaps }, { runs }] = await Promise.all([
        listLineGaps(sessionId, GAP_STATUSES),
        listMatchRuns(sessionId),
      ])
      setLineGaps(gaps)
      setMatchRuns(runs)
    } catch {
      setLineGaps([])
      setMatchRuns([])
    }
  }, [sessionId])

  const loadSearchTasks = useCallback(async () => {
    setSearchTasksLoading(true)
    try {
      const data = await listSessionSearchTasks(sessionId)
      setSearchTasks(data)
    } catch {
      setSearchTasks(null)
    } finally {
      setSearchTasksLoading(false)
    }
  }, [sessionId])

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    ;(async () => {
      await loadSession()
      await loadLines()
      await loadGapAndRuns()
      await loadSearchTasks()
      if (!cancelled) setLoading(false)
    })()
    return () => {
      cancelled = true
    }
  }, [loadGapAndRuns, loadLines, loadSearchTasks, loadSession])

  useEffect(() => {
    if (importStatus !== 'parsing') return
    const timer = window.setInterval(() => {
      void loadSession()
    }, 2000)
    return () => window.clearInterval(timer)
  }, [importStatus, loadSession])

  useEffect(() => {
    const previous = previousImportStatusRef.current
    previousImportStatusRef.current = importStatus
    if (previous === 'parsing' && importStatus === 'ready') {
      void loadLines()
      void loadGapAndRuns()
      void loadSearchTasks()
    }
  }, [importStatus, loadGapAndRuns, loadLines, loadSearchTasks])

  const togglePlatform = (id: string) => {
    setSelectedPlatforms((prev) =>
      prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]
    )
  }

  const handleSaveHeader = async () => {
    setSavingHeader(true)
    setLineMsg(null)
    try {
      await patchSession(sessionId, {
        title: sessionTitle,
        customer_name: customerName,
        contact_phone: contactPhone,
        contact_email: contactEmail,
        contact_extra: contactExtra,
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

  const retrySearchTaskItems = async (items: { mpn: string; platform_id: string }[]) => {
    if (items.length === 0) return
    setRetryingSearchTasks(true)
    try {
      await retrySearchTasks(sessionId, items)
      await loadLines()
      await loadSearchTasks()
    } catch (e) {
      setErr(e instanceof Error ? e.message : '重试失败')
    } finally {
      setRetryingSearchTasks(false)
    }
  }

  const handleRetrySearchTask = async (task: SessionSearchTaskRow) => {
    await retrySearchTaskItems([
      { mpn: task.mpn_raw || task.mpn_norm, platform_id: task.platform_id },
    ])
  }

  const handleRetrySearchTaskBatch = async () => {
    const items = (searchTasks?.tasks ?? [])
      .filter((task) => task.retryable && task.search_ui_state !== 'no_data')
      .map((task) => ({ mpn: task.mpn_raw || task.mpn_norm, platform_id: task.platform_id }))
    await retrySearchTaskItems(items)
  }

  const handleSaveMatchRun = async () => {
    setSavingRun(true)
    setRunMsg(null)
    try {
      const r = await saveMatchRun(sessionId)
      await loadGapAndRuns()
      setRunMsg(`已保存配单 V${r.run_no}`)
    } catch (e) {
      setRunMsg(e instanceof Error ? e.message : '保存配单方案失败')
    } finally {
      setSavingRun(false)
    }
  }

  const startGapAction = (gap: BOMLineGap) => {
    setGapActionId(gap.gap_id)
    setManualQuote((draft) => ({ ...draft, model: gap.mpn || draft.model }))
    setSubstitute({
      substitute_mpn: gap.substitute_mpn || '',
      reason: gap.substitute_reason || '',
    })
    setRunMsg(null)
  }

  const handleManualQuote = async (gapId: string) => {
    const model = manualQuote.model.trim()
    if (!model) {
      setRunMsg('请填写人工报价型号')
      return
    }
    setRunMsg(null)
    try {
      await resolveLineGapManualQuote(gapId, {
        model,
        manufacturer: manualQuote.manufacturer.trim(),
        package: manualQuote.package.trim(),
        stock: manualQuote.stock.trim(),
        price_tiers: manualQuote.price_tiers.trim(),
        note: manualQuote.note.trim(),
      })
      setGapActionId(null)
      await Promise.all([loadLines(), loadGapAndRuns()])
      setRunMsg('已补充人工报价')
    } catch (e) {
      setRunMsg(e instanceof Error ? e.message : '人工报价保存失败')
    }
  }

  const handleSelectSubstitute = async (gapId: string) => {
    const substituteMpn = substitute.substitute_mpn.trim()
    if (!substituteMpn) {
      setRunMsg('请填写替代料 MPN')
      return
    }
    setRunMsg(null)
    try {
      await selectLineGapSubstitute(gapId, {
        substitute_mpn: substituteMpn,
        reason: substitute.reason.trim(),
      })
      setGapActionId(null)
      await Promise.all([loadLines(), loadGapAndRuns()])
      setRunMsg('已选择替代料')
    } catch (e) {
      setRunMsg(e instanceof Error ? e.message : '替代料保存失败')
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
    return <div className="flex justify-center py-20 text-slate-500">加载会话...</div>
  }

  const importParsing = importStatus === 'parsing'
  const canEnterMatch =
    !importParsing && sessionStatus === SESSION_MATCH_READY && Boolean(onEnterMatch)
  const blockingAvailabilityLines = lines.filter((line) =>
    BLOCKING_AVAILABILITY_STATUSES.has(normalizeAvailabilityStatus(line.availability_status))
  )
  const noDataLineCount = blockingAvailabilityLines.filter(
    (line) => normalizeAvailabilityStatus(line.availability_status) === 'no_data'
  ).length
  const unavailableLineCount = blockingAvailabilityLines.filter(
    (line) => normalizeAvailabilityStatus(line.availability_status) === 'collection_unavailable'
  ).length
  const noMatchAfterFilterLineCount = blockingAvailabilityLines.filter(
    (line) => normalizeAvailabilityStatus(line.availability_status) === 'no_match_after_filter'
  ).length

  return (
    <div className={embedded ? 'space-y-6' : 'space-y-8'}>
      <div>
        {!embedded && <h2 className="text-2xl font-bold text-slate-800">货源会话</h2>}
        <p className={`text-slate-600 ${embedded ? '' : 'mt-1'}`}>
          <span className="font-medium text-slate-800">{sessionTitle}</span>
          {' · '}
          <span className="font-mono text-sm">{sessionId}</span>
          {' · '}
          <span className="text-slate-500">
            状态 <code className="rounded bg-slate-100 px-1 text-slate-800">{sessionStatus || '—'}</code>
          </span>
        </p>
        <p className="mt-2 text-sm text-slate-600">
          会话状态为 <code className="rounded bg-slate-100 px-1 text-slate-800">data_ready</code>{' '}
          时可使用下方“配单”或顶部“匹配单”。
        </p>
        <div className="mt-3 flex flex-wrap items-center gap-2">
          <button
            type="button"
            disabled={!canEnterMatch}
            title={
              canEnterMatch
                ? '进入配单'
                : !onEnterMatch
                  ? '未配置配单入口'
                  : `当前状态为“${sessionStatus || '—'}”，需 data_ready 后可配单`
            }
            onClick={() => {
              if (canEnterMatch) onEnterMatch?.()
            }}
            className={
              canEnterMatch
                ? 'rounded-lg bg-blue-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-700'
                : 'cursor-not-allowed rounded-lg bg-slate-200 px-3 py-1.5 text-sm font-medium text-slate-400'
            }
          >
            配单
          </button>
          <button
            type="button"
            disabled={exporting || importParsing}
            onClick={() => void handleExport('xlsx')}
            className="rounded-lg border border-slate-300 bg-white px-3 py-1.5 text-sm hover:bg-slate-50 disabled:opacity-50"
          >
            {exporting ? '导出中…' : '导出 Excel'}
          </button>
          <button
            type="button"
            disabled={exporting || importParsing}
            onClick={() => void handleExport('csv')}
            className="rounded-lg border border-slate-300 bg-white px-3 py-1.5 text-sm hover:bg-slate-50 disabled:opacity-50"
          >
            导出 CSV
          </button>
          <button
            type="button"
            disabled={savingRun || importParsing}
            onClick={() => void handleSaveMatchRun()}
            className="rounded-lg bg-emerald-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-emerald-700 disabled:opacity-50"
          >
            {savingRun ? '保存中...' : '保存配单方案'}
          </button>
          <button
            type="button"
            onClick={() => void Promise.all([loadSession(), loadLines(), loadGapAndRuns(), loadSearchTasks()])}
            className="rounded-lg border border-slate-300 bg-white px-3 py-1.5 text-sm hover:bg-slate-50"
          >
            刷新行列表
          </button>
          <span className="self-center text-xs text-slate-500">GET /bom-sessions/.../export</span>
        </div>
      </div>

      {err && (
        <div className="rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-900">
          <strong className="mb-1 block">接口提示</strong>
          {err}
        </div>
      )}

      {importStatus && (
        <SessionImportStatusCard
          status={importStatus}
          progress={importProgress}
          stage={importStage}
          message={importMessage}
          errorCode={importErrorCode}
          error={importError}
          updatedAt={importUpdatedAt}
        />
      )}

      <SearchTaskStatusPanel
        data={searchTasks}
        loading={searchTasksLoading}
        retrying={retryingSearchTasks}
        onRefresh={() => void loadSearchTasks()}
        onRetryBatch={() => void handleRetrySearchTaskBatch()}
        onRetryTask={(task) => void handleRetrySearchTask(task)}
      />

      <section className="rounded-xl border border-slate-200 bg-white p-6 shadow-sm">
        <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
          <div>
            <h3 className="font-semibold text-slate-800">缺口与配单方案</h3>
            <p className="mt-1 text-sm text-slate-600">
              当前缺口 {lineGaps.length} 个，已保存配单方案 {matchRuns.length} 个。
            </p>
          </div>
          <button
            type="button"
            disabled={savingRun || importParsing}
            onClick={() => void handleSaveMatchRun()}
            className="rounded-lg bg-emerald-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-emerald-700 disabled:opacity-50"
          >
            {savingRun ? '保存中...' : '保存配单方案'}
          </button>
        </div>

        {runMsg && <p className="mb-3 text-sm text-slate-700">{runMsg}</p>}

        {matchRuns.length > 0 && (
          <div className="mb-4 flex flex-wrap gap-2">
            {matchRuns.map((run) => (
              <span
                key={run.run_id}
                className="rounded-full bg-slate-100 px-3 py-1 text-xs text-slate-700"
                title={run.saved_at || run.created_at}
              >
                配单 V{run.run_no} · 匹配 {run.matched_line_count}/{run.line_total} · 未解决{' '}
                {run.unresolved_line_count}
              </span>
            ))}
          </div>
        )}

        {lineGaps.length === 0 ? (
          <p className="rounded-lg border border-dashed border-slate-300 bg-slate-50 px-3 py-3 text-sm text-slate-500">
            暂无待处理缺口。
          </p>
        ) : (
          <div className="space-y-3">
            {lineGaps.map((gap) => (
              <div key={gap.gap_id} className="rounded-lg border border-slate-200 p-3">
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div>
                    <div className="font-mono text-sm font-semibold text-slate-800">{gap.mpn}</div>
                    <div className="mt-1 text-xs text-slate-500">
                      行 {gap.line_no} · {gap.gap_type} · {gap.reason_code} · {gap.resolution_status}
                    </div>
                    {gap.reason_detail && (
                      <div className="mt-1 text-sm text-slate-600">{gap.reason_detail}</div>
                    )}
                    {gap.substitute_mpn && (
                      <div className="mt-1 text-sm text-emerald-700">
                        替代料 {gap.substitute_mpn}
                        {gap.substitute_reason ? `：${gap.substitute_reason}` : ''}
                      </div>
                    )}
                  </div>
                  <button
                    type="button"
                    onClick={() => startGapAction(gap)}
                    className="rounded-lg border border-slate-300 bg-white px-3 py-1.5 text-xs hover:bg-slate-50"
                  >
                    处理缺口
                  </button>
                </div>

                {gapActionId === gap.gap_id && (
                  <div className="mt-3 grid gap-3 border-t border-slate-100 pt-3 lg:grid-cols-2">
                    <div className="rounded-lg bg-slate-50 p-3">
                      <div className="mb-2 text-sm font-medium text-slate-700">补人工报价</div>
                      <div className="grid gap-2 sm:grid-cols-2">
                        <input
                          placeholder="型号 *"
                          value={manualQuote.model}
                          onChange={(e) => setManualQuote((d) => ({ ...d, model: e.target.value }))}
                          className="rounded border border-slate-300 px-2 py-1.5 text-sm font-mono"
                        />
                        <input
                          placeholder="厂牌"
                          value={manualQuote.manufacturer}
                          onChange={(e) =>
                            setManualQuote((d) => ({ ...d, manufacturer: e.target.value }))
                          }
                          className="rounded border border-slate-300 px-2 py-1.5 text-sm"
                        />
                        <input
                          placeholder="封装"
                          value={manualQuote.package}
                          onChange={(e) => setManualQuote((d) => ({ ...d, package: e.target.value }))}
                          className="rounded border border-slate-300 px-2 py-1.5 text-sm"
                        />
                        <input
                          placeholder="库存"
                          value={manualQuote.stock}
                          onChange={(e) => setManualQuote((d) => ({ ...d, stock: e.target.value }))}
                          className="rounded border border-slate-300 px-2 py-1.5 text-sm"
                        />
                        <input
                          placeholder="阶梯价"
                          value={manualQuote.price_tiers}
                          onChange={(e) =>
                            setManualQuote((d) => ({ ...d, price_tiers: e.target.value }))
                          }
                          className="rounded border border-slate-300 px-2 py-1.5 text-sm sm:col-span-2"
                        />
                      </div>
                      <button
                        type="button"
                        onClick={() => void handleManualQuote(gap.gap_id)}
                        className="mt-2 rounded-lg bg-slate-800 px-3 py-1.5 text-xs text-white hover:bg-slate-900"
                      >
                        保存人工报价
                      </button>
                    </div>

                    <div className="rounded-lg bg-slate-50 p-3">
                      <div className="mb-2 text-sm font-medium text-slate-700">选择替代料</div>
                      <div className="grid gap-2 sm:grid-cols-2">
                        <input
                          placeholder="替代料 MPN *"
                          value={substitute.substitute_mpn}
                          onChange={(e) =>
                            setSubstitute((d) => ({ ...d, substitute_mpn: e.target.value }))
                          }
                          className="rounded border border-slate-300 px-2 py-1.5 text-sm font-mono"
                        />
                        <input
                          placeholder="原因"
                          value={substitute.reason}
                          onChange={(e) => setSubstitute((d) => ({ ...d, reason: e.target.value }))}
                          className="rounded border border-slate-300 px-2 py-1.5 text-sm"
                        />
                      </div>
                      <button
                        type="button"
                        onClick={() => void handleSelectSubstitute(gap.gap_id)}
                        className="mt-2 rounded-lg bg-slate-800 px-3 py-1.5 text-xs text-white hover:bg-slate-900"
                      >
                        使用替代料
                      </button>
                    </div>
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </section>

      <section className="rounded-xl border border-slate-200 bg-white p-6 shadow-sm">
        <h3 className="mb-3 font-semibold text-slate-800">单据信息（PATCH /bom-sessions）</h3>
        <div className="grid gap-3 sm:grid-cols-2">
          <div className="sm:col-span-2">
            <label className="mb-1 block text-xs text-slate-500">标题</label>
            <input
              type="text"
              value={sessionTitle}
              onChange={(e) => setSessionTitle(e.target.value)}
              className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm"
            />
          </div>
          <div>
            <label className="mb-1 block text-xs text-slate-500">客户名称</label>
            <input
              type="text"
              value={customerName}
              onChange={(e) => setCustomerName(e.target.value)}
              className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm"
            />
          </div>
          <div>
            <label className="mb-1 block text-xs text-slate-500">联系电话</label>
            <input
              type="text"
              value={contactPhone}
              onChange={(e) => setContactPhone(e.target.value)}
              className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm"
            />
          </div>
          <div>
            <label className="mb-1 block text-xs text-slate-500">邮箱</label>
            <input
              type="email"
              value={contactEmail}
              onChange={(e) => setContactEmail(e.target.value)}
              className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm"
            />
          </div>
          <div>
            <label className="mb-1 block text-xs text-slate-500">备注</label>
            <input
              type="text"
              value={contactExtra}
              onChange={(e) => setContactExtra(e.target.value)}
              className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm"
            />
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
        <h3 className="mb-3 font-semibold text-slate-800">勾选平台（PUT /platforms）</h3>
        <p className="mb-3 text-sm text-slate-600">与接口清单 platform_id 枚举一致，全量替换。</p>
        <div className="mb-4 flex flex-wrap gap-3">
          {PLATFORM_IDS.map((id) => (
            <label key={id} className="flex cursor-pointer items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={selectedPlatforms.includes(id)}
                onChange={() => togglePlatform(id)}
              />
              {id}
            </label>
          ))}
        </div>
        {platformErr && <p className="mb-2 text-sm text-red-600">{platformErr}</p>}
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
        <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
          <h3 className="font-semibold text-slate-800">BOM 行（GET /lines · 增删改）</h3>
        </div>

        <div className="mb-6 rounded-lg border border-dashed border-slate-300 bg-slate-50/80 p-4">
          <p className="mb-2 text-sm font-medium text-slate-700">添加一行（POST /lines）</p>
          <div className="flex flex-wrap items-end gap-2">
            <input
              placeholder="MPN *"
              value={newLine.mpn}
              onChange={(e) => setNewLine((n) => ({ ...n, mpn: e.target.value }))}
              className="w-40 rounded border border-slate-300 px-2 py-1.5 text-sm font-mono"
            />
            <input
              placeholder="厂牌"
              value={newLine.mfr}
              onChange={(e) => setNewLine((n) => ({ ...n, mfr: e.target.value }))}
              className="w-28 rounded border border-slate-300 px-2 py-1.5 text-sm"
            />
            <input
              placeholder="封装"
              value={newLine.package}
              onChange={(e) => setNewLine((n) => ({ ...n, package: e.target.value }))}
              className="w-24 rounded border border-slate-300 px-2 py-1.5 text-sm"
            />
            <input
              placeholder="数量"
              value={newLine.qty}
              onChange={(e) => setNewLine((n) => ({ ...n, qty: e.target.value }))}
              className="w-20 rounded border border-slate-300 px-2 py-1.5 text-sm"
            />
            <button
              type="button"
              disabled={addingLine}
              onClick={() => void handleAddLine()}
              className="rounded-lg bg-emerald-600 px-3 py-1.5 text-sm text-white hover:bg-emerald-700 disabled:opacity-50"
            >
              {addingLine ? '添加中…' : '添加'}
            </button>
          </div>
        </div>

        {searchCoverage && !searchCoverage.consistent && (
          <div className="mb-3 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-950">
            搜索任务未完全对齐：缺 {searchCoverage.missing_tasks.length} 条应有任务（期望{' '}
            {searchCoverage.expected_task_count} / 库内 {searchCoverage.existing_task_count}）。
            {searchCoverage.orphan_task_count > 0 && (
              <span> 另有 {searchCoverage.orphan_task_count} 条历史任务与当前行/平台不一致。</span>
            )}
          </div>
        )}

        {blockingAvailabilityLines.length > 0 && (
          <div className="mb-3 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-950">
            当前 BOM 有 {blockingAvailabilityLines.length} 行暂不能配单：无数据 {noDataLineCount} 行，采集不可用{' '}
            {unavailableLineCount} 行，筛选后无匹配 {noMatchAfterFilterLineCount} 行。
          </div>
        )}

        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-slate-200 bg-slate-50 text-left">
                <th className="px-2 py-2">行</th>
                <th className="px-2 py-2">MPN</th>
                <th className="px-2 py-2">厂牌</th>
                <th className="px-2 py-2">封装</th>
                <th className="px-2 py-2">数量</th>
                <th className="px-2 py-2">数据可用性</th>
                <th className="px-2 py-2">match_status</th>
                <th className="px-2 py-2">platform（四态 / phase）</th>
                <th className="w-40 px-2 py-2">操作</th>
              </tr>
            </thead>
            <tbody>
              {lines.length === 0 ? (
                <tr>
                  <td colSpan={9} className="py-8 text-center text-slate-500">
                    暂无行数据（可上方添加，或上传 Excel）
                  </td>
                </tr>
              ) : (
                lines.map((row) => (
                  <tr key={row.line_id || String(row.line_no)} className="border-b border-slate-100 align-top">
                    <td className="px-2 py-2">{row.line_no}</td>
                    {editingId === row.line_id ? (
                      <>
                        <td className="px-2 py-2">
                          <input
                            value={editDraft.mpn}
                            onChange={(e) => setEditDraft((d) => ({ ...d, mpn: e.target.value }))}
                            className="w-full rounded border border-slate-300 px-1 py-0.5 text-xs font-mono"
                          />
                        </td>
                        <td className="px-2 py-2">
                          <input
                            value={editDraft.mfr}
                            onChange={(e) => setEditDraft((d) => ({ ...d, mfr: e.target.value }))}
                            className="w-full rounded border border-slate-300 px-1 py-0.5 text-xs"
                          />
                        </td>
                        <td className="px-2 py-2">
                          <input
                            value={editDraft.package}
                            onChange={(e) => setEditDraft((d) => ({ ...d, package: e.target.value }))}
                            className="w-full rounded border border-slate-300 px-1 py-0.5 text-xs"
                          />
                        </td>
                        <td className="px-2 py-2">
                          <input
                            value={editDraft.qty}
                            onChange={(e) => setEditDraft((d) => ({ ...d, qty: e.target.value }))}
                            className="w-20 rounded border border-slate-300 px-1 py-0.5 text-xs"
                          />
                        </td>
                      </>
                    ) : (
                      <>
                        <td className="px-2 py-2 font-mono">{row.mpn}</td>
                        <td className="px-2 py-2">{row.mfr || '—'}</td>
                        <td className="px-2 py-2">{row.package || '—'}</td>
                        <td className="px-2 py-2">{row.qty}</td>
                      </>
                    )}
                    <td className="px-2 py-2">
                      <span
                        className={`inline-flex whitespace-nowrap rounded border px-2 py-0.5 text-xs font-medium ${availabilityStatusClass(
                          row.availability_status
                        )}`}
                      >
                        {availabilityStatusLabel(row.availability_status)}
                      </span>
                      {row.availability_reason && (
                        <div className="mt-1 max-w-48 text-xs text-slate-500">
                          {row.availability_reason}
                        </div>
                      )}
                    </td>
                    <td className="px-2 py-2">{row.match_status || '—'}</td>
                    <td className="max-w-xs px-2 py-2">
                      {(row.platform_gaps || []).length === 0 ? (
                        '—'
                      ) : (
                        <ul className="list-disc space-y-1 pl-4 text-xs">
                          {row.platform_gaps.map((g, i) => (
                            <li key={i}>
                              <span className="font-mono">{g.platform_id}</span>
                              {g.search_ui_state && (
                                <span className="ml-1 font-medium text-violet-700">
                                  [{g.search_ui_state}]
                                </span>
                              )}{' '}
                              {g.phase}{' '}
                              {g.reason_code && (
                                <span className="text-slate-500">({g.reason_code})</span>
                              )}{' '}
                              {g.message}
                            </li>
                          ))}
                        </ul>
                      )}
                    </td>
                    <td className="whitespace-nowrap px-2 py-2">
                      {editingId === row.line_id ? (
                        <span className="flex flex-wrap gap-1">
                          <button
                            type="button"
                            onClick={() => void saveEdit()}
                            className="text-xs text-emerald-600 hover:underline"
                          >
                            保存
                          </button>
                          <button
                            type="button"
                            onClick={cancelEdit}
                            className="text-xs text-slate-500 hover:underline"
                          >
                            取消
                          </button>
                        </span>
                      ) : (
                        <span className="flex flex-wrap gap-2">
                          <button
                            type="button"
                            onClick={() => startEdit(row)}
                            className="text-xs text-blue-600 hover:underline"
                          >
                            编辑
                          </button>
                          <button
                            type="button"
                            onClick={() => void handleDeleteLine(row.line_id)}
                            className="text-xs text-red-600 hover:underline"
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
