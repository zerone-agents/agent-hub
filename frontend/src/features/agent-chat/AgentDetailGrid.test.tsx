import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ConfigProvider } from 'antd'
import { antdTheme } from '@/lib/antd-theme'
import AgentDetailGrid from './AgentDetailGrid'
import type { AgentDetail } from '@/api/agents'

function renderWith(ui: React.ReactElement) {
  return render(<ConfigProvider theme={antdTheme}>{ui}</ConfigProvider>)
}

const fullDetail: Pick<
  AgentDetail,
  'allowedTools' | 'mcpServers' | 'subagents' | 'datasets' | 'availableSkills' | 'maxTurns' | 'maxSessionQueries'
> = {
  maxTurns: 10,
  allowedTools: ['Bash', 'Read', 'Write'],
  mcpServers: {
    github: { transport: 'stdio', command: 'mcp-server-github' },
    remote: { transport: 'sse', url: 'https://mcp.example.com/sse' },
  },
  subagents: {
    coder: { description: 'Write code' },
    researcher: { description: 'Research topics' },
  },
  datasets: { 'kb-001': 'Knowledge base description' },
  availableSkills: [
    { name: 'commit', description: 'Commit changes with conventional message', source: 'project', location: '/app/.agents/skills/commit/SKILL.md' },
    { name: 'review', description: 'Review code for issues', source: 'project', location: '/app/.agents/skills/review/SKILL.md' },
  ],
}

describe('AgentDetailGrid', () => {
  it('renders all 5 categories when fully populated', () => {
    renderWith(<AgentDetailGrid {...fullDetail} />)

    expect(screen.getByText('Tools')).toBeInTheDocument()
    expect(screen.getByText('MCP')).toBeInTheDocument()
    expect(screen.getByText('Subagents')).toBeInTheDocument()
    expect(screen.getByText('Datasets')).toBeInTheDocument()
    expect(screen.getByText('Skills')).toBeInTheDocument()

    expect(screen.getByText('Bash')).toBeInTheDocument()
    expect(screen.getByText('github · stdio')).toBeInTheDocument()
    expect(screen.getByText('coder')).toBeInTheDocument()
    expect(screen.getByText('Knowledge …')).toBeInTheDocument()
    expect(screen.getByText('commit')).toBeInTheDocument()
  })

  it('hides Skills row when availableSkills array is empty but keeps other rows', () => {
    renderWith(<AgentDetailGrid {...fullDetail} availableSkills={[]} />)

    expect(screen.queryByText('Skills')).not.toBeInTheDocument()
    expect(screen.getByText('Tools')).toBeInTheDocument()
  })

  it('hides MCP row when mcpServers is undefined', () => {
    renderWith(<AgentDetailGrid {...fullDetail} mcpServers={undefined} />)

    expect(screen.queryByText('MCP')).not.toBeInTheDocument()
    expect(screen.getByText('Tools')).toBeInTheDocument()
  })

  it('returns null when all 5 categories are empty/undefined', () => {
    const { container } = renderWith(
      <AgentDetailGrid
        allowedTools={[]}
        mcpServers={undefined}
        subagents={undefined}
        datasets={undefined}
        availableSkills={[]}
        maxTurns={10}
      />
    )
    expect(container.firstChild).toBeNull()
  })

  it('renders Tag for each dataset description, truncating over 10 chars', () => {
    renderWith(
      <AgentDetailGrid
        allowedTools={[]}
        datasets={{ 'kb-001': 'desc 1', 'kb-002': 'desc 2' }}
        maxTurns={10}
      />
    )
    expect(screen.getByText('desc 1')).toBeInTheDocument()
    expect(screen.getByText('desc 2')).toBeInTheDocument()
  })

  it('truncates dataset description over 10 characters', () => {
    renderWith(
      <AgentDetailGrid
        allowedTools={[]}
        datasets={{ 'kb-001': 'this is a very long description' }}
        maxTurns={10}
      />
    )
    expect(screen.getByText('this is a …')).toBeInTheDocument()
  })

  it('renders Limits row with maxTurns and maxSessionQueries when maxSessionQueries is set', () => {
    renderWith(<AgentDetailGrid {...fullDetail} maxTurns={50} maxSessionQueries={20} />)

    expect(screen.getByText('Limits')).toBeInTheDocument()
    expect(screen.getByText('maxTurns: 50')).toBeInTheDocument()
    expect(screen.getByText('maxSessionQueries: 20')).toBeInTheDocument()
  })

  it('hides Limits row when maxSessionQueries is undefined', () => {
    renderWith(<AgentDetailGrid {...fullDetail} />)

    expect(screen.queryByText('Limits')).not.toBeInTheDocument()
    expect(screen.queryByText(/^maxTurns:/)).not.toBeInTheDocument()
  })

  it('renders Limits row alone when only maxSessionQueries is set', () => {
    renderWith(
      <AgentDetailGrid
        allowedTools={[]}
        mcpServers={undefined}
        subagents={undefined}
        datasets={undefined}
        availableSkills={[]}
        maxTurns={10}
        maxSessionQueries={30}
      />
    )

    expect(screen.getByText('Limits')).toBeInTheDocument()
    expect(screen.getByText('maxTurns: 10')).toBeInTheDocument()
    expect(screen.getByText('maxSessionQueries: 30')).toBeInTheDocument()
    expect(screen.queryByText('Tools')).not.toBeInTheDocument()
  })
})
