import { useState } from 'react'
import { hsItemDetail, hsItemsList } from '../../api/hsMeta'

type Row = Record<string, unknown>
type Col = { key: string; label: string }

function asRows(value: unknown): Row[] {
  if (!Array.isArray(value)) return []
  return value.filter((item): item is Row => item != null && typeof item === 'object' && !Array.isArray(item))
}

function pickListRows(resp: Row): Row[] {
  const data = resp.data
  if (data && typeof data === 'object' && !Array.isArray(data)) {
    const inner = data as Row
    return asRows(inner.data ?? inner.items ?? inner.rows)
  }
  return asRows(resp.items ?? resp.rows ?? resp.data)
}

function pickListHead(resp: Row): Row[] {
  const data = resp.data
  if (data && typeof data === 'object' && !Array.isArray(data)) {
    return asRows((data as Row).head)
  }
  return asRows(resp.head)
}

function prettifyLabel(key: string): string {
  const names: Record<string, string> = {
    ROWNO: '行号',
    CODE_TS: 'Code TS',
    G_NAME: '商品名称',
    UNIT_1: '第一单位',
    UNIT_2: '第二单位',
    CONTROL_MARK: '监管条件',
  }
  return names[key] ?? key.replace(/_/g, ' ')
}

function buildCols(headRows: Row[], rows: Row[]): Col[] {
  if (headRows.length > 0) {
    return headRows
      .map((head) => {
        const key = String(head.colname ?? '').trim()
        if (!key) return null
        return { key, label: String(head.coldisplay ?? key) }
      })
      .filter((item): item is Col => item != null)
  }

  const priority = ['ROWNO', 'CODE_TS', 'G_NAME', 'UNIT_1', 'UNIT_2', 'CONTROL_MARK']
  const seen = new Set<string>()
  const keys: string[] = []
  for (const key of priority) {
    if (rows.some((row) => key in row)) {
      seen.add(key)
      keys.push(key)
    }
  }
  for (const row of rows.slice(0, 20)) {
    for (const key of Object.keys(row)) {
      if (!seen.has(key)) {
        seen.add(key)
        keys.push(key)
      }
    }
  }
  return keys.map((key) => ({ key, label: prettifyLabel(key) }))
}

function getTotal(resp: Row, rowsLen: number): number {
  const data = resp.data
  if (data && typeof data === 'object' && !Array.isArray(data)) {
    const n = Number((data as Row).totalRows ?? (data as Row).total ?? (data as Row).total_count)
    if (!Number.isNaN(n) && n > 0) return n
  }
  const n = Number(resp.total ?? resp.total_count ?? resp.totalRows)
  return !Number.isNaN(n) && n > 0 ? n : rowsLen
}

