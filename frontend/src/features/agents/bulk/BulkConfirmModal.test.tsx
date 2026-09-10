import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ConfigProvider } from 'antd'
import { antdTheme } from '@/lib/antd-theme'
import BulkConfirmModal from './BulkConfirmModal'
import type { ClassifiedItem } from './classifyBulkOperation'
import type { Agent } from '@/api/agents'

const mk = (name: string, classification: ClassifiedItem['classification'], reason?: string): ClassifiedItem => ({
  agent: { id: 1, name, config: { title: { zh: name } } } as Agent,
  precheck: { kind: 'success', status: { status: 'not_found' } },
  classification,
  reason,
})

const items: ClassifiedItem[] = [
  mk('a', 'executable'),
  mk('b', 'executable'),
  mk('c', 'skipped', '已部署，建议重新部署'),
  mk('d', 'blocked', '有活跃部署，需先删除部署'),
]

function renderModal(props: Partial<Parameters<typeof BulkConfirmModal>[0]> = {}) {
  const base = {
    open: true,
    operation: 'deploy' as const,
    items,
    onCancel: vi.fn(),
    onConfirm: vi.fn(),
    ...props,
  }
  render(<ConfigProvider theme={antdTheme}><BulkConfirmModal {...base} /></ConfigProvider>)
  return base
}

describe('BulkConfirmModal', () => {
  it('renders three groups with counts and names', () => {
    renderModal()
    expect(screen.getByText('可执行 · 2')).toBeInTheDocument()
    expect(screen.getByText('跳过 · 1')).toBeInTheDocument()
    expect(screen.getByText('受限 · 1')).toBeInTheDocument()
    expect(screen.getByText('a')).toBeInTheDocument()
    // reason 渲染带中文括号（{reason}），完全匹配唯一命中 span
    expect(screen.getByText('（有活跃部署，需先删除部署）')).toBeInTheDocument()
  })

  it('confirm button carries executable count; fires onConfirm', async () => {
    const user = userEvent.setup()
    const p = renderModal()
    const ok = screen.getByRole('button', { name: '部署 2 个' })
    expect(ok).toBeEnabled()
    await user.click(ok)
    expect(p.onConfirm).toHaveBeenCalledTimes(1)
  })

  it('zero executable → confirm disabled', () => {
    renderModal({ items: [mk('c', 'skipped', '已部署，建议重新部署')] })
    expect(screen.getByRole('button', { name: '部署 0 个' })).toBeDisabled()
  })

  it('delete operation renders danger confirm button', () => {
    renderModal({ operation: 'delete' })
    expect(screen.getByRole('button', { name: '删除 2 个' })).toBeInTheDocument()
  })

  it('cancel fires onCancel', async () => {
    const user = userEvent.setup()
    const p = renderModal()
    // antd 对两个汉字按钮自动插空格（「取 消」）
    await user.click(screen.getByRole('button', { name: /^取\s*消$/ }))
    expect(p.onCancel).toHaveBeenCalledTimes(1)
  })
})
