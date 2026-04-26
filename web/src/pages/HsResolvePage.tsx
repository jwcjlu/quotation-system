import { useEffect, useMemo, useState } from 'react'
import { hsResolveByModel, type HsResolveCandidate, type HsResolveReply } from '../api'

export interface HsResolvePrefill {
  key: number
  model: string
  manufacturer: string
}

function emptyReply(): HsResolveReply {
  return {
    accepted: false,
    task_id: '',
    run_id: '',
    decision_mode: '',
    task_status: '',
    result_status: '',
    best_code_ts: '',
    best_score: 0,
    candidates: [],
    error_code: '',
    error_message: '',
  }
}

function CandidateTable({ candidates }: { candidates: HsResolveCandidate[] }) {
  if (candidates.length === 0) return null
  return (
    <div className="overflow-hidden rounded-lg border border-slate-200">
      <table className="min-w-full text-sm">
        <thead className="bg-slate-50 text-left">
          <tr>
            <th className="px-3 py-2 font-medium text-slate-600">排名</th>
            <th className="px-3 py-2 font-medium text-slate-600">Code TS</th>
            <th className="px-3 py-2 font-medium text-slate-600">分数</th>
            <th className="px-3 py-2 font-medium text-slate-600">原因</th>
          </tr>
        </thead>
        <tbody>
          {candidates.map((candidate) => (
            <tr key={`${candidate.candidate_rank}-${candidate.code_ts}`} className="border-t border-slate-100">
              <td className="px-3 py-2">{candidate.candidate_rank}</td>
              <td className="px-3 py-2 font-mono">{candidate.code_ts}</td>
              <td className="px-3 py-2">{candidate.score.toFixed(2)}</td>
              <td className="px-3 py-2 text-slate-600">{candidate.reason || '-'}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

export function HsResolvePage({ prefill }: { prefill?: HsResolvePrefill | null }) {
  const [model, setModel] = useState(prefill?.model ?? '')
  const [manufacturer, setManufacturer] = useState(prefill?.manufacturer ?? '')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [reply, setReply] = useState<HsResolveReply>(emptyReply)

  useEffect(() => {
    setModel(prefill?.model ?? '')
    setManufacturer(prefill?.manufacturer ?? '')
  }, [prefill?.key, prefill?.manufacturer, prefill?.model])

  const summary = useMemo(() => {
    if (!reply.best_code_ts) return null
    return `推荐编码 ${reply.best_code_ts}，置信度 ${reply.best_score.toFixed(2)}`
  }, [reply.best_code_ts, reply.best_score])

  const submit = async () => {
    if (!model.trim()) {
      setError('请输入型号')
      return
    }
    setBusy(true)
    setError(null)
    try {
      const result = await hsResolveByModel({
        model: model.trim(),
        manufacturer: manufacturer.trim(),
      })
      setReply(result)
    } catch (err) {
      setReply(emptyReply())
      setError(err instanceof Error ? err.message : '解析失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <section className="space-y-5 rounded-xl border border-slate-200 bg-white p-6 shadow-sm">
      <div>
        <h2 className="text-xl font-semibold text-slate-800">HS型号解析</h2>
        <p className="mt-2 text-sm text-slate-600">输入型号和厂牌，查看推荐编码、候选结果和解析状态。</p>
      </div>

      <div className="grid gap-4 md:grid-cols-2">
        <label className="block text-sm font-medium text-slate-700">
          型号
          <input
            className="mt-1 w-full rounded-md border border-slate-300 px-3 py-2"
            value={model}
            onChange={(event) => setModel(event.target.value)}
          />
        </label>
        <label className="block text-sm font-medium text-slate-700">
          厂牌
          <input
            className="mt-1 w-full rounded-md border border-slate-300 px-3 py-2"
            value={manufacturer}
            onChange={(event) => setManufacturer(event.target.value)}
          />
        </label>
      </div>

      <div className="flex flex-wrap gap-2">
        <button
          type="button"
          data-testid="hs-resolve-submit"
          className="rounded-md bg-slate-900 px-4 py-2 text-sm font-medium text-white disabled:bg-slate-300"
          disabled={busy}
          onClick={() => void submit()}
        >
          {busy ? '解析中...' : '开始解析'}
        </button>
      </div>

      {error && <div className="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">{error}</div>}

      {(summary || reply.task_status || reply.result_status) && (
        <div className="rounded-lg border border-slate-200 bg-slate-50 p-4 text-sm text-slate-700">
          {summary && <div className="font-medium text-slate-900">{summary}</div>}
          <div className="mt-2 flex flex-wrap gap-4 text-xs text-slate-500">
            <span>任务状态：{reply.task_status || '-'}</span>
            <span>结果状态：{reply.result_status || '-'}</span>
            <span>运行ID：{reply.run_id || '-'}</span>
          </div>
          {reply.error_message && <div className="mt-2 text-red-600">{reply.error_message}</div>}
        </div>
      )}

      <CandidateTable candidates={reply.candidates} />
    </section>
  )
}
