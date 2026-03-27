import { useCallback, useEffect, useState } from 'react'
import { listSessions, type SessionListItem } from '../api'
import { UploadPage } from './UploadPage'
import { SourcingSessionPage } from './SourcingSessionPage'

const LAST_BOM_KEY = 'bom_last_bom_id'
const LAST_SESSION_KEY = 'bom_last_session_id'

interface BomSessionListPageProps {
  onNavigateToMatch: (bomId: string) => void
}

export function BomSessionListPage({ onNavigateToMatch }: BomSessionListPageProps) {
  const [listPage, setListPage] = useState(1)
  const [pageSize] = useState(20)
  const [status, setStatus] = useState('')
  const [bizDate, setBizDate] = useState('')
  const [q, setQ] = useState('')
  const [items, setItems] = useState<SessionListItem[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [err, setErr] = useState<string | null>(null)
  const [uploadOpen, setUploadOpen] = useState(false)
  const [detailSessionId, setDetailSessionId] = useState<string | null>(null)

  const load = useCallback(async () => {
    setLoading(true)
    setErr(null)
    try {
      const res = await listSessions({
        page: listPage,
        page_size: pageSize,
        status: status.trim() || undefined,
        biz_date: bizDate.trim() || undefined,
        q: q.trim() || undefined,
      })
      setItems(res.items)
      setTotal(res.total)
    } catch (e) {
      setErr(e instanceof Error ? e.message : '加载失败')
      setItems([])
      setTotal(0)
    } finally {
      setLoading(false)
    }
  }, [listPage, pageSize, status, bizDate, q])

  useEffect(() => {
    void load()
  }, [load])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== 'Escape') return
      if (uploadOpen) setUploadOpen(false)
      else if (detailSessionId) setDetailSessionId(null)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [uploadOpen, detailSessionId])

  const onSessionUploadSuccess = (id: string) => {
    localStorage.setItem(LAST_SESSION_KEY, id)
    localStorage.setItem(LAST_BOM_KEY, id)
    setUploadOpen(false)
    if (listPage !== 1) setListPage(1)
    else void load()
  }

  const openDetail = (sid: string) => {
    localStorage.setItem(LAST_SESSION_KEY, sid)
    localStorage.setItem(LAST_BOM_KEY, sid)
    setDetailSessionId(sid)
  }

  const totalPages = Math.max(1, Math.ceil(total / pageSize) || 1)

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h2 className="text-2xl font-bold text-slate-800">BOM 会话</h2>
          <p className="text-slate-600 mt-1 text-sm">列表筛选会话；新建或打开详情时在弹框中操作</p>
        </div>
        <button
          type="button"
          onClick={() => setUploadOpen(true)}
          className="rounded-lg bg-slate-800 text-white px-4 py-2 text-sm font-medium hover:bg-slate-900 shrink-0"
        >
          新建 BOM 单
        </button>
      </div>

      {uploadOpen && (
        <div
          className="fixed inset-0 z-50 flex items-start justify-center p-4 pt-10 md:pt-16 bg-black/40 overflow-y-auto"
          role="dialog"
          aria-modal="true"
          aria-labelledby="bom-upload-dialog-title"
          onClick={(e) => {
            if (e.target === e.currentTarget) setUploadOpen(false)
          }}
        >
          <div
            className="bg-white rounded-xl shadow-xl max-w-4xl w-full max-h-[90vh] overflow-y-auto p-4 md:p-6 border border-slate-200"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between gap-4 mb-4">
              <h3 id="bom-upload-dialog-title" className="text-lg font-semibold text-slate-800">
                货源会话 · 新建 BOM
              </h3>
              <button
                type="button"
                onClick={() => setUploadOpen(false)}
                className="text-slate-500 hover:text-slate-800 text-sm px-2 py-1"
              >
                关闭
              </button>
            </div>
            <UploadPage embedded onSuccess={onSessionUploadSuccess} />
          </div>
        </div>
      )}

      {detailSessionId && (
        <div
          className="fixed inset-0 z-50 flex items-start justify-center p-2 md:p-4 pt-4 md:pt-8 bg-black/40 overflow-y-auto"
          role="dialog"
          aria-modal="true"
          aria-labelledby="bom-session-detail-title"
          onClick={(e) => {
            if (e.target === e.currentTarget) setDetailSessionId(null)
          }}
        >
          <div
            className="bg-slate-50 rounded-xl shadow-xl max-w-6xl w-full max-h-[96vh] overflow-y-auto p-3 md:p-5 border border-slate-200"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between gap-4 mb-3 sticky top-0 bg-slate-50 z-10 pb-2 border-b border-slate-200/80">
              <h3 id="bom-session-detail-title" className="text-lg font-semibold text-slate-800">
                会话看板
              </h3>
              <button
                type="button"
                onClick={() => setDetailSessionId(null)}
                className="text-slate-500 hover:text-slate-800 text-sm px-2 py-1"
              >
                关闭
              </button>
            </div>
            <SourcingSessionPage
              embedded
              sessionId={detailSessionId}
              onOpenMatch={() => {
                onNavigateToMatch(detailSessionId)
                setDetailSessionId(null)
              }}
            />
          </div>
        </div>
      )}

      <section className="space-y-4">
        <h3 className="text-lg font-semibold text-slate-800">会话列表</h3>
        <div className="rounded-lg border border-slate-200 bg-white p-4 flex flex-wrap gap-3 items-end">
          <div>
            <label className="block text-xs text-slate-500 mb-1">状态</label>
            <input
              type="text"
              value={status}
              onChange={(e) => {
                setStatus(e.target.value)
                setListPage(1)
              }}
              placeholder="如 draft"
              className="border border-slate-300 rounded px-2 py-1.5 text-sm w-32"
            />
          </div>
          <div>
            <label className="block text-xs text-slate-500 mb-1">业务日</label>
            <input
              type="date"
              value={bizDate}
              onChange={(e) => {
                setBizDate(e.target.value)
                setListPage(1)
              }}
              className="border border-slate-300 rounded px-2 py-1.5 text-sm"
            />
          </div>
          <div className="flex-1 min-w-[200px]">
            <label className="block text-xs text-slate-500 mb-1">关键词（标题 / 客户名）</label>
            <input
              type="text"
              value={q}
              onChange={(e) => {
                setQ(e.target.value)
                setListPage(1)
              }}
              placeholder="搜索…"
              className="border border-slate-300 rounded px-2 py-1.5 text-sm w-full max-w-md"
            />
          </div>
          <button
            type="button"
            onClick={() => void load()}
            className="rounded-lg bg-slate-800 text-white px-4 py-1.5 text-sm hover:bg-slate-900"
          >
            查询
          </button>
        </div>

        {err && (
          <div className="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-900">{err}</div>
        )}

        {loading ? (
          <p className="text-slate-500">加载中…</p>
        ) : (
          <>
            <div className="overflow-x-auto rounded-lg border border-slate-200 bg-white">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-slate-200 bg-slate-50 text-left">
                    <th className="py-2 px-3">标题</th>
                    <th className="py-2 px-3">客户</th>
                    <th className="py-2 px-3">状态</th>
                    <th className="py-2 px-3">业务日</th>
                    <th className="py-2 px-3">行数</th>
                    <th className="py-2 px-3">更新时间</th>
                    <th className="py-2 px-3">操作</th>
                  </tr>
                </thead>
                <tbody>
                  {items.length === 0 ? (
                    <tr>
                      <td colSpan={7} className="py-10 text-center text-slate-500">
                        暂无数据
                      </td>
                    </tr>
                  ) : (
                    items.map((row) => (
                      <tr
                        key={row.session_id}
                        className={`border-b border-slate-100 hover:bg-slate-50 ${
                          detailSessionId === row.session_id ? 'bg-blue-50/60' : ''
                        }`}
                      >
                        <td className="py-2 px-3 max-w-xs truncate" title={row.title}>
                          {row.title || '—'}
                        </td>
                        <td className="py-2 px-3 max-w-[140px] truncate">{row.customer_name || '—'}</td>
                        <td className="py-2 px-3">{row.status}</td>
                        <td className="py-2 px-3 font-mono text-xs">{row.biz_date}</td>
                        <td className="py-2 px-3">{row.line_count}</td>
                        <td className="py-2 px-3 text-xs text-slate-600 whitespace-nowrap">{row.updated_at}</td>
                        <td className="py-2 px-3">
                          <button
                            type="button"
                            onClick={() => openDetail(row.session_id)}
                            className="text-blue-600 hover:underline text-sm"
                          >
                            打开详情
                          </button>
                        </td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>
            <div className="flex items-center justify-between text-sm text-slate-600">
              <span>
                共 {total} 条 · 第 {listPage} / {totalPages} 页
              </span>
              <div className="flex gap-2">
                <button
                  type="button"
                  disabled={listPage <= 1}
                  onClick={() => setListPage((p) => Math.max(1, p - 1))}
                  className="px-3 py-1 rounded border border-slate-300 disabled:opacity-40"
                >
                  上一页
                </button>
                <button
                  type="button"
                  disabled={listPage >= totalPages}
                  onClick={() => setListPage((p) => p + 1)}
                  className="px-3 py-1 rounded border border-slate-300 disabled:opacity-40"
                >
                  下一页
                </button>
              </div>
            </div>
          </>
        )}
      </section>
    </div>
  )
}
