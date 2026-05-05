import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { ManufacturerAliasReviewPanel } from './ManufacturerAliasReviewPanel'

describe('ManufacturerAliasReviewPanel', () => {
  it('submits selected canonical for pending quote manufacturer', async () => {
    const onApprove = vi.fn().mockResolvedValue(undefined)
    render(
      <ManufacturerAliasReviewPanel
        pendingRows={[
          {
            kind: 'quote',
            alias: 'TI',
            lineIndexes: [1],
            platformIds: ['find_chips'],
            demandHint: 'Texas Instruments',
            recommendedCanonicalId: 'MFR_TI',
          },
        ]}
        canonicalRows={[{ canonical_id: 'MFR_TI', display_name: 'Texas Instruments' }]}
        onApprove={onApprove}
        onApplyExisting={vi.fn()}
      />
    )

    fireEvent.click(screen.getByRole('button', { name: '确认清洗' }))

    await waitFor(() => {
      expect(onApprove).toHaveBeenCalledWith({
        alias: 'TI',
        canonical_id: 'MFR_TI',
        display_name: 'Texas Instruments',
      })
    })
  })

  it('allows manual canonical_id and display_name when not in standard list', async () => {
    const onApprove = vi.fn().mockResolvedValue(undefined)
    render(
      <ManufacturerAliasReviewPanel
        pendingRows={[
          {
            kind: 'quote',
            alias: 'ESPRESSIF INC.',
            lineIndexes: [26],
            platformIds: ['ickey'],
            demandHint: 'Espressif Systems',
            recommendedCanonicalId: 'MFR_BAD_GUESS',
          },
        ]}
        canonicalRows={[{ canonical_id: 'MFR_TI', display_name: 'Texas Instruments' }]}
        onApprove={onApprove}
        onApplyExisting={vi.fn()}
      />
    )

    expect(screen.getByText(/推荐 ID 未在标准列表中/)).toBeInTheDocument()

    const cidInputs = screen.getAllByPlaceholderText('MFR_XXX')
    const dnInputs = screen.getAllByPlaceholderText(/Espressif Systems/)
    fireEvent.change(cidInputs[0], { target: { value: 'MFR_ESPRESSIF' } })
    fireEvent.change(dnInputs[0], { target: { value: 'Espressif Systems' } })

    fireEvent.click(screen.getByRole('button', { name: '确认清洗' }))

    await waitFor(() => {
      expect(onApprove).toHaveBeenCalledWith({
        alias: 'ESPRESSIF INC.',
        canonical_id: 'MFR_ESPRESSIF',
        display_name: 'Espressif Systems',
      })
    })
  })

  it('auto-selects best matching canonical when recommendation id is not in list', async () => {
    const onApprove = vi.fn().mockResolvedValue(undefined)
    render(
      <ManufacturerAliasReviewPanel
        pendingRows={[
          {
            kind: 'quote',
            alias: 'ESPRESSIF INC.',
            lineIndexes: [26],
            platformIds: ['ickey'],
            demandHint: 'Espressif Systems',
            recommendedCanonicalId: 'MFR_BAD_GUESS',
          },
        ]}
        canonicalRows={[
          { canonical_id: 'MFR_TI', display_name: 'Texas Instruments' },
          { canonical_id: 'MFR_ESPRESSIF', display_name: 'Espressif' },
        ]}
        onApprove={onApprove}
        onApplyExisting={vi.fn()}
      />,
    )

    expect(screen.queryByText(/推荐 ID 未在标准列表中/)).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: '确认清洗' }))

    await waitFor(() => {
      expect(onApprove).toHaveBeenCalledWith({
        alias: 'ESPRESSIF INC.',
        canonical_id: 'MFR_ESPRESSIF',
        display_name: 'Espressif',
      })
    })
  })
})
