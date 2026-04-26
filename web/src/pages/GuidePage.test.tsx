import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { GuidePage } from './GuidePage'

describe('GuidePage', () => {
  it('shows the complete captured workflow guide', () => {
    render(<GuidePage />)

    expect(screen.getByRole('heading', { name: '使用指南' })).toBeInTheDocument()
    expect(screen.getByText('1. 登录系统')).toBeInTheDocument()
    expect(screen.getByText('2. 新建 BOM 单')).toBeInTheDocument()
    expect(screen.getByText('3. 查看 BOM 搜索状态')).toBeInTheDocument()
    expect(screen.getByText('4. 进入 BOM 配单')).toBeInTheDocument()
    expect(screen.getByText('5. 查看配单详情')).toBeInTheDocument()
  })

  it('opens and closes enlarged image previews', () => {
    render(<GuidePage />)

    fireEvent.click(screen.getByRole('button', { name: /登录后的系统首页/ }))

    expect(screen.getByRole('dialog', { name: '1. 登录系统 大图预览' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '关闭' }))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })
})
