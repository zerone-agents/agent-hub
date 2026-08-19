import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { ConfigProvider } from 'antd'
import { antdTheme } from '@/lib/antd-theme'
import { tokens as t } from '@/styles/tokens'
import McpListPage from './McpListPage'
import type { Mcp } from '@/api/mcps'

const mockMcpPending: Mcp = {
  id: 1,
  name: 'filesystem',
  title: '文件系统',
  description: '本地文件系统访问',
  transportType: 'sse',
  url: 'https://mcp.example.com/sse',
  hasHeaders: false,
  isBuiltin: false,
  probeStatus: 'pending',
  lastProbedAt: null,
  createdAt: '2026-06-10T10:00:00Z',
  updatedAt: '2026-06-10T10:00:00Z',
}

const mockMcpSuccess: Mcp = {
  id: 2,
  name: 'search',
  title: '搜索引擎',
  description: 'Web 搜索工具',
  transportType: 'http',
  url: 'https://mcp.example.com/mcp',
  hasHeaders: true,
  isBuiltin: false,
  probeStatus: 'success',
  tools: [
    { name: 'web_search', description: '搜索网页' },
    { name: 'web_fetch', description: '获取网页内容' },
  ],
  lastProbedAt: new Date().toISOString(),
  createdAt: '2026-06-12T10:00:00Z',
  updatedAt: '2026-06-12T10:00:00Z',
}

const mockMcpFailed: Mcp = {
  id: 3,
  name: 'broken',
  title: '故障服务',
  description: '已下线的服务',
  transportType: 'sse',
  url: 'https://dead.example.com/sse',
  hasHeaders: false,
  isBuiltin: false,
  probeStatus: 'failed',
  lastProbedAt: '2026-06-01T10:00:00Z',
  createdAt: '2026-06-01T10:00:00Z',
  updatedAt: '2026-06-01T10:00:00Z',
}

const mockMcpBuiltin: Mcp = {
  id: 4,
  name: 'knowledge',
  title: '知识库检索',
  description: '系统内置',
  transportType: 'http',
  url: '',
  hasHeaders: false,
  isBuiltin: true,
  probeStatus: 'success',
  tools: [{ name: 'knowledge_search', description: '检索知识库' }],
  lastProbedAt: null,
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
}

// Mutable list, swap contents between tests
const mcpsList: Mcp[] = []
const probeMutateAsync = vi.fn().mockResolvedValue({ status: 'success' })
const deleteMutateAsync = vi.fn().mockResolvedValue({ data: { success: true } })

// 角色默认 admin：既有断言依赖新建/探测/删除按钮可见；member 分支用例内切换。
const authUser = vi.hoisted(() => ({
  user: { id: '1', name: 'admin', email: 'admin@zerone.run', role: 'admin' }
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: (selector: (s: {
    user: { id: string; name: string; email: string; role: string } | null
    setUser: () => void
    loginWithPassword: () => Promise<void>
    login: () => void
    logout: () => Promise<void>
  }) => unknown) => selector({
    user: authUser.user,
    setUser: vi.fn(),
    loginWithPassword: vi.fn(),
    login: vi.fn(),
    logout: vi.fn()
  })
}))

vi.mock('@/queries/useMcps', () => ({
  useMcps: () => ({ data: mcpsList, isLoading: false }),
  useDeleteMcp: () => ({ mutateAsync: deleteMutateAsync }),
  useProbeMcp: () => ({ mutateAsync: probeMutateAsync, isPending: false }),
  useCreateMcp: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUpdateMcp: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useMcp: () => ({ data: null, isLoading: false }),
}))

const renderPage = () =>
  render(
    <ConfigProvider theme={antdTheme}>
      <MemoryRouter>
        <McpListPage />
      </MemoryRouter>
    </ConfigProvider>
  )

const seedMcps = (...items: Mcp[]) => {
  mcpsList.length = 0
  mcpsList.push(...items)
}

