import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor, act } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { ConfigProvider } from 'antd'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { antdTheme } from '@/lib/antd-theme'
import AgentListPage from './AgentListPage'
import type { Agent } from '@/api/agents'
import { setAuthRole } from '@/test/auth-store-mock'

// vi.mock 工厂会被提升到 import 之前执行，不能引用静态 import；用 async 工厂动态 import helper。
vi.mock('@/stores/auth', async () => (await import('@/test/auth-store-mock')).createAuthStoreMock())

const mockAgents: Agent[] = [
  {
    id: 1, name: 'general',
    config: { title: { zh: '通用助手' }, description: { zh: '通用对话代理' }, iconName: 'Robot', iconColor: '#06B6D4', iconBgColor: '#E6F8FC', maxTurns: 50, permissionMode: 'auto', providerId: 1, modelId: 'GLM-5-Turbo' },
    subagents: [], tools: ['search'], skills: [],
    desktopEnabled: true, isDefault: true, createdAt: '2026-06-10T10:00:00Z'
  },
  {
    id: 2, name: 'coder',
    config: { title: { zh: '编程助手' }, description: { zh: '代码生成与审查' }, iconName: 'Code', iconColor: '#22C55E', iconBgColor: '#E8FCE8', maxTurns: 30, permissionMode: 'auto' },
    subagents: ['general'], tools: [], skills: ['py'],
    desktopEnabled: false, isDefault: false, createdAt: '2026-06-15T10:00:00Z',
    pendingArtifactUpdates: { tools: ['py-tool'], skills: [] }
  }
]

// 测试可替换的列表数据（新数组引用模拟真实 react-query 刷新——原地 splice 骗过 useMemo 依赖比较）
let mutableAgents: Agent[] | null = null

vi.mock('@/queries/useAgents', () => ({
  useAgents: () => ({ data: mutableAgents ?? mockAgents, isLoading: false }),
  useDeleteAgent: () => ({ mutate: vi.fn() }),
  useCreateAgent: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUpdateAgent: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUpdateSubagents: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUpdateAgentTools: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUpdateAgentSkills: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useProbeAgent: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useAgentKnowledgeDatasets: () => ({ data: [], isLoading: false }),
  useUpdateAgentKnowledgeDatasets: () => ({ mutateAsync: vi.fn(), isPending: false })
}))

vi.mock('@/queries/useTools', () => ({
  useTools: () => ({ data: [{ id: 1, name: 'search', title: '搜索', isDefault: true }] })
}))

vi.mock('@/queries/useSkills', () => ({
  useSkills: () => ({ data: [] })
}))

vi.mock('@/queries/useProviders', () => ({
  useProviders: () => ({
    data: [
      {
        id: 1, key: 'glm-cn', name: 'GLM Coding Plan',
        description: '', descriptionEn: '',
        protocol: 'anthropic', authStyle: 'api_key',
        baseUrl: 'https://example.com',
        fields: [
          { key: 'name', label: '名称', labelEn: 'Name', type: 'text', required: true, secret: false },
          { key: 'base_url', label: 'API 地址', labelEn: 'API URL', type: 'text', required: true, secret: false },
          { key: 'api_key', label: 'API 密钥', labelEn: 'API Key', type: 'password', required: false, secret: true },
        ],
        defaultModels: [
          { modelId: 'GLM-5-Turbo', displayName: 'GLM-5-Turbo', contextWindow: 200000, modelType: 'llm' },
          { modelId: 'embedding-3', displayName: 'Embedding-3', contextWindow: 8192, modelType: 'embedding' }
        ],
        iconKey: 'zhipu', builtin: false, lockedApiKey: 'test-api-key',
        createdAt: '', updatedAt: ''
      }
    ]
  }),
  useProbeConfig: () => ({
    mutateAsync: vi.fn().mockResolvedValue({
      data: { success: true, data: { success: true, latencyMs: 150 } }
    })
  })
}))

vi.mock('@/queries/useMcps', () => ({
  useMcps: () => ({ data: [] }),
  useUpdateAgentMcps: () => ({ mutateAsync: vi.fn(), isPending: false })
}))

