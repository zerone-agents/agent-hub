import { describe, it, expect, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ConfigProvider } from 'antd'
import { antdTheme } from '@/lib/antd-theme'
import ToolForm from './ToolForm'
import type { Tool } from '@/api/tools'

const mockDefaultTool: Tool = {
  id: 1,
  name: 'Bash',
  title: '执行命令',
  description: '执行 bash 命令',
  isDefault: true,
  source: 'builtin',
  createdAt: '2026-06-10T10:00:00Z',
  updatedAt: ''
}

const mockNormalTool: Tool = {
  id: 2,
  name: 'Read',
  title: '读取文件',
  description: '读取文件内容',
  isDefault: false,
  source: 'builtin',
  createdAt: '2026-06-12T10:00:00Z',
  updatedAt: ''
}

vi.mock('@/queries/useTools', () => ({
  useCreateTool: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUpdateTool: () => ({ mutateAsync: vi.fn(), isPending: false })
}))

describe('ToolForm', () => {
  it('renders default tool switch as ON when editing a default tool', async () => {
    render(
      <ConfigProvider theme={antdTheme}>
        <ToolForm open editingTool={mockDefaultTool} onClose={vi.fn()} />
      </ConfigProvider>
    )

    await waitFor(() => {
      const switchEl = screen.getByRole('switch')
      expect(switchEl).toHaveAttribute('aria-checked', 'true')
    })
  })

  it('renders default tool switch as OFF when editing a non-default tool', async () => {
    render(
      <ConfigProvider theme={antdTheme}>
        <ToolForm open editingTool={mockNormalTool} onClose={vi.fn()} />
      </ConfigProvider>
    )

    await waitFor(() => {
      const switchEl = screen.getByRole('switch')
      expect(switchEl).toHaveAttribute('aria-checked', 'false')
    })
  })

  it('toggles the default tool switch when clicked', async () => {
    const user = userEvent.setup()
    render(
      <ConfigProvider theme={antdTheme}>
        <ToolForm open editingTool={mockNormalTool} onClose={vi.fn()} />
      </ConfigProvider>
    )

    const switchEl = await screen.findByRole('switch')
    expect(switchEl).toHaveAttribute('aria-checked', 'false')

    // Clicking the switch itself should toggle it
    await user.click(switchEl)
    await waitFor(() => {
      expect(switchEl).toHaveAttribute('aria-checked', 'true')
    })
  })
})
