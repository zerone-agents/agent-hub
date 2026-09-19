import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ConfigProvider } from 'antd'
import { antdTheme } from '@/lib/antd-theme'
import BulkTaskModal from './BulkTaskModal'
import type { BulkTaskItem, BulkTaskSummary } from './useBulkAgentTask'

const summary: BulkTaskSummary = { total: 3, succeeded: 1, failed: 1, skipped: 1, blocked: 0 }

const items: BulkTaskItem[] = [
  { name: 'a', title: 'A', status: 'succeeded' },
  { name: 'b', title: 'B', status: 'failed', reason: '后端爆炸' },
  { name: 'c', title: 'C', status: 'skipped', reason: '已部署，建议重新部署' },
]

function renderModal(props: Partial<Parameters<typeof BulkTaskModal>[0]> = {}) {
  const base = {
    open: true,
    phase: 'done' as const,
    operation: 'deploy' as const,
    items,
    summary,
    onCollapse: vi.fn(),
    onClose: vi.fn(),
    ...props,
  }
  render(<ConfigProvider theme={antdTheme}><BulkTaskModal {...base} /></ConfigProvider>)
  return base
}

describe('BulkTaskModal', () => {
  it('renders per-item statuses, reasons and summary', () => {
    renderModal()
    expect(screen.getByText('成功 1 · 失败 1 · 跳过 1 · 受限 0')).toBeInTheDocument()
    expect(screen.getByText('后端爆炸')).toBeInTheDocument()
    expect(screen.getByText('已部署，建议重新部署')).toBeInTheDocument()
    expect(screen.getByText('3/3')).toBeInTheDocument()
  })

  it('running: footer shows 收起, X collapses (task continues)', async () => {
    const user = userEvent.setup()
    const p = renderModal({ phase: 'running', summary: { total: 3, succeeded: 0, failed: 0, skipped: 1, blocked: 0 } })
    // antd 对两个汉字按钮自动插空格（「收 起」）
    await user.click(screen.getByRole('button', { name: /^收\s*起$/ }))
    expect(p.onCollapse).toHaveBeenCalledTimes(1)
    expect(screen.queryByRole('button', { name: /^关\s*闭$/ })).not.toBeInTheDocument()
  })

  it('done: footer shows 关闭 and fires onClose', async () => {
    const user = userEvent.setup()
    const p = renderModal()
    await user.click(screen.getByRole('button', { name: /^关\s*闭$/ }))
    expect(p.onClose).toHaveBeenCalledTimes(1)
    expect(screen.queryByRole('button', { name: /^收\s*起$/ })).not.toBeInTheDocument()
  })

  it('shows pending/running badges for in-flight batch', () => {
    renderModal({
      phase: 'running',
      items: [
        { name: 'x', title: 'X', status: 'pending' },
        { name: 'y', title: 'Y', status: 'running' },
      ],
      summary: { total: 2, succeeded: 0, failed: 0, skipped: 0, blocked: 0 },
    })
    expect(screen.getByText('等待')).toBeInTheDocument()
    expect(screen.getByText('执行中')).toBeInTheDocument()
  })
})
