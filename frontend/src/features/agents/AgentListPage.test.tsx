import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { ConfigProvider } from 'antd'
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
    desktopEnabled: false, isDefault: false, createdAt: '2026-06-15T10:00:00Z'
  }
]

vi.mock('@/queries/useAgents', () => ({
  useAgents: () => ({ data: mockAgents, isLoading: false }),
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
  agentApi: { getTools: vi.fn(), getSkills: vi.fn() }
}))

describe('AgentListPage', () => {
  beforeEach(() => {
    setAuthRole('admin')
  })

  it('renders agent cards with names and stats', () => {
    render(
      <ConfigProvider theme={antdTheme}>
        <MemoryRouter>
          <AgentListPage />
        </MemoryRouter>
      </ConfigProvider>
    )

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
    render(
      <ConfigProvider theme={antdTheme}>
        <MemoryRouter>
          <AgentListPage />
        </MemoryRouter>
      </ConfigProvider>
    )

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
    render(
      <ConfigProvider theme={antdTheme}>
        <MemoryRouter>
          <AgentListPage />
        </MemoryRouter>
      </ConfigProvider>
    )
    const deployButtons = await screen.findAllByTitle('部署')
    expect(deployButtons.length).toBe(mockAgents.length)
  })

  it('member: hides write actions but still sees agent data', () => {
    setAuthRole('member')
    render(
      <ConfigProvider theme={antdTheme}>
        <MemoryRouter>
          <AgentListPage />
        </MemoryRouter>
      </ConfigProvider>
    )

    // 数据仍可见（只读）
    expect(screen.getByText('Agent 管理')).toBeInTheDocument()
    expect(screen.getByText('通用助手')).toBeInTheDocument()
    expect(screen.getByText('编程助手')).toBeInTheDocument()
    // 写操作按钮隐藏：新建代理、部署/编辑/删除
    expect(screen.queryByText('新建代理')).not.toBeInTheDocument()
    expect(screen.queryAllByTitle('部署')).toHaveLength(0)
    expect(screen.queryAllByTitle('编辑')).toHaveLength(0)
    expect(screen.queryAllByTitle('删除')).toHaveLength(0)
  })

  it('modelId dropdown excludes non-LLM models', async () => {
    const user = userEvent.setup()
    render(
      <ConfigProvider theme={antdTheme}>
        <MemoryRouter>
          <AgentListPage />
        </MemoryRouter>
      </ConfigProvider>
    )

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
