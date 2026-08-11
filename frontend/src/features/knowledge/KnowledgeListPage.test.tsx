import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { ConfigProvider } from 'antd'
import { antdTheme } from '@/lib/antd-theme'
import KnowledgeListPage from './KnowledgeListPage'

const h = vi.hoisted(() => ({
  datasets: [] as Record<string, unknown>[],
  total: 0,
  createMock: vi.fn(),
  deleteMock: vi.fn()
}))

vi.mock('@/queries/useKnowledge', () => ({
  useKnowledgeList: () => ({ data: { datasets: h.datasets, total: h.total }, isLoading: false }),
  useDeleteKnowledge: () => ({ mutate: h.deleteMock }),
  useCreateKnowledge: () => ({ mutateAsync: h.createMock, isPending: false }),
  useUpdateKnowledge: () => ({ mutateAsync: vi.fn(), isPending: false })
}))

vi.mock('@/queries/useProviders', () => ({
  useProviders: () => ({ data: [], isLoading: false }),
  useSyncProviderMultiRAG: () => ({ mutateAsync: vi.fn() })
}))

vi.mock('@/queries/useMultirag', () => ({
  useMultiragModels: () => ({ data: [], isLoading: false })
}))

const sampleDatasets = [
  {
    id: 'kb1',
    name: '产品知识库',
    display_name: '产品知识库',
    collection_name: 'kb_product',
    description: '产品文档',
    permission: 'me',
    doc_num: 3,
    chunk_num: 42,
    parser_id: 'naive',
    embd_id: 'bge-m3',
    parser_config: {}
  },
  {
    id: 'kb2',
    name: '客服知识库',
    display_name: '客服知识库',
    collection_name: 'kb_support',
    description: 'FAQ',
    permission: 'team',
    doc_num: 0,
    chunk_num: 0,
    parser_id: 'qa',
    embd_id: '',
    parser_config: {}
  }
]

function renderPage() {
  return render(
    <ConfigProvider theme={antdTheme}>
      <MemoryRouter>
        <KnowledgeListPage />
      </MemoryRouter>
    </ConfigProvider>
  )
}

describe('KnowledgeListPage', () => {
  beforeEach(() => {
    h.datasets = []
    h.total = 0
    h.createMock.mockReset()
    h.deleteMock.mockReset()
  })

  it('renders the dataset rows', () => {
    h.datasets = sampleDatasets
    h.total = sampleDatasets.length
    renderPage()
    expect(screen.getByText('知识库管理')).toBeInTheDocument()
    expect(screen.getByText('产品知识库')).toBeInTheDocument()
    expect(screen.getByText('客服知识库')).toBeInTheDocument()
    expect(screen.queryByRole('columnheader', { name: '权限' })).not.toBeInTheDocument()
    expect(screen.queryByText('仅自己')).not.toBeInTheDocument()
    expect(screen.queryByText('团队')).not.toBeInTheDocument()
  })

  it('shows an empty state when there are no datasets', () => {
    h.datasets = []
    h.total = 0
    renderPage()
    expect(screen.getByText('还没有知识库，点击右上角新建')).toBeInTheDocument()
  })

  it('submits a new dataset through the create modal', async () => {
    const user = userEvent.setup()
    renderPage()

    await user.click(screen.getByRole('button', { name: /新建知识库/ }))
    const nameInput = await screen.findByPlaceholderText('知识库名称')
    expect(screen.getByText('解析配置')).toBeInTheDocument()
    expect(screen.queryByText('parser_config (JSON)')).not.toBeInTheDocument()
    await user.type(nameInput, 'kb-new')
    await user.click(screen.getByRole('button', { name: /创\s*建/ }))

    await waitFor(() => { expect(h.createMock).toHaveBeenCalled(); })
    expect(h.createMock).toHaveBeenCalledWith(expect.objectContaining({ name: 'kb-new' }))
  })
})
