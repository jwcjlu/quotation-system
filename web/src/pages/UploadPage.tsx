import { useState, useCallback } from 'react'
import { uploadBOM, downloadTemplate } from '../api'

const PARSE_MODES = [
  { value: 'auto', label: '通用模式', desc: '自动识别表头' },
  { value: 'szlcsc', label: '立创标准', desc: '立创商城 BOM 模板' },
  { value: 'ickey', label: '云汉标准', desc: '云汉芯城 BOM 模板' },
  { value: 'custom', label: '自定义映射', desc: '手动指定列映射' },
] as const

interface UploadPageProps {
  onSuccess: (bomId: string) => void
}

export function UploadPage({ onSuccess }: UploadPageProps) {
  const [file, setFile] = useState<File | null>(null)
  const [parseMode, setParseMode] = useState<string>('auto')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [dragOver, setDragOver] = useState(false)

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
  }

  const handleUpload = async () => {
    if (!file) {
      setError('请选择文件')
      return
    }
    setLoading(true)
    setError(null)
    try {
      const res = await uploadBOM(file, parseMode)
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
    <div className="space-y-8">
      <div className="text-center">
        <h2 className="text-2xl font-bold text-slate-800">BOM 单导入</h2>
        <p className="text-slate-600 mt-1">支持 Excel 格式，选择解析模式后上传</p>
      </div>

      <div className="grid md:grid-cols-2 gap-8">
        <div>
          <div
            onDrop={onDrop}
            onDragOver={onDragOver}
            onDragLeave={onDragLeave}
            className={`border-2 border-dashed rounded-xl p-12 text-center transition-colors ${
              dragOver ? 'border-blue-400 bg-blue-50' : 'border-slate-300 bg-white'
            }`}
          >
            <input
              type="file"
              accept=".xlsx,.xls"
              onChange={onFileSelect}
              className="hidden"
              id="file-input"
            />
            <label htmlFor="file-input" className="cursor-pointer block">
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
