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
  /** 型号/封装已对齐但厂牌与需求不一致的报价原文（去重），与后端 MatchItem 扩展字段对齐 */
  mfr_mismatch_quote_manufacturers?: string[]
  /** HS：hs_found | hs_not_mapped | hs_code_invalid（与后端一致） */
  hs_code_status?: string
  code_ts?: string
  control_mark?: string
  import_tax_g_name?: string
  import_tax_imp_ordinary_rate?: string
  import_tax_imp_discount_rate?: string
  import_tax_imp_temp_rate?: string
  /** 分号拼接分项错误码 */
  hs_customs_error?: string
}

/** 是否展示「一键找 HS」（与 design：未映射或编码非法） */
export function matchItemNeedsHsResolve(item: MatchItem): boolean {
  const s = (item.hs_code_status || '').trim()
  return s === 'hs_not_mapped' || s === 'hs_code_invalid'
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
  customer_name?: string
  contact_phone?: string
  contact_email?: string
  contact_extra?: string
  readiness_mode?: string
}

export interface SessionListItem {
  session_id: string
  title: string
  customer_name: string
  status: string
  biz_date: string
  updated_at: string
  line_count: number
}

export interface ListSessionsReply {
  items: SessionListItem[]
  total: number
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
  /** pending | searching | succeeded | failed | missing */
  search_ui_state?: string
}

export interface GetSessionSearchTaskCoverageReply {
  consistent: boolean
  orphan_task_count: number
  expected_task_count: number
  existing_task_count: number
  missing_tasks: Array<{
    line_id: string
    line_no: number
    mpn_norm: string
    platform_id: string
    reason: string
  }>
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
