import { useState, useCallback, useEffect } from 'react'
import * as XLSX from 'xlsx'
import { createSession, uploadBOM, downloadTemplate, PLATFORM_IDS } from '../api'
import { validateSessionHeaderFields, type ReadinessMode } from '../utils/sessionFields'

const PARSE_MODES = [
  { value: 'auto', label: '通用模式', desc: '自动识别表头' },
  { value: 'custom', label: '自定义映射', desc: '手动指定列映射' },
] as const

const MAPPING_FIELDS = [
  { key: 'model', label: '型号', required: true },
  { key: 'manufacturer', label: '厂牌', required: false },
  { key: 'package', label: '封装', required: false },
  { key: 'quantity', label: '数量', required: false },
  { key: 'params', label: '参数/备注', required: false },
] as const

interface UploadPageProps {
  onSuccess: (bomId: string) => void
  /** 嵌入 BOM 列表页时紧凑排版，并避免与其它页面 file input id 冲突 */
  embedded?: boolean
}

function readExcelHeaders(file: File): Promise<string[]> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = (e) => {
      try {
        const data = e.target?.result
        if (!data || typeof data !== 'object') {
          reject(new Error('无法读取文件'))
          return
        }
        const wb = XLSX.read(data, { type: 'array' })
        const firstSheet = wb.SheetNames[0]
        if (!firstSheet) {
          reject(new Error('无工作表'))
          return
        }
        const ws = wb.Sheets[firstSheet]
        const rows = XLSX.utils.sheet_to_json<string[]>(ws, { header: 1 })
        const headers = (rows[0] ?? []).map((h) => String(h ?? '').trim()).filter(Boolean)
        resolve(headers)
      } catch (err) {
        reject(err)
      }
    }
    reader.onerror = () => reject(new Error('读取失败'))
    reader.readAsArrayBuffer(file)
  })
}

