import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { HsResolvePage } from './HsResolvePage'

const { hsResolveByModel } = vi.hoisted(() => ({
  hsResolveByModel: vi.fn(),
}))

vi.mock('../api', async () => {
  const actual = await vi.importActual<Record<string, unknown>>('../api')
  return {
    ...actual,
    hsResolveByModel,
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
})
