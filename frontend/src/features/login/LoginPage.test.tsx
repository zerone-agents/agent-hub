import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ConfigProvider } from 'antd'
import type { AxiosResponse } from 'axios'
import { antdTheme } from '@/lib/antd-theme'
import { authApi, type UserInfoResponse } from '@/api/auth'
import type { ApiResponse } from '@/types/api'
import LoginPage from './LoginPage'

// 登录回源用例需断言 navigate 目标；jsdom 不能整页跳转（Not implemented:
// navigation），useNavigate 打桩为 vi.fn。文件级部分 mock：react-router 其余
// 导出（MemoryRouter/useSearchParams 等）保持原样，既有用例不受影响。
const { navigateMock } = vi.hoisted(() => ({ navigateMock: vi.fn() }))
vi.mock('react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('react-router')>()
  return { ...actual, useNavigate: () => navigateMock }
})

// The login page now queries /auth/mode to decide which UI to render, so the
// auth API mock must expose getAuthMode (plus login/loginWithPassword). These
// tests cover the builtin (username+password) flow — the new feature. The
// casdoor SSO path is an unchanged one-line redirect covered elsewhere.
// getUserInfo 供已登录回源用例（token 存在时 useUserInfo 会发起查询）。
vi.mock('@/api/auth', () => ({
  authApi: {
    login: vi.fn(),
    checkOrg: vi.fn(),
    getAuthMode: vi.fn(),
    loginWithPassword: vi.fn(),
    getUserInfo: vi.fn()
  }
}))

function renderLogin(initialEntry = '/', opts: { token?: string } = {}) {
  // builtin 流程会经真实 auth store 落 token 到 localStorage，跨用例清理，
  // 否则后续 casdoor 用例因 token 存在走 useUserInfo 分支卡在 LoadingState。
  localStorage.clear()
  // 已登录回源用例：清理后预置 token，让组件渲染时 getAccessToken() 即非空。
  if (opts.token) localStorage.setItem('access_token', opts.token)
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false } }
  })
  return render(
    <ConfigProvider theme={antdTheme}>
      <QueryClientProvider client={qc}>
        <MemoryRouter initialEntries={[initialEntry]}>
          <LoginPage />
        </MemoryRouter>
      </QueryClientProvider>
    </ConfigProvider>
  )
}

describe('LoginPage (builtin mode)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(authApi.getAuthMode).mockResolvedValue({ mode: 'builtin', initialized: true })
    vi.mocked(authApi.loginWithPassword).mockResolvedValue({
      accessToken: 'a',
      refreshToken: 'r',
      expiresIn: 7200
    })
  })

  it('renders brand and builtin login form', async () => {
    renderLogin()
    expect(await screen.findByText('Zerone Agent Hub')).toBeInTheDocument()
    expect(screen.getByPlaceholderText('用户名')).toBeInTheDocument()
    expect(screen.getByPlaceholderText('密码')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '登录' })).toBeInTheDocument()
  })

  it('calls loginWithPassword on builtin form submit', async () => {
    const user = userEvent.setup()
    renderLogin()
    await screen.findByPlaceholderText('用户名')
    await user.type(screen.getByPlaceholderText('用户名'), 'alice')
    await user.type(screen.getByPlaceholderText('密码'), 'Passw0rd!')
    await user.click(screen.getByRole('button', { name: '登录' }))
    expect(authApi.loginWithPassword).toHaveBeenCalledWith('alice', 'Passw0rd!')
  })
})

describe('LoginPage (casdoor multi-org)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(authApi.getAuthMode).mockResolvedValue({ mode: 'casdoor', initialized: true, multiOrg: true })
    vi.mocked(authApi.checkOrg).mockResolvedValue({ exists: true })
  })

  function renderCasdoorLogin() {
    return renderLogin()
  }

  it('renders 更多 entry when multiOrg=true and hides it when false', async () => {
    const view = renderCasdoorLogin()
    expect(await screen.findByRole('button', { name: '登录 Agent Hub' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '更多' })).toBeInTheDocument()
    view.unmount()

    vi.mocked(authApi.getAuthMode).mockResolvedValue({ mode: 'casdoor', initialized: true, multiOrg: false })
    renderLogin()
    expect(await screen.findByRole('button', { name: '登录 Agent Hub' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '更多' })).not.toBeInTheDocument()
  })

  it('expands org input, prechecks then logs in with org', async () => {
    const user = userEvent.setup()
    renderCasdoorLogin()
    await screen.findByRole('button', { name: '登录 Agent Hub' })
    await user.click(screen.getByRole('button', { name: '更多' }))
    await user.type(screen.getByPlaceholderText('留空使用默认组织'), 'acme')
    await user.click(screen.getByRole('button', { name: '确认' }))
    await screen.findByRole('button', { name: '确认' })
    expect(authApi.checkOrg).toHaveBeenCalledWith('acme')
    // 无 ?redirect= 时 sanitizeRedirect 回退 '/'，login 第二参恒有值。
    expect(authApi.login).toHaveBeenCalledWith('acme', '/')
  })

  it('shows inline error and does not redirect when checkOrg fails', async () => {
    vi.mocked(authApi.checkOrg).mockRejectedValue(new Error('org not found'))
    const user = userEvent.setup()
    renderCasdoorLogin()
    await screen.findByRole('button', { name: '登录 Agent Hub' })
    await user.click(screen.getByRole('button', { name: '更多' }))
    await user.type(screen.getByPlaceholderText('留空使用默认组织'), 'ghost')
    await user.click(screen.getByRole('button', { name: '确认' }))
    expect(await screen.findByText('组织不存在或未注册，请检查后重试')).toBeInTheDocument()
    expect(authApi.login).not.toHaveBeenCalled()
  })

  it('empty org confirm logs in with default (no org arg)', async () => {
    const user = userEvent.setup()
    renderCasdoorLogin()
    await screen.findByRole('button', { name: '登录 Agent Hub' })
    await user.click(screen.getByRole('button', { name: '更多' }))
    await user.click(screen.getByRole('button', { name: '确认' }))
    expect(authApi.checkOrg).not.toHaveBeenCalled()
    // 空 org 分支同样显式携带 redirect（默认 '/'）。
    expect(authApi.login).toHaveBeenCalledWith(undefined, '/')
  })

  it('main login button is disabled while 更多 is expanded and re-enabled on collapse', async () => {
    const user = userEvent.setup()
    renderCasdoorLogin()
    const mainBtn = await screen.findByRole('button', { name: '登录 Agent Hub' })
    expect(mainBtn).toBeEnabled()

    await user.click(screen.getByRole('button', { name: '更多' }))
    // 展开时主按钮禁用：组织输入必须走「确认」的预检流程
    expect(screen.getByRole('button', { name: '登录 Agent Hub' })).toBeDisabled()

    await user.click(screen.getByRole('button', { name: '收起' }))
    expect(screen.getByRole('button', { name: '登录 Agent Hub' })).toBeEnabled()
  })
})

