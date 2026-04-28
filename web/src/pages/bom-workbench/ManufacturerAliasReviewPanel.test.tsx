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
})
