import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ConfigProvider } from 'antd'
import { MemoryRouter } from 'react-router'
import { AxiosError } from 'axios'
import {
  memoryApi,
  type MemoryContext,
  type MemoryFact,
  type MemoryOperation,
  type MemoryScope
} from '@/api/memory'
import { antdTheme } from '@/lib/antd-theme'
import MemoryManagementPage from './MemoryManagementPage'

const authState = vi.hoisted(() => ({
  user: { id: 'admin-a', role: 'admin' }
}))
vi.mock('@/stores/auth', () => ({
  useAuthStore: (selector: (state: typeof authState) => unknown) =>
    selector(authState)
}))
vi.mock('@/api/memory', async (original) => ({
  ...(await original<typeof import('@/api/memory')>()),
  memoryApi: {
    applications: vi.fn(),
    applicationModels: vi.fn(),
    createApplication: vi.fn(),
    applicationProvisioning: vi.fn(),
    nativeMessages: vi.fn(),
    nativeContext: vi.fn(),
    nativeConfig: vi.fn(),
    nativeSearch: vi.fn(),
    setNativeStatus: vi.fn(),
    subjects: vi.fn(),
    facts: vi.fn(),
    update: vi.fn(),
    delete: vi.fn(),
    revoke: vi.fn(),
    context: vi.fn(),
    operation: vi.fn()
  }
}))

const scope: MemoryScope = {
  tenantId: 'tenant-a',
  applicationId: 'app-a',
  userId: 'user-a',
  accountLifecycleId: 'lifecycle-a'
}
const fact: MemoryFact = {
  factId: 'fact-a',
  version: 7,
  content: '我喜欢无糖乌龙茶。',
  subject: 'asQuoted',
  temporal: 'asQuoted',
  category: 'preference',
  status: 'active',
  syncStatus: 'confirmed',
  consentRef: 'consent-a',
  updatedAt: '2026-11-06T10:00:00Z'
}
const pendingOperation: MemoryOperation = {
  operationId: 'receipt-a',
  status: 'pending',
  stoppedSupply: true,
  physicalCleanup: false,
  cleanupScope: 'retrievalReplica',
  ledgerRetained: true
}
const clients: QueryClient[] = []

it('offers page creation when no applications are configured', async () => {
  vi.mocked(memoryApi.applications).mockResolvedValue([])
  vi.mocked(memoryApi.applicationModels).mockResolvedValue({
    chat: [],
    embedding: []
  })
  renderPage()
  const create = await screen.findByRole('button', { name: '创建记忆应用' })
  await waitFor(() => {
    expect(create).toBeEnabled()
  })
  fireEvent.click(create)
  expect(await screen.findByLabelText('应用名称')).toBeInTheDocument()
  expect(memoryApi.applicationModels).toHaveBeenCalledOnce()
  expect(memoryApi.createApplication).not.toHaveBeenCalled()
})

it('shows unfinished provisioning without querying user memories', async () => {
  vi.mocked(memoryApi.applications).mockResolvedValue([
    {
      tenantId: 'default',
      applicationId: 'pending-app',
      policyVersion: 'synthetic-v1',
      dataClass: 'synthetic',
      status: 'unknown',
      origin: 'managed'
    }
  ])
  renderPage()
  const alert = await screen.findByRole('alert')
  expect(within(alert).getByText('创建结果待确认')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: '核对创建结果' })).toBeEnabled()
  expect(memoryApi.subjects).not.toHaveBeenCalled()
  expect(memoryApi.facts).not.toHaveBeenCalled()
  expect(memoryApi.nativeMessages).not.toHaveBeenCalled()
})

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((done) => {
    resolve = done
  })
  return { promise, resolve }
}

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } }
  })
  clients.push(client)
  const page = (
    <ConfigProvider theme={antdTheme}>
      <QueryClientProvider client={client}>
        <MemoryRouter>
          <MemoryManagementPage />
        </MemoryRouter>
      </QueryClientProvider>
    </ConfigProvider>
  )
  return { ...render(page), page, client }
}

