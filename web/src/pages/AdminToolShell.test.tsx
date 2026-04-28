import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { AgentAdminPage } from './AgentAdminPage'
import { AgentScriptsPage } from './AgentScriptsPage'
import { HsMetaAdminPage } from './HsMetaAdminPage'
import { HsResolvePage } from './HsResolvePage'

vi.mock('./BomPlatformsAdminSection', () => ({
  BomPlatformsAdminSection: () => <div>BOM platforms</div>,
}))

describe('admin tool page shells', () => {
  it.each([
    ['agent-scripts-page', <AgentScriptsPage />],
    ['agent-admin-page', <AgentAdminPage />],
    ['hs-resolve-page', <HsResolvePage />],
    ['hs-meta-page', <HsMetaAdminPage />],
  ])('uses the shared workbench shell style for %s', (testId, element) => {
    render(element)

    expect(screen.getByTestId(testId)).toHaveClass('bg-[#f4f6fa]')
    expect(screen.getByTestId(testId)).toHaveTextContent(/\S/)
  })
})
