import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { ConfigProvider } from 'antd'
import { antdTheme } from '@/lib/antd-theme'
import ToolListPage from './ToolListPage'
import type { Tool } from '@/api/tools'
import { setAuthRole } from '@/test/auth-store-mock'

// vi.mock 工厂会被提升到 import 之前执行，不能引用静态 import；用 async 工厂动态 import helper。
vi.mock('@/stores/auth', async () => (await import('@/test/auth-store-mock')).createAuthStoreMock())

const mockTools: Tool[] = [
  { id: 1, name: 'Read', title: '读取文件', description: '读取文件内容', isDefault: true, createdAt: '2026-06-10T10:00:00Z', updatedAt: '' },
  { id: 2, name: 'Write', title: '写入文件', description: '写入文件内容', isDefault: false, createdAt: '2026-06-12T10:00:00Z', updatedAt: '' }
]

vi.mock('@/queries/useTools', () => ({
  useTools: () => ({ data: mockTools, isLoading: false }),
  useDeleteTool: () => ({ mutateAsync: vi.fn() }),
  useCreateTool: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUpdateTool: () => ({ mutateAsync: vi.fn(), isPending: false })
}))

describe('ToolListPage', () => {
  beforeEach(() => {
    setAuthRole('admin')
  })

  it('renders tool cards and create button', () => {
    render(
      <ConfigProvider theme={antdTheme}>
        <MemoryRouter>
          <ToolListPage />
        </MemoryRouter>
      </ConfigProvider>
    )

    expect(screen.getByText('工具管理')).toBeInTheDocument()
    expect(screen.getByText('新建工具')).toBeInTheDocument()
    expect(screen.getByText('读取文件')).toBeInTheDocument()
    expect(screen.getByText('写入文件')).toBeInTheDocument()
  })

  it('opens create form on button click', async () => {
    const user = userEvent.setup()
    render(
      <ConfigProvider theme={antdTheme}>
        <MemoryRouter>
          <ToolListPage />
        </MemoryRouter>
      </ConfigProvider>
    )

    // Click the "新建工具" button in the page header
    const createBtn = screen.getByRole('button', { name: /新建工具/ })
    await user.click(createBtn)

    // The form modal should appear with the tool name field
    expect(await screen.findByText('工具标识')).toBeInTheDocument()
  })

  it('member: hides create/edit/delete but still sees tools', () => {
    setAuthRole('member')
    render(
      <ConfigProvider theme={antdTheme}>
        <MemoryRouter>
          <ToolListPage />
        </MemoryRouter>
      </ConfigProvider>
    )

    // 数据仍可见（只读）
    expect(screen.getByText('工具管理')).toBeInTheDocument()
    expect(screen.getByText('读取文件')).toBeInTheDocument()
    expect(screen.getByText('写入文件')).toBeInTheDocument()
    // 写操作按钮隐藏
    expect(screen.queryByRole('button', { name: /新建工具/ })).not.toBeInTheDocument()
    expect(screen.queryAllByTitle('编辑')).toHaveLength(0)
    expect(screen.queryAllByTitle('删除')).toHaveLength(0)
  })
})