describe('McpListPage', () => {
  beforeEach(() => {
    seedMcps(mockMcpPending, mockMcpSuccess, mockMcpFailed, mockMcpBuiltin)
    probeMutateAsync.mockClear()
    deleteMutateAsync.mockClear()
    authUser.user = { ...authUser.user, role: 'admin' }
  })

  it('renders page title and create button', () => {
    renderPage()
    expect(screen.getByText('MCP 配置')).toBeInTheDocument()
    expect(screen.getByText(/管理外部 MCP 服务器配置/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /新建 MCP/ })).toBeInTheDocument()
  })

  it('shows empty state when no mcps', () => {
    seedMcps()
    renderPage()
    expect(screen.getByText('暂无 MCP 配置')).toBeInTheDocument()
  })

  it('renders "未探测" when probeStatus is pending', () => {
    renderPage()
    const elements = screen.getAllByText('未探测')
    expect(elements.length).toBe(1)
  })

  it('renders "探测失败" when probeStatus is failed', () => {
    renderPage()
    expect(screen.getByText('探测失败')).toBeInTheDocument()
  })

  it('renders tool count and tool tags when probeStatus is success', () => {
    renderPage()
    expect(screen.getByText(/2 个 tools/)).toBeInTheDocument()
    expect(screen.getByText('web_search')).toBeInTheDocument()
    expect(screen.getByText('web_fetch')).toBeInTheDocument()
  })

  it('uses theme-aware colors for tool tags', () => {
    renderPage()

    const toolTag = screen.getByText('web_search')
    // ToolTag was simplified to a flat monospace chip on a subtle background;
    // see McpListPage.tsx:ToolTag. These assertions lock the current style
    // contract so accidental visual regressions surface here.
    expect(toolTag.style.background).toBe('var(--primary-subtle)')
    expect(toolTag.style.color).toBe('var(--text-secondary)')
    expect(toolTag.style.fontFamily).toBe(t.fontMono)
  })

  it('renders transport type tags', () => {
    renderPage()
    expect(screen.getAllByText('SSE').length).toBe(2)
    expect(screen.getAllByText('HTTP').length).toBe(2)
  })

  it('renders builtin badge for builtin mcps', () => {
    renderPage()
    expect(screen.getByText('内置')).toBeInTheDocument()
    expect(screen.getByText('1 个内置 tools')).toBeInTheDocument()
    expect(screen.getByText('knowledge_search')).toBeInTheDocument()
  })

  it('probe button triggers useProbeMcp mutation', async () => {
    const user = userEvent.setup()
    renderPage()

    const probeButtons = screen.getAllByTitle('探测')
    expect(probeButtons.length).toBeGreaterThan(0)

    await user.click(probeButtons[0])

    // McpListPage sorts by name ascending, so the first probe button
    // belongs to whichever MCP sorts earliest. In the default seed set
    // that's "broken" (mockMcpFailed), not the seed-order-first "filesystem".
    await waitFor(() => {
      expect(probeMutateAsync).toHaveBeenCalledWith({ name: mockMcpFailed.name })
    })
  })

  it('builtin mcp does not show delete button', () => {
    renderPage()
    const deleteButtons = screen.getAllByTitle('删除')
    const nonBuiltinCount = mcpsList.filter(m => !m.isBuiltin).length
    expect(deleteButtons.length).toBe(nonBuiltinCount)
  })

  it('builtin mcp does not show probe button', () => {
    renderPage()
    const probeButtons = screen.getAllByTitle('探测')
    const nonBuiltinCount = mcpsList.filter(m => !m.isBuiltin).length
    expect(probeButtons.length).toBe(nonBuiltinCount)
  })

  it('member: hides create/probe/edit/delete but still sees mcps', () => {
    authUser.user = { ...authUser.user, role: 'member' }
    renderPage()

    // 数据仍可见（只读）
    expect(screen.getByText('filesystem')).toBeInTheDocument()
    expect(screen.getByText('内置')).toBeInTheDocument()
    // 写操作按钮隐藏
    expect(screen.queryByRole('button', { name: /新建 MCP/ })).not.toBeInTheDocument()
    expect(screen.queryAllByTitle('探测')).toHaveLength(0)
    expect(screen.queryAllByTitle('编辑')).toHaveLength(0)
    expect(screen.queryAllByTitle('删除')).toHaveLength(0)
  })
})
