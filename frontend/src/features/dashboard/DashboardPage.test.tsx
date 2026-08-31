import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { ConfigProvider } from 'antd'
import { antdTheme } from '@/lib/antd-theme'
import DashboardPage from './DashboardPage'
import type { Agent } from '@/api/agents'
import type { Tool } from '@/api/tools'
import type { Skill } from '@/api/skills'
import type { Scene } from '@/api/scenes'
import type { Provider } from '@/api/providers'

import type { Mcp } from '@/api/mcps'
import type { KnowledgeDataset } from '@/api/knowledge'
import type { ChatSession } from '@/api/chat'

const mockAgents: Agent[] = [
  { id: 1, name: 'general', config: { title: { zh: '通用助手' } }, desktopEnabled: true, createdAt: '2026-06-10T10:00:00Z' },
  { id: 2, name: 'coder', config: { title: { zh: '编程助手' } }, desktopEnabled: false, createdAt: '2026-06-15T10:00:00Z' }
]
const mockTools: Tool[] = [
  { id: 1, name: 'search', title: '搜索', description: '', isDefault: false, source: 'builtin', createdAt: '2026-06-12T10:00:00Z', updatedAt: '' }
]
const mockSkills: Skill[] = [
  { id: 1, name: 'py', type: 'expert', title: 'Python', titleEn: 'Python', description: '', descriptionEn: '', url: '', fileHash: '', fileSize: 0, createdAt: '2026-06-14T10:00:00Z', updatedAt: '' },
  { id: 2, name: 'go', type: 'community', title: 'Go', titleEn: 'Go', description: '', descriptionEn: '', url: '', fileHash: '', fileSize: 0, createdAt: '2026-06-16T10:00:00Z', updatedAt: '' }
]
const mockScenes: Scene[] = [
  { id: 1, name: 'chat', agent: 'general', title: '聊天', titleEn: 'Chat', prompt: '', promptEn: '', enabled: true, createdAt: '2026-06-11T10:00:00Z', updatedAt: '' }
]
const mockMcps: Mcp[] = [
  { id: 1, name: 'filesystem', title: '文件系统', description: '', transportType: 'sse', url: 'https://example.com/sse', hasHeaders: false, isBuiltin: false, probeStatus: 'pending', createdAt: '2026-07-01T10:00:00Z', updatedAt: '' }
]
const mockProviders: Provider[] = [
  { id: 1, key: 'anthropic', name: 'Anthropic', description: '', descriptionEn: '', protocol: 'anthropic', authStyle: 'api_key', baseUrl: '', defaultModels: [{ modelId: 'claude-sonnet-4', displayName: 'Claude Sonnet 4', modelType: 'llm' }], fields: [], iconKey: '', builtin: true, lockedApiKey: '', attributes: {}, createdAt: '2026-06-13T10:00:00Z', updatedAt: '' }
]
const mockKnowledgeDatasets: KnowledgeDataset[] = [
  {
    id: 'kb-1',
    name: 'product-docs',
    display_name: '产品文档',
    collection_name: 'product-docs',
    description: '',
    permission: 'me',
    doc_num: 12,
    chunk_num: 86,
    parser_id: 'naive',
    embd_id: 'embedding',
    parser_config: {},
    create_date: '2026-06-17T10:00:00Z'
  }
]
const mockChatSessions: ChatSession[] = [
  {
    user_id: 'u1',
    id: 'session-1',
    title: '检查知识库',
    created_at: '2026-06-18T09:00:00Z',
    updated_at: '2026-06-18T10:00:00Z',
    model: 'claude-sonnet-4',
    system_prompt: '',
    status: 'active',
    mode: 'agent',
    provider_id: 'anthropic',
    agent_id: 'general',
    permission_profile: 'default',
    hidden: false,
    extra_directories: '',
    is_user_bound: true
  }
]

vi.mock('@/queries/useDashboardStats', () => ({
  useDashboardStats: () => ({
    agents: mockAgents,
    tools: mockTools,
    skills: mockSkills,
    scenes: mockScenes,
    providers: mockProviders,
    mcps: mockMcps,
    knowledgeDatasets: mockKnowledgeDatasets,
    chatSessions: mockChatSessions,
    chatSessionTotal: 7,
    isLoading: false,
    isError: false,
    refetch: vi.fn()
  })
}))

describe('DashboardPage', () => {
  it('renders resource visuals and the activity feed', () => {
    render(
      <ConfigProvider theme={antdTheme}>
        <MemoryRouter>
          <DashboardPage />
        </MemoryRouter>
      </ConfigProvider>
    )

    // Page header
    expect(screen.getByText('仪表盘')).toBeInTheDocument()

    // Resource stat and composition legend
    expect(screen.getAllByText('MCP 配置').length).toBeGreaterThanOrEqual(2)
    expect(screen.getAllByText('知识库').length).toBeGreaterThanOrEqual(2)
    expect(screen.getAllByText('模型').length).toBeGreaterThanOrEqual(2)
    const resourceStats = screen.getByLabelText('资源统计')
    expect([...resourceStats.querySelectorAll('button')].map((button) => button.textContent)).toEqual([
      '2Agent',
      '1工具',
      '1MCP 配置',
      '2技能',
      '1提供方',
      '1模型',
      '1知识库',
      '1场景'
    ])

    // Visual overview
    expect(screen.getByText('配置健康度')).toBeInTheDocument()
    expect(screen.getByRole('img', { name: '最近八周资源新增趋势' })).toBeInTheDocument()
    expect(screen.getByText('资源构成')).toBeInTheDocument()

    // Feed items (agent names appear as titles)
    expect(screen.getAllByText('编程助手').length).toBeGreaterThanOrEqual(1)

    // Recent MCP
    expect(screen.getAllByText('文件系统').length).toBeGreaterThanOrEqual(1)
    expect(screen.getByText('产品文档')).toBeInTheDocument()
    expect(screen.queryByText('检查知识库')).not.toBeInTheDocument()
    expect(screen.getByText('12/86')).toBeInTheDocument()
  })
})
