interface BomPlatformsAdminSectionProps {
  embedded?: boolean
  apiKey?: string
  requireKey?: () => string | null
  resetFlash?: () => void
  setError?: (message: string | null) => void
  setInfo?: (message: string | null) => void
}

export function BomPlatformsAdminSection(_props: BomPlatformsAdminSectionProps) {
  return (
    <section className="rounded-lg border border-slate-200 bg-white p-4">
      <h3 className="font-semibold text-slate-800">BOM 平台配置</h3>
      <p className="mt-2 text-sm text-slate-600">平台配置入口。</p>
    </section>
  )
}
