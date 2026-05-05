import { useEffect, useMemo, useState } from 'react'
import { autoMatch, hsBatchResolveByModels, matchItemNeedsHsResolve, type MatchItem } from '../../api'
import { SessionLineDemandQuoteItemsModal } from './SessionLineDemandQuoteItemsModal'
import {
  DEFAULT_PAGE_SIZE,
  PAGE_SIZE_OPTIONS,
  type PageSize,
  pageSummary,
  paginateRows,
  textMatchesKeyword,
} from './sessionPanelUtils'

interface SessionMatchResultPanelProps {
  bomId: string
  onNavigateToHsResolve?: (model: string, manufacturer: string) => void
}

function displayValue(value: number | string | null | undefined): string {
  if (value === null || value === undefined || value === '') return '-'
  return String(value)
}

function needsHsResolveAction(item: MatchItem): boolean {
  const codeTS = (item.code_ts || '').trim()
  if (codeTS) return false
  const hsStatus = (item.hs_code_status || '').trim()
  if (hsStatus === 'hs_found') return false
  return true
}

export function SessionMatchResultPanel({
  bomId,
  onNavigateToHsResolve,
}: SessionMatchResultPanelProps) {
  const [items, setItems] = useState<MatchItem[]>([])
  const [keyword, setKeyword] = useState('')
  const [status, setStatus] = useState('')
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState<PageSize>(DEFAULT_PAGE_SIZE)
  const [lineDetailOpen, setLineDetailOpen] = useState(false)
  const [lineDetailLineNo, setLineDetailLineNo] = useState<number | null>(null)
  const [batchBusy, setBatchBusy] = useState(false)
  const [batchMessage, setBatchMessage] = useState<string | null>(null)
  const [refreshing, setRefreshing] = useState(false)

  const loadMatchResult = async () => {
    try {
      const reply = await autoMatch(bomId)
      setItems(reply.items)
    } catch {
      setItems([])
    }
  }

  useEffect(() => {
    void loadMatchResult()
  }, [bomId])

  const filtered = useMemo(
    () =>
      items.filter((item) => {
        if (status && item.match_status !== status) return false
        return textMatchesKeyword(
          [
            item.index,
            item.model,
            item.matched_model,
            item.manufacturer,
            item.platform,
            item.demand_manufacturer,
          ],
          keyword
        )
      }),
    [items, keyword, status]
  )
  const paged = paginateRows(filtered, page, pageSize)
  const matchedCount = items.filter((item) => item.match_status === 'exact').length
  const noMatchCount = items.filter((item) => item.match_status === 'no_match').length
  const totalAmount = items.reduce((sum, item) => sum + (Number(item.subtotal) || 0), 0)

  useEffect(() => {
    setPage(1)
  }, [keyword, status, pageSize])

  const handleBatchResolveHs = async () => {
    const lines = items
      .filter((item) => item.match_status === 'exact' && matchItemNeedsHsResolve(item))
      .map((item) => ({
        line_no: item.index,
        model: (item.model || '').trim(),
        manufacturer: (item.demand_manufacturer || '').trim(),
        match_status: item.match_status || '',
        hs_code_status: item.hs_code_status || '',
      }))
      .filter((item) => item.model)
    if (lines.length === 0) {
      setBatchMessage('当前没有“已匹配且缺少HS”的行可触发。')
      return
    }
    setBatchBusy(true)
    setBatchMessage(null)
    try {
      const reply = await hsBatchResolveByModels({
        session_id: bomId,
        request_id: `batch-${Date.now()}`,
        lines,
      })
      setBatchMessage(
        `已提交：成功 ${reply.accepted_count}，跳过 ${reply.skipped_count}，失败 ${reply.failed_count}`
      )
    } catch (error) {
      setBatchMessage(error instanceof Error ? error.message : '批量触发 HS 解析失败')
    } finally {
      setBatchBusy(false)
    }
  }

  const refreshAndFilter = async (nextStatus: string) => {
    setRefreshing(true)
    await loadMatchResult()
    setStatus(nextStatus)
    setRefreshing(false)
  }

  return (
    <section className="space-y-4" data-testid="session-match-result-panel">
      <div className="grid gap-4 xl:grid-cols-4">
        <div className="rounded-lg border border-[#d7e0ed] bg-white p-4">
          <div className="text-sm font-bold text-slate-950">匹配状态</div>
          <div className="mt-4 text-3xl font-bold text-[#12805c]">
            {noMatchCount === 0 ? 'ready' : 'review'}
          </div>
        </div>
        <button
          type="button"
          onClick={() => void refreshAndFilter('exact')}
          disabled={refreshing}
          className="rounded-lg border border-[#d7e0ed] bg-white p-4 text-left hover:border-[#8aa7d8] disabled:opacity-60"
        >
          <div className="text-sm font-bold text-slate-950">已匹配行</div>
          <div className="mt-4 text-3xl font-bold text-[#2457c5]">{matchedCount}</div>
        </button>
        <button
          type="button"
          onClick={() => void refreshAndFilter('no_match')}
          disabled={refreshing}
          className="rounded-lg border border-[#d7e0ed] bg-white p-4 text-left hover:border-[#d7a063] disabled:opacity-60"
        >
          <div className="text-sm font-bold text-slate-950">无匹配</div>
          <div className="mt-4 text-3xl font-bold text-[#a76505]">{noMatchCount}</div>
        </button>
        <div className="rounded-lg border border-[#d7e0ed] bg-white p-4">
          <div className="text-sm font-bold text-slate-950">总金额</div>
          <div className="mt-4 text-2xl font-bold text-slate-950">
            {totalAmount ? `¥${totalAmount.toLocaleString()}` : '-'}
          </div>
        </div>
      </div>

      <div className="rounded-lg border border-[#d7e0ed] bg-white p-3">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h4 className="font-semibold text-slate-900">匹配结果</h4>
            <p className="mt-1 text-sm text-slate-500">双击行可查看该行原始需求与 t_bom_quote_item 明细</p>
          </div>
          <div className="text-sm text-slate-500">{pageSummary(paged.page, paged.totalPages, paged.total)}</div>
        </div>
        <div className="mt-3 flex flex-wrap items-center gap-2">
          <button
            type="button"
            onClick={() => void handleBatchResolveHs()}
            disabled={batchBusy}
            className="rounded-md border border-blue-300 bg-blue-50 px-3 py-1.5 text-sm font-medium text-blue-800 hover:bg-blue-100 disabled:opacity-50"
          >
            {batchBusy ? '批量提交中...' : '一键解析HS（已匹配未填HS）'}
          </button>
          {batchMessage ? <span className="text-sm text-slate-600">{batchMessage}</span> : null}
        </div>
        <div className="mt-4 grid gap-2 md:grid-cols-[minmax(0,1fr)_10rem_8rem]">
          <input
            value={keyword}
            onChange={(event) => setKeyword(event.target.value)}
            placeholder="MPN / 供应商 / 厂家"
            className="h-8 rounded-md border border-[#d7e0ed] px-3 text-sm"
          />
          <select
            value={status}
            onChange={(event) => setStatus(event.target.value)}
            className="h-8 rounded-md border border-[#d7e0ed] px-3 text-sm"
          >
            <option value="">全部状态</option>
            <option value="exact">精确匹配</option>
            <option value="pending">待确认</option>
            <option value="no_match">无匹配</option>
          </select>
          <select
            value={pageSize}
            onChange={(event) => setPageSize(Number(event.target.value) as PageSize)}
            className="h-8 rounded-md border border-[#d7e0ed] px-3 text-sm"
          >
            {PAGE_SIZE_OPTIONS.map((size) => (
              <option key={size} value={size}>
                每页 {size}
              </option>
            ))}
          </select>
        </div>
      </div>
      <div className="overflow-x-auto rounded-lg border border-[#d7e0ed] bg-white">
        <table className="w-full min-w-[1100px] text-sm">
          <thead className="bg-[#f1f5f9] text-left text-slate-700">
            <tr>
              <th className="px-3 py-2">行号</th>
              <th className="px-3 py-2">需求型号</th>
              <th className="px-3 py-2">匹配型号</th>
              <th className="px-3 py-2">供应商</th>
              <th className="px-3 py-2">库存</th>
              <th className="px-3 py-2">单价</th>
              <th className="px-3 py-2">商检</th>
              <th className="px-3 py-2">进口税率</th>
              <th className="px-3 py-2">最惠国税率</th>
              <th className="px-3 py-2">状态</th>
              <th className="px-3 py-2">操作</th>
            </tr>
          </thead>
          <tbody>
            {paged.rows.length === 0 ? (
              <tr>
                <td colSpan={11} className="px-3 py-8 text-center text-slate-500">
                  暂无匹配结果
                </td>
              </tr>
            ) : (
              paged.rows.map((item) => (
                <tr
                  key={`${item.index}-${item.model}-${item.platform}`}
                  className="cursor-pointer border-t border-[#d9e1ec] hover:bg-slate-50"
                  onDoubleClick={() => {
                    setLineDetailLineNo(item.index)
                    setLineDetailOpen(true)
                  }}
                  title="双击查看该行原始需求与报价子表明细"
                >
                  <td className="px-3 py-2">{item.index}</td>
                  <td className="px-3 py-2 font-mono">{item.model}</td>
                  <td className="px-3 py-2 font-mono">{item.matched_model || '-'}</td>
                  <td className="px-3 py-2">{item.platform || item.manufacturer || '-'}</td>
                  <td className="px-3 py-2">{item.stock}</td>
                  <td className="px-3 py-2">{item.unit_price}</td>
                  <td className="px-3 py-2">{displayValue(item.control_mark)}</td>
                  <td className="px-3 py-2">{displayValue(item.import_tax_imp_ordinary_rate)}</td>
                  <td className="px-3 py-2">{displayValue(item.import_tax_imp_discount_rate)}</td>
                  <td className="px-3 py-2">{item.match_status || '-'}</td>
                  <td className="px-3 py-2">
                    {needsHsResolveAction(item) ? (
                      <button
                        type="button"
                        onClick={(event) => {
                          event.stopPropagation()
                          onNavigateToHsResolve?.(item.model, item.manufacturer)
                        }}
                        className="text-sm font-medium text-blue-600 hover:underline"
                      >
                        解析HS
                      </button>
                    ) : null}
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
      <div className="flex justify-end gap-2">
        <button
          type="button"
          disabled={paged.page <= 1}
          onClick={() => setPage((value) => value - 1)}
          className="rounded-md border border-[#d7e0ed] px-4 py-1.5 text-sm disabled:opacity-40"
        >
          上一页
        </button>
        <button
          type="button"
          disabled={paged.page >= paged.totalPages}
          onClick={() => setPage((value) => value + 1)}
          className="rounded-md border border-[#d7e0ed] px-4 py-1.5 text-sm disabled:opacity-40"
        >
          下一页
        </button>
      </div>
      {lineDetailOpen && lineDetailLineNo != null ? (
        <SessionLineDemandQuoteItemsModal
          bomId={bomId}
          lineNo={lineDetailLineNo}
          onClose={() => {
            setLineDetailOpen(false)
            setLineDetailLineNo(null)
          }}
        />
      ) : null}
    </section>
  )
}
