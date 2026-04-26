import { useState } from 'react'
import { HsItemsSearchPanel } from './hs-meta/HsItemsSearchPanel'
import { MetaCrudPanel } from './hs-meta/MetaCrudPanel'
import { SyncJobsPanel } from './hs-meta/SyncJobsPanel'
import { ToolPageShell } from './ToolPageShell'

type TabKey = 'meta' | 'sync' | 'items'

const tabs: Array<{ key: TabKey; label: string }> = [
  { key: 'meta', label: '分类元数据' },
  { key: 'sync', label: '同步任务' },
  { key: 'items', label: 'HS 条目查询' },
]

export function HsMetaAdminPage() {
  const [tab, setTab] = useState<TabKey>('meta')

  return (
    <ToolPageShell
      testId="hs-meta-page"
      eyebrow="HS META"
      title="HS元数据"
      description="维护 HS 分类元数据、同步任务和海关条目查询。"
    >

      <div className="flex flex-wrap gap-3 overflow-x-auto rounded-lg border border-[#d7e0ed] bg-white px-4 py-2">
        {tabs.map((item) => (
          <button
            key={item.key}
            type="button"
            onClick={() => setTab(item.key)}
            className={`rounded-md px-4 py-2 text-sm font-semibold transition ${
              tab === item.key ? 'bg-[#e8eef7] text-[#244a86]' : 'text-slate-700 hover:bg-slate-100'
            }`}
          >
            {item.label}
          </button>
        ))}
      </div>

      {tab === 'meta' && <MetaCrudPanel />}
      {tab === 'sync' && <SyncJobsPanel />}
      {tab === 'items' && <HsItemsSearchPanel />}
    </ToolPageShell>
  )
}
