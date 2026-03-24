import { useCallback, useEffect, useState } from 'react'
import {
  PLATFORM_IDS,
  exportSessionFile,
  getBOMLines,
  getReadiness,
  getSession,
  putPlatforms,
  retrySearchTasks,
  type BOMLineRow,
  type GetReadinessReply,
} from '../api'

interface SourcingSessionPageProps {
  sessionId: string
  onOpenMatch: () => void
}

const POLL_MS = 2500

export function SourcingSessionPage({ sessionId, onOpenMatch }: SourcingSessionPageProps) {
  const [sessionTitle, setSessionTitle] = useState('')
  const [revision, setRevision] = useState(1)
  const [selectedPlatforms, setSelectedPlatforms] = useState<string[]>([...PLATFORM_IDS])
  const [readiness, setReadiness] = useState<GetReadinessReply | null>(null)
  const [lines, setLines] = useState<BOMLineRow[]>([])
  const [loading, setLoading] = useState(true)
  const [err, setErr] = useState<string | null>(null)
  const [platformErr, setPlatformErr] = useState<string | null>(null)
  const [savingPlatforms, setSavingPlatforms] = useState(false)
  const [exporting, setExporting] = useState(false)

  const loadSession = useCallback(async () => {
    try {
      const s = await getSession(sessionId)
      setSessionTitle(s.title || sessionId.slice(0, 8))
      setRevision(s.selection_revision)
      if (s.platform_ids?.length) setSelectedPlatforms(s.platform_ids)
    } catch (e) {
      setErr(e instanceof Error ? e.message : '加载会话失败')
    }
  }, [sessionId])

  const pollReadiness = useCallback(async () => {
    try {
      const r = await getReadiness(sessionId)
      setReadiness(r)
      setErr(null)
    } catch (e) {
      setReadiness(null)
      setErr(e instanceof Error ? e.message : '就绪接口失败')
    }
  }, [sessionId])

  const loadLines = useCallback(async () => {
    try {
      const { lines: rows } = await getBOMLines(sessionId)
      setLines(rows)
    } catch {
      setLines([])
    }
  }, [sessionId])

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    ;(async () => {
      await loadSession()
      await pollReadiness()
      await loadLines()
      if (!cancelled) setLoading(false)
    })()
    return () => {
      cancelled = true
    }
  }, [loadSession, pollReadiness, loadLines])

  useEffect(() => {
    const t = window.setInterval(() => {
      void pollReadiness()
      void loadLines()
    }, POLL_MS)
    return () => window.clearInterval(t)
  }, [pollReadiness, loadLines])

  const togglePlatform = (id: string) => {
    setSelectedPlatforms((prev) =>
      prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]
    )
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
      await pollReadiness()
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
      await pollReadiness()
    } catch (e) {
      setErr(e instanceof Error ? e.message : '重试失败')
    }
  }

  if (loading) {
    return (
      <div className="flex justify-center py-20 text-slate-500">加载会话...</div>
    )
  }

  return (
    <div className="space-y-8">
      <div>
        <h2 className="text-2xl font-bold text-slate-800">货源会话</h2>
        <p className="text-slate-600 mt-1">
          {sessionTitle} · <span className="font-mono text-sm">{sessionId}</span>
        </p>
        <div className="mt-3 flex flex-wrap gap-2">
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
          <span className="text-xs text-slate-500 self-center">GET /bom-sessions/.../export</span>
        </div>
      </div>

      {err && (
        <div className="rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-900">
          <strong className="block mb-1">接口提示</strong>
          {err}
          <p className="mt-2 text-amber-800/90">
            若显示 501 / NOT_IMPLEMENTED，表示服务端尚未实现该 RPC，前端已预留轮询与表格结构。
          </p>
        </div>
      )}

      <section className="rounded-xl border border-slate-200 bg-white p-6 shadow-sm">
        <h3 className="font-semibold text-slate-800 mb-3">就绪状态（轮询 {POLL_MS / 1000}s）</h3>
        {readiness ? (
          <dl className="grid gap-2 text-sm sm:grid-cols-2">
            <div>
              <dt className="text-slate-500">phase</dt>
              <dd className="font-medium">{readiness.phase || '—'}</dd>
            </div>
            <div>
              <dt className="text-slate-500">可进入配单</dt>
              <dd className="font-medium">{readiness.can_enter_match ? '是' : '否'}</dd>
            </div>
            <div className="sm:col-span-2">
              <dt className="text-slate-500">阻塞原因</dt>
              <dd>{readiness.block_reason || '—'}</dd>
            </div>
            <div>
              <dt className="text-slate-500">业务日 / revision</dt>
              <dd>
                {readiness.biz_date} · rev {readiness.selection_revision}
              </dd>
            </div>
          </dl>
        ) : (
          <p className="text-slate-500 text-sm">暂无就绪数据</p>
        )}
        <div className="mt-4 flex flex-wrap gap-3">
          <button
            type="button"
            onClick={() => {
              void pollReadiness()
              void loadLines()
            }}
            className="rounded-lg border border-slate-300 px-4 py-2 text-sm hover:bg-slate-50"
          >
            立即刷新
          </button>
          <button
            type="button"
            onClick={onOpenMatch}
            disabled={!readiness?.can_enter_match}
            className="rounded-lg bg-blue-600 px-4 py-2 text-sm text-white hover:bg-blue-700 disabled:opacity-40"
          >
            打开经典配单页
          </button>
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
          <h3 className="font-semibold text-slate-800">行与 platform_gaps（GET /lines）</h3>
          <button
            type="button"
            onClick={() => void handleRetryFirstGap()}
            className="text-sm text-blue-600 hover:underline"
          >
            重试第一条缺口（示例）
          </button>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-slate-200 bg-slate-50 text-left">
                <th className="py-2 px-2">行</th>
                <th className="py-2 px-2">MPN</th>
                <th className="py-2 px-2">数量</th>
                <th className="py-2 px-2">match_status</th>
                <th className="py-2 px-2">platform_gaps</th>
              </tr>
            </thead>
            <tbody>
              {lines.length === 0 ? (
                <tr>
                  <td colSpan={5} className="py-8 text-center text-slate-500">
                    暂无行数据（上传 BOM 后应出现；若后端未实现则为空）
                  </td>
                </tr>
              ) : (
                lines.map((row) => (
                  <tr key={row.line_id || String(row.line_no)} className="border-b border-slate-100">
                    <td className="py-2 px-2">{row.line_no}</td>
                    <td className="py-2 px-2 font-mono">{row.mpn}</td>
                    <td className="py-2 px-2">{row.qty}</td>
                    <td className="py-2 px-2">{row.match_status || '—'}</td>
                    <td className="py-2 px-2 max-w-md">
                      {(row.platform_gaps || []).length === 0 ? (
                        '—'
                      ) : (
                        <ul className="list-disc pl-4 space-y-1">
                          {row.platform_gaps.map((g, i) => (
                            <li key={i}>
                              <span className="font-mono">{g.platform_id}</span> {g.phase}{' '}
                              {g.reason_code && <span className="text-slate-500">({g.reason_code})</span>}{' '}
                              {g.message}
                            </li>
                          ))}
                        </ul>
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
