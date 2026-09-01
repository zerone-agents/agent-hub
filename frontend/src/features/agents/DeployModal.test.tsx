import { describe, it, expect, vi, beforeEach } from 'vitest'
import { StrictMode } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import DeployModal from './DeployModal'
import { ManualCopyHost } from '@/components/ManualCopyDialog'
import { agentApi } from '@/api/agents'
import type { Agent, DeploymentStatus } from '@/api/agents'
import type { Provider } from '@/api/providers'
import { setAuthRole } from '@/test/auth-store-mock'

vi.mock('@/stores/auth', async () => (await import('@/test/auth-store-mock')).createAuthStoreMock())

vi.mock('@/api/agents', () => ({
  agentApi: {
    deploy: vi.fn(),
    getDeployment: vi.fn(),
    stopDeployment: vi.fn(),
    deleteDeployment: vi.fn(),
  },
}))

// 避免在测试环境引入 QueryClientProvider；知识库名称映射对这些用例无影响
vi.mock('@/queries/useKnowledge', () => ({
  useKnowledgeList: () => ({ data: { datasets: [] } }),
}))

const makeAgent = (overrides?: Partial<Agent['config']>): Agent => ({
  id: 1,
  name: 'general',
  config: {
    providerId: 1,
    modelId: 'GLM-5-Turbo',
    ...overrides,
  },
})

const providers: Provider[] = [
  {
    id: 1,
    key: 'openai-compatible',
    name: 'OpenAI Compatible API',
    description: '',
    descriptionEn: '',
    protocol: 'openai',
    authStyle: 'api_key',
    baseUrl: '',
    defaultModels: [],
    fields: [],
    iconKey: '',
    builtin: true,
    lockedApiKey: '',
    attributes: {},
    createdAt: '',
    updatedAt: '',
  },
]

const makeStatus = (overrides: Partial<DeploymentStatus> = {}): DeploymentStatus => ({
  status: 'not_found',
  ...overrides,
})

// The component reads `res.data.data` (axios envelope + backend envelope),
// so mocks must wrap the payload twice.
const mockResponse = <T,>(data: T) => ({ data: { data, success: true } })

beforeEach(() => {
  vi.clearAllMocks()
  setAuthRole('admin')
})