beforeEach(() => {
  vi.resetAllMocks()
  authState.user = { id: 'admin-a', role: 'admin' }
  vi.mocked(memoryApi.applications).mockResolvedValue([
    {
      tenantId: scope.tenantId,
      applicationId: scope.applicationId,
      policyVersion: 'synthetic-v1',
      dataClass: 'synthetic',
      backend: 'multirag-knowledge',
      correctionSupported: true,
      forgetSupported: true,
      physicalDeleteSupported: true,
      consentRevokeSupported: true,
      deactivateSupported: true
    }
  ])
  vi.mocked(memoryApi.subjects).mockResolvedValue({
    subjects: [{ scope, state: 'active', revision: 8 }],
    missingScopeCount: 0,
    limited: false,
    limit: 1000
  })
  vi.mocked(memoryApi.facts).mockResolvedValue([fact])
  vi.mocked(memoryApi.update).mockResolvedValue({
    ...fact,
    version: 8,
    content: '我喜欢热咖啡。'
  })
  vi.mocked(memoryApi.delete).mockResolvedValue(pendingOperation)
  vi.mocked(memoryApi.revoke).mockResolvedValue(pendingOperation)
  vi.mocked(memoryApi.operation).mockResolvedValue(pendingOperation)
  vi.mocked(memoryApi.context).mockResolvedValue({
    status: 'normal',
    contextVersion: 8,
    facts: [fact]
  })
})

describe('native memory capabilities', () => {
  it.each([
    ['memory_disabled', '记忆服务未启用，请联系管理员启用后重试'],
    ['memory_not_configured', '记忆服务尚未配置，请联系管理员完成服务接入']
  ])(
    'shows %s without retrying or requesting account records',
    async (code, error) => {
      vi.mocked(memoryApi.applications).mockRejectedValue({
        isAxiosError: true,
        response: {
          status: 503,
          data: {
            code,
            error
          }
        }
      })
      renderPage()
      await screen.findByText(error)
      expect(memoryApi.applications).toHaveBeenCalledTimes(1)
      expect(memoryApi.subjects).not.toHaveBeenCalled()
    }
  )
  it('defaults to the automatic application while preserving explicit application choices', async () => {
    const oldApp = {
      tenantId: scope.tenantId,
      applicationId: 'old-original',
      policyVersion: 'synthetic-v1',
      dataClass: 'synthetic',
      backend: 'multirag-memory' as const,
      rawOnly: true
    }
    const autoApp = {
      ...oldApp,
      applicationId: 'current-auto',
      automaticExtractionSupported: true,
      rawOnly: false
    }
    vi.mocked(memoryApi.applications).mockResolvedValue([oldApp, autoApp])
    vi.mocked(memoryApi.subjects).mockResolvedValue({
      subjects: [],
      missingScopeCount: 0,
      limited: false,
      limit: 1000
    })
    const view = renderPage()
    await waitFor(() => {
      expect(memoryApi.subjects).toHaveBeenCalledWith(
        { tenantId: scope.tenantId, applicationId: 'current-auto' },
        expect.any(AbortSignal)
      )
    })
    const selector = screen.getByRole('combobox', { name: '应用' })
    fireEvent.mouseDown(selector)
    fireEvent.click(
      screen.getByText('old-original · tenant-a', {
        selector: '.ant-select-item-option-content'
      })
    )
    await waitFor(() => {
      expect(memoryApi.subjects).toHaveBeenCalledWith(
        { tenantId: scope.tenantId, applicationId: 'old-original' },
        expect.any(AbortSignal)
      )
    })
    view.rerender(view.page)
    expect(
      screen.getByText('old-original · tenant-a', {
        selector: '.ant-select-content'
      })
    ).toBeInTheDocument()
  })

  it('supports correction while keeping deletion and consent cleanup unavailable', async () => {
    vi.mocked(memoryApi.applications).mockResolvedValue([
      {
        tenantId: scope.tenantId,
        applicationId: scope.applicationId,
        policyVersion: 'synthetic-v1',
        dataClass: 'synthetic',
        backend: 'multirag-memory',
        rawOnly: true,
        correctionSupported: true,
        forgetSupported: false,
        physicalDeleteSupported: false,
        consentRevokeSupported: false,
        deactivateSupported: false
      }
    ])
    renderPage()
    await screen.findByText(fact.content)
    expect(screen.getByText('已审核事实')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /编\s*辑/ })).toBeEnabled()
    expect(screen.getByRole('button', { name: /删\s*除/ })).toBeDisabled()
    expect(screen.getByRole('button', { name: '撤回授权' })).toBeDisabled()
    fireEvent.click(screen.getByRole('button', { name: /删\s*除/ }))
    expect(memoryApi.delete).not.toHaveBeenCalled()
    expect(memoryApi.revoke).not.toHaveBeenCalled()
  })

  it('does not infer write capabilities when the application has not declared them', async () => {
    vi.mocked(memoryApi.applications).mockResolvedValue([
      {
        tenantId: scope.tenantId,
        applicationId: scope.applicationId,
        policyVersion: 'synthetic-v1',
        dataClass: 'synthetic'
      }
    ])
    renderPage()
    await screen.findByText(fact.content)
    expect(screen.getByRole('button', { name: /编\s*辑/ })).toBeDisabled()
    expect(screen.getByRole('button', { name: /删\s*除/ })).toBeDisabled()
  })
})

