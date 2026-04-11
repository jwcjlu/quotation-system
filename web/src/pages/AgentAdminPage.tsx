import { useCallback, useMemo, useState } from 'react'
import {
  deleteAgentScriptAuth,
  listAgentInstalledScripts,
  listAgentLeasedTasks,
  listAgents,
  listAgentScriptAuths,
  upsertAgentScriptAuth,
  type AgentScriptAuthRow,
  type AgentSummary,
  type InstalledScriptRow,
  type LeasedTaskRow,
} from '../api/agentAdmin'
import { BomPlatformsAdminSection } from './BomPlatformsAdminSection'

const STORAGE_KEY = 'caichip_web_agent_admin_api_key'

function formatTs(s: string | undefined): string {
  if (!s) return '—'
  const d = Date.parse(s)
  if (Number.isNaN(d)) return s
  return new Date(d).toLocaleString()
}

function agentStatusLabel(status: string | undefined, online: boolean): { text: string; cls: string } {
  const s = (status ?? '').toLowerCase()
  if (s === 'online' || (online && !status)) return { text: '在线', cls: 'bg-emerald-100 text-emerald-800' }
  if (s === 'unknown') return { text: '未知', cls: 'bg-amber-100 text-amber-900' }
  return { text: '离线', cls: 'bg-slate-200 text-slate-700' }
}

