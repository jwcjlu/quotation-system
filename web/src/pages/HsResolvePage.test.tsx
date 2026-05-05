import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { HsResolvePage } from './HsResolvePage'

const { hsResolveByModel, hsResolveTask, uploadHsManualDatasheet, hsResolveConfirm, hsListPendingReviews } = vi.hoisted(() => ({
  hsResolveByModel: vi.fn(),
  hsResolveTask: vi.fn(),
  uploadHsManualDatasheet: vi.fn(),
  hsResolveConfirm: vi.fn(),
  hsListPendingReviews: vi.fn(),
}))

vi.mock('../api', async () => {
  const actual = await vi.importActual<Record<string, unknown>>('../api')
  return {
    ...actual,
    hsResolveByModel,
    hsResolveTask,
    uploadHsManualDatasheet,
    hsResolveConfirm,
    hsListPendingReviews,
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

  it('confirms selected HS candidate from candidate table', async () => {
    hsResolveByModel.mockResolvedValue({
      accepted: false,
      task_id: 'task-3',
      run_id: 'run-3',
      decision_mode: 'manual',
      task_status: 'done',
      result_status: 'pending_review',
      best_code_ts: '',
      best_score: 0.89,
      candidates: [{ candidate_rank: 1, code_ts: '8504409999', score: 0.89, reason: 'manual review' }],
      error_code: '',
      error_message: '',
    })
    hsResolveConfirm.mockResolvedValue({})

    render(<HsResolvePage prefill={{ key: 3, model: 'TPS5430', manufacturer: 'TI' }} />)

    fireEvent.click(screen.getByTestId('hs-resolve-submit'))
    await screen.findByText('8504409999')

    fireEvent.click(screen.getByRole('button', { name: '设为最终编码' }))

    await waitFor(() => {
      expect(hsResolveConfirm).toHaveBeenCalledWith({
        model: 'TPS5430',
        manufacturer: 'TI',
        run_id: 'run-3',
        candidate_rank: 1,
        expected_code_ts: '8504409999',
        confirm_request_id: expect.any(String),
      })
    })
  })

  it('polls task endpoint after async accepted without run_id', async () => {
    hsResolveByModel.mockResolvedValue({
      accepted: true,
      task_id: 'task-4',
      run_id: '',
      decision_mode: 'auto',
      task_status: 'running',
      result_status: 'pending_review',
      best_code_ts: '',
      best_score: 0,
      candidates: [],
      error_code: '',
      error_message: '',
    })
    hsResolveTask.mockResolvedValue({
      accepted: false,
      task_id: 'task-4',
      run_id: 'run-4',
      decision_mode: 'auto',
      task_status: 'success',
      result_status: 'pending_review',
      best_code_ts: '8542399000',
      best_score: 0.72,
      candidates: [{ candidate_rank: 1, code_ts: '8542399000', score: 0.72, reason: 'polled' }],
      error_code: '',
      error_message: '',
    })

    render(<HsResolvePage prefill={{ key: 4, model: 'LM358', manufacturer: 'TI' }} />)
    fireEvent.click(screen.getByTestId('hs-resolve-submit'))

    await waitFor(() => {
      expect(hsResolveTask).toHaveBeenCalledWith('task-4')
    })
    await waitFor(() => {
      expect(screen.getAllByText('8542399000').length).toBeGreaterThan(0)
    })
  })

  it('shows pending review tab and confirms candidate from pending list', async () => {
    hsListPendingReviews.mockResolvedValue({
      items: [
        {
          run_id: 'run-pending-1',
          model: 'SN74LVC1T45',
          manufacturer: 'TI',
          task_status: 'success',
          result_status: 'pending_review',
          best_code_ts: '8542399000',
          best_score: 0.72,
          updated_at: '2026-05-05T11:00:00Z',
          candidates: [{ candidate_rank: 1, code_ts: '8542399000', score: 0.72, reason: 'pending' }],
        },
      ],
      total: 1,
    })
    hsResolveConfirm.mockResolvedValue({})
    render(<HsResolvePage prefill={{ key: 5, model: 'SN74LVC1T45', manufacturer: 'TI' }} />)

    fireEvent.click(screen.getByRole('button', { name: '待人工确认' }))
    await screen.findByText('待人工确认列表（1）')
    fireEvent.click(screen.getByRole('button', { name: '设为最终编码' }))

    await waitFor(() => {
      expect(hsResolveConfirm).toHaveBeenCalledWith(
        expect.objectContaining({
          model: 'SN74LVC1T45',
          manufacturer: 'TI',
          run_id: 'run-pending-1',
          candidate_rank: 1,
          expected_code_ts: '8542399000',
        }),
      )
    })
  })
})
