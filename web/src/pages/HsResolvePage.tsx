import { useEffect, useMemo, useState } from 'react'
import {
  hsResolveByModel,
  uploadHsManualDatasheet,
  type HsResolveCandidate,
  type HsResolveReply,
  type UploadHsManualDatasheetReply,
} from '../api'
import { ToolPageShell } from './ToolPageShell'

export interface HsResolvePrefill {
  key: number
  model: string
  manufacturer: string
}

interface ManualUploadState extends UploadHsManualDatasheetReply {
  file_name: string
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

function makeTraceID(): string {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) {
    return crypto.randomUUID()
  }
  return `hs-${Date.now()}-${Math.random().toString(16).slice(2)}`
}

function formatUnixTime(value: number): string {
  if (!value) return '-'
  return new Date(value * 1000).toLocaleString()
}

function CandidateTable({ candidates }: { candidates: HsResolveCandidate[] }) {
  if (candidates.length === 0) return null
  return (
    <div className="overflow-hidden rounded-lg border border-[#d7e0ed] bg-white">
      <table className="min-w-full text-sm">
        <thead className="bg-[#f4f6fa] text-left">
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
  const [manualDescription, setManualDescription] = useState('')
  const [manualUpload, setManualUpload] = useState<ManualUploadState | null>(null)
  const [uploadBusy, setUploadBusy] = useState(false)
  const [uploadError, setUploadError] = useState<string | null>(null)
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

  const handleManualUpload = async (file: File | undefined) => {
    if (!file) return
    setUploadBusy(true)
    setUploadError(null)
    try {
      const result = await uploadHsManualDatasheet(file)
      setManualUpload({ ...result, file_name: file.name || 'manual.pdf' })
    } catch (err) {
      setManualUpload(null)
      setUploadError(err instanceof Error ? err.message : '上传失败')
    } finally {
      setUploadBusy(false)
    }
  }

  const submit = async () => {
    const trimmedModel = model.trim()
    const trimmedManufacturer = manufacturer.trim()
    const trimmedManualDescription = manualDescription.trim()
    if (!trimmedModel) {
      setError('请输入型号')
      return
    }
    setBusy(true)
    setError(null)
    try {
      const result = await hsResolveByModel({
        model: trimmedModel,
        manufacturer: trimmedManufacturer,
        request_trace_id: makeTraceID(),
        manual_component_description: trimmedManualDescription || undefined,
        manual_upload_id: manualUpload?.upload_id,
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
    <ToolPageShell
      testId="hs-resolve-page"
      eyebrow="HS RESOLVE"
      title="HS型号解析"
      description="输入型号和厂牌，查看推荐编码、候选结果和解析状态。"
    >
      <section className="rounded-lg border border-[#d7e0ed] bg-white p-4">
        <div className="grid gap-4 md:grid-cols-2">
          <label className="block text-sm font-medium text-slate-700">
            型号
            <input
              className="mt-1 w-full rounded-md border border-[#d7e0ed] px-3 py-2 focus:outline-none focus:ring-2 focus:ring-[#244a86]/30"
              value={model}
              onChange={(event) => setModel(event.target.value)}
            />
          </label>
          <label className="block text-sm font-medium text-slate-700">
            厂牌
            <input
              className="mt-1 w-full rounded-md border border-[#d7e0ed] px-3 py-2 focus:outline-none focus:ring-2 focus:ring-[#244a86]/30"
              value={manufacturer}
              onChange={(event) => setManufacturer(event.target.value)}
            />
          </label>
        </div>

        <div className="mt-4 grid gap-4 lg:grid-cols-[minmax(0,1fr)_20rem]">
          <label className="block text-sm font-medium text-slate-700">
            手动描述
            <textarea
              className="mt-1 min-h-28 w-full resize-y rounded-md border border-[#d7e0ed] px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-[#244a86]/30"
              value={manualDescription}
              onChange={(event) => setManualDescription(event.target.value)}
              placeholder="例如：16Mbit SPI NOR Flash，SOIC-8 封装，工作电压 2.7V-3.6V"
            />
          </label>

          <div className="rounded-lg border border-[#d7e0ed] bg-[#f8fafc] p-3">
            <label className="block text-sm font-medium text-slate-700">
              上传 PDF 手册
              <input
                aria-label="上传 PDF 手册"
                type="file"
                accept="application/pdf,.pdf"
                className="mt-2 block w-full text-sm text-slate-600 file:mr-3 file:rounded-md file:border-0 file:bg-[#e8eef7] file:px-3 file:py-2 file:text-sm file:font-semibold file:text-[#244a86] hover:file:bg-[#dce7f5]"
                disabled={uploadBusy}
                onChange={(event) => void handleManualUpload(event.currentTarget.files?.[0])}
              />
            </label>
            {uploadBusy && <div className="mt-3 text-sm text-slate-500">上传中...</div>}
            {uploadError && <div className="mt-3 text-sm text-red-600">{uploadError}</div>}
            {manualUpload && (
              <div className="mt-3 rounded-md border border-[#d7e0ed] bg-white px-3 py-2 text-sm text-slate-600">
                <div className="font-medium text-slate-900">已上传：{manualUpload.file_name}</div>
                <div className="mt-1">过期时间：{formatUnixTime(manualUpload.expires_at_unix)}</div>
              </div>
            )}
          </div>
        </div>

        <div className="mt-4 flex flex-wrap gap-2">
          <button
            type="button"
            data-testid="hs-resolve-submit"
            className="rounded-md bg-[#e8eef7] px-4 py-2 text-sm font-semibold text-[#244a86] hover:bg-[#dce7f5] disabled:bg-slate-200 disabled:text-slate-400"
            disabled={busy || uploadBusy}
            onClick={() => void submit()}
          >
            {busy ? '解析中...' : '开始解析'}
          </button>
        </div>
      </section>

      {error && <div className="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">{error}</div>}

      {(summary || reply.task_status || reply.result_status) && (
        <div className="rounded-lg border border-[#d7e0ed] bg-white p-4 text-sm text-slate-700">
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
    </ToolPageShell>
  )
}
