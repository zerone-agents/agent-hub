import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ConfigProvider } from 'antd'
import { antdTheme } from '@/lib/antd-theme'
import LoginPage from './LoginPage'

// The login page now queries /auth/mode to decide which UI to render, so the
// auth API mock must expose getAuthMode (plus login/loginWithPassword). These
// tests cover the builtin (username+password) flow — the new feature. The
// casdoor SSO path is an unchanged one-line redirect covered elsewhere.
vi.mock('@/api/auth', () => ({
  authApi: {
    login: vi.fn(),
    checkOrg: vi.fn(),
    getAuthMode: vi.fn(),
    loginWithPassword: vi.fn()
  }
}))

import { authApi } from '@/api/auth'

function renderLogin() {
  // builtin 流程会经真实 auth store 落 token 到 localStorage，跨用例清理，
  // 否则后续 casdoor 用例因 token 存在走 useUserInfo 分支卡在 LoadingState。
  localStorage.clear()
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false } }
  })
  return render(
    <ConfigProvider theme={antdTheme}>
      <QueryClientProvider client={qc}>
        <MemoryRouter>
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
    expect(authApi.login).toHaveBeenCalledWith('acme')
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
    expect(authApi.login).toHaveBeenCalledWith()
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
