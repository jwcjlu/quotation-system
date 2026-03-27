import { useCallback, useState } from 'react'
import {
  listAgentInstalledScripts,
  listAgentLeasedTasks,
  listAgents,
  type AgentSummary,
  type InstalledScriptRow,
  type LeasedTaskRow,
} from '../api/agentAdmin'

const STORAGE_KEY = 'caichip_web_agent_admin_api_key'

function formatTs(s: string | undefined): string {
  if (!s) return '—'
  const d = Date.parse(s)
  if (Number.isNaN(d)) return s
  return new Date(d).toLocaleString()
}

export function AgentAdminPage() {
  const [apiKey, setApiKey] = useState(() => localStorage.getItem(STORAGE_KEY) ?? '')
  const [agents, setAgents] = useState<AgentSummary[]>([])
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [leased, setLeased] = useState<LeasedTaskRow[]>([])
  const [scripts, setScripts] = useState<InstalledScriptRow[]>([])
  const [loading, setLoading] = useState(false)
  const [detailLoading, setDetailLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [info, setInfo] = useState<string | null>(null)

  const persistKey = useCallback((k: string) => {
    setApiKey(k)
    if (k.trim()) localStorage.setItem(STORAGE_KEY, k.trim())
    else localStorage.removeItem(STORAGE_KEY)
  }, [])

  const requireKey = (): string | null => {
    const k = apiKey.trim()
    if (!k) {
      setError('请先填写运维 API Key（与 configs 中 agent_admin.api_keys 一致）')
      return null
    }
    return k
  }

  const resetFlash = () => {
    setError(null)
    setInfo(null)
  }

  const loadAgents = async () => {
    const k = requireKey()
    if (!k) return
    setLoading(true)
    resetFlash()
    try {
      const r = await listAgents(k)
      setAgents(r.agents ?? [])
      setInfo(`已加载 ${r.agents?.length ?? 0} 个 Agent`)
      if (selectedId && !(r.agents ?? []).some((a) => a.agentId === selectedId)) {
        setSelectedId(null)
        setLeased([])
        setScripts([])
      }
    } catch (e) {
      setAgents([])
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setLoading(false)
    }
  }

  const loadDetail = async (agentId: string) => {
    const k = requireKey()
    if (!k) return
    setDetailLoading(true)
    resetFlash()
    try {
      const [t, s] = await Promise.all([
        listAgentLeasedTasks(k, agentId),
        listAgentInstalledScripts(k, agentId),
      ])
      setLeased(t.tasks ?? [])
      setScripts(s.scripts ?? [])
      setSelectedId(agentId)
      setInfo(`已加载 ${agentId} 的租约任务与已装脚本`)
    } catch (e) {
      setLeased([])
      setScripts([])
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setDetailLoading(false)
    }
  }

  const inputCls =
    'w-full border border-slate-300 rounded px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-slate-400'

  return (
    <div className="space-y-8">
      <div>
        <h2 className="text-lg font-semibold text-slate-800">Agent 运维</h2>
        <p className="text-sm text-slate-600 mt-1">
          对应后端 <code className="bg-slate-100 px-1 rounded">/api/v1/admin/agents/*</code>，需配置{' '}
          <code className="bg-slate-100 px-1 rounded">agent_admin.api_keys</code>（与脚本包管理的{' '}
          <code className="bg-slate-100 px-1 rounded">script_admin</code> 密钥独立）。开发时 Vite 代理{' '}
          <code className="bg-slate-100 px-1 rounded">/api</code> → 后端。
        </p>
      </div>

      <section className="bg-white border border-slate-200 rounded-lg p-5 shadow-sm">
        <h3 className="font-medium text-slate-800 mb-3">鉴权</h3>
        <label className="block text-sm text-slate-600 mb-1">运维 API Key（agent_admin）</label>
        <input
          type="password"
          autoComplete="off"
          className={inputCls}
          placeholder="与 configs 中 agent_admin.api_keys 某项一致"
          value={apiKey}
          onChange={(e) => persistKey(e.target.value)}
        />
        <button
          type="button"
          disabled={loading}
          onClick={loadAgents}
          className="mt-3 px-4 py-2 bg-slate-800 text-white text-sm rounded hover:bg-slate-700 disabled:opacity-50"
        >
          刷新 Agent 列表
        </button>
      </section>

      {(error || info) && (
        <div
          className={`rounded-lg px-4 py-3 text-sm ${
            error ? 'bg-red-50 text-red-800 border border-red-200' : 'bg-emerald-50 text-emerald-900 border border-emerald-200'
          }`}
        >
          {error ?? info}
        </div>
      )}

      <section className="bg-white border border-slate-200 rounded-lg p-5 shadow-sm">
        <h3 className="font-medium text-slate-800 mb-3">Agent 列表</h3>
        <p className="text-xs text-slate-500 mb-3">
          「在线」按服务端与采集 Agent 相同规则：距最近任务心跳时间未超过离线窗口（默认约 120s 与心跳倍数取大）。
        </p>
        <div className="overflow-x-auto">
          <table className="min-w-full text-sm">
            <thead>
              <tr className="border-b border-slate-200 text-left text-slate-600">
                <th className="py-2 pr-4">状态</th>
                <th className="py-2 pr-4">agent_id</th>
                <th className="py-2 pr-4">queue</th>
                <th className="py-2 pr-4">hostname</th>
                <th className="py-2 pr-4">最近任务心跳</th>
                <th className="py-2">操作</th>
              </tr>
            </thead>
            <tbody>
              {agents.length === 0 ? (
                <tr>
                  <td colSpan={6} className="py-6 text-slate-500 text-center">
                    暂无数据，填写 Key 后点击「刷新 Agent 列表」
                  </td>
                </tr>
              ) : (
                agents.map((a) => (
                  <tr
                    key={a.agentId}
                    className={`border-b border-slate-100 ${selectedId === a.agentId ? 'bg-slate-50' : ''}`}
                  >
                    <td className="py-2 pr-4">
                      <span
                        className={`inline-block px-2 py-0.5 rounded text-xs font-medium ${
                          a.online ? 'bg-emerald-100 text-emerald-800' : 'bg-slate-200 text-slate-700'
                        }`}
                      >
                        {a.online ? '在线' : '离线'}
                      </span>
                    </td>
                    <td className="py-2 pr-4 font-mono text-xs break-all max-w-[14rem]">{a.agentId}</td>
                    <td className="py-2 pr-4">{a.queue || '—'}</td>
                    <td className="py-2 pr-4">{a.hostname || '—'}</td>
                    <td className="py-2 pr-4 text-slate-600 whitespace-nowrap">
                      {formatTs(a.lastTaskHeartbeatAt)}
                    </td>
                    <td className="py-2">
                      <button
                        type="button"
                        disabled={detailLoading}
                        onClick={() => loadDetail(a.agentId)}
                        className="px-2 py-1 text-xs border border-slate-300 rounded hover:bg-slate-50 disabled:opacity-50"
                      >
                        租约任务 / 已装脚本
                      </button>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </section>

      {selectedId && (
        <section className="bg-white border border-slate-200 rounded-lg p-5 shadow-sm space-y-6">
          <h3 className="font-medium text-slate-800">
            详情：<span className="font-mono text-sm">{selectedId}</span>
            {detailLoading && <span className="ml-2 text-slate-500 text-sm">加载中…</span>}
          </h3>

          <div>
            <h4 className="text-sm font-medium text-slate-700 mb-2">当前租约中的调度任务</h4>
            <div className="overflow-x-auto">
              <table className="min-w-full text-sm">
                <thead>
                  <tr className="border-b border-slate-200 text-left text-slate-600">
                    <th className="py-2 pr-4">task_id</th>
                    <th className="py-2 pr-4">script_id</th>
                    <th className="py-2 pr-4">version</th>
                    <th className="py-2 pr-4">leased_at</th>
                    <th className="py-2">lease_deadline</th>
                  </tr>
                </thead>
                <tbody>
                  {leased.length === 0 ? (
                    <tr>
                      <td colSpan={5} className="py-4 text-slate-500 text-center">
                        无租约中任务
                      </td>
                    </tr>
                  ) : (
                    leased.map((t) => (
                      <tr key={t.taskId} className="border-b border-slate-100">
                        <td className="py-2 pr-4 font-mono text-xs break-all max-w-[10rem]">{t.taskId}</td>
                        <td className="py-2 pr-4">{t.scriptId}</td>
                        <td className="py-2 pr-4">{t.version}</td>
                        <td className="py-2 pr-4 whitespace-nowrap">{formatTs(t.leasedAt)}</td>
                        <td className="py-2 whitespace-nowrap">{formatTs(t.leaseDeadlineAt)}</td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>
          </div>

          <div>
            <h4 className="text-sm font-medium text-slate-700 mb-2">已安装脚本（心跳上报快照）</h4>
            <div className="overflow-x-auto">
              <table className="min-w-full text-sm">
                <thead>
                  <tr className="border-b border-slate-200 text-left text-slate-600">
                    <th className="py-2 pr-4">script_id</th>
                    <th className="py-2 pr-4">version</th>
                    <th className="py-2 pr-4">env_status</th>
                    <th className="py-2">updated_at</th>
                  </tr>
                </thead>
                <tbody>
                  {scripts.length === 0 ? (
                    <tr>
                      <td colSpan={4} className="py-4 text-slate-500 text-center">
                        无记录（该 Agent 尚未上报或库中无行）
                      </td>
                    </tr>
                  ) : (
                    scripts.map((s) => (
                      <tr key={s.scriptId} className="border-b border-slate-100">
                        <td className="py-2 pr-4 font-mono text-xs">{s.scriptId}</td>
                        <td className="py-2 pr-4">{s.version}</td>
                        <td className="py-2 pr-4">{s.envStatus}</td>
                        <td className="py-2 whitespace-nowrap">{formatTs(s.updatedAt)}</td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>
          </div>
        </section>
      )}
    </div>
  )
}