export function HsItemsSearchPanel() {
  const [codeTs, setCodeTs] = useState('')
  const [gName, setGName] = useState('')
  const [core6, setCore6] = useState('')
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(50)
  const [pageInput, setPageInput] = useState('1')
  const [rows, setRows] = useState<Row[]>([])
  const [cols, setCols] = useState<Col[]>([])
  const [total, setTotal] = useState(0)
  const [detailJson, setDetailJson] = useState('')
  const [err, setErr] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const searchList = async (targetPage = page) => {
    const safePage = Math.max(1, targetPage)
    setErr(null)
    setBusy(true)
    try {
      const result = await hsItemsList({
        code_ts: codeTs.trim() || undefined,
        g_name: gName.trim() || undefined,
        source_core_hs6: core6.trim() || undefined,
        page: safePage,
        page_size: pageSize,
      })
      const nextRows = pickListRows(result)
      const nextTotal = getTotal(result, nextRows.length)
      const finalPage = Math.min(safePage, Math.max(1, Math.ceil(nextTotal / pageSize)))
      setRows(nextRows)
      setCols(buildCols(pickListHead(result), nextRows))
      setTotal(nextTotal)
      setPage(finalPage)
      setPageInput(String(finalPage))
    } catch (error) {
      setErr(error instanceof Error ? error.message : String(error))
      setRows([])
      setCols([])
      setTotal(0)
    } finally {
      setBusy(false)
    }
  }

  const searchDetail = async () => {
    if (!codeTs.trim()) {
      setErr('请输入 code_ts')
      return
    }
    setErr(null)
    setBusy(true)
    try {
      const result = await hsItemDetail(codeTs.trim())
      setDetailJson(JSON.stringify(result, null, 2))
    } catch (error) {
      setErr(error instanceof Error ? error.message : String(error))
    } finally {
      setBusy(false)
    }
  }

  const totalPages = Math.max(1, Math.ceil(total / pageSize))
  const goToPage = async (target: number) => searchList(Math.min(Math.max(1, target), totalPages))
  const clearInputs = () => {
    setCodeTs('')
    setGName('')
    setCore6('')
    setPage(1)
    setPageInput('1')
  }

  return (
    <div className="space-y-5">
      <div className="grid gap-3 lg:grid-cols-[10rem_1fr_10rem_9rem]">
        <input className="rounded-md border border-slate-300 px-3 py-2 font-mono text-sm" placeholder="code_ts" value={codeTs} onChange={(e) => setCodeTs(e.target.value)} />
        <input className="rounded-md border border-slate-300 px-3 py-2 text-sm" placeholder="商品名称" value={gName} onChange={(e) => setGName(e.target.value)} />
        <input className="rounded-md border border-slate-300 px-3 py-2 font-mono text-sm" placeholder="核心 HS6" value={core6} onChange={(e) => setCore6(e.target.value)} />
        <select className="rounded-md border border-slate-300 px-3 py-2 text-sm" value={pageSize} onChange={(e) => setPageSize(Number(e.target.value) || 50)}>
          <option value={10}>10 / 页</option>
          <option value={20}>20 / 页</option>
          <option value={50}>50 / 页</option>
          <option value={100}>100 / 页</option>
        </select>
      </div>

      <div className="flex flex-wrap gap-2">
        <button type="button" className="rounded-md bg-slate-900 px-4 py-2 text-sm font-medium text-white disabled:bg-slate-300" disabled={busy} onClick={() => void searchList(1)}>
          查询列表
        </button>
        <button type="button" className="rounded-md border border-slate-300 px-4 py-2 text-sm disabled:opacity-50" disabled={busy} onClick={() => void searchDetail()}>
          查看详情
        </button>
        <button type="button" className="rounded-md border border-slate-300 px-4 py-2 text-sm" disabled={busy} onClick={clearInputs}>
          清空条件
        </button>
      </div>

      {err && <div className="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">{err}</div>}

      {cols.length > 0 && (
        <div>
          <div className="mb-2 flex flex-wrap items-center gap-2 text-sm text-slate-600">
            <span>
              本页 {rows.length} 条，共 {total} 条
            </span>
            <span>
              第 {page} / {totalPages} 页
            </span>
          </div>
          <div className="max-h-[28rem] overflow-auto rounded-lg border border-slate-200">
            <table className="min-w-full text-sm">
              <thead className="sticky top-0 bg-slate-50">
                <tr>
                  {cols.map((col) => (
                    <th key={col.key} className="whitespace-nowrap border-b border-slate-200 px-3 py-2 text-left font-medium">
                      {col.label}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {rows.map((row, index) => (
                  <tr key={`${String(row.CODE_TS ?? row.code_ts ?? index)}-${index}`} className="odd:bg-white even:bg-slate-50/50">
                    {cols.map((col) => (
                      <td key={col.key} className="whitespace-nowrap border-b border-slate-100 px-3 py-2 align-top">
                        {String(row[col.key] ?? '')}
                      </td>
                    ))}
                  </tr>
                ))}
                {rows.length === 0 && (
                  <tr>
                    <td className="px-3 py-6 text-center text-slate-500" colSpan={Math.max(cols.length, 1)}>
                      没有匹配条目
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
          <div className="mt-3 flex flex-wrap items-center gap-2 text-sm text-slate-600">
            <button type="button" className="rounded-md border border-slate-300 px-3 py-1.5 disabled:opacity-50" disabled={busy || page <= 1} onClick={() => void goToPage(page - 1)}>
              上一页
            </button>
            <button type="button" className="rounded-md border border-slate-300 px-3 py-1.5 disabled:opacity-50" disabled={busy || page >= totalPages} onClick={() => void goToPage(page + 1)}>
              下一页
            </button>
            <input className="w-20 rounded-md border border-slate-300 px-2 py-1.5" inputMode="numeric" value={pageInput} onChange={(e) => setPageInput(e.target.value.replace(/[^\d]/g, ''))} />
            <button type="button" className="rounded-md bg-slate-900 px-3 py-1.5 text-white disabled:bg-slate-300" disabled={busy} onClick={() => void goToPage(Number(pageInput) || 1)}>
              跳转
            </button>
          </div>
        </div>
      )}

      {detailJson && (
        <div>
          <h4 className="mb-2 text-sm font-medium text-slate-950">条目详情</h4>
          <pre className="max-h-64 overflow-auto rounded-lg bg-slate-900 p-3 text-xs text-slate-100">{detailJson}</pre>
        </div>
      )}
    </div>
  )
}
