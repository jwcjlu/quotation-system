import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { UploadPage } from './UploadPage'

const { createSession, uploadBOM } = vi.hoisted(() => ({
  createSession: vi.fn(),
  uploadBOM: vi.fn(),
}))

vi.mock('../api', async () => {
  const actual = await vi.importActual<Record<string, unknown>>('../api')
  return {
    ...actual,
    PLATFORM_IDS: ['digikey'],
    createSession,
    uploadBOM,
    downloadTemplate: vi.fn(),
  }
})

describe('UploadPage', () => {
  it('redirects immediately after accepted llm upload', async () => {
    createSession.mockResolvedValue({ session_id: 'session-1' })
    uploadBOM.mockResolvedValue({
      bom_id: 'session-1',
      accepted: true,
      import_status: 'parsing',
      import_message: 'import started',
      items: [],
      total: 0,
    })
    const onSuccess = vi.fn()

    render(<UploadPage onSuccess={onSuccess} />)

    const file = new File(['bom'], 'bom.xlsx', {
      type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
    })

    fireEvent.change(screen.getByTestId('upload-file-input'), {
      target: { files: [file] },
    })
    fireEvent.click(screen.getByTestId('upload-submit-button'))

    await waitFor(() => {
      expect(onSuccess).toHaveBeenCalledWith('session-1')
    })
  })
})
