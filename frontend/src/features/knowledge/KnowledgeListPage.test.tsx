import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { ConfigProvider } from 'antd'
import { antdTheme } from '@/lib/antd-theme'
import KnowledgeListPage from './KnowledgeListPage'
import { setAuthRole } from '@/test/auth-store-mock'

// vi.mock 工厂会被提升到 import 之前执行，不能引用静态 import；用 async 工厂动态 import helper。
vi.mock('@/stores/auth', async () => {
  const { createAuthStoreMock, getAuthUser } = await import('@/test/auth-store-mock')
  const mock = createAuthStoreMock()
  Object.assign(mock.useAuthStore, { getState: () => ({ user: getAuthUser() }), subscribe: () => () => undefined })
  return mock
})

const h = vi.hoisted(() => ({
  datasets: [] as Record<string, unknown>[],
  total: 0,
  error: false,
  loading: false,
  origin: { id: '1', token: 'token-a', role: 'admin' },
  refetchMock: vi.fn(),
  listMock: vi.fn(),
  createMock: vi.fn(),
  deleteMock: vi.fn()
}))

vi.mock('@/queries/useKnowledge', () => ({
  useKnowledgeList: (params: unknown) => { h.listMock(params); return ({ data: { datasets: h.datasets, total: h.total }, isLoading: h.loading, isError: h.error, origin: h.origin, refetch: h.refetchMock }); },
  useDeleteKnowledgeSelection: () => ({ mutateAsync: h.deleteMock, isPending: false }),
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
    localStorage.setItem('access_token', 'token-a')
    h.datasets = []
    h.total = 0
    h.error = false
    h.loading = false
    h.refetchMock.mockReset()
    h.listMock.mockReset()
    h.createMock.mockReset()
    h.deleteMock.mockReset().mockImplementation(async ({ ids }) => ({ deletedIds: ids, failed: [] }))
    setAuthRole('admin')
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

  it('only offers stable update-time ordering and preserves server row order', async () => {
    h.datasets = [sampleDatasets[1], sampleDatasets[0]];
    renderPage();
    const user = userEvent.setup();
    await user.click(screen.getByRole('combobox', { name: '排序' }));
    expect(screen.queryByText('名称排序')).not.toBeInTheDocument();
    await user.click(screen.getByText('最早更新'));
    expect(h.listMock).toHaveBeenLastCalledWith(expect.objectContaining({ orderby: 'update_time', desc: false }));
    const links = screen.getAllByRole('link').filter((link) => link.getAttribute('href')?.startsWith('/knowledge/'));
    expect(links.map((link) => link.textContent)).toEqual(['客服知识库', '产品知识库']);
  });

  it('shows an empty state when there are no datasets', () => {
    h.datasets = []
    h.total = 0
    renderPage()
    expect(screen.getByText('还没有知识库，点击右上角新建')).toBeInTheDocument()
  })

  it('distinguishes failures from an empty list and retries', async () => {
    h.error = true;
    renderPage();
    expect(screen.getByText('知识库列表加载失败')).toBeInTheDocument();
    expect(screen.queryByText('还没有知识库，点击右上角新建')).not.toBeInTheDocument();
    await userEvent.setup().click(screen.getByRole('button', { name: /重\s*试/ }));
    expect(h.refetchMock).toHaveBeenCalledTimes(1);
  });

  it('shows loading without an empty-state claim', () => {
    h.loading = true;
    renderPage();
    expect(screen.getByRole('status', { name: '加载中...' })).toBeInTheDocument();
    expect(screen.queryByText('还没有知识库，点击右上角新建')).not.toBeInTheDocument();
  });

  it('keeps the delete confirmation pending until the request finishes', async () => {
    const user = userEvent.setup()
    let finishDelete!: () => void
    h.deleteMock.mockImplementation(() => new Promise((resolve) => {
      finishDelete = () => { resolve({ deletedIds: ['kb1'], failed: [] }) }
    }))
    h.datasets = [sampleDatasets[0]]
    h.total = 1
    renderPage()

    await user.click(screen.getByTitle('删除'))
    const confirmation = await screen.findByText('确认删除？')
    const popconfirm = confirmation.closest('.ant-popconfirm')
    expect(popconfirm).not.toBeNull()
    const confirmButton = within(popconfirm as HTMLElement).getByRole('button', { name: /删\s*除/ })

    await user.click(confirmButton)

    expect(h.deleteMock).toHaveBeenCalledTimes(1)
    expect(h.deleteMock).toHaveBeenCalledWith(expect.objectContaining({ ids: ['kb1'] }))
    await waitFor(() => { expect(confirmButton).toHaveClass('ant-btn-loading') })
    await user.click(confirmButton)
    expect(h.deleteMock).toHaveBeenCalledTimes(1)

    finishDelete()
    await waitFor(() => { expect(screen.getByText('确认删除？')).not.toBeVisible() })
  })

  it('deletes only the explicit selection and retains a failed selected library', async () => {
    h.datasets = sampleDatasets
    h.total = 20
    h.deleteMock.mockResolvedValue({ deletedIds: ['kb1'], failed: [{ id: 'kb2', error: new Error('Bound library') }] })
    renderPage()
    const user = userEvent.setup()
    expect(screen.queryByRole('button', { name: '删除所选知识库' })).not.toBeInTheDocument()
    await user.click(screen.getByRole('checkbox', { name: '选择知识库 产品知识库' }))
    await user.click(screen.getByRole('checkbox', { name: '选择知识库 客服知识库' }))
    await user.click(screen.getByRole('button', { name: '删除所选知识库' }))
    const confirmation = await screen.findByRole('dialog', { name: '删除所选 2 个知识库？' })
    expect(within(confirmation).getByText('产品知识库')).toBeInTheDocument()
    expect(within(confirmation).getByText('客服知识库')).toBeInTheDocument()
    await user.click(within(confirmation).getByRole('button', { name: /删\s*除/ }))
    await screen.findByText('部分删除结果未确认')
    expect(h.deleteMock).toHaveBeenCalledWith(expect.objectContaining({ ids: ['kb1', 'kb2'] }))
    expect(screen.getByRole('checkbox', { name: '选择知识库 产品知识库' })).not.toBeChecked()
    expect(screen.getByRole('checkbox', { name: '选择知识库 客服知识库' })).toBeChecked()
    expect(screen.getByText(/Bound library/)).toBeInTheDocument()
  })

  it('clears selection when sort, search or page changes', async () => {
    h.datasets = sampleDatasets
    h.total = 20
    renderPage()
    const user = userEvent.setup()
    const select = () => user.click(screen.getByRole('checkbox', { name: '选择知识库 产品知识库' }))
    const cleared = () => expect(screen.queryByRole('button', { name: '删除所选知识库' })).not.toBeInTheDocument()
    await select()
    await user.click(screen.getByRole('combobox', { name: '排序' }))
    await user.click(screen.getByText('最早更新'))
    cleared()
    await select()
    await user.type(screen.getByPlaceholderText('搜索知识库名称'), '客服{Enter}')
    cleared()
    await select()
    await user.click(screen.getByTitle('2'))
    cleared()
    expect(h.deleteMock).not.toHaveBeenCalled()
  })

  it.each(['before dialog', 'after dialog'])('blocks old selections when token changes %s before userinfo catches up', async (phase) => {
    h.datasets = sampleDatasets
    h.total = 2
    renderPage()
    const user = userEvent.setup()
    await user.click(screen.getByRole('checkbox', { name: '选择知识库 产品知识库' }))
    if (phase === 'before dialog') localStorage.setItem('access_token', 'token-b')
    await user.click(screen.getByRole('button', { name: '删除所选知识库' }))
    if (phase === 'after dialog') {
      const dialog = await screen.findByRole('dialog', { name: '删除所选 1 个知识库？' })
      localStorage.setItem('access_token', 'token-b')
      await user.click(within(dialog).getByRole('button', { name: /删\s*除/ }))
    }
    await screen.findByText(/账号或登录凭据已变化/)
    expect(h.deleteMock).not.toHaveBeenCalled()
    expect(screen.getByRole('checkbox', { name: '选择知识库 产品知识库' })).not.toBeChecked()
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  })

  it('does not relabel the old displayed list when token changes before the first selection', async () => {
    h.datasets = sampleDatasets
    h.total = 2
    renderPage()
    localStorage.setItem('access_token', 'token-b')
    await userEvent.setup().click(screen.getByRole('checkbox', { name: '选择知识库 产品知识库' }))
    await screen.findByText(/账号或登录凭据已变化/)
    expect(screen.getByRole('checkbox', { name: '选择知识库 产品知识库' })).not.toBeChecked()
    expect(screen.queryByRole('button', { name: '删除所选知识库' })).not.toBeInTheDocument()
    expect(h.deleteMock).not.toHaveBeenCalled()
  })

  it('blocks single-row deletion from an old list even after an earlier valid selection', async () => {
    h.datasets = sampleDatasets
    h.total = 2
    renderPage()
    const user = userEvent.setup()
    await user.click(screen.getByRole('checkbox', { name: '选择知识库 产品知识库' }))
    localStorage.setItem('access_token', 'token-b')
    await user.click(screen.getAllByTitle('删除')[0])
    const confirmation = (await screen.findByText('确认删除？')).closest('.ant-popconfirm') as HTMLElement
    await user.click(within(confirmation).getByRole('button', { name: /删\s*除/ }))
    await screen.findByText(/账号或登录凭据已变化/)
    expect(h.deleteMock).not.toHaveBeenCalled()
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

  it('member: hides create/edit/delete actions but still sees datasets', () => {
    h.datasets = sampleDatasets
    h.total = sampleDatasets.length
    setAuthRole('member')
    renderPage()

    // 数据仍可见（只读）
    expect(screen.getByText('产品知识库')).toBeInTheDocument()
    expect(screen.getByText('客服知识库')).toBeInTheDocument()
    // 写操作按钮隐藏
    expect(screen.queryByRole('button', { name: /新建知识库/ })).not.toBeInTheDocument()
    expect(screen.queryAllByTitle('编辑')).toHaveLength(0)
    expect(screen.queryAllByTitle('删除')).toHaveLength(0)
    expect(screen.queryByRole('checkbox')).not.toBeInTheDocument()
  })
})