export function UploadPage({ onSuccess, embedded }: UploadPageProps) {
  const fid = embedded ? 'file-input-session-embedded' : 'file-input'
  const [sessionTitle, setSessionTitle] = useState('')
  const [customerName, setCustomerName] = useState('')
  const [contactPhone, setContactPhone] = useState('')
  const [contactEmail, setContactEmail] = useState('')
  const [contactExtra, setContactExtra] = useState('')
  const [readinessMode, setReadinessMode] = useState<ReadinessMode>('lenient')
  const [sessionPlatforms, setSessionPlatforms] = useState<string[]>([...PLATFORM_IDS])
  const [file, setFile] = useState<File | null>(null)
  const [parseMode, setParseMode] = useState<string>('auto')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [dragOver, setDragOver] = useState(false)
  const [headers, setHeaders] = useState<string[]>([])
  const [columnMapping, setColumnMapping] = useState<Record<string, string>>({})

  const onDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    setDragOver(false)
    const f = e.dataTransfer.files[0]
    if (f && /\.(xlsx|xls)$/i.test(f.name)) setFile(f)
    else setError('请上传 Excel 文件 (.xlsx, .xls)')
  }, [])

  const onDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    setDragOver(true)
  }, [])

  const onDragLeave = useCallback(() => setDragOver(false), [])

  const onFileSelect = (e: React.ChangeEvent<HTMLInputElement>) => {
    const f = e.target.files?.[0]
    if (f) setFile(f)
    setError(null)
    setHeaders([])
    setColumnMapping({})
  }

  useEffect(() => {
    if (parseMode === 'custom' && file) {
      readExcelHeaders(file)
        .then(setHeaders)
        .catch(() => setHeaders([]))
    } else {
      setHeaders([])
      setColumnMapping({})
    }
  }, [parseMode, file])

  const toggleSessionPlatform = (id: string) => {
    setSessionPlatforms((prev) =>
      prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]
    )
  }

  const handleUpload = async () => {
    if (!file) {
      setError('请选择文件')
      return
    }
    if (parseMode === 'custom' && !columnMapping.model) {
      setError('自定义模式请至少配置「型号」列映射')
      return
    }
    if (sessionPlatforms.length === 0) {
      setError('请至少勾选一个货源平台')
      return
    }
    const fieldErr = validateSessionHeaderFields({
      title: sessionTitle.trim(),
      customerName: customerName.trim(),
      contactPhone: contactPhone.trim(),
      contactEmail: contactEmail.trim(),
      contactExtra: contactExtra.trim(),
    })
    if (fieldErr) {
      setError(fieldErr)
      return
    }
    setLoading(true)
    setError(null)
    try {
      const mapping = parseMode === 'custom' && Object.keys(columnMapping).length > 0 ? columnMapping : undefined
      const sess = await createSession({
        title: sessionTitle.trim(),
        platform_ids: sessionPlatforms,
        customer_name: customerName.trim(),
        contact_phone: contactPhone.trim(),
        contact_email: contactEmail.trim(),
        contact_extra: contactExtra.trim(),
        readiness_mode: readinessMode,
      })
      const res = await uploadBOM(file, parseMode, mapping, { sessionId: sess.session_id })
      onSuccess(res.bom_id)
    } catch (e) {
      setError(e instanceof Error ? e.message : '上传失败')
    } finally {
      setLoading(false)
    }
  }

  const handleDownloadTemplate = async () => {
    try {
      const blob = await downloadTemplate()
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = 'bom_template.xlsx'
      a.click()
      URL.revokeObjectURL(url)
    } catch (e) {
      setError(e instanceof Error ? e.message : '下载失败')
    }
  }

  return (
    <div className={embedded ? 'space-y-4' : 'space-y-8'}>
      <div className={embedded ? 'text-left' : 'text-center'}>
        <h2 className={embedded ? 'text-lg font-bold text-slate-800' : 'text-2xl font-bold text-slate-800'}>
          {embedded ? '新建会话并上传 BOM' : '货源会话 · BOM 上传'}
        </h2>
        <p className="text-slate-600 mt-1 text-sm">
          {embedded
            ? '创建 bom_session 并写入行数据；下方可打开会话查看与行维护'
            : '将创建会话并写入 bom_session_line，随后可在会话列表中打开详情'}
        </p>
      </div>

      <div className="rounded-lg border border-slate-200 bg-white p-4 space-y-3">
          <div>
            <label className="block text-sm font-medium text-slate-700 mb-1">数据就绪策略</label>
            <select
              value={readinessMode}
              onChange={(e) => setReadinessMode(e.target.value as ReadinessMode)}
              className="w-full max-w-md border border-slate-300 rounded-lg px-3 py-2 text-sm"
            >
              <option value="lenient">宽松：各平台任务到终态即可标「数据已准备」</option>
              <option value="strict">严格：每行至少一个平台为成功（succeeded）</option>
            </select>
            <p className="text-xs text-slate-500 mt-1">对应后端 readiness_mode（lenient / strict）</p>
          </div>
          <div>
            <label className="block text-sm font-medium text-slate-700 mb-1">会话标题（可选）</label>
            <input
              type="text"
              value={sessionTitle}
              onChange={(e) => setSessionTitle(e.target.value)}
              placeholder="例如：客户 A 询价单"
              className="w-full border border-slate-300 rounded-lg px-3 py-2 text-sm"
            />
          </div>
          <div className="grid sm:grid-cols-2 gap-3">
            <div>
              <label className="block text-sm font-medium text-slate-700 mb-1">客户名称</label>
              <input
                type="text"
                value={customerName}
                onChange={(e) => setCustomerName(e.target.value)}
                className="w-full border border-slate-300 rounded-lg px-3 py-2 text-sm"
                placeholder="选填"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-slate-700 mb-1">联系电话</label>
              <input
                type="text"
                value={contactPhone}
                onChange={(e) => setContactPhone(e.target.value)}
                className="w-full border border-slate-300 rounded-lg px-3 py-2 text-sm"
                placeholder="选填"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-slate-700 mb-1">邮箱</label>
              <input
                type="email"
                value={contactEmail}
                onChange={(e) => setContactEmail(e.target.value)}
                className="w-full border border-slate-300 rounded-lg px-3 py-2 text-sm"
                placeholder="选填"
              />
            </div>
            <div className="sm:col-span-2">
              <label className="block text-sm font-medium text-slate-700 mb-1">备注 / 微信等</label>
              <input
                type="text"
                value={contactExtra}
                onChange={(e) => setContactExtra(e.target.value)}
                className="w-full border border-slate-300 rounded-lg px-3 py-2 text-sm"
                placeholder="选填"
              />
            </div>
          </div>
          <div>
            <span className="block text-sm font-medium text-slate-700 mb-2">初始勾选平台（POST /bom-sessions）</span>
            <div className="flex flex-wrap gap-3">
              {PLATFORM_IDS.map((id) => (
                <label key={id} className="flex items-center gap-2 text-sm cursor-pointer">
                  <input
                    type="checkbox"
                    checked={sessionPlatforms.includes(id)}
                    onChange={() => toggleSessionPlatform(id)}
                  />
                  {id}
                </label>
              ))}
            </div>
          </div>
        </div>

      <div className={embedded ? 'grid md:grid-cols-2 gap-4' : 'grid md:grid-cols-2 gap-8'}>
        <div>
          <div
            onDrop={onDrop}
            onDragOver={onDragOver}
            onDragLeave={onDragLeave}
            className={`border-2 border-dashed rounded-xl ${embedded ? 'p-6' : 'p-12'} text-center transition-colors ${
              dragOver ? 'border-blue-400 bg-blue-50' : 'border-slate-300 bg-white'
            }`}
          >
            <input
              type="file"
              accept=".xlsx,.xls"
              onChange={onFileSelect}
              className="hidden"
              id={fid}
            />
            <label htmlFor={fid} className="cursor-pointer block">
              <svg className="mx-auto h-12 w-12 text-slate-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M15 13l-3-3m0 0l-3 3m3-3v12" />
              </svg>
              <p className="mt-2 text-slate-600">拖拽文件到此处，或点击选择</p>
              <p className="mt-1 text-sm text-slate-500">支持 .xlsx、.xls</p>
              {file && <p className="mt-2 text-blue-600 font-medium">{file.name}</p>}
            </label>
          </div>
        </div>

        <div className="space-y-8">
          <div>
            <h3 className="font-medium text-slate-800 mb-3">解析模式</h3>
            <div className="space-y-2">
              {PARSE_MODES.map((m) => (
                <label key={m.value} className="flex items-center gap-3 cursor-pointer">
                  <input
                    type="radio"
                    name="parseMode"
                    value={m.value}
                    checked={parseMode === m.value}
                    onChange={() => setParseMode(m.value)}
                    className=""
                  />
                  <span className="font-medium">{m.label}</span>
                  <span className="text-slate-500 text-sm">{m.desc}</span>
                </label>
              ))}
            </div>
          </div>

          {parseMode === 'custom' && (
            <div className="rounded-lg border border-slate-200 bg-slate-50 p-4">
              <h3 className="font-medium text-slate-800 mb-3">列映射配置</h3>
              {headers.length === 0 ? (
                <p className="text-slate-500 text-sm">请先选择 Excel 文件，将读取表头供映射</p>
              ) : (
                <div className="space-y-3">
                  {MAPPING_FIELDS.map(({ key, label, required }) => (
                    <div key={key} className="flex items-center gap-3">
                      <label className="w-24 text-sm text-slate-700">
                        {label}
                        {required && <span className="text-red-500">*</span>}
                      </label>
                      <select
                        value={columnMapping[key] ?? ''}
                        onChange={(e) =>
                          setColumnMapping((prev) => {
                            const v = e.target.value
                            if (!v) {
                              const next = { ...prev }
                              delete next[key]
                              return next
                            }
                            return { ...prev, [key]: v }
                          })
                        }
                        className="flex-1 border border-slate-300 rounded px-3 py-2 text-sm"
                      >
                        <option value="">— 不映射 —</option>
                        {headers.map((h, i) => (
                          <option key={i} value={h}>
                            {h}
                          </option>
                        ))}
                      </select>
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}

          <div className="flex flex-col gap-3">
            <button
              onClick={handleUpload}
              disabled={loading || !file}
              className="px-6 py-3 bg-blue-600 text-white rounded-lg font-medium hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {loading ? '解析中...' : '上传并解析'}
            </button>
            <button
              onClick={handleDownloadTemplate}
              className="px-6 py-2 border border-slate-300 rounded-lg text-slate-700 hover:bg-slate-50"
            >
              下载 BOM 模板
            </button>
          </div>
        </div>
      </div>

      {error && (
        <div className="p-4 bg-red-50 text-red-700 rounded-lg">
          {error}
        </div>
      )}
    </div>
  )
}
