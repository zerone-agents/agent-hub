import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { ConfigProvider } from 'antd'
import { antdTheme } from '@/lib/antd-theme'
import AgentDetailBar from './AgentDetailBar'

vi.mock('@/queries/useAgentDetail', () => ({
  useAgentDetail: vi.fn(),
}))

// usePublicAgents 与 useAgents 同文件导出，模块 mock 一并覆盖。组件现在只读
// usePublicAgents（公开列表）；保留 useAgents 导出使既有 mock 引用不炸。
vi.mock('@/queries/useAgents', () => ({
  useAgents: vi.fn(),
  usePublicAgents: vi.fn(),
}))

// isGuestUser 依赖：auth mode + 当前用户角色，控制 formal / guest 两态。
vi.mock('@/features/login/useAuthMode', () => ({
  useAuthMode: vi.fn(),
}))

vi.mock('@/queries/useUserInfo', () => ({
  useUserInfo: vi.fn(),
}))

import { useAgentDetail } from '@/queries/useAgentDetail'
import { useAgents, usePublicAgents } from '@/queries/useAgents'
import { useAuthMode } from '@/features/login/useAuthMode'
import { useUserInfo } from '@/queries/useUserInfo'
import type { Agent, AgentDetail } from '@/api/agents'

function renderWith(ui: React.ReactElement) {
  return render(<ConfigProvider theme={antdTheme}>{ui}</ConfigProvider>)
}

const fullDetail: AgentDetail = {
  id: 'threapy',
  name: 'threapy-agent',
  model: 'qwen3.7-plus',
  status: 'ready',
  maxTurns: 50,
  hasSystemPrompt: true,
  allowedTools: ['Bash', 'Read'],
  mcpServers: { github: { transport: 'stdio', command: 'srv' } },
  subagents: { coder: { description: 'write code' } },
  datasets: { 'kb-001': 'desc' },
  availableSkills: [],
}

/** view=chat 公开列表条目（guest 降级分支的唯一数据源）。 */
const publicAgent: Agent = {
  id: 1,
  name: 'threapy-agent',
  config: { title: { zh: '疗愈助手', en: 'Therapy' }, modelId: 'qwen3.7-plus' },
  tools: ['Bash', 'Read'],
  mcps: ['github'],
  skills: [],
  subagents: ['coder'],
  datasets: ['kb-001'],
}

