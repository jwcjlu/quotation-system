import { useCallback, useEffect, useState } from 'react'
import { listSessions, type SessionListItem } from '../../api'
import { DEFAULT_PAGE_SIZE } from '../pagination'

interface SessionListPanelProps {
  selectedSessionId: string | null
  onSelectSession: (sessionId: string, lineCount?: number) => void
  onSelectedSessionLineCount?: (lineCount: number | null) => void
  onCreateSession: () => void
}

export function SessionListPanel({
  selectedSessionId,
  onSelectSession,
  onSelectedSessionLineCount,
  onCreateSession,
}: SessionListPanelProps) {
  const [listPage, setListPage] = useState(1)
  const [pageSize] = useState(DEFAULT_PAGE_SIZE)
  const [status, setStatus] = useState('')
  const [bizDate, setBizDate] = useState('')
  const [q, setQ] = useState('')
  const [items, setItems] = useState<SessionListItem[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [err, setErr] = useState<string | null>(null)

  const load = useCallback(async () => {
    setLoading(true)
    setErr(null)
    try {
      const reply = await listSessions({
        page: listPage,
        page_size: pageSize,
        status: status.trim() || undefined,
        biz_date: bizDate.trim() || undefined,
        q: q.trim() || undefined,
      })
      setItems(reply.items)
      setTotal(reply.total)
      const selectedRow = reply.items.find((row) => row.session_id === selectedSessionId)
      onSelectedSessionLineCount?.(selectedRow ? selectedRow.line_count : null)
    } catch (error) {
      setErr(error instanceof Error ? error.message : '\u4f1a\u8bdd\u5217\u8868\u52a0\u8f7d\u5931\u8d25')
      setItems([])
      setTotal(0)
      onSelectedSessionLineCount?.(null)
    } finally {
      setLoading(false)
    }
  }, [bizDate, listPage, onSelectedSessionLineCount, pageSize, q, selectedSessionId, status])

  useEffect(() => {
    void load()
  }, [load])

  const totalPages = Math.max(1, Math.ceil(total / pageSize) || 1)

  return (
    <aside
      className="min-h-full space-y-4 border-[#d9e1ec] bg-white p-6 lg:border-r"
      data-testid="session-list-panel"
    >
      <div className="flex items-start justify-between gap-3">
        <div>
          <h3 className="text-[15px] font-bold text-slate-950">{'BOM\u4f1a\u8bdd'}</h3>
          <p className="mt-2 text-xs text-slate-500">{'\u9009\u62e9\u4f1a\u8bdd\u8fdb\u5165\u5de5\u4f5c\u533a'}</p>
        </div>
        <button
          type="button"
          onClick={onCreateSession}
          className="rounded-md bg-[#1f2a3d] px-4 py-3 text-sm font-bold text-white hover:bg-slate-900"
        >
          {'\u4e0a\u4f20 BOM'}
        </button>
      </div>

      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-1">
        <input
          value={status}
          onChange={(event) => {
            setStatus(event.target.value)
            setListPage(1)
          }}
          placeholder={'\u72b6\u6001'}
          className="h-9 rounded-md border border-[#d7e0ed] px-3 text-sm text-slate-700 placeholder:text-slate-400"
        />
        <input
          type="date"
          value={bizDate}
          onChange={(event) => {
            setBizDate(event.target.value)
            setListPage(1)
          }}
          className="h-9 rounded-md border border-[#d7e0ed] px-3 text-sm text-slate-700"
        />
        <input
          value={q}
          onChange={(event) => {
            setQ(event.target.value)
            setListPage(1)
          }}
          placeholder={'\u6807\u9898 / \u5ba2\u6237'}
          className="h-9 rounded-md border border-[#d7e0ed] px-3 text-sm text-slate-700 placeholder:text-slate-400 sm:col-span-2 lg:col-span-1"
        />
      </div>

      {err && <div className="rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-800">{err}</div>}
      {loading ? <p className="text-sm text-slate-500">{'\u52a0\u8f7d\u4e2d...'}</p> : null}

      <div className="space-y-2">
        {items.map((row) => (
          <button
            key={row.session_id}
            type="button"
            onClick={() => onSelectSession(row.session_id, row.line_count)}
            className={`w-full rounded-lg border px-4 py-3 text-left text-sm transition ${
              selectedSessionId === row.session_id
                ? 'border-[#8fb2ff] bg-[#eef4ff]'
                : 'border-[#d7e0ed] bg-white hover:bg-slate-50'
            }`}
          >
            <span className="block font-bold text-slate-950">{row.title || row.session_id}</span>
            <span className="mt-2 block text-xs text-slate-600">
              {row.status} {' | '} {row.biz_date} {' | '} {row.line_count} {'\u884c'}
            </span>
          </button>
        ))}
      </div>

      <div className="flex items-center justify-between text-xs text-slate-500">
        <span>
          {'\u5171 '} {total} {' | '} {listPage}/{totalPages}
        </span>
        <div className="flex gap-2">
          <button
            type="button"
            disabled={listPage <= 1}
            onClick={() => setListPage((page) => Math.max(1, page - 1))}
            className="rounded-md border border-[#d7e0ed] px-2 py-1 disabled:opacity-40"
          >
            {'\u4e0a\u4e00\u9875'}
          </button>
          <button
            type="button"
            disabled={listPage >= totalPages}
            onClick={() => setListPage((page) => page + 1)}
            className="rounded-md border border-[#d7e0ed] px-2 py-1 disabled:opacity-40"
          >
            {'\u4e0b\u4e00\u9875'}
          </button>
        </div>
      </div>
    </aside>
  )
}
