import { useCallback, useState } from 'react'
import type { HsMetaRow } from '../../api/hsMeta'
import { hsMetaCreate, hsMetaDelete, hsMetaList, hsMetaUpdate } from '../../api/hsMeta'

const emptyForm = {
  id: 0,
  category: '',
  component_name: '',
  core_hs6: '',
  description: '',
  enabled: true,
  sort_order: 0,
}

export function MetaCrudPanel() {
  const [page, setPage] = useState(1)
  const [items, setItems] = useState<HsMetaRow[]>([])
  const [total, setTotal] = useState(0)
  const [filterCat, setFilterCat] = useState('')
  const [filterName, setFilterName] = useState('')
  const [filterHs6, setFilterHs6] = useState('')
  const [err, setErr] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [form, setForm] = useState(emptyForm)

  const doLoad = useCallback(
    async (targetPage: number) => {
      setErr(null)
      setBusy(true)
      try {
        const result = await hsMetaList({
          page: targetPage,
          page_size: 20,
          category: filterCat.trim() || undefined,
          component_name: filterName.trim() || undefined,
          core_hs6: filterHs6.trim() || undefined,
        })
        setItems(result.items)
        setTotal(result.total)
        setPage(targetPage)
      } catch (error) {
        setErr(error instanceof Error ? error.message : String(error))
        setItems([])
      } finally {
        setBusy(false)
      }
    },
    [filterCat, filterHs6, filterName],
  )

  const resetForm = () => setForm(emptyForm)

  const saveForm = async () => {
    setBusy(true)
    setErr(null)
    try {
      if (form.id) {
        await hsMetaUpdate(form)
      } else {
        await hsMetaCreate({
          category: form.category,
          component_name: form.component_name,
          core_hs6: form.core_hs6,
          description: form.description,
          enabled: form.enabled,
          sort_order: form.sort_order,
        })
      }
      resetForm()
      await doLoad(page)
    } catch (error) {
      setErr(error instanceof Error ? error.message : String(error))
    } finally {
      setBusy(false)
    }
  }

  const deleteCurrent = async () => {
    if (!form.id || !confirm(`确认删除元数据 #${form.id}？`)) return
    setBusy(true)
    setErr(null)
    try {
      await hsMetaDelete({ id: form.id })
      resetForm()
      await doLoad(page)
    } catch (error) {
      setErr(error instanceof Error ? error.message : String(error))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="space-y-5">
      <div className="grid gap-3 lg:grid-cols-[1fr_1fr_10rem_auto_auto] lg:items-end">
        <label className="block text-sm font-medium text-slate-700">
          分类
          <input className="mt-1 w-full rounded-md border border-slate-300 px-3 py-2" value={filterCat} onChange={(e) => setFilterCat(e.target.value)} />
        </label>
        <label className="block text-sm font-medium text-slate-700">
          组件名称
          <input className="mt-1 w-full rounded-md border border-slate-300 px-3 py-2" value={filterName} onChange={(e) => setFilterName(e.target.value)} />
        </label>
        <label className="block text-sm font-medium text-slate-700">
          核心 HS6
          <input className="mt-1 w-full rounded-md border border-slate-300 px-3 py-2 font-mono" value={filterHs6} onChange={(e) => setFilterHs6(e.target.value)} />
        </label>
        <button type="button" className="rounded-md bg-slate-900 px-4 py-2 text-sm font-medium text-white disabled:bg-slate-300" disabled={busy} onClick={() => void doLoad(1)}>
          查询
        </button>
        <div className="text-sm text-slate-500">共 {total} 条</div>
      </div>

      {err && <div className="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">{err}</div>}

      <div className="overflow-hidden rounded-lg border border-slate-200">
        <table className="min-w-full divide-y divide-slate-200 text-sm">
          <thead className="bg-slate-50 text-left text-xs font-semibold uppercase text-slate-500">
            <tr>
              <th className="px-3 py-3">ID</th>
              <th className="px-3 py-3">分类</th>
              <th className="px-3 py-3">组件</th>
              <th className="px-3 py-3">核心 HS6</th>
              <th className="px-3 py-3">启用</th>
              <th className="px-3 py-3">排序</th>
              <th className="px-3 py-3 text-right">操作</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100 bg-white">
            {items.map((row) => (
              <tr key={row.id}>
                <td className="px-3 py-3">{row.id}</td>
                <td className="px-3 py-3">{row.category || '-'}</td>
                <td className="px-3 py-3 font-medium text-slate-950">{row.component_name}</td>
                <td className="px-3 py-3 font-mono">{row.core_hs6}</td>
                <td className="px-3 py-3">{row.enabled ? '是' : '否'}</td>
                <td className="px-3 py-3">{row.sort_order}</td>
                <td className="px-3 py-3 text-right">
                  <button type="button" className="text-blue-600 hover:underline" onClick={() => setForm({ ...emptyForm, ...row })}>
                    编辑
                  </button>
                </td>
              </tr>
            ))}
            {items.length === 0 && (
              <tr>
                <td className="px-3 py-6 text-center text-slate-500" colSpan={7}>
                  暂无数据，点击查询加载元数据
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      <div className="rounded-lg border border-slate-200 bg-slate-50 p-4">
        <h3 className="text-base font-semibold text-slate-950">{form.id ? `编辑元数据 #${form.id}` : '新增元数据'}</h3>
        <div className="mt-3 grid gap-3 sm:grid-cols-2">
          <input className="rounded-md border border-slate-300 px-3 py-2" placeholder="分类" value={form.category} onChange={(e) => setForm((f) => ({ ...f, category: e.target.value }))} />
          <input
            className="rounded-md border border-slate-300 px-3 py-2"
            placeholder="组件名称"
            value={form.component_name}
            onChange={(e) => setForm((f) => ({ ...f, component_name: e.target.value }))}
          />
          <input
            className="rounded-md border border-slate-300 px-3 py-2 font-mono"
            placeholder="核心 HS6"
            value={form.core_hs6}
            onChange={(e) => setForm((f) => ({ ...f, core_hs6: e.target.value }))}
          />
          <input
            className="rounded-md border border-slate-300 px-3 py-2"
            type="number"
            placeholder="排序"
            value={form.sort_order}
            onChange={(e) => setForm((f) => ({ ...f, sort_order: Number(e.target.value) }))}
          />
          <label className="flex items-center gap-2 text-sm text-slate-700">
            <input type="checkbox" checked={form.enabled} onChange={(e) => setForm((f) => ({ ...f, enabled: e.target.checked }))} />
            启用
          </label>
          <textarea
            className="rounded-md border border-slate-300 px-3 py-2 sm:col-span-2"
            placeholder="描述"
            rows={2}
            value={form.description}
            onChange={(e) => setForm((f) => ({ ...f, description: e.target.value }))}
          />
        </div>
        <div className="mt-4 flex flex-wrap gap-2">
          <button
            type="button"
            className="rounded-md bg-emerald-600 px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
            disabled={busy || !form.component_name.trim() || !/^\d{6}$/.test(form.core_hs6.trim())}
            onClick={() => void saveForm()}
          >
            保存
          </button>
          {form.id > 0 && (
            <button type="button" className="rounded-md bg-red-600 px-4 py-2 text-sm font-medium text-white disabled:opacity-50" disabled={busy} onClick={() => void deleteCurrent()}>
              删除
            </button>
          )}
          <button type="button" className="rounded-md border border-slate-300 px-4 py-2 text-sm" onClick={resetForm}>
            清空
          </button>
        </div>
      </div>
    </div>
  )
}
