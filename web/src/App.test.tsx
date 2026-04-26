import { fireEvent, render, screen, within } from '@testing-library/react'
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

vi.mock('./pages/BomWorkbenchPage', () => ({
  BomWorkbenchPage: () => <div>bom workbench page</div>,
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

vi.mock('./pages/GuidePage', () => ({
  GuidePage: () => <div>guide page</div>,
}))

describe('App navigation auth gating', () => {
  beforeEach(() => {
    vi.resetModules()
    vi.clearAllMocks()
    hasSessionToken.mockReturnValue(false)
    subscribeSessionChange.mockReturnValue(vi.fn())
  })

  it('shows bom workbench as the default entry for anonymous users', async () => {
    getMe.mockRejectedValue(new Error('unauthorized'))

    const { default: App } = await import('./App')
    render(<App />)

    expect(screen.getByText('bom workbench page')).toBeInTheDocument()
    expect(await screen.findByRole('button', { name: 'BOM\u5de5\u4f5c\u53f0' })).toBeInTheDocument()
    expect(await screen.findByRole('button', { name: '\u4f7f\u7528\u6307\u5357' })).toBeInTheDocument()
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

    expect(await screen.findByRole('button', { name: 'BOM\u5de5\u4f5c\u53f0' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'HS\u578b\u53f7\u89e3\u6790' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '\u4f7f\u7528\u6307\u5357' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '\u811a\u672c\u5305' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Agent\u8fd0\u7ef4' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'HS\u5143\u6570\u636e' })).not.toBeInTheDocument()
  })

  it('shows every protected page for admins with HS pages before script and agent admin pages', async () => {
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

    expect(await screen.findByRole('button', { name: 'BOM\u5de5\u4f5c\u53f0' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'HS\u578b\u53f7\u89e3\u6790' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '\u811a\u672c\u5305' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Agent\u8fd0\u7ef4' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'HS\u5143\u6570\u636e' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '\u4f7f\u7528\u6307\u5357' })).toBeInTheDocument()

    const labels = within(screen.getByRole('navigation'))
      .getAllByRole('button')
      .map((button) => button.textContent)

    expect(labels).toEqual([
      '\u4f7f\u7528\u6307\u5357',
      'BOM\u5de5\u4f5c\u53f0',
      'HS\u578b\u53f7\u89e3\u6790',
      'HS\u5143\u6570\u636e',
      '\u811a\u672c\u5305',
      'Agent\u8fd0\u7ef4',
    ])
  })

  it('uses dark gray by default and persists the customized navigation bar background', async () => {
    getMe.mockRejectedValue(new Error('unauthorized'))

    const { default: App } = await import('./App')
    render(<App />)

    const picker = await screen.findByLabelText('导航栏背景')
    expect(picker).toHaveValue('#334155')
    expect(screen.getByRole('banner')).toHaveStyle({ backgroundColor: '#334155' })

    fireEvent.change(picker, { target: { value: '#fef3c7' } })

    expect(screen.getByRole('banner')).toHaveStyle({ backgroundColor: '#fef3c7' })
    expect(localStorage.getItem('caichip_nav_color')).toBe('#fef3c7')
  })
})
