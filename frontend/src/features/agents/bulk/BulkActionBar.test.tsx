import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ConfigProvider } from 'antd'
import { antdTheme } from '@/lib/antd-theme'
import BulkActionBar from './BulkActionBar'

function renderBar(props: Partial<Parameters<typeof BulkActionBar>[0]> = {}) {
  const base = {
    selectedCount: 2,
    pendingUpdateCount: 1,
    onSelectAll: vi.fn(),
    onSelectPendingUpdates: vi.fn(),
    onClear: vi.fn(),
    onOperation: vi.fn(),
    onExit: vi.fn(),
    operationsDisabled: false,
    prechecking: null,
    ...props,
  }
  render(<ConfigProvider theme={antdTheme}><BulkActionBar {...base} /></ConfigProvider>)
  return base
}

describe('BulkActionBar', () => {
  it('shows selected count', () => {
    renderBar({ selectedCount: 5 })
    expect(screen.getByText('已选 5 个')).toBeInTheDocument()
  })

  it('invokes callbacks: 全选 / 全选待更新 / 清空 / 退出', async () => {
    const user = userEvent.setup()
    const p = renderBar()
    // antd 对两个汉字的纯文本按钮自动插入空格（「全 选」），用 regex 容忍
    await user.click(screen.getByRole('button', { name: /^全\s*选$/ }))
    await user.click(screen.getByRole('button', { name: /^全\s*选待更新$/ }))
    await user.click(screen.getByRole('button', { name: /^清\s*空$/ }))
    await user.click(screen.getByRole('button', { name: /^退\s*出$/ }))
    expect(p.onSelectAll).toHaveBeenCalledTimes(1)
    expect(p.onSelectPendingUpdates).toHaveBeenCalledTimes(1)
    expect(p.onClear).toHaveBeenCalledTimes(1)
    expect(p.onExit).toHaveBeenCalledTimes(1)
  })

  it('disable 全选待更新 when no pending updates', () => {
    renderBar({ pendingUpdateCount: 0 })
    expect(screen.getByRole('button', { name: /^全\s*选待更新$/ })).toBeDisabled()
  })

  it('operations disabled when operationsDisabled or nothing selected', async () => {
    const user = userEvent.setup()
    const p = renderBar({ operationsDisabled: true })
    for (const label of ['部署', '重新部署', '停止', '删除']) {
      expect(screen.getByRole('button', { name: label })).toBeDisabled()
    }
    await user.click(screen.getByRole('button', { name: '部署' }))
    expect(p.onOperation).not.toHaveBeenCalled()
  })

  it('fires onOperation with the operation key', async () => {
    const user = userEvent.setup()
    const p = renderBar()
    await user.click(screen.getByRole('button', { name: '停止' }))
    expect(p.onOperation).toHaveBeenCalledWith('stop')
  })
})