vi.mock('@/queries/useKnowledge', () => ({
  useKnowledgeList: () => ({ data: { datasets: [], total: 0 }, isLoading: false })
}))

vi.mock('@/api/agents', () => ({
  agentApi: {
    getTools: vi.fn(),
    getSkills: vi.fn(),
    getDeployment: vi.fn(),
    deploy: vi.fn(),
    stopDeployment: vi.fn(),
    delete: vi.fn(),
  },
}))

import { agentApi } from '@/api/agents'

// 页面集成 useBulkAgentTask（内部 useQueryClient）后所有渲染都需要 QueryClientProvider
function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <ConfigProvider theme={antdTheme}>
      <QueryClientProvider client={qc}>
        <MemoryRouter>
          <AgentListPage />
        </MemoryRouter>
      </QueryClientProvider>
    </ConfigProvider>
  )
}

describe('AgentListPage', () => {
  beforeEach(() => {
    setAuthRole('admin')
  })

  it('renders agent cards with names and stats', () => {
    renderPage()

    expect(screen.getByText('Agent 管理')).toBeInTheDocument()
    expect(screen.getByText('新建代理')).toBeInTheDocument()
    expect(screen.getByText('通用助手')).toBeInTheDocument()
    expect(screen.getByText('编程助手')).toBeInTheDocument()
    // Stats links
    expect(screen.getByText(/1 子代理/)).toBeInTheDocument()
    expect(screen.getByText(/1 工具/)).toBeInTheDocument()
    // Model stat link — selected state shows displayName
    expect(screen.getByText('GLM-5-Turbo')).toBeInTheDocument()
    // Model stat link — unselected state shows "未选模型"
    expect(screen.getByText('未选模型')).toBeInTheDocument()
  })

  it('shows model modal with test and confirm buttons', async () => {
    const user = userEvent.setup()
    renderPage()

    // Click model tag on first agent card
    await user.click(screen.getByText('GLM-5-Turbo'))

    // Modal should open
    expect(screen.getByText('设置模型')).toBeInTheDocument()

    // Footer buttons should be present.
    // Note: use regex matchers because antd inserts a space between CJK
    // characters in the computed accessible name (e.g. "取 消" not "取消").
    // This is an antd / aria-labelledby workaround for screen readers; the
    // regex tolerates both shapes.
    expect(screen.getByRole('button', { name: /取.?消/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /测.?试/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /确.?认/ })).toBeInTheDocument()
  })

  it('renders deploy button for each agent', async () => {
    renderPage()
    const deployButtons = await screen.findAllByTitle('部署')
    expect(deployButtons.length).toBe(mockAgents.length)
  })

  it('member: hides write actions but still sees agent data and deploy button', () => {
    setAuthRole('member')
    renderPage()

    // 数据仍可见（只读）
    expect(screen.getByText('Agent 管理')).toBeInTheDocument()
    expect(screen.getByText('通用助手')).toBeInTheDocument()
    expect(screen.getByText('编程助手')).toBeInTheDocument()
    // 部署按钮全角色可见（member 打开弹窗看状态 + 聊天入口）
    expect(screen.queryAllByTitle('部署')).toHaveLength(mockAgents.length)
    // 写操作按钮隐藏：新建代理、编辑/删除
    expect(screen.queryByText('新建代理')).not.toBeInTheDocument()
    expect(screen.queryAllByTitle('编辑')).toHaveLength(0)
    expect(screen.queryAllByTitle('删除')).toHaveLength(0)
  })

  it('modelId dropdown excludes non-LLM models', async () => {
    const user = userEvent.setup()
    renderPage()

    // Open the model config modal by clicking the model display name on the
    // first agent card (provider already pre-selected from agent.config).
    await user.click(screen.getByText('GLM-5-Turbo'))
    expect(screen.getByText('设置模型')).toBeInTheDocument()

    // The AutoComplete input shows the currently-selected model. Clear it so
    // the AutoComplete's own filterOption doesn't mask the modelType filter.
    const modelInput = screen.getByDisplayValue('GLM-5-Turbo')
    await user.clear(modelInput)

    // Trigger a re-open of the dropdown by focusing and searching with the
    // empty string (matches all options).
    await user.click(modelInput)

    // Dropdown options render with the label format `${displayName} (${modelId})`.
    // LLM model should appear in the dropdown options.
    expect(screen.queryAllByText('GLM-5-Turbo (GLM-5-Turbo)').length).toBeGreaterThan(0)

    // Embedding model must NOT appear in the dropdown options.
    expect(screen.queryAllByText('Embedding-3 (embedding-3)')).toHaveLength(0)
  })
})

