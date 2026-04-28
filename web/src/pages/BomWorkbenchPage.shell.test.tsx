import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { BomWorkbenchPage } from './BomWorkbenchPage'

const { listSessions } = vi.hoisted(() => ({
  listSessions: vi.fn(),
}))

vi.mock('../api', async () => {
  const actual = await vi.importActual<Record<string, unknown>>('../api')
  return {
    ...actual,
    listSessions,
  }
})

vi.mock('./UploadPage', () => ({
  UploadPage: () => <div>upload dialog</div>,
}))

describe('BomWorkbenchPage shell', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    listSessions.mockResolvedValue({ total: 0, items: [] })
  })

  it('renders the workbench shell without duplicate inner navigation chrome', () => {
    render(<BomWorkbenchPage />)

    expect(screen.queryByRole('banner')).toBeNull()
    expect(screen.queryByRole('navigation', { name: 'BOM \u5de5\u4f5c\u53f0\u5bfc\u822a' })).toBeNull()
    expect(screen.getByTestId('bom-workbench-shell')).toHaveClass('rounded-lg')
  })
})
