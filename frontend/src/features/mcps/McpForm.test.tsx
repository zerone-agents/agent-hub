import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ConfigProvider } from 'antd'
import { antdTheme } from '@/lib/antd-theme'
import McpForm from './McpForm'
import type { Mcp, McpProbeResult } from '@/api/mcps'

const createMutateAsync = vi.fn().mockResolvedValue({ data: { success: true } })
const updateMutateAsync = vi.fn().mockResolvedValue({ data: { success: true } })
const probeMutateAsync = vi.fn().mockResolvedValue({ status: 'success' })
const useMcpMock = vi.fn()

vi.mock('@/queries/useMcps', () => ({
  useCreateMcp: () => ({ mutateAsync: createMutateAsync, isPending: false }),
  useUpdateMcp: () => ({ mutateAsync: updateMutateAsync, isPending: false }),
  useMcp: (name: string | null) => useMcpMock(name),
  useProbeMcp: () => ({ mutateAsync: probeMutateAsync, isPending: false }),
}))

const renderForm = (props: { open?: boolean; editingMcp?: Mcp | null; onClose?: () => void } = {}) =>
  render(
    <ConfigProvider theme={antdTheme}>
      <McpForm
        open={props.open ?? true}
        editingMcp={props.editingMcp ?? null}
        onClose={props.onClose ?? vi.fn()}
      />
    </ConfigProvider>
  )

describe('McpForm', () => {
  beforeEach(() => {
    createMutateAsync.mockClear()
    updateMutateAsync.mockClear()
    probeMutateAsync.mockClear()
    useMcpMock.mockReturnValue({ data: null, isLoading: false })
  })

  it('renders create mode modal title when not editing', async () => {
    renderForm()
    expect(await screen.findByText('新建 MCP')).toBeInTheDocument()
  })

  it('renders edit mode modal title when editing', async () => {
    const editing: Mcp = {
      id: 1,
      name: 'filesystem',
      title: '文件系统',
      description: 'desc',
      transportType: 'sse',
      url: 'https://example.com/sse',
      hasHeaders: false,
      isBuiltin: false,
      probeStatus: 'pending',
      createdAt: '2026-01-01T00:00:00Z',
      updatedAt: '2026-01-01T00:00:00Z',
    }
    renderForm({ editingMcp: editing })
    expect(await screen.findByText('编辑 MCP')).toBeInTheDocument()
  })

  it('shows probe button in the form', async () => {
    renderForm()
    expect(await screen.findByRole('button', { name: /探测连接/ })).toBeInTheDocument()
  })

  it('shows tools placeholder text before probing', async () => {
    renderForm()
    expect(await screen.findByText(/点击"探测连接"获取 tools 列表/)).toBeInTheDocument()
  })

  it('probe button triggers useProbeMcp mutation with config after filling form', async () => {
    const user = userEvent.setup()
    renderForm()

    // Fill required fields: name, title, url
    const nameInput = await screen.findByPlaceholderText(/e\.g\. filesystem/)
    await user.type(nameInput, 'my-mcp')

    const titleInput = await screen.findByPlaceholderText(/e\.g\. 文件系统/)
    await user.type(titleInput, '我的 MCP')

    const urlInput = await screen.findByPlaceholderText(/https:\/\/mcp\.example\.com/)
    await user.type(urlInput, 'https://mcp.test.com/sse')

    // Click probe button
    const probeBtn = await screen.findByRole('button', { name: /探测连接/ })
    await user.click(probeBtn)

    await waitFor(() => {
      expect(probeMutateAsync).toHaveBeenCalledWith({
        config: {
          transportType: 'sse',
          url: 'https://mcp.test.com/sse',
          headers: {},
        },
      })
    })
  })

  it('shows success feedback when probe succeeds', async () => {
    const probeResult: McpProbeResult = {
      status: 'success',
      tools: [{ name: 'test_tool', description: 'A test tool' }],
    }
    probeMutateAsync.mockResolvedValueOnce(probeResult)

    const user = userEvent.setup()
    renderForm()

    await user.type(await screen.findByPlaceholderText(/e\.g\. filesystem/), 'test')
    await user.type(await screen.findByPlaceholderText(/e\.g\. 文件系统/), 'Test')
    await user.type(await screen.findByPlaceholderText(/https:\/\/mcp\.example\.com/), 'https://test.com/sse')

    await user.click(await screen.findByRole('button', { name: /探测连接/ }))

    expect(await screen.findByText(/连接成功/)).toBeInTheDocument()
    expect(await screen.findByText('test_tool')).toBeInTheDocument()
  })

  it('does not render modal content when open is false', () => {
    renderForm({ open: false })
    expect(screen.queryByText('新建 MCP')).not.toBeInTheDocument()
  })

  it('shows stored tools when editing MCP with existing tools', async () => {
    const detailWithTools = {
      id: 1,
      name: 'web-search-prime',
      title: 'web-search-prime',
      description: '搜索网络信息',
      transportType: 'http' as const,
      url: 'https://open.bigmodel.cn/api/mcp/web_search_prime/mcp',
      hasHeaders: true,
      isBuiltin: false,
      probeStatus: 'success' as const,
      tools: [{ name: 'web_search_prime', description: 'Search web information' }],
      headers: { Authorization: 'Bearer ***' },
      createdAt: '2026-01-01T00:00:00Z',
      updatedAt: '2026-01-01T00:00:00Z',
    }
    useMcpMock.mockReturnValue({ data: detailWithTools, isLoading: false })

    const editing: Mcp = {
      id: 1,
      name: 'web-search-prime',
      title: 'web-search-prime',
      description: '搜索网络信息',
      transportType: 'http',
      url: 'https://open.bigmodel.cn/api/mcp/web_search_prime/mcp',
      hasHeaders: true,
      isBuiltin: false,
      probeStatus: 'success',
      tools: [{ name: 'web_search_prime', description: 'Search web information' }],
      createdAt: '2026-01-01T00:00:00Z',
      updatedAt: '2026-01-01T00:00:00Z',
    }
    renderForm({ editingMcp: editing })

    expect(await screen.findByText('编辑 MCP')).toBeInTheDocument()
    expect(await screen.findByText('web_search_prime')).toBeInTheDocument()
    expect(screen.queryByText(/点击"探测连接"获取 tools 列表/)).not.toBeInTheDocument()
  })

  it('shows placeholder when editing MCP with no stored tools', async () => {
    const detailNoTools = {
      id: 2,
      name: 'empty-mcp',
      title: 'Empty MCP',
      description: 'No tools yet',
      transportType: 'sse' as const,
      url: 'https://example.com/sse',
      hasHeaders: false,
      isBuiltin: false,
      probeStatus: 'pending' as const,
      tools: [],
      headers: {},
      createdAt: '2026-01-01T00:00:00Z',
      updatedAt: '2026-01-01T00:00:00Z',
    }
    useMcpMock.mockReturnValue({ data: detailNoTools, isLoading: false })

    const editing: Mcp = {
      id: 2,
      name: 'empty-mcp',
      title: 'Empty MCP',
      description: 'No tools yet',
      transportType: 'sse',
      url: 'https://example.com/sse',
      hasHeaders: false,
      isBuiltin: false,
      probeStatus: 'pending',
      createdAt: '2026-01-01T00:00:00Z',
      updatedAt: '2026-01-01T00:00:00Z',
    }
    renderForm({ editingMcp: editing })

    expect(await screen.findByText(/点击"探测连接"获取 tools 列表/)).toBeInTheDocument()
  })
})
