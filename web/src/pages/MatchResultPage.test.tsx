import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { MatchResultPage } from './MatchResultPage'

const {
  getSession,
  getMatchResult,
  hsBatchResolveByModels,
} = vi.hoisted(() => ({
  getSession: vi.fn(),
  getMatchResult: vi.fn(),
  hsBatchResolveByModels: vi.fn(),
}))

vi.mock('../api', async () => {
  const actual = await vi.importActual<Record<string, unknown>>('../api')
  return {
    ...actual,
    getSession,
    getMatchResult,
    hsBatchResolveByModels,
  }
})

describe('MatchResultPage', () => {
  it('triggers HS batch resolve for matched lines without hs', async () => {
    getSession.mockResolvedValue({
      session_id: 'session-1',
      status: 'data_ready',
      biz_date: '2026-05-05',
      selection_revision: 1,
      platform_ids: ['hqchip'],
    })
    getMatchResult.mockResolvedValue({
      total_amount: 100,
      items: [
        {
          index: 1,
          model: 'STM32F103C8T6',
          quantity: 10,
          matched_model: 'STM32F103C8T6',
          manufacturer: 'ST',
          demand_manufacturer: 'ST',
          demand_package: 'LQFP-48',
          platform: 'hqchip',
          lead_time: '',
          stock: 100,
          unit_price: 1,
          subtotal: 10,
          match_status: 'exact',
          all_quotes: [],
          hs_code_status: 'hs_not_mapped',
        },
        {
          index: 2,
          model: 'TPS5430',
          quantity: 5,
          matched_model: 'TPS5430',
          manufacturer: 'TI',
          demand_manufacturer: 'TI',
          demand_package: 'SOIC-8',
          platform: 'hqchip',
          lead_time: '',
          stock: 200,
          unit_price: 2,
          subtotal: 10,
          match_status: 'exact',
          all_quotes: [],
          hs_code_status: 'hs_found',
          code_ts: '8542399000',
        },
      ],
    })
    hsBatchResolveByModels.mockResolvedValue({
      accepted_count: 1,
      skipped_count: 0,
      failed_count: 0,
      results: [],
    })

    render(<MatchResultPage bomId="session-1" />)

    await screen.findByText('配单结果')
    fireEvent.click(screen.getByRole('button', { name: '批量解析HS（已匹配未填HS）' }))

    await waitFor(() => {
      expect(hsBatchResolveByModels).toHaveBeenCalledWith(
        expect.objectContaining({
          session_id: 'session-1',
          lines: [
            expect.objectContaining({
              line_no: 1,
              model: 'STM32F103C8T6',
              match_status: 'exact',
              hs_code_status: 'hs_not_mapped',
            }),
          ],
        }),
      )
    })
    expect(await screen.findByText('已提交：成功 1，跳过 0，失败 0')).toBeInTheDocument()
  })
})
