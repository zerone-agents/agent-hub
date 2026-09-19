import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import CollaborationAuditPage from './CollaborationAuditPage'

const chain = {
  conversationId: 'conv-1', rootMessageId: 'root-1', messageCount: 2, verified: false,
  messages: [
    { id: 'root-1', relationId: 1, sourceAgentId: 1, targetAgentId: 2, sourceAgent: 'A', targetAgent: 'B', scope: 's', action: 'assign', deliveryPolicy: 'async', contextPolicy: 'summary_only', status: 'completed', content: '调查', reply: '完成', conversationId: 'conv-1', rootMessageId: 'root-1', hop: 1, maxHops: 4, verified: true, verification: '回复已持久化', createdAt: '2026-09-13T00:00:00Z' },
    { id: 'child-1', relationId: 2, sourceAgentId: 2, targetAgentId: 3, sourceAgent: 'B', targetAgent: 'C', scope: 's', action: 'consult', deliveryPolicy: 'async', contextPolicy: 'summary_only', status: 'running', conversationId: 'conv-1', rootMessageId: 'root-1', parentMessageId: 'root-1', hop: 2, maxHops: 4, verified: false, verification: '等待回复', createdAt: '2026-09-13T00:01:00Z' },
  ],
}

vi.mock('@/queries/useAgentMessageAudit', () => ({
  useAgentMessageChain: (selector?: unknown) => ({ data: selector ? chain : undefined, isLoading: false, isError: false }),
}))

describe('CollaborationAuditPage', () => {
  it('queries and presents durable evidence instead of agent claims', () => {
    render(<MemoryRouter><CollaborationAuditPage /></MemoryRouter>)
    expect(screen.getByText(/不采用 Agent 自己对执行过程的描述/)).toBeInTheDocument()
    fireEvent.change(screen.getByLabelText('审计线索'), { target: { value: 'conv-1' } })
    fireEvent.click(screen.getByRole('button', { name: /查询证据/ }))
    expect(screen.getByText('协作链仍有未验证结果')).toBeInTheDocument()
    expect(screen.getByText('A')).toBeInTheDocument()
    expect(screen.getByText('C')).toBeInTheDocument()
    expect(screen.getByText(/核验：回复已持久化/)).toBeInTheDocument()
    expect(screen.getByText(/核验：等待回复/)).toBeInTheDocument()
  })
})
