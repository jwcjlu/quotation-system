import { useEffect, useMemo, useState } from 'react'
import { exportSessionFile, getSession, putPlatforms, type GetSessionReply } from '../../api'

interface SessionMaintenancePanelProps {
  sessionId: string
}

const PLATFORM_OPTIONS = ['find_chips', 'hqchip', 'icgoo', 'ickey', 'szlcsc']

export function SessionMaintenancePanel({ sessionId }: SessionMaintenancePanelProps) {
  const [session, setSession] = useState<GetSessionReply | null>(null)
  const [platformIds, setPlatformIds] = useState<string[]>([])
  const [keyword, setKeyword] = useState('')
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    let cancelled = false
    ;(async () => {
      try {
        const reply = await getSession(sessionId)
        if (!cancelled) {
          setSession(reply)
          setPlatformIds(reply.platform_ids ?? [])
        }
      } catch {
        if (!cancelled) {
          setSession(null)
          setPlatformIds([])
        }
      }
    })()
    return () => {
      cancelled = true
    }
  }, [sessionId])

  const platforms = useMemo(
    () => PLATFORM_OPTIONS.filter((item) => item.toLowerCase().includes(keyword.trim().toLowerCase())),
    [keyword]
  )

  const togglePlatform = (platformId: string) => {
    setPlatformIds((current) =>
      current.includes(platformId)
        ? current.filter((item) => item !== platformId)
        : [...current, platformId]
    )
  }

  const savePlatforms = async () => {
    setSaving(true)
    try {
      const reply = await putPlatforms(sessionId, platformIds, session?.selection_revision)
      setSession((current) =>
        current ? { ...current, selection_revision: reply.selection_revision, platform_ids: platformIds } : current
      )
    } finally {
      setSaving(false)
    }
  }

  const exportXlsx = async () => {
    const result = await exportSessionFile(sessionId, 'xlsx')
    const url = URL.createObjectURL(result.blob)
    const link = document.createElement('a')
    link.href = url
    link.download = result.filename
    link.click()
    URL.revokeObjectURL(url)
  }

  return (
    <section className="space-y-4" data-testid="session-maintenance-panel">
      <div className="rounded-lg border border-slate-200 bg-white p-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h4 className="font-semibold text-slate-900">会话维护</h4>
            <p className="mt-1 text-sm text-slate-500">{session?.title || sessionId}</p>
          </div>
          <div className="flex gap-2">
            <button type="button" onClick={() => void exportXlsx()} className="rounded border border-slate-300 px-3 py-2 text-sm">导出</button>
            <button type="button" disabled={saving} onClick={() => void savePlatforms()} className="rounded bg-slate-900 px-3 py-2 text-sm font-medium text-white disabled:opacity-50">
              {saving ? '保存中...' : '保存平台'}
            </button>
          </div>
        </div>
      </div>
      <div className="rounded-lg border border-slate-200 bg-white p-4">
        <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_12rem]">
          <input value={keyword} onChange={(event) => setKeyword(event.target.value)} placeholder="平台 / 厂家 / 字段" className="rounded border border-slate-300 px-3 py-2 text-sm" />
          <div className="rounded border border-slate-200 px-3 py-2 text-sm text-slate-500">版本 {session?.selection_revision ?? '-'}</div>
        </div>
        <div className="mt-4 grid gap-2 md:grid-cols-2 xl:grid-cols-3">
          {platforms.map((platformId) => (
            <label key={platformId} className="flex items-center gap-2 rounded border border-slate-200 p-3 text-sm">
              <input type="checkbox" checked={platformIds.includes(platformId)} onChange={() => togglePlatform(platformId)} />
              <span className="font-mono">{platformId}</span>
            </label>
          ))}
        </div>
      </div>
    </section>
  )
}
