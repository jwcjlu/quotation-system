import { useState } from 'react'
import { HsItemsSearchPanel } from './hs-meta/HsItemsSearchPanel'
import { MetaCrudPanel } from './hs-meta/MetaCrudPanel'
import { SyncJobsPanel } from './hs-meta/SyncJobsPanel'

type TabKey = 'meta' | 'sync' | 'items'

const tabs: Array<{ key: TabKey; label: string }> = [
  { key: 'meta', label: '分类元数据' },
  { key: 'sync', label: '同步任务' },
  { key: 'items', label: 'HS 条目查询' },
]

export function HsMetaAdminPage() {
  const [tab, setTab] = useState<TabKey>('meta')

  return (
    <section className="space-y-5 rounded-xl border border-slate-200 bg-white p-6 shadow-sm">
      <div>
        <h2 className="text-xl font-semibold text-slate-800">HS元数据</h2>
        <p className="mt-2 text-sm text-slate-600">维护 HS 元数据、执行同步任务，并查询落库条目。</p>
      </div>

      <div className="flex flex-wrap gap-2">
        {tabs.map((item) => (
          <button
            key={item.key}
            type="button"
            onClick={() => setTab(item.key)}
            className={`rounded-md px-4 py-2 text-sm font-medium transition ${
              tab === item.key ? 'bg-slate-900 text-white' : 'border border-slate-300 text-slate-700 hover:bg-slate-50'
            }`}
          >
            {item.label}
          </button>
        ))}
      </div>

      {tab === 'meta' && <MetaCrudPanel />}
      {tab === 'sync' && <SyncJobsPanel />}
      {tab === 'items' && <HsItemsSearchPanel />}
    </section>
  )
}
