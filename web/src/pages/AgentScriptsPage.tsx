import { useCallback, useState } from 'react'
import { PLATFORM_IDS } from '../api'
import {
  getCurrentAgentScript,
  listAgentScriptPackages,
  publishAgentScriptPackage,
  uploadAgentScriptPackage,
  type CurrentPackageReply,
  type PackageListItem,
} from '../api/agentScripts'

const STORAGE_KEY = 'caichip_web_admin_script_api_key'

export function AgentScriptsPage() {
  const [apiKey, setApiKey] = useState(() => localStorage.getItem(STORAGE_KEY) ?? '')

  const [scriptId, setScriptId] = useState('')
  const [version, setVersion] = useState('')
  const [file, setFile] = useState<File | null>(null)
  const [entryFile, setEntryFile] = useState('')
  const [releaseNotes, setReleaseNotes] = useState('')
  const [packageSha256, setPackageSha256] = useState('')

  const [lastPackageId, setLastPackageId] = useState<number | null>(null)
  const [publishIdInput, setPublishIdInput] = useState('')

  const [queryScriptId, setQueryScriptId] = useState('')
  const [current, setCurrent] = useState<CurrentPackageReply | null>(null)

  const [packages, setPackages] = useState<PackageListItem[]>([])
  const [listOffset, setListOffset] = useState(0)

  const [loading, setLoading] = useState(false)
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
      setError('请先填写管理端 API Key（与 configs 中 script_admin.api_keys 一致）')
      return null
    }
    return k
  }

  const resetFlash = () => {
    setError(null)
    setInfo(null)
  }

  const handleUpload = async () => {
    const k = requireKey()
    if (!k) return
    if (!scriptId.trim() || !version.trim()) {
      setError('请选择 script_id（货源平台）并填写 version')
      return
    }
    if (!file) {
      setError('请选择 zip 包文件')
      return
    }
    setLoading(true)
    resetFlash()
    try {
      const r = await uploadAgentScriptPackage(k, {
        scriptId: scriptId.trim(),
        version: version.trim(),
        file,
        entryFile: entryFile.trim() || undefined,
        releaseNotes: releaseNotes.trim() || undefined,
        packageSha256: packageSha256.trim() || undefined,
      })
      setLastPackageId(r.package_id)
      setPublishIdInput(String(r.package_id))
      setInfo(`上传成功：package_id=${r.package_id}，sha256=${r.sha256}，路径 ${r.download_path}`)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setLoading(false)
    }
  }

  const handlePublish = async () => {
    const k = requireKey()
    if (!k) return
    const id = Number.parseInt(publishIdInput.trim(), 10)
    if (!Number.isFinite(id) || id <= 0) {
      setError('发布请填写有效的 package_id（数字）')
      return
    }
    setLoading(true)
    resetFlash()
    try {
      await publishAgentScriptPackage(k, id)
      setInfo(`已发布 package_id=${id}`)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setLoading(false)
    }
  }

  const handleQueryCurrent = async () => {
    const k = requireKey()
    if (!k) return
    if (!queryScriptId.trim()) {
      setError('查询请选择 script_id（货源平台）')
      return
    }
    setLoading(true)
    resetFlash()
    try {
      const r = await getCurrentAgentScript(k, queryScriptId.trim())
      setCurrent(r)
      setInfo('已加载当前发布版本')
    } catch (e) {
      setCurrent(null)
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setLoading(false)
    }
  }

  const loadList = async (offset: number) => {
    const k = requireKey()
    if (!k) return
    setLoading(true)
    resetFlash()
    try {
      const r = await listAgentScriptPackages(k, offset, 20)
      setPackages(r.packages ?? [])
      setListOffset(offset)
      setInfo(`已加载 ${r.packages?.length ?? 0} 条（offset=${offset}）`)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setLoading(false)
    }
  }

  const inputCls =
    'w-full border border-slate-300 rounded px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-slate-400'

  return (
    <div className="space-y-8">
      <div>
        <h2 className="text-lg font-semibold text-slate-800">Agent 脚本包管理</h2>
        <p className="text-sm text-slate-600 mt-1">
          对应后端 <code className="bg-slate-100 px-1 rounded">/api/v1/admin/agent-scripts/*</code>，需配置{' '}
          <code className="bg-slate-100 px-1 rounded">script_admin.api_keys</code>。开发时 Vite 已将{' '}
          <code className="bg-slate-100 px-1 rounded">/api</code> 代理到{' '}
          <code className="bg-slate-100 px-1 rounded">127.0.0.1:18080</code>。
        </p>
      </div>

      <section className="bg-white border border-slate-200 rounded-lg p-5 shadow-sm">
        <h3 className="font-medium text-slate-800 mb-3">鉴权</h3>
        <label className="block text-sm text-slate-600 mb-1">管理端 API Key</label>
        <input
          type="password"
          autoComplete="off"
          className={inputCls}
          placeholder="与 configs 中 script_admin.api_keys 某项一致"
          value={apiKey}
          onChange={(e) => persistKey(e.target.value)}
        />
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
        <h3 className="font-medium text-slate-800 mb-3">上传包</h3>
        <div className="grid gap-3 sm:grid-cols-2">
          <div>
            <label className="block text-sm text-slate-600 mb-1">
              script_id（与货源会话 platform_id 一致，仅可选）
            </label>
            <select
              className={inputCls}
              value={scriptId}
              onChange={(e) => setScriptId(e.target.value)}
            >
              <option value="">请选择平台</option>
              {PLATFORM_IDS.map((id) => (
                <option key={id} value={id}>
                  {id}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label className="block text-sm text-slate-600 mb-1">version</label>
            <input className={inputCls} value={version} onChange={(e) => setVersion(e.target.value)} />
          </div>
          <div className="sm:col-span-2">
            <label className="block text-sm text-slate-600 mb-1">
              入口文件名 entry_file（可选，留空为 <code className="bg-slate-100 px-1 rounded text-xs">{`{script_id}_crawler.py`}</code>）
            </label>
            <input
              className={inputCls}
              placeholder="例如 szlcsc_crawler.py"
              value={entryFile}
              onChange={(e) => setEntryFile(e.target.value)}
            />
          </div>
          <div className="sm:col-span-2">
            <label className="block text-sm text-slate-600 mb-1">zip 文件</label>
            <input
              type="file"
              accept=".zip,application/zip"
              className="text-sm"
              onChange={(e) => setFile(e.target.files?.[0] ?? null)}
            />
          </div>
          <div className="sm:col-span-2">
            <label className="block text-sm text-slate-600 mb-1">release_notes（可选）</label>
            <textarea
              className={inputCls}
              rows={2}
              value={releaseNotes}
              onChange={(e) => setReleaseNotes(e.target.value)}
            />
          </div>
          <div className="sm:col-span-2">
            <label className="block text-sm text-slate-600 mb-1">package_sha256（可选，小写 hex）</label>
            <input
              className={inputCls}
              value={packageSha256}
              onChange={(e) => setPackageSha256(e.target.value)}
            />
          </div>
        </div>
        <button
          type="button"
          disabled={loading}
          onClick={handleUpload}
          className="mt-4 px-4 py-2 bg-slate-800 text-white text-sm rounded hover:bg-slate-700 disabled:opacity-50"
        >
          上传
        </button>
        {lastPackageId != null && (
          <p className="mt-2 text-sm text-slate-600">最近一次上传 package_id：<strong>{lastPackageId}</strong></p>
        )}
      </section>

      <section className="bg-white border border-slate-200 rounded-lg p-5 shadow-sm">
        <h3 className="font-medium text-slate-800 mb-3">发布为当前版本</h3>
        <div className="flex flex-wrap gap-2 items-end">
          <div className="flex-1 min-w-[12rem]">
            <label className="block text-sm text-slate-600 mb-1">package_id</label>
            <input
              className={inputCls}
              value={publishIdInput}
              onChange={(e) => setPublishIdInput(e.target.value)}
              placeholder="上传成功后自动填入，也可手填"
            />
          </div>
          <button
            type="button"
            disabled={loading}
            onClick={handlePublish}
            className="px-4 py-2 bg-amber-600 text-white text-sm rounded hover:bg-amber-500 disabled:opacity-50"
          >
            发布
          </button>
        </div>
      </section>

      <section className="bg-white border border-slate-200 rounded-lg p-5 shadow-sm">
        <h3 className="font-medium text-slate-800 mb-3">查询当前发布</h3>
        <div className="flex flex-wrap gap-2 items-end">
          <div className="flex-1 min-w-[12rem]">
            <label className="block text-sm text-slate-600 mb-1">
              script_id（与货源会话 platform_id 一致，仅可选）
            </label>
            <select
              className={inputCls}
              value={queryScriptId}
              onChange={(e) => setQueryScriptId(e.target.value)}
            >
              <option value="">请选择平台</option>
              {PLATFORM_IDS.map((id) => (
                <option key={id} value={id}>
                  {id}
                </option>
              ))}
            </select>
          </div>
          <button
            type="button"
            disabled={loading}
            onClick={handleQueryCurrent}
            className="px-4 py-2 bg-slate-700 text-white text-sm rounded hover:bg-slate-600 disabled:opacity-50"
          >
            查询
          </button>
        </div>
        {current && (
          <dl className="mt-4 grid gap-2 sm:grid-cols-2 text-sm">
            <dt className="text-slate-500">version</dt>
            <dd className="font-mono">{current.version}</dd>
            <dt className="text-slate-500">sha256</dt>
            <dd className="font-mono break-all">{current.sha256}</dd>
            <dt className="text-slate-500">public_path</dt>
            <dd className="font-mono break-all">{current.public_path}</dd>
            <dt className="text-slate-500">filename</dt>
            <dd>{current.filename}</dd>
            <dt className="text-slate-500">entry_file</dt>
            <dd className="font-mono">{current.entry_file}</dd>
            <dt className="text-slate-500">status</dt>
            <dd>{current.status}</dd>
          </dl>
        )}
      </section>

      <section className="bg-white border border-slate-200 rounded-lg p-5 shadow-sm">
        <h3 className="font-medium text-slate-800 mb-3">最近包列表（审计）</h3>
        <div className="flex gap-2 mb-3">
          <button
            type="button"
            disabled={loading}
            onClick={() => loadList(0)}
            className="px-3 py-1.5 text-sm border border-slate-300 rounded hover:bg-slate-50 disabled:opacity-50"
          >
            刷新首页
          </button>
          <button
            type="button"
            disabled={loading || listOffset === 0}
            onClick={() => loadList(Math.max(0, listOffset - 20))}
            className="px-3 py-1.5 text-sm border border-slate-300 rounded hover:bg-slate-50 disabled:opacity-50"
          >
            上一页
          </button>
          <button
            type="button"
            disabled={loading || packages.length < 20}
            onClick={() => loadList(listOffset + 20)}
            className="px-3 py-1.5 text-sm border border-slate-300 rounded hover:bg-slate-50 disabled:opacity-50"
          >
            下一页
          </button>
          <span className="text-sm text-slate-500 self-center">offset={listOffset}</span>
        </div>
        <div className="overflow-x-auto">
          <table className="min-w-full text-sm">
            <thead>
              <tr className="border-b border-slate-200 text-left text-slate-600">
                <th className="py-2 pr-4">id</th>
                <th className="py-2 pr-4">script_id</th>
                <th className="py-2 pr-4">version</th>
                <th className="py-2 pr-4">entry_file</th>
                <th className="py-2 pr-4">status</th>
                <th className="py-2">sha256</th>
              </tr>
            </thead>
            <tbody>
              {packages.length === 0 ? (
                <tr>
                  <td colSpan={6} className="py-6 text-slate-500 text-center">
                    暂无数据，点击「刷新首页」加载
                  </td>
                </tr>
              ) : (
                packages.map((p) => (
                  <tr key={p.id} className="border-b border-slate-100">
                    <td className="py-2 pr-4 font-mono">{p.id}</td>
                    <td className="py-2 pr-4">{p.script_id}</td>
                    <td className="py-2 pr-4">{p.version}</td>
                    <td className="py-2 pr-4 font-mono text-xs">{p.entry_file}</td>
                    <td className="py-2 pr-4">{p.status}</td>
                    <td className="py-2 font-mono text-xs break-all max-w-xs">{p.sha256}</td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </section>
    </div>
  )
}