describe('DeployModal', () => {
  it('renders deploy button when not deployed', async () => {
    vi.mocked(agentApi.getDeployment).mockResolvedValue(mockResponse(makeStatus({ status: 'not_found' })) as never)

    render(<DeployModal agent={makeAgent()} providers={providers} open={true} onClose={vi.fn()} />)

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /部署/ })).toBeInTheDocument()
    })
  })

  it('shows warning when agent has no provider/model configured', async () => {
    vi.mocked(agentApi.getDeployment).mockResolvedValue(mockResponse(makeStatus({ status: 'not_found' })) as never)

    render(
      <DeployModal
        agent={makeAgent({ providerId: null, modelId: undefined })}
        providers={providers}
        open={true}
        onClose={vi.fn()}
      />
    )

    await waitFor(() => {
      expect(screen.getByText(/未配置模型/)).toBeInTheDocument()
    })
  })

  it('shows runtime link when running', async () => {
    vi.mocked(agentApi.getDeployment).mockResolvedValue(
      mockResponse(
        makeStatus({
          status: 'running',
          health: 'healthy',
          hostPort: 8080,
          runtimeUrl: 'http://localhost:8080',
        })
      ) as never
    )

    render(<DeployModal agent={makeAgent()} providers={providers} open={true} onClose={vi.fn()} />)

    await waitFor(() => {
      // 状态标签显示"运行中"（状态行也会包含该词，故精确匹配标签文本）
      expect(screen.getByText('运行中')).toBeInTheDocument()
      // Component renders runtimeUrl as plain text inside the API info card
      // (with a copy button), not as a clickable link.
      expect(screen.getByText(/localhost:8080/)).toBeInTheDocument()
      expect(screen.getByText('8080')).toBeInTheDocument()
    })
  })

  it('resolves relative runtimeUrl against current origin for display and copy', async () => {
    const writeText = vi.fn()
    // userEvent.setup() unconditionally installs its own navigator.clipboard stub
    // (attachClipboardStubToView), even with writeToClipboard: false — so the spy
    // must be defined AFTER setup to replace it. defineProperty is required:
    // the installed property is an accessor whose setter swallows plain assignment.
    const user = userEvent.setup({ writeToClipboard: false })
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    // jsdom does not implement window.isSecureContext (undefined), and handleCopy
    // gates the async clipboard API on it — stub it true to exercise the primary
    // copy path instead of the execCommand fallback.
    Object.defineProperty(window, 'isSecureContext', { value: true, configurable: true })
    vi.mocked(agentApi.getDeployment).mockResolvedValue(
      mockResponse(makeStatus({ status: 'running', runtimeUrl: '/runtime/default/test' })) as never
    )
    render(<DeployModal agent={makeAgent()} providers={providers} open={true} onClose={vi.fn()} />)
    const expected = `${window.location.origin}/runtime/default/test`
    expect(await screen.findByText(expected)).toBeInTheDocument() // 展示为绝对 URL
    await user.click(screen.getByTitle('复制 URL'))
    await waitFor(() => { expect(writeText).toHaveBeenCalledWith(expected) }) // 复制也是绝对 URL
  })

  it('非安全上下文（jsdom 默认无 isSecureContext）：复制 URL 弹出手动复制框', async () => {
    const user = userEvent.setup({ writeToClipboard: false })
    // 显式钉死非安全环境：前一个用例 stub 过的 isSecureContext / navigator.clipboard
    // 没有 afterEach 清理，会跨用例残留 → copyOrManual 必须走非安全分支
    Object.defineProperty(window, 'isSecureContext', { value: false, configurable: true })
    delete (navigator as unknown as { clipboard?: unknown }).clipboard
    vi.mocked(agentApi.getDeployment).mockResolvedValue(
      mockResponse(makeStatus({ status: 'running', runtimeUrl: '/runtime/default/test' })) as never
    )
    render(<><ManualCopyHost /><DeployModal agent={makeAgent()} providers={providers} open={true} onClose={vi.fn()} /></>)
    const expected = `${window.location.origin}/runtime/default/test`
    expect(await screen.findByText(expected)).toBeInTheDocument()

    await user.click(screen.getByTitle('复制 URL'))

    // 手动复制框（antd Modal，标题即无障碍名，与底层「部署 Agent」弹窗区分）
    const dialog = await screen.findByRole('dialog', { name: '手动复制' })
    expect(dialog.querySelector('textarea')?.value).toBe(expected)
    // P1 回归锁：非安全上下文只是「手动复制」，不得误报「已复制」
    expect(screen.queryByText('已复制')).toBeNull()
  })

  it('renders absolute runtimeUrl (Kong mode) unchanged', async () => {
    vi.mocked(agentApi.getDeployment).mockResolvedValue(
      mockResponse(makeStatus({ status: 'running', runtimeUrl: 'https://kong.example.com/default/test' })) as never
    )
    render(<DeployModal agent={makeAgent()} providers={providers} open={true} onClose={vi.fn()} />)
    expect(await screen.findByText('https://kong.example.com/default/test')).toBeInTheDocument()
  })

  it('member: 操作按钮全部 disabled，聊天按钮可用', async () => {
    setAuthRole('member')
    vi.mocked(agentApi.getDeployment).mockResolvedValue(
      mockResponse(
        makeStatus({
          status: 'running',
          health: 'healthy',
          hostPort: 8080,
          runtimeUrl: 'http://localhost:8080',
        })
      ) as never
    )

    render(<DeployModal agent={makeAgent()} providers={providers} open={true} onClose={vi.fn()} />)

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /停止/ })).toBeDisabled()
    })
    expect(screen.getByRole('button', { name: /归档/ })).toBeDisabled()
    expect(screen.getByRole('button', { name: /重新部署/ })).toBeDisabled()
    // 聊天是 member 唯一可用操作
    expect(screen.getByRole('button', { name: /聊天/ })).not.toBeDisabled()
  })

  it('clicking deploy button triggers API call', async () => {
    const user = userEvent.setup()
    vi.mocked(agentApi.getDeployment).mockResolvedValue(mockResponse(makeStatus({ status: 'not_found' })) as never)
    vi.mocked(agentApi.deploy).mockResolvedValue(mockResponse({}) as never)

    render(<DeployModal agent={makeAgent()} providers={providers} open={true} onClose={vi.fn()} />)

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /部署/ })).toBeInTheDocument()
    })

    await user.click(screen.getByRole('button', { name: /部署/ }))

    await waitFor(() => {
      // handleDeploy() is invoked with no argument; the defaults
      // `force = false` and `rotateKey = false` are what reach agentApi.deploy.
      expect(agentApi.deploy).toHaveBeenCalledWith('general', false, false)
    })
  })

  it('clicking stop triggers API call', async () => {
    const user = userEvent.setup()
    vi.mocked(agentApi.getDeployment).mockResolvedValue(
      mockResponse(makeStatus({ status: 'running', health: 'healthy' })) as never
    )
    vi.mocked(agentApi.stopDeployment).mockResolvedValue(mockResponse({}) as never)

    render(<DeployModal agent={makeAgent()} providers={providers} open={true} onClose={vi.fn()} />)

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /停止/ })).toBeInTheDocument()
    })

    await user.click(screen.getByRole('button', { name: /停止/ }))

    await waitFor(() => {
      expect(agentApi.stopDeployment).toHaveBeenCalledWith('general')
    })
  })

  it('clicking delete triggers API call', async () => {
    const user = userEvent.setup()
    vi.mocked(agentApi.getDeployment).mockResolvedValue(
      mockResponse(makeStatus({ status: 'stopped' })) as never
    )
    vi.mocked(agentApi.deleteDeployment).mockResolvedValue(mockResponse({}) as never)

    render(<DeployModal agent={makeAgent()} providers={providers} open={true} onClose={vi.fn()} />)

    // stopped state exposes "归档" (archive, calls deleteDeployment with purge=false),
    // not a literal "删除" button.
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /归档/ })).toBeInTheDocument()
    })

    await user.click(screen.getByRole('button', { name: /归档/ }))

    await waitFor(() => {
      expect(agentApi.deleteDeployment).toHaveBeenCalledWith('general')
    })
  })

  it('shows confirmation modal before redeploy and defaults to not rotating key', async () => {
    const user = userEvent.setup()
    vi.mocked(agentApi.getDeployment).mockResolvedValue(
      mockResponse(makeStatus({ status: 'running', health: 'healthy' })) as never
    )
    vi.mocked(agentApi.deploy).mockResolvedValue(mockResponse({}) as never)

    render(<DeployModal agent={makeAgent()} providers={providers} open={true} onClose={vi.fn()} />)

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /重新部署/ })).toBeInTheDocument()
    })

    await user.click(screen.getByRole('button', { name: /重新部署/ }))

    await waitFor(() => {
      expect(screen.getByText(/重新部署将重新创建容器/)).toBeInTheDocument()
    })

    await user.click(screen.getByRole('button', { name: /确认重新部署/ }))

    await waitFor(() => {
      expect(agentApi.deploy).toHaveBeenCalledWith('general', true, false)
    })
  })

  it('passes rotate_key=true when checkbox is checked', async () => {
    const user = userEvent.setup()
    vi.mocked(agentApi.getDeployment).mockResolvedValue(
      mockResponse(makeStatus({ status: 'running', health: 'healthy' })) as never
    )
    vi.mocked(agentApi.deploy).mockResolvedValue(mockResponse({}) as never)

    render(<DeployModal agent={makeAgent()} providers={providers} open={true} onClose={vi.fn()} />)

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /重新部署/ })).toBeInTheDocument()
    })

    await user.click(screen.getByRole('button', { name: /重新部署/ }))

    await waitFor(() => {
      expect(screen.getByText(/同时轮转 API Key/)).toBeInTheDocument()
    })

    await user.click(screen.getByRole('checkbox'))
    await user.click(screen.getByRole('button', { name: /确认重新部署/ }))

    await waitFor(() => {
      expect(agentApi.deploy).toHaveBeenCalledWith('general', true, true)
    })
  })
})

