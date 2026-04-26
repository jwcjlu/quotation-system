import { fetchJson } from './http'

export interface HsMetaListParams {
  page?: number
  page_size?: number
  category?: string
  component_name?: string
  core_hs6?: string
  enabled?: string
}

export interface HsMetaRow {
  id: number
  category: string
  component_name: string
  core_hs6: string
  description: string
  enabled: boolean
  sort_order: number
  updated_at: string
}

export interface HsMetaListReply {
  items: HsMetaRow[]
  total: number
}

export interface HsMetaCreateParams {
  category: string
  component_name: string
  core_hs6: string
  description?: string
  enabled?: boolean
  sort_order?: number
}

export interface HsMetaUpdateParams extends HsMetaCreateParams {
  id: number
}

export interface HsMetaDeleteParams {
  id: number
}

export interface HsSyncRunParams {
  mode: 'all_enabled' | 'selected'
  core_hs6?: string[]
}

export interface HsSyncJobsParams {
  page?: number
  page_size?: number
}

export interface HsItemsListParams {
  code_ts?: string
  g_name?: string
  source_core_hs6?: string
  page?: number
  page_size?: number
}

function buildQuery(params: Record<string, string | number | undefined>): string {
  const search = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === null || value === '') continue
    search.set(key, String(value))
  }
  const query = search.toString()
  return query ? `?${query}` : ''
}

function normalizeMetaRow(input: unknown): HsMetaRow {
  const row = (input ?? {}) as Record<string, unknown>
  return {
    id: Number(row.id ?? 0),
    category: String(row.category ?? ''),
    component_name: String(row.component_name ?? ''),
    core_hs6: String(row.core_hs6 ?? ''),
    description: String(row.description ?? ''),
    enabled: Boolean(row.enabled),
    sort_order: Number(row.sort_order ?? 0),
    updated_at: String(row.updated_at ?? ''),
  }
}

export async function hsMetaList(params: HsMetaListParams): Promise<HsMetaListReply> {
  const query = buildQuery({
    page: params.page,
    page_size: params.page_size,
    category: params.category,
    component_name: params.component_name,
    core_hs6: params.core_hs6,
    enabled: params.enabled,
  })
  const reply = await fetchJson<Record<string, unknown>>(`/api/hs/meta/list${query}`)
  const items = Array.isArray(reply.items) ? reply.items.map(normalizeMetaRow) : []
  return {
    items,
    total: Number(reply.total ?? items.length),
  }
}

export async function hsMetaCreate(params: HsMetaCreateParams): Promise<Record<string, unknown>> {
  return fetchJson('/api/hs/meta/create', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(params),
  })
}

export async function hsMetaUpdate(params: HsMetaUpdateParams): Promise<Record<string, unknown>> {
  return fetchJson('/api/hs/meta/update', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(params),
  })
}

export async function hsMetaDelete(params: HsMetaDeleteParams): Promise<Record<string, unknown>> {
  return fetchJson('/api/hs/meta/delete', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(params),
  })
}

export async function hsSyncRun(params: HsSyncRunParams): Promise<Record<string, unknown>> {
  return fetchJson('/api/hs/sync/run', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(params),
  })
}

export async function hsSyncJobs(params: HsSyncJobsParams): Promise<Record<string, unknown>> {
  const query = buildQuery({ page: params.page, page_size: params.page_size })
  return fetchJson(`/api/hs/sync/jobs${query}`)
}

export async function hsSyncJobDetail(id: string): Promise<Record<string, unknown>> {
  const query = buildQuery({ id })
  return fetchJson(`/api/hs/sync/job_detail${query}`)
}

export async function hsItemsList(params: HsItemsListParams): Promise<Record<string, unknown>> {
  const query = buildQuery({
    code_ts: params.code_ts,
    g_name: params.g_name,
    source_core_hs6: params.source_core_hs6,
    page: params.page,
    page_size: params.page_size,
  })
  return fetchJson(`/api/hs/items${query}`)
}

export async function hsItemDetail(codeTs: string): Promise<Record<string, unknown>> {
  return fetchJson(`/api/hs/items/${encodeURIComponent(codeTs.trim())}`)
}
