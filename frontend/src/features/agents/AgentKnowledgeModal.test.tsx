import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import AgentKnowledgeModal from './AgentKnowledgeModal'
import type { Agent } from '@/api/agents'

const h = {
  boundIds: ['kb-zombie-1234567890', 'kb-live-1'],
  datasets: [{ id: 'kb-live-1', name: '存活知识库', description: '' }],
  updateMock: vi.fn()
}

vi.mock('@/queries/useAgents', () => ({
  useAgentKnowledgeDatasets: () => ({ data: h.boundIds, isLoading: false }),
  useUpdateAgentKnowledgeDatasets: () => ({ mutateAsync: h.updateMock, isPending: false })
}))

vi.mock('@/queries/useKnowledge', () => ({
  useKnowledgeList: () => ({ data: { datasets: h.datasets, total: h.datasets.length }, isLoading: false })
}))

describe('AgentKnowledgeModal ghost items (issue #122)', () => {
  it('renders ghost items for bound datasets missing from the live list', () => {
    render(
      <AgentKnowledgeModal
        open
        agent={{ name: 'pharma', config: {} } as unknown as Agent}
        canWrite
        onClose={vi.fn()}
      />
    )
    expect(screen.getByText('已删除的知识库（kb-zombi…）')).toBeInTheDocument()
    expect(screen.getByText('存活知识库')).toBeInTheDocument()
  })
})
