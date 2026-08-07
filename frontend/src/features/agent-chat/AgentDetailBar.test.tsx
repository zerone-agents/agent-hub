import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { ConfigProvider } from 'antd'
import { antdTheme } from '@/lib/antd-theme'
import AgentDetailBar from './AgentDetailBar'

vi.mock('@/queries/useAgentDetail', () => ({
  useAgentDetail: vi.fn(),
}))

import { useAgentDetail } from '@/queries/useAgentDetail'
import type { AgentDetail } from '@/api/agents'

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

describe('AgentDetailBar', () => {
  beforeEach(() => vi.clearAllMocks())

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
    renderWith(<AgentDetailBar agentName="x" />)

    expect(screen.getByText('threapy-agent')).toBeInTheDocument()
    // Grid content not visible (collapsed) — use a grid-only tag text, not a
    // count label that also appears in the summary bar.
    expect(screen.queryByText('Bash')).not.toBeInTheDocument()
  })

  it('shows grid after clicking summary bar', () => {
    ;(useAgentDetail as any).mockReturnValue({ data: fullDetail, isLoading: false, isError: false })
    renderWith(<AgentDetailBar agentName="x" />)

    fireEvent.click(screen.getByText('threapy-agent'))
    expect(screen.getByText('Bash')).toBeInTheDocument()
    expect(screen.getByText('coder')).toBeInTheDocument()
  })

  it('hides grid again on second click', () => {
    ;(useAgentDetail as any).mockReturnValue({ data: fullDetail, isLoading: false, isError: false })
    renderWith(<AgentDetailBar agentName="x" />)

    fireEvent.click(screen.getByText('threapy-agent'))
    expect(screen.getByText('Bash')).toBeInTheDocument()

    fireEvent.click(screen.getByText('threapy-agent'))
    expect(screen.queryByText('Bash')).not.toBeInTheDocument()
  })
})
