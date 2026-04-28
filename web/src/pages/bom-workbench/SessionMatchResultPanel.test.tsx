import { render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SessionMatchResultPanel } from './SessionMatchResultPanel'

const { autoMatch } = vi.hoisted(() => ({
  autoMatch: vi.fn(),
}))

vi.mock('../../api', async () => {
  const actual = await vi.importActual<Record<string, unknown>>('../../api')
  return { ...actual, autoMatch }
})

describe('SessionMatchResultPanel', () => {
  it('shows customs control mark and import tax rates in match results', async () => {
    autoMatch.mockResolvedValue({
      items: [
        {
          index: 1,
          model: 'MP1658GTF-Z',
          quantity: 10,
          matched_model: 'MP1658GTF-Z',
          manufacturer: 'MPS',
          platform: 'find_chips',
          lead_time: '',
          stock: 195000,
          unit_price: 0.414,
          subtotal: 4.14,
          match_status: 'exact',
          all_quotes: [],
          demand_manufacturer: 'MPS',
          demand_package: '',
          control_mark: 'A',
          import_tax_imp_ordinary_rate: '20%',
          import_tax_imp_discount_rate: '5%',
        },
      ],
    })

    render(<SessionMatchResultPanel bomId="bom-1" />)

    await waitFor(() => expect(autoMatch).toHaveBeenCalledWith('bom-1'))
    expect(screen.getByRole('columnheader', { name: '商检' })).toBeInTheDocument()
    expect(screen.getByRole('columnheader', { name: '进口税率' })).toBeInTheDocument()
    expect(screen.getByRole('columnheader', { name: '最惠国税率' })).toBeInTheDocument()
    expect(screen.getByText('A')).toBeInTheDocument()
    expect(screen.getByText('20%')).toBeInTheDocument()
    expect(screen.getByText('5%')).toBeInTheDocument()
  })
})