describe('LoginPage mode 查询失败（fail-closed）', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // 429 / 网络错误：getAuthMode 抛错，绝不允许 fallback 到 builtin 表单
    // （casdoor 部署没有本地登录端点，渲染 builtin 表单必失败且误导用户）。
    vi.mocked(authApi.getAuthMode).mockRejectedValue(new Error('请求过于频繁，请稍后再试'))
  })

  // useAuthMode 自带 retry:1（指数退避 ~1s），等待错误卡渲染需放宽超时。
  const waitErrorCard = () => screen.findByText(/无法获取登录方式/, undefined, { timeout: 5000 })

  it('渲染错误卡而非 builtin 用户名/密码表单', async () => {
    renderLogin()
    expect(await waitErrorCard()).toBeInTheDocument()
    expect(screen.queryByPlaceholderText('用户名')).not.toBeInTheDocument()
    expect(screen.queryByPlaceholderText('密码')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '登录' })).not.toBeInTheDocument()
  })

  it('展示错误提示与重试按钮，点击重试恢复 casdoor 登录页', async () => {
    const user = userEvent.setup()
    renderLogin()
    expect(await waitErrorCard()).toBeInTheDocument()
    const retry = screen.getByRole('button', { name: /重试/ })
    vi.mocked(authApi.getAuthMode).mockResolvedValue({ mode: 'casdoor', initialized: true, multiOrg: false })
    await user.click(retry)
    expect(await screen.findByRole('button', { name: '登录 Agent Hub' })).toBeInTheDocument()
  })
})

describe('LoginPage 登录回源（?redirect=）', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(authApi.getAuthMode).mockResolvedValue({ mode: 'builtin', initialized: true })
    vi.mocked(authApi.loginWithPassword).mockResolvedValue({
      accessToken: 'a',
      refreshToken: 'r',
      expiresIn: 7200
    })
    // 登录成功落 token 后 userinfo 查询会被启用；已登录回源用例也依赖它出 user。
    vi.mocked(authApi.getUserInfo).mockResolvedValue({
      data: { success: true, data: { user_id: 'u1', display_name: 'Alice' } }
    } as unknown as AxiosResponse<ApiResponse<UserInfoResponse>>)
  })

  async function submitBuiltinLogin(user: ReturnType<typeof userEvent.setup>) {
    await screen.findByPlaceholderText('用户名')
    await user.type(screen.getByPlaceholderText('用户名'), 'alice')
    await user.type(screen.getByPlaceholderText('密码'), 'Passw0rd!')
    await user.click(screen.getByRole('button', { name: '登录' }))
  }

  it('?redirect=/agents/chat：builtin 登录成功后 navigate 到回源', async () => {
    const user = userEvent.setup()
    renderLogin('/login?redirect=/agents/chat')
    await submitBuiltinLogin(user)
    await waitFor(() => { expect(navigateMock).toHaveBeenCalledWith('/agents/chat', { replace: true }) })
  })

  it('?redirect=//evil.com：sanitize 回退后 navigate("/")', async () => {
    const user = userEvent.setup()
    renderLogin('/login?redirect=//evil.com')
    await submitBuiltinLogin(user)
    await waitFor(() => { expect(navigateMock).toHaveBeenCalledWith('/', { replace: true }) })
  })

  it('casdoor 模式 + redirect：主登录按钮把 redirect 作为第二参传给 authApi.login', async () => {
    vi.mocked(authApi.getAuthMode).mockResolvedValue({ mode: 'casdoor', initialized: true, multiOrg: false })
    const user = userEvent.setup()
    renderLogin('/login?redirect=/agents/chat')
    await user.click(await screen.findByRole('button', { name: '登录 Agent Hub' }))
    expect(authApi.login).toHaveBeenCalledWith(undefined, '/agents/chat')
  })

  it('已登录（token+user）访问 /login?redirect= 直接跳回源', async () => {
    renderLogin('/login?redirect=/agents/chat', { token: 'tok' })
    await waitFor(() => { expect(navigateMock).toHaveBeenCalledWith('/agents/chat', { replace: true }) })
  })
})