describe('AgentListPage bulk operations (#141)', () => {
  beforeEach(() => {
    vi.mocked(agentApi.getDeployment).mockReset()
    vi.mocked(agentApi.deploy).mockReset()
  })

  afterEach(() => { mutableAgents = null })

  it('admin sees 批量操作 entry; member does not', () => {
    setAuthRole('admin')
    const { unmount } = renderPage()
    expect(screen.getByRole('button', { name: /批量操作/ })).toBeInTheDocument()
    unmount()

    setAuthRole('member')
    renderPage()
    expect(screen.queryByRole('button', { name: /批量操作/ })).not.toBeInTheDocument()
  })

  it('enter selection mode → click cards → bulk bar shows count; exit restores', async () => {
    setAuthRole('admin')
    const user = userEvent.setup()
    renderPage()

    await user.click(screen.getByRole('button', { name: /批量操作/ }))
    expect(screen.getByTestId('bulk-action-bar')).toBeInTheDocument()
    // review P2a：选择模式下搜索框保留（跨搜索选择保留的前提）
    expect(screen.getByPlaceholderText('搜索代理名称')).toBeInTheDocument()

    // 选择卡片的 wrap 层带 aria-label「选择 <name>」
    await user.click(screen.getByLabelText('选择 general'))
    await user.click(screen.getByLabelText('选择 coder'))
    expect(screen.getByText('已选 2 个')).toBeInTheDocument()

    // antd 两汉字按钮自动插空格（「退 出」）
    await user.click(screen.getByRole('button', { name: /^退\s*出$/ }))
    expect(screen.queryByTestId('bulk-action-bar')).not.toBeInTheDocument()
    // 单卡操作恢复（每个 agent 卡片一个部署按钮）
    expect(screen.queryAllByTitle('部署')).toHaveLength(mockAgents.length)
  })

  it('deploy flow: precheck → confirm modal with groups → confirm executes deploy API', async () => {
    setAuthRole('admin')
    const user = userEvent.setup()
    // general 未部署（not_found → 可执行）；coder 运行中（running → 跳过）
    vi.mocked(agentApi.getDeployment).mockImplementation(async (name: string) => ({
      data: { success: true, data: name === 'general' ? { status: 'not_found' } : { status: 'running' } },
    } as never))
    vi.mocked(agentApi.deploy).mockResolvedValue({ data: { success: true } } as never)
    renderPage()

    await user.click(screen.getByRole('button', { name: /批量操作/ }))
    await user.click(screen.getByLabelText('选择 general'))
    await user.click(screen.getByLabelText('选择 coder'))
    await user.click(screen.getByRole('button', { name: '部署' }))

    await waitFor(() => { expect(agentApi.getDeployment).toHaveBeenCalledTimes(2) })
    expect(await screen.findByText('可执行 · 1')).toBeInTheDocument()
    expect(screen.getByText('跳过 · 1')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '部署 1 个' }))
    await waitFor(() => { expect(agentApi.deploy).toHaveBeenCalledWith('general') })
    // 进度 Modal 打开（执行中）
    expect(await screen.findByText('批量部署进度')).toBeInTheDocument()
  })

  it('全选待更新 selects only agents with pendingArtifactUpdates', async () => {
    setAuthRole('admin')
    const user = userEvent.setup()
    renderPage()
    await user.click(screen.getByRole('button', { name: /批量操作/ }))
    // coder 带 pendingArtifactUpdates（mockAgents），general 无
    await user.click(screen.getByRole('button', { name: /^全\s*选待更新$/ }))
    expect(screen.getByText('已选 1 个')).toBeInTheDocument()
  })

  it('group header shows 全选本组 in selection mode', async () => {
    setAuthRole('admin')
    const user = userEvent.setup()
    renderPage()
    await user.click(screen.getByRole('button', { name: /批量操作/ }))
    const link = screen.getAllByText('全选本组')[0]!
    await user.click(link)
    expect(screen.getByText('已选 2 个')).toBeInTheDocument() // 默认分组 2 个 agent
  })

  it('selection persists across search filtering (review P2a)', async () => {
    setAuthRole('admin')
    const user = userEvent.setup()
    renderPage()
    await user.click(screen.getByRole('button', { name: /批量操作/ }))
    await user.click(screen.getByLabelText('选择 general'))
    expect(screen.getByText('已选 1 个')).toBeInTheDocument()

    // 搜索 coder：general 卡片隐藏，但选择保留
    await user.type(screen.getByPlaceholderText('搜索代理名称'), 'coder')
    expect(screen.queryByLabelText('选择 general')).not.toBeInTheDocument()
    expect(screen.getByText('已选 1 个')).toBeInTheDocument()

    await user.click(screen.getByLabelText('选择 coder'))
    expect(screen.getByText('已选 2 个')).toBeInTheDocument()

    // 清空搜索：general 回来，两个仍选中
    await user.clear(screen.getByPlaceholderText('搜索代理名称'))
    expect(screen.getByText('已选 2 个')).toBeInTheDocument()
  })

  it('removed agent is dropped from selection and not resurrected on reappearance (review P2b)', async () => {
    setAuthRole('admin')
    const user = userEvent.setup()
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    // 工厂函数：rerender 需要新元素引用——相同引用会被 React bailout，组件不重执行
    const makePage = () => (
      <ConfigProvider theme={antdTheme}>
        <QueryClientProvider client={qc}>
          <MemoryRouter>
            <AgentListPage />
          </MemoryRouter>
        </QueryClientProvider>
      </ConfigProvider>
    )
    const { rerender } = render(makePage())
    await user.click(screen.getByRole('button', { name: /批量操作/ }))
    await user.click(screen.getByLabelText('选择 general'))
    await user.click(screen.getByLabelText('选择 coder'))
    expect(screen.getByText('已选 2 个')).toBeInTheDocument()

    // coder 从列表消失（新数组引用 = 真实列表刷新）：选中集真正剔除
    const general = mockAgents.find((a) => a.name === 'general')!
    const coder = mockAgents.find((a) => a.name === 'coder')!
    mutableAgents = [general]
    rerender(makePage())
    expect(await screen.findByText('已选 1 个')).toBeInTheDocument()

    // coder 重现（新数组引用）：不被静默复活选中
    mutableAgents = [general, coder]
    rerender(makePage())
    expect(await screen.findByLabelText('选择 coder')).toBeInTheDocument()
    expect(screen.getByText('已选 1 个')).toBeInTheDocument()
  })

  it('stale precheck result is discarded after exiting selection mode (review re-check P2)', async () => {
    setAuthRole('admin')
    const user = userEvent.setup()
    // 手动控制预检 promise（deferred）：保持 pending 直到测试放行
    let releasePrecheck: (() => void) | undefined
    vi.mocked(agentApi.getDeployment).mockImplementation(async () => {
      await new Promise<void>((r) => { releasePrecheck = r })
      return { data: { success: true, data: { status: 'not_found' } } } as never
    })
    renderPage()

    await user.click(screen.getByRole('button', { name: /批量操作/ }))
    await user.click(screen.getByLabelText('选择 general'))
    await user.click(screen.getByRole('button', { name: '部署' })) // 预检 pending

    // 预检 pending 期间退出选择模式
    await user.click(screen.getByRole('button', { name: /^退\s*出$/ }))
    expect(screen.queryByTestId('bulk-action-bar')).not.toBeInTheDocument()

    // 预检完成：旧批次结果被代次守卫丢弃，确认弹窗不出现
    await act(async () => { releasePrecheck?.() })
    expect(agentApi.getDeployment).toHaveBeenCalledTimes(1)
    expect(screen.queryByText('批量部署')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '部署 1 个' })).not.toBeInTheDocument()
  })
})
