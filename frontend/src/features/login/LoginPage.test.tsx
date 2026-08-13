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
    getAuthMode: vi.fn(),
    loginWithPassword: vi.fn()
  }
}))

import { authApi } from '@/api/auth'

function renderLogin() {
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
