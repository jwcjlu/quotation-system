import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { HsMetaAdminPage } from './HsMetaAdminPage'

describe('HsMetaAdminPage', () => {
  it('renders admin tools instead of the old visibility placeholder', () => {
    render(<HsMetaAdminPage />)

    expect(screen.getByRole('heading', { name: 'HS元数据' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '分类元数据' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'HS 条目查询' })).toBeInTheDocument()
    expect(screen.queryByText(/admin 可见范围/)).not.toBeInTheDocument()
    expect(screen.queryByText(/普通用户和匿名用户/)).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: '同步任务' }))
    expect(screen.getByText('同步全部启用分类')).toBeInTheDocument()
  })
})
