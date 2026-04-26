import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { AuthPanel } from './AuthPanel'

const { clearSessionToken, logout, register } = vi.hoisted(() => ({
  clearSessionToken: vi.fn(),
  logout: vi.fn(),
  register: vi.fn(),
}))

vi.mock('../api/auth', () => ({
  login: vi.fn(),
  logout,
  register,
}))

vi.mock('../auth/session', () => ({
  clearSessionToken,
  setSessionToken: vi.fn(),
}))

describe('AuthPanel', () => {
  it('loads a saved custom avatar for the current user', async () => {
    localStorage.setItem('auth_avatar:admin', 'data:image/png;base64,avatar')

    render(
      <AuthPanel
        currentUser={{ id: '1', username: 'admin', displayName: 'Admin', role: 'admin', status: 'active' }}
        onAuthenticated={vi.fn()}
        onLoggedOut={vi.fn()}
      />,
    )

    expect(await screen.findByAltText('Admin 头像')).toHaveAttribute('src', 'data:image/png;base64,avatar')
    expect(screen.getByRole('button', { name: 'Admin 用户菜单' })).toBeInTheDocument()
  })

  it('shows a GitHub-style account menu with avatar actions and logout', async () => {
    const onLoggedOut = vi.fn()
    localStorage.clear()

    render(
      <AuthPanel
        currentUser={{ id: '1', username: 'admin', displayName: 'Admin', role: 'admin', status: 'active' }}
        onAuthenticated={vi.fn()}
        onLoggedOut={onLoggedOut}
      />,
    )

    expect(screen.getByText('A')).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: 'Admin 用户菜单' }))

    expect(screen.getByRole('menu')).toBeInTheDocument()
    expect(screen.getByText('Signed in as')).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: '更换头像' })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('menuitem', { name: '退出登录' }))
    expect(logout).toHaveBeenCalled()
    await waitFor(() => expect(clearSessionToken).toHaveBeenCalled())
    expect(onLoggedOut).toHaveBeenCalled()
  })

  it('uses a subdued account button on dark navigation backgrounds', () => {
    render(
      <AuthPanel
        currentUser={{ id: '1', username: 'admin', displayName: 'Admin', role: 'admin', status: 'active' }}
        navIsDark
        onAuthenticated={vi.fn()}
        onLoggedOut={vi.fn()}
      />,
    )

    expect(screen.getByRole('button', { name: 'Admin 用户菜单' })).toHaveClass('bg-white/10')
  })

  it('validates register password length before submit', () => {
    render(<AuthPanel currentUser={null} onAuthenticated={vi.fn()} onLoggedOut={vi.fn()} />)

    fireEvent.click(screen.getByRole('button', { name: '注册' }))
    fireEvent.change(screen.getByLabelText('用户名'), { target: { value: 'alice' } })
    fireEvent.change(screen.getByLabelText('显示名'), { target: { value: 'Alice' } })
    fireEvent.change(screen.getByLabelText('密码'), { target: { value: '123' } })
    fireEvent.click(screen.getByRole('button', { name: '创建账号' }))

    expect(register).not.toHaveBeenCalled()
    expect(screen.getByText('密码至少 8 位')).toBeInTheDocument()
  })
})
