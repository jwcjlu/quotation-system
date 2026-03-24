/** 与后端 proto JSON（camelCase）对齐，读取时兼容 snake_case */

export interface ParsedItem {
  index: number
  raw: string
  model: string
  manufacturer: string
  package: string
  quantity: number
  params: string
}

export interface PlatformQuote {
  platform: string
  matched_model: string
  manufacturer: string
  package: string
  description: string
  stock: number
  lead_time: string
  moq: number
  increment: number
  price_tiers: string
  hk_price: string
  mainland_price: string
  unit_price: number
  subtotal: number
}

export interface MatchItem {
  index: number
  model: string
  quantity: number
  matched_model: string
  manufacturer: string
  platform: string
  lead_time: string
  stock: number
  unit_price: number
  subtotal: number
  match_status: string
  all_quotes: PlatformQuote[]
  demand_manufacturer: string
  demand_package: string
}

/** 接口清单 §10 平台枚举 */
export const PLATFORM_IDS = ['find_chips', 'hqchip', 'icgoo', 'ickey', 'szlcsc'] as const
export type PlatformId = (typeof PLATFORM_IDS)[number]

export interface CreateSessionReply {
  session_id: string
  biz_date: string
  selection_revision: number
}

export interface GetSessionReply {
  session_id: string
  title: string
  status: string
  biz_date: string
  selection_revision: number
  platform_ids: string[]
}

export interface GetReadinessReply {
  session_id: string
  biz_date: string
  selection_revision: number
  phase: string
  can_enter_match: boolean
  block_reason: string
}

export interface PlatformGap {
  platform_id: string
  phase: string
  reason_code: string
  message: string
  auto_attempt: number
  manual_attempt: number
}

export interface BOMLineRow {
  line_id: string
  line_no: number
  mpn: string
  mfr: string
  package: string
  qty: number
  match_status: string
  platform_gaps: PlatformGap[]
}

export interface GetBOMLinesReply {
  lines: BOMLineRow[]
}

export interface MatchHistoryListItem {
  match_result_id: number
  session_id: string
  version: number
  strategy: string
  created_at: string
  total_amount: number
}

export interface ListMatchHistoryReply {
  items: MatchHistoryListItem[]
  total: number
}
