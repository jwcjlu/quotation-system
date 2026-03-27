/** 与后端 t_bom_session 列宽一致，提交前校验减少 4xx/截断问题 */

export const SESSION_FIELD_MAX = {
  title: 256,
  customer_name: 256,
  contact_phone: 64,
  contact_email: 256,
  contact_extra: 512,
} as const

export const READINESS_MODES = ['lenient', 'strict'] as const
export type ReadinessMode = (typeof READINESS_MODES)[number]

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

export function validateSessionHeaderFields(f: {
  title: string
  customerName: string
  contactPhone: string
  contactEmail: string
  contactExtra: string
}): string | null {
  if (f.title.length > SESSION_FIELD_MAX.title) {
    return `标题长度不能超过 ${SESSION_FIELD_MAX.title}`
  }
  if (f.customerName.length > SESSION_FIELD_MAX.customer_name) {
    return `客户名称不能超过 ${SESSION_FIELD_MAX.customer_name} 字`
  }
  if (f.contactPhone.length > SESSION_FIELD_MAX.contact_phone) {
    return `电话不能超过 ${SESSION_FIELD_MAX.contact_phone} 字`
  }
  if (f.contactEmail.length > SESSION_FIELD_MAX.contact_email) {
    return `邮箱不能超过 ${SESSION_FIELD_MAX.contact_email} 字`
  }
  if (f.contactEmail.trim() !== '' && !EMAIL_RE.test(f.contactEmail.trim())) {
    return '邮箱格式不正确'
  }
  if (f.contactExtra.length > SESSION_FIELD_MAX.contact_extra) {
    return `备注不能超过 ${SESSION_FIELD_MAX.contact_extra} 字`
  }
  return null
}
