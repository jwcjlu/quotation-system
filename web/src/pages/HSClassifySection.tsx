import { useMemo, useState } from 'react'
import { classifyByModel, type HSClassifyReply, type HSClassifyRequest } from '../api'

interface HSClassifySectionProps {
  defaultModel?: string
}

function todayISO(): string {
  return new Date().toISOString().slice(0, 10)
}

export function HSClassifySection({ defaultModel = '' }: HSClassifySectionProps) {
  const [form, setForm] = useState<HSClassifyRequest>({
    trade_direction: 'import',
    declaration_date: todayISO(),
    model: defaultModel,
    product_name_cn: '',
    product_name_en: '',
    manufacturer: '',
    brand: '',
    package: '',
    description: '',
    category_hint: '',
  })
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [result, setResult] = useState<HSClassifyReply | null>(null)

  const reviewTagCls = useMemo(() => {
    if (!result?.final_suggestion) return 'bg-slate-100 text-slate-700'
    return result.final_suggestion.review_required ? 'bg-amber-100 text-amber-800' : 'bg-emerald-100 text-emerald-800'
  }, [result])

  const update = (key: keyof HSClassifyRequest, value: string) => {
    setForm((prev) => ({ ...prev, [key]: value }))
  }

  const submit = async () => {
    if (!form.model.trim() || !form.product_name_cn.trim() || !form.declaration_date.trim()) {
      setError('请至少填写申报日期、型号、中文品名')
      return
    }
    setLoading(true)
    setError(null)
    try {
      const resp = await classifyByModel(form)
      setResult(resp)
    } catch (e) {
      setError(e instanceof Error ? e.message : '归类失败')
      setResult(null)
    } finally {
      setLoading(false)
    }
  }

  return (
    <section className="rounded-xl border border-slate-200 bg-white p-6 shadow-sm space-y-4">
      <div>
        <h3 className="font-semibold text-slate-800">HS 归类辅助（ClassifyByModel）</h3>
        <p className="text-sm text-slate-600 mt-1">输入型号参数，返回 Top5 建议、终裁结果与 trace。</p>
      </div>

      <div className="grid gap-3 md:grid-cols-3">
        <label className="text-sm">
          <span className="text-slate-600">贸易方向</span>
          <select
            value={form.trade_direction}
            onChange={(e) => update('trade_direction', e.target.value as 'import' | 'export')}
            className="mt-1 w-full border border-slate-300 rounded px-3 py-2"
          >
            <option value="import">import</option>
            <option value="export">export</option>
          </select>
        </label>
        <label className="text-sm">
          <span className="text-slate-600">申报日期</span>
          <input
            type="date"
            value={form.declaration_date}
            onChange={(e) => update('declaration_date', e.target.value)}
            className="mt-1 w-full border border-slate-300 rounded px-3 py-2"
          />
        </label>
        <label className="text-sm">
          <span className="text-slate-600">型号 *</span>
          <input
            value={form.model}
            onChange={(e) => update('model', e.target.value)}
            className="mt-1 w-full border border-slate-300 rounded px-3 py-2 font-mono"
          />
        </label>
      </div>

      <div className="grid gap-3 md:grid-cols-2">
        <label className="text-sm">
          <span className="text-slate-600">中文品名 *</span>
          <input
            value={form.product_name_cn}
            onChange={(e) => update('product_name_cn', e.target.value)}
            className="mt-1 w-full border border-slate-300 rounded px-3 py-2"
          />
        </label>
        <label className="text-sm">
          <span className="text-slate-600">英文品名</span>
          <input
            value={form.product_name_en ?? ''}
            onChange={(e) => update('product_name_en', e.target.value)}
            className="mt-1 w-full border border-slate-300 rounded px-3 py-2"
          />
        </label>
        <label className="text-sm">
          <span className="text-slate-600">厂牌</span>
          <input
            value={form.manufacturer ?? ''}
            onChange={(e) => update('manufacturer', e.target.value)}
            className="mt-1 w-full border border-slate-300 rounded px-3 py-2"
          />
        </label>
        <label className="text-sm">
          <span className="text-slate-600">封装</span>
          <input
            value={form.package ?? ''}
            onChange={(e) => update('package', e.target.value)}
            className="mt-1 w-full border border-slate-300 rounded px-3 py-2"
          />
        </label>
      </div>

      <div className="flex items-center gap-3">
        <button
          type="button"
          onClick={() => void submit()}
          disabled={loading}
          className="rounded bg-indigo-600 text-white px-4 py-2 text-sm hover:bg-indigo-700 disabled:opacity-50"
        >
          {loading ? '归类中…' : '执行归类'}
        </button>
        {error && <span className="text-sm text-red-700">{error}</span>}
      </div>

      {result?.final_suggestion && (
        <div className="rounded border border-slate-200 p-3 bg-slate-50">
          <div className="flex flex-wrap items-center gap-2">
            <span className={`text-xs px-2 py-0.5 rounded ${reviewTagCls}`}>
              {result.final_suggestion.review_required ? '需人工复核' : '可自动通过'}
            </span>
            <span className="text-sm">HS: <strong>{result.final_suggestion.hs_code || '—'}</strong></span>
            <span className="text-sm">置信度: <strong>{result.final_suggestion.confidence.toFixed(1)}</strong></span>
          </div>
          {result.final_suggestion.review_reason_codes.length > 0 && (
            <p className="text-xs text-slate-600 mt-2">
              原因码：{result.final_suggestion.review_reason_codes.join(', ')}
            </p>
          )}
          {result.trace && (
            <p className="text-xs text-slate-600 mt-1">
              policy_version_id: {result.trace.policy_version_id || '—'}
            </p>
          )}
        </div>
      )}

      {result && (
        <div className="overflow-x-auto">
          <table className="w-full text-sm border border-slate-200 rounded">
            <thead className="bg-slate-50">
              <tr>
                <th className="text-left p-2">#</th>
                <th className="text-left p-2">HS Code</th>
                <th className="text-left p-2">Score</th>
                <th className="text-left p-2">Reason</th>
              </tr>
            </thead>
            <tbody>
              {result.candidates.length === 0 ? (
                <tr>
                  <td colSpan={4} className="p-3 text-slate-500 text-center">无候选结果</td>
                </tr>
              ) : (
                result.candidates.map((c, idx) => (
                  <tr key={`${c.hs_code}-${idx}`} className="border-t border-slate-100">
                    <td className="p-2">{idx + 1}</td>
                    <td className="p-2 font-mono">{c.hs_code}</td>
                    <td className="p-2">{c.score.toFixed(2)}</td>
                    <td className="p-2">{c.reason || '—'}</td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      )}
    </section>
  )
}
