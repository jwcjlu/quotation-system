import type { ReactNode } from 'react'

interface ToolPageShellProps {
  testId: string
  eyebrow: string
  title: string
  description: string
  children: ReactNode
}

export function ToolPageShell({
  testId,
  eyebrow,
  title,
  description,
  children,
}: ToolPageShellProps) {
  return (
    <section className="bg-[#f4f6fa] text-slate-950" data-testid={testId}>
      <div className="overflow-hidden rounded-lg border border-[#cbd6e5] bg-white">
        <header className="border-b border-[#d7e0ed] bg-[#f7f9fc] px-6 py-4 text-slate-950">
          <div className="flex flex-wrap items-end justify-between gap-4">
            <div className="min-w-0">
              <p className="text-xs font-semibold uppercase text-[#2d65ad]">{eyebrow}</p>
              <h2 className="mt-2 text-2xl font-bold leading-tight">{title}</h2>
            </div>
            <p className="max-w-2xl text-sm leading-6 text-slate-600 lg:text-right">{description}</p>
          </div>
        </header>
        <div className="space-y-4 bg-[#f8fafc] p-4 lg:p-6">{children}</div>
      </div>
    </section>
  )
}
