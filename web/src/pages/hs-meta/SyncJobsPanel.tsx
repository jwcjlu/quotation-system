import { useState } from 'react'
import { hsSyncJobDetail, hsSyncJobs, hsSyncRun } from '../../api/hsMeta'
import { DEFAULT_PAGE_SIZE } from '../pagination'

export function SyncJobsPanel() {
  const [mode, setMode] = useState<'all_enabled' | 'selected'>('all_enabled')
  const [selectedCsv, setSelectedCsv] = useState('')
  const [jobsJson, setJobsJson] = useState('')
  const [detailJson, setDetailJson] = useState('')
  const [jobId, setJobId] = useState('')
  const [err, setErr] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const runWithBusy = async (fn: () => Promise<void>) => {
    setErr(null)
    setBusy(true)
    try {
      await fn()
    } catch (error) {
      setErr(error instanceof Error ? error.message : String(error))
    } finally {
      setBusy(false)
    }
  }

  const runSync = () =>
    runWithBusy(async () => {
      const core = selectedCsv
        .split(/[\s,]+/)
        .map((value) => value.trim())
        .filter(Boolean)
      const body = mode === 'all_enabled' ? { mode } : { mode, core_hs6: core }
      const result = await hsSyncRun(body)
      setDetailJson(JSON.stringify(result, null, 2))
    })

  const loadJobs = () =>
    runWithBusy(async () => {
      const result = await hsSyncJobs({ page: 1, page_size: DEFAULT_PAGE_SIZE })
      setJobsJson(JSON.stringify(result, null, 2))
    })

  const loadDetail = () =>
    runWithBusy(async () => {
      if (!jobId.trim()) throw new Error('请输入任务 ID')
      const result = await hsSyncJobDetail(jobId.trim())
      setDetailJson(JSON.stringify(result, null, 2))
    })

  return (
    <div className="space-y-5">
      <div className="text-sm text-slate-600">同步全部启用分类，并支持按 HS6 定向执行。</div>
      <div className="rounded-lg border border-slate-200 bg-slate-50 p-4">
        <div className="flex flex-wrap gap-4 text-sm text-slate-700">
          <label className="flex items-center gap-2">
            <input type="radio" checked={mode === 'all_enabled'} onChange={() => setMode('all_enabled')} />
            同步全部启用分类
          </label>
          <label className="flex items-center gap-2">
            <input type="radio" checked={mode === 'selected'} onChange={() => setMode('selected')} />
            只同步指定 HS6
          </label>
        </div>
        {mode === 'selected' && (
          <textarea
            className="mt-3 w-full rounded-md border border-slate-300 p-3 text-sm font-mono"
            rows={3}
            placeholder="854110, 854210"
            value={selectedCsv}
            onChange={(event) => setSelectedCsv(event.target.value)}
          />
        )}
        <div className="mt-4 flex flex-wrap gap-2">
          <button type="button" className="rounded-md bg-slate-900 px-4 py-2 text-sm font-medium text-white disabled:bg-slate-300" disabled={busy} onClick={() => void runSync()}>
            启动同步
          </button>
          <button type="button" className="rounded-md border border-slate-300 px-4 py-2 text-sm disabled:opacity-50" disabled={busy} onClick={() => void loadJobs()}>
            加载任务列表
          </button>
        </div>
      </div>

      <div className="flex flex-wrap items-end gap-2">
        <label className="block text-sm font-medium text-slate-700">
          任务 ID
          <input className="mt-1 w-48 rounded-md border border-slate-300 px-3 py-2" placeholder="job id" value={jobId} onChange={(event) => setJobId(event.target.value)} />
        </label>
        <button type="button" className="rounded-md border border-slate-300 px-4 py-2 text-sm disabled:opacity-50" disabled={busy} onClick={() => void loadDetail()}>
          查看详情
        </button>
      </div>

      {err && <div className="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">{err}</div>}
      {jobsJson && (
        <div>
          <h4 className="mb-2 text-sm font-medium text-slate-950">任务列表</h4>
          <pre className="max-h-48 overflow-auto rounded-lg bg-slate-900 p-3 text-xs text-slate-100">{jobsJson}</pre>
        </div>
      )}
      {detailJson && (
        <div>
          <h4 className="mb-2 text-sm font-medium text-slate-950">任务详情 / 同步结果</h4>
          <pre className="max-h-64 overflow-auto rounded-lg bg-slate-900 p-3 text-xs text-slate-100">{detailJson}</pre>
        </div>
      )}
    </div>
  )
}
