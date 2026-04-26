import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { getMe, hasSessionToken } = vi.hoisted(() => ({
  getMe: vi.fn(),
  hasSessionToken: vi.fn(() => false),
}))

const subscribeSessionChange = vi.fn(() => vi.fn())

vi.mock('./api/auth', () => ({
  getMe,
  login: vi.fn(),
  logout: vi.fn(),
  register: vi.fn(),
}))

vi.mock('./auth/session', () => ({
  clearSessionToken: vi.fn(),
  getSessionToken: vi.fn(() => (hasSessionToken() ? 'test-token' : null)),
  hasSessionToken,
  setSessionToken: vi.fn(),
  subscribeSessionChange,
}))

vi.mock('./pages/BomSessionListPage', () => ({
  BomSessionListPage: () => <div>bom session page</div>,
}))

vi.mock('./pages/MatchResultPage', () => ({
  MatchResultPage: () => <div>match result page</div>,
}))

vi.mock('./pages/AgentScriptsPage', () => ({
  AgentScriptsPage: () => <div>agent scripts page</div>,
}))

vi.mock('./pages/AgentAdminPage', () => ({
  AgentAdminPage: () => <div>agent admin page</div>,
}))

vi.mock('./pages/HsResolvePage', () => ({
  HsResolvePage: () => <div>hs resolve page</div>,
}))

vi.mock('./pages/HsMetaAdminPage', () => ({
  HsMetaAdminPage: () => <div>hs meta page</div>,
}))

describe('App navigation auth gating', () => {
  beforeEach(() => {
    vi.resetModules()
    vi.clearAllMocks()
    localStorage.clear()
    hasSessionToken.mockReturnValue(false)
    subscribeSessionChange.mockReturnValue(vi.fn())
  })

  it('shows bom list as the default entry for anonymous users', async () => {
    getMe.mockRejectedValue(new Error('unauthorized'))

    const { default: App } = await import('./App')
    render(<App />)

    expect(screen.getByText('bom session page')).toBeInTheDocument()
    expect(await screen.findByRole('button', { name: 'BOM\u4f1a\u8bdd' })).toBeInTheDocument()
    expect(await screen.findByRole('button', { name: '\u4f7f\u7528\u6307\u5357' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '\u5339\u914d\u5355' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'HS\u578b\u53f7\u89e3\u6790' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '\u811a\u672c\u5305' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Agent\u8fd0\u7ef4' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'HS\u5143\u6570\u636e' })).not.toBeInTheDocument()
  })

  it('shows only user pages for a normal user', async () => {
    hasSessionToken.mockReturnValue(true)
    getMe.mockResolvedValue({
      user: {
        id: 'u-1',
        username: 'demo',
        role: 'user',
      },
    })

    const { default: App } = await import('./App')
    render(<App />)

    expect(await screen.findByRole('button', { name: 'BOM\u4f1a\u8bdd' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '\u5339\u914d\u5355' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'HS\u578b\u53f7\u89e3\u6790' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '\u4f7f\u7528\u6307\u5357' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '\u811a\u672c\u5305' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Agent\u8fd0\u7ef4' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'HS\u5143\u6570\u636e' })).not.toBeInTheDocument()
  })

  it('shows every protected page for admins', async () => {
    hasSessionToken.mockReturnValue(true)
    getMe.mockResolvedValue({
      user: {
        id: 'admin-1',
        username: 'root',
        role: 'admin',
      },
    })

    const { default: App } = await import('./App')
    render(<App />)

    expect(await screen.findByRole('button', { name: 'BOM\u4f1a\u8bdd' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '\u5339\u914d\u5355' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'HS\u578b\u53f7\u89e3\u6790' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '\u811a\u672c\u5305' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Agent\u8fd0\u7ef4' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'HS\u5143\u6570\u636e' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '\u4f7f\u7528\u6307\u5357' })).toBeInTheDocument()
  })
})
