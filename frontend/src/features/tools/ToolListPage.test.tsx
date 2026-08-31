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
  { id: 1, name: 'Bash', title: '执行命令', description: 'd', isDefault: false, source: 'builtin', createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z' },
  { id: 2, name: 'SayHello', title: '问候', description: 'd', isDefault: false, source: 'custom', artifactStatus: 'ready', fileName: 'say.ts', fileHash: 'abcd1234abcd1234', fileSize: 1024, createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z' },
  { id: 3, name: 'Legacy', title: '存量', description: 'd', isDefault: false, source: 'custom', artifactStatus: 'missing', createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z' }
]

// ToolForm 挂载即调用这些 hooks（Modal 关闭时组件仍在），mock 工厂需提供全部。
vi.mock('@/queries/useTools', () => ({
  useTools: () => ({ data: mockTools, isLoading: false }),
  useDeleteTool: () => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false }),
  useCreateCustomTool: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUploadToolFile: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUpdateTool: () => ({ mutateAsync: vi.fn(), isPending: false })
}))

const renderToolListPage = () =>
  render(
    <ConfigProvider theme={antdTheme}>
      <MemoryRouter>
        <ToolListPage />
      </MemoryRouter>
    </ConfigProvider>
  )

describe('ToolListPage', () => {
  beforeEach(() => {
    setAuthRole('admin')
  })

  it('renders builtin and custom sections with counts', async () => {
    renderToolListPage()
    expect(await screen.findByText('内置工具')).toBeInTheDocument()
    expect(screen.getByText('自定义工具')).toBeInTheDocument()
    expect(screen.getByText('SayHello')).toBeInTheDocument()
  })

  it('opens upload form on header button click', async () => {
    const user = userEvent.setup()
    renderToolListPage()

    const uploadBtn = screen.getByRole('button', { name: /上传自定义工具/ })
    await user.click(uploadBtn)

    // create 模式弹窗出现，含工具标识字段
    expect(await screen.findByText('工具标识')).toBeInTheDocument()
  })

  it('builtin cards are read-only (no edit/delete buttons)', async () => {
    renderToolListPage()
    await screen.findByText('执行命令')
    // builtin 卡片区域内无编辑/删除按钮（用 title 查询）
    expect(screen.queryByTitle('编辑工具')).not.toBeInTheDocument()
    expect(screen.queryAllByTitle('编辑')).toHaveLength(1) // 仅自定义 ready 卡片有
    expect(screen.queryAllByTitle('删除')).toHaveLength(2) // 仅自定义卡片有
  })

  it('missing custom card shows 补传 CTA and warning badge', async () => {
    renderToolListPage()
    expect(await screen.findByText('缺少文件')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /补传文件/ })).toBeInTheDocument()
  })

  it('member: hides write actions but still sees tools', () => {
    setAuthRole('member')
    renderToolListPage()

    // 数据仍可见（只读）
    expect(screen.getByText('工具管理')).toBeInTheDocument()
    expect(screen.getByText('执行命令')).toBeInTheDocument()
    expect(screen.getByText('SayHello')).toBeInTheDocument()
    // 写操作按钮隐藏
    expect(screen.queryByRole('button', { name: /上传自定义工具/ })).not.toBeInTheDocument()
    expect(screen.queryAllByTitle('编辑')).toHaveLength(0)
    expect(screen.queryAllByTitle('删除')).toHaveLength(0)
    expect(screen.queryAllByTitle('补传文件')).toHaveLength(0)
    expect(screen.queryAllByTitle('替换文件')).toHaveLength(0)
  })
})
