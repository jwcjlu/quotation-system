import { fetchJson } from './http'

export type UserRole = 'user' | 'admin'

export interface AuthUser {
  id: string
  username: string
  displayName: string
  role: UserRole
  status: string
}

export interface RegisterParams {
  username: string
  displayName: string
  password: string
}

export interface LoginParams {
  username: string
  password: string
}

export interface AdminCreateUserParams extends RegisterParams {
  role: UserRole
}

export interface AuthUserReply {
  user: AuthUser
}

export interface LoginReply extends AuthUserReply {
  sessionToken: string
}

function stringValue(value: unknown): string {
  return typeof value === 'string' ? value : value == null ? '' : String(value)
}

function normalizeRole(value: unknown): UserRole {
  return value === 'admin' ? 'admin' : 'user'
}

function normalizeUser(input: unknown): AuthUser {
  const record = (input ?? {}) as Record<string, unknown>
  return {
    id: stringValue(record.id ?? record.user_id ?? record.userId),
    username: stringValue(record.username),
    displayName: stringValue(record.display_name ?? record.displayName),
    role: normalizeRole(record.role),
    status: stringValue(record.status) || 'active',
  }
}

function normalizeUserReply(input: unknown): AuthUserReply {
  const record = (input ?? {}) as Record<string, unknown>
  return {
    user: normalizeUser(record.user ?? input),
  }
}

export async function register(params: RegisterParams): Promise<AuthUserReply> {
  const payload = await fetchJson<Record<string, unknown>>('/api/v1/auth/register', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      username: params.username.trim(),
      display_name: params.displayName.trim(),
      password: params.password,
    }),
  })
  return normalizeUserReply(payload)
}

export async function login(params: LoginParams): Promise<LoginReply> {
  const payload = await fetchJson<Record<string, unknown>>('/api/v1/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      username: params.username.trim(),
      password: params.password,
    }),
  })
  const record = payload as Record<string, unknown>
  return {
    sessionToken: stringValue(record.session_token ?? record.sessionToken),
    user: normalizeUser(record.user),
  }
}

export async function getMe(): Promise<AuthUserReply> {
  const payload = await fetchJson<Record<string, unknown>>('/api/v1/auth/me')
  return normalizeUserReply(payload)
}

export async function logout(): Promise<void> {
  await fetchJson<Record<string, unknown>>('/api/v1/auth/logout', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({}),
  })
}

export async function adminCreateUser(params: AdminCreateUserParams): Promise<AuthUserReply> {
  const payload = await fetchJson<Record<string, unknown>>('/api/v1/auth/admin/users', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      username: params.username.trim(),
      display_name: params.displayName.trim(),
      password: params.password,
      role: params.role,
    }),
  })
  return normalizeUserReply(payload)
}