export function AgentAdminPage() {
  const [apiKey, setApiKey] = useState(() => localStorage.getItem(STORAGE_KEY) ?? '')
  const [agents, setAgents] = useState<AgentSummary[]>([])
  const [offlineWindowSec, setOfflineWindowSec] = useState<number | null>(null)
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [leased, setLeased] = useState<LeasedTaskRow[]>([])
  const [scripts, setScripts] = useState<InstalledScriptRow[]>([])
  const [scriptAuths, setScriptAuths] = useState<AgentScriptAuthRow[]>([])
  const [formScriptId, setFormScriptId] = useState('')
  const [formUsername, setFormUsername] = useState('')
  const [formPassword, setFormPassword] = useState('')
  const [authSaving, setAuthSaving] = useState(false)
  const [loading, setLoading] = useState(false)
  const [detailLoading, setDetailLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [info, setInfo] = useState<string | null>(null)
  const [adminTab, setAdminTab] = useState<'bom-platforms' | 'agents'>('bom-platforms')

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
      const winSec =
        typeof r.offlineWindowSec === 'number'
          ? r.offlineWindowSec
          : typeof r.offline_window_sec === 'number'
            ? r.offline_window_sec
            : null
      setOfflineWindowSec(winSec)
      setInfo(`已加载 ${r.agents?.length ?? 0} 个 Agent`)
      if (selectedId && !(r.agents ?? []).some((a) => a.agentId === selectedId)) {
        setSelectedId(null)
        setLeased([])
        setScripts([])
        setScriptAuths([])
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
      const [t, s, a] = await Promise.all([
        listAgentLeasedTasks(k, agentId),
        listAgentInstalledScripts(k, agentId),
        listAgentScriptAuths(k, agentId),
      ])
      setLeased(t.tasks ?? [])
      setScripts(s.scripts ?? [])
      setScriptAuths(a.rows ?? [])
      setSelectedId(agentId)
      setInfo(`已加载 ${agentId} 的租约任务、已装脚本与平台凭据`)
    } catch (e) {
      setLeased([])
      setScripts([])
      setScriptAuths([])
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setDetailLoading(false)
    }
  }

  const reloadScriptAuthsOnly = async (agentId: string) => {
    const k = requireKey()
    if (!k) return
    try {
      const a = await listAgentScriptAuths(k, agentId)
      setScriptAuths(a.rows ?? [])
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    }
  }

  const saveScriptAuth = async () => {
    const k = requireKey()
    if (!k || !selectedId) return
    const sid = formScriptId.trim()
    const user = formUsername.trim()
    const pw = formPassword
    if (!sid || !user || !pw) {
      setError('请从下拉选择 script_id，并填写用户名与密码（后端 Upsert 要求四项齐全）')
      return
    }
    setAuthSaving(true)
    resetFlash()
    try {
      await upsertAgentScriptAuth(k, {
        agentId: selectedId,
        scriptId: sid,
        username: user,
        password: pw,
      })
      setFormPassword('')
      setInfo('凭据已保存')
      await reloadScriptAuthsOnly(selectedId)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setAuthSaving(false)
    }
  }

  const removeScriptAuth = async (scriptId: string) => {
    const k = requireKey()
    if (!k || !selectedId) return
    if (!window.confirm(`删除该 Agent 在「${scriptId}」上的凭据？`)) return
    resetFlash()
    try {
      await deleteAgentScriptAuth(k, selectedId, scriptId)
      setInfo('已删除')
      await reloadScriptAuthsOnly(selectedId)
      if (formScriptId.trim() === scriptId.trim()) {
        setFormScriptId('')
        setFormUsername('')
        setFormPassword('')
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    }
  }

  const fillAuthForm = (row: AgentScriptAuthRow) => {
    setFormScriptId(row.scriptId)
    setFormUsername(row.username)
    setFormPassword('')
    setError(null)
    setInfo('已填入 script_id 与用户名；更新密码请重新输入后保存')
  }

  const inputCls =
    'w-full border border-slate-300 rounded px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-slate-400'

  /** 凭据 script_id 下拉：优先已安装脚本；合并已有凭据行，避免仅库里有记录时下拉为空 */
  const credentialScriptIdOptions = useMemo(() => {
    const set = new Set<string>()
    for (const s of scripts) {
      const id = (s.scriptId ?? '').trim()
      if (id) set.add(id)
    }
    for (const a of scriptAuths) {
      const id = (a.scriptId ?? '').trim()
      if (id) set.add(id)
    }
    const cur = formScriptId.trim()
    if (cur) set.add(cur)
    return Array.from(set).sort((a, b) => a.localeCompare(b))
  }, [scripts, scriptAuths, formScriptId])

  return (
    <div className="space-y-8">
      <div>
        <h2 className="text-lg font-semibold text-slate-800">Agent 运维</h2>
        <p className="text-sm text-slate-600 mt-1">
          对应后端 <code className="bg-slate-100 px-1 rounded">/api/v1/admin/agents/*</code>（含{' '}
          <code className="bg-slate-100 px-1 rounded">…/script-auths</code>
          ）与 <code className="bg-slate-100 px-1 rounded">/api/v1/admin/bom-platforms</code>，需配置{' '}
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
        <p className="mt-3 text-xs text-slate-500">
          列表与平台配置在下方 Tab 中分别刷新；两页共用本 Key。
        </p>
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

      <section className="bg-white border border-slate-200 rounded-lg shadow-sm overflow-hidden">
        <div
          className="flex flex-wrap items-end gap-1 px-2 pt-2 border-b border-slate-200 bg-slate-50/90"
          role="tablist"
          aria-label="运维数据分类"
        >
          <button
            type="button"
            role="tab"
            aria-selected={adminTab === 'bom-platforms'}
            id="admin-tab-bom-platforms"
            onClick={() => setAdminTab('bom-platforms')}
            className={`px-4 py-2.5 text-sm font-medium rounded-t-lg border transition-colors ${
              adminTab === 'bom-platforms'
                ? 'bg-white border-slate-200 border-b-white text-slate-900 -mb-px relative z-[1]'
                : 'border-transparent text-slate-600 hover:text-slate-900 hover:bg-white/70'
            }`}
          >
            BOM 采集平台
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={adminTab === 'agents'}
            id="admin-tab-agents"
            onClick={() => setAdminTab('agents')}
            className={`px-4 py-2.5 text-sm font-medium rounded-t-lg border transition-colors ${
              adminTab === 'agents'
                ? 'bg-white border-slate-200 border-b-white text-slate-900 -mb-px relative z-[1]'
                : 'border-transparent text-slate-600 hover:text-slate-900 hover:bg-white/70'
            }`}
          >
            Agent 列表
          </button>
        </div>

        <div
          className="p-5"
          role="tabpanel"
          aria-labelledby={adminTab === 'bom-platforms' ? 'admin-tab-bom-platforms' : 'admin-tab-agents'}
        >
          {adminTab === 'bom-platforms' && (
            <BomPlatformsAdminSection
              embedded
              apiKey={apiKey}
              requireKey={requireKey}
              resetFlash={resetFlash}
              setError={setError}
              setInfo={setInfo}
            />
          )}

          {adminTab === 'agents' && (
            <div className="space-y-6">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div className="min-w-0 flex-1">
                  <h3 className="font-medium text-slate-800 mb-2">Agent 列表</h3>
                  <p className="text-xs text-slate-500">
                    状态按<strong>任务心跳</strong>判定：距最近心跳未超过离线窗口为在线；超时为离线；从未有心跳时间为「未知」。刷新列表时会将库内{' '}
                    <code className="bg-slate-100 px-1 rounded">agent_status</code> 同步为 offline。
                    {offlineWindowSec != null && (
                      <>
                        {' '}
                        当前窗口：<strong>{offlineWindowSec}s</strong>（与{' '}
                        <code className="bg-slate-100 px-1 rounded">agent</code> 配置一致）。
                      </>
                    )}
                  </p>
                </div>
                <button
                  type="button"
                  disabled={loading}
                  onClick={loadAgents}
                  className="shrink-0 px-4 py-2 bg-slate-800 text-white text-sm rounded hover:bg-slate-700 disabled:opacity-50"
                >
                  {loading ? '加载中…' : '刷新 Agent 列表'}
                </button>
              </div>
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
                agents.map((a) => {
                  const st = agentStatusLabel(a.status, a.online)
                  return (
                  <tr
                    key={a.agentId}
                    className={`border-b border-slate-100 ${selectedId === a.agentId ? 'bg-slate-50' : ''}`}
                  >
                    <td className="py-2 pr-4">
                      <span className={`inline-block px-2 py-0.5 rounded text-xs font-medium ${st.cls}`}>
                        {st.text}
                      </span>
                      {a.status && (
                        <span className="ml-1 text-[10px] text-slate-400 font-mono">{a.status}</span>
                      )}
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
                        加载详情
                      </button>
                    </td>
                  </tr>
                  )
                })
              )}
            </tbody>
          </table>
        </div>

              {selectedId && (
        <section className="border border-slate-200 rounded-lg p-5 bg-slate-50/40 space-y-6">
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

          <div>
            <h4 className="text-sm font-medium text-slate-700 mb-2">平台登录凭据（按 script_id）</h4>
            <p className="text-xs text-slate-500 mb-3">
              与任务下发时注入的 <code className="bg-slate-100 px-1 rounded">params.platform_auth</code> 对应；密码仅保存时提交，列表不展示。
              script_id 请从下拉选择，选项来自上方「已安装脚本」；若凭据仍在库但该脚本当前未上报，也会出现在下拉里。
              服务端须配置 AES 密钥（<code className="bg-slate-100 px-1 rounded">CAICHIP_AGENT_SCRIPT_AUTH_KEY</code> 或{' '}
              <code className="bg-slate-100 px-1 rounded">agent_script_auth.aes_key_base64</code>），否则保存会返回{' '}
              <code className="bg-slate-100 px-1 rounded">SCRIPT_AUTH_CIPHER_DISABLED</code>。
            </p>
            <div className="overflow-x-auto mb-4">
              <table className="min-w-full text-sm">
                <thead>
                  <tr className="border-b border-slate-200 text-left text-slate-600">
                    <th className="py-2 pr-4">script_id</th>
                    <th className="py-2 pr-4">username</th>
                    <th className="py-2 pr-4">updated_at</th>
                    <th className="py-2">操作</th>
                  </tr>
                </thead>
                <tbody>
                  {scriptAuths.length === 0 ? (
                    <tr>
                      <td colSpan={4} className="py-4 text-slate-500 text-center">
                        暂无凭据（或未加载）
                      </td>
                    </tr>
                  ) : (
                    scriptAuths.map((r) => (
                      <tr key={r.scriptId} className="border-b border-slate-100">
                        <td className="py-2 pr-4 font-mono text-xs break-all max-w-[12rem]">{r.scriptId}</td>
                        <td className="py-2 pr-4">{r.username || '—'}</td>
                        <td className="py-2 pr-4 whitespace-nowrap">{formatTs(r.updatedAt)}</td>
                        <td className="py-2 space-x-2 whitespace-nowrap">
                          <button
                            type="button"
                            onClick={() => fillAuthForm(r)}
                            className="px-2 py-1 text-xs border border-slate-300 rounded hover:bg-slate-50"
                          >
                            填入表单
                          </button>
                          <button
                            type="button"
                            onClick={() => removeScriptAuth(r.scriptId)}
                            className="px-2 py-1 text-xs border border-red-200 text-red-800 rounded hover:bg-red-50"
                          >
                            删除
                          </button>
                        </td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>
            <div className="border border-slate-100 rounded-md p-4 bg-slate-50/80 space-y-3">
              <p className="text-xs text-slate-600 font-medium">新增或更新（PUT 幂等）</p>
              <div className="grid gap-3 sm:grid-cols-2">
                <div>
                  <label className="block text-xs text-slate-600 mb-1">script_id（已安装脚本）</label>
                  <select
                    className={inputCls}
                    value={formScriptId}
                    onChange={(e) => setFormScriptId(e.target.value)}
                  >
                    <option value="">
                      {credentialScriptIdOptions.length === 0
                        ? '请先加载详情，且 Agent 需有已安装脚本记录'
                        : '请选择 script_id'}
                    </option>
                    {credentialScriptIdOptions.map((id) => (
                      <option key={id} value={id}>
                        {id}
                      </option>
                    ))}
                  </select>
                </div>
                <div>
                  <label className="block text-xs text-slate-600 mb-1">用户名</label>
                  <input
                    type="text"
                    className={inputCls}
                    value={formUsername}
                    onChange={(e) => setFormUsername(e.target.value)}
                    autoComplete="username"
                  />
                </div>
              </div>
              <div>
                <label className="block text-xs text-slate-600 mb-1">密码</label>
                <input
                  type="password"
                  className={inputCls}
                  placeholder="新建或更新均需填写"
                  value={formPassword}
                  onChange={(e) => setFormPassword(e.target.value)}
                  autoComplete="new-password"
                />
              </div>
              <div className="flex flex-wrap gap-2">
                <button
                  type="button"
                  disabled={authSaving || detailLoading}
                  onClick={saveScriptAuth}
                  className="px-4 py-2 bg-slate-800 text-white text-sm rounded hover:bg-slate-700 disabled:opacity-50"
                >
                  {authSaving ? '保存中…' : '保存凭据'}
                </button>
                <button
                  type="button"
                  disabled={authSaving}
                  onClick={() => {
                    setFormScriptId('')
                    setFormUsername('')
                    setFormPassword('')
                    resetFlash()
                  }}
                  className="px-4 py-2 border border-slate-300 text-sm rounded hover:bg-white disabled:opacity-50"
                >
                  清空表单
                </button>
              </div>
            </div>
          </div>
        </section>
              )}
            </div>
          )}
        </div>
      </section>
    </div>
  )
}
