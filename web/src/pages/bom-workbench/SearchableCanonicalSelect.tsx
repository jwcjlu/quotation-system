import { useEffect, useMemo, useRef, useState } from 'react'
import type { ManufacturerCanonicalRow } from '../../api'

type SearchableCanonicalSelectProps = {
  value: string
  onChange: (canonicalId: string) => void
  options: ManufacturerCanonicalRow[]
  placeholder?: string
  searchPlaceholder?: string
  'aria-label'?: string
}

export function SearchableCanonicalSelect({
  value,
  onChange,
  options,
  placeholder = '选择标准厂牌',
  searchPlaceholder = '搜索显示名或 canonical_id…',
  'aria-label': ariaLabel,
}: SearchableCanonicalSelectProps) {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const rootRef = useRef<HTMLDivElement>(null)

  const filtered = useMemo(() => {
    const f = query.trim().toLowerCase()
    if (!f) {
      return options
    }
    return options.filter(
      (o) =>
        o.display_name.toLowerCase().includes(f) || o.canonical_id.toLowerCase().includes(f),
    )
  }, [options, query])

  const selectedRow = options.find((o) => o.canonical_id === value)

  useEffect(() => {
    if (!open) {
      return
    }
    const onDocMouseDown = (e: MouseEvent) => {
      if (!rootRef.current?.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener('mousedown', onDocMouseDown)
    return () => document.removeEventListener('mousedown', onDocMouseDown)
  }, [open])

  useEffect(() => {
    if (!open) {
      setQuery('')
    }
  }, [open])

  const label = selectedRow
    ? `${selectedRow.display_name} (${selectedRow.canonical_id})`
    : placeholder

  return (
    <div ref={rootRef} className="relative min-w-[14rem] max-w-[22rem]">
      <button
        type="button"
        role="combobox"
        aria-expanded={open}
        aria-haspopup="listbox"
        aria-label={ariaLabel ?? placeholder}
        onClick={() => setOpen((v) => !v)}
        className="flex h-8 w-full min-w-[14rem] items-center justify-between gap-2 rounded-md border border-[#d7e0ed] bg-white px-3 text-left text-sm text-slate-800"
      >
        <span className="truncate">{label}</span>
        <span className="shrink-0 text-slate-400" aria-hidden>
          {open ? '▴' : '▾'}
        </span>
      </button>
      {open ? (
        <div
          className="absolute z-30 mt-1 w-full min-w-[14rem] max-w-[22rem] rounded-md border border-[#d7e0ed] bg-white py-1 shadow-lg"
          role="listbox"
        >
          <div className="border-b border-[#eef2f7] px-2 pb-1 pt-1">
            <input
              type="search"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder={searchPlaceholder}
              className="h-8 w-full rounded border border-[#d7e0ed] px-2 text-sm outline-none focus:border-[#2457c5]"
              autoComplete="off"
              autoFocus
            />
          </div>
          <ul className="max-h-52 overflow-y-auto py-1 text-sm">
            <li>
              <button
                type="button"
                role="option"
                className="w-full px-3 py-1.5 text-left text-slate-500 hover:bg-slate-50"
                onClick={() => {
                  onChange('')
                  setOpen(false)
                }}
              >
                （清空选择）
              </button>
            </li>
            {filtered.length === 0 ? (
              <li className="px-3 py-2 text-slate-400">无匹配项</li>
            ) : (
              filtered.map((row) => (
                <li key={row.canonical_id}>
                  <button
                    type="button"
                    role="option"
                    aria-selected={row.canonical_id === value}
                    className={`w-full px-3 py-1.5 text-left hover:bg-slate-50 ${
                      row.canonical_id === value ? 'bg-blue-50 font-medium text-slate-900' : 'text-slate-800'
                    }`}
                    onClick={() => {
                      onChange(row.canonical_id)
                      setOpen(false)
                    }}
                  >
                    <span className="block truncate">{row.display_name}</span>
                    <span className="block truncate font-mono text-xs text-slate-500">{row.canonical_id}</span>
                  </button>
                </li>
              ))
            )}
          </ul>
        </div>
      ) : null}
    </div>
  )
}
