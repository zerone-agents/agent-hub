import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import AgentKnowledgeModal from './AgentKnowledgeModal'
import type { Agent } from '@/api/agents'

const mockKnowledgeState = {
  boundIds: ['kb-zombie-1234567890', 'kb-live-1'],
  datasets: [{ id: 'kb-live-1', name: '存活知识库', description: '' }],
  total: 1,
  listError: null as Error | null,
  updateMock: vi.fn()
}

vi.mock('@/queries/useAgents', () => ({
  useAgentKnowledgeDatasets: () => ({ data: mockKnowledgeState.boundIds, isLoading: false }),
  useUpdateAgentKnowledgeDatasets: () => ({ mutateAsync: mockKnowledgeState.updateMock, isPending: false })
}))

vi.mock('@/queries/useKnowledge', () => ({
  useKnowledgeListAll: () => ({
    data: mockKnowledgeState.listError
      ? undefined
      : { datasets: mockKnowledgeState.datasets, total: mockKnowledgeState.total },
    error: mockKnowledgeState.listError,
    isLoading: false
  })
}))

function renderModal() {
  render(
    <AgentKnowledgeModal
      open
      agent={{ name: 'pharma', config: {} } as unknown as Agent}
      canWrite
      onClose={vi.fn()}
    />
  )
}

describe('AgentKnowledgeModal ghost items (issue #122)', () => {
  beforeEach(() => {
    // 默认：列表成功且完整（fetched ≥ total）。
    mockKnowledgeState.total = 1
    mockKnowledgeState.listError = null
  })

  it('renders ghost items for bound datasets missing from a complete list', () => {
    renderModal()
    expect(screen.getByText('已删除的知识库（kb-zombi…）')).toBeInTheDocument()
    expect(screen.getByText('存活知识库')).toBeInTheDocument()
  })

  it('renders no ghost when the list request failed (liveness unknown)', () => {
    mockKnowledgeState.listError = new Error('boom')
    renderModal()
    expect(screen.queryByText('已删除的知识库（kb-zombi…）')).not.toBeInTheDocument()
  })

  it('renders no ghost when the catalog is incomplete (total exceeds fetched)', () => {
    mockKnowledgeState.total = 5
    renderModal()
    expect(screen.queryByText('已删除的知识库（kb-zombi…）')).not.toBeInTheDocument()
    expect(screen.getByText('存活知识库')).toBeInTheDocument()
  })
})
