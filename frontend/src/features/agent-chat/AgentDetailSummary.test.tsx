import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ConfigProvider } from 'antd'
import { antdTheme } from '@/lib/antd-theme'
import AgentDetailSummary from './AgentDetailSummary'

function renderWith(ui: React.ReactElement) {
  return render(<ConfigProvider theme={antdTheme}>{ui}</ConfigProvider>)
}

const baseProps = {
  name: 'threapy-agent',
  model: 'qwen3.7-plus',
  status: 'ready' as const,
  counts: { tools: 7, mcps: 2, skills: 0, subagents: 2, datasets: 1 },
  expanded: false,
  onToggle: vi.fn(),
}

describe('AgentDetailSummary', () => {
  it('renders agent name, model, and 5 counts', () => {
    renderWith(<AgentDetailSummary {...baseProps} />)

    expect(screen.getByText('threapy-agent')).toBeInTheDocument()
    expect(screen.getByText('qwen3.7-plus')).toBeInTheDocument()
    expect(screen.getByText((_, el) => el?.textContent === 'Tools 7')).toBeInTheDocument()
    expect(screen.getByText((_, el) => el?.textContent === 'MCP 2')).toBeInTheDocument()
    expect(screen.getByText((_, el) => el?.textContent === 'Skills 0')).toBeInTheDocument()
    expect(screen.getByText((_, el) => el?.textContent === 'Subagents 2')).toBeInTheDocument()
    expect(screen.getByText((_, el) => el?.textContent === 'Datasets 1')).toBeInTheDocument()
  })

  it('shows zero count for empty categories (e.g. Skills 0)', () => {
    renderWith(<AgentDetailSummary {...baseProps} />)
    expect(screen.getByText((_, el) => el?.textContent === 'Skills 0')).toBeInTheDocument()
  })

  it('invokes onToggle when clicked', () => {
    const onToggle = vi.fn()
    renderWith(<AgentDetailSummary {...baseProps} onToggle={onToggle} />)

    fireEvent.click(screen.getByText('threapy-agent'))
    expect(onToggle).toHaveBeenCalledOnce()
  })

  it('does NOT render warning indicator when status is ready', () => {
    const { container } = renderWith(<AgentDetailSummary {...baseProps} />)
    expect(container.querySelector('[data-testid="status-warning"]')).not.toBeInTheDocument()
  })

  it('renders warning indicator when status is unavailable', () => {
    renderWith(<AgentDetailSummary {...baseProps} status="unavailable" />)
    expect(screen.getByTestId('status-warning')).toBeInTheDocument()
  })

  it('invokes onToggle when Enter is pressed on the summary bar', async () => {
    const onToggle = vi.fn()
    renderWith(<AgentDetailSummary {...baseProps} onToggle={onToggle} />)

    const user = userEvent.setup()
    const bar = screen.getByRole('button')
    bar.focus()
    await user.keyboard('{Enter}')
    expect(onToggle).toHaveBeenCalledOnce()
  })

  it('invokes onToggle when Space is pressed on the summary bar', async () => {
    const onToggle = vi.fn()
    renderWith(<AgentDetailSummary {...baseProps} onToggle={onToggle} />)

    const user = userEvent.setup()
    const bar = screen.getByRole('button')
    bar.focus()
    await user.keyboard(' ')
    expect(onToggle).toHaveBeenCalledOnce()
  })

  it('sets aria-expanded to false initially and true after expanding', () => {
    const { rerender } = renderWith(
      <AgentDetailSummary {...baseProps} expanded={false} />
    )

    const bar = screen.getByRole('button')
    expect(bar).toHaveAttribute('aria-expanded', 'false')

    rerender(
      <ConfigProvider theme={antdTheme}>
        <AgentDetailSummary {...baseProps} expanded={true} />
      </ConfigProvider>
    )
    expect(bar).toHaveAttribute('aria-expanded', 'true')
  })
})
