import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ConfigProvider } from 'antd'
import { MemoryRouter } from 'react-router'
import type { AgentRelation } from '@/api/agent-relations'
import { antdTheme } from '@/lib/antd-theme'
import { setAuthRole } from '@/test/auth-store-mock'
import RelationListPage from './RelationListPage'

vi.mock('@/stores/auth', async () => (await import('@/test/auth-store-mock')).createAuthStoreMock())

const relations: AgentRelation[] = [
  {
    id: 9,
    scope: 'speeding-hq',
    sourceAgentId: 1,
    sourceAgentName: 'chief-of-staff',
    targetAgentId: 2,
    targetAgentName: 'legal-counsel',
    relationType: 'reviewer',
    stance: 'wary',
    relationshipScore: -30,
    allowedActions: ['submit', 'review', 'challenge'],
    contextPolicy: 'summary_only',
    deliveryPolicy: 'async',
    constraint: '公开声明前必须复核',
    enabled: true,
    createdAt: '2026-09-09T10:00:00Z',
    updatedAt: '2026-09-09T10:00:00Z',
  },
]

vi.mock('@/queries/useAgentRelations', () => ({
  useAgentRelations: () => ({
    data: relations,
    isLoading: false,
    isError: false,
    refetch: vi.fn(),
  }),
  useDeleteAgentRelation: () => ({ mutate: vi.fn() }),
  useCreateAgentRelation: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUpdateAgentRelation: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useAgentRelationEvents: () => ({ data: [], isLoading: false }),
  useRecordAgentRelationEvent: () => ({
    mutateAsync: vi.fn(),
    isPending: false,
  }),
}))

vi.mock('@/queries/useRelationTypes', () => ({
  useRelationTypes: () => ({ data: [], isLoading: false }),
  useCreateRelationType: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUpdateRelationType: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useDeleteRelationType: () => ({ mutate: vi.fn() }),
}))

vi.mock('@/queries/useAgents', () => ({
  useAgents: () => ({
    data: [
      { id: 1, name: 'chief-of-staff', config: { title: { zh: '幕僚长' } } },
      { id: 2, name: 'legal-counsel', config: { title: { zh: '法务总监' } } },
    ],
    isLoading: false,
  }),
}))

function renderPage() {
  render(
    <ConfigProvider theme={antdTheme}>
      <MemoryRouter>
        <RelationListPage />
      </MemoryRouter>
    </ConfigProvider>,
  )
}

describe('RelationListPage', () => {
  beforeEach(() => {
    setAuthRole('admin')
  })

  it('renders directed relation semantics and actions', () => {
    renderPage()
    expect(screen.getByText('组织关系')).toBeInTheDocument()
    expect(screen.getByText('幕僚长')).toBeInTheDocument()
    expect(screen.getByText('法务总监')).toBeInTheDocument()
    expect(screen.getByText('复核关系')).toBeInTheDocument()
    expect(screen.getByText('戒备')).toBeInTheDocument()
    expect(screen.getByText('-30')).toBeInTheDocument()
    expect(screen.getByText('挑战')).toBeInTheDocument()
    expect(screen.getByText('新建关系')).toBeInTheDocument()
  })

  it('keeps member access read-only', () => {
    setAuthRole('member')
    renderPage()
    expect(screen.getByText('幕僚长')).toBeInTheDocument()
    expect(screen.queryByText('新建关系')).not.toBeInTheDocument()
    expect(screen.queryByTitle('编辑')).not.toBeInTheDocument()
    expect(screen.queryByTitle('删除')).not.toBeInTheDocument()
    expect(screen.getByTitle('关系动态')).toBeInTheDocument()
  })
})
