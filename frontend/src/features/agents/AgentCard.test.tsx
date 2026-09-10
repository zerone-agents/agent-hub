import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
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

function renderSelectable(agent: Agent, selected = false) {
  const onToggleSelect = vi.fn()
  renderWith(
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
      selectionMode
      selected={selected}
      onToggleSelect={onToggleSelect}
    />
  )
  return { onToggleSelect }
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

describe('AgentCard selection mode (#141)', () => {
  it('renders checkbox and hides single-card action buttons', () => {
    const { onToggleSelect } = renderSelectable(baseAgent)
    expect(onToggleSelect).toBeDefined()
    expect(screen.getByRole('checkbox')).toBeInTheDocument()
    expect(screen.queryByTitle('部署')).not.toBeInTheDocument()
    expect(screen.queryByTitle('编辑')).not.toBeInTheDocument()
    expect(screen.queryByTitle('删除')).not.toBeInTheDocument()
  })

  it('clicking the card toggles selection', async () => {
    const user = userEvent.setup()
    const { onToggleSelect } = renderSelectable(baseAgent)
    await user.click(screen.getByRole('checkbox'))
    expect(onToggleSelect).toHaveBeenCalledWith('general')
    // 整卡点击也切换（点标题区域）
    await user.click(screen.getByText('通用助手'))
    expect(onToggleSelect).toHaveBeenCalledTimes(2)
  })

  it('non-selection mode keeps action buttons and no checkbox', () => {
    renderCard(baseAgent) // 既有 helper：无 selection props
    expect(screen.queryByRole('checkbox')).not.toBeInTheDocument()
    expect(screen.getByTitle('部署')).toBeInTheDocument()
  })
})