afterEach(() => {
  clients.splice(0).forEach((client) => {
    client.clear()
  })
})

describe('memory manager permissions and identity', () => {
  it.each(['member', 'guest', ''])(
    'blocks %s without fetching memory identities',
    (role) => {
      authState.user.role = role
      renderPage()
      expect(
        screen.getByText('仅管理员和维护者可访问记忆管理')
      ).toBeInTheDocument()
      expect(memoryApi.applications).not.toHaveBeenCalled()
    }
  )

  it.each(['admin', 'maintainer'])(
    'shows actual facts and CAS versions to %s',
    async (role) => {
      authState.user.role = role
      renderPage()
      expect(await screen.findByText(fact.content)).toBeInTheDocument()
      expect(screen.getByText('v7')).toBeInTheDocument()
      expect(screen.getByText('已同步')).toBeInTheDocument()
      expect(
        screen.getByRole('button', { name: '选择账户' })
      ).toBeInTheDocument()
      expect(memoryApi.subjects).toHaveBeenCalledWith(
        { tenantId: 'tenant-a', applicationId: 'app-a' },
        expect.any(AbortSignal)
      )
      expect(memoryApi.facts).toHaveBeenCalledWith(
        scope,
        expect.any(AbortSignal)
      )
    }
  )

  it('shows an empty configuration without fabricated applications or facts', async () => {
    vi.mocked(memoryApi.applications).mockResolvedValue([])
    renderPage()
    expect(await screen.findByText('尚未配置记忆应用')).toBeInTheDocument()
    expect(memoryApi.subjects).not.toHaveBeenCalled()
    expect(memoryApi.facts).not.toHaveBeenCalled()
  })

  it('explains undisplayable older identities and a limited subject list', async () => {
    vi.mocked(memoryApi.subjects).mockResolvedValue({
      subjects: [],
      missingScopeCount: 3,
      limited: true,
      limit: 1000
    })
    renderPage()
    expect(
      await screen.findByText('该应用暂无可展示的用户账户')
    ).toBeInTheDocument()
    expect(
      screen.getByText(/3 个旧账户缺少可展示的用户身份/)
    ).toBeInTheDocument()
    expect(screen.getByText(/当前最多显示 1000 个账户/)).toBeInTheDocument()
    expect(memoryApi.facts).not.toHaveBeenCalled()
  })

  it('does not read or manage facts for a frozen lifecycle', async () => {
    vi.mocked(memoryApi.subjects).mockResolvedValue({
      subjects: [{ scope, state: 'frozen', revision: 8 }],
      missingScopeCount: 0,
      limited: false,
      limit: 1000
    })
    renderPage()
    expect(
      await screen.findByText('该账户生命周期已停止供给记忆')
    ).toBeInTheDocument()
    expect(memoryApi.facts).not.toHaveBeenCalled()
    expect(
      screen.queryByRole('button', { name: '检索记忆' })
    ).not.toBeInTheDocument()
  })

  it('does not reuse the previous authenticated workspace cache after an account switch', async () => {
    const rendered = renderPage()
    expect(await screen.findByText(fact.content)).toBeInTheDocument()
    const loading =
      deferred<Awaited<ReturnType<typeof memoryApi.applications>>>()
    vi.mocked(memoryApi.applications).mockReturnValue(loading.promise)
    authState.user = { id: 'admin-b', role: 'admin' }
    // Give React a new element so the mocked external auth selection is read.
    rendered.rerender(
      <ConfigProvider theme={antdTheme}>
        <QueryClientProvider client={rendered.client}>
          <MemoryRouter>
            <MemoryManagementPage />
          </MemoryRouter>
        </QueryClientProvider>
      </ConfigProvider>
    )
    expect(screen.queryByText(fact.content)).not.toBeInTheDocument()
    await waitFor(() => {
      expect(memoryApi.applications).toHaveBeenCalledTimes(2)
    })
  })
})

