interface SessionImportStatusCardProps {
  status: string
  progress: number
  stage?: string
  message?: string
  errorCode?: string
  error?: string
  updatedAt?: string
}

function clampProgress(value: number): number {
  if (!Number.isFinite(value)) return 0
  return Math.max(0, Math.min(100, Math.round(value)))
}

function statusCopy(status: string): { label: string; tone: string } {
  switch (status) {
    case 'parsing':
      return { label: '导入中', tone: 'text-blue-700 bg-blue-50 border-blue-200' }
    case 'ready':
      return { label: '导入完成', tone: 'text-emerald-700 bg-emerald-50 border-emerald-200' }
    case 'failed':
      return { label: '导入失败', tone: 'text-red-700 bg-red-50 border-red-200' }
    default:
      return { label: status || '未知状态', tone: 'text-slate-700 bg-slate-50 border-slate-200' }
  }
}

export function SessionImportStatusCard({
  status,
  progress,
  stage,
  message,
  errorCode,
  error,
  updatedAt,
}: SessionImportStatusCardProps) {
  if (!status) return null

  const normalizedStatus = status.trim().toLowerCase()
  const pct = clampProgress(progress)
  const copy = statusCopy(normalizedStatus)
  const barClass =
    normalizedStatus === 'failed'
      ? 'bg-red-500'
      : normalizedStatus === 'ready'
        ? 'bg-emerald-500'
        : 'bg-blue-500'

  return (
    <section className="rounded-xl border border-slate-200 bg-white p-6 shadow-sm">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h3 className="font-semibold text-slate-800">BOM 导入进度</h3>
          <p className="mt-1 text-sm text-slate-600">
            LLM 导入已切到后台执行，当前会话会自动刷新直到完成或失败。
          </p>
        </div>
        <span className={`rounded-full border px-3 py-1 text-sm font-medium ${copy.tone}`}>{copy.label}</span>
      </div>

      <div className="mt-4">
        <div className="flex items-center justify-between text-xs text-slate-500">
          <span>{stage ? `阶段：${stage}` : '等待状态更新'}</span>
          <span>{pct}%</span>
        </div>
        <div className="mt-2 h-2 overflow-hidden rounded-full bg-slate-100">
          <div
            role="progressbar"
            aria-label="BOM 导入进度"
            aria-valuemin={0}
            aria-valuemax={100}
            aria-valuenow={pct}
            className={`h-full rounded-full transition-all ${barClass}`}
            style={{ width: `${pct}%` }}
          />
        </div>
      </div>

      {message && <p className="mt-3 text-sm text-slate-700">{message}</p>}

      {normalizedStatus === 'failed' && (
        <div className="mt-3 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-900">
          {errorCode && (
            <p>
              错误码 <code className="rounded bg-white/80 px-1 py-0.5 text-xs">{errorCode}</code>
            </p>
          )}
          {error && <p className={errorCode ? 'mt-1' : ''}>{error}</p>}
        </div>
      )}

      {updatedAt && (
        <p className="mt-3 text-xs text-slate-500">
          最近更新 <time dateTime={updatedAt}>{updatedAt}</time>
        </p>
      )}
    </section>
  )
}
