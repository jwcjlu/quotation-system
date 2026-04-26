import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { HsResolvePage } from './HsResolvePage'

const { hsResolveByModel, uploadHsManualDatasheet } = vi.hoisted(() => ({
  hsResolveByModel: vi.fn(),
  uploadHsManualDatasheet: vi.fn(),
}))

vi.mock('../api', async () => {
  const actual = await vi.importActual<Record<string, unknown>>('../api')
  return {
    ...actual,
    hsResolveByModel,
    uploadHsManualDatasheet,
    hsResolveConfirm: vi.fn(),
  }
})

describe('HsResolvePage', () => {
  it('shows the working resolver UI instead of the old auth placeholder', async () => {
    hsResolveByModel.mockResolvedValue({
      accepted: true,
      task_id: 'task-1',
      run_id: 'run-1',
      decision_mode: 'auto',
      task_status: 'done',
      result_status: 'resolved',
      best_code_ts: '8542399000',
      best_score: 0.91,
      candidates: [{ candidate_rank: 1, code_ts: '8542399000', score: 0.91, reason: 'model match' }],
      error_code: '',
      error_message: '',
    })

    render(<HsResolvePage prefill={{ key: 1, model: 'STM32F103C8T6', manufacturer: 'ST' }} />)

    expect(screen.queryByText(/login|auth|permission/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/\u95e8\u7981/)).not.toBeInTheDocument()
    expect(screen.getByLabelText('\u578b\u53f7')).toHaveValue('STM32F103C8T6')
    expect(screen.getByLabelText('\u5382\u724c')).toHaveValue('ST')

    fireEvent.click(screen.getByTestId('hs-resolve-submit'))

    await waitFor(() => {
      expect(hsResolveByModel).toHaveBeenCalledWith(
        expect.objectContaining({
          model: 'STM32F103C8T6',
          manufacturer: 'ST',
        }),
      )
    })
    await waitFor(() => {
      expect(screen.getAllByText('8542399000').length).toBeGreaterThan(0)
    })
  })

  it('uploads a manual datasheet and submits manual inputs with resolve request', async () => {
    uploadHsManualDatasheet.mockResolvedValue({
      upload_id: 'upload-1',
      expires_at_unix: 1818768000,
      content_sha256: 'sha-1',
    })
    hsResolveByModel.mockResolvedValue({
      accepted: true,
      task_id: 'task-2',
      run_id: 'run-2',
      decision_mode: 'manual',
      task_status: 'done',
      result_status: 'pending_review',
      best_code_ts: '',
      best_score: 0,
      candidates: [],
      error_code: '',
      error_message: '',
    })

    render(<HsResolvePage prefill={{ key: 2, model: 'GD25Q16CSIGR', manufacturer: 'GigaDevice' }} />)

    fireEvent.change(screen.getByLabelText('手动描述'), {
      target: { value: '16Mbit SPI NOR Flash, SOIC-8 package' },
    })
    fireEvent.change(screen.getByLabelText('上传 PDF 手册'), {
      target: {
        files: [new File(['%PDF-1.7 manual'], 'gd25q16.pdf', { type: 'application/pdf' })],
      },
    })

    await waitFor(() => {
      expect(uploadHsManualDatasheet).toHaveBeenCalledWith(expect.any(File))
    })
    expect(await screen.findByText(/gd25q16\.pdf/)).toBeInTheDocument()

    fireEvent.click(screen.getByTestId('hs-resolve-submit'))

    await waitFor(() => {
      expect(hsResolveByModel).toHaveBeenCalledWith(
        expect.objectContaining({
          model: 'GD25Q16CSIGR',
          manufacturer: 'GigaDevice',
          manual_component_description: '16Mbit SPI NOR Flash, SOIC-8 package',
          manual_upload_id: 'upload-1',
        }),
      )
    })
  })
})