describe('memory changes and cleanup receipts', () => {
  it('counts Unicode characters and sends the displayed version and scope when editing', async () => {
    renderPage()
    fireEvent.click(await screen.findByRole('button', { name: /编\s*辑/ }))
    const dialog = screen.getByRole('dialog')
    const input = within(dialog).getByRole('textbox', { name: '记忆内容' })
    fireEvent.change(input, { target: { value: '🙂'.repeat(201) } })
    expect(
      within(dialog).getByRole('button', { name: '保存修改' })
    ).toBeDisabled()
    fireEvent.change(input, { target: { value: '🙂'.repeat(200) } })
    expect(
      within(dialog).getByRole('button', { name: '保存修改' })
    ).toBeEnabled()
    fireEvent.click(within(dialog).getByRole('button', { name: '保存修改' }))
    await waitFor(() => {
      expect(memoryApi.update).toHaveBeenCalledWith('fact-a', {
        scope,
        expectedVersion: 7,
        content: '🙂'.repeat(200),
        operationId: expect.stringMatching(/^admin-memory-/)
      })
    })
  })

  it('keeps the same idempotency key on an unchanged failed request and displays the error', async () => {
    vi.mocked(memoryApi.update).mockRejectedValue(
      new Error('版本冲突，请刷新后重试')
    )
    renderPage()
    fireEvent.click(await screen.findByRole('button', { name: /编\s*辑/ }))
    const dialog = screen.getByRole('dialog')
    fireEvent.change(
      within(dialog).getByRole('textbox', { name: '记忆内容' }),
      { target: { value: '我喜欢热咖啡。' } }
    )
    fireEvent.click(within(dialog).getByRole('button', { name: '保存修改' }))
    expect(
      await screen.findByText('版本冲突，请刷新后重试')
    ).toBeInTheDocument()
    fireEvent.click(within(dialog).getByRole('button', { name: '保存修改' }))
    await waitFor(() => {
      expect(memoryApi.update).toHaveBeenCalledTimes(2)
    })
    expect(vi.mocked(memoryApi.update).mock.calls[0][1]).toEqual(
      vi.mocked(memoryApi.update).mock.calls[1][1]
    )
    expect(screen.getByRole('dialog')).toBeInTheDocument()
  })

  it('confirms deletion, follows the receipt and distinguishes retained evidence from retrieval cleanup', async () => {
    const readback = deferred<MemoryOperation>()
    vi.mocked(memoryApi.operation).mockReturnValue(readback.promise)
    renderPage()
    fireEvent.click(await screen.findByRole('button', { name: /删\s*除/ }))
    expect(memoryApi.delete).not.toHaveBeenCalled()
    fireEvent.click(
      within(screen.getByRole('dialog')).getByRole('button', {
        name: /删\s*除/
      })
    )
    expect(await screen.findByText('清理待完成')).toBeInTheDocument()
    expect(
      screen.getByText('事实与证据记录仍保留，按保留策略管理。')
    ).toBeInTheDocument()
    expect(memoryApi.delete).toHaveBeenCalledWith('fact-a', {
      scope,
      expectedVersion: 7,
      operationId: expect.stringMatching(/^admin-memory-/)
    })
    await act(async () => {
      readback.resolve({
        ...pendingOperation,
        status: 'completed',
        physicalCleanup: true
      })
    })
    expect(await screen.findByText('清理已完成')).toBeInTheDocument()
    expect(
      screen.queryByText('本服务的事实与证据记录已清理。')
    ).not.toBeInTheDocument()
    expect(memoryApi.operation).toHaveBeenCalledWith(
      scope,
      'receipt-a',
      expect.any(AbortSignal)
    )
  })

  it('withdraws the exact displayed consent reference only after confirmation', async () => {
    renderPage()
    fireEvent.click(await screen.findByRole('button', { name: '撤回授权' }))
    expect(memoryApi.revoke).not.toHaveBeenCalled()
    const dialog = screen.getByRole('dialog')
    expect(within(dialog).getByText('consent-a')).toBeInTheDocument()
    fireEvent.click(within(dialog).getByRole('button', { name: '撤回授权' }))
    await waitFor(() => {
      expect(memoryApi.revoke).toHaveBeenCalledWith({
        scope,
        consentRef: 'consent-a',
        operationId: expect.stringMatching(/^admin-memory-/)
      })
    })
  })

  it('recovers a lost deletion response by reading the original operation without claiming success', async () => {
    vi.mocked(memoryApi.delete).mockRejectedValue(
      new AxiosError('Network Error', 'ERR_NETWORK')
    )
    const readback = deferred<MemoryOperation>()
    vi.mocked(memoryApi.operation).mockReturnValue(readback.promise)
    renderPage()
    fireEvent.click(await screen.findByRole('button', { name: /删\s*除/ }))
    fireEvent.click(
      within(screen.getByRole('dialog')).getByRole('button', {
        name: /删\s*除/
      })
    )
    expect(await screen.findByText(/尚未确认操作结果/)).toBeInTheDocument()
    expect(screen.queryByText('已停止供给')).not.toBeInTheDocument()
    expect(screen.queryByText('清理已完成')).not.toBeInTheDocument()
    const operationId = vi.mocked(memoryApi.delete).mock.calls[0][1].operationId
    await waitFor(() => {
      expect(memoryApi.operation).toHaveBeenCalledWith(
        scope,
        operationId,
        expect.any(AbortSignal)
      )
    })
    await act(async () => {
      readback.resolve({ ...pendingOperation, operationId })
    })
    expect(await screen.findByText('已停止供给')).toBeInTheDocument()
    expect(memoryApi.delete).toHaveBeenCalledTimes(1)
  })
})

