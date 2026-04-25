export interface HsResolvePrefill {
  key: number
  model: string
  manufacturer: string
}

export function HsResolvePage({ prefill }: { prefill?: HsResolvePrefill | null }) {
  return (
    <section className="rounded-xl border border-slate-200 bg-white p-6 shadow-sm">
      <h2 className="text-xl font-semibold text-slate-800">HS 归类</h2>
      <p className="mt-2 text-sm text-slate-600">
        {prefill?.model ? `${prefill.model} ${prefill.manufacturer}` : '请选择待归类型号。'}
      </p>
    </section>
  )
}