describe('AgentDetailBar', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // formal 用户默认态（既有用例全走此分支）。
    ;(useAuthMode as any).mockReturnValue({ data: { mode: 'builtin' } })
    ;(useUserInfo as any).mockReturnValue({ data: { id: 'u1', name: 'Admin', role: 'admin' } })
    ;(useAgents as any).mockReturnValue({ data: [] })
    ;(usePublicAgents as any).mockReturnValue({ data: [] })
  })

  it('renders null when isLoading', () => {
    ;(useAgentDetail as any).mockReturnValue({ data: undefined, isLoading: true, isError: false })
    const { container } = renderWith(<AgentDetailBar agentName="x" />)
    expect(container.firstChild).toBeNull()
  })

  it('renders null when isError', () => {
    ;(useAgentDetail as any).mockReturnValue({ data: undefined, isLoading: false, isError: true })
    const { container } = renderWith(<AgentDetailBar agentName="x" />)
    expect(container.firstChild).toBeNull()
  })

  it('renders summary bar when data is ready, grid hidden by default', () => {
    ;(useAgentDetail as any).mockReturnValue({ data: fullDetail, isLoading: false, isError: false })
    renderWith(<AgentDetailBar agentName="threapy-agent" />)

    expect(screen.getByText('threapy-agent')).toBeInTheDocument()
    // Grid content not visible (collapsed) — use a grid-only tag text, not a
    // count label that also appears in the summary bar.
    expect(screen.queryByText('Bash')).not.toBeInTheDocument()
  })

  it('shows grid after clicking summary bar', () => {
    ;(useAgentDetail as any).mockReturnValue({ data: fullDetail, isLoading: false, isError: false })
    renderWith(<AgentDetailBar agentName="threapy-agent" />)

    fireEvent.click(screen.getByText('threapy-agent'))
    expect(screen.getByText('Bash')).toBeInTheDocument()
    expect(screen.getByText('coder')).toBeInTheDocument()
  })

  it('prefers hub agent title over technical name when available', () => {
    ;(useAgentDetail as any).mockReturnValue({ data: fullDetail, isLoading: false, isError: false })
    ;(usePublicAgents as any).mockReturnValue({
      data: [{ id: 1, name: 'threapy-agent', config: { title: { zh: '疗愈助手', en: 'Therapy' } } }],
    })
    renderWith(<AgentDetailBar agentName="threapy-agent" />)
    expect(screen.getByText('疗愈助手')).toBeInTheDocument()
    expect(screen.queryByText('threapy-agent')).not.toBeInTheDocument()
  })

  it('falls back to technical name when hub agent record is missing', () => {
    ;(useAgentDetail as any).mockReturnValue({ data: fullDetail, isLoading: false, isError: false })
    ;(usePublicAgents as any).mockReturnValue({ data: [] })
    renderWith(<AgentDetailBar agentName="threapy-agent" />)
    expect(screen.getByText('threapy-agent')).toBeInTheDocument()
  })

  it('hides grid again on second click', () => {
    ;(useAgentDetail as any).mockReturnValue({ data: fullDetail, isLoading: false, isError: false })
    renderWith(<AgentDetailBar agentName="threapy-agent" />)

    fireEvent.click(screen.getByText('threapy-agent'))
    expect(screen.getByText('Bash')).toBeInTheDocument()

    fireEvent.click(screen.getByText('threapy-agent'))
    expect(screen.queryByText('Bash')).not.toBeInTheDocument()
  })

  describe('guest (degraded)', () => {
    beforeEach(() => {
      ;(useUserInfo as any).mockReturnValue({ data: { id: 'u2', name: 'Guest', role: 'guest' } })
      // enabled=false 时不发请求，query 停留在初始态（无数据无错误）。
      ;(useAgentDetail as any).mockReturnValue({ data: undefined, isLoading: false, isError: false })
    })

    it('renders summary from public list without firing admin detail request', () => {
      ;(usePublicAgents as any).mockReturnValue({ data: [publicAgent] })
      renderWith(<AgentDetailBar agentName="threapy-agent" />)

      // admin detail 请求被禁用（enabled: false 透传，不发注定 403 的请求）。
      expect(useAgentDetail).toHaveBeenCalledWith('threapy-agent', { enabled: false })
      expect(screen.getByText('疗愈助手')).toBeInTheDocument()
      expect(screen.getByText('qwen3.7-plus')).toBeInTheDocument()
      // counts 来自公开列表绑定名单长度。
      expect(screen.getByText((_, el) => el?.textContent === 'Tools 2')).toBeInTheDocument()
      expect(screen.getByText((_, el) => el?.textContent === 'MCP 1')).toBeInTheDocument()
      expect(screen.getByText((_, el) => el?.textContent === 'Datasets 1')).toBeInTheDocument()
    })

    it('has no expandable grid (click is a no-op)', () => {
      ;(usePublicAgents as any).mockReturnValue({ data: [publicAgent] })
      renderWith(<AgentDetailBar agentName="threapy-agent" />)

      fireEvent.click(screen.getByText('疗愈助手'))
      // Grid-only tag texts stay absent — guest 永远不渲染 AgentDetailGrid。
      expect(screen.queryByText('Bash')).not.toBeInTheDocument()
      expect(screen.queryByText('coder')).not.toBeInTheDocument()
    })

    it('renders null when public list lacks the agent', () => {
      ;(usePublicAgents as any).mockReturnValue({ data: [] })
      const { container } = renderWith(<AgentDetailBar agentName="threapy-agent" />)
      expect(container.firstChild).toBeNull()
      expect(useAgentDetail).toHaveBeenCalledWith('threapy-agent', { enabled: false })
    })
  })
})