describe('scoped retrieval', () => {
  it('renders server retrieval results rather than generating a chat response', async () => {
    renderPage()
    await screen.findByText(fact.content)
    fireEvent.change(screen.getByRole('textbox', { name: '检索验证' }), {
      target: { value: '这位用户喜欢什么？' }
    })
    fireEvent.click(screen.getByRole('button', { name: '检索记忆' }))
    const region = screen.getByRole('region', { name: '检索验证' })
    expect(await within(region).findByText('上下文版本 8')).toBeInTheDocument()
    expect(
      within(region).getByText(fact.content, { exact: false })
    ).toBeInTheDocument()
    expect(memoryApi.context).toHaveBeenCalledWith({
      scope,
      query: '这位用户喜欢什么？'
    })
  })

  it('ignores a retrieval response started before a deletion', async () => {
    const oldRecall = deferred<MemoryContext>()
    vi.mocked(memoryApi.context).mockReturnValue(oldRecall.promise)
    renderPage()
    await screen.findByText(fact.content)
    fireEvent.change(screen.getByRole('textbox', { name: '检索验证' }), {
      target: { value: '喜欢什么？' }
    })
    fireEvent.click(screen.getByRole('button', { name: '检索记忆' }))
    await waitFor(() => {
      expect(memoryApi.context).toHaveBeenCalledTimes(1)
    })
    fireEvent.click(screen.getByRole('button', { name: /删\s*除/ }))
    vi.mocked(memoryApi.facts).mockResolvedValue([])
    fireEvent.click(
      within(screen.getByRole('dialog')).getByRole('button', {
        name: /删\s*除/
      })
    )
    await screen.findByText('操作回执')
    await act(async () => {
      oldRecall.resolve({
        contextVersion: 8,
        facts: [fact],
        profile: '旧用户概况',
        status: 'normal'
      })
    })
    const region = screen.getByRole('region', { name: '检索验证' })
    expect(within(region).queryByText(fact.content)).not.toBeInTheDocument()
    expect(within(region).queryByText('旧用户概况')).not.toBeInTheDocument()
    expect(within(region).queryByText('上下文版本 8')).not.toBeInTheDocument()
  })

  it('clears local retrieval and dialogs when switching account lifecycles', async () => {
    const otherScope = {
      ...scope,
      userId: 'user-b',
      accountLifecycleId: 'lifecycle-b'
    }
    const oldRecall = deferred<MemoryContext>()
    vi.mocked(memoryApi.context).mockReturnValue(oldRecall.promise)
    vi.mocked(memoryApi.subjects).mockResolvedValue({
      subjects: [
        { scope, state: 'active', revision: 8 },
        { scope: otherScope, state: 'active', revision: 1 }
      ],
      missingScopeCount: 0,
      limited: false,
      limit: 1000
    })
    vi.mocked(memoryApi.facts).mockImplementation(async (selected) =>
      selected.userId === 'user-b' ? [] : [fact]
    )
    renderPage()
    await screen.findByText(fact.content)
    fireEvent.change(screen.getByRole('textbox', { name: '检索验证' }), {
      target: { value: '喜欢什么？' }
    })
    fireEvent.click(screen.getByRole('button', { name: '检索记忆' }))
    fireEvent.click(screen.getByRole('button', { name: /编\s*辑/ }))
    fireEvent.click(screen.getByRole('button', { name: '选择账户' }))
    fireEvent.click(
      screen.getByRole('button', { name: 'user-b · lifecycle-b' })
    )
    expect(await screen.findByText('该账户暂无有效记忆')).toBeInTheDocument()
    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    })
    await act(async () => {
      oldRecall.resolve({ contextVersion: 8, facts: [fact], status: 'normal' })
    })
    expect(screen.queryByText(fact.content)).not.toBeInTheDocument()
    expect(screen.getByRole('textbox', { name: '检索验证' })).toHaveValue('')
    expect(memoryApi.facts).toHaveBeenLastCalledWith(
      otherScope,
      expect.any(AbortSignal)
    )
  })
})

