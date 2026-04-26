import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { AuthPanel } from './AuthPanel'

const { register } = vi.hoisted(() => ({
  register: vi.fn(),
}))

vi.mock('../api/auth', () => ({
  login: vi.fn(),
  logout: vi.fn(),
  register,
}))

vi.mock('../auth/session', () => ({
  clearSessionToken: vi.fn(),
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

    expect(await screen.findByAltText('用户头像')).toHaveAttribute('src', 'data:image/png;base64,avatar')
    expect(screen.getByRole('button', { name: '退出' })).toBeInTheDocument()
  })

  it('validates register password length before submit', () => {
    render(<AuthPanel currentUser={null} onAuthenticated={vi.fn()} onLoggedOut={vi.fn()} />)

    fireEvent.click(screen.getByRole('button', { name: '\u6ce8\u518c' }))
    fireEvent.change(screen.getByLabelText('\u7528\u6237\u540d'), { target: { value: 'alice' } })
    fireEvent.change(screen.getByLabelText('\u663e\u793a\u540d'), { target: { value: 'Alice' } })
    fireEvent.change(screen.getByLabelText('\u5bc6\u7801'), { target: { value: '123' } })
    fireEvent.click(screen.getByRole('button', { name: '\u521b\u5efa\u8d26\u53f7' }))

    expect(register).not.toHaveBeenCalled()
    expect(screen.getByText('\u5bc6\u7801\u81f3\u5c11 8 \u4f4d')).toBeInTheDocument()
  })
})