// stepItem locates an AntD Steps item by its title text.
const stepItem = (name: string) =>
  screen.getByText(name).closest('.ant-steps-item')!

describe('DeployModal live status', () => {
  it('keeps step 3 (等待运行) in progress while health is starting', async () => {
    vi.mocked(agentApi.getDeployment).mockResolvedValue(
      mockResponse(makeStatus({ status: 'running', health: 'starting', hostPort: 8080 })) as never
    )

    render(<DeployModal agent={makeAgent()} providers={providers} open={true} onClose={vi.fn()} />)

    await waitFor(() => {
      expect(stepItem('等待运行')).toHaveClass('ant-steps-item-process')
    })
    expect(stepItem('健康检查通过')).toHaveClass('ant-steps-item-wait')
  })

  it('finishes all steps when healthy', async () => {
    vi.mocked(agentApi.getDeployment).mockResolvedValue(
      mockResponse(makeStatus({ status: 'running', health: 'healthy', hostPort: 8080 })) as never
    )

    render(<DeployModal agent={makeAgent()} providers={providers} open={true} onClose={vi.fn()} />)

    await waitFor(() => {
      expect(stepItem('健康检查通过')).toHaveClass('ant-steps-item-finish')
    })
  })

  it('shows composed live status line with gateway message', async () => {
    vi.mocked(agentApi.getDeployment).mockResolvedValue(
      mockResponse(
        makeStatus({ status: 'running', health: 'starting', hostPort: 8080, message: 'Kong 网关路由检测中' })
      ) as never
    )

    render(<DeployModal agent={makeAgent()} providers={providers} open={true} onClose={vi.fn()} />)

    await waitFor(() => {
      expect(screen.getByTestId('deploy-status-line')).toHaveTextContent(
        '容器运行中 · 健康检查中 · Kong 网关路由检测中'
      )
    })
  })

  it('shows creating state for transient docker status', async () => {
    vi.mocked(agentApi.getDeployment).mockResolvedValue(
      mockResponse(makeStatus({ status: 'created' })) as never
    )

    render(<DeployModal agent={makeAgent()} providers={providers} open={true} onClose={vi.fn()} />)

    await waitFor(() => {
      expect(stepItem('等待运行')).toHaveClass('ant-steps-item-process')
    })
    expect(screen.getByTestId('deploy-status-line')).toHaveTextContent('容器创建中')
  })

  it('keeps a single polling chain under StrictMode double-mount', async () => {
    vi.useFakeTimers()
    try {
      // Mid-state status keeps every chain fast-polling (2s), so duplicated
      // chains are directly visible as doubled request counts.
      vi.mocked(agentApi.getDeployment).mockResolvedValue(
        mockResponse(makeStatus({ status: 'running', health: 'starting' })) as never
      )

      render(
        <StrictMode>
          <DeployModal agent={makeAgent()} providers={providers} open={true} onClose={vi.fn()} />
        </StrictMode>
      )

      // Flush the initial fetch and the first scheduling round.
      await vi.advanceTimersByTimeAsync(100)
      // First poll fires after the initial 15s slow interval; mid-state then
      // switches the chain(s) to the 2s fast interval.
      await vi.advanceTimersByTimeAsync(15100)
      const before = vi.mocked(agentApi.getDeployment).mock.calls.length
      await vi.advanceTimersByTimeAsync(6000)
      const delta = vi.mocked(agentApi.getDeployment).mock.calls.length - before

      // Exactly one chain polling every 2s → 3 calls in 6s.
      expect(delta).toBe(3)
    } finally {
      vi.useRealTimers()
    }
  })
})
