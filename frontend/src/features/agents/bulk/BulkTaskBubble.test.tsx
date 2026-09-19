import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import BulkTaskBubble from './BulkTaskBubble'

function renderBubble(props: Partial<Parameters<typeof BulkTaskBubble>[0]> = {}) {
  const base = {
    visible: true,
    phase: 'running' as const,
    progressText: '2/5',
    dot: null,
    onClick: vi.fn(),
    ...props,
  }
  render(<BulkTaskBubble {...base} />)
  return base
}

describe('BulkTaskBubble', () => {
  it('hidden when not visible', () => {
    renderBubble({ visible: false })
    expect(screen.queryByTestId('bulk-task-bubble')).not.toBeInTheDocument()
  })

  it('running: shows progress text; click reopens', async () => {
    const user = userEvent.setup()
    const p = renderBubble()
    expect(screen.getByText('2/5')).toBeInTheDocument()
    await user.click(screen.getByTestId('bulk-task-bubble'))
    expect(p.onClick).toHaveBeenCalledTimes(1)
  })

  it('done with failures: red dot（failed/blocked 任一非零，spec §4.4）', () => {
    renderBubble({ phase: 'done', progressText: '5/5', dot: 'red' })
    expect(screen.getByTestId('bulk-bubble-dot-red')).toBeInTheDocument()
    expect(screen.queryByTestId('bulk-bubble-dot-green')).not.toBeInTheDocument()
  })

  it('done all-success: green dot', () => {
    renderBubble({ phase: 'done', progressText: '5/5', dot: 'green' })
    expect(screen.getByTestId('bulk-bubble-dot-green')).toBeInTheDocument()
  })
})
