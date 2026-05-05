import { describe, expect, it } from 'vitest'

import { pickBestCanonicalMatch, scoreCanonicalMatchRow } from './manufacturerAliasCanonicalPick'

const rows = [
  { canonical_id: 'MFR_TI', display_name: 'Texas Instruments' },
  { canonical_id: 'MFR_ESPRESSIF', display_name: 'Espressif' },
  { canonical_id: 'MFR_ON_SEMICONDUCTOR', display_name: 'ON Semiconductor' },
] as const

describe('pickBestCanonicalMatch', () => {
  it('uses recommended when it exists in list', () => {
    expect(pickBestCanonicalMatch('TI', 'Texas', 'MFR_TI', [...rows])).toBe('MFR_TI')
  })

  it('picks Espressif when recommendation missing from list but alias matches', () => {
    expect(
      pickBestCanonicalMatch('ESPRESSIF INC.', 'Espressif Systems', 'MFR_BAD', [...rows]),
    ).toBe('MFR_ESPRESSIF')
  })

  it('returns empty when nothing matches strongly enough', () => {
    expect(pickBestCanonicalMatch('FooBar', 'Unknown', 'MFR_BAD', [...rows])).toBe('')
  })
})

describe('scoreCanonicalMatchRow', () => {
  it('scores Espressif row above ON for Espressif-shaped alias', () => {
    const sEsp = scoreCanonicalMatchRow('ESPRESSIF(乐鑫)', 'Espressif Systems', {
      canonical_id: 'MFR_ESPRESSIF',
      display_name: 'Espressif',
    })
    const sOn = scoreCanonicalMatchRow('ESPRESSIF(乐鑫)', 'Espressif Systems', {
      canonical_id: 'MFR_ON_SEMICONDUCTOR',
      display_name: 'ON Semiconductor',
    })
    expect(sEsp).toBeGreaterThan(sOn)
  })
})