describe('compact native management workspace', () => {
  it('searches accounts on demand and drops pending recall results on account switch', async () => {
    const otherScope = {
      ...scope,
      userId: 'user-b',
      accountLifecycleId: 'lifecycle-b'
    }
    const oldRecall =
      deferred<Awaited<ReturnType<typeof memoryApi.nativeContext>>>()
    vi.mocked(memoryApi.applications).mockResolvedValue([
      {
        tenantId: scope.tenantId,
        applicationId: scope.applicationId,
        policyVersion: 'synthetic-v1',
        dataClass: 'synthetic',
        automaticExtractionSupported: true
      }
    ])
    vi.mocked(memoryApi.subjects).mockResolvedValue({
      subjects: [
        { scope, state: 'active', revision: 1 },
        { scope: otherScope, state: 'active', revision: 1 }
      ],
      missingScopeCount: 0,
      limited: false,
      limit: 1000
    })
    vi.mocked(memoryApi.nativeMessages).mockImplementation(async (selected) =>
      selected.userId === scope.userId
        ? [
            {
              id: '1',
              sourceId: '0',
              sourceSessionId: 'source-a',
              type: 'raw',
              content: '账户A私有记忆',
              status: true,
              createdAt: '',
              nativeStatus: true
            }
          ]
        : []
    )
    vi.mocked(memoryApi.nativeContext).mockReturnValue(oldRecall.promise)
    renderPage()
    await screen.findByText('账户A私有记忆')
    fireEvent.click(screen.getByRole('tab', { name: '召回诊断' }))
    fireEvent.change(screen.getByRole('textbox', { name: '检索原生记忆' }), {
      target: { value: '我的偏好' }
    })
    fireEvent.click(screen.getByRole('button', { name: '检索记忆' }))
    fireEvent.click(screen.getByRole('button', { name: '选择账户' }))
    fireEvent.change(
      screen.getByRole('textbox', { name: '搜索用户或账户生命周期' }),
      { target: { value: 'lifecycle-b' } }
    )
    expect(
      screen.getByRole('button', { name: 'user-b · lifecycle-b' })
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'user-a · lifecycle-a' })
    ).not.toBeInTheDocument()
    fireEvent.click(
      screen.getByRole('button', { name: 'user-b · lifecycle-b' })
    )
    await screen.findByText('暂无记忆，完成聊天后可刷新查看自动提取进度。')
    await act(async () => {
      oldRecall.resolve({
        messages: [
          {
            id: '2',
            sourceId: '1',
            sourceSessionId: 'source-a',
            type: 'semantic',
            content: '旧账户召回结果',
            createdAt: '',
            status: true
          }
        ],
        status: 'ready',
        budgetLimited: false,
        recentSourceIncluded: true,
        currentnessNote: ''
      })
    })
    expect(screen.queryByText('账户A私有记忆')).not.toBeInTheDocument()
    expect(screen.queryByText('旧账户召回结果')).not.toBeInTheDocument()
    expect(memoryApi.nativeMessages).toHaveBeenLastCalledWith(
      otherScope,
      expect.any(AbortSignal)
    )
  })

  it('keeps native configuration behind its single drawer entry', async () => {
    vi.mocked(memoryApi.applications).mockResolvedValue([
      {
        tenantId: scope.tenantId,
        applicationId: scope.applicationId,
        policyVersion: 'synthetic-v1',
        dataClass: 'synthetic',
        automaticExtractionSupported: true,
        nativeConfigurationSupported: true
      }
    ])
    vi.mocked(memoryApi.nativeMessages).mockResolvedValue([])
    vi.mocked(memoryApi.nativeConfig).mockResolvedValue({
      name: '实际安全配置',
      memoryTypes: ['raw'],
      capacityBytes: 5242880,
      chatModel: '356',
      embeddingModel: '337'
    })
    renderPage()
    await screen.findByText('暂无记忆，完成聊天后可刷新查看自动提取进度。')
    expect(memoryApi.nativeConfig).not.toHaveBeenCalled()
    expect(
      screen.queryByRole('tab', { name: '应用配置' })
    ).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '应用配置' }))
    const drawer = await screen.findByRole('dialog', { name: '应用配置' })
    await within(drawer).findByText('实际安全配置')
    expect(memoryApi.nativeConfig).toHaveBeenCalledWith(
      { tenantId: scope.tenantId, applicationId: scope.applicationId },
      expect.any(AbortSignal)
    )
  })
})
