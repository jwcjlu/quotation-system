import { useCallback, useEffect, useState } from 'react'
import { getMatchHistory, listMatchHistory, type MatchHistoryListItem, type MatchItem } from '../api'

export function MatchHistoryPage() {
  const [items, setItems] = useState<MatchHistoryListItem[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(true)
  const [err, setErr] = useState<string | null>(null)
  const [detail, setDetail] = useState<{
    meta: MatchHistoryListItem
    items: MatchItem[]
  } | null>(null)

  const pageSize = 15

  const load = useCallback(async () => {
    setLoading(true)
    setErr(null)
    try {
      const r = await listMatchHistory(page, pageSize)
      setItems(r.items)
      setTotal(r.total)
    } catch (e) {
      setErr(e instanceof Error ? e.message : '加载失败')
      setItems([])
      setTotal(0)
    } finally {
      setLoading(false)
    }
  }, [page])

  useEffect(() => {
    void load()
  }, [load])

  const openDetail = async (row: MatchHistoryListItem) => {
    try {
      const d = await getMatchHistory(row.match_result_id)
      setDetail({
        meta: {
          match_result_id: d.match_result_id,
          session_id: d.session_id,
          version: d.version,
          strategy: d.strategy,
          created_at: d.created_at,
          total_amount: d.total_amount,
        },
        items: d.items,
      })
    } catch (e) {
      setErr(e instanceof Error ? e.message : '加载详情失败')
    }
  }

  const totalPages = Math.max(1, Math.ceil(total / pageSize))

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-2xl font-bold text-slate-800">配单历史</h2>
        <p className="text-slate-600 mt-1 text-sm">GET /api/v1/bom-match-history — 会话 UUID 配单成功后会写入快照</p>
      </div>

      {err && <div className="rounded-lg bg-red-50 px-4 py-3 text-sm text-red-700">{err}</div>}

      {loading ? (
        <p className="text-slate-500">加载中...</p>
      ) : (
        <>
          <div className="overflow-x-auto rounded-lg border border-slate-200 bg-white">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-slate-200 bg-slate-50 text-left">
                  <th className="py-2 px-3">ID</th>
                  <th className="py-2 px-3">会话</th>
                  <th className="py-2 px-3">版本</th>
                  <th className="py-2 px-3">策略</th>
                  <th className="py-2 px-3">时间</th>
                  <th className="py-2 px-3">合计</th>
                  <th className="py-2 px-3">操作</th>
                </tr>
              </thead>
              <tbody>
                {items.length === 0 ? (
                  <tr>
                    <td colSpan={7} className="py-10 text-center text-slate-500">
                      暂无记录（需数据库且对会话执行过配单）
                    </td>
                  </tr>
                ) : (
                  items.map((row) => (
                    <tr key={row.match_result_id} className="border-b border-slate-100">
                      <td className="py-2 px-3 font-mono">{row.match_result_id}</td>
                      <td className="py-2 px-3 font-mono text-xs max-w-[200px] truncate" title={row.session_id}>
                        {row.session_id}
                      </td>
                      <td className="py-2 px-3">{row.version}</td>
                      <td className="py-2 px-3">{row.strategy || '—'}</td>
                      <td className="py-2 px-3 whitespace-nowrap">{row.created_at}</td>
                      <td className="py-2 px-3">¥{row.total_amount.toFixed(2)}</td>
                      <td className="py-2 px-3">
                        <button
                          type="button"
                          className="text-blue-600 hover:underline"
                          onClick={() => void openDetail(row)}
                        >
                          查看
                        </button>
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
          <div className="flex items-center justify-between text-sm">
            <span className="text-slate-600">
              共 {total} 条 · 第 {page} / {totalPages} 页
            </span>
            <div className="flex gap-2">
              <button
                type="button"
                disabled={page <= 1}
                onClick={() => setPage((p) => Math.max(1, p - 1))}
                className="rounded border border-slate-300 px-3 py-1 disabled:opacity-40"
              >
                上一页
              </button>
              <button
                type="button"
                disabled={page >= totalPages}
                onClick={() => setPage((p) => p + 1)}
                className="rounded border border-slate-300 px-3 py-1 disabled:opacity-40"
              >
                下一页
              </button>
            </div>
          </div>
        </>
      )}

      {detail && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
          onClick={() => setDetail(null)}
        >
          <div
            className="max-h-[90vh] w-full max-w-4xl overflow-auto rounded-lg bg-white p-6 shadow-xl"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="mb-4 flex justify-between items-start">
              <div>
                <h3 className="text-lg font-bold text-slate-800">快照 #{detail.meta.match_result_id}</h3>
                <p className="text-sm text-slate-600 mt-1">
                  会话 {detail.meta.session_id} · v{detail.meta.version} · {detail.meta.strategy} · 合计 ¥
                  {detail.meta.total_amount.toFixed(2)}
                </p>
              </div>
              <button type="button" onClick={() => setDetail(null)} className="text-slate-500 hover:text-slate-800">
                ✕
              </button>
            </div>
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="bg-slate-100 text-left">
                    <th className="py-2 px-2">#</th>
                    <th className="py-2 px-2">型号</th>
                    <th className="py-2 px-2">数量</th>
                    <th className="py-2 px-2">平台</th>
                    <th className="py-2 px-2">匹配型号</th>
                    <th className="py-2 px-2">单价</th>
                    <th className="py-2 px-2">小计</th>
                    <th className="py-2 px-2">状态</th>
                  </tr>
                </thead>
                <tbody>
                  {detail.items.map((it) => (
                    <tr key={it.index} className="border-b border-slate-100">
                      <td className="py-2 px-2">{it.index}</td>
                      <td className="py-2 px-2">{it.model}</td>
                      <td className="py-2 px-2">{it.quantity}</td>
                      <td className="py-2 px-2">{it.platform || '—'}</td>
                      <td className="py-2 px-2">{it.matched_model || '—'}</td>
                      <td className="py-2 px-2">{it.unit_price?.toFixed(2) ?? '—'}</td>
                      <td className="py-2 px-2">{it.subtotal?.toFixed(2) ?? '—'}</td>
                      <td className="py-2 px-2">{it.match_status}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
