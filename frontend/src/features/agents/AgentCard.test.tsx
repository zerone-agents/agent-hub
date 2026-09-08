import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ConfigProvider } from 'antd'
import { antdTheme } from '@/lib/antd-theme'
import AgentCard from './AgentCard'
import type { Agent } from '@/api/agents'

function renderWith(ui: React.ReactElement) {
  return render(<ConfigProvider theme={antdTheme}>{ui}</ConfigProvider>)
}

const baseAgent: Agent = {
  id: 1,
  name: 'general',
  config: {
    title: { zh: '通用助手' },
    description: { zh: '通用对话代理' },
    iconName: 'Robot',
    iconColor: '#06B6D4',
    iconBgColor: '#E6F8FC'
  },
  subagents: [],
  tools: ['search'],
  skills: [],
  mcps: [],
  datasets: [],
  createdAt: '2026-06-10T10:00:00Z'
}

function renderCard(agent: Agent) {
  return renderWith(
    <AgentCard
      agent={agent}
      modelDisplayName="GLM-5-Turbo"
      canWrite
      onEdit={vi.fn()}
      onDelete={vi.fn()}
      onEditSubagents={vi.fn()}
      onEditTools={vi.fn()}
      onEditSkills={vi.fn()}
      onEditMcps={vi.fn()}
      onEditModel={vi.fn()}
      onDeploy={vi.fn()}
      onEditKnowledge={vi.fn()}
    />
  )
}

describe('AgentCard pending artifact badge (#86)', () => {
  it('shows 待更新 badge when pendingArtifactUpdates non-empty', () => {
    renderCard({ ...baseAgent, pendingArtifactUpdates: { tools: ['calc'], skills: [] } })
    expect(screen.getByText('待更新')).toBeInTheDocument()
  })

  it('hides 待更新 badge when pendingArtifactUpdates is null or empty', () => {
    const cases = [
      null,
      { tools: [], skills: [] }
    ]
    for (const updates of cases) {
      renderCard({ ...baseAgent, pendingArtifactUpdates: updates })
      expect(screen.queryByText('待更新')).not.toBeInTheDocument()
    }
  })
})