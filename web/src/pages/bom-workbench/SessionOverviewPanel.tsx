interface SessionOverviewPanelProps {
  sessionId: string
  sessionName: string
  sessionStatus: string
}

export function SessionOverviewPanel({
  sessionId,
  sessionName,
  sessionStatus,
}: SessionOverviewPanelProps) {
  return (
    <section className="space-y-4" data-testid="session-overview-panel">
      <div className="rounded-lg border border-slate-200 bg-white p-5">
        <div className="text-sm font-medium text-slate-500">会话总览</div>
        <h4 className="mt-1 text-xl font-semibold text-slate-900">{sessionName || sessionId}</h4>
        <div className="mt-3 flex flex-wrap gap-2 text-sm">
          <span className="rounded-full bg-slate-100 px-3 py-1 font-mono text-slate-700">
            {sessionId}
          </span>
          <span className="rounded-full bg-blue-50 px-3 py-1 font-mono text-blue-700">
            {sessionStatus || 'unknown'}
          </span>
        </div>
      </div>
      <div className="grid gap-3 md:grid-cols-3">
        {[
          ['BOM 行', '查看明细、可用性和行级处理'],
          ['搜索清洗', '检查平台搜索任务和厂家别名'],
          ['缺口处理', '处理无报价、无库存和替代料'],
        ].map(([title, desc]) => (
          <div key={title} className="rounded-lg border border-slate-200 bg-white p-4">
            <div className="font-medium text-slate-800">{title}</div>
            <p className="mt-1 text-sm text-slate-500">{desc}</p>
          </div>
        ))}
      </div>
    </section>
  )
}
